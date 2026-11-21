package core_observability

import "time"

const (
	defaultServiceName           = "20h-project"
	defaultServiceVersion        = "dev"
	defaultDeploymentEnvironment = "development"
	defaultMetricsExportInterval = 10 * time.Second
)

// Config identifies the instrumented service and controls periodic metric export.
type Config struct {
	ServiceName           string
	ServiceVersion        string
	DeploymentEnvironment string
	MetricsExportInterval time.Duration
}

// withDefaults fills omitted telemetry settings with development-safe defaults.
func (c Config) withDefaults() Config {
	if c.ServiceName == "" {
		c.ServiceName = defaultServiceName
	}

	if c.ServiceVersion == "" {
		c.ServiceVersion = defaultServiceVersion
	}

	if c.DeploymentEnvironment == "" {
		c.DeploymentEnvironment = defaultDeploymentEnvironment
	}

	if c.MetricsExportInterval <= 0 {
		c.MetricsExportInterval = defaultMetricsExportInterval
	}

	return c
}
