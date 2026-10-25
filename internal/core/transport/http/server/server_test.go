package core_http_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/docs"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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
		recordRouterEvent(&events, "router"),
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
	if !equalStringSlices(events, wantEvents) {
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
	server := NewHTTPServer(Config{}, &core_logger.Logger{Logger: zap.NewNop()})
	server.RegisterSwagger()
	response := httptest.NewRecorder()
	server.mux.ServeHTTP(
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

func TestRunServesRequestsAndShutsDownGracefully(t *testing.T) {
	address := availableLocalAddress(t)
	events := []string{}
	logger := observedServerLogger()
	server := NewHTTPServer(
		Config{Addr: address, ShutdownTimeout: 2 * time.Second},
		logger,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events = append(events, "server before")
				next.ServeHTTP(w, r)
				events = append(events, "server after")
			})
		},
	)
	router := testRouter(&events)
	router.middleware = []core_http_middleware.Middleware{
		recordRouterEvent(&events, "router"),
	}
	server.RegisterAPIRouters(router)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runResult := make(chan error, 1)
	go func() {
		runResult <- server.Run(ctx)
	}()
	runFinished := false
	t.Cleanup(func() {
		cancel()
		if runFinished {
			return
		}
		select {
		case <-runResult:
		case <-time.After(3 * time.Second):
			t.Error("Run() did not stop during test cleanup")
		}
	})

	response, err := waitForHTTPResponse(
		t,
		"http://"+address+"/api/v1/ready",
		runResult,
	)
	if err != nil {
		runFinished = true
		if isLocalSocketPermissionError(err) {
			t.Skipf("sandbox does not permit loopback HTTP connections: %v", err)
		}
		t.Fatalf("server failed before becoming ready: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read HTTP response: %v", err)
	}
	if response.StatusCode != http.StatusAccepted || string(body) != "ready" {
		t.Fatalf("HTTP response = %d %q, want 202 ready", response.StatusCode, body)
	}
	wantEvents := []string{
		"server before",
		"router before",
		"handler",
		"router after",
		"server after",
	}
	if !equalStringSlices(events, wantEvents) {
		t.Fatalf("middleware events = %v, want %v", events, wantEvents)
	}

	cancel()
	select {
	case err := <-runResult:
		runFinished = true
		if err != nil {
			t.Fatalf("Run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not finish after context cancellation")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local port: %v", err)
	}
	defer listener.Close()

	server := NewHTTPServer(
		Config{Addr: listener.Addr().String(), ShutdownTimeout: time.Second},
		observedServerLogger(),
	)
	err = server.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want address-in-use error")
	}
	if !strings.Contains(err.Error(), "listen and server HTTP") {
		t.Fatalf("Run() error = %q, want listen error context", err)
	}
}

func testRouter(events *[]string) *APIVersionRouter {
	router := NewAPIVersionRouter(ApiVersion1)
	router.RegisterRouts(Route{
		Method: http.MethodGet,
		Path:   "/ready",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			*events = append(*events, "handler")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, "ready")
		},
	})
	return router
}

func availableLocalAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release local address: %v", err)
	}
	return address
}

func waitForHTTPResponse(
	t *testing.T,
	url string,
	runResult <-chan error,
) (*http.Response, error) {
	t.Helper()

	client := http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	pollInterval := time.NewTicker(10 * time.Millisecond)
	defer pollInterval.Stop()
	var lastError error
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			return response, nil
		}
		lastError = err
		select {
		case runErr := <-runResult:
			return nil, runErr
		case <-pollInterval.C:
		}
	}
	return nil, fmt.Errorf("server did not become ready at %s: %w", url, lastError)
}

func isLocalSocketPermissionError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "access permissions") ||
		strings.Contains(message, "operation not permitted") ||
		strings.Contains(message, "permission denied")
}

func observedServerLogger() *core_logger.Logger {
	core, _ := observer.New(zapcore.DebugLevel)
	return &core_logger.Logger{Logger: zap.New(core)}
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
