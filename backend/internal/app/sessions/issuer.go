// Package sessions issues and rotates the credential pair behind a signed-in
// user: a short-lived Ed25519 access token and a long-lived opaque refresh
// token.
//
// It sits in the application layer rather than in a single command because
// sign-in, token refresh and (later) federated sign-in all mint sessions the
// same way, and the reuse-detection rule must be identical in each.
package sessions

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
)

// Tokens is one issued credential pair, along with the expiries the transport
// layer needs to set cookie lifetimes.
type Tokens struct {
	SessionID        string
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// TTL bundles the lifetimes that shape a session.
type TTL struct {
	// Access is deliberately short. The access token is verified statelessly, so
	// a revoked session keeps working until its current access token expires;
	// this window is that exposure.
	Access time.Duration
	// Refresh is the idle timeout: a refresh token unused for this long dies.
	Refresh time.Duration
	// Session is the absolute cap. The session ends here no matter how often it
	// was refreshed, so a quietly stolen refresh token cannot be renewed
	// indefinitely.
	Session time.Duration
}

// Issuer mints and rotates sessions.
type Issuer struct {
	sessions ports.SessionRepository
	signer   *auth.Signer
	ttl      TTL
	now      func() time.Time
}

// NewIssuer constructs an Issuer.
func NewIssuer(repo ports.SessionRepository, signer *auth.Signer, ttl TTL) *Issuer {
	return &Issuer{sessions: repo, signer: signer, ttl: ttl, now: time.Now}
}

// Context describes the client a session is being issued to. It is recorded for
// the benefit of a "signed-in devices" view and post-incident review; it is
// never used to authenticate, since both fields are client-controlled.
type Context struct {
	UserAgent string
	IP        string
}

// Issue starts a new session for a user.
func (i *Issuer) Issue(ctx context.Context, userID string, client Context) (Tokens, error) {
	now := i.now().UTC()

	refreshToken, refreshHash, err := auth.NewOpaqueToken()
	if err != nil {
		return Tokens{}, err
	}

	session := domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		IssuedAt:  now,
		LastUsed:  now,
		ExpiresAt: now.Add(i.ttl.Session),
		UserAgent: client.UserAgent,
		IP:        client.IP,
	}
	refresh := domain.RefreshToken{
		Hash:      refreshHash,
		SessionID: session.ID,
		UserID:    userID,
		IssuedAt:  now,
		ExpiresAt: now.Add(i.ttl.Refresh),
	}
	if err := i.sessions.Create(ctx, session, refresh); err != nil {
		return Tokens{}, err
	}

	return i.mint(session.ID, userID, refreshToken, refresh.ExpiresAt, now)
}

// Rotate exchanges a refresh token for a fresh pair.
//
// Rotation is unconditional: every refresh retires the presented token. That is
// what makes theft detectable, because the victim and the attacker cannot both
// use the same token without one of them presenting a spent one.
//
// It takes no client Context, unlike Issue. The session's user agent and address
// are recorded once, at sign-in, and describe where the session was established;
// overwriting them on every refresh would erase exactly the detail a
// post-incident review wants. The repository records that the session was used
// in the same transaction as the rotation.
func (i *Issuer) Rotate(ctx context.Context, presented string) (Tokens, error) {
	now := i.now().UTC()
	hash := auth.HashToken(presented)

	stored, err := i.sessions.FindRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// An unknown token is indistinguishable from a very old one. Answer
			// the same way for both rather than confirming which.
			return Tokens{}, domain.ErrSessionInvalid
		}
		return Tokens{}, err
	}

	// A token that was already rotated away is being replayed, which means the
	// value leaked. Kill the whole session: the legitimate holder has to sign in
	// again, but the thief loses access too, and we cannot tell which of them is
	// making this request.
	if stored.Used() {
		if err := i.sessions.Revoke(ctx, stored.SessionID, now); err != nil {
			return Tokens{}, err
		}
		return Tokens{}, domain.ErrTokenReused
	}
	if !now.Before(stored.ExpiresAt) {
		return Tokens{}, domain.ErrSessionInvalid
	}

	session, err := i.sessions.FindSession(ctx, stored.SessionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Tokens{}, domain.ErrSessionInvalid
		}
		return Tokens{}, err
	}
	if !session.Active(now) {
		return Tokens{}, domain.ErrSessionInvalid
	}

	nextToken, nextHash, err := auth.NewOpaqueToken()
	if err != nil {
		return Tokens{}, err
	}
	// Do not extend past the session's absolute expiry; the idle window must
	// never outlive the session cap.
	nextExpiry := earliest(now.Add(i.ttl.Refresh), session.ExpiresAt)

	next := domain.RefreshToken{
		Hash:      nextHash,
		SessionID: session.ID,
		UserID:    session.UserID,
		IssuedAt:  now,
		ExpiresAt: nextExpiry,
	}
	if err := i.sessions.Rotate(ctx, hash, next); err != nil {
		// Rotate reports reuse when the guarded UPDATE matched no rows, which
		// means another request consumed this token between our read and write.
		if errors.Is(err, domain.ErrTokenReused) {
			if rerr := i.sessions.Revoke(ctx, session.ID, now); rerr != nil {
				return Tokens{}, rerr
			}
			return Tokens{}, domain.ErrTokenReused
		}
		return Tokens{}, err
	}

	return i.mint(session.ID, session.UserID, nextToken, nextExpiry, now)
}

// Revoke ends one session.
func (i *Issuer) Revoke(ctx context.Context, sessionID string) error {
	return i.sessions.Revoke(ctx, sessionID, i.now().UTC())
}

// RevokeAll signs a user out everywhere, used after a password reset where the
// previous credential must be assumed compromised.
func (i *Issuer) RevokeAll(ctx context.Context, userID string) error {
	return i.sessions.RevokeAllForUser(ctx, userID, i.now().UTC())
}

// Verify reports whether a session is still usable. The access token is checked
// statelessly by the transport layer; this is the database-backed check that
// makes revocation take effect, and callers apply it where the extra round trip
// is worth closing the access-token window.
func (i *Issuer) Verify(ctx context.Context, sessionID string) error {
	session, err := i.sessions.FindSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrSessionInvalid
		}
		return err
	}
	if !session.Active(i.now().UTC()) {
		return domain.ErrSessionInvalid
	}
	return nil
}

func (i *Issuer) mint(
	sessionID, userID, refreshToken string,
	refreshExpiry, now time.Time,
) (Tokens, error) {
	access, err := i.signer.Sign(userID, sessionID, i.ttl.Access)
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{
		SessionID:        sessionID,
		AccessToken:      access,
		AccessExpiresAt:  now.Add(i.ttl.Access),
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiry,
	}, nil
}

func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
