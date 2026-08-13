package analysis

import (
	"maps"
	"slices"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// genreSet is one constructed set of tracks measured against the live
// AcousticBrainz API while choosing what the bright/dark axis should read: what
// share of the set came back in a major key, and what valence the mapping in
// acousticbrainz.toFeatures computed for the same recordings.
//
// Provenance, because it bounds what these numbers can prove: these are sets
// assembled by genre rather than real playlists, tracks are only reachable
// inside the generation pipeline, and the candidate picker takes the first
// recording carrying data, which can be a live or instrumental version of the
// song. Set-level aggregates are what was recorded, not per-recording rows, so
// this table is coarser than the one in pace_test.go. It is the evidence for
// docs/adr/0025-palette-from-measured-signals.md and it wants re-measuring
// against pinned recording ids.
type genreSet struct {
	name string
	n    int
	// major is the proportion of the set AcousticBrainz measured in a major
	// key, which is what the palette now reads.
	major float64
	// valence is the mean of the mood classifiers, which is what it read
	// before, kept so the rejected signal can be shown to be one.
	valence float64
}

var genreSets = []genreSet{
	{"upbeat pop", 5, 0.800, 0.775},
	{"folk and acoustic", 6, 0.833, 0.502},
	{"electronic and house", 9, 0.667, 0.444},
	{"jazz standards", 9, 0.444, 0.470},
	{"metal and dark", 7, 0.143, 0.439},
}

// TestKeyScaleSeparatesPlaylists is #90's "done when": whatever replaces valence
// has to separate playlists a listener would call bright and dark.
//
// The measure is the signed share of the palette the bright and dark colours
// take, since one of the pair is always zero. Everything but the axis is held
// identical between the sets, so the whole difference is the key scale's doing.
func TestKeyScaleSeparatesPlaylists(t *testing.T) {
	byName := map[string]float64{}
	for _, set := range genreSets {
		byName[set.name] = brightShare(proportionToTonality(set.major))
	}
	shares := slices.Collect(maps.Values(byName))

	// Measured: metal -0.322 against folk 0.307.
	if spread := slices.Max(shares) - slices.Min(shares); spread < 0.5 {
		t.Errorf("the sets spread over %.3f of the palette, want at least 0.5: %v", spread, byName)
	}
	if byName["metal and dark"] >= 0 {
		t.Errorf("the metal set reads bright at %.3f", byName["metal and dark"])
	}
	if byName["jazz standards"] >= 0 {
		t.Errorf("the jazz set reads bright at %.3f", byName["jazz standards"])
	}
	for _, bright := range []string{"upbeat pop", "folk and acoustic", "electronic and house"} {
		if byName[bright] <= 0 {
			t.Errorf("the %s set reads dark at %.3f", bright, byName[bright])
		}
	}
}

// TestValenceDoesNotSeparatePlaylists is the same sets under the signal this
// replaced, and it is why the bright and dark colours no longer read a valence.
//
// It fails the moment someone points brightness back at it. Valence pins four of
// the five sets into 0.439 to 0.502, so once the pop set is set aside the whole
// remaining library is one shade: those four separate by under a tenth of what
// the key scale separates them by.
func TestValenceDoesNotSeparatePlaylists(t *testing.T) {
	var byKey, byValence []float64
	for _, set := range genreSets {
		if set.name == "upbeat pop" {
			continue
		}
		byKey = append(byKey, brightShare(proportionToTonality(set.major)))
		// Valence is [0,1] where the axis is [-1,1], so it is mapped the way
		// BlendFeatures maps the estimator's.
		byValence = append(byValence, brightShare(2*set.valence-1))
	}

	keySpread := slices.Max(byKey) - slices.Min(byKey)
	valenceSpread := slices.Max(byValence) - slices.Min(byValence)
	if valenceSpread > 0.10 {
		t.Errorf("valence separates the four sets by %.3f, so it is a usable signal after all", valenceSpread)
	}
	if keySpread < 5*valenceSpread {
		t.Errorf("the key scale separates them by %.3f against valence's %.3f, want at least five times",
			keySpread, valenceSpread)
	}
}

// A track nobody submitted a key for is not a track halfway between major and
// minor. The key comes from the low-level endpoint, which can miss on its own,
// so it averages over its own count exactly as the pace does.
func TestMeanAudioFeaturesUnmeasuredKey(t *testing.T) {
	tracks := []domain.Track{
		{Features: domain.AudioFeatures{Energy: 0.5, Tonality: 1, Present: true}},
		{Features: domain.AudioFeatures{Energy: 0.5, Tonality: 1, Present: true}},
		{Features: domain.AudioFeatures{Energy: 0.5, Present: true}},
	}

	got, analyzed := meanAudioFeatures(tracks)
	if analyzed != 3 {
		t.Errorf("analyzed = %d, want all three tracks counted", analyzed)
	}
	// Over all three it would read 0.667, which is a third of the way toward
	// minor on the strength of a track nobody measured.
	if got.Tonality != 1 {
		t.Errorf("Tonality = %v, want 1: both measured tracks are in a major key", got.Tonality)
	}
	if got.Energy != 0.5 {
		t.Errorf("Energy = %v, want 0.5: only the key averages over its own count", got.Energy)
	}
}

func proportionToTonality(major float64) float64 { return 2*major - 1 }

// brightShare is how much of the palette the bright colour takes less how much
// the dark one takes, for a playlist whose tracks are identical apart from
// their key. One of the two is always zero, so the result is a signed share in
// [-1,1].
//
// The mean is skipped and the palette built from the aggregate directly, since
// ten identical tracks average to themselves; that the mean carries a key at
// all is TestMeanAudioFeaturesUnmeasuredKey's job.
func brightShare(tonality float64) float64 {
	weights := weightsByDimension(BuildPalette(
		domain.AudioFeatures{
			Acousticness: 0.5,
			Danceability: 0.5,
			Energy:       0.5,
			Tonality:     tonality,
			Present:      true,
		},
		domain.Sentiment{},
	))
	return weights["euphoric"] - weights["melancholic"]
}
