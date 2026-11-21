package outbox_publisher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
	outbox_repository "github.com/Reigor1/20h-project/internal/features/outbox/repository"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const repositoryUpdateTimeout = 5 * time.Second

// OutboxRepository defines the lease-based persistence operations used by Publisher.
type OutboxRepository interface {
	ClaimPending(
		ctx context.Context,
		limit int,
		leaseDuration time.Duration,
	) ([]outbox_repository.ClaimedEvent, error)

	MarkPublished(
		ctx context.Context,
		id int64,
		leaseToken string,
	) error

	MarkFailed(
		ctx context.Context,
		id int64,
		leaseToken string,
		publishError error,
		retryAfter time.Duration,
	) error
}

// EventProducer publishes a batch and returns one delivery result per message.
type EventProducer interface {
	PublishBatch(
		ctx context.Context,
		topic string,
		messages []kafka_producer.Message,
	) []error
}

type Instrumentation struct {
	Metrics *core_observability.KafkaMetrics
	Tracer  trace.Tracer
}

// Publisher claims pending Outbox events, publishes them to Kafka, and records delivery outcomes.
type Publisher struct {
	config     Config
	repository OutboxRepository
	producer   EventProducer
	logger     *zap.Logger
	metrics    *core_observability.KafkaMetrics
	tracer     trace.Tracer
}

