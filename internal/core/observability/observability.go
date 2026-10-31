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
	meterProvider *sdkmetric.MeterProvider
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

	return &Observability{
		meterProvider: meterProvider,
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
		// The API version prefix is stripped before the nested ServeMux matches.
		// Restore the externally visible route template for OpenTelemetry.
		r.Pattern = path
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

func (o *Observability) Shutdown(ctx context.Context) error {
	if o == nil || o.meterProvider == nil {
		return nil
	}

	if err := o.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown meter provider: %w", err)
	}

	return nil
}
