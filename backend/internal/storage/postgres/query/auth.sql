-- name: UpsertCredential :exec
INSERT INTO user_credentials (user_id, password_hash, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    updated_at = EXCLUDED.updated_at;

-- name: GetCredentialByUser :one
SELECT user_id, password_hash, updated_at
FROM user_credentials
WHERE user_id = $1;

-- name: DeleteCredentialByUser :exec
DELETE FROM user_credentials WHERE user_id = $1;

-- name: SetUserEmailVerified :exec
UPDATE users SET email_verified = $2, updated_at = $3 WHERE id = $1;

-- name: GetIdentity :one
SELECT provider, subject, user_id, email, created_at
FROM user_identities
WHERE provider = $1 AND subject = $2;

-- name: UpsertIdentity :exec
INSERT INTO user_identities (provider, subject, user_id, email, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (provider, subject) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    email = EXCLUDED.email;

-- name: ListIdentitiesByUser :many
SELECT provider, subject, user_id, email, created_at
FROM user_identities
WHERE user_id = $1;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, issued_at, last_used_at, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetSession :one
SELECT id, user_id, issued_at, last_used_at, expires_at, revoked_at, user_agent, ip
FROM sessions
WHERE id = $1;

-- name: TouchSession :exec
UPDATE sessions SET last_used_at = $2 WHERE id = $1;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionsByUser :exec
UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (token_hash, session_id, issued_at, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetRefreshToken :one
-- Joined to sessions so a single round trip yields both the token's state and
-- whether the session behind it is still alive.
SELECT rt.token_hash, rt.session_id, rt.issued_at, rt.expires_at, rt.used_at,
       s.user_id, s.expires_at AS session_expires_at, s.revoked_at AS session_revoked_at
FROM refresh_tokens rt
JOIN sessions s ON s.id = rt.session_id
WHERE rt.token_hash = $1;

-- name: MarkRefreshTokenUsed :execrows
-- Guarded by used_at IS NULL so two concurrent refreshes cannot both succeed:
-- the loser affects zero rows and is treated as reuse.
UPDATE refresh_tokens SET used_at = $2 WHERE token_hash = $1 AND used_at IS NULL;

-- name: DeleteRefreshTokensBySession :exec
DELETE FROM refresh_tokens WHERE session_id = $1;

-- name: CreateEmailToken :exec
INSERT INTO email_tokens (token_hash, user_id, purpose, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetEmailToken :one
SELECT token_hash, user_id, purpose, expires_at, consumed_at, created_at
FROM email_tokens
WHERE token_hash = $1;

-- name: ConsumeEmailToken :execrows
-- Single-use: the consumed_at IS NULL guard means a replayed link affects zero
-- rows, so the caller can reject it without a separate read.
UPDATE email_tokens SET consumed_at = $2 WHERE token_hash = $1 AND consumed_at IS NULL;

-- name: DeleteEmailTokensByUserPurpose :exec
DELETE FROM email_tokens WHERE user_id = $1 AND purpose = $2;

-- name: GetAuthBlock :one
SELECT ip, identifier, blocked_at, failures, reason
FROM auth_blocks
WHERE ip = $1 AND identifier = $2;

-- name: CreateAuthBlock :exec
-- ON CONFLICT DO NOTHING keeps the original blocked_at, so a blocked pair that
-- keeps trying does not roll its own timestamp forward and hide when the abuse
-- actually started.
INSERT INTO auth_blocks (ip, identifier, blocked_at, failures, reason)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (ip, identifier) DO NOTHING;

-- name: DeleteAuthBlock :exec
-- Operator-only. Nothing in the request path calls this; it exists for the
-- future admin portal and for manual intervention.
DELETE FROM auth_blocks WHERE ip = $1 AND identifier = $2;
