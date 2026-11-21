package kafka_consumer

import (
	"testing"
	"time"
)

// validConfig returns a Consumer configuration that satisfies all validation rules.
func validConfig() Config {
	return Config{
		BootstrapServers:  "localhost:29092",
		GroupID:           "activity-workers",
		ClientID:          "activity-worker",
		Topic:             "task-events",
		DLQTopic:          "task-events.dlq",
		AutoOffsetReset:   "earliest",
		RetryBaseDelay:    time.Second,
		RetryMaxDelay:     30 * time.Second,
		MaxPendingMessages: 1000,
		ShutdownTimeout:    30 * time.Second,
	}
}

// TestConfigValidateRetrySettings checks valid and invalid retry delay combinations.
func TestConfigValidateRetrySettings(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
	}{
		{
			name: "zero base delay",
			modify: func(config *Config) {
				config.RetryBaseDelay = 0
			},
		},
		{
			name: "negative base delay",
			modify: func(config *Config) {
				config.RetryBaseDelay = -time.Second
			},
		},
		{
			name: "max delay less than base delay",
			modify: func(config *Config) {
				config.RetryMaxDelay = time.Millisecond
			},
		},
		{
			name: "empty DLQ topic",
			modify: func(config *Config) {
				config.DLQTopic = ""
			},
		},
		{
			name: "same source and DLQ topics",
			modify: func(config *Config) {
				config.DLQTopic = config.Topic
			},
		},
		{
			name: "zero max pending messages",
			modify: func(config *Config) {
				config.MaxPendingMessages = 0
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			config := validConfig()
			testCase.modify(&config)

			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}

// TestConfigValidateAcceptsEqualRetryDelays verifies that equal base and maximum delays are allowed.
func TestConfigValidateAcceptsEqualRetryDelays(t *testing.T) {
	config := validConfig()
	config.RetryMaxDelay = config.RetryBaseDelay

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}
