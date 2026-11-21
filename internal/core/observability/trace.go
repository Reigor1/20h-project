package core_observability

import (
	core_observability_postgres "github.com/Reigor1/20h-project/internal/core/observability/postgres"
	"github.com/jackc/pgx/v5"
)

// NewPostgresTracer creates a PostgreSQL tracer
// using the application's OpenTelemetry providers.
// NewPostgresTracer creates a pgx query tracer that records database operations
// as child spans in the active OpenTelemetry trace.
func (o *Observability) NewPostgresTracer() pgx.QueryTracer {
	return core_observability_postgres.NewSanitizingTracer(
		o.TracerProvider(),
		o.MeterProvider(),
	)
}
