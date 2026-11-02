package core_observability

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

type Observability struct {
	meterProvider  *sdkmetric.MeterProvider
	activeRequests metric.Int64UpDownCounter
}

func New(ctx context.Context, config Config) (*Observability, error) {
	config = config.withDefaults()

	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metrics exporter: %w", err)
	}

	res, err := resource.New(
		ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", config.ServiceName),
		),
		resource.WithFromEnv(),
	)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(config.MetricsExportInterval),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)

	meter := meterProvider.Meter(
		"github.com/Reigor1/20h-project/internal/core/observability",
	)

	activeRequests, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of active HTTP server requests."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		_ = meterProvider.Shutdown(ctx)

		return nil, fmt.Errorf("create active requests metric: %w", err)
	}

	return &Observability{
		meterProvider:  meterProvider,
		activeRequests: activeRequests,
	}, nil
}

// InstrumentHTTPRoute instruments one registered business API route.
// path must be a route template, never a concrete request URL.
func (o *Observability) InstrumentHTTPRoute(
	method string,
	path string,
	next http.Handler,
) http.Handler {
	operation := fmt.Sprintf("%s %s", method, path)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Pattern = path

		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}

		ctx := r.Context()

		attributes := metric.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("http.route", path),
			attribute.String("url.scheme", scheme),
		)

		o.activeRequests.Add(ctx, 1, attributes)
		defer o.activeRequests.Add(ctx, -1, attributes)

		next.ServeHTTP(w, r)
	})

	return otelhttp.NewHandler(
		handler,
		operation,
		otelhttp.WithMeterProvider(o.meterProvider),
	)
}

// MeterProvider exposes the SDK provider for additional metric collectors.
func (o *Observability) MeterProvider() metric.MeterProvider {
	return o.meterProvider
}

// ActiveRequestsCounter returns the active requests counter.
func (o *Observability) ActiveRequestsCounter() metric.Int64UpDownCounter {
	return o.activeRequests
}

func (o *Observability) Shutdown(ctx context.Context) error {
	if o == nil || o.meterProvider == nil {
		return nil
	}

	if err := o.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown meter provider: %w", err)
	}

	return nil
}
