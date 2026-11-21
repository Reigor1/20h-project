package core_http_middleware

import "net/http"

// Middleware wraps an HTTP handler with cross-cutting request behavior.
type Middleware func(http.Handler) http.Handler

// ChainMiddleware applies middleware in the listed execution order: the first middleware
// is outermost and runs before the remaining middleware on the request path.
func ChainMiddleware(
	h http.Handler,
	m ...Middleware,
) http.Handler {
	if len(m) == 0 {
		return h
	}

	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}

	return h
}
