package core_redis

import "errors"

// Shared Redis errors let callers distinguish cache misses, unavailable connections,
// and operation timeouts without depending on a concrete Redis client.
var (
	// ErrCacheMiss means that the requested key does not exist in Redis.
	ErrCacheMiss = errors.New("cache miss")

	// ErrUnavailable means that Redis could not be reached or its client is closed.
	ErrUnavailable = errors.New("redis unavailable")

	// ErrUnknown marks an error returned by Redis that has no dedicated mapping.
	ErrUnknown = errors.New("unknown redis error")

	// ErrInvalidTTL means that a cache entry was created with a non-positive TTL.
	ErrInvalidTTL = errors.New("cache TTL must be greater than zero")
)
