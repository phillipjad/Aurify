// Package verifyemail consumes an emailed verification token and marks the
// address proven.
package verifyemail

import (
	"context"
	"errors"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// Command presents a verification token.
type Command struct {
	Token string
}

// Handler executes the VerifyEmail command.
type Handler struct {
	tokens      ports.EmailTokenRepository
	credentials ports.CredentialRepository
}

// NewHandler constructs a VerifyEmail handler.
func NewHandler(tokens ports.EmailTokenRepository, credentials ports.CredentialRepository) *Handler {
	return &Handler{tokens: tokens, credentials: credentials}
}

// Handle validates and consumes the token, then marks the address verified.
//
// Every rejection is domain.ErrTokenInvalid: an unknown, expired, already-used
// or wrong-purpose token look identical from outside, so the endpoint cannot be
// used to probe which tokens exist.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	if cmd.Token == "" {
		return domain.ErrTokenInvalid
	}

	hash := auth.HashToken(cmd.Token)
	stored, err := h.tokens.Find(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrTokenInvalid
		}
		return err
	}

	now := time.Now().UTC()
	// A reset token must not double as a verification token, or a password-reset
	// link would silently prove ownership of an address it never checked.
	if stored.Purpose != domain.PurposeVerifyEmail {
		return domain.ErrTokenInvalid
	}
	if !stored.ConsumedAt.IsZero() || !now.Before(stored.ExpiresAt) {
		return domain.ErrTokenInvalid
	}

	// Consume first. The guarded UPDATE is the single point that decides who
	// wins if the same link is opened twice at once, so nothing may take effect
	// before it succeeds.
	if err := h.tokens.Consume(ctx, hash, now); err != nil {
		return err
	}
	return h.credentials.SetEmailVerified(ctx, stored.UserID, true)
}
