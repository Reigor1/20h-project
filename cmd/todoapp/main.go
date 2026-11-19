package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	core_config "github.com/Reigor1/20h-project/internal/core/config"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
	core_observability_postgres "github.com/Reigor1/20h-project/internal/core/observability/postgres"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"
	core_redis_v9 "github.com/Reigor1/20h-project/internal/core/repository/redis/redis_v9"
	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
	activity_postgres_repository "github.com/Reigor1/20h-project/internal/features/activity/repository/postgres"
	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
	activity_transport_http "github.com/Reigor1/20h-project/internal/features/activity/transport/http"
	outbox_publisher "github.com/Reigor1/20h-project/internal/features/outbox/publisher"
	outbox_postgres_repository "github.com/Reigor1/20h-project/internal/features/outbox/repository"
	statistics_redis_cache "github.com/Reigor1/20h-project/internal/features/statistics/cache/redis"
	statistics_postgres_repository "github.com/Reigor1/20h-project/internal/features/statistics/repository/postgres"
	statistics_service "github.com/Reigor1/20h-project/internal/features/statistics/service"
	statistics_transport_http "github.com/Reigor1/20h-project/internal/features/statistics/transport/http"
	tasks_postgres_repository "github.com/Reigor1/20h-project/internal/features/tasks/repository/postgres"
	tasks_service "github.com/Reigor1/20h-project/internal/features/tasks/service"
	tasks_transport_http "github.com/Reigor1/20h-project/internal/features/tasks/transport/http"
	users_postgres_repository "github.com/Reigor1/20h-project/internal/features/users/repository/postgres"
	users_service "github.com/Reigor1/20h-project/internal/features/users/service"
	users_transport_http "github.com/Reigor1/20h-project/internal/features/users/transport/http"

	"go.uber.org/zap"

	_ "github.com/Reigor1/20h-project/docs"
)

