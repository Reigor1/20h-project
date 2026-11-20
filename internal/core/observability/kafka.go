package core_observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type OutboxMetricsSource interface {
	PendingStats(ctx context.Context) (pending int64, oldestEventAgeSeconds float64, err error)
}

type KafkaMetrics struct {
	outboxPublished     metric.Int64Counter
	outboxPublishErrors metric.Int64Counter
	activityProcessed   metric.Int64Counter
	activityFailed      metric.Int64Counter
	activityDuplicate   metric.Int64Counter
	activityDLQMessages metric.Int64Counter
	activityDuration    metric.Float64Histogram
}

func NewKafkaMetrics(
	provider metric.MeterProvider,
	outboxSource OutboxMetricsSource,
) (*KafkaMetrics, error) {
	if provider == nil {
		return nil, fmt.Errorf("Kafka metrics MeterProvider must not be nil")
	}

	meter := provider.Meter("github.com/Reigor1/20h-project/internal/core/observability/kafka")
	metrics := &KafkaMetrics{}
	var err error

	if metrics.outboxPublished, err = meter.Int64Counter(
		"outbox_published_total",
		metric.WithDescription("Number of Outbox events acknowledged by Kafka."),
	); err != nil {
		return nil, fmt.Errorf("create Outbox published counter: %w", err)
	}
	if metrics.outboxPublishErrors, err = meter.Int64Counter(
		"outbox_publish_errors_total",
		metric.WithDescription("Number of failed Kafka Outbox publication attempts."),
	); err != nil {
		return nil, fmt.Errorf("create Outbox publish errors counter: %w", err)
	}
	if metrics.activityProcessed, err = meter.Int64Counter(
		"activity_events_processed_total",
		metric.WithDescription("Number of Activity events persisted and committed."),
	); err != nil {
		return nil, fmt.Errorf("create Activity processed counter: %w", err)
	}
	if metrics.activityFailed, err = meter.Int64Counter(
		"activity_events_failed_total",
		metric.WithDescription("Number of Activity processing attempts that failed."),
	); err != nil {
		return nil, fmt.Errorf("create Activity failed counter: %w", err)
	}
	if metrics.activityDuplicate, err = meter.Int64Counter(
		"activity_events_duplicate_total",
		metric.WithDescription("Number of duplicate Activity deliveries successfully committed."),
	); err != nil {
		return nil, fmt.Errorf("create Activity duplicate counter: %w", err)
	}
	if metrics.activityDLQMessages, err = meter.Int64Counter(
		"activity_dlq_messages_total",
		metric.WithDescription("Number of messages successfully published to the Activity DLQ."),
	); err != nil {
		return nil, fmt.Errorf("create Activity DLQ counter: %w", err)
	}
	if metrics.activityDuration, err = meter.Float64Histogram(
		"activity_processing_duration_seconds",
		metric.WithDescription("Activity event processing duration, including retries and offset commit."),
		metric.WithUnit("s"),
	); err != nil {
		return nil, fmt.Errorf("create Activity processing duration histogram: %w", err)
	}

	if outboxSource != nil {
		pendingGauge, err := meter.Int64ObservableGauge(
			"outbox_pending_events",
			metric.WithDescription("Outbox rows that have not been marked as published."),
			)
		if err != nil {
			return nil, fmt.Errorf("create Outbox pending gauge: %w", err)
		}
		oldestAgeGauge, err := meter.Float64ObservableGauge(
			"outbox_oldest_event_age_seconds",
			metric.WithDescription("Age in seconds of the oldest unpublished Outbox event."),
			metric.WithUnit("s"),
		)
		if err != nil {
			return nil, fmt.Errorf("create Outbox oldest event age gauge: %w", err)
		}

		if _, err := meter.RegisterCallback(
			func(ctx context.Context, observer metric.Observer) error {
				pending, oldestAge, err := outboxSource.PendingStats(ctx)
				if err != nil {
					return fmt.Errorf("collect Outbox queue metrics: %w", err)
				}
				observer.ObserveInt64(pendingGauge, pending)
				observer.ObserveFloat64(oldestAgeGauge, oldestAge)
				return nil
			},
			pendingGauge,
			oldestAgeGauge,
		); err != nil {
			return nil, fmt.Errorf("register Outbox metrics callback: %w", err)
		}
	}

	return metrics, nil
}

func (m *KafkaMetrics) RecordOutboxPublished(ctx context.Context, eventType string) {
	if m != nil {
		m.outboxPublished.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
	}
}

func (m *KafkaMetrics) RecordOutboxPublishError(ctx context.Context, eventType string) {
	if m != nil {
		m.outboxPublishErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
	}
}

func (m *KafkaMetrics) RecordActivityProcessed(ctx context.Context, eventType string) {
	if m != nil {
		m.activityProcessed.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
	}
}

func (m *KafkaMetrics) RecordActivityFailed(ctx context.Context, eventType string) {
	if m != nil {
		m.activityFailed.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
	}
}

func (m *KafkaMetrics) RecordActivityDuplicate(ctx context.Context, eventType string) {
	if m != nil {
		m.activityDuplicate.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
	}
}

func (m *KafkaMetrics) RecordActivityDLQMessage(ctx context.Context, eventType string) {
	if m != nil {
		m.activityDLQMessages.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
	}
}

func (m *KafkaMetrics) RecordActivityProcessingDuration(
	ctx context.Context,
	duration time.Duration,
	eventType string,
	outcome string,
) {
	if m != nil {
		m.activityDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
			attribute.String("event_type", eventType),
			attribute.String("outcome", outcome),
		))
	}
}
