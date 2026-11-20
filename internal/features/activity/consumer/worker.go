package activity_consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	kafka_consumer "github.com/Reigor1/20h-project/internal/core/kafka/consumer"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	activity_dlq "github.com/Reigor1/20h-project/internal/features/activity/dlq"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const pollTimeout = 100 * time.Millisecond

var ErrPendingQueueFull = errors.New("Kafka pending queue is full")

type KafkaConsumer interface {
	Subscribe(topic string) error
	Poll(timeout time.Duration) kafka_consumer.PollResult
	CommitMessage(message *kafka_consumer.Message) error
	Pause() error
	Resume() error
	IsAssigned(message *kafka_consumer.Message) bool
}

type KafkaProducer interface {
	Publish(
		ctx context.Context,
		topic string,
		message kafka_producer.Message,
	) error
}

type ActivityService interface {
	Record(ctx context.Context, event core_events.Envelope) (inserted bool, err error)
}

type Instrumentation struct {
	Metrics *core_observability.KafkaMetrics
	Tracer  trace.Tracer
}

type Worker struct {
	topic              string
	dlqTopic           string
	retryBaseDelay     time.Duration
	retryMaxDelay      time.Duration
	maxPendingMessages int
	shutdownTimeout    time.Duration

	consumer KafkaConsumer
	producer KafkaProducer
	service  ActivityService
	logger   *zap.Logger
	metrics  *core_observability.KafkaMetrics
	tracer   trace.Tracer
}

func NewWorker(
	topic string,
	dlqTopic string,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	shutdownTimeout time.Duration,
	maxPendingMessages int,
	consumer KafkaConsumer,
	producer KafkaProducer,
	service ActivityService,
	logger *zap.Logger,
	instrumentation ...Instrumentation,
) (*Worker, error) {
	switch {
	case strings.TrimSpace(topic) == "":
		return nil, fmt.Errorf("Kafka topic must not be empty")
	case strings.TrimSpace(dlqTopic) == "":
		return nil, fmt.Errorf("Kafka DLQ topic must not be empty")
	case topic == dlqTopic:
		return nil, fmt.Errorf("Kafka topic and DLQ topic must differ")
	case retryBaseDelay <= 0:
		return nil, fmt.Errorf("retry base delay must be positive")
	case retryMaxDelay < retryBaseDelay:
		return nil, fmt.Errorf("retry max delay must not be less than base delay")
	case shutdownTimeout <= 0:
		return nil, fmt.Errorf("shutdown timeout must be positive")
	case maxPendingMessages <= 0:
		return nil, fmt.Errorf("max pending messages must be positive")
	case consumer == nil:
		return nil, fmt.Errorf("Kafka Consumer must not be nil")
	case producer == nil:
		return nil, fmt.Errorf("Kafka Producer must not be nil")
	case service == nil:
		return nil, fmt.Errorf("Activity Service must not be nil")
	case logger == nil:
		return nil, fmt.Errorf("Worker logger must not be nil")
	}

	worker := &Worker{
		topic:              topic,
		dlqTopic:           dlqTopic,
		retryBaseDelay:     retryBaseDelay,
		retryMaxDelay:      retryMaxDelay,
		maxPendingMessages: maxPendingMessages,
		shutdownTimeout:    shutdownTimeout,
		consumer:           consumer,
		producer:           producer,
		service:            service,
		logger:             logger,
	}
	if len(instrumentation) > 0 {
		worker.metrics = instrumentation[0].Metrics
		worker.tracer = instrumentation[0].Tracer
	}

	return worker, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.consumer.Subscribe(w.topic); err != nil {
		return fmt.Errorf("subscribe Activity Worker: %w", err)
	}

	w.logger.Info(
		"Activity Worker started",
		zap.String("topic", w.topic),
		zap.String("dlq_topic", w.dlqTopic),
		zap.Duration("shutdown_timeout", w.shutdownTimeout),
	)

	// Messages fetched while partitions are paused are kept here.
	// They are processed in the order returned by Poll().
	var pending []*kafka_consumer.Message

	for {
		if ctx.Err() != nil {
			w.logger.Info("Activity Worker stopped")
			return nil
		}

		var message *kafka_consumer.Message

		if len(pending) > 0 {
			message = pending[0]
			pending = pending[1:]
		} else {
			result := w.consumer.Poll(pollTimeout)

			// A message can be returned by Poll at the same time as
			// shutdown is requested. Do not start another message then.
			if ctx.Err() != nil {
				w.logger.Info("Activity Worker stopped before starting another message")
				return nil
			}

			if result.Err != nil {
				if result.Fatal {
					return fmt.Errorf(
						"fatal Kafka Consumer error: %w",
						result.Err,
					)
				}

				w.logger.Warn(
					"Kafka Consumer reported a recoverable error",
					zap.Error(result.Err),
				)

				continue
			}

			message = result.Message
		}

		if message == nil {
			continue
		}

		// The signal context controls intake and retry waits, but does not
		// cancel an operation already in progress. After the shutdown signal,
		// the operation receives the configured grace period to finish.
		processCtx, cancelProcess := context.WithCancel(
			context.WithoutCancel(ctx),
		)
		processDone := make(chan struct{})
		stopGraceTimer := context.AfterFunc(ctx, func() {
			timer := time.NewTimer(w.shutdownTimeout)
			defer timer.Stop()

			select {
			case <-timer.C:
				cancelProcess()
			case <-processDone:
			}
		})

		err := w.processMessageWithStop(
			processCtx,
			ctx,
			message,
			&pending,
		)

		close(processDone)
		stopGraceTimer()
		cancelProcess()

		if err != nil {
			if ctx.Err() != nil {
				fields := w.messageLogFields(message)
				fields = append(fields, zap.Error(err))
				w.logger.Warn(
					"Activity Worker shutdown interrupted the current message; it will be redelivered",
					fields...,
				)
				return nil
			}

			if errors.Is(err, kafka_consumer.ErrAssignmentLost) {
				w.logger.Info(
					"Activity Worker lost partition assignment; message will be redelivered",
					zap.String("topic", message.Topic),
					zap.Int32("partition", message.Partition),
					zap.Int64("offset", message.Offset),
				)

				continue
			}

			return fmt.Errorf(
				"process Kafka message %s partition %d offset %d: %w",
				message.Topic,
				message.Partition,
				message.Offset,
				err,
			)
		}
	}
}

