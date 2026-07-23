package federatedsignin

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres"
)

// These tests run against a real PostgreSQL rather than in-memory doubles.
//
// That is a deliberate reaction to a bug they failed to catch. The fake user
// repository stored the domain object verbatim, so an account created from a
// verified Google identity looked verified in the test. The real adapter does
// not: UpsertUser deliberately refuses to write email_verified, so the row
// landed with false, and the linking rule then refused a user the provider had
// already vouched for. Every unit test passed the whole time.
//
// The lesson is narrow but sharp: this handler's contract is mostly about what
// ends up in the database, so a double that cannot disagree with the schema
// cannot test it.

// testDSN is the database these tests run against. Without it they skip rather
// than fail, so `go test ./...` still works on a machine with no database. CI
// sets it, which is what keeps them honest.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("AURIFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AURIFY_TEST_DATABASE_URL is not set, skipping the PostgreSQL integration tests")
	}
	return dsn
}

type fixture struct {
	h     *Handler
	store *postgres.Store
	db    *pgx.Conn
}

// newFixture connects to PostgreSQL, applies the migrations and empties every
// table, so each test starts from a known schema and no rows.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := testDSN(t)
	ctx := t.Context()

	// Connect runs the goose migrations, so the schema under test is the real
	// one rather than something the test hand-rolled.
	store, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to %s: %v", dsn, err)
	}
	t.Cleanup(store.Close)

	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.WithoutCancel(ctx)) })

	// Truncate everything except goose's bookkeeping. Enumerating the tables
	// here would rot the moment a migration adds one.
	if _, err := db.Exec(ctx, `
		DO $$
		DECLARE r record;
		BEGIN
			FOR r IN
				SELECT tablename FROM pg_tables
				WHERE schemaname = 'public' AND tablename <> 'goose_db_version'
			LOOP
				EXECUTE 'TRUNCATE TABLE ' || quote_ident(r.tablename) || ' CASCADE';
			END LOOP;
		END $$;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

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

	issuer := sessions.NewIssuer(store.Sessions(), signer, sessions.TTL{
		Access:  15 * time.Minute,
		Refresh: 30 * 24 * time.Hour,
		Session: 90 * 24 * time.Hour,
	})

	return &fixture{
		h:     NewHandler(store.Users(), store.Identities(), issuer),
		store: store,
		db:    db,
	}
}

// count runs a scalar count query against the live schema.
func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// seedUser inserts a local account directly, so a test can set up the exact
// verification state it needs.
func (f *fixture) seedUser(t *testing.T, email string, verified bool) *domain.User {
	t.Helper()
	u := &domain.User{
		Email:         email,
		DisplayName:   "Seeded",
		EmailVerified: verified,
		Connections:   map[domain.DSPPlatform]domain.DSPConnection{},
		CreatedAt:     time.Now().UTC(),
	}
	if err := f.store.Users().Create(t.Context(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
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
	f := newFixture(t)
	user := f.seedUser(t, "person@example.com", true)
	if err := f.store.Identities().Upsert(t.Context(), domain.Identity{
		Provider:  domain.ProviderGoogle,
		Subject:   "google-sub-123",
		UserID:    user.ID,
		Email:     "person@example.com",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	cmd := googleCommand()
	cmd.Email = "renamed@example.com"

	tokens, err := f.h.Handle(t.Context(), cmd)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("no access token issued")
	}
	if n := f.count(t, `SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("users = %d, want 1: a changed provider email must not fork an account", n)
	}
	if n := f.count(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, user.ID); n != 1 {
		t.Fatalf("sessions for the seeded user = %d, want 1", n)
	}
}

