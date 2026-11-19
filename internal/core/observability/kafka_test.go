package core_observability

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type testOutboxMetricsSource struct {
	pending int64
	oldest  float64
}

func (s testOutboxMetricsSource) PendingStats(context.Context) (int64, float64, error) {
	return s.pending, s.oldest, nil
}

func TestKafkaMetricsRecordsApplicationMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	metrics, err := NewKafkaMetrics(
		provider,
		testOutboxMetricsSource{pending: 7, oldest: 12.5},
	)
	if err != nil {
		t.Fatalf("NewKafkaMetrics() error = %v", err)
	}

	ctx := context.Background()
	metrics.RecordOutboxPublished(ctx, "task.created")
	metrics.RecordOutboxPublishError(ctx, "task.updated")
	metrics.RecordActivityProcessed(ctx, "task.created")
	metrics.RecordActivityFailed(ctx, "task.updated")
	metrics.RecordActivityDuplicate(ctx, "task.created")
	metrics.RecordActivityDLQMessage(ctx, "task.updated")
	metrics.RecordActivityProcessingDuration(ctx, 250*time.Millisecond, "task.created", "processed")

	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(data.ScopeMetrics) != 1 {
		t.Fatalf("scope metrics = %d, want 1", len(data.ScopeMetrics))
	}

	got := make(map[string]bool)
	for _, item := range data.ScopeMetrics[0].Metrics {
		got[item.Name] = true
	}
	for _, name := range []string{
		"outbox_pending_events",
		"outbox_oldest_event_age_seconds",
		"outbox_published_total",
		"outbox_publish_errors_total",
		"activity_events_processed_total",
		"activity_events_failed_total",
		"activity_events_duplicate_total",
		"activity_dlq_messages_total",
		"activity_processing_duration_seconds",
	} {
		if !got[name] {
			t.Errorf("metric %q was not collected", name)
		}
	}
}
