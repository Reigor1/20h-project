package core_http_server

import (
	"net/http"

	core_http_middleware "github.com/Reigor1/20h-project/internal/core/transport/http/middleware"
)

// Route describes one method-and-path endpoint, its handler, and route-specific middleware.
type Route struct {
	Method     string
	Path       string
	Handler    http.HandlerFunc
	Middleware []core_http_middleware.Middleware
}

// WithMiddleware builds the route handler with its middleware in the configured order.
func (r *Route) WithMiddleware() http.Handler {
	return core_http_middleware.ChainMiddleware(
		r.Handler,
		r.Middleware...,
	)
}
