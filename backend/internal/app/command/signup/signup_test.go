package signup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

type fakeUsers struct {
	byEmail map[string]*domain.User
	saved   []*domain.User
}

func (f *fakeUsers) Save(_ context.Context, u *domain.User) error {
	if u.ID == "" {
		u.ID = "generated-id"
	}
	f.saved = append(f.saved, u)
	f.byEmail[u.Email] = u
	return nil
}

func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

type fakeCredentials struct{ upserts []domain.Credential }

func (f *fakeCredentials) Upsert(_ context.Context, c domain.Credential) error {
	f.upserts = append(f.upserts, c)
	return nil
}
func (f *fakeCredentials) FindByUser(context.Context, string) (domain.Credential, error) {
	return domain.Credential{}, domain.ErrNotFound
}
func (f *fakeCredentials) SetEmailVerified(context.Context, string, bool) error { return nil }

type fakeTokens struct{ created []domain.EmailToken }

func (f *fakeTokens) Create(_ context.Context, t domain.EmailToken) error {
	f.created = append(f.created, t)
	return nil
}
func (f *fakeTokens) Find(context.Context, []byte) (domain.EmailToken, error) {
	return domain.EmailToken{}, domain.ErrNotFound
}
func (f *fakeTokens) Consume(context.Context, []byte, time.Time) error { return nil }
func (f *fakeTokens) DeleteForUser(context.Context, string, domain.EmailTokenPurpose) error {
	return nil
}

type sentMail struct{ to, subject, body string }

type fakeMailer struct{ sent []sentMail }

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	f.sent = append(f.sent, sentMail{to, subject, body})
	return nil
}

func newHandler() (*Handler, *fakeUsers, *fakeCredentials, *fakeTokens, *fakeMailer) {
	users := &fakeUsers{byEmail: map[string]*domain.User{}}
	creds := &fakeCredentials{}
	tokens := &fakeTokens{}
	mailer := &fakeMailer{}
	return NewHandler(users, creds, tokens, mailer, "https://aurify.test"), users, creds, tokens, mailer
}

func TestSignupCreatesAccountAndSendsVerification(t *testing.T) {
	h, users, creds, tokens, mailer := newHandler()

	if err := h.Handle(t.Context(), Command{Email: "New@Example.com", Password: "a-good-password"}); err != nil {
		t.Fatalf("signup: %v", err)
	}

	if len(users.saved) != 1 {
		t.Fatalf("saved %d users, want 1", len(users.saved))
	}
	// The address must be normalised before storage, or the unique index does
	// not actually prevent a second account for the same mailbox.
	if got := users.saved[0].Email; got != "new@example.com" {
		t.Fatalf("stored email = %q, want %q", got, "new@example.com")
	}
	if users.saved[0].EmailVerified {
		t.Fatal("a new account must not start out verified")
	}
	if len(creds.upserts) != 1 {
		t.Fatalf("stored %d credentials, want 1", len(creds.upserts))
	}
	if creds.upserts[0].PasswordHash == "a-good-password" {
		t.Fatal("password was stored in plaintext")
	}
	if len(tokens.created) != 1 || tokens.created[0].Purpose != domain.PurposeVerifyEmail {
		t.Fatalf("expected one verification token, got %+v", tokens.created)
	}
	if len(mailer.sent) != 1 || mailer.sent[0].to != "new@example.com" {
		t.Fatalf("expected a verification email, got %+v", mailer.sent)
	}
}

// A duplicate signup reports that the account exists so the UI can send the
// user to sign in. This is a deliberate product decision that accepts account
// enumeration on this route; do not "fix" it back to a silent success without
// changing the sign-in copy too.
func TestSignupReportsAlreadyRegisteredAddress(t *testing.T) {
	h, users, creds, _, mailer := newHandler()
	users.byEmail["taken@example.com"] = &domain.User{ID: "existing", Email: "taken@example.com"}

	err := h.Handle(t.Context(), Command{Email: "Taken@Example.com", Password: "a-good-password"})
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("duplicate signup: err = %v, want ErrEmailTaken", err)
	}

	// The disclosure is the entire exposure: the existing account must not be
	// touched, or signup would become a way to overwrite someone's password.
	if len(users.saved) != 0 {
		t.Fatal("duplicate signup modified the user record")
	}
	if len(creds.upserts) != 0 {
		t.Fatal("duplicate signup overwrote the existing credential")
	}
	if len(mailer.sent) != 0 {
		t.Fatal("duplicate signup emailed the existing account holder")
	}
}

func TestSignupRejectsWeakPasswordAndBadEmail(t *testing.T) {
	h, _, _, _, _ := newHandler()

	if err := h.Handle(t.Context(), Command{Email: "a@b.test", Password: "short"}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password: err = %v, want ErrWeakPassword", err)
	}
	if err := h.Handle(
		t.Context(),
		Command{Email: "not-an-email", Password: "a-good-password"},
	); !errors.Is(
		err,
		ErrInvalidEmail,
	) {
		t.Fatalf("bad email: err = %v, want ErrInvalidEmail", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail("  User@Example.COM ")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got != "user@example.com" {
		t.Fatalf("normalized = %q, want %q", got, "user@example.com")
	}
	if _, err := NormalizeEmail(""); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("empty: err = %v, want ErrInvalidEmail", err)
	}
}
