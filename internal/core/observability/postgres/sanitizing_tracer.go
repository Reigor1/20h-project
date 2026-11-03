package core_observability_postgres

import (
	"context"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func NewSanitizingTracer(tracer *otelpgx.Tracer) *SanitizingTracer {
	return &SanitizingTracer{
		Tracer: tracer,
	}
}

type sanitizedError struct {
	cause error
}

func (e sanitizedError) Error() string {
	return "database operation failed"
}

func (e sanitizedError) Unwrap() error {
	return e.cause
}

func sanitizeError(err error) error {
	if err == nil {
		return nil
	}

	return sanitizedError{cause: err}
}

func (t *SanitizingTracer) TraceQueryEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func (t *SanitizingTracer) TraceBatchQuery(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceBatchQueryData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceBatchQuery(ctx, conn, data)
}

func (t *SanitizingTracer) TraceBatchEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceBatchEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceBatchEnd(ctx, conn, data)
}

func (t *SanitizingTracer) TraceCopyFromStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceCopyFromStartData,
) context.Context {
	data.TableName = pgx.Identifier{"redacted"}
	return t.Tracer.TraceCopyFromStart(ctx, conn, data)
}

func (t *SanitizingTracer) TraceCopyFromEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceCopyFromEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceCopyFromEnd(ctx, conn, data)
}

func (t *SanitizingTracer) TracePrepareStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TracePrepareStartData,
) context.Context {
	data.Name = ""
	return t.Tracer.TracePrepareStart(ctx, conn, data)
}

func (t *SanitizingTracer) TracePrepareEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TracePrepareEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TracePrepareEnd(ctx, conn, data)
}

func (t *SanitizingTracer) TraceConnectEnd(
	ctx context.Context,
	data pgx.TraceConnectEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceConnectEnd(ctx, data)
}

func (t *SanitizingTracer) TraceAcquireEnd(
	ctx context.Context,
	pool *pgxpool.Pool,
	data pgxpool.TraceAcquireEndData,
) {
	data.Err = sanitizeError(data.Err)
	t.Tracer.TraceAcquireEnd(ctx, pool, data)
}
