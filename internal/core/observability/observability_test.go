package core_observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core_observability_postgres "github.com/Reigor1/20h-project/internal/core/observability/postgres"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
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

func TestInstrumentHTTPRouteCreatesPostgresChildSpan(t *testing.T) {
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

	pgxTracer := core_observability_postgres.NewSanitizingTracer(
		otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tracerProvider),
			otelpgx.WithMeterProvider(meterProvider),
			otelpgx.WithMetricOperationName(otelpgx.SQLOperationName),
			otelpgx.WithDisableSQLStatementInAttributes(),
			otelpgx.WithDisableConnectionDetailsInAttributes(),
		),
	)

	const secret = "sensitive@example.com"

	originalErr := &pgconn.PgError{
		Code:    "23505",
		Message: "duplicate key contains " + secret,
	}

	handler := observability.InstrumentHTTPRoute(
		http.MethodGet,
		"/api/v1/users/{id}",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			queryCtx := pgxTracer.TraceQueryStart(
				r.Context(),
				nil,
				pgx.TraceQueryStartData{
					SQL:  "SELECT id FROM users WHERE email = $1",
					Args: []any{secret},
				},
			)

			pgxTracer.TraceQueryEnd(
				queryCtx,
				nil,
				pgx.TraceQueryEndData{
					Err: originalErr,
				},
			)

			w.WriteHeader(http.StatusNoContent)
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", handler)

	request := httptest.NewRequest(
		http.MethodGet,
		"/users/42",
		nil,
	)

	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"response status = %d, want %d",
			recorder.Code,
			http.StatusNoContent,
		)
	}

	spans := traceExporter.GetSpans()

	var serverSpan *tracetest.SpanStub
	var querySpan *tracetest.SpanStub

	for i := range spans {
		switch {
		case spans[i].SpanKind == trace.SpanKindServer:
			serverSpan = &spans[i]
		case spans[i].Name == "SELECT":
			querySpan = &spans[i]
		}
	}

	if serverSpan == nil {
		t.Fatal("HTTP server span was not exported")
	}

	if querySpan == nil {
		t.Fatal("PostgreSQL query span was not exported")
	}

	if serverSpan.SpanContext.TraceID() != querySpan.SpanContext.TraceID() {
		t.Fatal("HTTP and PostgreSQL spans have different trace IDs")
	}

	if querySpan.Parent.SpanID() != serverSpan.SpanContext.SpanID() {
		t.Fatal("PostgreSQL span is not a child of the HTTP span")
	}

	if got := querySpan.Status.Description; got != "database operation failed" {
		t.Fatalf("query span status description = %q", got)
	}

	if originalErr.Error() == "database operation failed" {
		t.Fatal("original PostgreSQL error was unexpectedly modified")
	}

	if !strings.Contains(originalErr.Error(), secret) {
		t.Fatal("test error does not contain the expected sensitive value")
	}

	if got, ok := findSpanAttribute(querySpan.Attributes, "pgx.sql_state"); !ok ||
		got.AsString() != "23505" {
		t.Fatal("PostgreSQL SQLSTATE was not preserved")
	}

	for _, key := range []attribute.Key{
		"db.query.text",
		"pgx.query.parameters",
		"server.address",
		"server.port",
		"user.name",
		"db.namespace",
		"pgx.prepare_stmt.name",
	} {
		if _, ok := findSpanAttribute(querySpan.Attributes, key); ok {
			t.Errorf("sensitive or disabled attribute %q was exported", key)
		}
	}

	var exceptionMessage string

	for _, event := range querySpan.Events {
		if event.Name != "exception" {
			continue
		}

		if value, ok := findSpanAttribute(event.Attributes, "exception.message"); ok {
			exceptionMessage = value.AsString()
		}
	}

	if exceptionMessage != "database operation failed" {
		t.Fatalf("exception message = %q", exceptionMessage)
	}

	if strings.Contains(exceptionMessage, secret) {
		t.Fatal("sensitive value leaked into exception message")
	}
}

func findSpanAttribute(
	attributes []attribute.KeyValue,
	key attribute.Key,
) (attribute.Value, bool) {
	for _, attr := range attributes {
		if attr.Key == key {
			return attr.Value, true
		}
	}

	return attribute.Value{}, false
}
