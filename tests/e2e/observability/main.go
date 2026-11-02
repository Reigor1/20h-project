package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := run(ctx, config); err != nil {
		fmt.Fprintln(os.Stderr, "observability generator:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, config Config) (runErr error) {
	client := newAPIClient(config)
	defer client.httpClient.CloseIdleConnections()

	fmt.Println("Checking application availability...")

	response, err := client.do(
		ctx,
		http.MethodGet,
		usersPath+"?limit=1",
		nil,
	)
	if err != nil {
		return fmt.Errorf("preflight request: %w", err)
	}

	if err := expectStatus(
		response,
		http.MethodGet,
		usersPath,
		http.StatusOK,
	); err != nil {
		return fmt.Errorf("application preflight failed: %w", err)
	}

	fmt.Println("Application is available.")

	fixtures := newFixtureSet()

	// Cleanup uses an independent context so it can still run
	// after the workload context has been canceled.
	defer func() {
		fmt.Println("Cleaning up test fixtures...")

		cleanupCtx, cancel := context.WithTimeout(
			context.Background(),
			config.CleanupTimeout,
		)
		defer cancel()

		if err := fixtures.cleanup(cleanupCtx, client); err != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("cleanup fixtures: %w", err),
			)
			return
		}

		fmt.Println("Cleanup completed.")
	}()

	fmt.Println("Creating test fixtures...")

	if err := fixtures.create(ctx, client, config); err != nil {
		return fmt.Errorf("create test fixtures: %w", err)
	}

	fmt.Printf(
		"Fixtures created: users=%d, tasks=%d\n",
		len(fixtures.users),
		fixtures.taskCount(),
	)

	fmt.Println("Fixture lifecycle phase completed.")
	fmt.Println("Next phases: route coverage, ramp-up workload, HTTP 5xx scenario.")

	return nil
}
