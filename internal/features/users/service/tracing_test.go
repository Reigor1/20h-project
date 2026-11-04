package users_service

import (
	"context"
	"testing"

	"github.com/Reigor1/20h-project/internal/core/domain"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type recordedSpan struct {
	spanContext trace.SpanContext
	isRecording bool
}

type tracingUsersRepository struct {
	spans []recordedSpan
}

func (r *tracingUsersRepository) recordContext(ctx context.Context) {
	span := trace.SpanFromContext(ctx)

	r.spans = append(r.spans, recordedSpan{
		spanContext: span.SpanContext(),
		isRecording: span.IsRecording(),
	})
}

func (r *tracingUsersRepository) CreateUser(
	ctx context.Context,
	user domain.User,
) (domain.User, error) {
	r.recordContext(ctx)
	return user, nil
}

func (r *tracingUsersRepository) GetUsers(
	ctx context.Context,
	_ *int,
	_ *int,
) ([]domain.User, error) {
	r.recordContext(ctx)
	return []domain.User{}, nil
}

func (r *tracingUsersRepository) GetUser(
	ctx context.Context,
	_ int,
) (domain.User, error) {
	r.recordContext(ctx)
	return domain.NewUser(42, 1, "Ada Lovelace", nil), nil
}

func (r *tracingUsersRepository) DeleteUser(
	ctx context.Context,
	_ int,
) error {
	r.recordContext(ctx)
	return nil
}

func (r *tracingUsersRepository) PatchUser(
	ctx context.Context,
	_ int,
	user domain.User,
) (domain.User, error) {
	r.recordContext(ctx)
	return user, nil
}

func TestUsersServiceCreatesSpansAndPassesSpanContextToRepository(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	t.Cleanup(func() {
		if err := tracerProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	repository := &tracingUsersRepository{}
	service := NewUsersService(repository, tracerProvider)

	requestCtx, requestSpan := tracerProvider.Tracer("users-service-test").Start(
		context.Background(),
		"GET /api/v1/users",
	)

	phoneNumber := "+1234567890"

	if _, err := service.CreateUser(
		requestCtx,
		domain.NewUserUninitialized("Ada Lovelace", &phoneNumber),
	); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := service.GetUsers(requestCtx, nil, nil); err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}

	if _, err := service.GetUser(requestCtx, 42); err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}

	updatedName := "Grace Hopper"

	patch := domain.NewUserPatch(
		domain.Nullable[string]{Value: &updatedName, Set: true},
		domain.Nullable[string]{},
	)

	if _, err := service.PatchUser(requestCtx, 42, patch); err != nil {
		t.Fatalf("PatchUser() error = %v", err)
	}

	if err := service.DeleteUser(requestCtx, 42); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	requestSpan.End()

	spans := exporter.GetSpans()

	if len(spans) != 6 {
		t.Fatalf(
			"exported spans = %d, want 6 (one request + five Users operations)",
			len(spans),
		)
	}

	requestSpanID := requestSpan.SpanContext().SpanID()
	requestTraceID := requestSpan.SpanContext().TraceID()

	wantNames := map[string]bool{
		"users.create":      false,
		"users.list":        false,
		"users.get":         false,
		"users.update":      false,
		"users.delete":      false,
		"GET /api/v1/users": false,
	}

	serviceSpanIDs := make(map[trace.SpanID]struct{}, 5)

	for _, span := range spans {
		if _, ok := wantNames[span.Name]; !ok {
			t.Errorf("unexpected span name %q", span.Name)
			continue
		}

		if wantNames[span.Name] {
			t.Errorf("duplicate span %q", span.Name)
		}

		wantNames[span.Name] = true

		if span.SpanContext.TraceID() != requestTraceID {
			t.Errorf("span %q has a different trace ID", span.Name)
		}

		if span.Name == "GET /api/v1/users" {
			continue
		}

		if span.Parent.SpanID() != requestSpanID {
			t.Errorf(
				"span %q parent = %s, want request span %s",
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

	if len(repository.spans) != 6 {
		t.Fatalf(
			"repository calls = %d, want 6 (PatchUser performs GetUser + PatchUser)",
			len(repository.spans),
		)
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
				"repository call %d received a context outside the Users service spans",
				i+1,
			)
		}
	}
}
