package testutil

import (
	"os"
	"testing"
)

// UnsetEnvironmentVariable temporarily unsets key and restores its original value during cleanup.
func UnsetEnvironmentVariable(t *testing.T, key string) {
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
