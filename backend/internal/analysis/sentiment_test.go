package analysis

import (
	"context"
	"math"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// lyricSample is one real track, scored by running both analyzers over the
// lyrics already sitting in the dev database's lyrics_cache. Unlike the
// constructed genre sets in tonal_test.go these are the actual texts the
// pipeline reads, so the table costs no network and re-deriving it needs only
// the cache.
//
// Provenance, because it bounds what these numbers can prove: the two sets are
// songs a listener would place on either side, chosen from one person's library,
// which is heavy on hip-hop, EDM and emo. Songs whose *title* contains one of the
// 24 scaffold words are excluded, since selecting by title otherwise leaks the
// rejected signal's own vocabulary into the test and reverses the result.
type lyricSample struct {
	artist, title string
	// polarity is the weighted lexicon's balance of matched valence, in [-1,1].
	polarity float64
	// oldPolarity is the same track under the 24-word scaffold, kept so the
	// rejected signal can be shown to be one.
	oldPolarity float64
}

// bleakLyrics and brightLyrics are two playlists a listener would not argue
// about: songs about death, grief and despair against songs about celebration
// and summer.
var (
	bleakLyrics = []lyricSample{
		{"billie eilish", "when the party's over", 0.786, 1.000},
		{"billie eilish", "listen before i go", 0.060, 0.500},
		{"my chemical romance", "helena", -0.277, -1.000},
		{"linkin park", "somewhere i belong", -0.301, -1.000},
		{"flyleaf", "i'm so sick", -0.798, -1.000},
		{"deftones", "change (in the house of flies)", 1.000, 1.000},
		{"d4vd", "romantic homicide", -0.463, -1.000},
		{"50 cent", "many men (wish death)", -0.287, -0.600},
		{"avenged sevenfold", "so far away", 0.170, 0.167},
		{"dave", "survivor's guilt", 0.201, 0.571},
		{"$uicideboy$", "misery in waking hours", -0.485, 0.333},
		{"ynw melly", "murder on my mind", -0.788, 0.333},
		{"post malone", "hollywood's bleeding", -0.271, 1.000},
		{"kevin sherwood", "lullaby of a deadman", -0.036, 1.000},
	}
	brightLyrics = []lyricSample{
		{"frankie valli", "can't take my eyes off you", 0.858, 0.765},
		{"dj jazzy jeff & the fresh prince", "summertime", 1.000, 1.000},
		{"kanye west", "celebration", -0.164, 0.000},
		{"dj khaled", "celebrate", 0.409, 0.115},
		{"zac brown band", "chicken fried", 0.941, 0.333},
		{"oh wonder", "don't you worry", 1.000, 1.000},
		{"fetty wap", "trap queen", 0.050, 1.000},
		{"drake", "god's plan", -0.022, 1.000},
		{"kero kero bonito", "flamingo", 0.636, 0.000},
		{"crush 40", `open your heart - main theme of "sonic adventure" -`, -0.855, -0.500},
		{"dram", "broccoli (feat. lil yachty)", 0.007, 1.000},
		{"nelly", "ride wit me", 0.423, 1.000},
	}
)

// TestLyricSentimentSeparatesPlaylists is #95's "done when": two playlists a
// listener would call bleak and joyful have to land on opposite sides of the
// palette's bright/dark axis.
//
// Every audio feature is held identical between the two and no key is measured,
// so the entire difference is the lyrics' doing.
func TestLyricSentimentSeparatesPlaylists(t *testing.T) {
	bleak := lyricBrightShare(t, bleakLyrics, sampleMeasured)
	bright := lyricBrightShare(t, brightLyrics, sampleMeasured)

	if bleak >= 0 {
		t.Errorf("the bleak playlist reads bright at %.3f", bleak)
	}
	if bright <= 0 {
		t.Errorf("the joyful playlist reads dark at %.3f", bright)
	}
	// Measured: -0.103 against +0.118. No key is measured here, so the lyrics
	// carry the whole of what polarityWeight allows and this is the most the axis
	// can move on words alone.
	if gap := bright - bleak; gap < 0.15 {
		t.Errorf("the playlists spread over %.3f of the palette, want at least 0.15: bleak %.3f, joyful %.3f",
			gap, bleak, bright)
	}
}

// TestTwentyFourWordLexiconDoesNotSeparatePlaylists is the same two playlists
// under the signal this replaced, and it is why the analyzer is no longer a
// membership test over 24 words.
//
// It fails the moment someone points the analyzer back at one. The scaffold does
// not merely separate them less; it puts the bleak playlist on the wrong side,
// because a ratio over three matched words is decided by whichever of "love" or
// "lost" happened to be sung.
func TestTwentyFourWordLexiconDoesNotSeparatePlaylists(t *testing.T) {
	bleak := lyricBrightShare(t, bleakLyrics, sampleRejected)
	bright := lyricBrightShare(t, brightLyrics, sampleRejected)

	if bleak <= 0 {
		t.Errorf("the 24-word lexicon reads the bleak playlist dark at %.3f, so it is a usable signal after all", bleak)
	}
	measured := lyricBrightShare(t, brightLyrics, sampleMeasured) - lyricBrightShare(t, bleakLyrics, sampleMeasured)
	if gap := bright - bleak; gap >= measured {
		t.Errorf("the 24-word lexicon separates the playlists by %.3f against the weighted lexicon's %.3f",
			gap, measured)
	}
}

// A playlist whose tracks all had lyrics but none of them clearly either way is
// not a playlist half bright and half bleak. It is one the words say nothing
// about, and the axis is left to the key.
func TestMeanSentimentAllNeutral(t *testing.T) {
	got := meanSentiment([]domain.Sentiment{
		{Polarity: 0.1, Subjectivity: 0.2, HasLyrics: true},
		{Polarity: -0.2, Subjectivity: 0.1, HasLyrics: true},
		{HasLyrics: false},
	})

	if !got.HasLyrics {
		t.Error("HasLyrics = false, want true: two tracks had lyrics")
	}
	if got.Polarity != 0 {
		t.Errorf("Polarity = %v, want 0: neither track clears the threshold", got.Polarity)
	}
	// Averaged over the tracks with lyrics, not over all three.
	if math.Abs(got.Subjectivity-0.15) > 1e-9 {
		t.Errorf("Subjectivity = %v, want 0.15", got.Subjectivity)
	}
}

// sampleMeasured and sampleRejected pick which of a row's two measurements the
// playlist is built from, so both signals run through the identical aggregation
// and palette.
func sampleMeasured(s lyricSample) float64 { return s.polarity }
func sampleRejected(s lyricSample) float64 { return s.oldPolarity }

func lyricSentiments(samples []lyricSample, of func(lyricSample) float64) []domain.Sentiment {
	out := make([]domain.Sentiment, len(samples))
	for i, s := range samples {
		out[i] = domain.Sentiment{Polarity: of(s), HasLyrics: true}
	}
	return out
}

// lyricBrightShare is how much of the palette the bright colour takes less how
// much the dark one takes, for a playlist whose tracks are identical apart from
// their lyrics. One of the two is always zero, so the result is a signed share.
//
// No key is measured, which is the case that matters: AcousticBrainz stopped
// collecting in 2022, so a playlist of recent music reaches the palette with
// Tonality at zero and the lyrics carrying the axis alone.
func lyricBrightShare(t *testing.T, samples []lyricSample, of func(lyricSample) float64) float64 {
	t.Helper()

	tracks := make([]domain.Track, len(samples))
	for i, s := range samples {
		tracks[i] = domain.Track{
			Title:   s.title,
			Artists: []string{s.artist},
			Features: domain.AudioFeatures{
				Acousticness: 0.5,
				Danceability: 0.5,
				Energy:       0.5,
				Present:      true,
			},
		}
	}

	result, err := NewEngine().Analyze(context.Background(), "p", tracks, lyricSentiments(samples, of))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	weights := weightsByDimension(result.Palette)
	return weights["euphoric"] - weights["melancholic"]
}
