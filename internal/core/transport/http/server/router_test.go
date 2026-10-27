package core_http_server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Reigor1/20h-project/internal/testutil"
)

func TestAPIVersionRouterRegistersMethodsAndPaths(t *testing.T) {
	var receivedPath string
	router := NewAPIVersionRouter(ApiVersion1)
	router.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/items/{id}",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			_, _ = w.Write([]byte(r.PathValue("id")))
		},
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/items/42", nil),
	)
	if response.Code != http.StatusOK || response.Body.String() != "42" || receivedPath != "/items/42" {
		t.Fatalf(
			"GET response = %d %q, handler path = %q",
			response.Code,
			response.Body.String(),
			receivedPath,
		)
	}

	wrongMethodResponse := httptest.NewRecorder()
	router.ServeHTTP(
		wrongMethodResponse,
		httptest.NewRequest(http.MethodPost, "/items/42", nil),
	)
	if wrongMethodResponse.Code != http.StatusMethodNotAllowed {
		t.Errorf(
			"POST status = %d, want %d",
			wrongMethodResponse.Code,
			http.StatusMethodNotAllowed,
		)
	}

	missingRouteResponse := httptest.NewRecorder()
	router.ServeHTTP(
		missingRouteResponse,
		httptest.NewRequest(http.MethodGet, "/missing", nil),
	)
	if missingRouteResponse.Code != http.StatusNotFound {
		t.Errorf(
			"missing route status = %d, want %d",
			missingRouteResponse.Code,
			http.StatusNotFound,
		)
	}
}

func TestAPIVersionRouterMiddlewareAndVersions(t *testing.T) {
	events := []string{}
	versionOne := NewAPIVersionRouter(
		ApiVersion1,
		testutil.RecordMiddleware(&events, "router"),
	)
	versionTwo := NewAPIVersionRouter(ApiVersion2)
	versionOne.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/version",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			events = append(events, "v1 handler")
			_, _ = w.Write([]byte("v1"))
		},
	})
	versionTwo.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/version",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			events = append(events, "v2 handler")
			_, _ = w.Write([]byte("v2"))
		},
	})

	response := httptest.NewRecorder()
	versionOne.WithMiddleware().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/version", nil),
	)
	if response.Body.String() != "v1" {
		t.Fatalf("v1 response body = %q, want v1", response.Body.String())
	}
	if want := []string{"router before", "v1 handler", "router after"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("middleware events = %v, want %v", events, want)
	}

	events = nil
	response = httptest.NewRecorder()
	versionTwo.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/version", nil),
	)
	if response.Body.String() != "v2" || !reflect.DeepEqual(events, []string{"v2 handler"}) {
		t.Fatalf("v2 response body = %q, events = %v", response.Body.String(), events)
	}
}
