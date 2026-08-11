-- +goose Up
-- One `covers` row per playlist, with every finished run kept as a revision
-- (see docs/adr/0022-covers-by-playlist.md).
--
-- The order below matters. `covers` has no UNIQUE (user_id, platform,
-- playlist_id) today, which is exactly why regenerating a playlist produced a
-- second tile, so the duplicates have to be collapsed before the constraint can
-- be added. Adding it first fails immediately on any database with real data.

CREATE TABLE cover_revisions (
    id           TEXT PRIMARY KEY,
    cover_id     TEXT NOT NULL REFERENCES covers(id) ON DELETE CASCADE,
    -- What the UI calls the run: "Revision #28". Assigned here rather than
    -- derived from position in the list, because position is not stable:
    -- deleting one run renumbers every older one, so a label on an open page
    -- and a link to a revision would both start pointing somewhere else.
    --
    -- Failed runs take a number too. They are rows, and skipping them would put
    -- the same instability back in by another route.
    revision_number INT NOT NULL,
    -- Terminal only: ready | failed. In-flight lifecycle stays on `covers`, so
    -- the notify trigger and the SSE stream are untouched by this change.
    status       TEXT NOT NULL,
    prompt       TEXT NOT NULL DEFAULT '',
    analysis     JSONB NOT NULL DEFAULT '{}'::jsonb,
    error        TEXT NOT NULL DEFAULT '',
    -- The finish time, not the start time: a revision is written once, when the
    -- run reaches a terminal status.
    completed_at TIMESTAMPTZ NOT NULL,
    -- Only one run per cover is ever in flight (StartCoverRun), so the writer
    -- can take MAX + 1 without a sequence. This is what makes that safe to rely
    -- on rather than merely likely.
    UNIQUE (cover_id, revision_number)
);

-- Serves both readers: the lateral join that picks a cover's current artwork,
-- and the detail page's run history.
CREATE INDEX idx_cover_revisions_cover ON cover_revisions (cover_id, completed_at DESC, id DESC);

-- Every finished run so far becomes a revision, keeping its own id. That is
-- what keeps the stored bytes reachable: `cover_images` is keyed by the old
-- cover id, so re-pointing it at `cover_revisions` below is a rename rather
-- than a data migration, and every image_url already handed out still resolves.
--
-- The surviving cover per group is the newest run, so `/covers/{id}` links to
-- the most recent generation of each playlist keep working too.
WITH survivor AS (
    SELECT DISTINCT ON (user_id, platform, playlist_id)
           id AS cover_id, user_id, platform, playlist_id
    FROM covers
    ORDER BY user_id, platform, playlist_id, created_at DESC, id DESC
)
INSERT INTO cover_revisions (
    id, cover_id, revision_number, status, prompt, analysis, error, completed_at
)
SELECT c.id, s.cover_id,
       -- Oldest run is #1, so the numbers read as the history's own order.
       row_number() OVER (PARTITION BY s.cover_id ORDER BY c.updated_at, c.id),
       c.status, c.prompt, c.analysis, c.error, c.updated_at
FROM covers c
JOIN survivor s USING (user_id, platform, playlist_id)
WHERE c.status IN ('ready', 'failed');

-- Re-key the bytes onto the run that produced them. A rename rather than a new
-- table, so the STORAGE EXTERNAL setting from 00005 (which skips a pointless
-- compression pass over an already-compressed JPEG) survives.
ALTER TABLE cover_images RENAME COLUMN cover_id TO revision_id;
ALTER TABLE cover_images DROP CONSTRAINT cover_images_cover_id_fkey;

-- A crash between storing the bytes and saving the cover as ready leaves an
-- image whose run never reached a terminal status, so it has no revision. Those
-- are unreachable by any route and would block the new foreign key.
DELETE FROM cover_images WHERE revision_id NOT IN (SELECT id FROM cover_revisions);

ALTER TABLE cover_images
    ADD CONSTRAINT cover_images_revision_id_fkey
    FOREIGN KEY (revision_id) REFERENCES cover_revisions(id) ON DELETE CASCADE;

-- Collapse the duplicates: keep the newest run per playlist as the identity.
-- Their revisions already point at the survivor, so the cascade takes nothing.
DELETE FROM covers c
WHERE EXISTS (
    SELECT 1 FROM covers other
    WHERE other.user_id = c.user_id
      AND other.platform = c.platform
      AND other.playlist_id = c.playlist_id
      AND (other.created_at, other.id) > (c.created_at, c.id)
);

-- The run's output now lives on the revision. Keeping a copy on `covers` is
-- what would blank the tile mid-regeneration, which is half of what this change
-- is for; the current artwork is read as "the newest revision that is ready".
ALTER TABLE covers
    DROP COLUMN prompt,
    DROP COLUMN image_url,
    DROP COLUMN analysis;

ALTER TABLE covers ADD CONSTRAINT covers_user_platform_playlist_key
    UNIQUE (user_id, platform, playlist_id);

-- Keyset pagination orders by (updated_at DESC, id DESC): a regeneration moves
-- a playlist to the front mid-scroll, and offset paging duplicates or skips
-- tiles when that happens.
DROP INDEX idx_covers_user_created;
CREATE INDEX idx_covers_user_updated ON covers (user_id, updated_at DESC, id DESC);

-- The stuck-cover sweep runs every 30 seconds and asks the one question the
-- index above cannot answer, because it spans every user: which covers are
-- non-terminal and stale? Partial, so it holds only the handful of rows that are
-- actually mid-generation rather than a row per playlist per user.
CREATE INDEX idx_covers_in_flight ON covers (updated_at)
    WHERE status IN ('pending', 'analyzing', 'generating');

-- +goose Down
-- Restores the shape, not the history: the runs collapsed above are gone, and
-- so is any image belonging to one of them.
ALTER TABLE covers
    ADD COLUMN prompt    TEXT NOT NULL DEFAULT '',
    ADD COLUMN image_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN analysis  JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE covers c
SET prompt = r.prompt, analysis = r.analysis
FROM (
    SELECT DISTINCT ON (cover_id) cover_id, prompt, analysis
    FROM cover_revisions
    WHERE status = 'ready'
    ORDER BY cover_id, completed_at DESC, id DESC
) r
WHERE r.cover_id = c.id;

ALTER TABLE cover_images DROP CONSTRAINT cover_images_revision_id_fkey;
ALTER TABLE cover_images RENAME COLUMN revision_id TO cover_id;
DELETE FROM cover_images WHERE cover_id NOT IN (SELECT id FROM covers);
ALTER TABLE cover_images
    ADD CONSTRAINT cover_images_cover_id_fkey
    FOREIGN KEY (cover_id) REFERENCES covers(id) ON DELETE CASCADE;

UPDATE covers SET image_url = '/api/v1/covers/' || id || '/image'
WHERE EXISTS (SELECT 1 FROM cover_images i WHERE i.cover_id = covers.id);

DROP TABLE cover_revisions;
ALTER TABLE covers DROP CONSTRAINT covers_user_platform_playlist_key;
DROP INDEX idx_covers_in_flight;
DROP INDEX idx_covers_user_updated;
CREATE INDEX idx_covers_user_created ON covers (user_id, created_at DESC);
