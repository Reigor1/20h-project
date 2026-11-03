package core_observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestInstrumentHTTPRouteCreatesServerSpan(t *testing.T) {
	meterProvider := sdkmetric.NewMeterProvider()

	traceExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(traceExporter),
	)

	activeRequests, err := meterProvider.Meter(
		"github.com/Reigor1/20h-project/internal/core/observability",
	).Int64UpDownCounter(
		"http.server.active_requests",
	)
	if err != nil {
		t.Fatalf("create active requests counter: %v", err)
	}

	observability := &Observability{
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
		propagators: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
		activeRequests: activeRequests,
	}

	defer func() {
		if err := meterProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}

		if err := tracerProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	}()

	var gotPattern string

	handler := observability.InstrumentHTTPRoute(
		http.MethodGet,
		"/api/v1/users/{id}",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPattern = r.Pattern
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", handler)

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)

	const (
		traceID      = "4bf92f3577b34da6a3ce929d0e0e4736"
		parentSpanID = "00f067aa0ba902b7"
	)

	request.Header.Set(
		"traceparent",
		"00-"+traceID+"-"+parentSpanID+"-01",
	)

	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if gotPattern != "/api/v1/users/{id}" {
		t.Fatalf(
			"request pattern = %q, want %q",
			gotPattern,
			"/api/v1/users/{id}",
		)
	}

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"response status = %d, want %d",
			recorder.Code,
			http.StatusNoContent,
		)
	}

	spans := traceExporter.GetSpans()

	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}

	span := spans[0]

	if span.SpanKind != trace.SpanKindServer {
		t.Fatalf(
			"span kind = %v, want %v",
			span.SpanKind,
			trace.SpanKindServer,
		)
	}

	if got := span.SpanContext.TraceID().String(); got != traceID {
		t.Fatalf("trace ID = %q, want %q", got, traceID)
	}

	if got := span.Parent.SpanID().String(); got != parentSpanID {
		t.Fatalf("parent span ID = %q, want %q", got, parentSpanID)
	}
}
