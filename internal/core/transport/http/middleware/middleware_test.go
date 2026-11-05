package core_http_middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_http_response "github.com/Reigor1/20h-project/internal/core/transport/http/response"
	"github.com/Reigor1/20h-project/internal/testutil"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestChainMiddleware(t *testing.T) {
	events := []string{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		events = append(events, "handler")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})
	wrap := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events = append(events, name+" before")
				next.ServeHTTP(w, r)
				events = append(events, name+" after")
			})
		}
	}
	emptyChainCalled := false
	emptyChain := ChainMiddleware(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			emptyChainCalled = true
		}),
	)
	emptyChain.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	if !emptyChainCalled {
		t.Fatal("empty chain did not invoke its handler")
	}
	singleMiddlewareCalled := false
	singleMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				singleMiddlewareCalled = true
				next.ServeHTTP(w, r)
			},
		)
	}
	singleMiddlewareChain := ChainMiddleware(handler, singleMiddleware)
	singleMiddlewareChain.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	if !singleMiddlewareCalled {
		t.Fatal("single middleware was not called")
	}
	events = nil
	chainedHandler := ChainMiddleware(handler, wrap("first"), wrap("second"))
	chainedHandler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	want := []string{"first before", "second before", "handler", "second after", "first after"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	response := httptest.NewRecorder()
	ChainMiddleware(handler).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	if response.Code != http.StatusCreated || response.Body.String() != "created" {
		t.Fatalf("response = %d %q, want 201 created", response.Code, response.Body.String())
	}
}

func TestRequestID(t *testing.T) {
	t.Run("generated UUID reaches response and downstream", func(t *testing.T) {
		var downstreamID string
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		handler := ChainMiddleware(
			http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				downstreamID = r.Header.Get(requestIDHeader)
			}),
			RequestID(),
		)
		handler.ServeHTTP(recorder, request)
		id := recorder.Header().Get(requestIDHeader)
		if _, err := uuid.Parse(id); err != nil {
			t.Fatalf("generated ID %q is not a UUID: %v", id, err)
		}
		if downstreamID != id {
			t.Fatalf("downstream ID = %q, response ID = %q", downstreamID, id)
		}
	})
	t.Run("provided ID is preserved", func(t *testing.T) {
		const id = "caller-id"
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(requestIDHeader, id)
		recorder := httptest.NewRecorder()
		handler := ChainMiddleware(
			http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get(requestIDHeader); got != id {
					t.Errorf("downstream ID = %q, want %q", got, id)
				}
			}),
			RequestID(),
		)
		handler.ServeHTTP(recorder, request)
		if got := recorder.Header().Get(requestIDHeader); got != id {
			t.Fatalf("response ID = %q, want %q", got, id)
		}
	})
}

func TestCORS(t *testing.T) {
	const allowedOrigin = "http://localhost:5050"
	for _, tc := range []struct {
		name    string
		origin  string
		allowed bool
	}{
		{name: "allowed origin", origin: allowedOrigin, allowed: true},
		{name: "forbidden origin", origin: "https://example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Origin", tc.origin)
			recorder := httptest.NewRecorder()
			handler := ChainMiddleware(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					called = true
				}),
				CORS(),
			)
			handler.ServeHTTP(recorder, request)
			if !called {
				t.Fatal("ordinary request did not reach downstream")
			}
			if tc.allowed {
				if recorder.Header().Get("Access-Control-Allow-Origin") != allowedOrigin ||
					recorder.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PATCH, DELETE, OPTIONS" ||
					recorder.Header().Get("Access-Control-Allow-Headers") != "Content-Type, Authorization" {
					t.Fatalf("CORS headers = %v", recorder.Header())
				}
			} else if recorder.Header().Get("Access-Control-Allow-Origin") != "" ||
				recorder.Header().Get("Access-Control-Allow-Methods") != "" ||
				recorder.Header().Get("Access-Control-Allow-Headers") != "" {
				t.Fatalf("forbidden origin got allow header: %v", recorder.Header())
			}
		})
	}
	for _, tc := range []struct {
		name            string
		origin          string
		wantAllowHeader bool
	}{
		{name: "allowed preflight", origin: allowedOrigin, wantAllowHeader: true},
		{name: "forbidden preflight", origin: "https://example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			request := httptest.NewRequest(http.MethodOptions, "/", nil)
			request.Header.Set("Origin", tc.origin)
			recorder := httptest.NewRecorder()
			handler := ChainMiddleware(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					called = true
				}),
				CORS(),
			)
			handler.ServeHTTP(recorder, request)
			if called || recorder.Code != http.StatusOK {
				t.Fatalf(
					"called=%v status=%d; want false and 200",
					called,
					recorder.Code,
				)
			}
			gotAllowHeader := recorder.Header().Get("Access-Control-Allow-Origin") != ""
			if gotAllowHeader != tc.wantAllowHeader {
				t.Fatalf("allow-origin present=%v, want %v", gotAllowHeader, tc.wantAllowHeader)
			}
		})
	}
}

