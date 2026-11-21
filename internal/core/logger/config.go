package core_logger

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config contains the minimum severity and directory used for application log output.
type Config struct {
	Level  string `envconfig:"LEVEL" default:"debug"`
	Folder string `envconfig:"FOLDER" required:"true"`
}

// NewConfig loads LOGGER_* settings from the environment.
func NewConfig() (Config, error) {
	var config Config

	if err := envconfig.Process("LOGGER", &config); err != nil {
		return Config{}, fmt.Errorf("process envconfig: %w", err)
	}

	fmt.Printf("LOGGER CONFIG: %+v\n", config)

	return config, nil
}

// NewConfigMust returns the logger configuration or panics when it is invalid.
func NewConfigMust() Config {
	config, err := NewConfig()
	if err != nil {
		err := fmt.Errorf("get Logger config: %w", err)
		panic(err)
	}

	return config
}
