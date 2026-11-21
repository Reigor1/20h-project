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

// apiResult captures the HTTP status, headers, and decoded response body for an API request.
type apiResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// apiClient provides the base URL and HTTP client used by API end-to-end scenarios.
type apiClient struct {
	httpClient *http.Client
	baseURL    string
}

// request sends an HTTP request, optionally JSON-encodes its body, and decodes the response.
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

// expectedErrorCase describes a request that must produce a particular API error status.
type expectedErrorCase struct {
	name   string
	method string
	path   string
	body   any
	status int
}

// expectedSuccessCase describes a successful JSON request and its expected response.
type expectedSuccessCase struct {
	name   string
	method string
	path   string
	body   any
	status int
	check  func(t *testing.T, result apiResult)
}

// errorResponse represents the error envelope returned by the API.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// assertStatus checks the HTTP status returned by an API request and reports request context on failure.
func assertStatus(t *testing.T, result apiResult, method, path string, want int) {
	t.Helper()
	if result.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d; body: %s", method, path, result.StatusCode, want, result.Body)
	}
}

// assertErrorResponse decodes and validates the standard API error response body.
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

// runExpectedErrors executes a table of invalid API requests and checks their expected statuses.
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

// newJSONSuccessCase creates a typed test case for a successful JSON API operation.
func newJSONSuccessCase[Response any, Request any](
	method string,
	status int,
	validate func(t *testing.T, got Response, input *Request),
) func(
	name string,
	path string,
	body *Request,
) expectedSuccessCase {
	return func(
		name string,
		path string,
		body *Request,
	) expectedSuccessCase {
		var requestBody any
		if body != nil {
			requestBody = *body
		}

		return expectedSuccessCase{
			name:   name,
			method: method,
			path:   path,
			body:   requestBody,
			status: status,
			check: func(t *testing.T, result apiResult) {
				t.Helper()

				var got Response
				if err := json.Unmarshal(result.Body, &got); err != nil {
					t.Fatalf(
						"decode response: %v; body: %s",
						err,
						result.Body,
					)
				}

				if validate != nil {
					validate(t, got, body)
				}
			},
		}
	}
}

// func newNoContentSuccessCase(
// 	method string,
// 	status int,
// ) func(name, path string, body any) expectedSuccessCase {
// 	return func(
// 		name string,
// 		path string,
// 		body any,
// 	) expectedSuccessCase {
// 		return expectedSuccessCase{
// 			name:   name,
// 			method: method,
// 			path:   path,
// 			body:   body,
// 			status: status,
// 			check: func(t *testing.T, result apiResult) {
//                 t.Helper()

//                 if len(result.Body) != 0 {
//                     t.Fatalf("expected empty response body, got: %s", result.Body)
//                 }
//             },
// 		}
// 	}
// }

// runExpectedSuccesses executes successful API cases and validates their decoded response bodies.
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

// registerUserCleanup schedules deletion of a test user after the end-to-end test.
func registerUserCleanup(t *testing.T, api *apiClient, userID int) {
	t.Helper()
	t.Cleanup(func() {
		const usersPath = "/api/v1/users"
		path := fmt.Sprintf("%s/%d", usersPath, userID)
		result, err := api.request(http.MethodDelete, path, nil)
		if err != nil {
			t.Errorf("cleanup DELETE %s: %v", path, err)
			return
		}
		if result.StatusCode != http.StatusNoContent &&
			result.StatusCode != http.StatusNotFound {
			t.Errorf(
				"cleanup DELETE %s status = %d, want %d or %d; body: %s",
				path,
				result.StatusCode,
				http.StatusNoContent,
				http.StatusNotFound,
				result.Body,
			)
		}
	})
}

// createUser creates a user through the public API and fails the test if creation is unsuccessful.
func createUser(t *testing.T, api *apiClient, name string, phone *string) userResponse {
	t.Helper()
	const usersPath = "/api/v1/users"
	result, err := api.request(
		http.MethodPost,
		usersPath,
		userRequest{FullName: name, PhoneNumber: phone},
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assertStatus(t, result, http.MethodPost, usersPath, http.StatusCreated)

	var user userResponse
	if err := json.Unmarshal(result.Body, &user); err != nil {
		t.Fatalf("decode created user: %v; body: %s", err, result.Body)
	}
	if user.ID <= 0 {
		t.Fatalf("created user ID = %d, want positive ID", user.ID)
	}
	registerUserCleanup(t, api, user.ID)
	return user
}
