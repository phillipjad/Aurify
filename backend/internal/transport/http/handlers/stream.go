package handlers

import (
	"net/http"
	"strings"

	"github.com/fgrzl/mux"
)

// streamWriterKey carries the untouched http.ResponseWriter to a streaming
// handler through the request context.
type streamWriterKey struct{}

// StreamPassthroughMiddleware prepares /events routes for streaming, and must
// run before anything that wraps the response writer. Neither mux's compression
// writer (buffers, no Flush) nor its logging recorder (hides Flush, no Unwrap)
// can stream, so the header is stripped and the still-raw writer is stashed.
func StreamPassthroughMiddleware() mux.MiddlewareFunc {
	return func(c mux.MutableRouteContext, next mux.HandlerFunc) {
		if strings.HasSuffix(c.Request().URL.Path, "/events") {
			c.Request().Header.Del("Accept-Encoding")
			c.SetContextValue(streamWriterKey{}, c.Response())
		}
		next(c)
	}
}

// streamWriter returns the raw writer stashed above, falling back to the
// context's own so a missing middleware degrades rather than panicking.
func streamWriter(c mux.RouteContext) http.ResponseWriter {
	if w, ok := c.Request().Context().Value(streamWriterKey{}).(http.ResponseWriter); ok {
		return w
	}
	return c.Response()
}
