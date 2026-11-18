package kafka_consumer

import (
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	BootstrapServers string `envconfig:"BOOTSTRAP_SERVERS" default:"localhost:29092,localhost:39092,localhost:49092"`

	GroupID         string `envconfig:"CONSUMER_GROUP_ID" default:"activity-workers"`
	ClientID        string `envconfig:"CONSUMER_CLIENT_ID" default:"20h-project-activity-worker"`
	Topic           string `envconfig:"CONSUMER_TOPIC" default:"task-events"`
	AutoOffsetReset string `envconfig:"CONSUMER_AUTO_OFFSET_RESET" default:"earliest"`
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

	switch c.AutoOffsetReset {
	case "earliest", "latest", "error":
	default:
		return fmt.Errorf(
			"unsupported Kafka auto offset reset value %q",
			c.AutoOffsetReset,
		)
	}

	return nil
}
