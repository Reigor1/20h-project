package core_redis_v9

import (
	"context"
	"errors"
	"io"
	"net"

	core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"
	redis "github.com/redis/go-redis/v9"
)

// mapErrors converts go-redis errors into errors owned by the Redis
// infrastructure package. Context cancellation and deadlines are preserved.
// mapErrors translates go-redis errors into the shared cache error contract.
func mapErrors(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, redis.Nil) {
		return core_redis.ErrCacheMiss
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var networkError net.Error
	if errors.As(err, &networkError) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, redis.ErrClosed) {
		return errors.Join(core_redis.ErrUnavailable, err)
	}

	return errors.Join(core_redis.ErrUnknown, err)
}
