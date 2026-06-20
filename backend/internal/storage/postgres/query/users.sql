-- name: UpsertUser :exec
INSERT INTO users (id, email, display_name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    updated_at = EXCLUDED.updated_at;

-- name: GetUserByID :one
SELECT id, email, display_name, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, display_name, created_at, updated_at
FROM users
WHERE email = $1;