// No identity and no local account: this is a first-time Google user.
//
// The verification assertion here is the one that matters, and it reads the
// database rather than the in-memory object the handler built.
func TestUnknownIdentityCreatesVerifiedAccount(t *testing.T) {
	f := newFixture(t)

	if _, err := f.h.Handle(t.Context(), googleCommand()); err != nil {
		t.Fatalf("sign in: %v", err)
	}

	created, err := f.store.Users().FindByEmail(t.Context(), "person@example.com")
	if err != nil {
		t.Fatalf("find created user: %v", err)
	}
	// Google asserted email_verified, and we refuse the sign-in otherwise, so
	// the address is proven. A federated account has no password, so leaving it
	// unverified strands it behind a flow it can never complete, and trips the
	// linking rule on the next visit.
	if !created.EmailVerified {
		t.Fatal("account created from a verified federated identity is not verified in the database")
	}
	if n := f.count(t,
		`SELECT count(*) FROM user_identities WHERE user_id = $1 AND provider = $2 AND subject = $3`,
		created.ID, string(domain.ProviderGoogle), "google-sub-123",
	); n != 1 {
		t.Fatalf("identity rows linked to the new user = %d, want 1", n)
	}
	if n := f.count(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, created.ID); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

// The linking rule from ADR 0011: auto-link only when the local account is
// already verified.
func TestLinksToVerifiedLocalAccount(t *testing.T) {
	f := newFixture(t)
	user := f.seedUser(t, "person@example.com", true)

	if _, err := f.h.Handle(t.Context(), googleCommand()); err != nil {
		t.Fatalf("sign in: %v", err)
	}

	if n := f.count(t, `SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("users = %d, want 1: linking must not create a second account", n)
	}
	if n := f.count(t, `SELECT count(*) FROM user_identities WHERE user_id = $1`, user.ID); n != 1 {
		t.Fatalf("identities linked to the existing account = %d, want 1", n)
	}
	if n := f.count(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, user.ID); n != 1 {
		t.Fatalf("sessions for the existing account = %d, want 1", n)
	}
}

// The attack this rule exists to stop: an attacker pre-registers the victim's
// address, never verifies it, and waits to inherit the account the first time
// the victim signs in with Google.
func TestRefusesToLinkUnverifiedLocalAccount(t *testing.T) {
	f := newFixture(t)
	squatter := f.seedUser(t, "person@example.com", false)

	_, err := f.h.Handle(t.Context(), googleCommand())
	if !errors.Is(err, domain.ErrLinkRequiresVerification) {
		t.Fatalf("err = %v, want ErrLinkRequiresVerification", err)
	}
	if n := f.count(t, `SELECT count(*) FROM user_identities`); n != 0 {
		t.Fatalf("identity rows = %d, want 0: an unverified local account was linked", n)
	}
	if n := f.count(t, `SELECT count(*) FROM sessions`); n != 0 {
		t.Fatalf("sessions = %d, want 0: a session was issued for an account that must not be linked", n)
	}
	// The squatter's row must be untouched, in particular not promoted to
	// verified as a side effect of the attempt.
	var verified bool
	if err := f.db.QueryRow(t.Context(),
		`SELECT email_verified FROM users WHERE id = $1`, squatter.ID,
	).Scan(&verified); err != nil {
		t.Fatalf("read squatter: %v", err)
	}
	if verified {
		t.Fatal("the pre-registered account was promoted to verified")
	}
}

// If the provider will not vouch for the address we have nothing to link on and
// nothing to create an account from.
func TestRefusesUnverifiedProviderEmail(t *testing.T) {
	f := newFixture(t)
	cmd := googleCommand()
	cmd.EmailVerified = false

	_, err := f.h.Handle(t.Context(), cmd)
	if !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("err = %v, want ErrEmailNotVerified", err)
	}
	if n := f.count(t, `SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("users = %d, want 0", n)
	}
	if n := f.count(t, `SELECT count(*) FROM user_identities`); n != 0 {
		t.Fatalf("identity rows = %d, want 0", n)
	}
}

func TestRejectsMissingSubject(t *testing.T) {
	f := newFixture(t)
	cmd := googleCommand()
	cmd.Subject = ""

	if _, err := f.h.Handle(t.Context(), cmd); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if n := f.count(t, `SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("users = %d, want 0", n)
	}
}
