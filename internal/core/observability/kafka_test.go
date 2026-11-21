package core_observability

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// testOutboxMetricsSource supplies deterministic backlog values to observable gauge tests.
type testOutboxMetricsSource struct {
	pending int64
	oldest  float64
}

// PendingStats returns the fixed backlog values configured in the test source.
func (s testOutboxMetricsSource) PendingStats(context.Context) (int64, float64, error) {
	return s.pending, s.oldest, nil
}

// TestKafkaMetricsRecordsApplicationMetrics checks that Kafka counters, duration histogram,
// and Outbox backlog gauges are registered and collected with the expected measurements.
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
		if item.Name == "outbox_pending_events" {
			gauge, ok := item.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("outbox_pending_events data type = %T, want Gauge[int64]", item.Data)
			}
			if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != 7 {
				t.Errorf("outbox_pending_events = %+v, want 7", gauge.DataPoints)
			}
		}
		if item.Name == "outbox_oldest_event_age" {
			gauge, ok := item.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("outbox_oldest_event_age data type = %T, want Gauge[float64]", item.Data)
			}
			if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != 12.5 {
				t.Errorf("outbox_oldest_event_age = %+v, want 12.5", gauge.DataPoints)
			}
		}
	}
	for _, name := range []string{
		"outbox_pending_events",
		"outbox_oldest_event_age",
		"outbox_published",
		"outbox_publish_errors",
		"activity_events_processed",
		"activity_events_failed",
		"activity_events_duplicate",
		"activity_dlq_messages",
		"activity_processing_duration",
	} {
		if !got[name] {
			t.Errorf("metric %q was not collected", name)
		}
	}
}
