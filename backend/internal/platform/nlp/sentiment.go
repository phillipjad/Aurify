// Package nlp performs lyric sentiment analysis.
//
// SCAFFOLD: this ships a naive lexicon-based analyzer so the pipeline produces
// real (if crude) numbers end-to-end. Replace Analyze with a proper NLP model
// (or a sidecar call) when ready.
package nlp

import (
	"context"
	"strings"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Analyzer is a lexicon-based sentiment analyzer.
type Analyzer struct{}

var _ ports.SentimentAnalyzer = (*Analyzer)(nil)

// NewAnalyzer constructs an Analyzer.
func NewAnalyzer() *Analyzer { return &Analyzer{} }

var positiveLexicon = map[string]struct{}{
	"love": {}, "happy": {}, "joy": {}, "bright": {}, "dream": {}, "dance": {},
	"smile": {}, "shine": {}, "good": {}, "alive": {}, "free": {}, "light": {},
}

var negativeLexicon = map[string]struct{}{
	"sad": {}, "cry": {}, "pain": {}, "lonely": {}, "dark": {}, "tears": {},
	"broken": {}, "hate": {}, "lost": {}, "fear": {}, "cold": {}, "empty": {},
}

// Analyze returns a Sentiment for the given lyrics. Empty lyrics yield a
// neutral result with HasLyrics=false.
func (a *Analyzer) Analyze(ctx context.Context, lyrics string) (domain.Sentiment, error) {
	_ = ctx
	if strings.TrimSpace(lyrics) == "" {
		return domain.Sentiment{HasLyrics: false}, nil
	}

	var pos, neg, total int
	for _, raw := range strings.Fields(strings.ToLower(lyrics)) {
		word := strings.Trim(raw, ".,!?;:\"'()[]")
		if word == "" {
			continue
		}
		total++
		if _, ok := positiveLexicon[word]; ok {
			pos++
		}
		if _, ok := negativeLexicon[word]; ok {
			neg++
		}
	}

	sentiment := domain.Sentiment{HasLyrics: true}
	if pos+neg > 0 {
		sentiment.Polarity = float64(pos-neg) / float64(pos+neg)
	}
	if total > 0 {
		sentiment.Subjectivity = float64(pos+neg) / float64(total)
	}
	return sentiment, nil
}
