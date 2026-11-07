package core_redis_v9

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"
	redis "github.com/redis/go-redis/v9"
)

// Redis implements the shared Cache contract using go-redis/v9.
type Redis struct {
	client *redis.Client

	closeOnce sync.Once
	closeErr  error
}

var _ core_redis.Cache = (*Redis)(nil)

// NewRedis creates a Redis client and verifies connectivity with PING.
// A failed initial connection is returned to the caller; the application
// bootstrap decides whether Redis failure is fatal or whether cache use
// should be disabled while the application continues to run.
func NewRedis(ctx context.Context, config Config) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:                  config.Addr,
		Username:              config.Username,
		Password:              config.Password,
		DB:                    config.DB,
		DialTimeout:           config.Timeout,
		ReadTimeout:           config.Timeout,
		WriteTimeout:          config.Timeout,
		ContextTimeoutEnabled: true,
	})

	pingCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		closeErr := client.Close()
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close Redis client after failed ping: %w", closeErr))
		}

		return nil, fmt.Errorf("Redis ping: %w", mapErrors(err))
	}

	return &Redis{client: client}, nil
}

// Get returns the value stored under key. A missing key is reported as
// core_redis.ErrCacheMiss.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, mapErrors(err)
	}

	return value, nil
}

// Set stores value under key. TTL must be strictly positive.
func (r *Redis) Set(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) error {
	if ttl <= 0 {
		return core_redis.ErrInvalidTTL
	}

	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return mapErrors(err)
	}

	return nil
}

// Delete removes key. Deleting a key that does not exist is not an error.
func (r *Redis) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return mapErrors(err)
	}

	return nil
}

// Close closes the Redis client. Repeated calls are safe.
func (r *Redis) Close() error {
	if r == nil || r.client == nil {
		return nil
	}

	r.closeOnce.Do(func() {
		r.closeErr = mapErrors(r.client.Close())
	})

	return r.closeErr
}
