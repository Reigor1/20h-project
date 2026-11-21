package testutil

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	core_http_response "github.com/Reigor1/20h-project/internal/core/transport/http/response"
)

// AssertJSONContentType checks that an HTTP JSON response declares application/json.
// AssertJSONContentType fails the test unless the response declares application/json.
func AssertJSONContentType(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

// AssertJSONStatus checks the response status and JSON content type.
// AssertJSONStatus checks the expected status code and JSON content type.
func AssertJSONStatus(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantStatus int,
) {
	t.Helper()

	if response.Code != wantStatus {
		t.Fatalf(
			"status = %d, want %d; body = %q",
			response.Code,
			wantStatus,
			response.Body.String(),
		)
	}

	AssertJSONContentType(t, response)
}

// AssertErrorResponse checks the status and required fields of a JSON error response.
// AssertErrorResponse validates the status and checks that the decoded error payload
// contains both its error identifier and human-readable message.
func AssertErrorResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantStatus int,
) {
	t.Helper()
	AssertJSONStatus(t, response, wantStatus)

	var body core_http_response.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v; body = %q", err, response.Body.String())
	}

	if body.Error == "" || body.Message == "" {
		t.Fatalf("error response = %+v, want error and message fields", body)
	}
}
