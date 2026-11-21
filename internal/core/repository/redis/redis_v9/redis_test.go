package core_redis_v9

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"
	"github.com/alicebob/miniredis/v2"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestRedisGetSetDelete exercises the cache adapter's basic read, write, and delete operations.
func TestRedisGetSetDelete(t *testing.T) {
	server := miniredis.RunT(t)
	cache := newTestRedis(t, server.Addr())
	ctx := context.Background()

	if _, err := cache.Get(ctx, "test:key"); !errors.Is(err, core_redis.ErrCacheMiss) {
		t.Fatalf("Get() error = %v, want ErrCacheMiss", err)
	}

	value := []byte(`{"status":"ok"}`)
	ttl := 30 * time.Second
	if err := cache.Set(ctx, "test:key", value, ttl); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}

	if gotTTL := server.TTL("test:key"); gotTTL != ttl {
		t.Errorf("stored TTL = %s, want %s", gotTTL, ttl)
	}

	got, err := cache.Get(ctx, "test:key")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("Get() = %q, want %q", got, value)
	}

	if err := cache.Delete(ctx, "test:key"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if server.Exists("test:key") {
		t.Fatal("key still exists after Delete()")
	}
	if _, err := cache.Get(ctx, "test:key"); !errors.Is(err, core_redis.ErrCacheMiss) {
		t.Fatalf("Get() after Delete() error = %v, want ErrCacheMiss", err)
	}

	if err := cache.Delete(ctx, "missing:key"); err != nil {
		t.Fatalf("Delete() of a missing key error = %v, want nil", err)
	}
}

// TestRedisSetRejectsNonPositiveTTL verifies that cache entries require a positive expiration.
func TestRedisSetRejectsNonPositiveTTL(t *testing.T) {
	server := miniredis.RunT(t)
	cache := newTestRedis(t, server.Addr())

	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{name: "zero TTL", ttl: 0},
		{name: "negative TTL", ttl: -time.Second},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := cache.Set(context.Background(), "invalid:ttl", []byte("value"), testCase.ttl)
			if !errors.Is(err, core_redis.ErrInvalidTTL) {
				t.Fatalf("Set() error = %v, want ErrInvalidTTL", err)
			}
			if server.Exists("invalid:ttl") {
				t.Fatal("Set() wrote a key with a non-positive TTL")
			}
		})
	}
}

// TestNewRedisFailsWhenInitialPingCannotReachServer checks startup failure and cleanup
// when the configured Redis endpoint is unavailable.
func TestNewRedisFailsWhenInitialPingCannotReachServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on temporary port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	cache, err := NewRedis(context.Background(), Config{
		Addr:    addr,
		Timeout: 250 * time.Millisecond,
	}, noop.NewTracerProvider())
	if err == nil {
		if cache != nil {
			_ = cache.Close()
		}
		t.Fatal("NewRedis() error = nil, want connection error")
	}
	if cache != nil {
		t.Fatalf("NewRedis() cache = %v, want nil on failed initial Ping", cache)
	}
}

// TestRedisCloseIsIdempotent verifies that repeated client shutdown calls are safe.
func TestRedisCloseIsIdempotent(t *testing.T) {
	server := miniredis.RunT(t)
	cache := newTestRedis(t, server.Addr())

	if err := cache.Close(); err != nil {
		t.Fatalf("first Close() error = %v, want nil", err)
	}
	if err := cache.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
}

// newTestRedis starts a test Redis server and registers its cleanup with the test.
func newTestRedis(t *testing.T, addr string) *Redis {
	t.Helper()

	cache, err := NewRedis(context.Background(), Config{
		Addr:    addr,
		Timeout: time.Second,
	}, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("NewRedis() error = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("close Redis cache: %v", err)
		}
	})

	return cache
}
