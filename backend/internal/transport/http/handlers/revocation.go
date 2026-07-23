package handlers

import (
	"context"
	"net/http"

	"github.com/fgrzl/mux"
)

// SessionCheck reports whether a session is still usable. It is satisfied by
// sessions.Issuer.Verify.
type SessionCheck func(ctx context.Context, sessionID string) error

// SessionRevocationMiddleware rejects requests whose session has been revoked
// or has expired, even when their access token is still cryptographically
// valid.
//
// Without it, signing out does not take effect until the access token expires.
// A token minted moments before sign-out keeps working for the remainder of its
// lifetime, which on a shared machine means "sign out" does not sign the user
// out. Refresh-token reuse has the same problem: the session is revoked, but
// the attacker's current access token outlives the revocation.
//
// The cost is one indexed primary-key lookup per authenticated request. That is
// a smaller price than it first appears: every protected endpoint in this API
// already queries PostgreSQL at least once, so this does not turn a
// database-free request into a database-bound one.
//
// It runs after the authentication middleware, so the principal is populated by
// the time it executes. Anonymous routes are skipped, which is what keeps
// sign-in and refresh reachable when no session exists yet.
func SessionRevocationMiddleware(check SessionCheck) mux.MiddlewareFunc {
	return func(c mux.MutableRouteContext, next mux.HandlerFunc) {
		principal := c.User()
		if principal == nil || principal.Subject() == "" {
			// Anonymous route, or a route the authentication middleware skipped.
			next(c)
			return
		}

		sessionID := principal.CustomClaimValue(SessionClaim)
		if sessionID == "" {
			// A token carrying an identity but no session cannot be checked, so
			// it is refused rather than trusted. Reaching this means a token was
			// minted by something that is not our issuer.
			c.Unauthorized()
			return
		}

		if err := check(c, sessionID); err != nil {
			// Deliberately the same answer for a revoked session, an expired one
			// and a lookup failure: the client's only useful next step is to
			// sign in again, and distinguishing the cases tells an attacker
			// whether a session id was real.
			c.JSON(http.StatusUnauthorized, map[string]string{
				"title":  "Session ended",
				"detail": "Your session is no longer valid. Sign in again.",
			})
			return
		}
		next(c)
	}
}
