-- +goose Up
-- The bytes behind a cover's image_url. See
-- docs/adr/0016-generation-providers.md.
--
-- Image providers hand back the image itself, or a link that expires within the
-- hour, so the bytes have to land somewhere of ours. ADR 0014 planned a GCS
-- bucket; at roughly 150KB per JPEG that is a cloud dependency this does not yet
-- earn, and PostgreSQL works identically on a laptop and on Cloud Run.
--
-- A table of its own rather than a column on `covers`, because the gallery lists
-- covers constantly and must never read image bytes to do it.
CREATE TABLE cover_images (
    cover_id     TEXT PRIMARY KEY REFERENCES covers(id) ON DELETE CASCADE,
    bytes        BYTEA NOT NULL,
    content_type TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);

-- JPEG and PNG are already compressed, so the default EXTENDED storage would
-- spend CPU on both writes and reads to achieve nothing. EXTERNAL keeps the
-- out-of-line TOAST storage and skips the compression attempt.
ALTER TABLE cover_images ALTER COLUMN bytes SET STORAGE EXTERNAL;

-- +goose Down
DROP TABLE cover_images;
