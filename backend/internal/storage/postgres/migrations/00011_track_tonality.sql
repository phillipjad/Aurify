-- +goose Up
-- The two signals the palette gained in docs/adr/0025-palette-from-measured-signals.md.
--
-- tonality is the bright/dark axis in [-1,1]: +1 a recording in a major key, -1
-- a minor one, from tonal.key_scale on the low-level endpoint 00009 already
-- fetches. Signed rather than a proportion so that 0 means "nobody measured a
-- key", which is the same thing an evenly split playlist averages to and the
-- same thing the palette reads as no weight. That is why the default is safe for
-- every row already here.
--
-- instrumentalness has been on the model since the beginning and written by
-- nothing but the estimator. It now comes from the voice_instrumental
-- classifier, which ADR 0018 rejected on one country ballad and ADR 0025
-- reinstates on 24 recordings.
--
-- Rows already cached have neither and would otherwise be served forever, since
-- they are present and nothing about them looks wrong. Bumping
-- domain.FeaturesVersion to 2 is what retires them, per
-- docs/adr/0024-feature-cache-versioning.md, and no DELETE belongs here.
ALTER TABLE track_features ADD COLUMN tonality DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE track_features ADD COLUMN instrumentalness DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE track_features DROP COLUMN instrumentalness;
ALTER TABLE track_features DROP COLUMN tonality;
