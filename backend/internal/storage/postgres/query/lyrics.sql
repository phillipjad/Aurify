-- name: FindLyricsByKeys :many
-- One round trip for a whole playlist. Looking these up one key at a time is
-- what this table exists to avoid: a 194-track playlist would otherwise open
-- 194 conversations with the database before doing any work.
SELECT track_key, lyrics, found, fetched_at
FROM lyrics_cache
WHERE track_key = ANY(@keys::TEXT[]);

-- name: SaveLyrics :exec
-- Batched upsert, so a generation writes once however many misses it had.
-- ON CONFLICT overwrites rather than skipping, which is what lets a negative
-- entry become a positive one when lrclib later gains the track.
INSERT INTO lyrics_cache (track_key, lyrics, found, fetched_at)
SELECT
    unnest(@keys::TEXT[]),
    unnest(@lyrics::TEXT[]),
    unnest(@found::BOOLEAN[]),
    unnest(@fetched_at::TIMESTAMPTZ[])
ON CONFLICT (track_key) DO UPDATE SET
    lyrics = EXCLUDED.lyrics,
    found = EXCLUDED.found,
    fetched_at = EXCLUDED.fetched_at;
