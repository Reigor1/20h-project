package core_observability_postgres

import (
	"fmt"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type PGXPool interface {
	Stat() *pgxpool.Stat
}

func RegisterPoolMetrics(
	provider metric.MeterProvider,
	pool otelpgx.PoolStats,
) error {
	if provider == nil {
		return fmt.Errorf("meter provider is nil")
	}

	if pool == nil {
		return fmt.Errorf("postgres pool is nil")
	}

	if err := otelpgx.RecordStats(
		pool,
		otelpgx.WithStatsMeterProvider(provider),
		otelpgx.WithStatsAttributes(
			attribute.String(
				"db.client.connection.pool.name",
				"primary",
			),
		),
	); err != nil {
		return fmt.Errorf("record postgres pool stats: %w", err)
	}

	return nil
}
