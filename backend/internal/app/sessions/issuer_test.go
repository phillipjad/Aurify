package sessions

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// fakeSessions is an in-memory ports.SessionRepository. It reproduces the one
// behaviour the real adapter enforces in SQL: Rotate only succeeds if the
// presented token has not already been used.
type fakeSessions struct {
	sessions map[string]domain.Session
	tokens   map[string]domain.RefreshToken // keyed by string(hash)
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{
		sessions: map[string]domain.Session{},
		tokens:   map[string]domain.RefreshToken{},
	}
}

func (f *fakeSessions) Create(_ context.Context, s domain.Session, r domain.RefreshToken) error {
	f.sessions[s.ID] = s
	f.tokens[string(r.Hash)] = r
	return nil
}

func (f *fakeSessions) FindSession(_ context.Context, id string) (domain.Session, error) {
	s, ok := f.sessions[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) FindRefreshToken(_ context.Context, hash []byte) (domain.RefreshToken, error) {
	t, ok := f.tokens[string(hash)]
	if !ok {
		return domain.RefreshToken{}, domain.ErrNotFound
	}
	return t, nil
}

func (f *fakeSessions) Rotate(_ context.Context, presented []byte, next domain.RefreshToken) error {
	current, ok := f.tokens[string(presented)]
	if !ok || current.Used() {
		return domain.ErrTokenReused
	}
	current.UsedAt = time.Now().UTC()
	f.tokens[string(presented)] = current
	f.tokens[string(next.Hash)] = next
	return nil
}

func (f *fakeSessions) Touch(_ context.Context, id string, at time.Time) error {
	s := f.sessions[id]
	s.LastUsed = at
	f.sessions[id] = s
	return nil
}

func (f *fakeSessions) Revoke(_ context.Context, id string, at time.Time) error {
	s, ok := f.sessions[id]
	if !ok {
		return domain.ErrNotFound
	}
	s.RevokedAt = at
	f.sessions[id] = s
	return nil
}

func (f *fakeSessions) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	for id, s := range f.sessions {
		if s.UserID == userID {
			s.RevokedAt = at
			f.sessions[id] = s
		}
	}
	return nil
}

func newTestIssuer(t *testing.T) (*Issuer, *fakeSessions) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := auth.NewSigner(priv, "aurify", "aurify-api")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	repo := newFakeSessions()
	ttl := TTL{Access: 15 * time.Minute, Refresh: 30 * 24 * time.Hour, Session: 90 * 24 * time.Hour}
	return NewIssuer(repo, signer, ttl), repo
}

func TestIssueCreatesSessionAndTokens(t *testing.T) {
	issuer, repo := newTestIssuer(t)

	tokens, err := issuer.Issue(t.Context(), "user-1", Context{UserAgent: "test", IP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected both an access and a refresh token")
	}
	if _, ok := repo.sessions[tokens.SessionID]; !ok {
		t.Fatal("session was not persisted")
	}
}

func TestRotateIssuesNewPairAndRetiresOldToken(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	first, err := issuer.Issue(t.Context(), "user-1", Context{})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	second, err := issuer.Rotate(t.Context(), first.RefreshToken, Context{})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}

	if second.RefreshToken == first.RefreshToken {
		t.Fatal("rotation returned the same refresh token")
	}
	if second.SessionID != first.SessionID {
		t.Fatal("rotation should stay within the same session")
	}
}

// The core theft-detection property: presenting a refresh token that has
// already been rotated away means the value leaked, so the entire session must
// die rather than the request merely failing.
func TestRotateDetectsReuseAndRevokesSession(t *testing.T) {
	issuer, repo := newTestIssuer(t)

	first, err := issuer.Issue(t.Context(), "user-1", Context{})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := issuer.Rotate(t.Context(), first.RefreshToken, Context{}); err != nil {
		t.Fatalf("first rotate: %v", err)
	}

	// Replay the now-spent token, as a thief holding a captured copy would.
	_, err = issuer.Rotate(t.Context(), first.RefreshToken, Context{})
	if !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("replay: err = %v, want ErrTokenReused", err)
	}

	session := repo.sessions[first.SessionID]
	if !session.Revoked() {
		t.Fatal("session was not revoked after refresh-token reuse")
	}
}

// After reuse revokes the session, the token the legitimate client is holding
// must stop working too. Otherwise the thief is locked out but the victim is
// not, and the session survives the compromise.
func TestRotateRefusesAfterSessionRevoked(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	first, err := issuer.Issue(t.Context(), "user-1", Context{})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	second, err := issuer.Rotate(t.Context(), first.RefreshToken, Context{})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := issuer.Rotate(t.Context(), first.RefreshToken, Context{}); !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("replay: err = %v, want ErrTokenReused", err)
	}

	if _, err := issuer.Rotate(t.Context(), second.RefreshToken, Context{}); !errors.Is(err, domain.ErrSessionInvalid) {
		t.Fatalf("live token after revocation: err = %v, want ErrSessionInvalid", err)
	}
}

func TestRotateRejectsUnknownToken(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	if _, err := issuer.Rotate(t.Context(), "not-a-real-token", Context{}); !errors.Is(err, domain.ErrSessionInvalid) {
		t.Fatalf("unknown token: err = %v, want ErrSessionInvalid", err)
	}
}

func TestRefreshExpiryNeverOutlivesSessionCap(t *testing.T) {
	issuer, repo := newTestIssuer(t)
	// Absolute cap shorter than the idle window, so the cap must win.
	issuer.ttl = TTL{Access: time.Minute, Refresh: 30 * 24 * time.Hour, Session: time.Hour}

	first, err := issuer.Issue(t.Context(), "user-1", Context{})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	next, err := issuer.Rotate(t.Context(), first.RefreshToken, Context{})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}

	session := repo.sessions[first.SessionID]
	if next.RefreshExpiresAt.After(session.ExpiresAt) {
		t.Fatalf("refresh expiry %v outlives session cap %v", next.RefreshExpiresAt, session.ExpiresAt)
	}
}

func TestVerifyRejectsRevokedSession(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	tokens, err := issuer.Issue(t.Context(), "user-1", Context{})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := issuer.Verify(t.Context(), tokens.SessionID); err != nil {
		t.Fatalf("verify fresh session: %v", err)
	}

	if err := issuer.Revoke(t.Context(), tokens.SessionID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := issuer.Verify(t.Context(), tokens.SessionID); !errors.Is(err, domain.ErrSessionInvalid) {
		t.Fatalf("verify revoked session: err = %v, want ErrSessionInvalid", err)
	}
}
