-- +goose Up
-- Cached acoustic features, so a track is matched against MusicBrainz once
-- rather than once per generation. See docs/adr/0018-audio-features.md.
--
-- MusicBrainz rate-limits anonymous callers to one request per second and
-- blocks rather than throttles, so this cache is what makes the lookup usable
-- at all: without it a 91-track playlist would spend 91 seconds matching on
-- every single generation.
CREATE TABLE track_features (
    -- The same normalized "artist\ntitle" as lyrics_cache, so both caches agree
    -- on what counts as the same song.
    track_key    TEXT PRIMARY KEY,
    -- False records that the track could not be matched, or matched a recording
    -- nobody ever analyzed. Caching the absence is the highest-value part of
    -- this table: a YouTube Music library is full of things that are not songs,
    -- and each costs a second of rate limit every run without it.
    present      BOOLEAN NOT NULL,
    danceability DOUBLE PRECISION NOT NULL DEFAULT 0,
    acousticness DOUBLE PRECISION NOT NULL DEFAULT 0,
    energy       DOUBLE PRECISION NOT NULL DEFAULT 0,
    valence      DOUBLE PRECISION NOT NULL DEFAULT 0,
    fetched_at   TIMESTAMPTZ NOT NULL
);

-- Supports expiring negative entries, which are the only ones that expire:
-- a recording's audio does not change, but a track absent from MusicBrainz
-- today may be added later.
CREATE INDEX idx_track_features_negative ON track_features (fetched_at) WHERE NOT present;

-- +goose Down
DROP TABLE track_features;
