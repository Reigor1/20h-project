package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var workloadStages = []int{5, 10, 25, 50}

const maxLatencySamplesPerWorker = 10_000

type workloadTarget struct {
	method string
	path   string
}

type workloadStats struct {
	requests           int64
	successful         int64
	failed             int64
	transportErrors    int64
	cancelledRequests  int64
	totalLatency       time.Duration
	maxLatency         time.Duration
	latencySamples     []time.Duration
	latencySamplesSeen int64
	statusCodes        map[int]int64
}

type loadStageReport struct {
	Profile           string        `json:"profile"`
	Stage             int           `json:"stage"`
	Workers           int           `json:"workers"`
	LoadDuration      string        `json:"load_duration"`
	DrainDuration     string        `json:"drain_duration"`
	Requests          int64         `json:"requests"`
	Successful        int64         `json:"successful"`
	Failed            int64         `json:"failed"`
	TransportErrors   int64         `json:"transport_errors"`
	Cancelled         int64         `json:"cancelled"`
	RequestsPerSecond float64       `json:"requests_per_second"`
	ErrorRatePercent  float64       `json:"error_rate_percent"`
	AverageLatency    string        `json:"average_latency"`
	P50               string        `json:"p50"`
	P95               string        `json:"p95"`
	P99               string        `json:"p99"`
	MaxLatency        string        `json:"max_latency"`
	LatencySamples    int           `json:"latency_samples"`
	StatusCodes       map[int]int64 `json:"status_codes"`
}

func runWorkload(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
	stageDuration time.Duration,
) error {
	_, err := runWorkerStages(
		ctx,
		client,
		fixtures,
		"ramp-up",
		workloadStages,
		stageDuration,
	)
	return err
}

func runWorkerStages(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
	profileName string,
	workerStages []int,
	stageDuration time.Duration,
) ([]loadStageReport, error) {
	if len(workerStages) == 0 {
		return nil, fmt.Errorf("profile %q has no workload stages", profileName)
	}
	if stageDuration <= 0 {
		return nil, fmt.Errorf("profile %q stage duration must be positive", profileName)
	}

	targets, err := buildWorkloadTargets(fixtures)
	if err != nil {
		return nil, fmt.Errorf("build workload targets: %w", err)
	}

	fmt.Printf(
		"Starting %s workload: stage_duration=%s, stages=%v, targets=%d\n",
		profileName,
		stageDuration,
		workerStages,
		len(targets),
	)

	reports := make([]loadStageReport, 0, len(workerStages))
	var totalStats workloadStats
	totalStats.statusCodes = make(map[int]int64)

	for stageIndex, workerCount := range workerStages {
		if workerCount <= 0 {
			return reports, fmt.Errorf(
				"profile %q stage %d has invalid worker count %d",
				profileName,
				stageIndex+1,
				workerCount,
			)
		}

		fmt.Printf(
			"Starting %s stage %d/%d: workers=%d, duration=%s\n",
			profileName,
			stageIndex+1,
			len(workerStages),
			workerCount,
			stageDuration,
		)

		stageStartedAt := time.Now()
		stageCtx, cancelStage := context.WithTimeout(ctx, stageDuration)
		workersStats := make([]workloadStats, workerCount)

		var workers sync.WaitGroup
		for workerID := range workersStats {
			workersStats[workerID].statusCodes = make(map[int]int64)
			workers.Add(1)

			go func(workerID int) {
				defer workers.Done()
				runWorkloadWorker(
					stageCtx,
					ctx,
					client,
					targets,
					workerID,
					&workersStats[workerID],
				)
			}(workerID)
		}

		// Wait until the stage expires or the parent context is cancelled.
		<-stageCtx.Done()

		loadDuration := time.Since(stageStartedAt)
		if ctx.Err() == nil {
			loadDuration = stageDuration
		}

		cancelStage()
		workers.Wait()

		drainDuration := time.Since(stageStartedAt) - loadDuration
		if drainDuration < 0 {
			drainDuration = 0
		}

		stageStats := workloadStats{
			statusCodes: make(map[int]int64),
		}
		for _, workerStats := range workersStats {
			stageStats.add(workerStats, true)
		}

		report := printWorkloadStage(
			profileName,
			stageIndex+1,
			workerCount,
			loadDuration,
			drainDuration,
			stageStats,
		)
		reports = append(reports, report)
		totalStats.add(stageStats, false)

		if err := ctx.Err(); err != nil {
			return reports, fmt.Errorf("%s workload interrupted: %w", profileName, err)
		}
		if stageStats.failed > 0 {
			return reports, fmt.Errorf(
				"%s stage %d completed with %d failed requests (%d transport errors)",
				profileName,
				stageIndex+1,
				stageStats.failed,
				stageStats.transportErrors,
			)
		}
	}

	fmt.Printf(
		"%s workload completed: requests=%d, successful=%d, failed=%d, cancelled=%d\n",
		profileName,
		totalStats.requests,
		totalStats.successful,
		totalStats.failed,
		totalStats.cancelledRequests,
	)

	return reports, nil
}

