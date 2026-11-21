package testutil

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// StartApp starts the application and returns its base URL.
// StartApp builds and starts the API using .env.test, waits for its HTTP listener,
// and registers process cleanup with the test. It returns the application base URL.
func StartApp(t *testing.T) string {
	t.Helper()

	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("find project root: %v", err)
	}

	// Read test configuration without changing the test process environment.
	testEnv, err := godotenv.Read(filepath.Join(root, ".env.test"))
	if err != nil {
		t.Fatalf("read .env.test: %v", err)
	}

	get := func(key string) string {
		value, ok := testEnv[key]
		if !ok || value == "" {
			t.Fatalf("missing %s in .env.test", key)
		}

		return value
	}

	httpAddr := get("HTTP_ADDR")

	probeAddr, err := httpProbeAddress(httpAddr)
	if err != nil {
		t.Fatalf("prepare HTTP probe address: %v", err)
	}

	// Build the application binary.
	binaryName := "todoapp-e2e"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(t.TempDir(), binaryName)

	build := exec.Command(
		"go",
		"build",
		"-o",
		binaryPath,
		"./cmd/todoapp",
	)
	build.Dir = root

	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build app: %v\n%s", err, output)
	}

	// Configure the application process.
	cmd := exec.Command(binaryPath)
	cmd.Dir = root

	// envconfig.Process("POSTGRES", ...) expects POSTGRES_*,
	// while .env.test uses POSTGRES_TEST_*.
	cmd.Env = append(os.Environ(),
		"HTTP_ADDR="+httpAddr,
		"HTTP_SHUTDOWN_TIMEOUT="+get("HTTP_SHUTDOWN_TIMEOUT"),

		"POSTGRES_HOST="+get("POSTGRES_TEST_HOST"),
		"POSTGRES_PORT="+get("POSTGRES_TEST_PORT"),
		"POSTGRES_USER="+get("POSTGRES_TEST_USER"),
		"POSTGRES_PASSWORD="+get("POSTGRES_TEST_PASSWORD"),
		"POSTGRES_DB="+get("POSTGRES_TEST_DB"),
		"POSTGRES_TIMEOUT="+get("POSTGRES_TIMEOUT"),

		"LOGGER_LEVEL="+get("LOGGER_LEVEL"),
		"TIME_ZONE="+get("TIME_ZONE"),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start app: %v", err)
	}

	// Stop the application after the test finishes.
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Wait until the HTTP server accepts TCP connections.
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout(
			"tcp",
			probeAddr,
			200*time.Millisecond,
		)
		if err == nil {
			_ = conn.Close()
			return "http://" + probeAddr
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("application did not start listening on %s", probeAddr)
	return ""
}

// findProjectRoot searches upward for the project's go.mod file.
// findProjectRoot walks upward from the working directory until it finds go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))

		if err == nil {
			return dir, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("check go.mod in %q: %w", dir, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(
				"go.mod not found in %q or its parent directories",
				dir,
			)
		}

		dir = parent
	}
}

// httpProbeAddress converts a server listen address into a loopback TCP probe address.
// Wildcard hosts are replaced with loopback addresses so tests can check readiness locally.
func httpProbeAddress(httpAddr string) (string, error) {
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return "", fmt.Errorf("parse HTTP_ADDR %q: %w", httpAddr, err)
	}

	if port == "" || port == "0" {
		return "", fmt.Errorf(
			"HTTP_ADDR must specify a known, non-zero port: %q",
			httpAddr,
		)
	}

	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}

	return net.JoinHostPort(host, port), nil
}
