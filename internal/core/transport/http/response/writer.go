package core_http_response

import "net/http"

// StatusCodeUninitialized marks a response writer that has not observed WriteHeader.
const StatusCodeUninitialized = -1

// ResponseWriter records the first explicit status code while forwarding writes to the
// underlying net/http response writer.
type ResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// NewResponseWriter wraps w and initializes status tracking as not yet written.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		statusCode:     StatusCodeUninitialized,
	}
}

// WriteHeader forwards the status code and records only the first explicit status.
func (rw *ResponseWriter) WriteHeader(statusCode int) {
	rw.ResponseWriter.WriteHeader(statusCode)

	if rw.statusCode == StatusCodeUninitialized {
		rw.statusCode = statusCode
	}
}

// GetStatusCode returns the first written status, or HTTP 200 when none was explicit.
func (rw *ResponseWriter) GetStatusCode() int {
	if rw.statusCode == StatusCodeUninitialized {
		return http.StatusOK
	}

	return rw.statusCode
}
