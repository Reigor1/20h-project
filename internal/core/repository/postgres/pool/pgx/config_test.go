package core_pgx_pool

import (
	"os"
	"testing"
	"time"
)

// TestNewConfigReadsEnvironment verifies that PostgreSQL settings are loaded from POSTGRES_* variables.
func TestNewConfigReadsEnvironment(t *testing.T) {
	setValidPostgresEnvironment(t)

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v, want nil", err)
	}

	if config.Host != "db.example.test" {
		t.Errorf("NewConfig() Host = %q, want %q", config.Host, "db.example.test")
	}
	if config.Port != "6543" {
		t.Errorf("NewConfig() Port = %q, want %q", config.Port, "6543")
	}
	if config.User != "test-user" {
		t.Errorf("NewConfig() User = %q, want %q", config.User, "test-user")
	}
	if config.Password != "test-password" {
		t.Errorf("NewConfig() Password = %q, want configured password", config.Password)
	}
	if config.Database != "test-database" {
		t.Errorf("NewConfig() Database = %q, want %q", config.Database, "test-database")
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("NewConfig() Timeout = %s, want %s", config.Timeout, 5*time.Second)
	}
}

// TestNewConfigUsesDefaultPortWhenPortIsNotSet checks the default PostgreSQL port.
func TestNewConfigUsesDefaultPortWhenPortIsNotSet(t *testing.T) {
	setValidPostgresEnvironment(t)
	unsetEnvironmentVariable(t, "POSTGRES_PORT")

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v, want nil", err)
	}
	if config.Port != "5432" {
		t.Errorf("NewConfig() Port = %q, want default %q", config.Port, "5432")
	}
}

// TestNewConfigRequiresEachMandatoryEnvironmentVariable verifies every required connection setting.
func TestNewConfigRequiresEachMandatoryEnvironmentVariable(t *testing.T) {
	mandatoryVariables := []string{
		"POSTGRES_HOST",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"POSTGRES_TIMEOUT",
	}

	for _, variable := range mandatoryVariables {
		t.Run(variable, func(t *testing.T) {
			setValidPostgresEnvironment(t)
			unsetEnvironmentVariable(t, variable)

			if _, err := NewConfig(); err == nil {
				t.Fatalf("NewConfig() error = nil, want error when %s is empty", variable)
			}
		})
	}
}

// TestNewConfigMustReturnsConfiguration checks the panic-on-error constructor's successful path.
func TestNewConfigMustReturnsConfiguration(t *testing.T) {
	setValidPostgresEnvironment(t)

	config := NewConfigMust()
	if config.Host != "db.example.test" || config.Database != "test-database" {
		t.Fatalf("NewConfigMust() = %+v, want configured host and database", config)
	}
}

// TestNewConfigMustPanicsWhenConfigurationIsInvalid verifies failure behavior for invalid settings.
func TestNewConfigMustPanicsWhenConfigurationIsInvalid(t *testing.T) {
	setValidPostgresEnvironment(t)
	t.Setenv("POSTGRES_TIMEOUT", "not-a-duration")

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("NewConfigMust() did not panic for invalid configuration")
		}
	}()

	NewConfigMust()
}

// setValidPostgresEnvironment configures a complete PostgreSQL environment for a test.
func setValidPostgresEnvironment(t *testing.T) {
	t.Helper()

	t.Setenv("POSTGRES_HOST", "db.example.test")
	t.Setenv("POSTGRES_PORT", "6543")
	t.Setenv("POSTGRES_USER", "test-user")
	t.Setenv("POSTGRES_PASSWORD", "test-password")
	t.Setenv("POSTGRES_DB", "test-database")
	t.Setenv("POSTGRES_TIMEOUT", "5s")
}

// unsetEnvironmentVariable removes an environment variable for the duration of a test.
func unsetEnvironmentVariable(t *testing.T, key string) {
	t.Helper()

	previousValue, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			if err := os.Setenv(key, previousValue); err != nil {
				t.Errorf("restore %s: %v", key, err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			t.Errorf("restore unset %s: %v", key, err)
		}
	})
}
