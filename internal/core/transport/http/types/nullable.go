package core_http_types

import (
	"encoding/json"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

// Nullable preserves whether a JSON field was supplied, including explicit null values.
type Nullable[T any] struct {
	domain.Nullable[T]
}

// UnmarshalJSON marks the field as present and decodes null or a concrete value.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true

	if string(b) == "null" {
		n.Value = nil

		return nil
	}

	var value T
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}

	n.Value = &value

	return nil
}

// ToDomain converts the transport field into the domain patch representation.
func (n *Nullable[T]) ToDomain() domain.Nullable[T] {
	return domain.Nullable[T]{
		Value: n.Value,
		Set:   n.Set,
	}
}
