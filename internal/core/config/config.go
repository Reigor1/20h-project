package core_config

import (
	"fmt"
	"os"
	"time"
)

// Config contains process-wide settings shared by application entry points.
type Config struct {
	TimeZone *time.Location
}

// NewConfig loads the process time zone from TIME_ZONE, defaulting to UTC.
// It returns an error when the configured IANA time zone cannot be loaded.
func NewConfig() (*Config, error) {
	tz := os.Getenv("TIME_ZONE")
	if tz == "" {
		tz = "UTC"
	}

	zone, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("load time zone: %s: %w", tz, err)
	}

	return &Config{
		TimeZone: zone,
	}, nil
}

// NewConfigMust returns the core configuration and panics if it cannot be loaded.
func NewConfigMust() *Config {
	config, err := NewConfig()
	if err != nil {
		err = fmt.Errorf("get core config: %w", err)
		panic(err)
	}

	return config
}
