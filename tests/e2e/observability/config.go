package main

import (
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string
	UsersCount     int
	TasksPerUser   int
	RequestTimeout time.Duration
	CleanupTimeout time.Duration
}

func loadConfig() (Config, error) {
	var config Config

	flag.StringVar(
		&config.BaseURL,
		"base-url",
		"http://localhost:5050",
		"Base URL of the application",
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
		"Number of tasks created for each user",
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

	flag.Parse()

	if config.UsersCount <= 0 {
		return Config{}, fmt.Errorf("users must be greater than zero")
	}

	if config.TasksPerUser <= 0 {
		return Config{}, fmt.Errorf("tasks-per-user must be greater than zero")
	}

	if config.RequestTimeout <= 0 || config.CleanupTimeout <= 0 {
		return Config{}, fmt.Errorf("timeouts must be greater than zero")
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
