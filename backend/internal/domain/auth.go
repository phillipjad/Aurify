package domain

import "time"

// IdentityProvider names a federated sign-in provider. This is distinct from
// DSPPlatform: a DSPPlatform grants access to a user's music library, an
// IdentityProvider asserts who the user is. YouTube Music and Google both speak
// OAuth to Google, but only one of them may establish a login.
type IdentityProvider string

// Supported identity providers.
const (
	ProviderGoogle IdentityProvider = "google"
)

// EmailTokenPurpose distinguishes the two kinds of single-use emailed link.
type EmailTokenPurpose string

// Email token purposes.
const (
	PurposeVerifyEmail   EmailTokenPurpose = "verify"
	PurposeResetPassword EmailTokenPurpose = "reset"
)

// Identity links an Aurify user to an account at a federated provider.
type Identity struct {
	Provider IdentityProvider
	// Subject is the provider's stable identifier for the account. It is never
	// the email address: addresses can be changed or reassigned, so keying on
	// one would let a recycled address inherit an account.
	Subject   string
	UserID    string
	Email     string
	CreatedAt time.Time
}

// Session is one sign-in. Access tokens carry its id, and refresh tokens hang
// off it, so revoking a Session invalidates the whole chain.
type Session struct {
	ID       string
	UserID   string
	IssuedAt time.Time
	LastUsed time.Time
	// ExpiresAt is absolute: the session dies at this instant regardless of how
	// recently it was refreshed, which bounds the damage from a stolen token.
	ExpiresAt time.Time
	RevokedAt time.Time
	UserAgent string
	IP        string
}

// Revoked reports whether the session has been explicitly revoked.
func (s Session) Revoked() bool { return !s.RevokedAt.IsZero() }

// Active reports whether the session may still be used at the given time.
func (s Session) Active(now time.Time) bool {
	return !s.Revoked() && now.Before(s.ExpiresAt)
}

// RefreshToken is one issued refresh credential. Tokens are rotated on every
// use: the presented token is marked used and a fresh one takes its place.
type RefreshToken struct {
	// Hash is the SHA-256 digest of the token. The token itself is never stored,
	// so a database leak cannot be replayed against us.
	Hash      []byte
	SessionID string
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	// UsedAt is set once the token has been rotated away. Presenting a token
	// that already carries a UsedAt means the value leaked and is being replayed,
	// which is why it triggers revocation of the entire session rather than a
	// plain rejection.
	UsedAt time.Time
}

// Used reports whether this token has already been rotated away.
func (t RefreshToken) Used() bool { return !t.UsedAt.IsZero() }

// EmailToken is a single-use, expiring credential delivered by email.
type EmailToken struct {
	Hash       []byte
	UserID     string
	Purpose    EmailTokenPurpose
	ExpiresAt  time.Time
	ConsumedAt time.Time
	CreatedAt  time.Time
}

// Credential is a user's local password credential, stored as an Argon2id PHC
// string. A user with no Credential can only sign in through a federated
// identity.
type Credential struct {
	UserID       string
	PasswordHash string
	UpdatedAt    time.Time
}
