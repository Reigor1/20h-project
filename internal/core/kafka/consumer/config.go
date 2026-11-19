package kafka_consumer

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	BootstrapServers string `envconfig:"BOOTSTRAP_SERVERS" default:"localhost:29092,localhost:39092,localhost:49092"`

	GroupID         string `envconfig:"CONSUMER_GROUP_ID" default:"activity-workers"`
	ClientID        string `envconfig:"CONSUMER_CLIENT_ID" default:"20h-project-activity-worker"`
	Topic           string `envconfig:"CONSUMER_TOPIC" default:"task-events"`
	DLQTopic        string `envconfig:"CONSUMER_DLQ_TOPIC" default:"task-events.dlq"`
	AutoOffsetReset string `envconfig:"CONSUMER_AUTO_OFFSET_RESET" default:"earliest"`

	RetryBaseDelay     time.Duration `envconfig:"CONSUMER_RETRY_BASE_DELAY" default:"1s"`
	RetryMaxDelay      time.Duration `envconfig:"CONSUMER_RETRY_MAX_DELAY" default:"30s"`
	MaxPendingMessages int           `envconfig:"CONSUMER_MAX_PENDING_MESSAGES" default:"1000"`
	ShutdownTimeout    time.Duration `envconfig:"CONSUMER_SHUTDOWN_TIMEOUT" default:"30s"`
}

func NewConfig() (Config, error) {
	var config Config

	if err := envconfig.Process("KAFKA", &config); err != nil {
		return Config{}, fmt.Errorf("process Kafka Consumer config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate Kafka Consumer config: %w", err)
	}

	return config, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.BootstrapServers) == "" {
		return fmt.Errorf("Kafka bootstrap servers must not be empty")
	}

	if strings.TrimSpace(c.GroupID) == "" {
		return fmt.Errorf("Kafka Consumer group ID must not be empty")
	}

	if strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("Kafka Consumer client ID must not be empty")
	}

	if strings.TrimSpace(c.Topic) == "" {
		return fmt.Errorf("Kafka Consumer topic must not be empty")
	}

	if strings.TrimSpace(c.DLQTopic) == "" {
		return fmt.Errorf("Kafka Consumer DLQ topic must not be empty")
	}

	if c.Topic == c.DLQTopic {
		return fmt.Errorf("Kafka Consumer topic and DLQ topic must differ")
	}

	switch c.AutoOffsetReset {
	case "earliest", "latest", "error":
	default:
		return fmt.Errorf(
			"unsupported Kafka auto offset reset value %q",
			c.AutoOffsetReset,
		)
	}

	if c.RetryBaseDelay <= 0 {
		return fmt.Errorf("Kafka Consumer retry base delay must be positive")
	}

	if c.RetryMaxDelay < c.RetryBaseDelay {
		return fmt.Errorf(
			"Kafka Consumer retry max delay must not be less than base delay",
		)
	}

	if c.MaxPendingMessages <= 0 {
		return fmt.Errorf(
			"Kafka Consumer max pending messages must be positive",
		)
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("Kafka Consumer shutdown timeout must be positive")
	}

	return nil
}
