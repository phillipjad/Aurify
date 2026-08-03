package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// LyricsRepository is the PostgreSQL-backed ports.LyricsRepository.
//
// Both methods take and return whole batches. A generation resolves a playlist
// at a time, so a per-track interface would put one round trip per track in
// front of work that needs exactly two.
type LyricsRepository struct {
	q *db.Queries
}

var _ ports.LyricsRepository = (*LyricsRepository)(nil)

// FindMany returns the cached entries for the given keys. Keys with no entry are
// absent from the map rather than an error: a miss is the ordinary case.
func (r *LyricsRepository) FindMany(
	ctx context.Context,
	keys []string,
) (map[string]domain.CachedLyrics, error) {
	if len(keys) == 0 {
		return map[string]domain.CachedLyrics{}, nil
	}

	rows, err := r.q.FindLyricsByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}

	out := make(map[string]domain.CachedLyrics, len(rows))
	for _, row := range rows {
		out[row.TrackKey] = domain.CachedLyrics{
			Key:       row.TrackKey,
			Lyrics:    row.Lyrics,
			Found:     row.Found,
			FetchedAt: timeFromTS(row.FetchedAt),
		}
	}
	return out, nil
}

// SaveMany upserts a batch of entries in one statement.
func (r *LyricsRepository) SaveMany(ctx context.Context, entries []domain.CachedLyrics) error {
	if len(entries) == 0 {
		return nil
	}

	params := db.SaveLyricsParams{
		Keys:      make([]string, len(entries)),
		Lyrics:    make([]string, len(entries)),
		Found:     make([]bool, len(entries)),
		FetchedAt: make([]pgtype.Timestamptz, len(entries)),
	}
	for i, entry := range entries {
		params.Keys[i] = entry.Key
		params.Lyrics[i] = entry.Lyrics
		params.Found[i] = entry.Found
		params.FetchedAt[i] = tsFromTime(entry.FetchedAt)
	}
	return r.q.SaveLyrics(ctx, params)
}
