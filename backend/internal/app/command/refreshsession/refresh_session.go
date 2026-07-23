// Package refreshsession exchanges a refresh token for a fresh credential pair.
package refreshsession

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/sessions"
)

// Command presents a refresh token.
type Command struct {
	RefreshToken string
}

// Handler executes the RefreshSession command.
type Handler struct {
	issuer *sessions.Issuer
}

// NewHandler constructs a RefreshSession handler.
func NewHandler(issuer *sessions.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Handle rotates the token. Reuse detection and session revocation live in the
// issuer so refresh, sign-in and federated sign-in cannot drift apart on the
// rule that matters most here.
func (h *Handler) Handle(ctx context.Context, cmd Command) (sessions.Tokens, error) {
	return h.issuer.Rotate(ctx, cmd.RefreshToken)
}
