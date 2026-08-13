package analysis

import (
	"math"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// TestUnknownPlaylistHasNoPalette is the case measured on the dev database: 26
// of 87 completed revisions carried no features, and every one of them rendered
// as roughly 76% melancholic and 24% euphoric, because those two formulas carry
// constant terms and the other six dimensions were zero.
func TestUnknownPlaylistHasNoPalette(t *testing.T) {
	for _, c := range []struct {
		name string
		s    domain.Sentiment
	}{
		{"nothing at all", domain.Sentiment{}},
		// Sixteen of the 26 were this: lyrics resolved, nothing matched
		// AcousticBrainz, and no estimate to fall back on. A palette built from
		// polarity alone would normalize a couple of lexicon hits into the
		// whole cover.
		{"lyrics but no features", domain.Sentiment{Polarity: -0.8, HasLyrics: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := BuildPalette(domain.AudioFeatures{}, c.s); len(got) != 0 {
				t.Errorf("BuildPalette = %v, want no palette at all", got)
			}
		})
	}
}

// TestEstimatedFeaturesStillBuildAPalette holds the guard to what it is for.
// It asks whether anything is present, not whether it was measured, so a
// playlist the estimator spoke for still gets its colors.
func TestEstimatedFeaturesStillBuildAPalette(t *testing.T) {
	got := BuildPalette(
		domain.AudioFeatures{Energy: 0.6, Danceability: 0.4, Valence: 0.5, Present: true},
		domain.Sentiment{},
	)
	if len(got) != len(palette) {
		t.Fatalf("got %d dimensions, want all %d", len(got), len(palette))
	}
	var total float64
	for _, c := range got {
		total += c.Weight
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("weights sum to %v, want 1", total)
	}
}

// TestNoLyricsIsNotANeutralLyric: normalizePolarity(0) is 0.5, so a playlist
// nobody wrote lyrics for was handed an invented neutral one. It was worth 0.2
// of melancholy on its own, and pulled euphoric a quarter of the way down from
// whatever the music actually measured.
func TestNoLyricsIsNotANeutralLyric(t *testing.T) {
	// Full valence, nothing else measured, no lyrics. Melancholic has nothing
	// to read; euphoric is then the only live dimension and takes the palette.
	got := weightsByDimension(BuildPalette(
		domain.AudioFeatures{Valence: 1, Present: true},
		domain.Sentiment{},
	))
	if got["melancholic"] != 0 {
		t.Errorf("melancholic = %v at full measured valence with no lyrics, want 0", got["melancholic"])
	}
	if math.Abs(got["euphoric"]-1) > 1e-9 {
		t.Errorf("euphoric = %v, want the whole palette: it is the only live dimension", got["euphoric"])
	}
}

func weightsByDimension(palette []domain.ColorWeight) map[string]float64 {
	out := make(map[string]float64, len(palette))
	for _, c := range palette {
		out[c.Dimension] = c.Weight
	}
	return out
}
