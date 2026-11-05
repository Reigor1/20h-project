package infrastructure_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
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
	if !strings.Contains(err.Error(), "listen HTTP server") {
		t.Fatalf(
			"Run() error = %q, want listen HTTP server context",
			err,
		)
	}
}

func TestRunWaitsForInFlightRequestBeforeReturning(t *testing.T) {
	address := availableHTTPTestAddress(t)
	logger, logs := testutil.NewObservedLogger()

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})

	var releaseOnce sync.Once

	release := func() {
		releaseOnce.Do(func() {
			close(releaseHandler)
		})
	}

	server := core_http_server.NewHTTPServer(
		core_http_server.Config{
			Addr:            address,
			ShutdownTimeout: 2 * time.Second,
		},
		logger,
	)

	router := core_http_server.NewAPIVersionRouter(
		core_http_server.ApiVersion1,
	)

	router.RegisterRouts(core_http_server.Route{
		Method: http.MethodGet,
		Path:   "/in-flight",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			close(handlerStarted)

			// Simulate a long-running request.
			<-releaseHandler

			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, "completed")
		},
	})

	server.RegisterAPIRouters(router)

	ctx, cancel := context.WithCancel(context.Background())

	runResult := make(chan error, 1)

	go func() {
		runResult <- server.Run(ctx)
	}()

	runFinished := false

	t.Cleanup(func() {
		cancel()
		release()

		if runFinished {
			return
		}

		select {
		case err := <-runResult:
			if err != nil {
				t.Errorf("Run() during cleanup: %v", err)
			}

		case <-time.After(3 * time.Second):
			t.Error("Run() did not stop during test cleanup")
		}
	})

	// Wait until the HTTP server starts listening.
	var probeConn net.Conn

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout(
			"tcp",
			address,
			50*time.Millisecond,
		)

		if err == nil {
			probeConn = conn
			break
		}

		select {
		case err := <-runResult:
			runFinished = true
			t.Fatalf("HTTP server stopped before becoming ready: %v", err)

		default:
		}

		time.Sleep(10 * time.Millisecond)
	}

	if probeConn == nil {
		t.Fatal("HTTP server did not become ready")
	}

	if err := probeConn.Close(); err != nil {
		t.Fatalf("close readiness connection: %v", err)
	}

	// Start an HTTP request that will remain in-flight.
	requestResult := make(chan error, 1)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	go func() {
		response, err := client.Get(
			"http://" + address + "/api/v1/in-flight",
		)
		if err != nil {
			requestResult <- err
			return
		}
		defer response.Body.Close()

		body, err := io.ReadAll(response.Body)
		if err != nil {
			requestResult <- err
			return
		}

		if response.StatusCode != http.StatusAccepted ||
			string(body) != "completed" {
			requestResult <- fmt.Errorf(
				"response = %d %q, want 202 completed",
				response.StatusCode,
				body,
			)
			return
		}

		requestResult <- nil
	}()

	// Ensure that the request has reached the handler.
	select {
	case <-handlerStarted:

	case err := <-runResult:
		runFinished = true
		t.Fatalf("HTTP server stopped before handler started: %v", err)

	case <-time.After(3 * time.Second):
		t.Fatal("in-flight handler was not started")
	}

	// Trigger graceful shutdown while the handler is still running.
	cancel()

	// Wait until the HTTP server begins graceful shutdown.
	shutdownTicker := time.NewTicker(10 * time.Millisecond)
	defer shutdownTicker.Stop()

	shutdownTimeout := time.NewTimer(time.Second)
	defer shutdownTimeout.Stop()

	for logs.FilterMessage("shutdown HTTP server...").Len() == 0 {
		select {
		case err := <-runResult:
			runFinished = true
			t.Fatalf(
				"Run() returned before the active request completed: %v",
				err,
			)

		case <-shutdownTicker.C:

		case <-shutdownTimeout.C:
			t.Fatal("HTTP server did not begin graceful shutdown")
		}
	}

	// Run() must wait until the active handler completes.
	select {
	case err := <-runResult:
		runFinished = true
		t.Fatalf(
			"Run() returned while the handler was still active: %v",
			err,
		)

	case <-time.After(50 * time.Millisecond):
	}

	// Allow the handler to finish.
	release()

	select {
	case err := <-requestResult:
		if err != nil {
			t.Fatalf("in-flight HTTP request failed: %v", err)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("in-flight HTTP request did not finish")
	}

	// Wait until the HTTP server shuts down completely.
	select {
	case err := <-runResult:
		runFinished = true

		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after the in-flight request completed")
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
