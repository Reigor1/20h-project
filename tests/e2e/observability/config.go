package main

import (
	"flag"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxLoadWorkers = 1_000

type Config struct {
	BaseURL             string
	Scenario            string
	LoadProfile         string
	UsersCount          int
	TasksPerUser        int
	RequestTimeout      time.Duration
	CleanupTimeout      time.Duration
	StageDuration       time.Duration
	SteadyStateWorkers  int
	SteadyStateDuration time.Duration
	StressWorkers       []int
	ReportDir           string
	ErrorRequestsCount  int
}

func loadConfig() (Config, error) {
	var config Config
	var stressWorkers string

	flag.StringVar(
		&config.BaseURL,
		"base-url",
		"http://localhost:5050",
		"Base URL of the application",
	)
	flag.StringVar(
		&config.Scenario,
		"scenario",
		"observability",
		"Scenario to run: observability or load",
	)
	flag.StringVar(
		&config.LoadProfile,
		"load-profile",
		"standard",
		"Load profile: standard, ramp-up, steady-state, stress or all",
	)
	flag.IntVar(
		&config.UsersCount,
		"users",
		10,
		"Number of test users",
	)
	flag.IntVar(
		&config.TasksPerUser,
		"tasks-per-user",
		2,
		"Number of test tasks per user",
	)
	flag.DurationVar(
		&config.RequestTimeout,
		"request-timeout",
		5*time.Second,
		"Timeout for individual HTTP requests",
	)
	flag.DurationVar(
		&config.CleanupTimeout,
		"cleanup-timeout",
		15*time.Second,
		"Timeout for fixture cleanup",
	)
	flag.DurationVar(
		&config.StageDuration,
		"stage-duration",
		30*time.Second,
		"Duration of each ramp-up or stress stage",
	)
	flag.IntVar(
		&config.SteadyStateWorkers,
		"steady-workers",
		50,
		"Number of workers in the steady-state profile",
	)
	flag.DurationVar(
		&config.SteadyStateDuration,
		"steady-duration",
		5*time.Minute,
		"Duration of the steady-state profile",
	)
	flag.StringVar(
		&stressWorkers,
		"stress-workers",
		"75,100,150",
		"Comma-separated, strictly increasing worker counts for stress profile",
	)
	flag.StringVar(
		&config.ReportDir,
		"report-dir",
		"out/load-tests",
		"Directory for JSON load-test reports",
	)
	flag.IntVar(
		&config.ErrorRequestsCount,
		"error-requests",
		10,
		"Number of requests in the HTTP 5xx scenario",
	)

	flag.Parse()

	var err error
	config.StressWorkers, err = parseWorkerStages(stressWorkers)
	if err != nil {
		return Config{}, fmt.Errorf("invalid stress-workers: %w", err)
	}

	if config.Scenario != "observability" && config.Scenario != "load" {
		return Config{}, fmt.Errorf(
			"unsupported scenario %q: want observability or load",
			config.Scenario,
		)
	}

	switch config.LoadProfile {
	case "standard", "ramp-up", "steady-state", "stress", "all":
	default:
		return Config{}, fmt.Errorf(
			"unsupported load profile %q: want standard, ramp-up, steady-state, stress or all",
			config.LoadProfile,
		)
	}

	if config.UsersCount <= 0 {
		return Config{}, fmt.Errorf("users must be greater than zero")
	}

	if config.TasksPerUser <= 0 {
		return Config{}, fmt.Errorf("tasks-per-user must be greater than zero")
	}

	if config.RequestTimeout <= 0 || config.CleanupTimeout <= 0 {
		return Config{}, fmt.Errorf("timeouts must be greater than zero")
	}

	if config.StageDuration <= 0 || config.SteadyStateDuration <= 0 {
		return Config{}, fmt.Errorf("load stage durations must be greater than zero")
	}

	if config.SteadyStateWorkers <= 0 || config.SteadyStateWorkers > maxLoadWorkers {
		return Config{}, fmt.Errorf("steady-workers must be between 1 and %d", maxLoadWorkers)
	}
	for _, workers := range config.StressWorkers {
		if workers > maxLoadWorkers {
			return Config{}, fmt.Errorf("stress worker count %d exceeds maximum %d", workers, maxLoadWorkers)
		}
	}

	if strings.TrimSpace(config.ReportDir) == "" {
		return Config{}, fmt.Errorf("report-dir must not be empty")
	}

	if config.ErrorRequestsCount <= 0 {
		return Config{}, fmt.Errorf("error-requests must be greater than zero")
	}

	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("parse base URL: %w", err)
	}

	if (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
		parsedURL.Host == "" ||
		(parsedURL.Path != "" && parsedURL.Path != "/") ||
		parsedURL.RawQuery != "" ||
		parsedURL.Fragment != "" {
		return Config{}, fmt.Errorf("invalid base URL: %q", config.BaseURL)
	}

	config.BaseURL = strings.TrimRight(config.BaseURL, "/")

	return config, nil
}

func parseWorkerStages(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	workers := make([]int, 0, len(parts))
	previous := 0

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("worker count must not be empty")
		}

		count, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("parse worker count %q: %w", part, err)
		}
		if count <= previous {
			return nil, fmt.Errorf(
				"worker counts must be positive and strictly increasing: %d after %d",
				count,
				previous,
			)
		}

		workers = append(workers, count)
		previous = count
	}

	return workers, nil
}
