package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// UserRepository is the PostgreSQL-backed ports.UserRepository. A user and its
// DSP connections form one aggregate, so writes run in a single transaction.
type UserRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
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
		if err := q.UpsertDSPConnection(ctx, db.UpsertDSPConnectionParams{
			UserID:         user.ID,
			Platform:       string(platform),
			ProviderUserID: conn.ProviderUserID,
			AccessToken:    conn.AccessToken,
			RefreshToken:   conn.RefreshToken,
			ExpiresAt:      tsFromTime(conn.ExpiresAt),
			Scopes:         conn.Scopes,
		}); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
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
		ID:          row.ID,
		Email:       row.Email,
		DisplayName: row.DisplayName,
		Connections: make(map[domain.DSPPlatform]domain.DSPConnection, len(conns)),
		CreatedAt:   timeFromTS(row.CreatedAt),
		UpdatedAt:   timeFromTS(row.UpdatedAt),
	}
	for _, c := range conns {
		platform := domain.DSPPlatform(c.Platform)
		user.Connections[platform] = domain.DSPConnection{
			Platform:       platform,
			ProviderUserID: c.ProviderUserID,
			AccessToken:    c.AccessToken,
			RefreshToken:   c.RefreshToken,
			ExpiresAt:      timeFromTS(c.ExpiresAt),
			Scopes:         c.Scopes,
		}
	}
	return user, nil
}
