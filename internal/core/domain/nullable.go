package domain

// Nullable distinguishes an absent value from an explicitly provided zero value
// or null, which is useful when processing partial updates.
type Nullable[T any] struct {
	Value *T
	Set   bool
}
