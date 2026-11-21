package domain

// Nullable distinguishes an absent value from an explicitly provided zero value
// or null, which is useful when processing partial updates. Set indicates whether the
// field was supplied; Value may be nil when the caller explicitly supplied null.
type Nullable[T any] struct {
	Value *T
	Set   bool
}
