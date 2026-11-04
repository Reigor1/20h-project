package users_service

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// markSpanError uses a static description so repository error text cannot leak
// database values into the trace status.
func markSpanError(span trace.Span, operation string) {
	span.SetStatus(codes.Error, operation+" failed")
}