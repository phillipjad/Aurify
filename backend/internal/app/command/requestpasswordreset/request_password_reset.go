// Package requestpasswordreset issues a password-reset link.
package requestpasswordreset

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// ResetTokenTTL is short: a reset link is a full account takeover if it leaks,
// so it should be usable for about as long as it takes to read the email.
const ResetTokenTTL = time.Hour

// Command asks for a reset link.
type Command struct {
	Email string
}

// Handler executes the RequestPasswordReset command.
type Handler struct {
	users   ports.UserRepository
	tokens  ports.EmailTokenRepository
	mailer  ports.EmailSender
	baseURL string
}

// NewHandler constructs a RequestPasswordReset handler.
func NewHandler(
	users ports.UserRepository,
	tokens ports.EmailTokenRepository,
	mailer ports.EmailSender,
	baseURL string,
) *Handler {
	return &Handler{users: users, tokens: tokens, mailer: mailer, baseURL: baseURL}
}

// Handle sends a reset link when the address exists.
//
// It returns nil for an unknown address as well as a known one. The endpoint is
// unauthenticated and enumerable by design otherwise: a distinguishable
// response would turn it into a free "is this address registered here" oracle.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	email, err := signup.NormalizeEmail(cmd.Email)
	if err != nil {
		return nil
	}

	user, err := h.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}

	// Issuing a new link invalidates any earlier one, so a forwarded or
	// intercepted older email stops working.
	if err := h.tokens.DeleteForUser(ctx, user.ID, domain.PurposeResetPassword); err != nil {
		return err
	}

	token, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := h.tokens.Create(ctx, domain.EmailToken{
		Hash:      hash,
		UserID:    user.ID,
		Purpose:   domain.PurposeResetPassword,
		ExpiresAt: now.Add(ResetTokenTTL),
		CreatedAt: now,
	}); err != nil {
		return err
	}

	link := strings.TrimRight(h.baseURL, "/") + "/reset-password?token=" + token
	body := "Reset your Aurify password:\n\n" + link +
		"\n\nThe link expires in one hour and can be used once. If you did not ask for this, ignore this message and your password will stay as it is."
	return h.mailer.Send(ctx, email, "Reset your Aurify password", body)
}
