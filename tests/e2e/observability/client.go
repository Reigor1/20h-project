package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxResponseBodyBytes defines a shared limit or value used by the test workflow.
const maxResponseBodyBytes = 1 << 20

// apiClient stores the data required by the corresponding test scenario.
type apiClient struct {
	httpClient *http.Client
	baseURL    string
}

// apiResponse stores the data required by the corresponding test scenario.
type apiResponse struct {
	StatusCode int
	Body       []byte
}

// newAPIClient constructs the test client or fixture required by the scenario.
func newAPIClient(config Config) *apiClient {
	maxConnections := config.SteadyStateWorkers
	if len(workloadStages) > 0 && workloadStages[len(workloadStages)-1] > maxConnections {
		maxConnections = workloadStages[len(workloadStages)-1]
	}
	for _, workers := range config.StressWorkers {
		if workers > maxConnections {
			maxConnections = workers
		}
	}
	if maxConnections < 1 {
		maxConnections = 1
	}

	transport := &http.Transport{
		MaxIdleConns:          maxConnections,
		MaxIdleConnsPerHost:   maxConnections,
		MaxConnsPerHost:       maxConnections,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
	}

	return &apiClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   config.RequestTimeout,
		},
		baseURL: config.BaseURL,
	}
}

// do sends an HTTP request as part of the observability test scenario.
func (c *apiClient) do(
	ctx context.Context,
	method string,
	path string,
	body any,
) (apiResponse, error) {
	var requestBody io.Reader

	if body != nil {
		encodedBody, err := json.Marshal(body)
		if err != nil {
			return apiResponse{}, fmt.Errorf(
				"encode %s %s body: %w",
				method,
				path,
				err,
			)
		}

		requestBody = bytes.NewReader(encodedBody)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		method,
		c.baseURL+path,
		requestBody,
	)
	if err != nil {
		return apiResponse{}, fmt.Errorf(
			"create %s %s request: %w",
			method,
			path,
			err,
		)
	}

	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return apiResponse{}, fmt.Errorf(
			"send %s %s request: %w",
			method,
			path,
			err,
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(response.Body, maxResponseBodyBytes+1),
	)
	if err != nil {
		return apiResponse{}, fmt.Errorf(
			"read %s %s response: %w",
			method,
			path,
			err,
		)
	}

	if len(responseBody) > maxResponseBodyBytes {
		return apiResponse{}, fmt.Errorf(
			"%s %s response exceeds maximum allowed size",
			method,
			path,
		)
	}

	return apiResponse{
		StatusCode: response.StatusCode,
		Body:       responseBody,
	}, nil
}

// expectStatus supports setup, execution, or verification of the corresponding test scenario.
func expectStatus(
	response apiResponse,
	method string,
	path string,
	expectedStatus int,
) error {
	if response.StatusCode == expectedStatus {
		return nil
	}

	body := strings.TrimSpace(string(response.Body))
	if len(body) > 300 {
		body = body[:300] + "..."
	}

	return fmt.Errorf(
		"%s %s: status = %d, want %d; body: %s",
		method,
		path,
		response.StatusCode,
		expectedStatus,
		body,
	)
}

// decodeJSON supports setup, execution, or verification of the corresponding test scenario.
func decodeJSON(response apiResponse, destination any) error {
	if err := json.Unmarshal(response.Body, destination); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
