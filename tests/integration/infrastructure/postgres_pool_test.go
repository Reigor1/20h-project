package infrastructure_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	"github.com/Reigor1/20h-project/internal/testutil"
)

// Integration tests use only the separately configured POSTGRES_TEST_* settings.
// Set POSTGRES_TEST_HOST, POSTGRES_TEST_USER, POSTGRES_TEST_PASSWORD, and
// POSTGRES_TEST_DB to enable them. POSTGRES_TEST_PORT is optional and defaults
// to 5432. The target database must be dedicated to tests.
func TestPostgresPoolIntegration(t *testing.T) {
	config := testutil.PostgresTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := core_pgx_pool.NewPool(ctx, config, nil)
	if err != nil {
		t.Fatalf("NewPool() error = %v, want successful connection", err)
	}
	poolClosed := false
	defer func() {
		if !poolClosed {
			pool.Close()
		}
	}()

	const expectedOperationTimeout = 5 * time.Second
	if got := pool.OpTimeout(); got != expectedOperationTimeout {
		t.Errorf("OpTimeout() = %s, want %s", got, expectedOperationTimeout)
	}

	schemaName := fmt.Sprintf("pool_it_%d_%d", os.Getpid(), time.Now().UnixNano())
	createSchemaQuery := "CREATE SCHEMA " + schemaName
	if _, err := pool.Exec(ctx, createSchemaQuery); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	schemaCreated := true
	defer func() {
		if schemaCreated {
			dropTestSchema(t, pool, schemaName)
		}
	}()

	parentTable := schemaName + ".parents"
	childTable := schemaName + ".children"
	createParentTableQuery := "CREATE TABLE " + parentTable + " (id integer PRIMARY KEY)"
	if _, err := pool.Exec(ctx, createParentTableQuery); err != nil {
		t.Fatalf("create parent table: %v", err)
	}
	createChildTableQuery := "CREATE TABLE " + childTable +
		" (parent_id integer REFERENCES " + parentTable + " (id))"
	if _, err := pool.Exec(ctx, createChildTableQuery); err != nil {
		t.Fatalf("create child table: %v", err)
	}

	commandTag, err := pool.Exec(ctx, "INSERT INTO "+parentTable+" (id) VALUES (42)")
	if err != nil {
		t.Fatalf("insert parent row: %v", err)
	}
	const expectedInsertedRows = int64(1)
	if got := commandTag.RowsAffected(); got != expectedInsertedRows {
		t.Fatalf("RowsAffected() = %d, want %d", got, expectedInsertedRows)
	}

	insertInvalidChildQuery := "INSERT INTO " + childTable + " (parent_id) VALUES (999)"
	if _, err := pool.Exec(ctx, insertInvalidChildQuery); err == nil {
		t.Fatal("Exec() foreign key violation error = nil, want an error")
	}

	rows, err := pool.Query(ctx, "SELECT generate_series(1, 3)")
	if err != nil {
		t.Fatalf("Query() error = %v, want nil", err)
	}

	var values []int
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			t.Fatalf("scan Query() row: %v", err)
		}
		values = append(values, value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("Query() rows error = %v, want nil", err)
	}
	if len(values) != 3 || values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Fatalf("Query() values = %v, want [1 2 3]", values)
	}

	var parentID int
	existingParentQuery := "SELECT id FROM " + parentTable + " WHERE id = $1"
	if err := pool.QueryRow(ctx, existingParentQuery, 42).Scan(&parentID); err != nil {
		t.Fatalf("QueryRow() existing row error = %v, want nil", err)
	}
	if parentID != 42 {
		t.Fatalf("QueryRow() id = %d, want 42", parentID)
	}

	missingParentQuery := "SELECT id FROM " + parentTable + " WHERE id = $1"
	err = pool.QueryRow(ctx, missingParentQuery, 404).Scan(&parentID)
	if !errors.Is(err, core_postgres_pool.ErrNoRows) {
		t.Fatalf("QueryRow() missing row error = %v, want ErrNoRows", err)
	}

	dropTestSchema(t, pool, schemaName)
	schemaCreated = false
	pool.Close()
	poolClosed = true
	if _, err := pool.Exec(ctx, "SELECT 1"); err == nil {
		t.Fatal("Exec() after Close() error = nil, want closed-pool error")
	}
}

// TestNewPoolRejectsInvalidConfiguration verifies the integration behavior represented by its test scenarios.
func TestNewPoolRejectsInvalidConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := core_pgx_pool.NewPool(ctx, core_pgx_pool.Config{
		Host:     "localhost",
		Port:     "not-a-port",
		User:     "test-user",
		Password: "test-password",
		Database: "test-database",
		Timeout:  time.Second,
	}, nil)
	if err == nil {
		t.Fatal("NewPool() error = nil, want invalid connection configuration error")
	}
}

// dropTestSchema removes test data or schema created by the integration scenario.
func dropTestSchema(t *testing.T, pool *core_pgx_pool.Pool, schemaName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dropSchemaQuery := "DROP SCHEMA " + schemaName + " CASCADE"
	if _, err := pool.Exec(ctx, dropSchemaQuery); err != nil {
		t.Errorf("drop isolated test schema: %v", err)
	}
}
