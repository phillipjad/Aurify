package analysis

import (
	"context"
	"math"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// sample is one real AcousticBrainz recording: what its rhythm block reported,
// and the tempo its sheet music and every chart site agree on. Measured against
// the live API while deciding what the driving dimension should read; the whole
// table is the evidence for docs/adr/0023-pace-from-onset-rate.md.
type sample struct {
	name      string
	onsetRate float64
	bpm       float64
	published float64
}

// slowTracks and fastTracks are two playlists that differ in tempo and little
// else: mean published tempo 73 against 158.
var (
	slowTracks = []sample{
		{"Chopin - Nocturne op. 9 no. 2", 2.48, 129.2, 56},
		{"Brian Eno - An Ending (Ascent)", 0.66, 132.1, 0},
		{"Stratovarius - Forever", 1.26, 117.2, 68},
		{"Norah Jones - Don't Know Why", 2.66, 116.9, 87},
		{"Johnny Cash - Hurt", 2.67, 91.0, 82},
		{"Radiohead - Creep", 2.19, 84.8, 92},
		{"Enya - Orinoco Flow", 2.65, 114.9, 92},
		{"Simon & Garfunkel - The Sound of Silence", 2.41, 178.2, 106},
		{"Adele - Someone Like You", 3.22, 123.0, 67},
		{"Bill Withers - Ain't No Sunshine", 3.48, 143.9, 78},
	}
	fastTracks = []sample{
		{"Deadmau5 - Strobe", 3.39, 128.1, 128},
		{"Armin van Buuren - Communication", 4.59, 137.7, 138},
		{"Burial - Archangel", 5.28, 135.3, 138},
		{"Kendrick Lamar - HUMBLE.", 4.62, 150.2, 150},
		{"Pendulum - Propane Nightmares", 2.47, 86.7, 174},
		{"Ramones - Blitzkrieg Bop", 2.67, 123.1, 177},
		{"Bad Religion - American Jesus", 3.47, 96.2, 192},
		{"Slayer - Raining Blood", 2.43, 117.2, 210},
		{"Nirvana - Smells Like Teen Spirit", 2.21, 122.3, 117},
	}
)

func TestNormalizePace(t *testing.T) {
	cases := []struct {
		name      string
		onsetRate float64
		want      float64
	}{
		{"unmeasured is not motionless, it is no weight", 0, 0},
		{"the quietest thing sampled, beatless ambient", 0.66, 0},
		{"the floor itself", 1.5, 0},
		{"halfway", 3, 0.5},
		{"the ceiling itself", 4.5, 1},
		{"the busiest thing sampled saturates", 5.28, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizePace(c.onsetRate); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("normalizePace(%v) = %v, want %v", c.onsetRate, got, c.want)
			}
		})
	}
}

// TestPaceSeparatesPlaylists is issue #77's "done when": two playlists that
// differ mainly in tempo have to produce visibly different covers.
//
// Every feature but the rhythm is held identical between the two, so the whole
// difference in the palette is the driving dimension's doing.
func TestPaceSeparatesPlaylists(t *testing.T) {
	slow := drivingWeight(t, slowTracks)
	fast := drivingWeight(t, fastTracks)

	// A tenth of the whole palette moving hands is a different cover, not a
	// different shade of one. Measured: 0.104 against 0.207.
	if fast-slow < 0.05 {
		t.Errorf("driving weight: slow %.3f, fast %.3f, want the fast playlist ahead by 0.05", slow, fast)
	}
}

// TestTempoDoesNotSeparatePlaylists is the same two playlists under the choice
// this ADR rejected, and it is why the driving dimension does not read a BPM.
//
// It fails the moment someone points the dimension at rhythm.bpm instead: on
// these 19 recordings the slow playlist measures *faster* than the fast one,
// because Essentia folds tempo into a single octave and so does every other beat
// tracker. Their published tempos differ by 85 BPM.
func TestTempoDoesNotSeparatePlaylists(t *testing.T) {
	mean := func(tracks []sample, of func(sample) float64) float64 {
		var sum float64
		for _, s := range tracks {
			sum += of(s)
		}
		return sum / float64(len(tracks))
	}
	published := func(s sample) float64 { return s.published }
	measured := func(s sample) float64 { return s.bpm }

	if gap := mean(fastTracks, published) - mean(slowTracks, published); gap < 80 {
		t.Fatalf("the two sets differ by %.1f published BPM, want at least 80", gap)
	}
	if gap := mean(fastTracks, measured) - mean(slowTracks, measured); gap > 5 {
		t.Errorf("rhythm.bpm separates the sets by %.1f, so it is a usable signal after all", gap)
	}
}

func TestMeanAudioFeaturesUnmeasuredPace(t *testing.T) {
	// Two tracks classified by AcousticBrainz, one of which the low-level fetch
	// never answered for. Averaged over both it would read 2, which
	// normalizePace floors to no weight.
	tracks := []domain.Track{
		{Features: domain.AudioFeatures{Energy: 0.5, OnsetRate: 4, Present: true}},
		{Features: domain.AudioFeatures{Energy: 0.5, Present: true}},
	}

	got, analyzed := meanAudioFeatures(tracks)
	if analyzed != 2 {
		t.Errorf("analyzed = %d, want both tracks counted", analyzed)
	}
	if got.OnsetRate != 4 {
		t.Errorf("OnsetRate = %v, want the one measured 4", got.OnsetRate)
	}
	if got.Energy != 0.5 {
		t.Errorf("Energy = %v, want 0.5: only the pace averages over its own count", got.Energy)
	}
}

// drivingWeight analyzes a playlist whose tracks are identical apart from their
// rhythm, and returns the driving dimension's share of the palette.
func drivingWeight(t *testing.T, tracks []sample) float64 {
	t.Helper()

	playlist := make([]domain.Track, len(tracks))
	for i, s := range tracks {
		playlist[i] = domain.Track{
			Title: s.name,
			Features: domain.AudioFeatures{
				Acousticness: 0.5,
				Danceability: 0.5,
				Energy:       0.5,
				Valence:      0.5,
				OnsetRate:    s.onsetRate,
				Present:      true,
			},
		}
	}

	result, err := NewEngine().Analyze(context.Background(), "p", playlist, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return weightsByDimension(result.Palette)["driving"]
}
