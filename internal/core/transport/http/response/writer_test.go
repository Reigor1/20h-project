package core_http_response

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewResponseWriterStartsUninitialized(t *testing.T) {
	response := httptest.NewRecorder()
	writer := NewResponseWriter(response)

	if writer.statusCode != StatusCodeUninitialized {
		t.Fatalf("initial status = %d, want %d", writer.statusCode, StatusCodeUninitialized)
	}
	if got := writer.GetStatusCodeOrPanic(); got != http.StatusOK {
		t.Fatalf("GetStatusCodeOrPanic() = %d, want %d", got, http.StatusOK)
	}
}

func TestResponseWriterWriteHeaderStoresStatus(t *testing.T) {
	response := httptest.NewRecorder()
	writer := NewResponseWriter(response)

	writer.WriteHeader(http.StatusCreated)

	if got := writer.GetStatusCodeOrPanic(); got != http.StatusCreated {
		t.Fatalf("wrapped status = %d, want %d", got, http.StatusCreated)
	}
	if response.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusCreated)
	}
}

func TestResponseWriterWriteBodyUsesImplicitOKStatus(t *testing.T) {
	response := httptest.NewRecorder()
	writer := NewResponseWriter(response)

	if _, err := writer.Write([]byte("body")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if got := writer.GetStatusCodeOrPanic(); got != http.StatusOK {
		t.Errorf("wrapped status = %d, want %d", got, http.StatusOK)
	}
	if response.Code != http.StatusOK || response.Body.String() != "body" {
		t.Fatalf("HTTP response = %d %q, want 200 body", response.Code, response.Body.String())
	}
}

func TestResponseWriterRepeatedWriteHeaderKeepsHTTPFirstStatus(t *testing.T) {
	response := httptest.NewRecorder()
	writer := NewResponseWriter(response)

	writer.WriteHeader(http.StatusCreated)
	writer.WriteHeader(http.StatusTeapot)

	if response.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, want first status %d", response.Code, http.StatusCreated)
	}
	if got := writer.GetStatusCodeOrPanic(); got != response.Code {
		t.Errorf(
			"wrapped status = %d, want it to match first HTTP status %d",
			got,
			response.Code,
		)
	}
}
