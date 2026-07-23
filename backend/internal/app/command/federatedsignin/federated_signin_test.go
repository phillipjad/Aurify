package federatedsignin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

type fakeUsers struct {
	byID    map[string]*domain.User
	byEmail map[string]*domain.User
	saved   []*domain.User
	nextID  int
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*domain.User{}, byEmail: map[string]*domain.User{}}
}

func (f *fakeUsers) add(u *domain.User) {
	f.byID[u.ID] = u
	f.byEmail[u.Email] = u
}

func (f *fakeUsers) Save(_ context.Context, u *domain.User) error {
	if u.ID == "" {
		f.nextID++
		u.ID = "user-" + string(rune('0'+f.nextID))
	}
	f.saved = append(f.saved, u)
	f.add(u)
	return nil
}

func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

type fakeIdentities struct {
	byKey   map[string]domain.Identity
	upserts []domain.Identity
}

func newFakeIdentities() *fakeIdentities {
	return &fakeIdentities{byKey: map[string]domain.Identity{}}
}

func key(p domain.IdentityProvider, subject string) string { return string(p) + "|" + subject }

func (f *fakeIdentities) Find(
	_ context.Context,
	p domain.IdentityProvider,
	subject string,
) (domain.Identity, error) {
	if i, ok := f.byKey[key(p, subject)]; ok {
		return i, nil
	}
	return domain.Identity{}, domain.ErrNotFound
}

func (f *fakeIdentities) Upsert(_ context.Context, i domain.Identity) error {
	f.upserts = append(f.upserts, i)
	f.byKey[key(i.Provider, i.Subject)] = i
	return nil
}

func (f *fakeIdentities) ListByUser(_ context.Context, userID string) ([]domain.Identity, error) {
	out := []domain.Identity{}
	for _, i := range f.byKey {
		if i.UserID == userID {
			out = append(out, i)
		}
	}
	return out, nil
}

// fakeSessions is the minimum ports.SessionRepository the issuer needs to mint a
// session; rotation is exercised in the sessions package's own tests.
type fakeSessions struct{ created []domain.Session }

func (f *fakeSessions) Create(_ context.Context, s domain.Session, _ domain.RefreshToken) error {
	f.created = append(f.created, s)
	return nil
}

func (f *fakeSessions) FindSession(context.Context, string) (domain.Session, error) {
	return domain.Session{}, domain.ErrNotFound
}

func (f *fakeSessions) FindRefreshToken(context.Context, []byte) (domain.RefreshToken, error) {
	return domain.RefreshToken{}, domain.ErrNotFound
}

func (f *fakeSessions) Rotate(context.Context, []byte, domain.RefreshToken) error { return nil }
func (f *fakeSessions) Touch(context.Context, string, time.Time) error            { return nil }
func (f *fakeSessions) Revoke(context.Context, string, time.Time) error           { return nil }
func (f *fakeSessions) RevokeAllForUser(context.Context, string, time.Time) error { return nil }

func newHandler(t *testing.T) (*Handler, *fakeUsers, *fakeIdentities, *fakeSessions) {
	t.Helper()

	seed, err := auth.GenerateKeySeed()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	priv, err := auth.ParsePrivateKeySeed(seed)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	signer, err := auth.NewSigner(priv, "aurify", "aurify-api")
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	users := newFakeUsers()
	identities := newFakeIdentities()
	store := &fakeSessions{}
	issuer := sessions.NewIssuer(store, signer, sessions.TTL{
		Access:  15 * time.Minute,
		Refresh: 30 * 24 * time.Hour,
		Session: 90 * 24 * time.Hour,
	})
	return NewHandler(users, identities, issuer), users, identities, store
}

func googleCommand() Command {
	return Command{
		Provider:      domain.ProviderGoogle,
		Subject:       "google-sub-123",
		Email:         "Person@Example.com",
		EmailVerified: true,
		DisplayName:   "A Person",
	}
}

