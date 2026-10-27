package core_http_response

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	"github.com/Reigor1/20h-project/internal/testutil"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestJSONResponse(t *testing.T) {
	tests := []struct {
		name         string
		responseBody any
		decode       func([]byte) (any, error)
	}{
		{
			name:         "ordinary response",
			responseBody: map[string]string{"name": "Ada"},
			decode: func(body []byte) (any, error) {
				var decoded map[string]string
				err := json.Unmarshal(body, &decoded)
				return decoded, err
			},
		},
		{
			name: "error response",
			responseBody: ErrorResponse{
				Error:   "invalid argument",
				Message: "Invalid input.",
			},
			decode: func(body []byte) (any, error) {
				var decoded ErrorResponse
				err := json.Unmarshal(body, &decoded)
				return decoded, err
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler := NewHTTPResponseHandler(testutil.NewNopLogger(), response)

			handler.JSONResponse(testCase.responseBody, http.StatusAccepted)

			if response.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
			}
			testutil.AssertJSONContentType(t, response)
			decoded, err := testCase.decode(response.Body.Bytes())
			if err != nil {
				t.Fatalf("decode JSON response: %v", err)
			}
			wantJSON, err := json.Marshal(testCase.responseBody)
			if err != nil {
				t.Fatalf("marshal expected response: %v", err)
			}
			gotJSON, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("marshal decoded response: %v", err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("decoded response = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestJSONResponseLogsEncodingError(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()
	response := httptest.NewRecorder()
	handler := NewHTTPResponseHandler(logger, response)

	handler.JSONResponse(make(chan int), http.StatusAccepted)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}
	assertSingleResponseLog(t, logs, "write HTTP response", zapcore.ErrorLevel)
}

func TestJSONResponseLogsWriterError(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()
	writer := &errorResponseWriter{header: make(http.Header)}
	handler := NewHTTPResponseHandler(logger, writer)

	handler.JSONResponse(map[string]string{"status": "ok"}, http.StatusAccepted)

	assertSingleResponseLog(t, logs, "write HTTP response", zapcore.ErrorLevel)
}

func TestNoContentResponse(t *testing.T) {
	response := httptest.NewRecorder()
	handler := NewHTTPResponseHandler(testutil.NewNopLogger(), response)

	handler.NoContentResponse()

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", response.Body.String())
	}
}

func TestErrorResponse(t *testing.T) {
	unknownError := errors.New("database unavailable")
	tests := []struct {
		name       string
		err        error
		statusCode int
		logLevel   zapcore.Level
	}{
		{
			name:       "invalid argument",
			err:        core_errors.ErrInvalidArgument,
			statusCode: http.StatusBadRequest,
			logLevel:   zapcore.WarnLevel,
		},
		{
			name:       "not found",
			err:        core_errors.ErrNotFound,
			statusCode: http.StatusNotFound,
			logLevel:   zapcore.DebugLevel,
		},
		{
			name:       "wrapped not found",
			err:        fmt.Errorf("load resource: %w", core_errors.ErrNotFound),
			statusCode: http.StatusNotFound,
			logLevel:   zapcore.DebugLevel,
		},
		{
			name:       "conflict",
			err:        core_errors.ErrConflict,
			statusCode: http.StatusConflict,
			logLevel:   zapcore.WarnLevel,
		},
		{
			name:       "unknown error",
			err:        unknownError,
			statusCode: http.StatusInternalServerError,
			logLevel:   zapcore.ErrorLevel,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			const message = "request could not be completed"
			logger, logs := testutil.NewObservedLogger()
			response := httptest.NewRecorder()
			handler := NewHTTPResponseHandler(logger, response)

			handler.ErrorResponse(testCase.err, message)

			if response.Code != testCase.statusCode {
				t.Errorf("status = %d, want %d", response.Code, testCase.statusCode)
			}
			var body ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Error != testCase.err.Error() || body.Message != message {
				t.Errorf("body = %+v, want error %q and message %q", body, testCase.err, message)
			}
			assertLoggedError(t, logs, message, testCase.logLevel, testCase.err.Error())
		})
	}
}

func TestPanicResponse(t *testing.T) {
	const (
		panicValue = "unexpected state"
		message    = "An unexpected panic occurred."
	)
	logger, logs := testutil.NewObservedLogger()
	response := httptest.NewRecorder()
	handler := NewHTTPResponseHandler(logger, response)

	handler.PanicResponse(panicValue, message)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	var body ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode panic response: %v", err)
	}
	wantError := "unexpected panic: " + panicValue
	if body.Error != wantError || body.Message != message {
		t.Fatalf("panic response = %+v, want error %q and message %q", body, wantError, message)
	}
	assertLoggedError(t, logs, message, zapcore.ErrorLevel, wantError)
}

func assertSingleResponseLog(t *testing.T, logs *observer.ObservedLogs, message string, wantLevel zapcore.Level) {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	if len(entries) != 1 || entries[0].Level != wantLevel {
		t.Fatalf("logs = %v, want one %s entry for %q", entries, wantLevel, message)
	}
}

func assertLoggedError(t *testing.T, logs *observer.ObservedLogs, message string, wantLevel zapcore.Level, wantError string) {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	if len(entries) != 1 {
		t.Fatalf("matching log count = %d, want 1", len(entries))
	}
	if entries[0].Level != wantLevel {
		t.Errorf("log level = %s, want %s", entries[0].Level, wantLevel)
	}
	if got := entries[0].ContextMap()["error"]; got != wantError {
		t.Errorf("logged error = %v, want %q", got, wantError)
	}
}

type errorResponseWriter struct {
	header http.Header
}

func (w *errorResponseWriter) Header() http.Header {
	return w.header
}

func (*errorResponseWriter) WriteHeader(int) {}

func (*errorResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("response writer failed")
}
