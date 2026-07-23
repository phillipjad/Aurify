-- +goose Up
-- Authentication: local password credentials, federated identities, and the
-- session/refresh-token machinery behind cookie auth.
-- See docs/adr/0011-authentication-and-sessions.md.

-- Whether the address on users.email has been proven. This gates account
-- linking: an unverified account must never be auto-linked to a federated
-- identity, or an attacker could pre-register a victim's address and inherit
-- the account when the victim later signs in with Google.
ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- Local password credentials. A user with no row here has no password and can
-- only sign in through a federated identity.
CREATE TABLE user_credentials (
    user_id       TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    -- Argon2id in PHC string form, carrying its own parameters and salt so the
    -- cost can be raised later without invalidating existing hashes.
    password_hash TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

-- Federated sign-in identities (currently Google).
--
-- Deliberately separate from dsp_connections: that table holds data-access
-- grants for Spotify/Apple/YouTube playlists, whereas this one asserts *who the
-- user is*. Sharing a table would let a YouTube Music data connection imply a
-- login identity, which is a privilege escalation.
CREATE TABLE user_identities (
    provider   TEXT NOT NULL,
    -- The provider's stable subject identifier. Never the email address, which
    -- can be reassigned or changed by the user.
    subject    TEXT NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (provider, subject)
);

CREATE INDEX idx_user_identities_user ON user_identities (user_id);

-- One sign-in. Holds the absolute lifetime and the audit fields that make a
-- "signed in devices" view and a "sign out everywhere" action possible.
CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issued_at    TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL,
    -- Absolute expiry: a session dies at this point no matter how recently it
    -- was refreshed, bounding the damage from a stolen refresh token.
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    user_agent   TEXT NOT NULL DEFAULT '',
    ip           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_sessions_user ON sessions (user_id);

-- Refresh tokens, one row per issued token, rotated on every use.
--
-- Only the SHA-256 digest is stored, so a database leak yields values that
-- cannot be replayed. Keeping superseded rows (rather than overwriting one
-- current token) is what makes reuse detection possible: presenting a token
-- whose used_at is already set means the token was captured, so the whole
-- session is revoked.
CREATE TABLE refresh_tokens (
    token_hash BYTEA PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    issued_at  TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);

CREATE INDEX idx_refresh_tokens_session ON refresh_tokens (session_id);

-- Single-use, expiring tokens for email verification and password reset.
-- Hashed for the same reason as refresh tokens: the emailed link is the secret,
-- the database only needs to recognise it.
CREATE TABLE email_tokens (
    token_hash  BYTEA PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose     TEXT NOT NULL CHECK (purpose IN ('verify', 'reset')),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_email_tokens_user_purpose ON email_tokens (user_id, purpose);

-- +goose Down
DROP TABLE email_tokens;
DROP TABLE refresh_tokens;
DROP TABLE sessions;
DROP TABLE user_identities;
DROP TABLE user_credentials;
ALTER TABLE users DROP COLUMN email_verified;
