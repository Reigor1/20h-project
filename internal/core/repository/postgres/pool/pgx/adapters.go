package core_pgx_pool

import (
	"errors"
	"fmt"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgxRows adapts pgx.Rows to the repository Rows interface.
type pgxRows struct {
	pgx.Rows
}

// pgxRow adapts pgx.Row and maps scan errors to shared repository errors.
type pgxRow struct {
	pgx.Row
}

// Scan decodes the row and converts pgx-specific errors into repository error sentinels.
func (r pgxRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	if err != nil {
		return mapErrors(err)
	}

	return nil
}

// pgxCommandTag adapts pgconn.CommandTag to the repository CommandTag interface.
type pgxCommandTag struct {
	pgconn.CommandTag
}

// mapErrors translates common pgx failures into stable repository errors while
// retaining the original database error for diagnostics and error inspection.
func mapErrors(err error) error {
	if err == nil {
		return nil
	}

	const (
		pgxViolatesForeignKeyErrorCode = "23503"
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return core_postgres_pool.ErrNoRows
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == pgxViolatesForeignKeyErrorCode {
			return fmt.Errorf(
				"%v: %w",
				err,
				core_postgres_pool.ErrViolatesForeignKey,
			)
		}
	}

	return fmt.Errorf(
		"%v: %w",
		err,
		core_postgres_pool.ErrUnknown,
	)
}
