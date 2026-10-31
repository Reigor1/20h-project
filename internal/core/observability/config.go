package core_observability

import "time"

const (
	defaultServiceName           = "20h-project"
	defaultMetricsExportInterval = 10 * time.Second
)

type Config struct {
	ServiceName           string
	MetricsExportInterval time.Duration
}

func (c Config) withDefaults() Config {
	if c.ServiceName == "" {
		c.ServiceName = defaultServiceName
	}

	if c.MetricsExportInterval <= 0 {
		c.MetricsExportInterval = defaultMetricsExportInterval
	}

	return c
}
