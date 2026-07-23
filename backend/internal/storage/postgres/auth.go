package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// CredentialRepository is the PostgreSQL-backed ports.CredentialRepository.
type CredentialRepository struct {
	q *db.Queries
}

var _ ports.CredentialRepository = (*CredentialRepository)(nil)

// Upsert stores or replaces a user's password hash.
func (r *CredentialRepository) Upsert(ctx context.Context, cred domain.Credential) error {
	return r.q.UpsertCredential(ctx, db.UpsertCredentialParams{
		UserID:       cred.UserID,
		PasswordHash: cred.PasswordHash,
		UpdatedAt:    tsFromTime(time.Now().UTC()),
	})
}

// FindByUser returns a user's credential, mapping a miss to domain.ErrNotFound.
// A miss is normal: it means the account is federated-only.
func (r *CredentialRepository) FindByUser(ctx context.Context, userID string) (domain.Credential, error) {
	row, err := r.q.GetCredentialByUser(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Credential{}, domain.ErrNotFound
		}
		return domain.Credential{}, err
	}
	return domain.Credential{
		UserID:       row.UserID,
		PasswordHash: row.PasswordHash,
		UpdatedAt:    timeFromTS(row.UpdatedAt),
	}, nil
}

// SetEmailVerified marks a user's address proven (or unproven).
func (r *CredentialRepository) SetEmailVerified(ctx context.Context, userID string, verified bool) error {
	return r.q.SetUserEmailVerified(ctx, db.SetUserEmailVerifiedParams{
		ID:            userID,
		EmailVerified: verified,
		UpdatedAt:     tsFromTime(time.Now().UTC()),
	})
}

// IdentityRepository is the PostgreSQL-backed ports.IdentityRepository.
type IdentityRepository struct {
	q *db.Queries
}

var _ ports.IdentityRepository = (*IdentityRepository)(nil)

// Find looks up an identity by provider and subject.
func (r *IdentityRepository) Find(
	ctx context.Context,
	provider domain.IdentityProvider,
	subject string,
) (domain.Identity, error) {
	row, err := r.q.GetIdentity(ctx, db.GetIdentityParams{Provider: string(provider), Subject: subject})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Identity{}, domain.ErrNotFound
		}
		return domain.Identity{}, err
	}
	return identityFromRow(row), nil
}

// Upsert stores a federated identity link.
func (r *IdentityRepository) Upsert(ctx context.Context, identity domain.Identity) error {
	created := identity.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return r.q.UpsertIdentity(ctx, db.UpsertIdentityParams{
		Provider:  string(identity.Provider),
		Subject:   identity.Subject,
		UserID:    identity.UserID,
		Email:     identity.Email,
		CreatedAt: tsFromTime(created),
	})
}

// ListByUser returns every identity linked to a user.
func (r *IdentityRepository) ListByUser(ctx context.Context, userID string) ([]domain.Identity, error) {
	rows, err := r.q.ListIdentitiesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, identityFromRow(row))
	}
	return out, nil
}

func identityFromRow(row db.UserIdentity) domain.Identity {
	return domain.Identity{
		Provider:  domain.IdentityProvider(row.Provider),
		Subject:   row.Subject,
		UserID:    row.UserID,
		Email:     row.Email,
		CreatedAt: timeFromTS(row.CreatedAt),
	}
}

// SessionRepository is the PostgreSQL-backed ports.SessionRepository. It owns
// the pool as well as the queries because session creation and refresh rotation
// each span two statements that must not be observed half-applied.
type SessionRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

var _ ports.SessionRepository = (*SessionRepository)(nil)

