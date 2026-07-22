// Package handlers contains the mux HTTP handlers for the Aurify API. Handlers
// are thin: they parse input, delegate to a command or query handler on the
// application layer, and map the result (or error) onto an HTTP response.
package handlers

import (
	"errors"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// currentUser extracts the caller's user id from the authenticated principal.
//
// The principal is populated by the authentication middleware from a verified
// access token, so this value is trustworthy. It replaced an X-User-ID request
// header, which was a complete authentication bypass: any client could name any
// user and be believed.
func currentUser(c mux.RouteContext) string {
	principal := c.User()
	if principal == nil {
		return ""
	}
	return principal.Subject()
}

// respondError maps a domain/application error onto an RFC-7807-style response
// using mux's built-in problem helpers.
//
// The authentication cases are deliberately coarse. Sign-in answers every
// failure with the same 401 and the same wording, so the response cannot be
// used to tell "no such account" from "wrong password"; the sign-in handler
// also spends the same CPU either way, so the timing does not tell either.
func respondError(c mux.RouteContext, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.NotFound()
	case errors.Is(err, domain.ErrInvalidCredentials):
		c.JSON(401, map[string]string{
			"title":  "Sign-in failed",
			"detail": "Invalid email or password.",
		})
	case errors.Is(err, domain.ErrEmailNotVerified):
		// Only reachable after a correct password, so it discloses nothing an
		// attacker did not already have, and the user genuinely needs to know.
		c.JSON(403, map[string]string{
			"title":  "Email not verified",
			"detail": "Check your inbox for the verification link before signing in.",
		})
	case errors.Is(err, domain.ErrEmailTaken):
		c.Conflict("Email already registered", "An account with that email already exists. Try signing in instead.")
	case errors.Is(err, signup.ErrWeakPassword):
		c.BadRequest("Password too short", err.Error())
	case errors.Is(err, signup.ErrInvalidEmail):
		c.BadRequest("Invalid email", "Enter a valid email address.")
	case errors.Is(err, domain.ErrTokenInvalid):
		c.BadRequest("Link is invalid or expired", "Request a new link and try again.")
	case errors.Is(err, domain.ErrTokenReused), errors.Is(err, domain.ErrSessionInvalid):
		// Reuse means the refresh token leaked and the session has just been
		// revoked. The client is told only that it must sign in again.
		c.JSON(401, map[string]string{
			"title":  "Session ended",
			"detail": "Your session is no longer valid. Sign in again.",
		})
	case errors.Is(err, domain.ErrUnauthorized):
		c.Unauthorized()
	case errors.Is(err, domain.ErrUnsupportedPlatform):
		c.BadRequest("unsupported platform", err.Error())
	default:
		c.ServerError("internal error", err.Error())
	}
}
