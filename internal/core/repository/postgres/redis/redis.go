package core_redis

import (
	"context"
	"time"
)

// Cache is the application-wide contract for byte-oriented cache operations.
// Serialization, key construction, TTL selection, and cache policies belong
// to the feature packages that use this interface.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Close() error
}
