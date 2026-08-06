package handlers

import (
	"net/http"
	"strings"

	"github.com/fgrzl/mux"
)

// streamWriterKey carries the untouched http.ResponseWriter to a streaming
// handler through the request context.
type streamWriterKey struct{}

// StreamPassthroughMiddleware prepares event-stream routes for streaming. It
// must be registered before any middleware that wraps the response writer.
//
// Two things stand between an SSE handler and the wire. The compression
// middleware's gzip writer buffers and exposes no Flush, so events would only
// arrive when the stream closed; it steps aside on its own when the request
// does not accept an encoding, so stripping the header is the whole bypass.
// The logging middleware's status recorder hides Flush too — its wrapper
// neither implements it nor exposes Unwrap for ResponseController to walk — so
// the raw writer is stashed here, while it is still raw, for the handler to
// stream through. Headers and WriteHeader still go through the wrapped writer,
// which is what keeps the access log seeing the stream's status code.
func StreamPassthroughMiddleware() mux.MiddlewareFunc {
	return func(c mux.MutableRouteContext, next mux.HandlerFunc) {
		if strings.HasSuffix(c.Request().URL.Path, "/events") {
			c.Request().Header.Del("Accept-Encoding")
			c.SetContextValue(streamWriterKey{}, c.Response())
		}
		next(c)
	}
}

// streamWriter returns the writer a streaming handler should write through:
// the raw one stashed by StreamPassthroughMiddleware, or the context's own
// writer if that middleware somehow did not run. The fallback keeps this
// total; a nil writer here would panic mid-response.
func streamWriter(c mux.RouteContext) http.ResponseWriter {
	if w, ok := c.Request().Context().Value(streamWriterKey{}).(http.ResponseWriter); ok {
		return w
	}
	return c.Response()
}
