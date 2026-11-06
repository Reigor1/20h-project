package core_http_server

import (
	"fmt"
	"net/http"

	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
)

type ApiVersion string

var (
	ApiVersion1 = ApiVersion("v1")
	ApiVersion2 = ApiVersion("v2")
)

// APIRouteInstrumenter wraps one registered API route with instrumentation.
type APIRouteInstrumenter func(method, path string, handler http.Handler) http.Handler

type registeredRoute struct {
	route   Route
	handler http.Handler
}

type APIVersionRouter struct {
	*http.ServeMux
	apiVersion ApiVersion
	middleware []core_http_middleware.Middleware
	routes     []registeredRoute
}

func NewAPIVersionRouter(
	apiVersion ApiVersion,
	middleware ...core_http_middleware.Middleware,
) *APIVersionRouter {
	return &APIVersionRouter{
		ServeMux:   http.NewServeMux(),
		apiVersion: apiVersion,
		middleware: middleware,
	}
}

func (r *APIVersionRouter) RegisterRouts(routes ...Route) {
	for _, route := range routes {
		pattern := fmt.Sprintf("%s %s", route.Method, route.Path)
		handler := route.WithMiddleware()

		r.routes = append(r.routes, registeredRoute{
			route:   route,
			handler: handler,
		})
		r.Handle(pattern, handler)
	}
}

func (r *APIVersionRouter) WithMiddleware(instrumenters ...APIRouteInstrumenter) http.Handler {
	var handler http.Handler = r.ServeMux

	if len(instrumenters) > 0 && instrumenters[0] != nil {
		mux := http.NewServeMux()

		for _, registered := range r.routes {
			pattern := fmt.Sprintf(
				"%s %s",
				registered.route.Method,
				registered.route.Path,
			)
			routeHandler := instrumenters[0](
				registered.route.Method,
				registered.route.Path,
				registered.handler,
			)

			mux.Handle(pattern, routeHandler)
		}

		// Keep routes registered directly on the embedded ServeMux available.
		mux.Handle("/", r.ServeMux)
		handler = mux
	}

	return core_http_middleware.ChainMiddleware(
		handler,
		r.middleware...,
	)
}
