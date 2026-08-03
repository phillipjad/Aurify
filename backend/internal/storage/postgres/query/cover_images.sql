-- name: SaveCoverImage :exec
-- Upsert, so regenerating a cover replaces its image rather than failing on the
-- primary key.
INSERT INTO cover_images (cover_id, bytes, content_type, created_at)
VALUES (@cover_id, @bytes, @content_type, @created_at)
ON CONFLICT (cover_id) DO UPDATE SET
    bytes = EXCLUDED.bytes,
    content_type = EXCLUDED.content_type,
    created_at = EXCLUDED.created_at;

-- name: FindCoverImage :one
SELECT bytes, content_type
FROM cover_images
WHERE cover_id = @cover_id;