// Create stores a new session and its first refresh token in one transaction.
// A session with no usable refresh token, or a token pointing at a session that
// failed to insert, would both strand the user, so neither may exist alone.
func (r *SessionRepository) Create(
	ctx context.Context,
	session domain.Session,
	refresh domain.RefreshToken,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	if err := q.CreateSession(ctx, db.CreateSessionParams{
		ID:         session.ID,
		UserID:     session.UserID,
		IssuedAt:   tsFromTime(session.IssuedAt),
		LastUsedAt: tsFromTime(session.LastUsed),
		ExpiresAt:  tsFromTime(session.ExpiresAt),
		UserAgent:  session.UserAgent,
		Ip:         session.IP,
	}); err != nil {
		return err
	}
	if err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		TokenHash: refresh.Hash,
		SessionID: session.ID,
		IssuedAt:  tsFromTime(refresh.IssuedAt),
		ExpiresAt: tsFromTime(refresh.ExpiresAt),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FindSession returns a session by id, mapping a miss to domain.ErrNotFound.
func (r *SessionRepository) FindSession(ctx context.Context, id string) (domain.Session, error) {
	row, err := r.q.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, domain.ErrNotFound
		}
		return domain.Session{}, err
	}
	return domain.Session{
		ID:        row.ID,
		UserID:    row.UserID,
		IssuedAt:  timeFromTS(row.IssuedAt),
		LastUsed:  timeFromTS(row.LastUsedAt),
		ExpiresAt: timeFromTS(row.ExpiresAt),
		RevokedAt: timeFromTS(row.RevokedAt),
		UserAgent: row.UserAgent,
		IP:        row.Ip,
	}, nil
}

// FindRefreshToken looks a token up by digest. The returned token may be
// expired or already used; interpreting that is the caller's job, because a
// used token is a security event rather than a plain miss.
func (r *SessionRepository) FindRefreshToken(ctx context.Context, hash []byte) (domain.RefreshToken, error) {
	row, err := r.q.GetRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshToken{}, domain.ErrNotFound
		}
		return domain.RefreshToken{}, err
	}
	return domain.RefreshToken{
		Hash:      row.TokenHash,
		SessionID: row.SessionID,
		UserID:    row.UserID,
		IssuedAt:  timeFromTS(row.IssuedAt),
		ExpiresAt: timeFromTS(row.ExpiresAt),
		UsedAt:    timeFromTS(row.UsedAt),
	}, nil
}