// @Title        Golang Todo API
// @Version      1.0
// @Description  Todo Application REST-API scheme
// @Description  `Перед работой помолитесь Богу машины`
// @Host         127.0.0.1:5050
// @BasePath     /api/v1
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "application failed:", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	cfg := core_config.NewConfigMust()
	time.Local = cfg.TimeZone

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	logger, err := core_logger.NewLogger(core_logger.NewConfigMust())
	if err != nil {
		return fmt.Errorf("init application logger: %w", err)
	}

	defer func() {
		if err := logger.Close(); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("close application logger: %w", err),
			)
		}
	}()

	logger.Debug("application time zone", zap.Any("zone", time.Local))

	core_observability.ConfigureErrorHandler(logger)

	logger.Debug("initializing observability")

	observability, err := core_observability.New(
		ctx,
		core_observability.Config{
			ServiceName:           "20h-project",
			ServiceVersion:        os.Getenv("SERVICE_VERSION"),
			DeploymentEnvironment: os.Getenv("DEPLOYMENT_ENVIRONMENT"),
		},
	)
	if err != nil {
		return fmt.Errorf("init observability: %w", err)
	}

	// Observability owns the MeterProvider and its registrations.
	// Shutdown it before closing the pool so the final metric collection
	// can still read pgxpool.Stat().
	var (
		pool          *core_pgx_pool.Pool
		cache         core_redis.Cache
		kafkaProducer *kafka_producer.Producer
		publisherDone   chan error
		publisherCancel context.CancelFunc
		publisherConfig outbox_publisher.Config
	)

	defer func() {
		// Stop application work before shutting down its dependencies.
		cancel()

		if publisherCancel != nil {
			publisherCancel()
		}

		if publisherDone != nil {
			select {
			case err := <-publisherDone:
				if err != nil {
					logger.Error(
						"Outbox Publisher stopped with error",
						zap.Error(err),
					)
				}
			case <-time.After(publisherConfig.ShutdownTimeout + time.Second):
				logger.Error("Outbox Publisher shutdown timed out; active lease will expire")
				runErr = errors.Join(runErr, fmt.Errorf("Outbox Publisher shutdown timed out"))
			}
		}

		if kafkaProducer != nil {
			if err := kafkaProducer.Close(10 * time.Second); err != nil {
				logger.Warn(
					"Kafka Producer shutdown failed",
					zap.Error(err),
				)

				runErr = errors.Join(
					runErr,
					fmt.Errorf("close Kafka Producer: %w", err),
				)
			}
		}

		if cache != nil {
			if err := cache.Close(); err != nil {
				logger.Warn("Redis shutdown failed", zap.Error(err))

				runErr = errors.Join(
					runErr,
					fmt.Errorf("close Redis cache: %w", err),
				)
			}
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer shutdownCancel()

		if err := observability.Shutdown(shutdownCtx); err != nil {
			logger.Error("OpenTelemetry shutdown failed", zap.Error(err))

			runErr = errors.Join(
				runErr,
				fmt.Errorf("shutdown observability: %w", err),
			)
		}

		// pgxpool.Close() waits until all acquired connections are returned.
		if pool != nil {
			pool.Close()
		}
	}()

	logger.Debug("initializing pgx tracer")

	pgxTracer := observability.NewPostgresTracer()

	logger.Debug("initializing postgres connection pool")

	pool, err = core_pgx_pool.NewPool(
		ctx,
		core_pgx_pool.NewConfigMust(),
		pgxTracer,
	)
	if err != nil {
		return fmt.Errorf("init PostgreSQL connection pool: %w", err)
	}

	logger.Debug("initializing postgres pool metrics")

	if err := core_observability_postgres.RegisterPoolMetrics(
		observability.MeterProvider(),
		pool,
	); err != nil {
		return fmt.Errorf("register PostgreSQL pool metrics: %w", err)
	}

	logger.Debug("initializing Redis cache")

	redisConfig, err := core_redis_v9.NewConfig()
	if err != nil {
		logger.Warn(
			"Redis configuration is invalid; cache is disabled",
			zap.Error(err),
		)
	} else {
		if err := observability.InstrumentRedisMetrics(); err != nil {
			logger.Warn(
				"Redis metrics instrumentation failed",
				zap.Error(err),
			)
		}

		redisClient, err := core_redis_v9.NewRedis(
			ctx,
			redisConfig,
			observability.TracerProvider(),
		)
		if err != nil {
			logger.Warn(
				"Redis is unavailable; continuing without cache",
				zap.Error(err),
			)
		} else {
			cache = redisClient
			logger.Info("Redis cache initialized")
		}
	}

	logger.Debug("initializing feature", zap.String("feature", "users"))

	usersRepository := users_postgres_repository.NewUsersRepository(pool)
	usersService := users_service.NewUsersService(
		usersRepository,
		observability.TracerProvider(),
	)
	usersTransportHTTP := users_transport_http.NewUsersHTTPHandler(usersService)

	logger.Debug("initializing feature", zap.String("feature", "tasks"))

	kafkaConfig, err := kafka_producer.NewConfig()
	if err != nil {
		return fmt.Errorf("load Kafka Producer config: %w", err)
	}

	publisherConfig, err = outbox_publisher.NewConfig()
	if err != nil {
		return fmt.Errorf("load Outbox Publisher config: %w", err)
	}

	// Lease must outlive the maximum Kafka delivery timeout
	// plus bounded PostgreSQL operations and a safety margin.
	minimumLease := kafkaConfig.MessageTimeout +
		pool.OpTimeout() +
		15*time.Second

	if publisherConfig.LeaseDuration <= minimumLease {
		return fmt.Errorf(
			"Outbox lease duration %s must exceed %s",
			publisherConfig.LeaseDuration,
			minimumLease,
		)
	}

	kafkaProducer, err = kafka_producer.NewProducer(
		kafkaConfig,
		func(kafkaError error) {
			logger.Warn(
				"Kafka Producer event",
				zap.Error(kafkaError),
			)
		},
	)
	if err != nil {
		return fmt.Errorf("initialize Kafka Producer: %w", err)
	}

	outboxRepository := outbox_postgres_repository.NewOutboxRepository(pool)

	tasksRepository := tasks_postgres_repository.NewTasksRepository(
		pool,
		outboxRepository,
	)
	tasksService := tasks_service.NewTasksService(
		tasksRepository,
		observability.TracerProvider(),
	)
	tasksTransportHTTP := tasks_transport_http.NewTasksHTTPHandler(tasksService)

	logger.Debug("initializing feature", zap.String("feature", "activity"))

	activityRepository := activity_postgres_repository.NewActivityRepository(pool)

	activityService := activity_service.NewActivityService(
		activityRepository,
	)

	activityTransportHTTP := activity_transport_http.NewActivityHTTPHandler(
		activityService,
	)

	publisher, err := outbox_publisher.NewPublisher(
		publisherConfig,
		outboxRepository,
		kafkaProducer,
		logger.Logger,
	)
	if err != nil {
		return fmt.Errorf("initialize Outbox Publisher: %w", err)
	}

	publisherDone = make(chan error, 1)
	publisherCtx, stopPublisher := context.WithCancel(context.Background())
	publisherCancel = stopPublisher

	go func() {
		publisherDone <- publisher.Run(publisherCtx)
	}()

	logger.Debug("initializing feature", zap.String("feature", "statistics"))

	var statisticsCache statistics_service.StatisticsCache

	if cache != nil {
		statisticsCache = statistics_redis_cache.NewStatisticsCache(cache)
	}

	statisticsRepository := statistics_postgres_repository.NewStatisticsRepository(pool)
	statisticsService := statistics_service.NewStatisticsService(
		statisticsRepository,
		statisticsCache,
		observability.TracerProvider(),
	)
	statisticsTransportHTTP := statistics_transport_http.NewStatisticsHTTPHandler(statisticsService)

	logger.Debug("initializing HTTP server")

	httpServer := core_http_server.NewHTTPServer(
		core_http_server.NewConfigMust(),
		logger,
		core_http_middleware.CORS(),
		core_http_middleware.RequestID(),
		core_http_middleware.Logger(logger),
		core_http_middleware.Panic(),
	)

	httpServer.RegisterAPIRouteInstrumenter(
		func(method, path string, handler http.Handler) http.Handler {
			handler = core_http_middleware.ChainMiddleware(
				handler,
				core_http_middleware.Trace(),
				core_http_middleware.Panic(),
			)

			return observability.InstrumentHTTPRoute(
				method,
				path,
				handler,
			)
		},
	)

	apiVersionRouter1 := core_http_server.NewAPIVersionRouter(
		core_http_server.ApiVersion1,
	)

	apiVersionRouter1.RegisterRouts(usersTransportHTTP.Routes()...)
	apiVersionRouter1.RegisterRouts(tasksTransportHTTP.Routes()...)
	apiVersionRouter1.RegisterRouts(activityTransportHTTP.Routes()...)
	apiVersionRouter1.RegisterRouts(statisticsTransportHTTP.Routes()...)

	// Example of usage apiVersionRouter2 with dummy middleware.
	// apiVersionRouter2 := core_http_server.NewAPIVersionRouter(
	//     core_http_server.ApiVersion2,
	//     core_http_middleware.Dummy("api v2 middleware"),
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

	logger.Info(
		"Kafka management UI",
		zap.String("url", "http://127.0.0.1:8080"),
	)

	if err := httpServer.Run(ctx); err != nil {
		return fmt.Errorf("run HTTP server: %w", err)
	}

	return nil
}
