-- name: SaveCoverImage :exec
-- Keyed by revision, so each run keeps its own bytes and they cascade with it.
-- Still an upsert, so a retried store replaces the bytes rather than failing on
-- the primary key.
INSERT INTO cover_images (revision_id, bytes, content_type, created_at)
VALUES (@revision_id, @bytes, @content_type, @created_at)
ON CONFLICT (revision_id) DO UPDATE SET
    bytes = EXCLUDED.bytes,
    content_type = EXCLUDED.content_type,
    created_at = EXCLUDED.created_at;

-- name: FindCoverImage :one
SELECT bytes, content_type
FROM cover_images
WHERE revision_id = @revision_id;
