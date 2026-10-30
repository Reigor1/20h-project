package core_http_server

import (
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/testutil"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name            string
		address         string
		shutdownTimeout string
		unsetVariable   string
		wantAddress     string
		wantTimeout     time.Duration
		wantError       bool
	}{
		{
			name:          "configured address and default shutdown timeout",
			address:       "127.0.0.1:5050",
			unsetVariable: "HTTP_SHUTDOWN_TIMEOUT",
			wantAddress:   "127.0.0.1:5050",
			wantTimeout:   30 * time.Second,
		},
		{
			name:            "configured shutdown timeout",
			address:         "127.0.0.1:5050",
			shutdownTimeout: "7s",
			wantAddress:     "127.0.0.1:5050",
			wantTimeout:     7 * time.Second,
		},
		{
			name:            "missing address",
			shutdownTimeout: "30s",
			unsetVariable:   "HTTP_ADDR",
			wantError:       true,
		},
		{
			name:            "invalid shutdown timeout",
			address:         "127.0.0.1:5050",
			shutdownTimeout: "soon",
			wantError:       true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", testCase.address)
			t.Setenv("HTTP_SHUTDOWN_TIMEOUT", testCase.shutdownTimeout)
			if testCase.unsetVariable != "" {
				testutil.UnsetEnvironmentVariable(t, testCase.unsetVariable)
			}

			config, err := NewConfig()
			if (err != nil) != testCase.wantError {
				t.Fatalf("NewConfig() error = %v, wantError %v", err, testCase.wantError)
			}
			if testCase.wantError {
				return
			}
			if config.Addr != testCase.wantAddress {
				t.Errorf("NewConfig() Addr = %q, want %q", config.Addr, testCase.wantAddress)
			}
			if config.ShutdownTimeout != testCase.wantTimeout {
				t.Errorf(
					"NewConfig() ShutdownTimeout = %s, want %s",
					config.ShutdownTimeout,
					testCase.wantTimeout,
				)
			}
		})
	}
}

func TestNewConfigMust(t *testing.T) {
	t.Run("returns valid configuration", func(t *testing.T) {
		t.Setenv("HTTP_ADDR", "127.0.0.1:5050")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "4s")

		config := NewConfigMust()
		if config.Addr != "127.0.0.1:5050" || config.ShutdownTimeout != 4*time.Second {
			t.Fatalf("NewConfigMust() = %+v, want configured address and 4s timeout", config)
		}
	})

	t.Run("panics on invalid configuration", func(t *testing.T) {
		t.Setenv("HTTP_ADDR", "")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "30s")
		testutil.UnsetEnvironmentVariable(t, "HTTP_ADDR")

		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("NewConfigMust() did not panic for invalid configuration")
			}
		}()

		NewConfigMust()
	})
}
