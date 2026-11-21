package core_pgx_pool

import (
	"context"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/jackc/pgx/v5"
)

// pgxTransaction adapts pgx.Tx to the application's database transaction interface.
type pgxTransaction struct {
	tx pgx.Tx
}

// Query executes a transaction-scoped query and maps pgx errors to repository errors.
func (tx pgxTransaction) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.Rows, error) {
	rows, err := tx.tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapErrors(err)
	}

	return pgxRows{rows}, nil
}

// QueryRow executes a transaction-scoped single-row query.
func (tx pgxTransaction) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) core_postgres_pool.Row {
	return pgxRow{
		Row: tx.tx.QueryRow(ctx, sql, args...),
	}
}

// Exec executes a transaction-scoped command and adapts its command tag.
func (tx pgxTransaction) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.CommandTag, error) {
	tag, err := tx.tx.Exec(ctx, sql, args...)
	if err != nil {
		return nil, mapErrors(err)
	}

	return pgxCommandTag{tag}, nil
}

// Commit atomically commits the transaction and maps any pgx error.
func (tx pgxTransaction) Commit(ctx context.Context) error {
	return mapErrors(tx.tx.Commit(ctx))
}

// Rollback aborts the transaction and maps any pgx error.
func (tx pgxTransaction) Rollback(ctx context.Context) error {
	return mapErrors(tx.tx.Rollback(ctx))
}
