// Package federatedsignin turns a set of claims already proven to come from an
// identity provider into an Aurify session, creating or linking the local
// account as the rules in docs/adr/0011-authentication-and-sessions.md require.
//
// It deliberately knows nothing about OAuth, PKCE or Google. The transport layer
// completes the provider handshake and hands the verified claims in, so the
// linking rules — the part with the security consequences — can be tested
// without a network.
package federatedsignin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Command carries the claims the provider asserted about the account. Every
// field here is provider-supplied and must already have been verified by the
// caller; nothing in this package re-checks a signature.
type Command struct {
	Provider domain.IdentityProvider
	// Subject is the provider's stable id for the account, and is the only
	// value this package keys on. Addresses can be changed or reassigned at the
	// provider, so keying on one would let a recycled address inherit an account.
	Subject string
	Email   string
	// EmailVerified is the provider's assertion that the user controls the
	// address. It is the hinge of the whole linking rule.
	EmailVerified bool
	DisplayName   string
	UserAgent     string
	IP            string
}

// Handler executes the FederatedSignIn command.
type Handler struct {
	users      ports.UserRepository
	identities ports.IdentityRepository
	issuer     *sessions.Issuer
}

// NewHandler constructs a FederatedSignIn handler.
func NewHandler(
	users ports.UserRepository,
	identities ports.IdentityRepository,
	issuer *sessions.Issuer,
) *Handler {
	return &Handler{users: users, identities: identities, issuer: issuer}
}

// Handle resolves the provider claims to a user and issues a session.
//
// There are exactly three outcomes, in this order:
//
//  1. The (provider, subject) pair is already linked. Sign that user in. The
//     address is not consulted, so changing it at the provider does not fork a
//     second account.
//  2. No link, but the address matches a local account. Link the two only if
//     that account has verified its address; otherwise refuse.
//  3. No link and no local account. Create one, already verified, and link it.
func (h *Handler) Handle(ctx context.Context, cmd Command) (sessions.Tokens, error) {
	if cmd.Provider == "" || strings.TrimSpace(cmd.Subject) == "" {
		return sessions.Tokens{}, domain.ErrUnauthorized
	}

	// An address the provider will not vouch for is worth nothing to us: it can
	// neither be matched against a local account nor used to seed a new one, and
	// treating it as proof is exactly the escalation the linking rule exists to
	// stop.
	if !cmd.EmailVerified {
		return sessions.Tokens{}, domain.ErrEmailNotVerified
	}
	email, err := signup.NormalizeEmail(cmd.Email)
	if err != nil {
		return sessions.Tokens{}, domain.ErrEmailNotVerified
	}

	identity, err := h.identities.Find(ctx, cmd.Provider, cmd.Subject)
	switch {
	case err == nil:
		return h.issue(ctx, identity.UserID, cmd)
	case !errors.Is(err, domain.ErrNotFound):
		return sessions.Tokens{}, err
	}

	user, err := h.users.FindByEmail(ctx, email)
	switch {
	case err == nil:
		// The provider has vouched for this address, so telling the caller that a
		// local account exists discloses nothing they could not already confirm:
		// they demonstrably control the mailbox.
		if !user.EmailVerified {
			return sessions.Tokens{}, domain.ErrLinkRequiresVerification
		}
	case errors.Is(err, domain.ErrNotFound):
		user = &domain.User{
			Email:       email,
			DisplayName: strings.TrimSpace(cmd.DisplayName),
			// Verified on the provider's assertion, which we required above. A new
			// federated account has no password, so leaving it unverified would
			// strand it behind a verification flow it can never complete.
			EmailVerified: true,
			Connections:   map[domain.DSPPlatform]domain.DSPConnection{},
			CreatedAt:     time.Now().UTC(),
		}
		// Create, not Save: Save does not write EmailVerified, so going through
		// it left provider-verified accounts sitting at false in the database
		// and tripping the linking rule above on a later visit.
		// Create assigns the id when it is empty.
		if err := h.users.Create(ctx, user); err != nil {
			return sessions.Tokens{}, err
		}
	default:
		return sessions.Tokens{}, err
	}

	if err := h.identities.Upsert(ctx, domain.Identity{
		Provider:  cmd.Provider,
		Subject:   cmd.Subject,
		UserID:    user.ID,
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return sessions.Tokens{}, err
	}
	return h.issue(ctx, user.ID, cmd)
}

func (h *Handler) issue(ctx context.Context, userID string, cmd Command) (sessions.Tokens, error) {
	return h.issuer.Issue(ctx, userID, sessions.Context{UserAgent: cmd.UserAgent, IP: cmd.IP})
}