func (w *Worker) processMessage(
	ctx context.Context,
	message *kafka_consumer.Message,
	pending *[]*kafka_consumer.Message,
) error {
	return w.processMessageWithStop(ctx, ctx, message, pending)
}

func (w *Worker) processMessageWithStop(
	ctx context.Context,
	stopCtx context.Context,
	message *kafka_consumer.Message,
	pending *[]*kafka_consumer.Message,
) (returnErr error) {
	if !w.consumer.IsAssigned(message) {
		return kafka_consumer.ErrAssignmentLost
	}

	startedAt := time.Now()
	metadata := readEventMetadata(message.Value)
	ctx = propagation.TraceContext{}.Extract(
		ctx,
		traceCarrierFromMessage(message.Headers),
	)

	var span trace.Span
	if w.tracer != nil {
		ctx, span = w.tracer.Start(
			ctx,
			"kafka.process",
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "kafka"),
				attribute.String("messaging.destination.name", message.Topic),
				attribute.Int("messaging.destination.partition.id", int(message.Partition)),
				attribute.Int64("messaging.kafka.offset", message.Offset),
				attribute.String("event.id", metadata.EventID),
				attribute.String("event.type", metadata.EventType),
				attribute.Int64("task.id", metadata.TaskID),
			),
		)
	}

	outcome := "failed"
	defer func() {
		if w.metrics != nil {
			w.metrics.RecordActivityProcessingDuration(
				ctx,
				time.Since(startedAt),
				metadata.EventType,
				outcome,
			)
		}
		if span != nil {
			if returnErr != nil {
				span.RecordError(returnErr)
				span.SetStatus(codes.Error, returnErr.Error())
			} else {
				span.SetAttributes(attribute.String("activity.processing.outcome", outcome))
			}
			span.End()
		}
	}()

	event, err := core_events.Unmarshal(message.Value)
	if err != nil {
		if !isPermanentMessageError(err) {
			return fmt.Errorf("classify event message: %w", err)
		}
		if err := w.moveToDLQ(ctx, stopCtx, message, err, pending); err != nil {
			return err
		}
		outcome = "dlq"
		return nil
	}

	metadata = eventMetadata{
		EventID:   event.EventID,
		EventType: string(event.EventType),
		TaskID:    event.AggregateID,
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("event.id", event.EventID),
			attribute.String("event.type", string(event.EventType)),
			attribute.Int64("task.id", event.AggregateID),
		)
	}

	var inserted bool
	attempts, paused, err := w.retry(
		ctx,
		stopCtx,
		message,
		func() error {
			var recordErr error
			inserted, recordErr = w.service.Record(ctx, event)
			return recordErr
		},
		pending,
		"record Activity event",
	)
	if err != nil {
		return w.resumeAfterAssignmentLoss(paused, err)
	}

	paused, err = w.commitWithRetry(
		ctx,
		stopCtx,
		message,
		pending,
		paused,
	)
	if err != nil {
		return w.resumeAfterAssignmentLoss(paused, err)
	}

	if paused {
		if err := w.consumer.Resume(); err != nil {
			return fmt.Errorf("resume Kafka Consumer: %w", err)
		}
	}

	if w.metrics != nil {
		w.metrics.RecordActivityProcessed(ctx, string(event.EventType))
		if !inserted {
			w.metrics.RecordActivityDuplicate(ctx, string(event.EventType))
		}
	}
	if span != nil {
		span.SetAttributes(
			attribute.Bool("messaging.message.duplicate", !inserted),
			attribute.Int("activity.processing.attempt_count", attempts),
		)
	}
	outcome = "processed"

	w.logger.Info(
		"Activity event processed",
		zap.String("event_id", event.EventID),
		zap.String("event_type", string(event.EventType)),
		zap.Int64("task_id", event.AggregateID),
		zap.Int64("task_version", event.AggregateVersion),
		zap.Int("attempt_count", attempts),
		zap.String("topic", message.Topic),
		zap.Int32("partition", message.Partition),
		zap.Int64("offset", message.Offset),
		zap.Bool("duplicate", !inserted),
	)

	return nil
}

