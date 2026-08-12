package postgres

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// TrackFeatureRepository is the PostgreSQL-backed ports.TrackFeatureRepository.
//
// Batched for the same reason LyricsRepository is: a generation resolves a
// playlist at a time, and MusicBrainz's one-request-per-second limit makes
// re-asking expensive in a way a per-track interface would hide.
type TrackFeatureRepository struct {
	q *db.Queries
}

var _ ports.TrackFeatureRepository = (*TrackFeatureRepository)(nil)

// FindMany returns cached entries for the given keys. Keys with no entry are
// absent from the map rather than an error: a miss is the ordinary case.
func (r *TrackFeatureRepository) FindMany(
	ctx context.Context,
	keys []string,
) (map[string]domain.CachedFeatures, error) {
	if len(keys) == 0 {
		return map[string]domain.CachedFeatures{}, nil
	}

	rows, err := r.q.FindTrackFeaturesByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}

	out := make(map[string]domain.CachedFeatures, len(rows))
	for _, row := range rows {
		out[row.TrackKey] = domain.CachedFeatures{
			Key: row.TrackKey,
			Features: domain.AudioFeatures{
				Danceability: row.Danceability,
				Acousticness: row.Acousticness,
				Energy:       row.Energy,
				Valence:      row.Valence,
				OnsetRate:    row.OnsetRate,
				Present:      row.Present,
			},
			FetchedAt: timeFromTS(row.FetchedAt),
			Version:   int(row.FeaturesVersion),
		}
	}
	return out, nil
}

// SaveMany upserts a batch of entries in one statement.
func (r *TrackFeatureRepository) SaveMany(ctx context.Context, entries []domain.CachedFeatures) error {
	if len(entries) == 0 {
		return nil
	}

	// Deduplicated by key: PostgreSQL refuses an ON CONFLICT DO UPDATE that
	// touches the same row twice in one statement, and a playlist holding the
	// same song twice is ordinary rather than exotic.
	seen := make(map[string]struct{}, len(entries))
	params := db.SaveTrackFeaturesParams{}
	for _, entry := range entries {
		if _, dup := seen[entry.Key]; dup {
			continue
		}
		seen[entry.Key] = struct{}{}
		params.Keys = append(params.Keys, entry.Key)
		params.Present = append(params.Present, entry.Features.Present)
		params.Danceability = append(params.Danceability, entry.Features.Danceability)
		params.Acousticness = append(params.Acousticness, entry.Features.Acousticness)
		params.Energy = append(params.Energy, entry.Features.Energy)
		params.Valence = append(params.Valence, entry.Features.Valence)
		params.OnsetRate = append(params.OnsetRate, entry.Features.OnsetRate)
		// Stamped here rather than taken from the entry: what a row was written
		// under is a fact about this build, not something a caller should be
		// able to claim. Writing an inflated version would make a stale row look
		// current and defeat the whole invalidation.
		params.FeaturesVersion = append(params.FeaturesVersion, int32(domain.FeaturesVersion))
		params.FetchedAt = append(params.FetchedAt, tsFromTime(entry.FetchedAt))
	}
	return r.q.SaveTrackFeatures(ctx, params)
}
