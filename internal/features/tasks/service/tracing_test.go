package tasks_service

import (
	"context"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type recordedSpan struct {
	spanContext trace.SpanContext
	isRecording bool
}

type tracingTasksRepository struct {
	task  domain.Task
	spans []recordedSpan
}

// recordContext captures the active span context passed to the Tasks repository.
func (r *tracingTasksRepository) recordContext(ctx context.Context) {
	span := trace.SpanFromContext(ctx)

	r.spans = append(r.spans, recordedSpan{
		spanContext: span.SpanContext(),
		isRecording: span.IsRecording(),
	})
}

// CreateTask supports the corresponding integration or tracing test.
func (r *tracingTasksRepository) CreateTask(
	ctx context.Context,
	task domain.Task,
) (domain.Task, error) {
	r.recordContext(ctx)
	return task, nil
}

// GetTasks supports the corresponding integration or tracing test.
func (r *tracingTasksRepository) GetTasks(
	ctx context.Context,
	_ *int,
	_ *int,
	_ *int,
) ([]domain.Task, error) {
	r.recordContext(ctx)
	return []domain.Task{r.task}, nil
}

// GetTask supports the corresponding integration or tracing test.
func (r *tracingTasksRepository) GetTask(
	ctx context.Context,
	_ int,
) (domain.Task, error) {
	r.recordContext(ctx)
	return r.task, nil
}

// DeleteTask supports the corresponding integration or tracing test.
func (r *tracingTasksRepository) DeleteTask(
	ctx context.Context,
	_ int,
) error {
	r.recordContext(ctx)
	return nil
}

// PatchTask supports the corresponding integration or tracing test.
func (r *tracingTasksRepository) PatchTask(
	ctx context.Context,
	_ int,
	_ domain.Task,
	after domain.Task,
) (domain.Task, error) {
	r.recordContext(ctx)
	return after, nil
}

// TestTasksServiceMethodsCreateSpans verifies the integration behavior represented by its test scenarios.
func TestTasksServiceMethodsCreateSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	t.Cleanup(func() {
		if err := tracerProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	repository := &tracingTasksRepository{
		task: sampleTask(),
	}

	service := NewTasksService(repository, tracerProvider)

	requestCtx, requestSpan := tracerProvider.Tracer(
		"tasks-service-tracing-test",
	).Start(context.Background(), "test.request")

	description := "Prepare a clear report"

	createdAt := time.Date(
		2026, 10, 20, 9, 30, 0, 0, time.UTC,
	)

	newTask := domain.NewTask(
		domain.UninitializedID,
		domain.UninitializedVersion,
		"Write report",
		&description,
		false,
		createdAt,
		nil,
		23,
	)

	if _, err := service.CreateTask(requestCtx, newTask); err != nil {
		t.Fatalf("CreateTask() error = %v, want nil", err)
	}

	if _, err := service.GetTasks(requestCtx, nil, nil, nil); err != nil {
		t.Fatalf("GetTasks() error = %v, want nil", err)
	}

	if _, err := service.GetTask(requestCtx, repository.task.ID); err != nil {
		t.Fatalf("GetTask() error = %v, want nil", err)
	}

	updatedTitle := "Updated report"

	patch := domain.NewTaskPatch(
		domain.Nullable[string]{Value: &updatedTitle, Set: true},
		domain.Nullable[string]{},
		domain.Nullable[bool]{},
	)

	if _, err := service.PathTask(
		requestCtx,
		repository.task.ID,
		patch,
	); err != nil {
		t.Fatalf("PathTask() error = %v, want nil", err)
	}

	if err := service.DeleteTask(requestCtx, repository.task.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v, want nil", err)
	}

	requestSpan.End()

	spans := exporter.GetSpans()

	if len(spans) != 6 {
		t.Fatalf(
			"exported spans = %d, want 6 (one request + five Tasks operations)",
			len(spans),
		)
	}

	requestSpanID := requestSpan.SpanContext().SpanID()
	requestTraceID := requestSpan.SpanContext().TraceID()

	wantNames := map[string]bool{
		"test.request": false,
		"tasks.create": false,
		"tasks.list":   false,
		"tasks.get":    false,
		"tasks.update": false,
		"tasks.delete": false,
	}

	serviceSpanIDs := make(map[trace.SpanID]struct{}, 5)

	for _, span := range spans {
		if _, ok := wantNames[span.Name]; !ok {
			t.Errorf("unexpected span %q", span.Name)
			continue
		}

		if wantNames[span.Name] {
			t.Errorf("duplicate span %q", span.Name)
			continue
		}

		wantNames[span.Name] = true

		if span.SpanContext.TraceID() != requestTraceID {
			t.Errorf("span %q has a different trace ID", span.Name)
		}

		if span.Name == "test.request" {
			continue
		}

		if span.Parent.SpanID() != requestSpanID {
			t.Errorf(
				"span %q parent span ID = %s, want %s",
				span.Name,
				span.Parent.SpanID(),
				requestSpanID,
			)
		}

		serviceSpanIDs[span.SpanContext.SpanID()] = struct{}{}
	}

	for name, found := range wantNames {
		if !found {
			t.Errorf("span %q was not exported", name)
		}
	}

	// PathTask reads the task and then updates it, so the repository
	// receives the service span context twice for that operation.
	if got, want := len(repository.spans), 6; got != want {
		t.Fatalf("repository calls = %d, want %d", got, want)
	}

	for i, recorded := range repository.spans {
		if !recorded.isRecording {
			t.Errorf(
				"repository call %d did not receive an active service span",
				i+1,
			)
		}

		if !recorded.spanContext.IsValid() {
			t.Errorf(
				"repository call %d received an invalid span context",
				i+1,
			)
			continue
		}

		if _, ok := serviceSpanIDs[recorded.spanContext.SpanID()]; !ok {
			t.Errorf(
				"repository call %d received span %s which is not a TasksService span",
				i+1,
				recorded.spanContext.SpanID(),
			)
		}
	}
}
