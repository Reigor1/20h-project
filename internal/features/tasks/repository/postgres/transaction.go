package tasks_postgres_repository

import (
	"context"
	"fmt"
	"time"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

func (r *TasksRepository) beginTransaction(
	ctx context.Context,
) (
	context.Context,
	context.CancelFunc,
	core_postgres_pool.Transaction,
	error,
) {
	ctx, cancel := context.WithTimeout(ctx, r.pool.OpTimeout())

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("begin PostgreSQL transaction: %w", err)
	}

	return ctx, cancel, tx, nil
}

func rollbackTransaction(tx core_postgres_pool.Transaction) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	// Rollback after a successful Commit returns a closed-transaction error.
	// The primary operation or commit error is handled by the caller.
	_ = tx.Rollback(ctx)
}
