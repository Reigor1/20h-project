package core_http_server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/docs"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
	"github.com/Reigor1/20h-project/internal/testutil"
	"go.uber.org/zap"
)

func TestNewHTTPServer(t *testing.T) {
	config := Config{
		Addr:            "127.0.0.1:5050",
		ShutdownTimeout: 3 * time.Second,
	}
	logger := &core_logger.Logger{Logger: zap.NewNop()}
	middleware := core_http_middleware.Middleware(func(next http.Handler) http.Handler {
		return next
	})

	server := NewHTTPServer(config, logger, middleware)

	if server.config != config {
		t.Fatalf("server config = %+v, want %+v", server.config, config)
	}
	if server.log != logger {
		t.Fatal("server logger does not match the supplied logger")
	}
	if len(server.middleware) != 1 || server.middleware[0] == nil {
		t.Fatalf("server middleware = %v, want the supplied middleware", server.middleware)
	}
	if server.mux == nil {
		t.Fatal("server mux is nil")
	}
}

func TestRegisterAPIRoutersStripsPrefixAndAppliesRouterMiddleware(t *testing.T) {
	events := []string{}
	router := NewAPIVersionRouter(
		ApiVersion1,
		testutil.RecordMiddleware(&events, "router"),
	)
	router.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/items/{id}",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "handler")
			_, _ = io.WriteString(w, r.URL.Path+":"+r.PathValue("id"))
		},
	})
	server := NewHTTPServer(Config{}, &core_logger.Logger{Logger: zap.NewNop()})
	server.RegisterAPIRouters(router)

	response := httptest.NewRecorder()
	server.mux.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil),
	)

	if response.Code != http.StatusOK || response.Body.String() != "/items/42:42" {
		t.Fatalf("response = %d %q, want 200 /items/42:42", response.Code, response.Body.String())
	}
	wantEvents := []string{"router before", "handler", "router after"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("router events = %v, want %v", events, wantEvents)
	}
}

func TestRegisterAPIRoutersKeepsVersionsSeparate(t *testing.T) {
	versionOne := NewAPIVersionRouter(ApiVersion1)
	versionOne.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/version",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "v1")
		},
	})
	versionTwo := NewAPIVersionRouter(ApiVersion2)
	versionTwo.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/version",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "v2")
		},
	})
	server := NewHTTPServer(Config{}, &core_logger.Logger{Logger: zap.NewNop()})
	server.RegisterAPIRouters(versionOne, versionTwo)

	for _, testCase := range []struct {
		path     string
		wantBody string
	}{
		{path: "/api/v1/version", wantBody: "v1"},
		{path: "/api/v2/version", wantBody: "v2"},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.mux.ServeHTTP(
				response,
				httptest.NewRequest(http.MethodGet, testCase.path, nil),
			)
			if response.Code != http.StatusOK || response.Body.String() != testCase.wantBody {
				t.Fatalf(
					"response = %d %q, want 200 %q",
					response.Code,
					response.Body.String(),
					testCase.wantBody,
				)
			}
		})
	}
}

func TestRegisterSwagger(t *testing.T) {
	logger := &core_logger.Logger{Logger: zap.NewNop()}
	server := NewHTTPServer(
		Config{},
		logger,
		core_http_middleware.CORS(),
		core_http_middleware.RequestID(),
		core_http_middleware.Logger(logger),
		core_http_middleware.Panic(),
	)
	server.RegisterSwagger()

	handler := core_http_middleware.ChainMiddleware(server.mux, server.middleware...)
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil),
	)

	if response.Code != http.StatusOK {
		t.Fatalf("swagger status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("swagger Content-Type = %q, want application/json", got)
	}
	if response.Body.Len() == 0 {
		t.Fatal("swagger document is empty")
	}

	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode swagger document: %v", err)
	}
	if len(document) == 0 {
		t.Fatal("decoded swagger document is empty")
	}
	if got, want := strings.TrimSpace(response.Body.String()), strings.TrimSpace(docs.SwaggerInfo.ReadDoc()); got != want {
		t.Fatal("served swagger document does not match SwaggerInfo.ReadDoc()")
	}
}

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
