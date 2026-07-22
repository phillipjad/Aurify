// Package signout ends a session.
package signout

import (
	"context"
	"errors"

	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Command ends one session, or every session for a user.
type Command struct {
	SessionID string
	UserID    string
	// Everywhere revokes all of the user's sessions rather than just this one.
	Everywhere bool
}

// Handler executes the SignOut command.
type Handler struct {
	issuer *sessions.Issuer
}

// NewHandler constructs a SignOut handler.
func NewHandler(issuer *sessions.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Handle revokes the session.
//
// Revoking an already-gone session is not an error: sign-out must be idempotent
// so a client that retries, or one whose session expired in flight, still ends
// up signed out rather than seeing a failure it cannot act on.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	if cmd.Everywhere {
		if cmd.UserID == "" {
			return domain.ErrUnauthorized
		}
		return h.issuer.RevokeAll(ctx, cmd.UserID)
	}
	if cmd.SessionID == "" {
		return nil
	}
	if err := h.issuer.Revoke(ctx, cmd.SessionID); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}
