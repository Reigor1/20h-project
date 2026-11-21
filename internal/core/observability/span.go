package core_observability

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// MarkSpanError marks a span as failed using a static description.
// Raw error messages are intentionally excluded to prevent sensitive data leaks.
// MarkSpanError records an operation failure on a span, including its error details,
// and marks the span status as failed.
func MarkSpanError(span trace.Span, operation string) {
	span.SetStatus(codes.Error, operation+" failed")
}
