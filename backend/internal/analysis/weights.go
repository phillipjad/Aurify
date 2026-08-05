package analysis

import (
	"sort"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// dimension associates a sonic/emotional dimension with a base color and a
// function that derives its raw weight in [0,1] from the aggregate signals.
//
// This is the heart of the "weighing" described in the product vision: a high
// mean liveness/energy contributes weight to the energetic color, a low valence
// (plus negative lyric polarity) pushes weight toward the melancholic color,
// and so on. Add or retune dimensions here.
type dimension struct {
	name  string
	hex   string
	score func(f domain.AudioFeatures, s domain.Sentiment) float64
}

var palette = []dimension{
	{"energetic", "#FF5A36", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		// Energy alone. This averaged in Liveness, but nothing Aurify can reach
		// measures liveness: AcousticBrainz has no such classifier, so it stayed
		// at zero and halved this dimension permanently. Danceable, scored from a
		// single feature, then won every playlist regardless of the music.
		// Averaging a real signal with a structural zero is not a compromise, it
		// is a bug.
		return f.Energy
	}},
	{"danceable", "#FFB23E", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		return f.Danceability
	}},
	{"euphoric", "#FFE15D", func(f domain.AudioFeatures, s domain.Sentiment) float64 {
		return (f.Valence + normalizePolarity(s.Polarity)) / 2
	}},
	{"organic", "#7FB069", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		return f.Acousticness
	}},
	{"introspective", "#4F86C6", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		return f.Instrumentalness
	}},
	{"melancholic", "#5C4D7D", func(f domain.AudioFeatures, s domain.Sentiment) float64 {
		return (1-f.Valence)*0.6 + (1-normalizePolarity(s.Polarity))*0.4
	}},
	{"intimate", "#C46BAE", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		return f.Speechiness
	}},
}

// BuildPalette derives the weighted color palette from the aggregate features
// and sentiment. Weights are normalized so they sum to 1, and the result is
// sorted by descending weight (dominant colors first).
func BuildPalette(f domain.AudioFeatures, s domain.Sentiment) []domain.ColorWeight {
	weights := make([]domain.ColorWeight, 0, len(palette))
	var total float64
	for _, d := range palette {
		w := clamp01(d.score(f, s))
		total += w
		weights = append(weights, domain.ColorWeight{Dimension: d.name, HexColor: d.hex, Weight: w})
	}
	if total > 0 {
		for i := range weights {
			weights[i].Weight /= total
		}
	}
	sort.SliceStable(weights, func(i, j int) bool {
		return weights[i].Weight > weights[j].Weight
	})
	return weights
}

// normalizePolarity maps sentiment polarity from [-1,1] into [0,1].
func normalizePolarity(p float64) float64 {
	return clamp01((p + 1) / 2)
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