func TestLoggerAddsRequestFieldsToContext(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()
	request := httptest.NewRequest(http.MethodGet, "/items?limit=5", nil)
	request.Header.Set(requestIDHeader, "request-123")
	handler := ChainMiddleware(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			core_logger.FromContext(r.Context()).Info("downstream")
		}),
		Logger(logger),
	)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	entries := logs.FilterMessage("downstream").All()
	if len(entries) != 1 {
		t.Fatalf("downstream logs = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["request_id"] != "request-123" || fields["url"] != "/items?limit=5" {
		t.Fatalf("logger fields = %v", fields)
	}
}

func TestTraceLogsRequestLifecycle(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()
	request := httptest.NewRequest(http.MethodGet, "/trace", nil)
	request = request.WithContext(core_logger.ToContext(request.Context(), logger))
	handler := ChainMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}),
		Trace(),
	)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if logs.FilterMessage(">>> incoming HTTP request").Len() != 1 {
		t.Fatal("start log missing")
	}
	entries := logs.FilterMessage("<<< done HTTP request").All()
	if len(entries) != 1 {
		t.Fatalf("completion logs = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["status_code"] != int64(http.StatusAccepted) {
		t.Fatalf("status_code = %v, want 202", fields["status_code"])
	}
	if latency, ok := fields["latency"].(time.Duration); !ok || latency < 0 {
		t.Fatalf("latency = %T(%v), want nonnegative duration", fields["latency"], fields["latency"])
	}
}

func TestTraceUsesImplicitOKStatus(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(core_logger.ToContext(request.Context(), logger))
	handler := ChainMiddleware(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		Trace(),
	)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	entries := logs.FilterMessage("<<< done HTTP request").All()
	if len(entries) != 1 || entries[0].ContextMap()["status_code"] != int64(http.StatusOK) {
		t.Fatalf("completion entries = %v, want status 200", entries)
	}
}

func TestPanicMiddleware(t *testing.T) {
	logger, _ := testutil.NewObservedLogger()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(core_logger.ToContext(request.Context(), logger))
	t.Run("normal request passes through", func(t *testing.T) {
		called := false
		recorder := httptest.NewRecorder()
		handler := ChainMiddleware(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				_, _ = w.Write([]byte("ok"))
			}),
			Panic(),
		)
		handler.ServeHTTP(recorder, request)
		if !called || recorder.Body.String() != "ok" {
			t.Fatalf("called=%v body=%q", called, recorder.Body.String())
		}
	})
	t.Run("panic returns project JSON response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler := ChainMiddleware(
			http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("boom")
			}),
			Panic(),
		)
		handler.ServeHTTP(recorder, request)
		var body core_http_response.ErrorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v; body=%q", err, recorder.Body.String())
		}
		if recorder.Code != http.StatusInternalServerError ||
			body.Error != "unexpected panic: boom" ||
			body.Message != "An unexpected panic occurred during the request processing. Please try again later." {
			t.Fatalf("panic response = status %d, %+v", recorder.Code, body)
		}
	})
}

func TestDummyCallsNextBetweenLogs(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(core_logger.ToContext(request.Context(), logger))
	called := false
	handler := ChainMiddleware(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		}),
		Dummy("test"),
	)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !called {
		t.Fatal("downstream handler was not called")
	}
	entries := logs.All()
	if len(entries) != 2 || entries[0].Message != "- > before: test" || entries[1].Message != "< - after: test" {
		t.Fatalf("dummy log events = %v", entries)
	}
}

func TestTraceLogsIncludeActiveHTTPSpanContext(t *testing.T) {
	logger, logs := testutil.NewObservedLogger()

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	t.Cleanup(func() {
		if err := tracerProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	handler := ChainMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			core_logger.FromContext(r.Context()).Info("handler log")
			w.WriteHeader(http.StatusAccepted)
		}),
		Trace(),
	)

	instrumentedHandler := otelhttp.NewHandler(
		handler,
		"GET /trace",
		otelhttp.WithTracerProvider(tracerProvider),
	)

	request := httptest.NewRequest(http.MethodGet, "/trace", nil)
	request = request.WithContext(
		core_logger.ToContext(request.Context(), logger),
	)

	instrumentedHandler.ServeHTTP(
		httptest.NewRecorder(),
		request,
	)

	messages := []string{
		">>> incoming HTTP request",
		"handler log",
		"<<< done HTTP request",
	}

	var traceID string
	var spanID string

	for i, message := range messages {
		entries := logs.FilterMessage(message).All()
		if len(entries) != 1 {
			t.Fatalf(
				"logs for %q = %d, want 1",
				message,
				len(entries),
			)
		}

		fields := entries[0].ContextMap()

		gotTraceID, ok := fields["trace_id"].(string)
		if !ok || len(gotTraceID) != 32 {
			t.Fatalf("trace_id = %v, want 32-character string", fields["trace_id"])
		}

		gotSpanID, ok := fields["span_id"].(string)
		if !ok || len(gotSpanID) != 16 {
			t.Fatalf("span_id = %v, want 16-character string", fields["span_id"])
		}

		if i == 0 {
			traceID = gotTraceID
			spanID = gotSpanID
			continue
		}

		if gotTraceID != traceID {
			t.Errorf("%q trace_id = %s, want %s", message, gotTraceID, traceID)
		}

		if gotSpanID != spanID {
			t.Errorf("%q span_id = %s, want %s", message, gotSpanID, spanID)
		}
	}
}
