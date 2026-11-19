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
	Record(ctx context.Context, event core_events.Envelope) error
}

type Worker struct {
	topic              string
	dlqTopic           string
	retryBaseDelay     time.Duration
	retryMaxDelay      time.Duration
	maxPendingMessages int

	consumer KafkaConsumer
	producer KafkaProducer
	service  ActivityService
	logger   *zap.Logger
}

func NewWorker(
	topic string,
	dlqTopic string,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	maxPendingMessages int,
	consumer KafkaConsumer,
	producer KafkaProducer,
	service ActivityService,
	logger *zap.Logger,
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

	return &Worker{
		topic:              topic,
		dlqTopic:           dlqTopic,
		retryBaseDelay:     retryBaseDelay,
		retryMaxDelay:      retryMaxDelay,
		maxPendingMessages: maxPendingMessages,
		consumer:           consumer,
		producer:           producer,
		service:            service,
		logger:             logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.consumer.Subscribe(w.topic); err != nil {
		return fmt.Errorf("subscribe Activity Worker: %w", err)
	}

	w.logger.Info(
		"Activity Worker started",
		zap.String("topic", w.topic),
		zap.String("dlq_topic", w.dlqTopic),
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

		if err := w.processMessage(ctx, message, &pending); err != nil {
			if errors.Is(err, kafka_consumer.ErrAssignmentLost) {
				w.logger.Info(
					"Activity Worker lost partition assignment; message will be redelivered",
					zap.String("topic", message.Topic),
					zap.Int32("partition", message.Partition),
					zap.Int64("offset", message.Offset),
				)

				continue
			}

			if ctx.Err() != nil {
				w.logger.Info("Activity Worker stopped during message processing")
				return nil
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
	if !w.consumer.IsAssigned(message) {
		return kafka_consumer.ErrAssignmentLost
	}

	event, err := core_events.Unmarshal(message.Value)
	if err != nil {
		if !isPermanentMessageError(err) {
			return fmt.Errorf("classify event message: %w", err)
		}

		return w.moveToDLQ(ctx, message, err, pending)
	}

	attempts, paused, err := w.retry(
		ctx,
		message,
		func() error {
			return w.service.Record(ctx, event)
		},
		pending,
		"record Activity event",
	)
	if err != nil {
		return w.resumeAfterAssignmentLoss(paused, err)
	}

	paused, err = w.commitWithRetry(
		ctx,
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
	)

	return nil
}

func (w *Worker) moveToDLQ(
	ctx context.Context,
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
		message,
		func() error {
			return w.producer.Publish(
				ctx,
				w.dlqTopic,
				kafka_producer.Message{
					Key:   []byte(record.EventID),
					Value: payload,
				},
			)
		},
		pending,
		"publish DLQ record",
	)
	if err != nil {
		return w.resumeAfterAssignmentLoss(paused, err)
	}

	// Commit the source offset only after the DLQ Delivery Report succeeds.
	paused, err = w.commitWithRetry(
		ctx,
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
		w.logger.Warn(
			"Activity Worker operation failed; retrying",
			zap.String("operation", operationName),
			zap.Int("attempt", attempt),
			zap.Error(err),
		)
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
			message,
			delay,
			pending,
		); err != nil {
			return attempt, true, err
		}

		if !w.consumer.IsAssigned(message) {
			return attempt, true, kafka_consumer.ErrAssignmentLost
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
	currentMessage *kafka_consumer.Message,
	delay time.Duration,
	pending *[]*kafka_consumer.Message,
) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Keep polling while the current assignment is paused.
		result := w.consumer.Poll(pollTimeout)

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

		case <-timer.C:
			return nil

		default:
		}
	}
}

func (w *Worker) commitWithRetry(
	ctx context.Context,
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

		w.logger.Warn(
			"Kafka offset commit failed; retrying",
			zap.String("topic", message.Topic),
			zap.Int32("partition", message.Partition),
			zap.Int64("offset", message.Offset),
			zap.Int("attempt", attempt),
			zap.Duration("retry_delay", delay),
			zap.Error(err),
		)

		if err := w.waitBackoff(
			ctx,
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
