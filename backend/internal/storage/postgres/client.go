// Package postgres provides the PostgreSQL storage adapters (pgx/v5 + sqlc). The
// Store owns the connection pool and exposes typed repositories that implement
// the application's ports. The schema is versioned with goose migrations
// embedded in the binary and applied on Connect (see docs/adr/0010).
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"

	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// Store holds the pgx pool and the repositories built on top of it.
type Store struct {
	pool        *pgxpool.Pool
	users       *UserRepository
	covers      *CoverRepository
	credentials *CredentialRepository
	identities  *IdentityRepository
	sessions    *SessionRepository
	emailTokens *EmailTokenRepository
	authBlocks  *AuthBlockRepository
}

// Connect dials PostgreSQL, verifies the connection, brings the schema up to
// date with the embedded migrations, and constructs the repositories.
func Connect(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	if err := migrateUp(dsn); err != nil {
		pool.Close()
		return nil, err
	}

	queries := db.New(pool)
	return &Store{
		pool:        pool,
		users:       &UserRepository{pool: pool, q: queries},
		covers:      &CoverRepository{q: queries},
		credentials: &CredentialRepository{q: queries},
		identities:  &IdentityRepository{q: queries},
		sessions:    &SessionRepository{pool: pool, q: queries},
		emailTokens: &EmailTokenRepository{q: queries},
		authBlocks:  &AuthBlockRepository{q: queries},
	}, nil
}

// migrateUp applies the embedded goose migrations. goose uses database/sql, so
// it opens a short-lived connection via the pgx stdlib driver, separate from the
// pgxpool used at runtime.
func migrateUp(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open migrator: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("postgres: migrate: %w", err)
	}
	return nil
}

// Users returns the user repository.
func (s *Store) Users() *UserRepository { return s.users }

// Covers returns the cover repository.
func (s *Store) Covers() *CoverRepository { return s.covers }

// Credentials returns the password-credential repository.
func (s *Store) Credentials() *CredentialRepository { return s.credentials }

// Identities returns the federated-identity repository.
func (s *Store) Identities() *IdentityRepository { return s.identities }

// Sessions returns the session and refresh-token repository.
func (s *Store) Sessions() *SessionRepository { return s.sessions }

// EmailTokens returns the email-token repository.
func (s *Store) EmailTokens() *EmailTokenRepository { return s.emailTokens }

// Ping checks connectivity; used by the readiness probe.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// tsFromTime converts a Go time into the pgtype the generated queries expect.
func tsFromTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// timeFromTS converts a stored timestamp back into a Go time (zero if NULL).
func timeFromTS(ts pgtype.Timestamptz) time.Time { return ts.Time }

// AuthBlocks returns the permanent authentication lockout repository.
func (s *Store) AuthBlocks() *AuthBlockRepository { return s.authBlocks }
