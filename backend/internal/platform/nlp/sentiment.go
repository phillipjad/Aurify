// Package nlp performs lyric sentiment analysis.
//
// The analyzer is a weighted lexicon: deterministic, offline, and local CPU work
// measured in microseconds, so the per-track pass in generatecover can stay
// sequential. Nothing here reaches the network.
package nlp

import (
	"context"
	"strings"

	"github.com/jonreiter/govader"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Analyzer scores lyrics with VADER's valence lexicon.
//
// It replaces a 24-word membership test that matched 1.3% of a song, so a track
// reading -1 was a ratio over three words
// (docs/adr/0026-lyric-sentiment-from-a-weighted-lexicon.md).
type Analyzer struct {
	vader *govader.SentimentIntensityAnalyzer
}

var _ ports.SentimentAnalyzer = (*Analyzer)(nil)

// NewAnalyzer constructs an Analyzer.
//
// This parses a 7520-entry lexicon, so it is built once at wiring time and
// reused for every track of every playlist.
func NewAnalyzer() *Analyzer {
	return &Analyzer{vader: govader.NewSentimentIntensityAnalyzer()}
}

// Analyze returns a Sentiment for the given lyrics. Empty lyrics yield a
// neutral result with HasLyrics=false.
//
// Polarity is the balance of the valence the lexicon matched, in [-1,1]: the
// same ratio the 24-word scaffold computed, but over words that carry a score
// and with negation and intensifiers applied, so "not happy" no longer reads +1.
// Subjectivity is what that ratio rests on, the share of the song's words the
// lexicon scores at all.
func (a *Analyzer) Analyze(ctx context.Context, lyrics string) (domain.Sentiment, error) {
	_ = ctx
	if strings.TrimSpace(lyrics) == "" {
		return domain.Sentiment{HasLyrics: false}, nil
	}

	// Positive and Negative are the matched valence sums over a shared total, so
	// dividing one by the other cancels that total. Compound is not used: it
	// saturates to ±1 on anything song-length.
	s := a.vader.PolarityScores(lyrics)

	sentiment := domain.Sentiment{HasLyrics: true}
	if s.Positive+s.Negative > 0 {
		sentiment.Polarity = (s.Positive - s.Negative) / (s.Positive + s.Negative)
	}
	sentiment.Subjectivity = a.coverage(lyrics)
	return sentiment, nil
}

// coverage is the share of the song's words the lexicon scores.
//
// Counted here rather than taken from PolarityScores: VADER's Neutral is a token
// count over a total that also holds two valence sums, so 1-Neutral is not a
// word share and overstates one by roughly the mean valence.
func (a *Analyzer) coverage(lyrics string) float64 {
	var matched, total int
	for _, raw := range strings.Fields(strings.ToLower(lyrics)) {
		word := strings.Trim(raw, ".,!?;:\"'()[]")
		if word == "" {
			continue
		}
		total++
		if _, ok := a.vader.Lexicon[word]; ok {
			matched++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(matched) / float64(total)
}
