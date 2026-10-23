package core_logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNewLoggerCreatesLogFileAndWritesMessage(t *testing.T) {
	logFolder := filepath.Join(t.TempDir(), "nested", "logs")
	logger, err := NewLogger(Config{
		Level:  "debug",
		Folder: logFolder,
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v, want nil", err)
	}
	t.Cleanup(logger.Close)

	logger.Info("test log message")
	if err := logger.Sync(); err != nil {
		t.Fatalf("sync log file: %v", err)
	}

	entries, err := os.ReadDir(logFolder)
	if err != nil {
		t.Fatalf("read log folder: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log folder contains %d entries, want 1", len(entries))
	}
	if filepath.Ext(entries[0].Name()) != ".log" {
		t.Fatalf("log file name = %q, want .log extension", entries[0].Name())
	}

	logPath := filepath.Join(logFolder, entries[0].Name())
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(contents), "test log message") {
		t.Fatalf("log file contents = %q, want test message", contents)
	}
}

func TestNewLoggerRejectsInvalidLevel(t *testing.T) {
	logger, err := NewLogger(Config{
		Level:  "not-a-level",
		Folder: t.TempDir(),
	})
	if err == nil {
		if logger != nil {
			logger.Close()
		}
		t.Fatal("NewLogger() error = nil, want invalid-level error")
	}
	if logger != nil {
		t.Fatalf("NewLogger() logger = %v, want nil when configuration is invalid", logger)
	}
}

func TestToContextAndFromContextPreserveLoggerIdentity(t *testing.T) {
	logger := &Logger{}
	ctx := ToContext(context.Background(), logger)

	if got := FromContext(ctx); got != logger {
		t.Fatalf("FromContext() = %p, want original logger %p", got, logger)
	}
}

func TestFromContextPanicsWhenLoggerIsMissing(t *testing.T) {
	defer func() {
		got := recover()
		if got != "no logger in context" {
			t.Fatalf("FromContext() panic = %v, want %q", got, "no logger in context")
		}
	}()

	FromContext(context.Background())
}

func TestWithWritesMessageAndAdditionalFields(t *testing.T) {
	logFolder := filepath.Join(t.TempDir(), "logs")
	logger, err := NewLogger(Config{
		Level:  "debug",
		Folder: logFolder,
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v, want nil", err)
	}
	t.Cleanup(logger.Close)

	derivedLogger := logger.With(zap.String("request_id", "request-123"))
	derivedLogger.Info("message from derived logger")
	if err := logger.file.Sync(); err != nil {
		t.Fatalf("sync log file: %v", err)
	}

	entries, err := os.ReadDir(logFolder)
	if err != nil {
		t.Fatalf("read log folder: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log folder contains %d entries, want 1", len(entries))
	}

	contents, err := os.ReadFile(filepath.Join(logFolder, entries[0].Name()))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	logOutput := string(contents)
	for _, expected := range []string{"message from derived logger", "request-123"} {
		if !strings.Contains(logOutput, expected) {
			t.Errorf("log file contents = %q, want it to contain %q", logOutput, expected)
		}
	}
}

func TestCloseClosesLogFile(t *testing.T) {
	logger, err := NewLogger(Config{
		Level:  "debug",
		Folder: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v, want nil", err)
	}

	logger.Close()

	if _, err := logger.file.Write([]byte("after close")); err == nil {
		t.Fatal("writing to the log file after Close() succeeded, want a closed-file error")
	} else if !errors.Is(err, os.ErrClosed) && !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("writing to the log file after Close() error = %v, want a closed-file error", err)
	}
}
