-- +goose Up
-- Cached lyric lookups, so the same track is fetched from lrclib once rather
-- than once per playlist per run. See docs/adr/0015-lyrics-cache-and-failure-policy.md.
--
-- lrclib is a free community service with no documented rate limit and no
-- Retry-After header to coordinate with, so restraint has to come from our side.
-- Before this table, a 194-track playlist meant 194 outbound requests every
-- single time it was aurified.
CREATE TABLE lyrics_cache (
    -- Lowercased, whitespace-collapsed "artist\ntitle". Deliberately excludes
    -- duration so the same song from different sources shares one entry.
    track_key  TEXT PRIMARY KEY,
    -- COMPRESSION lz4 rather than the cluster default of pglz: it decompresses
    -- several times faster at a comparable ratio. It only engages once a row
    -- passes the ~2KB TOAST threshold, so long lyrics are compressed and short
    -- ones stay inline. Kept as TEXT rather than a compressed BYTEA so the
    -- column stays readable in psql and available to full-text search later.
    lyrics     TEXT COMPRESSION lz4 NOT NULL DEFAULT '',
    -- False records that lrclib has no lyrics for this track. Caching the
    -- absence is the single highest-value part of this table: YouTube Music
    -- libraries are full of titles that will never match, and without this each
    -- one costs a request on every run, forever.
    found      BOOLEAN NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL
);

-- Supports expiring negative entries, which are the only ones that expire:
-- lyrics are immutable, but a track missing today may be added to lrclib later.
CREATE INDEX idx_lyrics_cache_negative ON lyrics_cache (fetched_at) WHERE NOT found;

-- +goose Down
DROP TABLE lyrics_cache;
