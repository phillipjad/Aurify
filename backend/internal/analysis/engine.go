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
func (e *Engine) Analyze(
	ctx context.Context,
	playlistID string,
	tracks []domain.Track,
	sentiments []domain.Sentiment,
) (domain.PlaylistAnalysis, error) {
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

// measuredCoverageFloor and measuredCountFloor are what a measured mean has to
// clear before it stands on its own: half the playlist, and enough tracks for a
// mean to mean anything. Neither alone is sufficient, and the numbers are
// argued from measurements in docs/adr/0020-coverage-weighted-features.md.
const (
	measuredCoverageFloor = 0.5
	measuredCountFloor    = 8
)

// CoverageWeight is how much of the palette the measurement should carry, in
// [0,1]. The rest belongs to the estimate.
func CoverageWeight(analyzed, total int) float64 {
	if total <= 0 {
		return 0
	}
	share := float64(analyzed) / float64(total) / measuredCoverageFloor
	count := float64(analyzed) / measuredCountFloor
	return min(1, share, count)
}

// BlendFeatures mixes measured features toward estimated ones, with w the
// weight on the measurement. w == 0 returns the estimate untouched, which is
// exactly the behaviour when nothing at all matched.
func BlendFeatures(measured, estimated domain.AudioFeatures, w float64) domain.AudioFeatures {
	if !measured.Present {
		return estimated
	}
	if !estimated.Present {
		return measured
	}
	mix := func(m, e float64) float64 { return m*w + e*(1-w) }
	return domain.AudioFeatures{
		Acousticness:     mix(measured.Acousticness, estimated.Acousticness),
		Danceability:     mix(measured.Danceability, estimated.Danceability),
		Energy:           mix(measured.Energy, estimated.Energy),
		Instrumentalness: mix(measured.Instrumentalness, estimated.Instrumentalness),
		Liveness:         mix(measured.Liveness, estimated.Liveness),
		Loudness:         mix(measured.Loudness, estimated.Loudness),
		Speechiness:      mix(measured.Speechiness, estimated.Speechiness),
		Valence:          mix(measured.Valence, estimated.Valence),
		// Not blended, and the one exemption left. The estimator answers in
		// [0,1]; onsets per second is not that, and a text model has nothing to
		// ground a DSP event rate in, so it is never asked for one. Mixing in
		// the zero it therefore always reports would read a thinly matched
		// playlist as motionless rather than as unmeasured.
		OnsetRate: measured.OnsetRate,
		Present:   true,
	}
}

// meanAudioFeatures averages features over tracks that actually carry them. It
// returns the mean and the count of tracks that contributed.
func meanAudioFeatures(tracks []domain.Track) (domain.AudioFeatures, int) {
	var sum domain.AudioFeatures
	var n, paced int
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
		// Averaged over its own count. The rhythm data comes from a second
		// endpoint that can miss on its own, and a track classified but never
		// analyzed for rhythm reports zero: divided by n it would read as a
		// motionless track dragging the playlist down rather than as silence
		// about one.
		if t.Features.OnsetRate > 0 {
			paced++
			sum.OnsetRate += t.Features.OnsetRate
		}
	}
	if n == 0 {
		return domain.AudioFeatures{}, 0
	}
	inv := 1 / float64(n)
	out := domain.AudioFeatures{
		Acousticness:     sum.Acousticness * inv,
		Danceability:     sum.Danceability * inv,
		Energy:           sum.Energy * inv,
		Instrumentalness: sum.Instrumentalness * inv,
		Liveness:         sum.Liveness * inv,
		Loudness:         sum.Loudness * inv,
		Speechiness:      sum.Speechiness * inv,
		Valence:          sum.Valence * inv,
		Present:          true,
	}
	if paced > 0 {
		out.OnsetRate = sum.OnsetRate / float64(paced)
	}
	return out, n
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
