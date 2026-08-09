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
	measured := domain.AudioFeatures{Danceability: 1, Valence: 0.5, TempoBPM: 140, Present: true}
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

	// The estimator answers in [0,1] and never reports tempo, so blending its
	// zero in would quietly halve a real BPM.
	t.Run("tempo stays measured", func(t *testing.T) {
		if got := BlendFeatures(measured, estimated, 0.25); got.TempoBPM != 140 {
			t.Errorf("TempoBPM = %v, want the measured 140", got.TempoBPM)
		}
	})

	// CoverageWeight only reaches 0 when nothing matched, and then there is no
	// measurement to keep: the estimate is the whole answer, as it is today.
	t.Run("nothing measured yields the estimate", func(t *testing.T) {
		if got := BlendFeatures(domain.AudioFeatures{}, estimated, 0); got != estimated {
			t.Errorf("blend = %+v, want the estimate %+v", got, estimated)
		}
	})

	// A failed or disabled estimate must not dilute a real measurement toward
	// the zero value, which is the bug ADR 0018 exists to fix.
	t.Run("no estimate yields the measurement", func(t *testing.T) {
		if got := BlendFeatures(measured, domain.AudioFeatures{}, 0.25); got != measured {
			t.Errorf("blend = %+v, want the measurement %+v", got, measured)
		}
	})
}
