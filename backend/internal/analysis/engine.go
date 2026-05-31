// Package analysis aggregates per-track audio features and lyric sentiment for
// a whole playlist and derives the weighted color palette used to drive cover
// generation.
package analysis

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Engine is the default AnalysisEngine implementation.
type Engine struct{}

var _ ports.AnalysisEngine = (*Engine)(nil)

// NewEngine constructs an analysis Engine.
func NewEngine() *Engine { return &Engine{} }

// Analyze computes mean audio features and mean sentiment across the playlist,
// then builds the weighted palette from those aggregates.
func (e *Engine) Analyze(ctx context.Context, playlistID string, tracks []domain.Track, sentiments []domain.Sentiment) (domain.PlaylistAnalysis, error) {
	_ = ctx

	meanFeatures, analyzed := meanAudioFeatures(tracks)
	meanSentiment := meanSentiment(sentiments)

	return domain.PlaylistAnalysis{
		PlaylistID:    playlistID,
		TrackCount:    len(tracks),
		AnalyzedCount: analyzed,
		MeanFeatures:  meanFeatures,
		MeanSentiment: meanSentiment,
		Palette:       BuildPalette(meanFeatures, meanSentiment),
	}, nil
}

// meanAudioFeatures averages features over tracks that actually carry them. It
// returns the mean and the count of tracks that contributed.
func meanAudioFeatures(tracks []domain.Track) (domain.AudioFeatures, int) {
	var sum domain.AudioFeatures
	var n int
	for _, t := range tracks {
		if !t.Features.Present {
			continue
		}
		n++
		sum.Acousticness += t.Features.Acousticness
		sum.Danceability += t.Features.Danceability
		sum.Energy += t.Features.Energy
		sum.Instrumentalness += t.Features.Instrumentalness
		sum.Liveness += t.Features.Liveness
		sum.Loudness += t.Features.Loudness
		sum.Speechiness += t.Features.Speechiness
		sum.Valence += t.Features.Valence
		sum.TempoBPM += t.Features.TempoBPM
	}
	if n == 0 {
		return domain.AudioFeatures{}, 0
	}
	inv := 1 / float64(n)
	return domain.AudioFeatures{
		Acousticness:     sum.Acousticness * inv,
		Danceability:     sum.Danceability * inv,
		Energy:           sum.Energy * inv,
		Instrumentalness: sum.Instrumentalness * inv,
		Liveness:         sum.Liveness * inv,
		Loudness:         sum.Loudness * inv,
		Speechiness:      sum.Speechiness * inv,
		Valence:          sum.Valence * inv,
		TempoBPM:         sum.TempoBPM * inv,
		Present:          true,
	}, n
}

// meanSentiment averages sentiment over tracks that had lyrics.
func meanSentiment(sentiments []domain.Sentiment) domain.Sentiment {
	var polarity, subjectivity float64
	var n int
	for _, s := range sentiments {
		if !s.HasLyrics {
			continue
		}
		n++
		polarity += s.Polarity
		subjectivity += s.Subjectivity
	}
	if n == 0 {
		return domain.Sentiment{HasLyrics: false}
	}
	inv := 1 / float64(n)
	return domain.Sentiment{
		Polarity:     polarity * inv,
		Subjectivity: subjectivity * inv,
		HasLyrics:    true,
	}
}
