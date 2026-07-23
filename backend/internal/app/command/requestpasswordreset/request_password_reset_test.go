package requestpasswordreset

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

type fakeUsers struct{ known map[string]*domain.User }

func (f *fakeUsers) Save(context.Context, *domain.User) error { return nil }
func (f *fakeUsers) FindByID(context.Context, string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.known[email]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

type fakeTokens struct{ created int }

func (f *fakeTokens) Create(context.Context, domain.EmailToken) error { f.created++; return nil }
func (f *fakeTokens) Find(context.Context, []byte) (domain.EmailToken, error) {
	return domain.EmailToken{}, domain.ErrNotFound
}
func (f *fakeTokens) Consume(context.Context, []byte, time.Time) error { return nil }
func (f *fakeTokens) DeleteForUser(context.Context, string, domain.EmailTokenPurpose) error {
	return nil
}

// failingMailer stands in for an unreachable relay.
type failingMailer struct{ called int }

func (m *failingMailer) Send(context.Context, string, string, string) error {
	m.called++
	return errors.New("relay unreachable")
}

func newHandler(mailer *failingMailer) *Handler {
	users := &fakeUsers{known: map[string]*domain.User{
		"known@example.com": {ID: "u1", Email: "known@example.com"},
	}}
	return NewHandler(users, &fakeTokens{}, mailer, "https://aurify.test")
}

// A broken relay used to surface as a 500 for a known address while an unknown
// one returned 200, which made this endpoint an account-existence oracle.
func TestSendFailureIsNotSurfaced(t *testing.T) {
	mailer := &failingMailer{}
	h := newHandler(mailer)

	if err := h.Handle(t.Context(), Command{Email: "known@example.com"}); err != nil {
		t.Fatalf("a failing relay must not surface an error, got %v", err)
	}
	if err := h.Handle(t.Context(), Command{Email: "nobody@example.com"}); err != nil {
		t.Fatalf("unknown address: %v", err)
	}
}

func TestUnknownAddressMintsNoToken(t *testing.T) {
	mailer := &failingMailer{}
	users := &fakeUsers{known: map[string]*domain.User{}}
	tokens := &fakeTokens{}
	h := NewHandler(users, tokens, mailer, "https://aurify.test")

	if err := h.Handle(t.Context(), Command{Email: "nobody@example.com"}); err != nil {
		t.Fatalf("unknown: %v", err)
	}
	if tokens.created != 0 {
		t.Fatalf("minted %d tokens for an unknown address, want 0", tokens.created)
	}
}
