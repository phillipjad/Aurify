package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/crypto"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// UserRepository is the PostgreSQL-backed ports.UserRepository. A user and its
// DSP connections form one aggregate, so writes run in a single transaction.
type UserRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
	// tokens encrypts DSP credentials on the way to the database and back, so a
	// stolen dump does not hand over live access to every connected library.
	tokens *crypto.Cipher
}

var _ ports.UserRepository = (*UserRepository)(nil)

// Save upserts the user row and replaces its DSP connections in one transaction.
// Replacing the whole connection set keeps the stored rows in sync with the
// in-memory map, including any removals.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	now := time.Now().UTC()
	user.UpdatedAt = now
	if user.ID == "" {
		user.ID = uuid.NewString()
		user.CreatedAt = now
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	if err := q.UpsertUser(ctx, db.UpsertUserParams{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		CreatedAt:   tsFromTime(user.CreatedAt),
		UpdatedAt:   tsFromTime(user.UpdatedAt),
	}); err != nil {
		return err
	}

	if err := q.DeleteConnectionsByUser(ctx, user.ID); err != nil {
		return err
	}
	for platform, conn := range user.Connections {
		access, err := r.tokens.Encrypt(conn.AccessToken)
		if err != nil {
			return err
		}
		refresh, err := r.tokens.Encrypt(conn.RefreshToken)
		if err != nil {
			return err
		}
		if err := q.UpsertDSPConnection(ctx, db.UpsertDSPConnectionParams{
			UserID:         user.ID,
			Platform:       string(platform),
			ProviderUserID: conn.ProviderUserID,
			AccessToken:    access,
			RefreshToken:   refresh,
			ExpiresAt:      tsFromTime(conn.ExpiresAt),
			Scopes:         conn.Scopes,
		}); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Create inserts a new user row, carrying EmailVerified through, which Save
// deliberately will not do.
//
// It writes no DSP connections: a user being created has none, and a federated
// sign-in is the only caller. Save remains the path for an existing user whose
// connection set has to be reconciled.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	now := time.Now().UTC()
	if user.ID == "" {
		user.ID = uuid.NewString()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now

	return r.q.CreateUser(ctx, db.CreateUserParams{
		ID:            user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		CreatedAt:     tsFromTime(user.CreatedAt),
		UpdatedAt:     tsFromTime(user.UpdatedAt),
		EmailVerified: user.EmailVerified,
	})
}

// FindByID looks up a user by id, mapping a miss to domain.ErrNotFound.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

// FindByEmail looks up a user by email, mapping a miss to domain.ErrNotFound.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.hydrate(ctx, row)
}

// hydrate loads a user's DSP connections and assembles the domain aggregate. The
// Connections map is always non-nil so callers can append without a nil check.
func (r *UserRepository) hydrate(ctx context.Context, row db.User) (*domain.User, error) {
	conns, err := r.q.ListConnectionsByUser(ctx, row.ID)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:            row.ID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		EmailVerified: row.EmailVerified,
		Connections:   make(map[domain.DSPPlatform]domain.DSPConnection, len(conns)),
		CreatedAt:     timeFromTS(row.CreatedAt),
		UpdatedAt:     timeFromTS(row.UpdatedAt),
	}
	for _, c := range conns {
		platform := domain.DSPPlatform(c.Platform)
		access, err := r.tokens.Decrypt(c.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("postgres: decrypt %s access token: %w", platform, err)
		}
		refresh, err := r.tokens.Decrypt(c.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("postgres: decrypt %s refresh token: %w", platform, err)
		}
		user.Connections[platform] = domain.DSPConnection{
			Platform:       platform,
			ProviderUserID: c.ProviderUserID,
			AccessToken:    access,
			RefreshToken:   refresh,
			ExpiresAt:      timeFromTS(c.ExpiresAt),
			Scopes:         c.Scopes,
		}
	}
	return user, nil
}
