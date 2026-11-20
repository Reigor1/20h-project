package core_postgres_pool

import (
	"context"
	"time"
)

type Executor interface {
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Exec(ctx context.Context, sql string, arguments ...any) (CommandTag, error)
}

type Pool interface {
	Executor

	Close()
	OpTimeout() time.Duration
}

type Transaction interface {
	Executor

	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type TransactionalPool interface {
	Pool

	Begin(ctx context.Context) (Transaction, error)
}

type Rows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

type Row interface {
	Scan(dest ...any) error
}

type CommandTag interface {
	RowsAffected() int64
}
