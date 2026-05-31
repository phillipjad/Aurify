// Package handlers contains the mux HTTP handlers for the Aurify API. Handlers
// are thin: they parse input, delegate to a command or query handler on the
// application layer, and map the result (or error) onto an HTTP response.
package handlers

import (
	"errors"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// userIDHeader carries the caller's user id.
//
// SCAFFOLD: real session/JWT auth is not wired yet. fgrzl/mux ships
// authentication middleware (mux.UseAuthentication) that would populate
// c.User(); until that is configured, handlers read the user id from this
// header. See docs/adr/0007 and the backend README.
const userIDHeader = "X-User-ID"

// currentUser extracts the caller's user id from the request.
func currentUser(c mux.RouteContext) string {
	id, _ := c.Headers().String(userIDHeader)
	return id
}

// respondError maps a domain/application error onto an RFC-7807-style response
// using mux's built-in problem helpers.
func respondError(c mux.RouteContext, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.NotFound()
	case errors.Is(err, domain.ErrUnauthorized):
		c.Unauthorized()
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		c.BadRequest("unsupported platform", err.Error())
	default:
		c.ServerError("internal error", err.Error())
	}
}
