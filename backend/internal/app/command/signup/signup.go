// Package signup registers a new email/password account and sends the address
// verification link.
package signup

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// MinPasswordLength follows NIST SP 800-63B: length is the only composition
// rule worth enforcing. Complexity requirements push people towards predictable
// substitutions and write-it-down behaviour without adding real entropy.
const MinPasswordLength = 8

// VerifyTokenTTL bounds how long a verification link stays usable.
const VerifyTokenTTL = 24 * time.Hour

// ErrWeakPassword is returned when the password is too short. It is safe to
// surface: it describes the submitted value, not any stored account.
var ErrWeakPassword = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// ErrInvalidEmail is returned when the address is not parseable.
var ErrInvalidEmail = errors.New("email address is not valid")

// Command registers an account.
type Command struct {
	Email       string
	Password    string
	DisplayName string
}

// Handler executes the Signup command.
type Handler struct {
	users       ports.UserRepository
	credentials ports.CredentialRepository
	tokens      ports.EmailTokenRepository
	mailer      ports.EmailSender
	baseURL     string
}

// NewHandler constructs a Signup handler. baseURL is the public origin used to
// build the verification link.
func NewHandler(
	users ports.UserRepository,
	credentials ports.CredentialRepository,
	tokens ports.EmailTokenRepository,
	mailer ports.EmailSender,
	baseURL string,
) *Handler {
	return &Handler{users: users, credentials: credentials, tokens: tokens, mailer: mailer, baseURL: baseURL}
}

// Handle registers the account and emails a verification link.
//
// When the address is already registered it returns nil without touching the
// existing account. The caller therefore answers identically whether or not the
// address was free, which is what stops signup from being used to enumerate
// accounts. The real owner is not disturbed, and someone who genuinely owns the
// address can still recover it through password reset.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	email, err := NormalizeEmail(cmd.Email)
	if err != nil {
		return err
	}
	if len(cmd.Password) < MinPasswordLength {
		return ErrWeakPassword
	}

	if _, err := h.users.FindByEmail(ctx, email); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	hash, err := auth.HashPassword(cmd.Password)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	user := &domain.User{
		Email:       email,
		DisplayName: strings.TrimSpace(cmd.DisplayName),
		Connections: map[domain.DSPPlatform]domain.DSPConnection{},
		CreatedAt:   now,
	}
	// Save assigns the id when it is empty.
	if err := h.users.Save(ctx, user); err != nil {
		return err
	}
	if err := h.credentials.Upsert(ctx, domain.Credential{UserID: user.ID, PasswordHash: hash}); err != nil {
		return err
	}

	return SendVerification(ctx, h.tokens, h.mailer, h.baseURL, user.ID, email)
}

// SendVerification issues a fresh verification token and emails the link. It is
// exported so a "resend verification" path reuses exactly this logic.
func SendVerification(
	ctx context.Context,
	tokens ports.EmailTokenRepository,
	mailer ports.EmailSender,
	baseURL, userID, email string,
) error {
	// Drop any outstanding verification tokens so an older link cannot be used
	// after a newer one was requested.
	if err := tokens.DeleteForUser(ctx, userID, domain.PurposeVerifyEmail); err != nil {
		return err
	}

	token, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := tokens.Create(ctx, domain.EmailToken{
		Hash:      hash,
		UserID:    userID,
		Purpose:   domain.PurposeVerifyEmail,
		ExpiresAt: now.Add(VerifyTokenTTL),
		CreatedAt: now,
	}); err != nil {
		return err
	}

	link := strings.TrimRight(baseURL, "/") + "/verify-email?token=" + token
	body := "Confirm your email address to finish setting up Aurify:\n\n" + link +
		"\n\nThe link expires in 24 hours. If you did not create an account, ignore this message."
	return mailer.Send(ctx, email, "Verify your Aurify email address", body)
}

// NormalizeEmail trims and lowercases an address, and checks it parses.
//
// Normalizing before storing is what makes the unique index meaningful: without
// it "User@Example.com" and "user@example.com" are two accounts, and either
// could be used to shadow the other during federated linking.
func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidEmail
	}
	return addr.Address, nil
}
