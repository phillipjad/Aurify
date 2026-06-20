-- name: UpsertDSPConnection :exec
INSERT INTO dsp_connections (
    user_id, platform, provider_user_id, access_token, refresh_token, expires_at, scopes
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, platform) DO UPDATE SET
    provider_user_id = EXCLUDED.provider_user_id,
    access_token = EXCLUDED.access_token,
    refresh_token = EXCLUDED.refresh_token,
    expires_at = EXCLUDED.expires_at,
    scopes = EXCLUDED.scopes;

-- name: ListConnectionsByUser :many
SELECT user_id, platform, provider_user_id, access_token, refresh_token, expires_at, scopes
FROM dsp_connections
WHERE user_id = $1;

-- name: DeleteConnectionsByUser :exec
DELETE FROM dsp_connections
WHERE user_id = $1;
