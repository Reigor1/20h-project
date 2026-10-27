package testutil

import "net/http"

// RecordMiddleware appends before and after events around the wrapped handler.
func RecordMiddleware(events *[]string, name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*events = append(*events, name+" before")
			next.ServeHTTP(w, r)
			*events = append(*events, name+" after")
		})
	}
}
