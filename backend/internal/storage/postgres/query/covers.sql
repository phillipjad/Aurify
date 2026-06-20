-- name: UpsertCover :exec
INSERT INTO covers (
    id, user_id, platform, playlist_id, playlist_name, status,
    prompt, image_url, analysis, error, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    platform = EXCLUDED.platform,
    playlist_id = EXCLUDED.playlist_id,
    playlist_name = EXCLUDED.playlist_name,
    status = EXCLUDED.status,
    prompt = EXCLUDED.prompt,
    image_url = EXCLUDED.image_url,
    analysis = EXCLUDED.analysis,
    error = EXCLUDED.error,
    updated_at = EXCLUDED.updated_at;

-- name: GetCoverByID :one
SELECT id, user_id, platform, playlist_id, playlist_name, status,
       prompt, image_url, analysis, error, created_at, updated_at
FROM covers
WHERE id = $1;

-- name: ListCoversByUser :many
SELECT id, user_id, platform, playlist_id, playlist_name, status,
       prompt, image_url, analysis, error, created_at, updated_at
FROM covers
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
