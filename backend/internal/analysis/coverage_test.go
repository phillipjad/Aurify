package analysis

import (
	"math"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

func TestCoverageWeight(t *testing.T) {
	cases := []struct {
		name            string
		analyzed, total int
		want            float64
	}{
		{"nothing matched leaves the estimate whole", 0, 100, 0},
		{"the EDM case: 6 of 192 is barely a sample", 6, 192, 0.0625},
		{"Chill Stratovarius, 2 of 7: too few, whatever the share", 2, 7, 0.25},
		{"both floors cleared", 16, 32, 1},
		{"share alone is not enough: 1 of 2 is half a playlist", 1, 2, 0.125},
		{"count alone is not enough: 10 of 100 is plenty of tracks", 10, 100, 0.2},
		{"an empty playlist cannot be measured", 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CoverageWeight(c.analyzed, c.total); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("CoverageWeight(%d, %d) = %v, want %v", c.analyzed, c.total, got, c.want)
			}
		})
	}
}

func TestBlendFeatures(t *testing.T) {
	measured := domain.AudioFeatures{Danceability: 1, Valence: 0.5, OnsetRate: 4, Present: true}
	estimated := domain.AudioFeatures{Danceability: 0, Valence: 0.1, Present: true}

	t.Run("w is the weight on the measurement", func(t *testing.T) {
		got := BlendFeatures(measured, estimated, 0.25)
		if got.Danceability != 0.25 {
			t.Errorf("danceability = %v, want 0.25", got.Danceability)
		}
		if !got.Present {
			t.Error("a blend of two present features is present")
		}
	})

	// The estimator answers in [0,1] and is never asked for an onset rate, so
	// blending its zero in would quietly quarter a real one. Blended at 0.25
	// this would read 1, which normalizePace floors to no weight at all: a
	// well-paced playlist would lose its driving color for being thinly matched.
	t.Run("pace stays measured", func(t *testing.T) {
		got := BlendFeatures(measured, estimated, 0.25)
		if got.OnsetRate != 4 {
			t.Errorf("OnsetRate = %v, want the measured 4", got.OnsetRate)
		}
		if w := normalizePace(got.OnsetRate); w <= 0 {
			t.Errorf("normalizePace(%v) = %v, want a real weight", got.OnsetRate, w)
		}
		if diluted := normalizePace(measured.OnsetRate * 0.25); diluted != 0 {
			t.Errorf("blending in the estimator's zero leaves %v, want it to prove the floor", diluted)
		}
	})

	// CoverageWeight only reaches 0 when nothing matched, and then there is no
	// measurement to keep: the estimate is the whole answer, as it is today.
	// Its brightness is expressed on the axis the palette reads, which is the
	// one thing the estimate cannot say in its own words.
	t.Run("nothing measured yields the estimate", func(t *testing.T) {
		want := estimated
		want.Tonality = 2*estimated.Valence - 1
		if got := BlendFeatures(domain.AudioFeatures{}, estimated, 0); got != want {
			t.Errorf("blend = %+v, want the estimate %+v", got, want)
		}
	})

	// A failed or disabled estimate must not dilute a real measurement toward
	// the zero value, which is the bug ADR 0018 exists to fix.
	t.Run("no estimate yields the measurement", func(t *testing.T) {
		if got := BlendFeatures(measured, domain.AudioFeatures{}, 0.25); got != measured {
			t.Errorf("blend = %+v, want the measurement %+v", got, measured)
		}
	})

	// The estimator is never asked for a key, but valence and tonality are the
	// same bright-to-dark axis on different scales, so its valence is what
	// stands in. Without it a playlist released after AcousticBrainz stopped
	// collecting would have no bright or dark colour at all.
	t.Run("the estimator's valence stands in for a key", func(t *testing.T) {
		// Estimated valence 0.1 is tonality -0.8; measured is +1, all major.
		got := BlendFeatures(
			domain.AudioFeatures{Tonality: 1, Present: true},
			domain.AudioFeatures{Valence: 0.1, Present: true},
			0.25,
		)
		if want := 1*0.25 + -0.8*0.75; math.Abs(got.Tonality-want) > 1e-9 {
			t.Errorf("Tonality = %v, want %v", got.Tonality, want)
		}

		// And with nothing measured at all it is the whole answer, which is the
		// case the mapping exists for: a playlist released after AcousticBrainz
		// stopped collecting would otherwise show neither colour.
		whole := BlendFeatures(
			domain.AudioFeatures{},
			domain.AudioFeatures{Valence: 1, Present: true},
			0,
		)
		if whole.Tonality != 1 {
			t.Errorf("Tonality = %v, want 1 from an estimate of full valence", whole.Tonality)
		}
	})
}
