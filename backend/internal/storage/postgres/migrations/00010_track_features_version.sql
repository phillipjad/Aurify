-- +goose Up
-- Which build of the feature extractor produced a row.
-- See docs/adr/0024-feature-cache-versioning.md.
--
-- This is the lever for invalidating the cache without deleting it: a row
-- carrying a lower number than domain.FeaturesVersion is treated as a miss,
-- re-fetched, and overwritten in place.
--
-- Deliberately not a DELETE. A delete is not safe during a rolling deploy. The
-- old instances are still serving while the migration runs, and they would
-- refill the rows they emptied in the old shape, which the new instances then
-- read as complete. A version number the old build cannot write is the only
-- invalidation that survives both builds running at once.
--
-- Two ways to pull it, and neither throws a row away:
--
--   * Bump domain.FeaturesVersion. Invalidates everything written by every
--     earlier build, and needs no migration at all. This is the usual one.
--   * UPDATE track_features SET features_version = 0. Forces the same thing
--     from SQL, for when a schema change is what prompted the re-analysis.
--
-- Existing rows default to 0, which is the point: none of them carry the onset
-- rate added in 00009, so all of them are stale by definition and will refill
-- themselves as playlists are analyzed.
ALTER TABLE track_features ADD COLUMN features_version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE track_features DROP COLUMN features_version;
