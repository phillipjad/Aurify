// Package resetpassword consumes a reset token and replaces a user's password.
package resetpassword

import (
	"context"
	"errors"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// Command sets a new password using a reset token.
type Command struct {
	Token       string
	NewPassword string
}

// Handler executes the ResetPassword command.
type Handler struct {
	tokens      ports.EmailTokenRepository
	credentials ports.CredentialRepository
	issuer      *sessions.Issuer
}

// NewHandler constructs a ResetPassword handler.
func NewHandler(
	tokens ports.EmailTokenRepository,
	credentials ports.CredentialRepository,
	issuer *sessions.Issuer,
) *Handler {
	return &Handler{tokens: tokens, credentials: credentials, issuer: issuer}
}

// Handle validates the token, stores the new password and signs the user out
// everywhere.
//
// The global sign-out is the point of the whole flow: a reset is the response
// to a possible compromise, so any session an attacker already holds has to die
// with the old password. Leaving them alive would let someone who had already
// signed in keep their access after the owner "fixed" the account.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	if cmd.Token == "" {
		return domain.ErrTokenInvalid
	}
	if len(cmd.NewPassword) < signup.MinPasswordLength {
		return signup.ErrWeakPassword
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
	// A verification token must not be usable to set a password, or anyone who
	// intercepted a signup email could take the account over.
	if stored.Purpose != domain.PurposeResetPassword {
		return domain.ErrTokenInvalid
	}
	if !stored.ConsumedAt.IsZero() || !now.Before(stored.ExpiresAt) {
		return domain.ErrTokenInvalid
	}

	if err := h.tokens.Consume(ctx, hash, now); err != nil {
		return err
	}

	newHash, err := auth.HashPassword(cmd.NewPassword)
	if err != nil {
		return err
	}
	if err := h.credentials.Upsert(ctx, domain.Credential{
		UserID:       stored.UserID,
		PasswordHash: newHash,
	}); err != nil {
		return err
	}

	// Completing a reset proves control of the address, so it also verifies it.
	// This is what lets a user who never clicked the original signup link still
	// reach a usable account.
	if err := h.credentials.SetEmailVerified(ctx, stored.UserID, true); err != nil {
		return err
	}

	return h.issuer.RevokeAll(ctx, stored.UserID)
}
