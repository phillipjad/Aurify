// Package pgtest is the shared harness for tests that run against a real
// PostgreSQL.
//
// It exists because more than one package needs one, and they must not run at
// the same time: `go test ./...` builds and runs packages in parallel, and these
// tests truncate every table. Without the advisory lock below, one package's
// reset deletes the rows another package just inserted, which surfaces as a
// foreign key violation in whichever one lost.
package pgtest

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/phillipjad/aurify/backend/internal/platform/crypto"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres"
)

// testDBSuffix is required at the end of the test database's name.
//
// These tests truncate every table in the schema, so pointing them at a database
// someone is actually using destroys it, silently and instantly. That is not
// hypothetical: this suite was first run against the local development database
// and wiped it. Requiring the name to declare itself disposable makes the
// mistake impossible to make by accident.
const testDBSuffix = "_test"

// resetLockKey is an arbitrary constant every integration package agrees on. The
// lock is held for one test, so tests serialize against each other rather than
// whole packages serializing.
const resetLockKey = 0x4155524946590001

// DSN is the database these tests run against. Without it they skip rather than
// fail, so `go test ./...` still works on a machine with no database. CI sets
// it, which is what keeps them honest.
//
// A DSN that is set but unsafe is a failure, not a skip: skipping would turn a
// misconfigured CI into a suite that silently tests nothing.
func DSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("AURIFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AURIFY_TEST_DATABASE_URL is not set, skipping the PostgreSQL integration tests")
	}

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("AURIFY_TEST_DATABASE_URL is not a valid URL: %v", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !strings.HasSuffix(name, testDBSuffix) {
		// The DSN is deliberately not echoed back: it carries a password, and
		// this message can end up in CI logs.
		t.Fatalf(
			"refusing to run: AURIFY_TEST_DATABASE_URL points at database %q, which does not end in %q.\n"+
				"These tests TRUNCATE every table in the schema, so they must not be aimed at a database "+
				"anyone is using. Create a disposable one and point the variable at it:\n"+
				"  createdb %[1]s%[2]s\n"+
				"then change the database name in AURIFY_TEST_DATABASE_URL to %[1]s%[2]s",
			name, testDBSuffix,
		)
	}
	return dsn
}

// Reset connects, applies the migrations, and empties every table, returning a
// store and a second connection for assertions that bypass the repositories.
//
// It holds a session-level advisory lock for the duration of the test, so a
// concurrently running package waits rather than truncating underneath it.
func Reset(t *testing.T) (*postgres.Store, *pgx.Conn) {
	t.Helper()
	dsn := DSN(t)
	ctx := t.Context()

	// Taken before anything else touches the schema, and released by closing
	// this connection: advisory locks are scoped to the session that holds them.
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.WithoutCancel(ctx)) })
	if _, err := db.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(resetLockKey)); err != nil {
		t.Fatalf("take the reset lock: %v", err)
	}

	// Connect runs the goose migrations, so the schema under test is the real
	// one rather than something the test hand-rolled.
	store, err := postgres.Connect(ctx, dsn, crypto.DeriveKey([]byte("integration-test-seed")))
	if err != nil {
		t.Fatalf("connect to %s: %v", dsn, err)
	}
	t.Cleanup(store.Close)

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

	return store, db
}
