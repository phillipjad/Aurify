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

// TestNoLyricsIsNotANeutralLyric: a playlist nobody wrote lyrics for used to be
// handed an invented neutral one, worth 0.2 of melancholy on its own, because
// polarity was normalized into [0,1] and its neutral was 0.5.
func TestNoLyricsIsNotANeutralLyric(t *testing.T) {
	// Every track in a major key, nothing else measured, no lyrics. Melancholic
	// has nothing to read; euphoric is the only live dimension and takes the
	// whole palette.
	got := weightsByDimension(BuildPalette(
		domain.AudioFeatures{Tonality: 1, Present: true},
		domain.Sentiment{},
	))
	if got["melancholic"] != 0 {
		t.Errorf("melancholic = %v on an all-major playlist with no lyrics, want 0", got["melancholic"])
	}
	if math.Abs(got["euphoric"]-1) > 1e-9 {
		t.Errorf("euphoric = %v, want the whole palette: it is the only live dimension", got["euphoric"])
	}
}

// The two are never both lit: whichever side of neutral the playlist is on takes
// the weight, and the other reads zero. Together they used to be a fixed 45% of
// every palette, mean 0.450 measured over the 61 revisions that carried
// features, while moving by two and four points respectively.
func TestBrightAndDarkAreNeverBothLit(t *testing.T) {
	for _, tone := range []float64{-1, -0.42, -0.01, 0, 0.01, 0.42, 1} {
		got := weightsByDimension(BuildPalette(
			domain.AudioFeatures{Tonality: tone, Danceability: 0.5, Present: true},
			domain.Sentiment{},
		))
		bright, dark := got["euphoric"], got["melancholic"]
		if bright > 0 && dark > 0 {
			t.Errorf("tonality %v lit both: euphoric %v, melancholic %v", tone, bright, dark)
		}
		// A colour cannot take a negative share of a cover. The dark half of
		// the axis is a negative brightness, so this is what stops the sign
		// reaching the palette rather than the weight.
		if bright < 0 || dark < 0 {
			t.Errorf("tonality %v produced a negative weight: euphoric %v, melancholic %v",
				tone, bright, dark)
		}
		if tone < 0 && dark <= 0 {
			t.Errorf("tonality %v is a minor-leaning playlist but melancholic is %v", tone, dark)
		}
		if tone > 0 && bright <= 0 {
			t.Errorf("tonality %v is a major-leaning playlist but euphoric is %v", tone, bright)
		}
	}

	// And an evenly split playlist spends nothing on either, leaving the room
	// to the dimensions that did measure something.
	even := weightsByDimension(BuildPalette(
		domain.AudioFeatures{Tonality: 0, Danceability: 0.5, Present: true},
		domain.Sentiment{},
	))
	if math.Abs(even["danceable"]-1) > 1e-9 {
		t.Errorf("danceable = %v on an evenly split playlist, want the whole palette", even["danceable"])
	}
}

// The lyrics keep a minority share of the axis rather than taking it when the
// key is unmeasured. A cover should not be decided by a word count.
func TestLyricsCannotOutweighTheKey(t *testing.T) {
	// Bleakest possible lyrics against an all-major playlist.
	got := weightsByDimension(BuildPalette(
		domain.AudioFeatures{Tonality: 1, Present: true},
		domain.Sentiment{Polarity: -1, HasLyrics: true},
	))
	if got["melancholic"] != 0 {
		t.Errorf("melancholic = %v, want 0: the lyrics may darken the axis, not flip it", got["melancholic"])
	}

	// They still move it: brightness falls from 1 to 0.5, so euphoric gives up
	// half its share to the one other live dimension.
	mixed := weightsByDimension(BuildPalette(
		domain.AudioFeatures{Tonality: 1, Danceability: 0.5, Present: true},
		domain.Sentiment{Polarity: -1, HasLyrics: true},
	))
	if math.Abs(mixed["euphoric"]-0.5) > 1e-9 {
		t.Errorf("euphoric = %v against an equal danceable, want 0.5", mixed["euphoric"])
	}
}

func weightsByDimension(palette []domain.ColorWeight) map[string]float64 {
	out := make(map[string]float64, len(palette))
	for _, c := range palette {
		out[c.Dimension] = c.Weight
	}
	return out
}
