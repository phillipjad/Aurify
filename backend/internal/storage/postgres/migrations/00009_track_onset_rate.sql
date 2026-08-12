-- +goose Up
-- Onsets per second, from AcousticBrainz's low-level endpoint, feeding the
-- driving dimension of the palette. See docs/adr/0023-pace-from-onset-rate.md.
--
-- Stored raw rather than normalized: the curve that turns it into a weight was
-- chosen from 19 recordings and will want retuning, and re-deriving it from the
-- rate costs nothing while re-fetching would cost a second of MusicBrainz rate
-- limit per cached track.
--
-- Zero is "nobody measured one" rather than a motionless track, which is why the
-- default is safe for every row already here: those rows predate the low-level
-- fetch, and the palette reads zero as no weight.
--
-- Rows already cached have no rate and would otherwise be served forever, since
-- they are Present and nothing about them looks wrong. 00010 is what retires
-- them.
ALTER TABLE track_features ADD COLUMN onset_rate DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE track_features DROP COLUMN onset_rate;
