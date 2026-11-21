package outbox_publisher

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config controls the Outbox Publisher topic, polling, batching, leases, retries, and shutdown.
type Config struct {
	Topic          string        `envconfig:"TOPIC" default:"task-events"`
	PollInterval   time.Duration `envconfig:"POLL_INTERVAL" default:"1s"`
	BatchSize      int           `envconfig:"BATCH_SIZE" default:"100"`
	LeaseDuration  time.Duration `envconfig:"LEASE_DURATION" default:"2m"`
	RetryBaseDelay time.Duration `envconfig:"RETRY_BASE_DELAY" default:"1s"`
	RetryMaxDelay  time.Duration `envconfig:"RETRY_MAX_DELAY" default:"5m"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"30s"`
}

// NewConfig loads Outbox Publisher settings from the environment and validates them.
func NewConfig() (Config, error) {
	var config Config

	if err := envconfig.Process("OUTBOX_PUBLISHER", &config); err != nil {
		return Config{}, fmt.Errorf("process Outbox Publisher config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate Outbox Publisher config: %w", err)
	}

	return config, nil
}

// Validate checks the topic and requires positive polling, batch, lease, retry,
// and shutdown settings with a maximum retry delay no shorter than the base delay.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Topic) == "" {
		return fmt.Errorf("Outbox Publisher topic must not be empty")
	}

	if c.PollInterval <= 0 {
		return fmt.Errorf("Outbox Publisher poll interval must be positive")
	}

	if c.BatchSize <= 0 {
		return fmt.Errorf("Outbox Publisher batch size must be positive")
	}

	if c.LeaseDuration <= 0 {
		return fmt.Errorf("Outbox Publisher lease duration must be positive")
	}

	if c.RetryBaseDelay <= 0 || c.RetryMaxDelay < c.RetryBaseDelay {
		return fmt.Errorf("invalid Outbox Publisher retry delays")
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("Outbox Publisher shutdown timeout must be positive")
	}

	return nil
}
