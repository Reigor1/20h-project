package outbox_publisher

import (
	"context"
	"testing"
	"time"

	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	outbox_repository "github.com/Reigor1/20h-project/internal/features/outbox/repository"
	"go.uber.org/zap"
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


type shutdownTestRepository struct {
	entered chan struct{}
	release chan struct{}
}

func (r *shutdownTestRepository) ClaimPending(
	ctx context.Context,
	_ int,
	_ time.Duration,
) ([]outbox_repository.ClaimedEvent, error) {
	close(r.entered)

	select {
	case <-r.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*shutdownTestRepository) MarkPublished(
	context.Context,
	int64,
	string,
) error {
	return nil
}

func (*shutdownTestRepository) MarkFailed(
	context.Context,
	int64,
	string,
	error,
	time.Duration,
) error {
	return nil
}

type shutdownTestProducer struct{}

func (shutdownTestProducer) PublishBatch(
	context.Context,
	string,
	[]kafka_producer.Message,
) []error {
	return nil
}

func TestRunWaitsForActiveBatchAfterShutdownSignal(t *testing.T) {
	repository := &shutdownTestRepository{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	publisher, err := NewPublisher(
		Config{
			Topic:          "task-events",
			PollInterval:   time.Millisecond,
			BatchSize:      10,
			LeaseDuration:  time.Minute,
			RetryBaseDelay: time.Second,
			RetryMaxDelay:  time.Minute,
			ShutdownTimeout: time.Second,
		},
		repository,
		shutdownTestProducer{},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- publisher.Run(ctx)
	}()

	<-repository.entered
	cancel()

	select {
	case err := <-done:
		t.Fatalf("Run() returned before active batch finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(repository.release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after active batch finished")
	}
}

func TestRunCancelsActiveBatchAfterShutdownGracePeriod(t *testing.T) {
	repository := &shutdownTestRepository{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	publisher, err := NewPublisher(
		Config{
			Topic:           "task-events",
			PollInterval:    time.Millisecond,
			BatchSize:       10,
			LeaseDuration:   time.Minute,
			RetryBaseDelay:  time.Second,
			RetryMaxDelay:   time.Minute,
			ShutdownTimeout: 10 * time.Millisecond,
		},
		repository,
		shutdownTestProducer{},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- publisher.Run(ctx)
	}()

	<-repository.entered
	started := time.Now()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after the shutdown grace period")
	}

	if elapsed := time.Since(started); elapsed < 10*time.Millisecond {
		t.Fatalf("Run() stopped after %s, before grace period", elapsed)
	}
}
