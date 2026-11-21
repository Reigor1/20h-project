package core_observability

import (
	"fmt"

	redisotel_native "github.com/redis/go-redis/extra/redisotel-native/v9"
)

// InstrumentRedisMetrics enables the supported Redis command, connection, and resiliency metric groups.
// The instrumentation instance is retained so it can be shut down with Observability.
func (o *Observability) InstrumentRedisMetrics() error {
	if o == nil || o.meterProvider == nil {
		return fmt.Errorf("Redis metrics: meter provider is nil")
	}

	redisMetrics := redisotel_native.GetObservabilityInstance()

	config := redisotel_native.NewConfig().
		WithEnabled(true).
		WithMeterProvider(o.MeterProvider()).
		WithMetricGroups(
			redisotel_native.MetricGroupFlagCommand |
				redisotel_native.MetricGroupFlagConnectionBasic |
				redisotel_native.MetricGroupFlagResiliency |
				redisotel_native.MetricGroupFlagConnectionAdvanced,
		).
		WithRecordNilErrors(false)

	if err := redisMetrics.Init(config); err != nil {
		return fmt.Errorf("initialize Redis metrics instrumentation: %w", err)
	}

	o.redisMetrics = redisMetrics

	return nil
}
