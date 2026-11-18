package activity_consumer

import (
	"context"
	"fmt"
	"time"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	kafka_consumer "github.com/Reigor1/20h-project/internal/core/kafka/consumer"
	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
	"go.uber.org/zap"
)

const pollTimeout = 100 * time.Millisecond

type KafkaConsumer interface {
	Subscribe(topic string) error
	Poll(timeout time.Duration) kafka_consumer.PollResult
	CommitMessage(message *kafka_consumer.Message) error
}

type ActivityService interface {
	Record(
		ctx context.Context,
		event core_events.Envelope,
	) error
}

type Worker struct {
	topic    string
	consumer KafkaConsumer
	service  ActivityService
	logger   *zap.Logger
}

func NewWorker(
	topic string,
	consumer KafkaConsumer,
	service *activity_service.ActivityService,
	logger *zap.Logger,
) (*Worker, error) {
	if consumer == nil {
		return nil, fmt.Errorf("Kafka Consumer must not be nil")
	}

	if service == nil {
		return nil, fmt.Errorf("Activity Service must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("Worker logger must not be nil")
	}

	return &Worker{
		topic:    topic,
		consumer: consumer,
		service:  service,
		logger:   logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.consumer.Subscribe(w.topic); err != nil {
		return fmt.Errorf("subscribe Activity Worker: %w", err)
	}

	w.logger.Info(
		"Activity Worker started",
		zap.String("topic", w.topic),
	)

	for {
		if ctx.Err() != nil {
			w.logger.Info("Activity Worker stopped")
			return nil
		}

		result := w.consumer.Poll(pollTimeout)

		if result.Err != nil {
			w.logger.Warn(
				"Kafka Consumer reported an error",
				zap.Error(result.Err),
			)

			if result.Fatal {
				return fmt.Errorf(
					"fatal Kafka Consumer error: %w",
					result.Err,
				)
			}

			continue
		}

		if result.Message == nil {
			continue
		}

		message := result.Message

		event, err := core_events.Unmarshal(message.Value)
		if err != nil {
			return fmt.Errorf(
				"unmarshal Kafka event from %s partition %d offset %d: %w",
				message.Topic,
				message.Partition,
				message.Offset,
				err,
			)
		}

		if err := w.service.Record(ctx, event); err != nil {
			return fmt.Errorf(
				"process Activity event %s: %w",
				event.EventID,
				err,
			)
		}

		// Commit only after the PostgreSQL transaction succeeds.
		if err := w.consumer.CommitMessage(message); err != nil {
			return fmt.Errorf(
				"commit offset for Activity event %s: %w",
				event.EventID,
				err,
			)
		}

		w.logger.Info(
			"Activity event processed",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)),
			zap.Int64("task_id", event.AggregateID),
			zap.Int64("task_version", event.AggregateVersion),
			zap.String("topic", message.Topic),
			zap.Int32("partition", message.Partition),
			zap.Int64("offset", message.Offset),
		)
	}
}
