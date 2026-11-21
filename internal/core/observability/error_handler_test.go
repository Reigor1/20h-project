package core_observability

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestOpenTelemetryExportErrorsAreLogged verifies that failed OTLP trace and metric exports
// are forwarded to the application's error logger.
func TestOpenTelemetryExportErrorsAreLogged(t *testing.T) {
	originalErrorHandler := otel.GetErrorHandler()
	t.Cleanup(func() {
		otel.SetErrorHandler(originalErrorHandler)
	})

	logCore, observedLogs := observer.New(zapcore.ErrorLevel)
	logger := &core_logger.Logger{
		Logger: zap.New(logCore),
	}

	ConfigureErrorHandler(logger)

	otlpServer := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		http.Error(
			w,
			fmt.Sprintf("test rejected export at %s", r.URL.Path),
			http.StatusBadRequest,
		)
	}))
	t.Cleanup(otlpServer.Close)

	t.Setenv(
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		otlpServer.URL+"/v1/traces",
	)
	t.Setenv(
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		otlpServer.URL+"/v1/metrics",
	)
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "10")

	observability, err := New(
		context.Background(),
		Config{
			ServiceName:           "20h-project-test",
			ServiceVersion:        "test",
			DeploymentEnvironment: "test",
			MetricsExportInterval: 10 * time.Millisecond,
		},
	)
	if err != nil {
		t.Fatalf("create observability: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Shutdown may return the expected export errors. The test verifies
		// background export diagnostics, so those errors are not test failures.
		_ = observability.Shutdown(ctx)
	})

	_, span := observability.TracerProvider().
		Tracer("observability-error-handler-test").
		Start(context.Background(), "test.export.failure")
	span.End()

	counter, err := observability.MeterProvider().
		Meter("observability-error-handler-test").
		Int64Counter("test.export.failure")
	if err != nil {
		t.Fatalf("create test counter: %v", err)
	}
	counter.Add(context.Background(), 1)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries := observedLogs.FilterMessage("OpenTelemetry SDK error").All()
		var traceErrorLogged, metricErrorLogged bool

		for _, entry := range entries {
			fields := entry.ContextMap()
			exportError, ok := fields["error"].(string)
			if !ok {
				continue
			}

			traceErrorLogged = traceErrorLogged ||
				strings.Contains(exportError, "/v1/traces")
			metricErrorLogged = metricErrorLogged ||
				strings.Contains(exportError, "/v1/metrics")
		}

		if traceErrorLogged && metricErrorLogged {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	entries := observedLogs.FilterMessage("OpenTelemetry SDK error").All()
	var loggedErrors []string
	for _, entry := range entries {
		loggedErrors = append(loggedErrors, fmt.Sprint(entry.ContextMap()["error"]))
	}

	t.Fatalf(
		"did not observe both trace and metric export errors; logged errors: %v",
		loggedErrors,
	)
}
