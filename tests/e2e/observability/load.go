package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type loadProfile struct {
	name          string
	workerStages  []int
	stageDuration time.Duration
}

type loadTestReport struct {
	StartedAt           time.Time         `json:"started_at"`
	FinishedAt          time.Time         `json:"finished_at"`
	Profile             string            `json:"profile"`
	RampUpStageDuration string            `json:"ramp_up_stage_duration"`
	SteadyStateWorkers  int               `json:"steady_state_workers"`
	SteadyStateDuration string            `json:"steady_state_duration"`
	StressWorkers       []int             `json:"stress_workers"`
	Stages              []loadStageReport `json:"stages"`
}

func runLoadTest(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
	config Config,
) (runErr error) {
	startedAt := time.Now().UTC()
	var stageReports []loadStageReport

	defer func() {
		report := loadTestReport{
			StartedAt:           startedAt,
			FinishedAt:          time.Now().UTC(),
			Profile:             config.LoadProfile,
			RampUpStageDuration: config.StageDuration.String(),
			SteadyStateWorkers:  config.SteadyStateWorkers,
			SteadyStateDuration: config.SteadyStateDuration.String(),
			StressWorkers:       config.StressWorkers,
			Stages:              stageReports,
		}

		path, err := writeLoadTestReport(config.ReportDir, report)
		if err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("write load-test report: %w", err))
			return
		}

		fmt.Printf("Load-test report saved: %s\n", path)
	}()

	profiles, err := loadProfiles(config)
	if err != nil {
		return err
	}

	for _, profile := range profiles {
		fmt.Printf("\n========== Load profile: %s ==========\n", profile.name)

		results, err := runWorkerStages(
			ctx,
			client,
			fixtures,
			profile.name,
			profile.workerStages,
			profile.stageDuration,
		)
		stageReports = append(stageReports, results...)
		if err != nil {
			return fmt.Errorf("load profile %q: %w", profile.name, err)
		}
	}

	return nil
}

func loadProfiles(config Config) ([]loadProfile, error) {
	rampUp := loadProfile{
		name:          "ramp-up",
		workerStages:  workloadStages,
		stageDuration: config.StageDuration,
	}
	steadyState := loadProfile{
		name:          "steady-state",
		workerStages:  []int{config.SteadyStateWorkers},
		stageDuration: config.SteadyStateDuration,
	}
	stress := loadProfile{
		name:          "stress",
		workerStages:  config.StressWorkers,
		stageDuration: config.StageDuration,
	}

	switch config.LoadProfile {
	case "standard":
		return []loadProfile{rampUp, steadyState}, nil
	case "ramp-up":
		return []loadProfile{rampUp}, nil
	case "steady-state":
		return []loadProfile{steadyState}, nil
	case "stress":
		return []loadProfile{stress}, nil
	case "all":
		return []loadProfile{rampUp, steadyState, stress}, nil
	default:
		return nil, fmt.Errorf("unsupported load profile %q", config.LoadProfile)
	}
}

func writeLoadTestReport(
	directory string,
	report loadTestReport,
) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create report directory: %w", err)
	}

	filename := "load-test-" + report.StartedAt.Format("20060102T150405.000000000Z") + ".json"
	path := filepath.Join(directory, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("create report file: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(report)
	closeErr := file.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write report file: %w", err)
	}

	return path, nil
}
