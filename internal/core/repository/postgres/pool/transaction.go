package core_postgres_pool

import (
	"context"
	"fmt"
	"time"
)

// BeginTransaction creates a transaction using a context bounded by timeout.
// The returned cancel function must be called by the caller after transaction handling.
func BeginTransaction(
	ctx context.Context,
	pool TransactionalPool,
	timeout time.Duration,
) (
	context.Context,
	context.CancelFunc,
	Transaction,
	error,
) {
	txCtx, cancel := context.WithTimeout(ctx, timeout)

	tx, err := pool.Begin(txCtx)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf(
			"begin PostgreSQL transaction: %w",
			err,
		)
	}

	return txCtx, cancel, tx, nil
}

// RollbackTransaction attempts rollback with a short independent timeout so it can
// still clean up after the request context has been canceled.
func RollbackTransaction(ctx context.Context, tx Transaction, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	// Rollback after a successful Commit returns a closed-transaction error.
	// The primary operation or commit error is handled by the caller.
	_ = tx.Rollback(ctx)
}