func (w *Worker) moveToDLQ(
	ctx context.Context,
	stopCtx context.Context,
	message *kafka_consumer.Message,
	cause error,
	pending *[]*kafka_consumer.Message,
) error {
	record := activity_dlq.Record{
		EventID:         extractEventID(message.Value),
		OriginalPayload: message.Value,
		OriginalKey:     message.Key,
		SourceTopic:     message.Topic,
		SourcePartition: message.Partition,
		SourceOffset:    message.Offset,
		FailedAt:        time.Now().UTC(),
		Error:           cause.Error(),
		AttemptCount:    1,
	}

	payload, err := activity_dlq.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal DLQ record: %w", err)
	}

	attempts, paused, err := w.retry(
		ctx,
		stopCtx,
		message,
		func() error {
			return w.producer.Publish(
				ctx,
				w.dlqTopic,
				kafka_producer.Message{
					Key:     []byte(record.EventID),
					Value:   payload,
					Headers: traceHeadersFromContext(ctx),
				},
			)
		},
		pending,
		"publish DLQ record",
	)
	if err != nil {
		return w.resumeAfterAssignmentLoss(paused, err)
	}

	metadata := readEventMetadata(message.Value)
	if w.metrics != nil {
		w.metrics.RecordActivityFailed(ctx, metadata.EventType)
		w.metrics.RecordActivityDLQMessage(ctx, metadata.EventType)
	}

	// Commit the source offset only after the DLQ Delivery Report succeeds.
	paused, err = w.commitWithRetry(
		ctx,
		stopCtx,
		message,
		pending,
		paused,
	)
	if err != nil {
		return w.resumeAfterAssignmentLoss(paused, err)
	}

	if paused {
		if err := w.consumer.Resume(); err != nil {
			return fmt.Errorf("resume Kafka Consumer: %w", err)
		}
	}

	w.logger.Error(
		"Activity event moved to DLQ",
		zap.String("event_id", record.EventID),
		zap.Int64("task_id", metadata.TaskID),
		zap.String("event_type", metadata.EventType),
		zap.String("source_topic", record.SourceTopic),
		zap.Int32("source_partition", record.SourcePartition),
		zap.Int64("source_offset", record.SourceOffset),
		zap.Int("attempt_count", attempts),
		zap.Error(cause),
	)

	return nil
}

