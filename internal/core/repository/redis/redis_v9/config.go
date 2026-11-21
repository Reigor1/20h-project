package core_redis_v9

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config contains the connection settings for go-redis/v9.
type Config struct {
	Addr     string        `envconfig:"ADDR" default:"localhost:6379"`
	Username string        `envconfig:"USERNAME"`
	Password string        `envconfig:"PASSWORD"`
	DB       int           `envconfig:"DB" default:"0"`
	Timeout  time.Duration `envconfig:"TIMEOUT" default:"5s"`
}

// NewConfig reads Redis connection settings from REDIS_* environment variables.
func NewConfig() (Config, error) {
	var config Config

	if err := envconfig.Process("REDIS", &config); err != nil {
		return Config{}, fmt.Errorf("process Redis envconfig: %w", err)
	}

	if err := config.validate(); err != nil {
		return Config{}, fmt.Errorf("validate Redis config: %w", err)
	}

	return config, nil
}

// validate checks the Redis address, database index, and operation timeout.
func (c Config) validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("Redis address must not be empty")
	}
	if c.DB < 0 {
		return fmt.Errorf("Redis database number must not be negative")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("Redis timeout must be greater than zero")
	}

	return nil
}
