package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	core_config "github.com/Reigor1/20h-project/internal/core/config"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
	core_observability_postgres "github.com/Reigor1/20h-project/internal/core/observability/postgres"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
	statistics_postgres_repository "github.com/Reigor1/20h-project/internal/features/statistics/repository/postgres"
	statistics_service "github.com/Reigor1/20h-project/internal/features/statistics/service"
	statistics_transport_http "github.com/Reigor1/20h-project/internal/features/statistics/transport/http"
	tasks_postgres_repository "github.com/Reigor1/20h-project/internal/features/tasks/repository/postgres"
	tasks_service "github.com/Reigor1/20h-project/internal/features/tasks/service"
	tasks_transport_http "github.com/Reigor1/20h-project/internal/features/tasks/transport/http"
	users_postgres_repository "github.com/Reigor1/20h-project/internal/features/users/repository/postgres"
	users_service "github.com/Reigor1/20h-project/internal/features/users/service"
	users_transport_http "github.com/Reigor1/20h-project/internal/features/users/transport/http"
	"github.com/exaring/otelpgx"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	_ "github.com/Reigor1/20h-project/docs"
)

// @Title 		 Golang Todo API
// @Version 	 1.0
// @Description  Todo Application REST-API scheme
// @Description  `Перед работой помолитесь Богу машины`
// @Host 		 127.0.0.1:5050
// @BasePath 	 /api/v1
func main() {
	cfg := core_config.NewConfigMust()
	time.Local = cfg.TimeZone

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	logger, err := core_logger.NewLogger(core_logger.NewConfigMust())
	if err != nil {
		fmt.Println("failed to init application logger:", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Debug("application time zone", zap.Any("zone", time.Local))

	logger.Debug("initializing observability")

	observability, err := core_observability.New(
		ctx,
		core_observability.Config{
			ServiceName: "20h-project",
		},
	)
	if err != nil {
		logger.Fatal("failed to init observability", zap.Error(err))
	}

	// Observability owns the MeterProvider and its registrations.
	// Shutdown it before closing the pool so the final metric collection
	// can still read pgxpool.Stat().
	var pool *core_pgx_pool.Pool

	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer shutdownCancel()

		if err := observability.Shutdown(shutdownCtx); err != nil {
			logger.Error("failed to shutdown observability", zap.Error(err))
		}

		if pool != nil {
			pool.Close()
		}
	}()

	logger.Debug("initializing pgx tracer")

	pgxTracer := otelpgx.NewTracer(
		otelpgx.WithMeterProvider(
			observability.MeterProvider(),
		),
		otelpgx.WithMetricOperationName(
			otelpgx.SQLOperationName,
		),
		otelpgx.WithMeterAttributes(
			attribute.String(
				"db.client.connection.pool.name",
				"primary",
			),
		),
		otelpgx.WithTracerAttributes(
			attribute.String(
				"db.client.connection.pool.name",
				"primary",
			),
		),
		otelpgx.WithDisableSQLStatementInAttributes(),
		otelpgx.WithDisableConnectionDetailsInAttributes(),
	)

	logger.Debug("initializing postgres connection pool")

	pool, err = core_pgx_pool.NewPool(
		ctx,
		core_pgx_pool.NewConfigMust(),
		pgxTracer,
	)
	if err != nil {
		logger.Fatal(
			"failed to init postgres connection pool",
			zap.Error(err),
		)
	}

	logger.Debug("initializing postgres pool metrics")

	if err := core_observability_postgres.RegisterPoolMetrics(
		observability.MeterProvider(),
		pool,
	); err != nil {
		logger.Fatal(
			"failed to init postgres pool metrics",
			zap.Error(err),
		)
	}

	logger.Debug("initializing feature", zap.String("feature", "users"))
	usersRepository := users_postgres_repository.NewUsersRepository(pool)
	usersService := users_service.NewUsersService(usersRepository)
	usersTransportHTTP := users_transport_http.NewUsersHTTPHandler(usersService)

	logger.Debug("initializing feature", zap.String("feature", "tasks"))
	tasksRepository := tasks_postgres_repository.NewTasksRepository(pool)
	tasksService := tasks_service.NewTasksService(tasksRepository)
	tasksTransportHTTP := tasks_transport_http.NewTasksHTTPHandler(tasksService)

	logger.Debug("initializing feature", zap.String("feature", "statistics"))
	statisticsRepository := statistics_postgres_repository.NewStatisticsRepository(pool)
	statisticsService := statistics_service.NewStatisticsService(statisticsRepository)
	statisticsTransportHTTP := statistics_transport_http.NewStatisticsHTTPHandler(statisticsService)

	logger.Debug("initializing HTTP server")
	httpServer := core_http_server.NewHTTPServer(
		core_http_server.NewConfigMust(),
		logger,
		core_http_middleware.CORS(),
		core_http_middleware.RequestID(),
		core_http_middleware.Logger(logger),
		core_http_middleware.Trace(),
		core_http_middleware.Panic(),
	)
	httpServer.RegisterAPIRouteInstrumenter(observability.InstrumentHTTPRoute)

	apiVersionRouter1 := core_http_server.NewAPIVersionRouter(
		core_http_server.ApiVersion1,
	)
	apiVersionRouter1.RegisterRouts(usersTransportHTTP.Routes()...)
	apiVersionRouter1.RegisterRouts(tasksTransportHTTP.Routes()...)
	apiVersionRouter1.RegisterRouts(statisticsTransportHTTP.Routes()...)

	// Example of usage apiVersionRouter2 with dummy middleware
	// apiVersionRouter2 := core_http_server.NewAPIVersionRouter(
	// 	core_http_server.ApiVersion2,
	// 	core_http_middleware.Dummy("api v2 middleware"),
	// )
	// apiVersionRouter2.RegisterRouts(usersTransportHTTP.Routes()...)

	httpServer.RegisterAPIRouters(
		apiVersionRouter1,
		// apiVersionRouter2,
	)
	httpServer.RegisterSwagger()

	logger.Info(
		"observability dashboards",
		zap.String("prometheus_url", "http://localhost:9090/graph"),
		zap.String("grafana_url", "http://localhost:3000"),
		zap.String("swagger_url", "http://127.0.0.1:5050/swagger/index.html"),
	)

	if err := httpServer.Run(ctx); err != nil {
		logger.Error("HTTP server run error", zap.Error(err))
	}
}
