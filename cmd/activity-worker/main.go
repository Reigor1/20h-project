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

	pool, err := core_pgx_pool.NewPool(
		ctx,
		core_pgx_pool.NewConfigMust(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL pool: %w", err)
	}
	defer pool.Close()

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
