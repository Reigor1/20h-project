package core_postgres_pool

import (
	"context"
	"time"
)

// Executor defines the query and command operations shared by pools and transactions.
type Executor interface {
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Exec(ctx context.Context, sql string, arguments ...any) (CommandTag, error)
}

// Pool is the long-lived database executor and exposes its operation timeout and shutdown.
type Pool interface {
	Executor

	Close()
	OpTimeout() time.Duration
}

// Transaction is an executor whose changes can be committed or rolled back.
type Transaction interface {
	Executor

	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TransactionalPool extends Pool with the ability to begin database transactions.
type TransactionalPool interface {
	Pool

	Begin(ctx context.Context) (Transaction, error)
}

// Rows is the subset of pgx.Rows required by repository implementations.
type Rows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

// Row represents a single query result that is decoded with Scan.
type Row interface {
	Scan(dest ...any) error
}

// CommandTag exposes the number of rows affected by a database command.
type CommandTag interface {
	RowsAffected() int64
}
