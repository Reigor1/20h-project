package core_http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterAPIRoutersInstrumentsBusinessRoutes(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := NewHTTPServer(Config{}, nil)
	server.RegisterAPIRouteInstrumenter(
		func(method, path string, next http.Handler) http.Handler {
			gotMethod = method
			gotPath = path
			return next
		},
	)

	router := NewAPIVersionRouter(ApiVersion1)
	router.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/users/{id}",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	server.RegisterAPIRouters(router)

	if gotMethod != http.MethodGet {
		t.Fatalf("instrumenter method = %q, want %q", gotMethod, http.MethodGet)
	}

	if gotPath != "/api/v1/users/{id}" {
		t.Fatalf("instrumenter path = %q, want %q", gotPath, "/api/v1/users/{id}")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
	rec := httptest.NewRecorder()

	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