// Rotate consumes the presented refresh token and issues its replacement in one
// transaction.
//
// The UPDATE carries a `used_at IS NULL` guard, so if two requests present the
// same token concurrently exactly one can win. The loser sees zero affected rows
// and gets domain.ErrTokenReused, which is the correct reading: either the token
// leaked, or the client raced itself. Both warrant killing the session.
func (r *SessionRepository) Rotate(ctx context.Context, presented []byte, next domain.RefreshToken) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	affected, err := q.MarkRefreshTokenUsed(ctx, db.MarkRefreshTokenUsedParams{
		TokenHash: presented,
		UsedAt:    tsFromTime(time.Now().UTC()),
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrTokenReused
	}

	if err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		TokenHash: next.Hash,
		SessionID: next.SessionID,
		IssuedAt:  tsFromTime(next.IssuedAt),
		ExpiresAt: tsFromTime(next.ExpiresAt),
	}); err != nil {
		return err
	}
	if err := q.TouchSession(ctx, db.TouchSessionParams{
		ID:         next.SessionID,
		LastUsedAt: tsFromTime(time.Now().UTC()),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Touch records that a session was just used.
func (r *SessionRepository) Touch(ctx context.Context, sessionID string, at time.Time) error {
	return r.q.TouchSession(ctx, db.TouchSessionParams{ID: sessionID, LastUsedAt: tsFromTime(at)})
}

// Revoke kills one session and deletes its refresh tokens, so a leaked token
// cannot be replayed even if the revocation check is ever bypassed.
func (r *SessionRepository) Revoke(ctx context.Context, sessionID string, at time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	if err := q.RevokeSession(ctx, db.RevokeSessionParams{ID: sessionID, RevokedAt: tsFromTime(at)}); err != nil {
		return err
	}
	if err := q.DeleteRefreshTokensBySession(ctx, sessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RevokeAllForUser signs a user out everywhere. Used after a password reset,
// where the assumption must be that the old credential was compromised.
func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	return r.q.RevokeSessionsByUser(ctx, db.RevokeSessionsByUserParams{
		UserID:    userID,
		RevokedAt: tsFromTime(at),
	})
}

// EmailTokenRepository is the PostgreSQL-backed ports.EmailTokenRepository.
type EmailTokenRepository struct {
	q *db.Queries
}

var _ ports.EmailTokenRepository = (*EmailTokenRepository)(nil)

// Create stores a new single-use email token.
func (r *EmailTokenRepository) Create(ctx context.Context, token domain.EmailToken) error {
	return r.q.CreateEmailToken(ctx, db.CreateEmailTokenParams{
		TokenHash: token.Hash,
		UserID:    token.UserID,
		Purpose:   string(token.Purpose),
		ExpiresAt: tsFromTime(token.ExpiresAt),
		CreatedAt: tsFromTime(token.CreatedAt),
	})
}

// Find returns an email token by digest.
func (r *EmailTokenRepository) Find(ctx context.Context, hash []byte) (domain.EmailToken, error) {
	row, err := r.q.GetEmailToken(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.EmailToken{}, domain.ErrNotFound
		}
		return domain.EmailToken{}, err
	}
	return domain.EmailToken{
		Hash:       row.TokenHash,
		UserID:     row.UserID,
		Purpose:    domain.EmailTokenPurpose(row.Purpose),
		ExpiresAt:  timeFromTS(row.ExpiresAt),
		ConsumedAt: timeFromTS(row.ConsumedAt),
		CreatedAt:  timeFromTS(row.CreatedAt),
	}, nil
}

// Consume marks a token used. The `consumed_at IS NULL` guard lives in SQL, so
// a replayed link loses the race deterministically instead of depending on
// read-then-write ordering in Go.
func (r *EmailTokenRepository) Consume(ctx context.Context, hash []byte, at time.Time) error {
	affected, err := r.q.ConsumeEmailToken(ctx, db.ConsumeEmailTokenParams{
		TokenHash:  hash,
		ConsumedAt: tsFromTime(at),
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrTokenInvalid
	}
	return nil
}

// DeleteForUser clears a user's outstanding tokens of one purpose, so issuing a
// new reset link invalidates any earlier one.
func (r *EmailTokenRepository) DeleteForUser(
	ctx context.Context,
	userID string,
	purpose domain.EmailTokenPurpose,
) error {
	return r.q.DeleteEmailTokensByUserPurpose(ctx, db.DeleteEmailTokensByUserPurposeParams{
		UserID:  userID,
		Purpose: string(purpose),
	})
}

// AuthBlockRepository is the PostgreSQL-backed ports.AuthBlockRepository.
//
// Blocks live in the database rather than in process memory because they are
// permanent: an in-memory set would silently forget every lockout on the next
// deploy, which would make "permanent" untrue in exactly the situation it
// matters.
type AuthBlockRepository struct {
	q *db.Queries
}

var _ ports.AuthBlockRepository = (*AuthBlockRepository)(nil)

// IsBlocked reports whether an (ip, identifier) pair is locked out.
func (r *AuthBlockRepository) IsBlocked(ctx context.Context, ip, identifier string) (bool, error) {
	_, err := r.q.GetAuthBlock(ctx, db.GetAuthBlockParams{Ip: ip, Identifier: identifier})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Block records a permanent lockout. Re-blocking an already-blocked pair is a
// no-op, so the original timestamp survives as the record of when the abuse
// began.
func (r *AuthBlockRepository) Block(ctx context.Context, ip, identifier string, failures int, reason string) error {
	return r.q.CreateAuthBlock(ctx, db.CreateAuthBlockParams{
		Ip:         ip,
		Identifier: identifier,
		BlockedAt:  tsFromTime(time.Now().UTC()),
		Failures:   int32(failures),
		Reason:     reason,
	})
}
