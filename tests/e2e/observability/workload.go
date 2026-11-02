package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var workloadStages = []int{5, 10, 25, 50}

type workloadTarget struct {
	method string
	path   string
}

type workloadStats struct {
	requests        int64
	successful      int64
	failed          int64
	transportErrors int64
	totalLatency    time.Duration
	maxLatency      time.Duration
	statusCodes     map[int]int64
}

func runWorkload(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
	stageDuration time.Duration,
) error {
	targets, err := buildWorkloadTargets(fixtures)
	if err != nil {
		return fmt.Errorf("build workload targets: %w", err)
	}

	fmt.Printf(
		"Starting ramp-up workload: stage_duration=%s, targets=%d\n",
		stageDuration,
		len(targets),
	)

	var totalStats workloadStats
	totalStats.statusCodes = make(map[int]int64)

	for stageIndex, workerCount := range workloadStages {
		fmt.Printf(
			"Starting stage %d/%d: workers=%d, duration=%s\n",
			stageIndex+1,
			len(workloadStages),
			workerCount,
			stageDuration,
		)

		stageStartedAt := time.Now()

		stageCtx, cancel := context.WithTimeout(ctx, stageDuration)

		workersStats := make([]workloadStats, workerCount)

		var workers sync.WaitGroup

		for workerID := range workersStats {
			workersStats[workerID].statusCodes = make(map[int]int64)

			workers.Add(1)

			go func(workerID int) {
				defer workers.Done()

				runWorkloadWorker(
					stageCtx,
					client,
					targets,
					workerID,
					&workersStats[workerID],
				)
			}(workerID)
		}

		workers.Wait()
		cancel()

		elapsed := time.Since(stageStartedAt)

		var stageStats workloadStats
		stageStats.statusCodes = make(map[int]int64)

		for _, workerStats := range workersStats {
			stageStats.add(workerStats)
		}

		printWorkloadStage(
			stageIndex+1,
			workerCount,
			elapsed,
			stageStats,
		)

		totalStats.add(stageStats)

		if err := ctx.Err(); err != nil {
			return fmt.Errorf("workload interrupted: %w", err)
		}
	}

	fmt.Printf(
		"Ramp-up workload completed: requests=%d, successful=%d, failed=%d\n",
		totalStats.requests,
		totalStats.successful,
		totalStats.failed,
	)

	if totalStats.failed > 0 {
		return fmt.Errorf(
			"workload completed with %d failed requests (%d transport errors)",
			totalStats.failed,
			totalStats.transportErrors,
		)
	}

	return nil
}

func buildWorkloadTargets(fixtures *fixtureSet) ([]workloadTarget, error) {
	if fixtures == nil || len(fixtures.users) == 0 {
		return nil, fmt.Errorf("no fixture users available")
	}

	targets := []workloadTarget{
		{
			method: http.MethodGet,
			path: usersPath + "?" + url.Values{
				"limit":  {"10"},
				"offset": {"0"},
			}.Encode(),
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/statistics",
		},
	}

	for _, user := range fixtures.users {
		// Users without tasks may have been created and deleted
		// by the route coverage phase.
		if len(user.TaskIDs) == 0 {
			continue
		}

		userID := strconv.Itoa(user.ID)

		targets = append(targets,
			workloadTarget{
				method: http.MethodGet,
				path:   usersPath + "/" + userID,
			},
			workloadTarget{
				method: http.MethodGet,
				path: tasksPath + "?" + url.Values{
					"user_id": {userID},
					"limit":   {"10"},
					"offset":  {"0"},
				}.Encode(),
			},
			workloadTarget{
				method: http.MethodGet,
				path:   tasksPath + "/" + strconv.Itoa(user.TaskIDs[0]),
			},
			workloadTarget{
				method: http.MethodGet,
				path: "/api/v1/statistics?" + url.Values{
					"user_id": {userID},
				}.Encode(),
			},
		)
	}

	if len(targets) == 2 {
		return nil, fmt.Errorf("no fixture tasks available for workload")
	}

	return targets, nil
}

func runWorkloadWorker(
	ctx context.Context,
	client *apiClient,
	targets []workloadTarget,
	workerID int,
	stats *workloadStats,
) {
	requestIndex := workerID

	for {
		if ctx.Err() != nil {
			return
		}

		target := targets[requestIndex%len(targets)]
		requestIndex++

		startedAt := time.Now()

		response, err := client.do(
			ctx,
			target.method,
			target.path,
			nil,
		)

		duration := time.Since(startedAt)

		// Do not count requests interrupted by the stage deadline
		// or by cancellation of the parent context.
		if ctx.Err() != nil {
			return
		}

		stats.record(response.StatusCode, err, duration)
	}
}

func (s *workloadStats) record(
	statusCode int,
	err error,
	duration time.Duration,
) {
	s.requests++
	s.totalLatency += duration

	if duration > s.maxLatency {
		s.maxLatency = duration
	}

	if err != nil {
		s.failed++
		s.transportErrors++
		return
	}

	s.statusCodes[statusCode]++

	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		s.successful++
		return
	}

	s.failed++
}

func (s *workloadStats) add(other workloadStats) {
	s.requests += other.requests
	s.successful += other.successful
	s.failed += other.failed
	s.transportErrors += other.transportErrors
	s.totalLatency += other.totalLatency

	if other.maxLatency > s.maxLatency {
		s.maxLatency = other.maxLatency
	}

	for statusCode, count := range other.statusCodes {
		s.statusCodes[statusCode] += count
	}
}

func printWorkloadStage(
	stageNumber int,
	workerCount int,
	elapsed time.Duration,
	stats workloadStats,
) {
	var requestsPerSecond float64

	if elapsed > 0 {
		requestsPerSecond = float64(stats.requests) / elapsed.Seconds()
	}

	var averageLatency time.Duration

	if stats.requests > 0 {
		averageLatency = stats.totalLatency / time.Duration(stats.requests)
	}

	fmt.Printf(
		"Stage %d complete: workers=%d, duration=%s, requests=%d, successful=%d, failed=%d, transport_errors=%d, rps=%.1f, avg_latency=%s, max_latency=%s, statuses={%s}\n",
		stageNumber,
		workerCount,
		elapsed.Round(time.Millisecond),
		stats.requests,
		stats.successful,
		stats.failed,
		stats.transportErrors,
		requestsPerSecond,
		averageLatency.Round(time.Microsecond),
		stats.maxLatency.Round(time.Microsecond),
		formatStatusCounts(stats.statusCodes),
	)
}

func formatStatusCounts(statusCodes map[int]int64) string {
	if len(statusCodes) == 0 {
		return "none"
	}

	codes := make([]int, 0, len(statusCodes))

	for statusCode := range statusCodes {
		codes = append(codes, statusCode)
	}

	sort.Ints(codes)

	var parts []string

	for _, statusCode := range codes {
		parts = append(
			parts,
			fmt.Sprintf("%d=%d", statusCode, statusCodes[statusCode]),
		)
	}

	return strings.Join(parts, ", ")
}
