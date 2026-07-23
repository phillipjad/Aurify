package signin

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

type fakeUsers struct{ user *domain.User }

func (f *fakeUsers) Save(context.Context, *domain.User) error   { return nil }
func (f *fakeUsers) Create(context.Context, *domain.User) error { return nil }
func (f *fakeUsers) FindByID(context.Context, string) (*domain.User, error) {
	return f.user, nil
}
func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if f.user != nil && f.user.Email == email {
		return f.user, nil
	}
	return nil, domain.ErrNotFound
}

type fakeCredentials struct{ hash string }

func (f *fakeCredentials) Upsert(context.Context, domain.Credential) error { return nil }
func (f *fakeCredentials) FindByUser(_ context.Context, userID string) (domain.Credential, error) {
	return domain.Credential{UserID: userID, PasswordHash: f.hash}, nil
}
func (f *fakeCredentials) SetEmailVerified(context.Context, string, bool) error { return nil }

type fakeSessions struct{}

func (fakeSessions) Create(context.Context, domain.Session, domain.RefreshToken) error { return nil }
func (fakeSessions) FindSession(context.Context, string) (domain.Session, error) {
	return domain.Session{}, domain.ErrNotFound
}
func (fakeSessions) FindRefreshToken(context.Context, []byte) (domain.RefreshToken, error) {
	return domain.RefreshToken{}, domain.ErrNotFound
}
func (fakeSessions) Rotate(context.Context, []byte, domain.RefreshToken) error { return nil }
func (fakeSessions) Touch(context.Context, string, time.Time) error            { return nil }
func (fakeSessions) Revoke(context.Context, string, time.Time) error           { return nil }
func (fakeSessions) RevokeAllForUser(context.Context, string, time.Time) error { return nil }

const testPassword = "a-good-password"

func newHandler(t *testing.T, verified bool) *Handler {
	t.Helper()

	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	signer, err := auth.NewSigner(priv, "aurify", "aurify-api")
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	issuer := sessions.NewIssuer(fakeSessions{}, signer, sessions.TTL{
		Access: time.Minute, Refresh: time.Hour, Session: 24 * time.Hour,
	})

	users := &fakeUsers{user: &domain.User{
		ID: "u1", Email: "user@example.com", EmailVerified: verified,
	}}
	return NewHandler(users, &fakeCredentials{hash: hash}, issuer)
}

// Production behaviour: an unverified address cannot sign in. Verification is
// what makes federated account linking safe, so this gate is load-bearing.
func TestUnverifiedEmailIsRefusedByDefault(t *testing.T) {
	h := newHandler(t, false)

	_, err := h.Handle(t.Context(), Command{Email: "user@example.com", Password: testPassword})
	if !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("err = %v, want ErrEmailNotVerified", err)
	}
}

func TestVerifiedEmailSignsInNormally(t *testing.T) {
	h := newHandler(t, true)

	tokens, err := h.Handle(t.Context(), Command{Email: "user@example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected both tokens")
	}
}
