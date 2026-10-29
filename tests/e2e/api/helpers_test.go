package e2e_api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type apiResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type apiClient struct {
	httpClient *http.Client
	baseURL    string
}

func (c apiClient) request(method, path string, body any) (apiResult, error) {
	var requestBody io.Reader
	if body != nil {
		encodedBody, err := json.Marshal(body)
		if err != nil {
			return apiResult{}, fmt.Errorf("encode %s %s body: %w", method, path, err)
		}
		requestBody = bytes.NewReader(encodedBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, requestBody)
	if err != nil {
		return apiResult{}, fmt.Errorf("create %s %s request: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return apiResult{}, fmt.Errorf("send %s %s request: %w", method, path, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResult{}, fmt.Errorf("read %s %s response: %w", method, path, err)
	}

	return apiResult{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: responseBody}, nil
}

type expectedErrorCase struct {
	name   string
	method string
	path   string
	body   any
	status int
}

type expectedSuccessCase struct {
	name   string
	method string
	path   string
	body   any
	status int
	check  func(t *testing.T, result apiResult)
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func assertStatus(t *testing.T, result apiResult, method, path string, want int) {
	t.Helper()
	if result.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d; body: %s", method, path, result.StatusCode, want, result.Body)
	}
}

func assertErrorResponse(t *testing.T, result apiResult) errorResponse {
	t.Helper()
	var got errorResponse
	if err := json.Unmarshal(result.Body, &got); err != nil {
		t.Fatalf("decode error response: %v; body: %s", err, result.Body)
	}
	if strings.TrimSpace(got.Error) == "" || strings.TrimSpace(got.Message) == "" {
		t.Fatalf("error response = %+v, want nonempty error and message", got)
	}
	return got
}

func runExpectedErrors(t *testing.T, client apiClient, cases []expectedErrorCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.request(tc.method, tc.path, tc.body)
			if err != nil {
				t.Fatalf("request %s %s: %v", tc.method, tc.path, err)
			}
			assertStatus(t, result, tc.method, tc.path, tc.status)
			assertErrorResponse(t, result)
		})
	}
}

func runExpectedSuccesses(t *testing.T, client apiClient, cases []expectedSuccessCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.request(tc.method, tc.path, tc.body)
			if err != nil {
				t.Fatalf("request %s %s: %v", tc.method, tc.path, err)
			}
			assertStatus(t, result, tc.method, tc.path, tc.status)
			if tc.check != nil {
				tc.check(t, result)
			}
		})
	}
}
