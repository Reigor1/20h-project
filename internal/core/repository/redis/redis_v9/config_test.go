package core_redis_v9

import (
	"testing"
	"time"
)

// TestNewConfigReadsEnvironment verifies that REDIS_* variables populate Redis configuration.
func TestNewConfigReadsEnvironment(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis.example.test:6380")
	t.Setenv("REDIS_USERNAME", "test-user")
	t.Setenv("REDIS_PASSWORD", "test-password")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("REDIS_TIMEOUT", "3s")

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v, want nil", err)
	}

	if config.Addr != "redis.example.test:6380" {
		t.Errorf("NewConfig() Addr = %q, want %q", config.Addr, "redis.example.test:6380")
	}
	if config.Username != "test-user" {
		t.Errorf("NewConfig() Username = %q, want %q", config.Username, "test-user")
	}
	if config.Password != "test-password" {
		t.Errorf("NewConfig() Password = %q, want configured password", config.Password)
	}
	if config.DB != 2 {
		t.Errorf("NewConfig() DB = %d, want 2", config.DB)
	}
	if config.Timeout != 3*time.Second {
		t.Errorf("NewConfig() Timeout = %s, want %s", config.Timeout, 3*time.Second)
	}
}

// TestNewConfigRejectsInvalidValues checks validation of empty addresses, negative DB indexes,
// and non-positive operation timeouts.
func TestNewConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		db      string
		timeout string
	}{
		{
			name:    "empty address",
			addr:    " ",
			db:      "0",
			timeout: "5s",
		},
		{
			name:    "negative database number",
			addr:    "localhost:6379",
			db:      "-1",
			timeout: "5s",
		},
		{
			name:    "zero timeout",
			addr:    "localhost:6379",
			db:      "0",
			timeout: "0s",
		},
		{
			name:    "invalid timeout",
			addr:    "localhost:6379",
			db:      "0",
			timeout: "not-a-duration",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("REDIS_ADDR", testCase.addr)
			t.Setenv("REDIS_DB", testCase.db)
			t.Setenv("REDIS_TIMEOUT", testCase.timeout)

			if _, err := NewConfig(); err == nil {
				t.Fatal("NewConfig() error = nil, want validation error")
			}
		})
	}
}
