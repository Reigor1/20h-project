package core_logger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerContextKey struct{}

var key = loggerContextKey{}

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

type Logger struct {
	*zap.Logger
	fileState *loggerFileState

	file *os.File
}

func FromContext(ctx context.Context) *Logger {
	log, ok := ctx.Value(key).(*Logger)
	if !ok {
		panic("no logger in context")
	}

	return log
}

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

func (l *Logger) With(field ...zap.Field) *Logger {
	return &Logger{
		Logger:    l.Logger.With(field...),
		fileState: l.fileState,
		file:      l.file,
	}
}

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
