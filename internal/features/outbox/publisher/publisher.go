package outbox_publisher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	outbox_repository "github.com/Reigor1/20h-project/internal/features/outbox/repository"
	"go.uber.org/zap"
)

const repositoryUpdateTimeout = 5 * time.Second

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

type EventProducer interface {
	PublishBatch(
		ctx context.Context,
		topic string,
		messages []kafka_producer.Message,
	) []error
}

type Publisher struct {
	config     Config
	repository OutboxRepository
	producer   EventProducer
	logger     *zap.Logger
}

func NewPublisher(
	config Config,
	repository OutboxRepository,
	producer EventProducer,
	logger *zap.Logger,
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

	return &Publisher{
		config:     config,
		repository: repository,
		producer:   producer,
		logger:     logger,
	}, nil
}

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
		claimedCount, err := p.publishBatch(ctx)

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

	for index, event := range events {
		// The Event field already contains the complete Envelope.
		value, err := core_events.Marshal(event.Event)
		if err != nil {
			deliveryErrors[index] = fmt.Errorf(
				"serialize outbox event %s: %w",
				event.Event.EventID,
				err,
			)
			continue
		}

		producerMessages = append(
			producerMessages,
			kafka_producer.Message{
				// All events for one Task use the same Kafka key.
				Key: []byte(
					strconv.FormatInt(event.Event.AggregateID, 10),
				),
				Value: value,
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
			return len(events), fmt.Errorf(
				"Kafka Producer returned %d results for %d messages",
				len(results),
				len(eventIndexes),
			)
		}

		for index, eventIndex := range eventIndexes {
			deliveryErrors[eventIndex] = results[index]
		}
	}

	for index, event := range events {
		deliveryErr := deliveryErrors[index]

		if deliveryErr == nil {
			// Delivery Report confirms successful broker acknowledgement.
			// Preserve context values but detach from cancellation.
			// The update itself is still bounded by a timeout.
			markCtx, cancel := context.WithTimeout(
				context.WithoutCancel(ctx),
				repositoryUpdateTimeout,
			)

			err := p.repository.MarkPublished(
				markCtx,
				event.ID,
				event.LeaseToken,
			)

			cancel()

			if err != nil {
				// Kafka may already contain the message.
				// Lease expiration allows at-least-once redelivery.
				p.logger.Error(
					"Kafka acknowledged event but Outbox update failed",
					zap.String("event_id", event.Event.EventID),
					zap.Int64("outbox_id", event.ID),
					zap.Error(err),
				)
			}

			continue
		}

		// An interrupted wait has an ambiguous delivery outcome.
		// Keep the lease until it expires instead of releasing the record
		// immediately and potentially racing another Publisher.
		if errors.Is(deliveryErr, context.Canceled) ||
			errors.Is(deliveryErr, context.DeadlineExceeded) {
			continue
		}

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
				zap.Int64("outbox_id", event.ID),
				zap.Error(err),
			)
			continue
		}

		p.logger.Warn(
			"Kafka event delivery failed; retry scheduled",
			zap.String("event_id", event.Event.EventID),
			zap.Int64("outbox_id", event.ID),
			zap.Int("attempt", event.Attempts+1),
			zap.Duration("retry_after", retryAfter),
			zap.Error(deliveryErr),
		)
	}

	return len(events), nil
}

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
