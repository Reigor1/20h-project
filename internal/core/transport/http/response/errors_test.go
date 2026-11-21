package core_http_response

import (
	"encoding/json"
	"testing"
)

// TestErrorResponseJSONRoundTrip verifies that the public error response preserves its fields in JSON.
func TestErrorResponseJSONRoundTrip(t *testing.T) {
	response := ErrorResponse{
		Error:   "invalid argument",
		Message: "The request is invalid.",
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal ErrorResponse: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode ErrorResponse fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("JSON fields = %v, want only error and message", fields)
	}
	if _, ok := fields["error"]; !ok {
		t.Fatalf("JSON fields = %v, missing error", fields)
	}
	if _, ok := fields["message"]; !ok {
		t.Fatalf("JSON fields = %v, missing message", fields)
	}

	var decoded ErrorResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if decoded != response {
		t.Fatalf("decoded response = %+v, want %+v", decoded, response)
	}
}