func (w *Worker) retry(
	ctx context.Context,
	stopCtx context.Context,
	message *kafka_consumer.Message,
	operation func() error,
	pending *[]*kafka_consumer.Message,
	operationName string,
) (int, bool, error) {
	if !w.consumer.IsAssigned(message) {
		return 0, false, kafka_consumer.ErrAssignmentLost
	}

	attempt := 1

	if err := operation(); err == nil {
		return attempt, false, nil
	} else {
		fields := []zap.Field{
			zap.String("operation", operationName),
			zap.Int("attempt", attempt),
		}
		fields = append(fields, w.messageLogFields(message)...)
		fields = append(fields, zap.Error(err))

		w.logger.Warn(
			"Activity Worker operation failed; retrying",
			fields...,
		)
	}

	if err := stopCtx.Err(); err != nil {
		return attempt, false, err
	}

	if err := w.consumer.Pause(); err != nil {
		if !w.consumer.IsAssigned(message) {
			return attempt, false, kafka_consumer.ErrAssignmentLost
		}

		return attempt, false, fmt.Errorf(
			"pause Kafka Consumer before retry: %w",
			err,
		)
	}

	for {
		delay := retryDelay(
			w.retryBaseDelay,
			w.retryMaxDelay,
			attempt,
		)

		if err := w.waitBackoff(
			ctx,
			stopCtx,
			message,
			delay,
			pending,
		); err != nil {
			return attempt, true, err
		}

		if !w.consumer.IsAssigned(message) {
			return attempt, true, kafka_consumer.ErrAssignmentLost
		}
		if err := stopCtx.Err(); err != nil {
			return attempt, true, err
		}

		attempt++

		if err := operation(); err != nil {
			w.logger.Warn(
				"Activity Worker retry failed",
				zap.String("operation", operationName),
				zap.Int("attempt", attempt),
				zap.Duration(
					"next_delay",
					retryDelay(
						w.retryBaseDelay,
						w.retryMaxDelay,
						attempt,
					),
				),
				zap.Error(err),
			)

			continue
		}

		return attempt, true, nil
	}
}

func (w *Worker) waitBackoff(
	ctx context.Context,
	stopCtx context.Context,
	currentMessage *kafka_consumer.Message,
	delay time.Duration,
	pending *[]*kafka_consumer.Message,
) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	// Poll at a bounded cadence while waiting. Poll(0) is non-blocking;
	// the ticker keeps the loop from spinning when no events are available.
	pollTicker := time.NewTicker(pollTimeout)
	defer pollTicker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := stopCtx.Err(); err != nil {
			return err
		}

		result := w.consumer.Poll(0)

		if err := stopCtx.Err(); err != nil {
			return err
		}

		if result.Err != nil {
			if result.Fatal {
				return fmt.Errorf(
					"fatal Kafka Consumer error during retry: %w",
					result.Err,
				)
			}

			w.logger.Warn(
				"Kafka Consumer reported an error during retry",
				zap.Error(result.Err),
			)
		}

		if result.Message != nil {
			if len(*pending) >= w.maxPendingMessages {
				return fmt.Errorf(
					"%w: limit=%d",
					ErrPendingQueueFull,
					w.maxPendingMessages,
				)
			}

			*pending = append(*pending, result.Message)
		}

		if !w.consumer.IsAssigned(currentMessage) {
			return kafka_consumer.ErrAssignmentLost
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stopCtx.Done():
			return stopCtx.Err()
		case <-timer.C:
			return nil
		case <-pollTicker.C:
		}
	}
}

