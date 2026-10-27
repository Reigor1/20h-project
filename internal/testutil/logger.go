package testutil

import (
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// NewNopLogger creates a logger that discards output during tests.
func NewNopLogger() *core_logger.Logger {
	return &core_logger.Logger{Logger: zap.NewNop()}
}

// NewObservedLogger creates a debug-level logger and its in-memory log observer.
func NewObservedLogger() (*core_logger.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return &core_logger.Logger{Logger: zap.New(core)}, logs
}
