-- name: CreateUser :exec
-- Unlike UpsertUser this does write email_verified. Creation is the one moment
-- the value is being asserted rather than saved back, so there is no earlier
-- read to go stale: the caller is stating what the row is born as. UpsertUser
-- still refuses to carry the column, which is what stops a stale in-memory User
-- from silently un-verifying an account later.
INSERT INTO users (id, email, display_name, created_at, updated_at, email_verified)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpsertUser :exec
-- Deliberately does not write email_verified. Verification state is owned by
-- SetUserEmailVerified (see auth.sql); if a general-purpose user save could
-- carry it, a stale in-memory User would silently un-verify an account.
INSERT INTO users (id, email, display_name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    updated_at = EXCLUDED.updated_at;

-- name: GetUserByID :one
SELECT id, email, display_name, created_at, updated_at, email_verified
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, display_name, created_at, updated_at, email_verified
FROM users
WHERE email = $1;
