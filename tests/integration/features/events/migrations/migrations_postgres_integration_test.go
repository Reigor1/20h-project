package migrations_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	"github.com/Reigor1/20h-project/internal/testutil"
)

// postgresUniqueViolationCode defines a shared limit or value used by the test workflow.
const postgresUniqueViolationCode = "23505"

// TestEventStorageMigrationsIntegration verifies the end-to-end or integration behavior represented by its test scenarios.
func TestEventStorageMigrationsIntegration(t *testing.T) {
	config := testutil.PostgresTestConfig(t)

	if !strings.HasSuffix(strings.ToLower(config.Database), "_test") {
		t.Fatalf(
			"refusing to modify database %q: database name must end in _test",
			config.Database,
		)
	}

	connectionContext, cancelConnection := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancelConnection()

	pool, err := core_pgx_pool.NewPool(connectionContext, config, nil)
	if err != nil {
		t.Fatalf("connect to configured PostgreSQL test database: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancelOperations := context.WithTimeout(
		context.Background(),
		time.Minute,
	)
	defer cancelOperations()

	assertConnectedTestDatabase(t, ctx, pool, config.Database)

	// This test applies the complete migration sequence to a dedicated test database.
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS todoapp CASCADE"); err != nil {
		t.Fatalf("reset todoapp schema: %v", err)
	}

	applyMigration(t, ctx, pool, "000001_init.up.sql")
	applyMigration(t, ctx, pool, "000002_create_outbox.up.sql")
	applyMigration(t, ctx, pool, "000003_create_task_activity.up.sql")

	t.Run("outbox rejects duplicate event ID", func(t *testing.T) {
		eventID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")
		occurredAt := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

		insertOutboxEvent(t, ctx, pool, eventID, occurredAt)

		err := insertOutboxEventError(ctx, pool, eventID, occurredAt)

		assertUniqueViolation(t, "outbox_event_id_unique", err)
	})

	t.Run("task activity rejects duplicate event ID", func(t *testing.T) {
		eventID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174002")
		occurredAt := time.Date(2026, 11, 12, 10, 5, 0, 0, time.UTC)

		insertTaskActivity(t, ctx, pool, eventID, occurredAt)

		err := insertTaskActivityError(ctx, pool, eventID, occurredAt)

		assertUniqueViolation(t, "task_activity_event_id_unique", err)
	})
}

// applyMigration supports setup, execution, or verification of the corresponding test scenario.
func applyMigration(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	filename string,
) {
	t.Helper()

	migrationPath := filepath.Join(
		"..", "..", "..", "..", "..",
		"migrations",
		filename,
	)

	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration %q: %v", migrationPath, err)
	}

	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply migration %q: %v", filename, err)
	}
}

// insertOutboxEvent inserts the corresponding fixture data into the integration-test database.
func insertOutboxEvent(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	eventID uuid.UUID,
	occurredAt time.Time,
) {
	t.Helper()

	if err := insertOutboxEventError(ctx, pool, eventID, occurredAt); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
}

// insertOutboxEventError inserts the corresponding fixture data into the integration-test database.
func insertOutboxEventError(
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	eventID uuid.UUID,
	occurredAt time.Time,
) error {
	_, err := pool.Exec(
		ctx,
		`INSERT INTO todoapp.outbox (
			event_id,
			event_type,
			schema_version,
			aggregate_type,
			aggregate_id,
			aggregate_version,
			occurred_at,
			payload
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		eventID,
		"task.created",
		1,
		"task",
		123,
		1,
		occurredAt,
		`{"title":"Prepare report"}`,
	)

	return err
}

// insertTaskActivity inserts the corresponding fixture data into the integration-test database.
func insertTaskActivity(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	eventID uuid.UUID,
	occurredAt time.Time,
) {
	t.Helper()

	if err := insertTaskActivityError(ctx, pool, eventID, occurredAt); err != nil {
		t.Fatalf("insert task activity: %v", err)
	}
}

// insertTaskActivityError inserts the corresponding fixture data into the integration-test database.
func insertTaskActivityError(
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	eventID uuid.UUID,
	occurredAt time.Time,
) error {
	_, err := pool.Exec(
		ctx,
		`INSERT INTO todoapp.task_activity (
			event_id,
			task_id,
			task_version,
			event_type,
			schema_version,
			occurred_at,
			payload
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		eventID,
		123,
		1,
		"task.created",
		1,
		occurredAt,
		`{"title":"Prepare report"}`,
	)

	return err
}

// assertUniqueViolation checks the expected state or result and reports failures through the test.
func assertUniqueViolation(t *testing.T, constraint string, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf(
			"insert error = nil, want PostgreSQL unique violation for constraint %q",
			constraint,
		)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf(
			"insert error = %T (%v), want *pgconn.PgError",
			err,
			err,
		)
	}

	if pgErr.Code != postgresUniqueViolationCode {
		t.Fatalf(
			"PostgreSQL error code = %q, want %q",
			pgErr.Code,
			postgresUniqueViolationCode,
		)
	}

	if pgErr.ConstraintName != constraint {
		t.Fatalf(
			"PostgreSQL constraint = %q, want %q",
			pgErr.ConstraintName,
			constraint,
		)
	}
}

// assertConnectedTestDatabase checks the expected state or result and reports failures through the test.
func assertConnectedTestDatabase(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	wantDatabase string,
) {
	t.Helper()

	var connectedDatabase string

	if err := pool.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&connectedDatabase); err != nil {
		t.Fatalf("read connected database name: %v", err)
	}

	if connectedDatabase != wantDatabase {
		t.Fatalf(
			"connected database = %q, want %q",
			connectedDatabase,
			wantDatabase,
		)
	}
}
