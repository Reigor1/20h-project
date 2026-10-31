package core_observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestInstrumentHTTPRouteSetsFullRoutePattern(t *testing.T) {
	provider := sdkmetric.NewMeterProvider()
	defer func() {
		_ = provider.Shutdown(t.Context())
	}()

	observability := &Observability{meterProvider: provider}

	var gotPattern string
	handler := observability.InstrumentHTTPRoute(
		http.MethodGet,
		"/api/v1/users/{id}",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPattern = r.Pattern
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if gotPattern != "/api/v1/users/{id}" {
		t.Fatalf("request pattern = %q, want %q", gotPattern, "/api/v1/users/{id}")
	}

	if rec.Code != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
