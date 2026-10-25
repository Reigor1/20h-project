package core_http_server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
)

func TestRouteWithMiddleware(t *testing.T) {
	events := []string{}
	route := Route{
		Method: http.MethodGet,
		Path:   "/items",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "handler")
			if r.URL.Path != "/items" {
				t.Errorf("handler path = %q, want /items", r.URL.Path)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("accepted"))
		},
		Middleware: []core_http_middleware.Middleware{
			recordRouteEvent(&events, "first"),
			recordRouteEvent(&events, "second"),
		},
	}

	request := httptest.NewRequest(http.MethodGet, "/items", nil)
	response := httptest.NewRecorder()
	route.WithMiddleware().ServeHTTP(response, request)

	wantEvents := []string{
		"first before",
		"second before",
		"handler",
		"second after",
		"first after",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("middleware events = %v, want %v", events, wantEvents)
	}
	if response.Code != http.StatusAccepted || response.Body.String() != "accepted" {
		t.Fatalf("response = %d %q, want 202 accepted", response.Code, response.Body.String())
	}
}

func TestRouteWithMiddlewareWithoutMiddleware(t *testing.T) {
	called := false
	route := Route{
		Method: http.MethodGet,
		Path:   "/",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		},
	}
	response := httptest.NewRecorder()
	route.WithMiddleware().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/", nil),
	)

	if !called || response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf(
			"called=%v response=%d %q; want handler called and empty 204 response",
			called,
			response.Code,
			response.Body.String(),
		)
	}
}

func recordRouteEvent(events *[]string, name string) core_http_middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*events = append(*events, name+" before")
			next.ServeHTTP(w, r)
			*events = append(*events, name+" after")
		})
	}
}
