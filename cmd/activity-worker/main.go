package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	core_config "github.com/Reigor1/20h-project/internal/core/config"
	kafka_consumer "github.com/Reigor1/20h-project/internal/core/kafka/consumer"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
	core_observability_postgres "github.com/Reigor1/20h-project/internal/core/observability/postgres"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	activity_consumer "github.com/Reigor1/20h-project/internal/features/activity/consumer"
	activity_postgres_repository "github.com/Reigor1/20h-project/internal/features/activity/repository/postgres"
	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Activity Worker failed:", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	appConfig := core_config.NewConfigMust()
	time.Local = appConfig.TimeZone

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	logger, err := core_logger.NewLogger(core_logger.NewConfigMust())
	if err != nil {
		return fmt.Errorf("initialize Activity Worker logger: %w", err)
	}

	defer func() {
		if err := logger.Close(); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("close Activity Worker logger: %w", err),
			)
		}
	}()

	observability, err := core_observability.New(
		ctx,
		core_observability.Config{
			ServiceName:           "20h-project-activity-worker",
			ServiceVersion:        os.Getenv("SERVICE_VERSION"),
			DeploymentEnvironment: os.Getenv("DEPLOYMENT_ENVIRONMENT"),
		},
	)
	if err != nil {
		return fmt.Errorf("initialize Activity Worker observability: %w", err)
	}

	var pool *core_pgx_pool.Pool

	// Shutdown observability while the PostgreSQL pool is still available
	// for the final metric collection, then close the pool.
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer shutdownCancel()

		if err := observability.Shutdown(shutdownCtx); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("shutdown Activity Worker observability: %w", err),
			)
		}

		if pool != nil {
			pool.Close()
		}
	}()

	pgxTracer := observability.NewPostgresTracer()

	pool, err = core_pgx_pool.NewPool(
		ctx,
		core_pgx_pool.NewConfigMust(),
		pgxTracer,
	)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL pool: %w", err)
	}

	if err := core_observability_postgres.RegisterPoolMetrics(
		observability.MeterProvider(),
		pool,
	); err != nil {
		return fmt.Errorf("register Activity Worker PostgreSQL metrics: %w", err)
	}

	kafkaConfig, err := kafka_consumer.NewConfig()
	if err != nil {
		return fmt.Errorf("load Kafka Consumer config: %w", err)
	}

	kafkaClient, err := kafka_consumer.NewConsumer(kafkaConfig)
	if err != nil {
		return fmt.Errorf("initialize Kafka Consumer: %w", err)
	}

	defer func() {
		if err := kafkaClient.Close(); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("close Kafka Consumer: %w", err),
			)
		}
	}()

	producerConfig, err := kafka_producer.NewConfig()
	if err != nil {
		return fmt.Errorf("load Kafka Producer config: %w", err)
	}

	producerConfig.ClientID = kafkaConfig.ClientID + "-dlq"

	kafkaProducer, err := kafka_producer.NewProducer(
		producerConfig,
		func(err error) {
			logger.Warn(
				"Kafka DLQ Producer reported an error",
				zap.Error(err),
			)
		},
	)
	if err != nil {
		return fmt.Errorf("initialize Kafka DLQ Producer: %w", err)
	}

	defer func() {
		if err := kafkaProducer.Close(30 * time.Second); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("close Kafka DLQ Producer: %w", err),
			)
		}
	}()

	activityRepository := activity_postgres_repository.NewActivityRepository(
		pool,
	)

	activityService := activity_service.NewActivityService(
		activityRepository,
	)

	worker, err := activity_consumer.NewWorker(
		kafkaConfig.Topic,
		kafkaConfig.DLQTopic,
		kafkaConfig.RetryBaseDelay,
		kafkaConfig.RetryMaxDelay,
		kafkaConfig.ShutdownTimeout,
		kafkaConfig.MaxPendingMessages,
		kafkaClient,
		kafkaProducer,
		activityService,
		logger.Logger,
	)
	if err != nil {
		return fmt.Errorf("initialize Activity Worker: %w", err)
	}

	logger.Info(
		"Activity Worker initialized",
		zap.String("group_id", kafkaConfig.GroupID),
		zap.String("topic", kafkaConfig.Topic),
		zap.String("dlq_topic", kafkaConfig.DLQTopic),
	)

	return worker.Run(ctx)
}
