package handlers

import (
	"net/http"

	"github.com/fgrzl/mux"
)

// CSRFMiddleware rejects state-changing requests that are authenticated by
// cookie but do not echo the CSRF token.
//
// It is registered centrally rather than checked inside each mutating handler,
// because a per-handler check is one someone can forget on the next route they
// add, and the failure is silent.
//
// Only cookie-authenticated requests are checked. A request carrying no access
// cookie has no ambient authority to abuse: either it is anonymous, or it
// authenticated with a bearer token that a cross-site attacker cannot cause the
// browser to attach.
func CSRFMiddleware(cookies *CookieWriter, exempt map[string]bool) mux.MiddlewareFunc {
	return func(c mux.MutableRouteContext, next mux.HandlerFunc) {
		req := c.Request()
		if req == nil || !isStateChanging(req.Method) {
			next(c)
			return
		}
		if exempt[req.URL.Path] {
			next(c)
			return
		}
		if cookies.AccessToken(c) == "" {
			next(c)
			return
		}
		if !CheckCSRF(c) {
			c.JSON(http.StatusForbidden, map[string]string{
				"title":  "CSRF check failed",
				"detail": "Missing or invalid CSRF token.",
			})
			return
		}
		next(c)
	}
}

// isStateChanging reports whether a method may modify state. GET, HEAD and
// OPTIONS are excluded because they are supposed to be safe; any handler that
// mutates on a GET has a bigger problem than CSRF.
func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
