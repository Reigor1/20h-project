package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
)

// Integration tests use only the separately configured POSTGRES_TEST_* settings.
// Set POSTGRES_TEST_HOST, POSTGRES_TEST_USER, POSTGRES_TEST_PASSWORD, and
// POSTGRES_TEST_DB to enable them. POSTGRES_TEST_PORT is optional and defaults
// to 5432. The target database must be dedicated to tests.
func TestPostgresPoolIntegration(t *testing.T) {
	config := postgresTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := core_pgx_pool.NewPool(ctx, config)
	if err != nil {
		t.Fatalf("NewPool() error = %v, want successful connection", err)
	}
	poolClosed := false
	defer func() {
		if !poolClosed {
			pool.Close()
		}
	}()

	if got, want := pool.OpTimeout(), 5*time.Second; got != want {
		t.Errorf("OpTimeout() = %s, want %s", got, want)
	}

	schemaName := fmt.Sprintf("pool_it_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
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
	if _, err := pool.Exec(ctx, "CREATE TABLE "+parentTable+" (id integer PRIMARY KEY)"); err != nil {
		t.Fatalf("create parent table: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE "+childTable+" (parent_id integer REFERENCES "+parentTable+" (id))"); err != nil {
		t.Fatalf("create child table: %v", err)
	}

	commandTag, err := pool.Exec(ctx, "INSERT INTO "+parentTable+" (id) VALUES (42)")
	if err != nil {
		t.Fatalf("insert parent row: %v", err)
	}
	if got, want := commandTag.RowsAffected(), int64(1); got != want {
		t.Fatalf("RowsAffected() = %d, want %d", got, want)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO "+childTable+" (parent_id) VALUES (999)"); err == nil {
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
	if err := pool.QueryRow(ctx, "SELECT id FROM "+parentTable+" WHERE id = $1", 42).Scan(&parentID); err != nil {
		t.Fatalf("QueryRow() existing row error = %v, want nil", err)
	}
	if parentID != 42 {
		t.Fatalf("QueryRow() id = %d, want 42", parentID)
	}

	if err := pool.QueryRow(ctx, "SELECT id FROM "+parentTable+" WHERE id = $1", 404).Scan(&parentID); !errors.Is(err, core_postgres_pool.ErrNoRows) {
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
	})
	if err == nil {
		t.Fatal("NewPool() error = nil, want invalid connection configuration error")
	}
}

func postgresTestConfig(t *testing.T) core_pgx_pool.Config {
	t.Helper()

	const (
		hostKey     = "POSTGRES_TEST_HOST"
		portKey     = "POSTGRES_TEST_PORT"
		userKey     = "POSTGRES_TEST_USER"
		passwordKey = "POSTGRES_TEST_PASSWORD"
		databaseKey = "POSTGRES_TEST_DB"
	)

	keys := []string{hostKey, userKey, passwordKey, databaseKey}
	values := make(map[string]string, len(keys)+1)
	configured := false
	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		if exists && value != "" {
			configured = true
		}
		values[key] = value
	}

	if !configured {
		t.Skip("PostgreSQL integration tests require a dedicated database configured with POSTGRES_TEST_* variables")
	}

	for _, key := range keys {
		if values[key] == "" {
			t.Fatalf("%s must be set when PostgreSQL integration tests are enabled", key)
		}
	}
	if applicationDatabase, exists := os.LookupEnv("POSTGRES_DB"); exists && applicationDatabase == values[databaseKey] {
		t.Fatalf("%s must identify a database different from POSTGRES_DB", databaseKey)
	}

	port, exists := os.LookupEnv(portKey)
	if !exists || port == "" {
		port = "5432"
	}

	return core_pgx_pool.Config{
		Host:     values[hostKey],
		Port:     port,
		User:     values[userKey],
		Password: values[passwordKey],
		Database: values[databaseKey],
		Timeout:  5 * time.Second,
	}
}

func dropTestSchema(t *testing.T, pool *core_pgx_pool.Pool, schemaName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
		t.Errorf("drop isolated test schema: %v", err)
	}
}