// A known identity signs straight in. The email is deliberately different from
// the one stored at link time: the subject is the key, so a changed address at
// the provider must not fork a second account.
func TestExistingIdentitySignsInRegardlessOfEmailChange(t *testing.T) {
	h, users, identities, store := newHandler(t)
	users.add(&domain.User{ID: "user-1", Email: "person@example.com", EmailVerified: true})
	identities.byKey[key(domain.ProviderGoogle, "google-sub-123")] = domain.Identity{
		Provider: domain.ProviderGoogle,
		Subject:  "google-sub-123",
		UserID:   "user-1",
		Email:    "person@example.com",
	}

	cmd := googleCommand()
	cmd.Email = "renamed@example.com"

	tokens, err := h.Handle(t.Context(), cmd)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("no access token issued")
	}
	if len(store.created) != 1 || store.created[0].UserID != "user-1" {
		t.Fatalf("session created for %+v, want user-1", store.created)
	}
	if len(users.saved) != 0 {
		t.Fatal("an existing identity must not create or rewrite a user")
	}
}

// No identity and no local account: this is a first-time Google user.
func TestUnknownIdentityCreatesVerifiedAccount(t *testing.T) {
	h, users, identities, store := newHandler(t)

	if _, err := h.Handle(t.Context(), googleCommand()); err != nil {
		t.Fatalf("sign in: %v", err)
	}

	if len(users.saved) != 1 {
		t.Fatalf("saved %d users, want 1", len(users.saved))
	}
	created := users.saved[0]
	if created.Email != "person@example.com" {
		t.Fatalf("stored email = %q, want it normalised", created.Email)
	}
	// Google asserted email_verified, and we refuse the sign-in otherwise, so
	// the address is proven and the account must not be stuck behind a
	// verification link it will never receive a password for.
	if !created.EmailVerified {
		t.Fatal("an account created from a verified federated identity must be verified")
	}
	if len(identities.upserts) != 1 || identities.upserts[0].UserID != created.ID {
		t.Fatalf("identity link = %+v, want one linked to %q", identities.upserts, created.ID)
	}
	if len(store.created) != 1 {
		t.Fatalf("created %d sessions, want 1", len(store.created))
	}
}

// The linking rule from ADR 0011: auto-link only when the local account is
// already verified.
func TestLinksToVerifiedLocalAccount(t *testing.T) {
	h, users, identities, store := newHandler(t)
	users.add(&domain.User{ID: "user-1", Email: "person@example.com", EmailVerified: true})

	if _, err := h.Handle(t.Context(), googleCommand()); err != nil {
		t.Fatalf("sign in: %v", err)
	}

	if len(users.saved) != 0 {
		t.Fatal("linking must not create a second account for the same address")
	}
	if len(identities.upserts) != 1 || identities.upserts[0].UserID != "user-1" {
		t.Fatalf("identity link = %+v, want it pointing at user-1", identities.upserts)
	}
	if len(store.created) != 1 || store.created[0].UserID != "user-1" {
		t.Fatalf("session = %+v, want one for user-1", store.created)
	}
}

// The attack this rule exists to stop: an attacker pre-registers the victim's
// address, never verifies it, and waits to inherit the account the first time
// the victim signs in with Google.
func TestRefusesToLinkUnverifiedLocalAccount(t *testing.T) {
	h, users, identities, store := newHandler(t)
	users.add(&domain.User{ID: "squatter", Email: "person@example.com", EmailVerified: false})

	_, err := h.Handle(t.Context(), googleCommand())
	if !errors.Is(err, domain.ErrLinkRequiresVerification) {
		t.Fatalf("err = %v, want ErrLinkRequiresVerification", err)
	}
	if len(identities.upserts) != 0 {
		t.Fatal("an unverified local account was linked anyway")
	}
	if len(store.created) != 0 {
		t.Fatal("a session was issued for an account that must not be linked")
	}
	if len(users.saved) != 0 {
		t.Fatal("the pre-registered account was modified")
	}
}

// If the provider will not vouch for the address we have nothing to link on and
// nothing to create an account from.
func TestRefusesUnverifiedProviderEmail(t *testing.T) {
	h, users, identities, _ := newHandler(t)
	cmd := googleCommand()
	cmd.EmailVerified = false

	_, err := h.Handle(t.Context(), cmd)
	if !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("err = %v, want ErrEmailNotVerified", err)
	}
	if len(users.saved) != 0 || len(identities.upserts) != 0 {
		t.Fatal("an unverified provider address created state")
	}
}

func TestRejectsMissingSubject(t *testing.T) {
	h, _, _, _ := newHandler(t)
	cmd := googleCommand()
	cmd.Subject = ""

	if _, err := h.Handle(t.Context(), cmd); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}
