package core_observability_postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type PGXPool interface {
	Stat() *pgxpool.Stat
}

type PoolMetrics struct {
	registration metric.Registration
}

func RegisterPoolMetrics(
	provider metric.MeterProvider,
	pool PGXPool,
) (*PoolMetrics, error) {
	meter := provider.Meter(
		"github.com/Reigor1/20h-project/internal/core/observability/postgres",
	)

	total, err := meter.Int64ObservableGauge(
		"postgresql.pool.connections.total",
		metric.WithDescription("Total number of connections in the pool."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create total connections metric: %w", err)
	}

	acquired, err := meter.Int64ObservableGauge(
		"postgresql.pool.connections.acquired",
		metric.WithDescription("Number of acquired connections."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create acquired connections metric: %w", err)
	}

	idle, err := meter.Int64ObservableGauge(
		"postgresql.pool.connections.idle",
		metric.WithDescription("Number of idle connections."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create idle connections metric: %w", err)
	}

	max, err := meter.Int64ObservableGauge(
		"postgresql.pool.connections.max",
		metric.WithDescription("Maximum number of connections."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create max connections metric: %w", err)
	}

	constructing, err := meter.Int64ObservableGauge(
		"postgresql.pool.connections.constructing",
		metric.WithDescription("Number of connections being constructed."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create constructing connections metric: %w", err)
	}

	poolName := metric.WithAttributes(
		attribute.String("db.client.connection.pool.name", "primary"),
	)

	registration, err := meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			stats := pool.Stat()

			observer.ObserveInt64(
				total,
				int64(stats.TotalConns()),
				poolName,
			)
			observer.ObserveInt64(
				acquired,
				int64(stats.AcquiredConns()),
				poolName,
			)
			observer.ObserveInt64(
				idle,
				int64(stats.IdleConns()),
				poolName,
			)
			observer.ObserveInt64(
				max,
				int64(stats.MaxConns()),
				poolName,
			)
			observer.ObserveInt64(
				constructing,
				int64(stats.ConstructingConns()),
				poolName,
			)

			return nil
		},
		total,
		acquired,
		idle,
		max,
		constructing,
	)
	if err != nil {
		return nil, fmt.Errorf("register pool metrics callback: %w", err)
	}

	return &PoolMetrics{
		registration: registration,
	}, nil
}

func (m *PoolMetrics) Shutdown() error {
	if m == nil || m.registration == nil {
		return nil
	}

	if err := m.registration.Unregister(); err != nil {
		return fmt.Errorf("unregister pool metrics: %w", err)
	}

	return nil
}