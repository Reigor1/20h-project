package testutil

import (
	"net/http/httptest"
	"testing"
)

// AssertJSONContentType checks that an HTTP JSON response declares application/json.
func AssertJSONContentType(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}
