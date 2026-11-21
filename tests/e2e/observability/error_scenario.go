package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// postgresInt32OverflowID defines a shared limit or value used by the test workflow.
const postgresInt32OverflowID = "2147483648"

// runErrorScenario executes the corresponding observability test workflow.
func runErrorScenario(
	ctx context.Context,
	client *apiClient,
	requestsCount int,
) error {
	if strconv.IntSize < 64 {
		return fmt.Errorf(
			"error scenario requires a 64-bit Go architecture",
		)
	}

	if requestsCount <= 0 {
		return fmt.Errorf("requests count must be greater than zero")
	}

	const (
		method = http.MethodGet
		path   = usersPath + "/" + postgresInt32OverflowID
	)

	fmt.Printf(
		"Starting HTTP 5xx scenario: requests=%d, target=%s %s\n",
		requestsCount,
		method,
		path,
	)

	startedAt := time.Now()

	var (
		expected5xx  int
		totalLatency time.Duration
		statusCodes  = make(map[int]int64)
	)

	for requestNumber := 1; requestNumber <= requestsCount; requestNumber++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("error scenario interrupted: %w", err)
		}

		requestStartedAt := time.Now()

		response, err := client.do(
			ctx,
			method,
			path,
			nil,
		)

		latency := time.Since(requestStartedAt)

		if err != nil {
			return fmt.Errorf(
				"request %d/%d failed before receiving HTTP response: %w",
				requestNumber,
				requestsCount,
				err,
			)
		}

		statusCodes[response.StatusCode]++

		if response.StatusCode != http.StatusInternalServerError {
			return fmt.Errorf(
				"request %d/%d: expected HTTP %d, got: %w",
				requestNumber,
				requestsCount,
				http.StatusInternalServerError,
				expectStatus(
					response,
					method,
					path,
					http.StatusInternalServerError,
				),
			)
		}

		expected5xx++
		totalLatency += latency
	}

	elapsed := time.Since(startedAt)

	var averageLatency time.Duration
	if expected5xx > 0 {
		averageLatency = totalLatency / time.Duration(expected5xx)
	}

	var requestsPerSecond float64
	if elapsed > 0 {
		requestsPerSecond = float64(expected5xx) / elapsed.Seconds()
	}

	fmt.Printf(
		"HTTP 5xx scenario completed: requests=%d, expected_5xx=%d, rps=%.1f, avg_latency=%s, statuses={%s}\n",
		requestsCount,
		expected5xx,
		requestsPerSecond,
		averageLatency.Round(time.Microsecond),
		formatStatusCounts(statusCodes),
	)

	return nil
}
