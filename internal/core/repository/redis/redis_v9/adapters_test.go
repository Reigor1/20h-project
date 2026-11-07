package core_redis_v9

import (
	"context"
	"errors"
	"net"
	"testing"

	core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"
	redis "github.com/redis/go-redis/v9"
)

func TestMapErrors(t *testing.T) {
	originalError := errors.New("unexpected Redis response")
	networkError := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}

	tests := []struct {
		name      string
		input     error
		want      error
		wantNil   bool
		wantSame  bool
		wantCause error
	}{
		{
			name:    "nil remains nil",
			wantNil: true,
		},
		{
			name:  "missing key maps to cache miss",
			input: redis.Nil,
			want:  core_redis.ErrCacheMiss,
		},
		{
			name:      "network error maps to unavailable",
			input:     networkError,
			want:      core_redis.ErrUnavailable,
			wantCause: networkError,
		},
		{
			name:      "unknown error maps to unknown",
			input:     originalError,
			want:      core_redis.ErrUnknown,
			wantCause: originalError,
		},
		{
			name:     "context cancellation is preserved",
			input:    context.Canceled,
			want:     context.Canceled,
			wantSame: true,
		},
		{
			name:     "context deadline is preserved",
			input:    context.DeadlineExceeded,
			want:     context.DeadlineExceeded,
			wantSame: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := mapErrors(testCase.input)

			if testCase.wantNil {
				if got != nil {
					t.Fatalf("mapErrors() = %v, want nil", got)
				}
				return
			}

			if !errors.Is(got, testCase.want) {
				t.Fatalf("mapErrors() = %v, want error matching %v", got, testCase.want)
			}

			if testCase.wantSame && got != testCase.input {
				t.Fatalf("mapErrors() = %v, want original error %v", got, testCase.input)
			}

			if testCase.wantCause != nil && !errors.Is(got, testCase.wantCause) {
				t.Fatalf("mapErrors() = %v, want original cause %v to be preserved", got, testCase.wantCause)
			}
		})
	}
}
