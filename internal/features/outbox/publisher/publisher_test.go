package outbox_publisher

import (
	"testing"
	"time"
)

func TestRetryDelay(t *testing.T) {
	publisher := &Publisher{
		config: Config{
			RetryBaseDelay: time.Second,
			RetryMaxDelay:  5 * time.Minute,
		},
	}

	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{
			name:    "first attempt",
			attempt: 1,
			want:    time.Second,
		},
		{
			name:    "second attempt",
			attempt: 2,
			want:    2 * time.Second,
		},
		{
			name:    "third attempt",
			attempt: 3,
			want:    4 * time.Second,
		},
		{
			name:    "fourth attempt",
			attempt: 4,
			want:    8 * time.Second,
		},
		{
			name:    "retry delay is capped",
			attempt: 20,
			want:    5 * time.Minute,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := publisher.retryDelay(testCase.attempt)

			if got != testCase.want {
				t.Fatalf(
					"retryDelay(%d) = %s, want %s",
					testCase.attempt,
					got,
					testCase.want,
				)
			}
		})
	}
}
