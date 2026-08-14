package nlp

import (
	"context"
	"testing"
)

// TestAnalyzeReadsNegation is the shortest statement of what the 24-word
// scaffold could not do: it scored "not happy" as +1, because membership in a
// word list has no room for the word before it.
func TestAnalyzeReadsNegation(t *testing.T) {
	a := NewAnalyzer()

	happy, err := a.Analyze(context.Background(), "i am happy")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	sad, err := a.Analyze(context.Background(), "i am not happy")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if happy.Polarity <= 0 {
		t.Errorf(`"i am happy" reads %.3f, want positive`, happy.Polarity)
	}
	if sad.Polarity >= 0 {
		t.Errorf(`"i am not happy" reads %.3f, want negative`, sad.Polarity)
	}
}

// The lexicon reaches words the 24-word scaffold never held, and stays silent on
// words nothing scores.
func TestAnalyze(t *testing.T) {
	a := NewAnalyzer()

	grief, err := a.Analyze(context.Background(), "tragedy grief funeral")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if grief.Polarity >= 0 || grief.Subjectivity == 0 {
		t.Errorf("bleak words read polarity %.3f, coverage %.3f, want negative with coverage",
			grief.Polarity, grief.Subjectivity)
	}

	none, err := a.Analyze(context.Background(), "the table the chair the window")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if none.Polarity != 0 || none.Subjectivity != 0 {
		t.Errorf("unscored words read polarity %.3f, coverage %.3f, want zero for both",
			none.Polarity, none.Subjectivity)
	}
}

// Nothing found is not a neutral song. The palette reads HasLyrics to decide
// whether the lyric term takes any of the bright/dark axis at all, so a track
// lrclib has never heard of has to be silence rather than a zero.
func TestAnalyzeWithoutLyrics(t *testing.T) {
	for _, lyrics := range []string{"", "   ", "\n\n"} {
		got, err := NewAnalyzer().Analyze(context.Background(), lyrics)
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if got.HasLyrics || got.Polarity != 0 || got.Subjectivity != 0 {
			t.Errorf("Analyze(%q) = %+v, want the zero value", lyrics, got)
		}
	}
}