func (w *Worker) commitWithRetry(
	ctx context.Context,
	stopCtx context.Context,
	message *kafka_consumer.Message,
	pending *[]*kafka_consumer.Message,
	paused bool,
) (bool, error) {
	attempt := 1

	for {
		if !w.consumer.IsAssigned(message) {
			return paused, kafka_consumer.ErrAssignmentLost
		}

		err := w.consumer.CommitMessage(message)
		if err == nil {
			return paused, nil
		}

		if errors.Is(err, kafka_consumer.ErrAssignmentLost) ||
			!w.consumer.IsAssigned(message) {
			return paused, kafka_consumer.ErrAssignmentLost
		}

		if !kafka_consumer.IsRetriableError(err) {
			return paused, fmt.Errorf("commit Kafka offset: %w", err)
		}
		if stopCtx.Err() != nil {
			return paused, stopCtx.Err()
		}

		if !paused {
			if err := w.consumer.Pause(); err != nil {
				if !w.consumer.IsAssigned(message) {
					return paused, kafka_consumer.ErrAssignmentLost
				}

				return paused, fmt.Errorf(
					"pause Kafka Consumer before commit retry: %w",
					err,
				)
			}

			paused = true
		}

		delay := retryDelay(
			w.retryBaseDelay,
			w.retryMaxDelay,
			attempt,
		)

		fields := w.messageLogFields(message)
		fields = append(fields,
			zap.Int("attempt", attempt),
			zap.Duration("retry_delay", delay),
			zap.Error(err),
		)
		w.logger.Warn(
			"Kafka offset commit failed; retrying",
			fields...,
		)

		if err := w.waitBackoff(
			ctx,
			stopCtx,
			message,
			delay,
			pending,
		); err != nil {
			return paused, err
		}

		attempt++
	}
}

func (w *Worker) resumeAfterAssignmentLoss(
	paused bool,
	cause error,
) error {
	if !paused || !errors.Is(
		cause,
		kafka_consumer.ErrAssignmentLost,
	) {
		return cause
	}

	if err := w.consumer.Resume(); err != nil {
		return fmt.Errorf(
			"resume Kafka Consumer after assignment loss: %w",
			err,
		)
	}

	return cause
}

func retryDelay(
	base time.Duration,
	max time.Duration,
	attempt int,
) time.Duration {
	delay := base

	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			return max
		}

		delay *= 2
	}

	if delay > max {
		return max
	}

	return delay
}

func isPermanentMessageError(err error) bool {
	return errors.Is(err, core_events.ErrMalformedJSON) ||
		errors.Is(err, core_events.ErrInvalidEnvelope) ||
		errors.Is(err, core_events.ErrUnsupportedEventType) ||
		errors.Is(err, core_events.ErrUnsupportedSchemaVersion)
}

func extractEventID(payload []byte) string {
	var event struct {
		EventID string `json:"event_id"`
	}

	if err := json.Unmarshal(payload, &event); err != nil {
		return ""
	}

	return event.EventID
}

type eventMetadata struct {
	EventID   string
	EventType string
	TaskID    int64
}

func readEventMetadata(payload []byte) eventMetadata {
	var metadata eventMetadata
	var envelope struct {
		EventID     string `json:"event_id"`
		EventType   string `json:"event_type"`
		AggregateID int64  `json:"aggregate_id"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return metadata
	}
	metadata.EventID = envelope.EventID
	metadata.EventType = envelope.EventType
	metadata.TaskID = envelope.AggregateID
	return metadata
}

func traceCarrierFromMessage(headers []kafka.Header) propagation.MapCarrier {
	carrier := propagation.MapCarrier{}
	for _, header := range headers {
		if header.Key == "traceparent" || header.Key == "tracestate" {
			carrier.Set(header.Key, string(header.Value))
		}
	}
	return carrier
}

func traceHeadersFromContext(ctx context.Context) []kafka.Header {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	headers := make([]kafka.Header, 0, 2)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier.Get(key); value != "" {
			headers = append(headers, kafka.Header{Key: key, Value: []byte(value)})
		}
	}
	return headers
}

func (w *Worker) messageLogFields(message *kafka_consumer.Message) []zap.Field {
	metadata := readEventMetadata(message.Value)
	fields := []zap.Field{
		zap.String("topic", message.Topic),
		zap.Int32("partition", message.Partition),
		zap.Int64("offset", message.Offset),
	}
	if metadata.EventID != "" {
		fields = append(fields, zap.String("event_id", metadata.EventID))
	}
	if metadata.EventType != "" {
		fields = append(fields, zap.String("event_type", metadata.EventType))
	}
	if metadata.TaskID > 0 {
		fields = append(fields, zap.Int64("task_id", metadata.TaskID))
	}
	return fields
}