func buildWorkloadTargets(
	fixtures *fixtureSet,
) ([]workloadTarget, error) {
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
	stageCtx context.Context,
	requestCtx context.Context,
	client *apiClient,
	targets []workloadTarget,
	workerID int,
	stats *workloadStats,
) {
	requestIndex := workerID

	for {
		select {
		case <-stageCtx.Done():
			return
		case <-requestCtx.Done():
			return
		default:
		}

		target := targets[requestIndex%len(targets)]
		requestIndex++

		startedAt := time.Now()
		response, err := client.do(
			requestCtx,
			target.method,
			target.path,
			nil,
		)
		duration := time.Since(startedAt)

		if err != nil && requestCtx.Err() != nil {
			stats.recordCancellation(duration)
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
	s.recordLatency(duration)

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

func (s *workloadStats) recordCancellation(duration time.Duration) {
	s.requests++
	s.cancelledRequests++
	s.totalLatency += duration
	s.recordLatency(duration)

	if duration > s.maxLatency {
		s.maxLatency = duration
	}
}

func (s *workloadStats) recordLatency(duration time.Duration) {
	s.latencySamplesSeen++

	if len(s.latencySamples) < maxLatencySamplesPerWorker {
		s.latencySamples = append(s.latencySamples, duration)
		return
	}

	// Reservoir sampling keeps memory bounded while preserving a representative
	// latency sample across the complete lifetime of this worker.
	index := rand.Int63n(s.latencySamplesSeen)
	if index < maxLatencySamplesPerWorker {
		s.latencySamples[index] = duration
	}
}

func (s *workloadStats) add(other workloadStats, includeLatencySamples bool) {
	s.requests += other.requests
	s.successful += other.successful
	s.failed += other.failed
	s.transportErrors += other.transportErrors
	s.cancelledRequests += other.cancelledRequests
	s.totalLatency += other.totalLatency

	if other.maxLatency > s.maxLatency {
		s.maxLatency = other.maxLatency
	}

	for statusCode, count := range other.statusCodes {
		s.statusCodes[statusCode] += count
	}

	if includeLatencySamples {
		s.latencySamples = append(s.latencySamples, other.latencySamples...)
	}
}

func printWorkloadStage(
	profileName string,
	stageNumber int,
	workerCount int,
	loadDuration time.Duration,
	drainDuration time.Duration,
	stats workloadStats,
) loadStageReport {
	sort.Slice(stats.latencySamples, func(i, j int) bool {
		return stats.latencySamples[i] < stats.latencySamples[j]
	})

	var requestsPerSecond float64
	if loadDuration > 0 {
		requestsPerSecond = float64(stats.requests) / loadDuration.Seconds()
	}

	var averageLatency time.Duration
	if stats.requests > 0 {
		averageLatency = stats.totalLatency / time.Duration(stats.requests)
	}

	var errorRatePercent float64
	completedRequests := stats.requests - stats.cancelledRequests
	if completedRequests > 0 {
		errorRatePercent = 100 * float64(stats.failed) / float64(completedRequests)
	}

	report := loadStageReport{
		Profile:           profileName,
		Stage:             stageNumber,
		Workers:           workerCount,
		LoadDuration:      loadDuration.Round(time.Millisecond).String(),
		DrainDuration:     drainDuration.Round(time.Millisecond).String(),
		Requests:          stats.requests,
		Successful:        stats.successful,
		Failed:            stats.failed,
		TransportErrors:   stats.transportErrors,
		Cancelled:         stats.cancelledRequests,
		RequestsPerSecond: requestsPerSecond,
		ErrorRatePercent:  errorRatePercent,
		AverageLatency:    averageLatency.Round(time.Microsecond).String(),
		P50:               percentileDuration(stats.latencySamples, 50).Round(time.Microsecond).String(),
		P95:               percentileDuration(stats.latencySamples, 95).Round(time.Microsecond).String(),
		P99:               percentileDuration(stats.latencySamples, 99).Round(time.Microsecond).String(),
		MaxLatency:        stats.maxLatency.Round(time.Microsecond).String(),
		LatencySamples:    len(stats.latencySamples),
		StatusCodes:       stats.statusCodes,
	}

	fmt.Printf(
		"%s stage %d complete: workers=%d, load_duration=%s, drain_duration=%s, requests=%d, successful=%d, failed=%d, transport_errors=%d, cancelled=%d, rps=%.1f, error_rate=%.2f%%, avg_latency=%s, p50=%s, p95=%s, p99=%s, max_latency=%s, samples=%d, statuses={%s}\n",
		profileName,
		stageNumber,
		workerCount,
		loadDuration.Round(time.Millisecond),
		drainDuration.Round(time.Millisecond),
		stats.requests,
		stats.successful,
		stats.failed,
		stats.transportErrors,
		stats.cancelledRequests,
		requestsPerSecond,
		errorRatePercent,
		averageLatency.Round(time.Microsecond),
		report.P50,
		report.P95,
		report.P99,
		stats.maxLatency.Round(time.Microsecond),
		len(stats.latencySamples),
		formatStatusCounts(stats.statusCodes),
	)

	return report
}

func percentileDuration(sortedSamples []time.Duration, percentile int) time.Duration {
	if len(sortedSamples) == 0 || percentile <= 0 || percentile > 100 {
		return 0
	}

	index := (len(sortedSamples)*percentile+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedSamples) {
		index = len(sortedSamples) - 1
	}

	return sortedSamples[index]
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
