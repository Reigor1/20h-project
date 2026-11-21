package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
)

// PreparePostgresSchema creates the todoapp schema and executes a test schema SQL file.
// PreparePostgresSchema ensures the application schema exists and executes the supplied
// SQL file against the test database.
func PreparePostgresSchema(
	t *testing.T,
	ctx context.Context,
	pool core_postgres_pool.Pool,
	schemaPath string,
) {
	t.Helper()

	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS todoapp"); err != nil {
		t.Fatalf("prepare todoapp schema in test database: %v", err)
	}

	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read PostgreSQL test schema %q: %v", schemaPath, err)
	}

	if _, err := pool.Exec(ctx, string(schemaSQL)); err != nil {
		t.Fatalf("execute PostgreSQL test schema %q: %v", schemaPath, err)
	}
}

// PostgresTestConfig reads the isolated POSTGRES_TEST_* settings used by integration tests.
// PostgresTestConfig loads dedicated integration-test credentials, skips when none are
// configured, and rejects using the same database as the application.
func PostgresTestConfig(t *testing.T) core_pgx_pool.Config {
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
	applicationDatabase, applicationDatabaseSet := os.LookupEnv("POSTGRES_DB")
	if applicationDatabaseSet && applicationDatabase == values[databaseKey] {
		t.Fatalf(
			"%s must identify a database different from POSTGRES_DB",
			databaseKey,
		)
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
