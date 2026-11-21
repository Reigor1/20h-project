package statistics_service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// tracingStatisticsRepository records the context passed by the service and returns configured task data.
type tracingStatisticsRepository struct {
	ctx   context.Context
	err   error
	tasks []domain.Task
}

// GetTasks captures the service context so the test can inspect its active span.
func (r *tracingStatisticsRepository) GetTasks(
	ctx context.Context,
	_ *int,
	_ *time.Time,
	_ *time.Time,
) ([]domain.Task, error) {
	r.ctx = ctx
	return r.tasks, r.err
}

// newTracingTestProvider creates an in-memory tracer provider and registers its cleanup.
func newTracingTestProvider(
	t *testing.T,
) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	t.Cleanup(func() {
		if err := tracerProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	return tracerProvider, exporter
}

// TestGetStatisticsCreatesChildSpanAndPassesContextToRepository verifies the statistics span
// is a child of the request span and that the repository receives that service span context.
func TestGetStatisticsCreatesChildSpanAndPassesContextToRepository(t *testing.T) {
	tracerProvider, exporter := newTracingTestProvider(t)
	repository := &tracingStatisticsRepository{}
	service := NewStatisticsService(repository, nil, tracerProvider)

	rootCtx, rootSpan := tracerProvider.Tracer("statistics-service-tracing-test").Start(
		context.Background(),
		"GET /api/v1/statistics",
	)
	rootSpanContext := rootSpan.SpanContext()

	got, err := service.GetStatistics(rootCtx, nil, nil, nil)
	rootSpan.End()

	if err != nil {
		t.Fatalf("GetStatistics() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, domain.NewStatistics(0, 0, nil, nil)) {
		t.Fatalf("GetStatistics() = %+v, want zero statistics", got)
	}

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("exported spans = %d, want root and statistics spans", len(spans))
	}

	var statisticsSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "statistics.get" {
			statisticsSpan = &spans[i]
			break
		}
	}
	if statisticsSpan == nil {
		t.Fatal("statistics.get span was not exported")
	}

	if statisticsSpan.SpanContext.TraceID() != rootSpanContext.TraceID() {
		t.Fatal("request and statistics spans have different trace IDs")
	}
	if statisticsSpan.Parent.SpanID() != rootSpanContext.SpanID() {
		t.Fatal("statistics span is not a child of the request span")
	}

	repositorySpanContext := oteltrace.SpanFromContext(repository.ctx).SpanContext()
	if !repositorySpanContext.IsValid() {
		t.Fatal("repository context does not contain an active span")
	}
	if repositorySpanContext.SpanID() != statisticsSpan.SpanContext.SpanID() {
		t.Fatal("repository context does not contain the statistics span")
	}
}

// TestGetStatisticsMarksSpanErrorForInvalidDateRange checks invalid input marks the span as failed
// and prevents the repository from being called.
func TestGetStatisticsMarksSpanErrorForInvalidDateRange(t *testing.T) {
	tracerProvider, exporter := newTracingTestProvider(t)
	repository := &tracingStatisticsRepository{}
	service := NewStatisticsService(repository, nil, tracerProvider)

	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from

	_, err := service.GetStatistics(context.Background(), nil, &from, &to)
	if err == nil {
		t.Fatal("GetStatistics() error = nil, want invalid date range error")
	}
	if repository.ctx != nil {
		t.Fatal("repository was called for an invalid date range")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}

	span := spans[0]
	if span.Name != "statistics.get" {
		t.Fatalf("span name = %q, want %q", span.Name, "statistics.get")
	}
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want %v", span.Status.Code, codes.Error)
	}
	if span.Status.Description != "statistics.get failed" {
		t.Fatalf(
			"span status description = %q, want %q",
			span.Status.Description,
			"statistics.get failed",
		)
	}
}
