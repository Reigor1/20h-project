package core_pgx_pool

import (
	"errors"
	"testing"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapErrors(t *testing.T) {
	foreignKeyError := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
	otherPostgresError := &pgconn.PgError{Code: "23505", Message: "unique violation"}
	ordinaryError := errors.New("connection failed")

	tests := []struct {
		name    string
		input   error
		wantErr error
	}{
		{
			name:    "no rows maps to package sentinel",
			input:   pgx.ErrNoRows,
			wantErr: core_postgres_pool.ErrNoRows,
		},
		{
			name:    "foreign key violation maps to package sentinel",
			input:   foreignKeyError,
			wantErr: core_postgres_pool.ErrViolatesForeignKey,
		},
		{
			name:    "other PostgreSQL errors map to unknown",
			input:   otherPostgresError,
			wantErr: core_postgres_pool.ErrUnknown,
		},
		{
			name:    "ordinary errors map to unknown",
			input:   ordinaryError,
			wantErr: core_postgres_pool.ErrUnknown,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := mapErrors(testCase.input)
			if !errors.Is(got, testCase.wantErr) {
				t.Fatalf("mapErrors() = %v, want error matching %v", got, testCase.wantErr)
			}
		})
	}
}

func TestPGXRowScanMapsErrors(t *testing.T) {
	foreignKeyError := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
	otherPostgresError := &pgconn.PgError{Code: "23505", Message: "unique violation"}
	ordinaryError := errors.New("scan failed")

	tests := []struct {
		name    string
		rowErr  error
		wantErr error
	}{
		{
			name: "successful scan returns nil",
		},
		{
			name:    "no rows maps to package sentinel",
			rowErr:  pgx.ErrNoRows,
			wantErr: core_postgres_pool.ErrNoRows,
		},
		{
			name:    "foreign key violation maps to package sentinel",
			rowErr:  foreignKeyError,
			wantErr: core_postgres_pool.ErrViolatesForeignKey,
		},
		{
			name:    "other PostgreSQL errors map to unknown",
			rowErr:  otherPostgresError,
			wantErr: core_postgres_pool.ErrUnknown,
		},
		{
			name:    "ordinary errors map to unknown",
			rowErr:  ordinaryError,
			wantErr: core_postgres_pool.ErrUnknown,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			row := &fakePGXRow{scanErr: testCase.rowErr}
			adapter := pgxRow{Row: row}

			gotErr := adapter.Scan()
			if testCase.wantErr == nil {
				if gotErr != nil {
					t.Fatalf("Scan() error = %v, want nil", gotErr)
				}
			} else if !errors.Is(gotErr, testCase.wantErr) {
				t.Fatalf("Scan() error = %v, want error matching %v", gotErr, testCase.wantErr)
			}

			if row.scanCalls != 1 {
				t.Fatalf("underlying Scan() calls = %d, want 1", row.scanCalls)
			}
		})
	}
}

type fakePGXRow struct {
	scanErr   error
	scanCalls int
}

func (r *fakePGXRow) Scan(...any) error {
	r.scanCalls++
	return r.scanErr
}

func (*fakePGXRow) RawValues() [][]byte {
	return nil
}

func (*fakePGXRow) Conn() *pgx.Conn {
	return nil
}
