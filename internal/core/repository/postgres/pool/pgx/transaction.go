package core_pgx_pool

import (
	"context"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/jackc/pgx/v5"
)

type pgxTransaction struct {
	tx pgx.Tx
}

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

func (tx pgxTransaction) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) core_postgres_pool.Row {
	return pgxRow{
		Row: tx.tx.QueryRow(ctx, sql, args...),
	}
}

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

func (tx pgxTransaction) Commit(ctx context.Context) error {
	return mapErrors(tx.tx.Commit(ctx))
}

func (tx pgxTransaction) Rollback(ctx context.Context) error {
	return mapErrors(tx.tx.Rollback(ctx))
}
