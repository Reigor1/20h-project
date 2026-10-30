package infrastructure_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
	"github.com/Reigor1/20h-project/internal/testutil"
)

func TestRunServesRequestsAndShutsDownGracefully(t *testing.T) {
	address := availableHTTPTestAddress(t)
	events := []string{}
	logger, _ := testutil.NewObservedLogger()
	server := core_http_server.NewHTTPServer(
		core_http_server.Config{
			Addr:            address,
			ShutdownTimeout: 2 * time.Second,
		},
		logger,
		testutil.RecordMiddleware(&events, "server"),
	)
	router := core_http_server.NewAPIVersionRouter(
		core_http_server.ApiVersion1,
		testutil.RecordMiddleware(&events, "router"),
	)
	router.RegisterRouts(core_http_server.Route{
		Method: http.MethodGet,
		Path:   "/ready",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			events = append(events, "handler")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, "ready")
		},
		Middleware: []core_http_middleware.Middleware{testutil.RecordMiddleware(&events, "middleware")},
	})
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

	response, err := waitForHTTPTestResponse(
		t,
		"http://"+address+"/api/v1/ready",
		runResult,
	)
	if err != nil {
		runFinished = true
		if isLoopbackPermissionError(err) {
			t.Skipf("environment does not permit loopback HTTP connections: %v", err)
		}
		t.Fatalf("server failed before becoming ready: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read HTTP response: %v", err)
	}
	if response.StatusCode != http.StatusAccepted || string(body) != "ready" {
		t.Fatalf("HTTP response = %d %q, want 202 ready", response.StatusCode, body)
	}

	wantEvents := []string{
		"server before",
		"router before",
		"middleware before",
		"handler",
		"middleware after",
		"router after",
		"server after",
	}
	if !slices.Equal(events, wantEvents) {
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

	logger, _ := testutil.NewObservedLogger()
	server := core_http_server.NewHTTPServer(
		core_http_server.Config{
			Addr:            listener.Addr().String(),
			ShutdownTimeout: time.Second,
		},
		logger,
	)

	err = server.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want address-in-use error")
	}
	if !strings.Contains(err.Error(), "listen and server HTTP") {
		t.Fatalf("Run() error = %q, want listen error context", err)
	}
}

func availableHTTPTestAddress(t *testing.T) string {
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

func waitForHTTPTestResponse(
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

func isLoopbackPermissionError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "access permissions") ||
		strings.Contains(message, "operation not permitted") ||
		strings.Contains(message, "permission denied")
}
