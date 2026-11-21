package main

import (
	"reflect"
	"testing"
	"time"
)

// TestParseWorkerStages verifies the end-to-end or integration behavior represented by its test scenarios.
func TestParseWorkerStages(t *testing.T) {
	got, err := parseWorkerStages("75, 100, 150")
	if err != nil {
		t.Fatalf("parseWorkerStages() error = %v", err)
	}

	want := []int{75, 100, 150}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWorkerStages() = %v, want %v", got, want)
	}
}

// TestParseWorkerStagesRejectsInvalidValues verifies the end-to-end or integration behavior represented by its test scenarios.
func TestParseWorkerStagesRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{
		"",
		"0,10",
		"10,10",
		"20,10",
		"10,invalid",
		"10,,20",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := parseWorkerStages(value); err == nil {
				t.Fatalf("parseWorkerStages(%q) error = nil, want error", value)
			}
		})
	}
}

// TestPercentileDuration verifies the end-to-end or integration behavior represented by its test scenarios.
func TestPercentileDuration(t *testing.T) {
	samples := []time.Duration{
		time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
	}

	tests := []struct {
		name       string
		percentile int
		want       time.Duration
	}{
		{name: "p50", percentile: 50, want: 3 * time.Millisecond},
		{name: "p95", percentile: 95, want: 5 * time.Millisecond},
		{name: "p99", percentile: 99, want: 5 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentileDuration(samples, tt.percentile); got != tt.want {
				t.Fatalf("percentileDuration() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestLoadProfiles verifies the end-to-end or integration behavior represented by its test scenarios.
func TestLoadProfiles(t *testing.T) {
	config := Config{
		LoadProfile:         "standard",
		StageDuration:       30 * time.Second,
		SteadyStateWorkers:  50,
		SteadyStateDuration: 5 * time.Minute,
		StressWorkers:       []int{75, 100, 150},
	}

	profiles, err := loadProfiles(config)
	if err != nil {
		t.Fatalf("loadProfiles() error = %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("loadProfiles() returned %d profiles, want 2", len(profiles))
	}
	if profiles[0].name != "ramp-up" || profiles[1].name != "steady-state" {
		t.Fatalf("profile order = %q, %q; want ramp-up, steady-state", profiles[0].name, profiles[1].name)
	}
}
