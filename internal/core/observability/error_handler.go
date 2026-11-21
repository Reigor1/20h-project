package core_observability

import (
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// ConfigureErrorHandler routes OpenTelemetry SDK diagnostics
// through the application logger.
// Call it once, before creating SDK providers.
// ConfigureErrorHandler installs an OpenTelemetry SDK error handler that reports
// asynchronous exporter and instrumentation errors through the application logger.
func ConfigureErrorHandler(logger *core_logger.Logger) {
	if logger == nil {
		return
	}

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		if err == nil {
			return
		}

		logger.Error("OpenTelemetry SDK error", zap.Error(err))
	}))
}
