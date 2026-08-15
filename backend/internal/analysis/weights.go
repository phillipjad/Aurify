package analysis

import (
	"sort"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// dimension associates a sonic/emotional dimension with a base color and a
// function that derives its raw weight in [0,1] from the aggregate signals.
//
// This is the heart of the "weighing" described in the product vision: a high
// mean energy contributes weight to the energetic color, a playlist mostly in
// minor keys pushes weight toward the melancholic one, and so on. Add or retune
// dimensions here.
//
// Every dimension reads something a source measures. Two used to read features
// nothing reachable does: intimate was dropped for it, and introspective was
// given a real source instead (docs/adr/0025-palette-from-measured-signals.md).
// A dimension scored from an always-zero field is not a quiet gap, it is a
// colour that can never appear and a share of the palette permanently spent.
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
		return max(0, brightness(f, s))
	}},
	{"organic", "#7FB069", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		return f.Acousticness
	}},
	{"introspective", "#4F86C6", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		// Measured since ADR 0025, from voice_instrumental. Near-binary per
		// track, so the mean is the proportion of the playlist with no singing.
		return f.Instrumentalness
	}},
	{"melancholic", "#5C4D7D", func(f domain.AudioFeatures, s domain.Sentiment) float64 {
		return max(0, -brightness(f, s))
	}},
	{"driving", "#31C3B3", func(f domain.AudioFeatures, _ domain.Sentiment) float64 {
		return normalizePace(f.OnsetRate)
	}},
}

// polarityWeight is how much of the bright/dark axis the lyrics carry.
//
// Raised from 0.25 once the analyzer stopped being a 24-word membership test.
// Over the 1157 cached lyric sets it now matches 40.2 words a track rather than
// 4.3, and meanSentiment reports a proportion rather than a mean, so what
// reaches this line is the share of the playlist whose words are clearly one way
// (docs/adr/0026-lyric-sentiment-from-a-weighted-lexicon.md).
//
// Still a minority share. The key scale is a measurement of the audio; this is a
// bag of words that reads Billie Eilish's "when the party's over" as bright
// because one chorus line repeats eight times. On the 26 tracks in
// sentiment_test.go it commits to a side on 15 and is right about 12 of them.
const polarityWeight = 0.4

// brightness is the bright/dark axis, in [-1,1].
//
// The tonal term carries it. Major/minor is a measurement rather than a trained
// classifier and is near-binary per track, so its mean over a playlist is a
// proportion and survives averaging: five constructed genre sets spread over a
// range of 0.690, where valence pinned four of the five into 0.44 to 0.50.
// Argued in docs/adr/0025-palette-from-measured-signals.md.
//
// The lyric term keeps its fixed share instead of taking the whole axis when the
// tonal one is unmeasured. A playlist that matched nothing acoustically should
// not have its cover decided by a word count.
func brightness(f domain.AudioFeatures, s domain.Sentiment) float64 {
	if !s.HasLyrics {
		return f.Tonality
	}
	return f.Tonality*(1-polarityWeight) + s.Polarity*polarityWeight
}

// paceFloor and paceCeiling bound the onset rate the palette can tell apart, in
// onsets per second.
//
// Measured over 19 AcousticBrainz recordings chosen across the tempo range: the
// slowest thing sampled was a beatless ambient piece at 0.66, the busiest a
// Burial track at 5.28, and the middle half of the sample fell between 2.41 and
// 3.47. A curve has to be steep across that middle to separate anything, so it
// saturates well inside the observed extremes rather than at them. Argued in
// docs/adr/0023-pace-from-onset-rate.md.
//
// ponytail: 19 recordings picked to span the tempo range, not sampled from a
// library. Retune from the onset rates in track_features once enough real
// playlists have been analyzed to have a distribution rather than a spread.
const (
	paceFloor   = 1.5
	paceCeiling = 4.5
)

// normalizePace maps an onset rate onto [0,1].
//
// Zero is not a slow track, it is an unmeasured one: a track AcousticBrainz
// classified but never had its rhythm submitted for, or a playlist nothing
// matched. Both land below the floor and contribute no weight, which in a
// palette normalized to sum to 1 means the driving color simply does not show.
func normalizePace(onsetRate float64) float64 {
	return clamp01((onsetRate - paceFloor) / (paceCeiling - paceFloor))
}

// BuildPalette derives the weighted color palette from the aggregate features
// and sentiment. Weights are normalized so they sum to 1, and the result is
// sorted by descending weight (dominant colors first).
//
// A playlist with no features has no palette. Measured on the dev database, 26
// of 87 completed revisions had none, and every one of them rendered as roughly
// 76% melancholic and 24% euphoric, from constant terms that are now gone.
// Aurify knowing nothing about a playlist is not the same as the playlist being
// sad.
//
// Lyrics alone are not enough to build one from either. Sixteen of those 26 did
// have lyrics, and their polarity comes from a 24-word lexicon that matches
// about 1.4% of a song, so normalizing it against five zeroes would promote a
// couple of word hits to the whole cover.
func BuildPalette(f domain.AudioFeatures, s domain.Sentiment) []domain.ColorWeight {
	if !f.Present {
		return nil
	}

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
