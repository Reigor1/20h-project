package testutil

import (
	"context"
	"testing"
	"time"
)

// RepositoryOperationTimeout is the operation timeout used by repository test pools.
const RepositoryOperationTimeout = 3 * time.Second

// RepositoryContextKey identifies the value used to verify parent-context preservation.
type RepositoryContextKey struct{}

// AssertRepositoryContext checks that a repository passed a derived context to its pool.
func AssertRepositoryContext(t *testing.T, got, parent context.Context) {
	t.Helper()

	if got == nil {
		t.Fatal("repository did not pass a context to the pool")
	}
	if got.Value(RepositoryContextKey{}) != parent.Value(RepositoryContextKey{}) {
		t.Fatal("repository context did not preserve the parent context value")
	}
	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("repository context has no timeout deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > RepositoryOperationTimeout {
		t.Fatalf(
			"repository context deadline remaining = %v, want within %v",
			remaining,
			RepositoryOperationTimeout,
		)
	}
}
