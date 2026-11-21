package core_logger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerContextKey struct{}

var key = loggerContextKey{}

// ToContext stores the Logger in a context so downstream code can retrieve it.
func ToContext(ctx context.Context, log *Logger) context.Context {
	return context.WithValue(
		ctx,
		key,
		log,
	)
}

type loggerFileState struct {
	once sync.Once
	err  error
}

// TODO: add Logger interface :)

// Logger wraps zap.Logger and shares file lifecycle state between derived loggers.
type Logger struct {
	*zap.Logger
	fileState *loggerFileState

	file *os.File
}

// FromContext retrieves the Logger from context and enriches it with trace and span IDs
// when the context contains a valid OpenTelemetry span.
func FromContext(ctx context.Context) *Logger {
	log, ok := ctx.Value(key).(*Logger)
	if !ok {
		panic("no logger in context")
	}

	spanContext := trace.SpanFromContext(ctx).SpanContext()
	if !spanContext.IsValid() {
		return log
	}

	return log.With(
		zap.String("trace_id", spanContext.TraceID().String()),
		zap.String("span_id", spanContext.SpanID().String()),
	)
}

// NewLogger creates a zap logger that writes console-encoded entries to stdout and a timestamped file.
func NewLogger(config Config) (*Logger, error) {
	zapLvl := zap.NewAtomicLevel()
	if err := zapLvl.UnmarshalText([]byte(config.Level)); err != nil {
		return nil, fmt.Errorf("unmarshal log level: %w", err)
	}

	if err := os.MkdirAll(config.Folder, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log folder: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02T15-04-05.000000")
	logFilePath := filepath.Join(
		config.Folder,
		fmt.Sprintf("%s.log", timestamp),
	)

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	zapConfig := zap.NewDevelopmentEncoderConfig()
	zapConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02T15:04:05.000000")

	zapEncoder := zapcore.NewConsoleEncoder(zapConfig)

	core := zapcore.NewTee(
		zapcore.NewCore(zapEncoder, zapcore.AddSync(os.Stdout), zapLvl),
		zapcore.NewCore(zapEncoder, zapcore.AddSync(logFile), zapLvl),
	)

	zapLogger := zap.New(core, zap.AddCaller())

	return &Logger{
		Logger:    zapLogger,
		fileState: &loggerFileState{},
		file:      logFile,
	}, nil
}

// With returns a logger enriched with fields while sharing the original output file lifecycle.
func (l *Logger) With(field ...zap.Field) *Logger {
	return &Logger{
		Logger:    l.Logger.With(field...),
		fileState: l.fileState,
		file:      l.file,
	}
}

// Close syncs and closes the log file once, even when called through derived loggers.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	if l.fileState == nil {
		var closeErr error

		if err := l.file.Sync(); err != nil {
			closeErr = errors.Join(
				closeErr,
				fmt.Errorf("sync application log file: %w", err),
			)
		}

		if err := l.file.Close(); err != nil {
			closeErr = errors.Join(
				closeErr,
				fmt.Errorf("close application log file: %w", err),
			)
		}

		return closeErr
	}

	l.fileState.once.Do(func() {
		if err := l.file.Sync(); err != nil {
			l.fileState.err = errors.Join(
				l.fileState.err,
				fmt.Errorf("sync application log file: %w", err),
			)
		}

		if err := l.file.Close(); err != nil {
			l.fileState.err = errors.Join(
				l.fileState.err,
				fmt.Errorf("close application log file: %w", err),
			)
		}
	})

	return l.fileState.err
}
