package core_http_server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/Reigor1/20h-project/docs"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type HTTPServer struct {
	mux    *http.ServeMux
	config Config
	log    *core_logger.Logger

	middleware           []core_http_middleware.Middleware
	apiRouteInstrumenter APIRouteInstrumenter
}

func NewHTTPServer(
	config Config,
	log *core_logger.Logger,
	middleware ...core_http_middleware.Middleware,
) *HTTPServer {
	return &HTTPServer{
		mux:        http.NewServeMux(),
		config:     config,
		log:        log,
		middleware: middleware,
	}
}

func (s *HTTPServer) RegisterAPIRouteInstrumenter(instrumenter APIRouteInstrumenter) {
	s.apiRouteInstrumenter = instrumenter
}

func (s *HTTPServer) RegisterAPIRouters(routers ...*APIVersionRouter) {
	for _, router := range routers {
		prefix := "/api/" + string(router.apiVersion)

		var instrumenter APIRouteInstrumenter
		if s.apiRouteInstrumenter != nil {
			instrumenter = func(method, path string, handler http.Handler) http.Handler {
				return s.apiRouteInstrumenter(method, prefix+path, handler)
			}
		}

		s.mux.Handle(
			prefix+"/",
			http.StripPrefix(prefix, router.WithMiddleware(instrumenter)),
		)
	}
}

func (s *HTTPServer) RegisterSwagger() {
	swaggerHandler := core_http_middleware.ChainMiddleware(
		httpSwagger.Handler(
			httpSwagger.URL("/swagger/doc.json"),
			httpSwagger.DefaultModelsExpandDepth(-1),
		),
		core_http_middleware.Trace(),
	)

	s.mux.Handle("/swagger/", swaggerHandler)

	swaggerDocHandler := http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
		},
	)

	s.mux.Handle(
		"/swagger/doc.json",
		core_http_middleware.ChainMiddleware(
			swaggerDocHandler,
			core_http_middleware.Trace(),
		),
	)
}

func (s *HTTPServer) Run(ctx context.Context) error {
	mux := core_http_middleware.ChainMiddleware(s.mux, s.middleware...)

	server := http.Server{
		Addr:    s.config.Addr,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP server: %w", err)
	}

	serveResult := make(chan error, 1)

	go func() {
		serveResult <- server.Serve(listener)
	}()

	var (
		serveErr      error
		serveReturned bool
		runErr        error
	)

	select {
	case serveErr = <-serveResult:
		serveReturned = true

	case <-ctx.Done():
		if s.log != nil {
			s.log.Warn("shutdown HTTP server...")
		}
	}

	if serveReturned &&
		serveErr != nil &&
		!errors.Is(serveErr, http.ErrServerClosed) {
		runErr = fmt.Errorf("serve HTTP server: %w", serveErr)
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		s.config.ShutdownTimeout,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(
			runErr,
			fmt.Errorf("shutdown HTTP server: %w", err),
		)

		if closeErr := server.Close(); closeErr != nil {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("force close HTTP server: %w", closeErr),
			)
		}
	}

	if !serveReturned {
		serveErr = <-serveResult

		if serveErr != nil &&
			!errors.Is(serveErr, http.ErrServerClosed) {
			runErr = errors.Join(
				runErr,
				fmt.Errorf("serve HTTP server: %w", serveErr),
			)
		}
	}

	if runErr != nil {
		return runErr
	}

	if s.log != nil {
		s.log.Warn("HTTP server stopped")
	}

	return nil
}
