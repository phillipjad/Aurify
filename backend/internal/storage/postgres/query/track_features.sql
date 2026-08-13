-- name: FindTrackFeaturesByKeys :many
-- One round trip for a whole playlist, as with lyrics.
SELECT track_key, present, danceability, acousticness, energy, valence, instrumentalness, onset_rate, tonality, features_version, fetched_at
FROM track_features
WHERE track_key = ANY(@keys::TEXT[]);

-- name: SaveTrackFeatures :exec
-- Batched upsert. ON CONFLICT overwrites rather than skipping, so a negative
-- entry becomes positive once MusicBrainz gains the release.
INSERT INTO track_features (
    track_key, present, danceability, acousticness, energy, valence, instrumentalness, onset_rate, tonality, features_version, fetched_at
)
SELECT
    unnest(@keys::TEXT[]),
    unnest(@present::BOOLEAN[]),
    unnest(@danceability::DOUBLE PRECISION[]),
    unnest(@acousticness::DOUBLE PRECISION[]),
    unnest(@energy::DOUBLE PRECISION[]),
    unnest(@valence::DOUBLE PRECISION[]),
    unnest(@instrumentalness::DOUBLE PRECISION[]),
    unnest(@onset_rate::DOUBLE PRECISION[]),
    unnest(@tonality::DOUBLE PRECISION[]),
    unnest(@features_version::INTEGER[]),
    unnest(@fetched_at::TIMESTAMPTZ[])
ON CONFLICT (track_key) DO UPDATE SET
    present = EXCLUDED.present,
    danceability = EXCLUDED.danceability,
    acousticness = EXCLUDED.acousticness,
    energy = EXCLUDED.energy,
    valence = EXCLUDED.valence,
    instrumentalness = EXCLUDED.instrumentalness,
    onset_rate = EXCLUDED.onset_rate,
    tonality = EXCLUDED.tonality,
    features_version = EXCLUDED.features_version,
    fetched_at = EXCLUDED.fetched_at;