// NewPublisher validates dependencies and creates a Publisher with optional telemetry instrumentation.
func NewPublisher(
	config Config,
	repository OutboxRepository,
	producer EventProducer,
	logger *zap.Logger,
	instrumentation ...Instrumentation,
) (*Publisher, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if repository == nil {
		return nil, fmt.Errorf("Outbox Repository must not be nil")
	}

	if producer == nil {
		return nil, fmt.Errorf("Kafka Producer must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("Publisher logger must not be nil")
	}

	publisher := &Publisher{
		config:     config,
		repository: repository,
		producer:   producer,
		logger:     logger,
	}
	if len(instrumentation) > 0 {
		publisher.metrics = instrumentation[0].Metrics
		publisher.tracer = instrumentation[0].Tracer
	}

	return publisher, nil
}

// Run polls the Outbox, drains available batches without unnecessary polling delays,
// and gives an active batch a bounded grace period when shutdown is requested.
func (p *Publisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	p.logger.Info(
		"Outbox Publisher started",
		zap.String("topic", p.config.Topic),
		zap.Int("batch_size", p.config.BatchSize),
		zap.Duration("poll_interval", p.config.PollInterval),
		zap.Duration("lease_duration", p.config.LeaseDuration),
	)

	for {
		if ctx.Err() != nil {
			p.logger.Info("Outbox Publisher stopping")
			return nil
		}

		// Keep an active batch alive after the shutdown signal, but only
		// for the configured grace period.
		batchCtx, cancelBatch := context.WithCancel(
			context.WithoutCancel(ctx),
		)
		batchDone := make(chan struct{})
		stopGraceTimer := context.AfterFunc(ctx, func() {
			timer := time.NewTimer(p.config.ShutdownTimeout)
			defer timer.Stop()

			select {
			case <-timer.C:
				cancelBatch()
			case <-batchDone:
			}
		})

		claimedCount, err := p.publishBatch(batchCtx)

		close(batchDone)
		stopGraceTimer()
		cancelBatch()

		if ctx.Err() != nil {
			p.logger.Info("Outbox Publisher stopping")
			return nil
		}

		if err != nil {
			p.logger.Error(
				"Outbox Publisher batch failed",
				zap.Error(err),
			)
		}

		// Drain ready events without waiting for the next polling tick.
		if claimedCount > 0 {
			continue
		}

		select {
		case <-ctx.Done():
			p.logger.Info("Outbox Publisher stopping")
			return nil

		case <-ticker.C:
		}
	}
}

// publishBatch claims a leased batch, propagates trace context to Kafka, publishes records,
// and marks each event published or schedules a retry according to its delivery result.
func (p *Publisher) publishBatch(ctx context.Context) (int, error) {
	events, err := p.repository.ClaimPending(
		ctx,
		p.config.BatchSize,
		p.config.LeaseDuration,
	)
	if err != nil {
		return 0, fmt.Errorf("claim pending events: %w", err)
	}

	if len(events) == 0 {
		return 0, nil
	}

	producerMessages := make([]kafka_producer.Message, 0, len(events))
	eventIndexes := make([]int, 0, len(events))
	deliveryErrors := make([]error, len(events))
	eventSpans := make([]trace.Span, len(events))

	defer func() {
		for _, span := range eventSpans {
			if span != nil {
				span.End()
			}
		}
	}()

	for index, event := range events {
		eventCtx := propagation.TraceContext{}.Extract(
			ctx,
			propagation.MapCarrier{
				"traceparent": event.TraceParent,
				"tracestate":  event.TraceState,
			},
		)

		spanAttributes := []attribute.KeyValue{
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", p.config.Topic),
			attribute.String("messaging.operation.type", "publish"),
			attribute.String("event.id", event.Event.EventID),
			attribute.String("event.type", string(event.Event.EventType)),
			attribute.Int64("task.id", event.Event.AggregateID),
		}

		if p.tracer != nil {
			eventCtx, eventSpans[index] = p.tracer.Start(
				eventCtx,
				"kafka.publish",
				trace.WithSpanKind(trace.SpanKindProducer),
				trace.WithAttributes(spanAttributes...),
			)
		}

		value, err := core_events.Marshal(event.Event)
		if err != nil {
			deliveryErrors[index] = fmt.Errorf(
				"serialize outbox event %s: %w",
				event.Event.EventID,
				err,
			)
			recordPublishSpanError(eventSpans[index], deliveryErrors[index])
			continue
		}

		carrier := propagation.MapCarrier{}
		propagation.TraceContext{}.Inject(eventCtx, carrier)

		headers := make([]kafka.Header, 0, 2)
		for _, key := range []string{"traceparent", "tracestate"} {
			if value := carrier.Get(key); value != "" {
				headers = append(headers, kafka.Header{
					Key:   key,
					Value: []byte(value),
				})
			}
		}

		producerMessages = append(
			producerMessages,
			kafka_producer.Message{
				Key: []byte(
					strconv.FormatInt(event.Event.AggregateID, 10),
				),
				Value:   value,
				Headers: headers,
			},
		)
		eventIndexes = append(eventIndexes, index)
	}

	if len(producerMessages) > 0 {
		results := p.producer.PublishBatch(
			ctx,
			p.config.Topic,
			producerMessages,
		)

		if len(results) != len(eventIndexes) {
			batchErr := fmt.Errorf(
				"Kafka Producer returned %d results for %d messages",
				len(results),
				len(eventIndexes),
			)
			for _, eventIndex := range eventIndexes {
				deliveryErrors[eventIndex] = batchErr
			}
		} else {
			for index, eventIndex := range eventIndexes {
				deliveryErrors[eventIndex] = results[index]
			}
		}
	}

	for index, event := range events {
		if ctx.Err() != nil {
			recordPublishSpanError(eventSpans[index], ctx.Err())
			continue
		}

		deliveryErr := deliveryErrors[index]
		if deliveryErr == nil {
			// Kafka acknowledged the event, even if the Outbox state update
			// later fails and the lease causes at-least-once redelivery.
			if p.metrics != nil {
				p.metrics.RecordOutboxPublished(ctx, string(event.Event.EventType))
			}
			if span := eventSpans[index]; span != nil {
				span.AddEvent("kafka.delivery_report_received")
				span.SetAttributes(attribute.String("outbox.publish.outcome", "published"))
			}

			markCtx, cancel := context.WithTimeout(
				context.WithoutCancel(ctx),
				repositoryUpdateTimeout,
			)
			err := p.repository.MarkPublished(markCtx, event.ID, event.LeaseToken)
			cancel()

			if err != nil {
				recordPublishSpanError(eventSpans[index], err)
				p.logger.Error(
					"Kafka acknowledged event but Outbox update failed",
					zap.String("event_id", event.Event.EventID),
					zap.Int64("task_id", event.Event.AggregateID),
					zap.String("event_type", string(event.Event.EventType)),
					zap.String("topic", p.config.Topic),
					zap.Int64("outbox_id", event.ID),
					zap.Error(err),
				)
			} else {
				p.logger.Info(
					"Outbox event published to Kafka",
					zap.String("event_id", event.Event.EventID),
					zap.Int64("task_id", event.Event.AggregateID),
					zap.String("event_type", string(event.Event.EventType)),
					zap.String("topic", p.config.Topic),
					zap.Int64("outbox_id", event.ID),
				)
			}
			continue
		}

		// Shutdown cancellation is an ambiguous delivery outcome, not a
		// broker failure. The lease is intentionally left to expire.
		if errors.Is(deliveryErr, context.Canceled) ||
			errors.Is(deliveryErr, context.DeadlineExceeded) {
			recordPublishSpanError(eventSpans[index], deliveryErr)
			continue
		}

		if p.metrics != nil {
			p.metrics.RecordOutboxPublishError(ctx, string(event.Event.EventType))
		}
		recordPublishSpanError(eventSpans[index], deliveryErr)

		retryAfter := p.retryDelay(event.Attempts + 1)
		markCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			repositoryUpdateTimeout,
		)
		err := p.repository.MarkFailed(
			markCtx,
			event.ID,
			event.LeaseToken,
			deliveryErr,
			retryAfter,
		)
		cancel()

		if err != nil {
			p.logger.Error(
				"failed to schedule Outbox retry",
				zap.String("event_id", event.Event.EventID),
				zap.Int64("task_id", event.Event.AggregateID),
				zap.String("event_type", string(event.Event.EventType)),
				zap.String("topic", p.config.Topic),
				zap.Int64("outbox_id", event.ID),
				zap.Error(err),
			)
			continue
		}

		p.logger.Warn(
			"Kafka event delivery failed; retry scheduled",
			zap.String("event_id", event.Event.EventID),
			zap.Int64("task_id", event.Event.AggregateID),
			zap.String("event_type", string(event.Event.EventType)),
			zap.String("topic", p.config.Topic),
			zap.Int("attempt", event.Attempts+1),
			zap.Duration("retry_after", retryAfter),
			zap.Error(deliveryErr),
		)
	}

	return len(events), nil
}

// recordPublishSpanError records an error and marks the producer span as failed when present.
func recordPublishSpanError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// retryDelay calculates capped exponential backoff for the requested publish attempt.
func (p *Publisher) retryDelay(attempt int) time.Duration {
	delay := p.config.RetryBaseDelay

	for currentAttempt := 1; currentAttempt < attempt; currentAttempt++ {
		if delay >= p.config.RetryMaxDelay/2 {
			return p.config.RetryMaxDelay
		}

		delay *= 2
	}

	if delay > p.config.RetryMaxDelay {
		return p.config.RetryMaxDelay
	}

	return delay
}
