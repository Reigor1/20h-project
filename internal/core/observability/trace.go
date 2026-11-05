package core_observability

import (
	core_observability_postgres "github.com/Reigor1/20h-project/internal/core/observability/postgres"
	"github.com/jackc/pgx/v5"
)

// NewPostgresTracer creates a PostgreSQL tracer
// using the application's OpenTelemetry providers.
func (o *Observability) NewPostgresTracer() pgx.QueryTracer {
	return core_observability_postgres.NewSanitizingTracer(
		o.TracerProvider(),
		o.MeterProvider(),
	)
}
