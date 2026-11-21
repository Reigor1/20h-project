package core_observability_postgres

import (
	"context"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SanitizingTracer delegates pgx tracing to otelpgx after removing sensitive
// statement, identifier, and connection-error details from trace attributes.
type SanitizingTracer struct {
	*otelpgx.Tracer
}

var (
	_ pgx.QueryTracer       = (*SanitizingTracer)(nil)
	_ pgx.BatchTracer       = (*SanitizingTracer)(nil)
	_ pgx.CopyFromTracer    = (*SanitizingTracer)(nil)
	_ pgx.PrepareTracer     = (*SanitizingTracer)(nil)
	_ pgx.ConnectTracer     = (*SanitizingTracer)(nil)
	_ pgxpool.AcquireTracer = (*SanitizingTracer)(nil)
)

// NewSanitizingTracer creates a PostgreSQL tracer
// with sensitive SQL and connection details disabled.
func NewSanitizingTracer(
	tracerProvider oteltrace.TracerProvider,
	meterProvider metric.MeterProvider,
) *SanitizingTracer {
	return &SanitizingTracer{
		Tracer: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tracerProvider),
			otelpgx.WithMeterProvider(meterProvider),
			otelpgx.WithMetricOperationName(
				otelpgx.SQLOperationName,
			),
			otelpgx.WithMeterAttributes(
				attribute.String(
					"db.client.connection.pool.name",
					"primary",
				),
			),
			otelpgx.WithTracerAttributes(
				attribute.String(
					"db.client.connection.pool.name",
					"primary",
				),
			),
			otelpgx.WithDisableSQLStatementInAttributes(),
			otelpgx.WithDisableConnectionDetailsInAttributes(),
		),
	}
}

// sanitizedError exposes a generic message to telemetry while preserving the original
// error for errors.Is and errors.As through Unwrap.
type sanitizedError struct {
	cause error
}

func (e sanitizedError) Error() string {
	return "database operation failed"
}

func (e sanitizedError) Unwrap() error {
	return e.cause
}

// sanitizeError replaces the externally visible error message without losing its cause.
func sanitizeError(err error) error {
	if err == nil {
		return nil
	}

	return sanitizedError{cause: err}
}

// TraceQueryEnd sanitizes query errors before forwarding the completed query span.
func (t *SanitizingTracer) TraceQueryEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

// TraceBatchQuery sanitizes batch-query errors before forwarding the trace event.
func (t *SanitizingTracer) TraceBatchQuery(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceBatchQueryData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceBatchQuery(ctx, conn, data)
}

// TraceBatchEnd sanitizes the batch completion error before forwarding it to otelpgx.
func (t *SanitizingTracer) TraceBatchEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceBatchEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceBatchEnd(ctx, conn, data)
}

// TraceCopyFromStart redacts the destination table name before tracing a COPY operation.
func (t *SanitizingTracer) TraceCopyFromStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceCopyFromStartData,
) context.Context {
	data.TableName = pgx.Identifier{"redacted"}
	return t.Tracer.TraceCopyFromStart(ctx, conn, data)
}

// TraceCopyFromEnd sanitizes COPY errors before forwarding the completion event.
func (t *SanitizingTracer) TraceCopyFromEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceCopyFromEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceCopyFromEnd(ctx, conn, data)
}

// TracePrepareStart removes the prepared statement name before tracing preparation.
func (t *SanitizingTracer) TracePrepareStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TracePrepareStartData,
) context.Context {
	data.Name = ""
	return t.Tracer.TracePrepareStart(ctx, conn, data)
}

// TracePrepareEnd sanitizes preparation errors before forwarding the trace event.
func (t *SanitizingTracer) TracePrepareEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TracePrepareEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TracePrepareEnd(ctx, conn, data)
}

// TraceConnectEnd sanitizes connection errors before forwarding connection telemetry.
func (t *SanitizingTracer) TraceConnectEnd(
	ctx context.Context,
	data pgx.TraceConnectEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceConnectEnd(ctx, data)
}

// TraceAcquireEnd sanitizes pool-acquisition errors before forwarding the trace event.
func (t *SanitizingTracer) TraceAcquireEnd(
	ctx context.Context,
	pool *pgxpool.Pool,
	data pgxpool.TraceAcquireEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceAcquireEnd(ctx, pool, data)
}
