package core_observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	redisotel_native "github.com/redis/go-redis/extra/redisotel-native/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Observability struct {
	meterProvider  *sdkmetric.MeterProvider
	tracerProvider *sdktrace.TracerProvider
	propagators    propagation.TextMapPropagator

	activeRequests metric.Int64UpDownCounter

	redisMetrics *redisotel_native.ObservabilityInstance
}

func New(ctx context.Context, config Config) (*Observability, error) {
	config = config.withDefaults()

	res, err := newResource(ctx, config)
	if err != nil {
		return nil, err
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create OTLP metrics exporter: %w", err),
			shutdownProvidersAfterInitializationFailure(
				tracerProvider,
				nil,
			),
		)
	}

	reader := sdkmetric.NewPeriodicReader(
		metricExporter,
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
		return nil, errors.Join(
			fmt.Errorf("create active requests metric: %w", err),
			shutdownProvidersAfterInitializationFailure(
				tracerProvider,
				meterProvider,
			),
		)
	}

	propagators := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	return &Observability{
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
		propagators:    propagators,
		activeRequests: activeRequests,
	}, nil
}

func newResource(
	ctx context.Context,
	config Config,
) (*resource.Resource, error) {
	res, err := resource.New(
		ctx,
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
		resource.WithAttributes(
			attribute.String("service.name", config.ServiceName),
			attribute.String("service.version", config.ServiceVersion),
			attribute.String(
				"deployment.environment.name",
				config.DeploymentEnvironment,
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	return res, nil
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
		otelhttp.WithTracerProvider(o.tracerProvider),
		otelhttp.WithPropagators(o.propagators),
		otelhttp.WithMeterProvider(o.meterProvider),
	)
}

// TracerProvider exposes the SDK provider for tracing integrations.
func (o *Observability) TracerProvider() trace.TracerProvider {
	return o.tracerProvider
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
	if o == nil {
		return nil
	}

	var shutdownErr error

	if o.redisMetrics != nil {
		if err := o.redisMetrics.Shutdown(); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("shutdown Redis metrics: %w", err),
			)
		}

		o.redisMetrics = nil
	}

	return errors.Join(
		shutdownErr,
		shutdownProviders(
			ctx,
			o.tracerProvider,
			o.meterProvider,
		),
	)
}

const providerInitializationCleanupTimeout = 5 * time.Second

func shutdownProviders(
	ctx context.Context,
	tracerProvider *sdktrace.TracerProvider,
	meterProvider *sdkmetric.MeterProvider,
) error {
	var shutdownErr error

	if tracerProvider != nil {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("shutdown tracer provider: %w", err),
			)
		}
	}

	if meterProvider != nil {
		if err := meterProvider.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("shutdown meter provider: %w", err),
			)
		}
	}

	return shutdownErr
}

func shutdownProvidersAfterInitializationFailure(
	tracerProvider *sdktrace.TracerProvider,
	meterProvider *sdkmetric.MeterProvider,
) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		providerInitializationCleanupTimeout,
	)
	defer cancel()

	return shutdownProviders(ctx, tracerProvider, meterProvider)
}
