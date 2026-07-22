// Package signin authenticates an email/password pair and starts a session.
package signin

import (
	"context"
	"errors"

	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// Command authenticates a user.
type Command struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

// Handler executes the SignIn command.
type Handler struct {
	users       ports.UserRepository
	credentials ports.CredentialRepository
	issuer      *sessions.Issuer
}

// NewHandler constructs a SignIn handler.
func NewHandler(
	users ports.UserRepository,
	credentials ports.CredentialRepository,
	issuer *sessions.Issuer,
) *Handler {
	return &Handler{users: users, credentials: credentials, issuer: issuer}
}

// Handle verifies the credentials and issues a session.
//
// Every failure path returns domain.ErrInvalidCredentials, and every path that
// does not find a usable password still performs an Argon2id derivation before
// answering. Both matter: the shared error stops the response body from
// distinguishing "no such account" from "wrong password", and the decoy hash
// stops the response *time* from doing the same, since a real verification
// costs ~19 MiB of work that an early return would skip.
func (h *Handler) Handle(ctx context.Context, cmd Command) (sessions.Tokens, error) {
	email, err := signup.NormalizeEmail(cmd.Email)
	if err != nil {
		auth.VerifyDecoy(cmd.Password)
		return sessions.Tokens{}, domain.ErrInvalidCredentials
	}

	user, err := h.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			auth.VerifyDecoy(cmd.Password)
			return sessions.Tokens{}, domain.ErrInvalidCredentials
		}
		return sessions.Tokens{}, err
	}

	cred, err := h.credentials.FindByUser(ctx, user.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// A federated-only account. Saying so would reveal that the address
			// is registered and how it signs in, so it looks like any other
			// failure.
			auth.VerifyDecoy(cmd.Password)
			return sessions.Tokens{}, domain.ErrInvalidCredentials
		}
		return sessions.Tokens{}, err
	}

	if err := auth.VerifyPassword(cred.PasswordHash, cmd.Password); err != nil {
		return sessions.Tokens{}, domain.ErrInvalidCredentials
	}

	// Verification is required before the first sign-in. It is what makes the
	// federated linking rule safe: an unverified account could have been created
	// by someone who does not control the address.
	if !user.EmailVerified {
		return sessions.Tokens{}, domain.ErrEmailNotVerified
	}

	// The plaintext is in hand and correct, so this is the only moment we can
	// cheaply upgrade a hash stored under weaker parameters.
	if auth.NeedsRehash(cred.PasswordHash) {
		if upgraded, herr := auth.HashPassword(cmd.Password); herr == nil {
			_ = h.credentials.Upsert(ctx, domain.Credential{UserID: user.ID, PasswordHash: upgraded})
		}
	}

	return h.issuer.Issue(ctx, user.ID, sessions.Context{UserAgent: cmd.UserAgent, IP: cmd.IP})
}
