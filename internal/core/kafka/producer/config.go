package kafka_producer

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config contains Kafka Producer connection, batching, compression, delivery,
// and local queue limits.
type Config struct {
	BootstrapServers string `envconfig:"BOOTSTRAP_SERVERS" default:"localhost:29092,localhost:39092,localhost:49092"`
	ClientID         string `envconfig:"CLIENT_ID" default:"20h-project-todoapp"`

	Linger           time.Duration `envconfig:"LINGER" default:"5ms"`
	BatchSizeBytes   int           `envconfig:"BATCH_SIZE_BYTES" default:"1000000"`
	BatchNumMessages int           `envconfig:"BATCH_NUM_MESSAGES" default:"10000"`
	CompressionType  string        `envconfig:"COMPRESSION_TYPE" default:"lz4"`
	MessageTimeout   time.Duration `envconfig:"MESSAGE_TIMEOUT" default:"60s"`

	QueueMaxMessages  int `envconfig:"QUEUE_BUFFERING_MAX_MESSAGES" default:"10000"`
	QueueMaxKilobytes int `envconfig:"QUEUE_BUFFERING_MAX_KB" default:"65536"`
}

// NewConfig loads KAFKA_* Producer settings from the environment and validates them.
func NewConfig() (Config, error) {
	var config Config

	if err := envconfig.Process("KAFKA", &config); err != nil {
		return Config{}, fmt.Errorf("process Kafka config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate Kafka config: %w", err)
	}

	return config, nil
}

// Validate rejects invalid broker, batching, timeout, queue, and compression settings.
func (c Config) Validate() error {
	if strings.TrimSpace(c.BootstrapServers) == "" {
		return fmt.Errorf("Kafka bootstrap servers must not be empty")
	}

	if strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("Kafka client ID must not be empty")
	}

	if c.Linger < 0 {
		return fmt.Errorf("Kafka linger must not be negative")
	}

	if c.BatchSizeBytes <= 0 {
		return fmt.Errorf("Kafka batch size must be positive")
	}

	if c.BatchNumMessages <= 0 {
		return fmt.Errorf("Kafka batch message count must be positive")
	}

	if c.MessageTimeout <= 0 {
		return fmt.Errorf("Kafka message timeout must be positive")
	}

	if c.QueueMaxMessages <= 0 || c.QueueMaxKilobytes <= 0 {
		return fmt.Errorf("Kafka producer queue limits must be positive")
	}

	switch c.CompressionType {
	case "none", "gzip", "snappy", "lz4", "zstd":
	default:
		return fmt.Errorf("unsupported Kafka compression type %q", c.CompressionType)
	}

	return nil
}
