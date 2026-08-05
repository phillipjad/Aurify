package acousticbrainz

import (
	"encoding/json"
	"math"
	"testing"
)

// Recorded from the live API for Thomas Rhett, "Die a Happy Man": a gentle
// acoustic ballad. Kept verbatim, including the classifiers this mapping
// deliberately ignores, so a future change that starts trusting them fails here.
const dieAHappyMan = `{"highlevel":{
  "danceability":       {"value":"not_danceable",  "probability":0.75},
  "gender":             {"value":"female",         "probability":0.94},
  "genre_electronic":   {"value":"ambient",        "probability":0.68},
  "mood_acoustic":      {"value":"acoustic",       "probability":0.55},
  "mood_aggressive":    {"value":"not_aggressive", "probability":0.99},
  "mood_electronic":    {"value":"not_electronic", "probability":0.92},
  "mood_happy":         {"value":"not_happy",      "probability":0.98},
  "mood_party":         {"value":"not_party",      "probability":0.89},
  "mood_relaxed":       {"value":"relaxed",        "probability":0.88},
  "mood_sad":           {"value":"sad",            "probability":0.74},
  "timbre":             {"value":"bright",         "probability":0.96},
  "voice_instrumental": {"value":"instrumental",   "probability":0.72}
}}`

// Blake Shelton, "God's Country": heavy, driving.
const godsCountry = `{"highlevel":{
  "danceability":    {"value":"danceable",   "probability":0.71},
  "mood_acoustic":   {"value":"not_acoustic","probability":0.97},
  "mood_aggressive": {"value":"aggressive",  "probability":0.52},
  "mood_happy":      {"value":"not_happy",   "probability":0.78},
  "mood_party":      {"value":"party",       "probability":0.57},
  "mood_relaxed":    {"value":"relaxed",     "probability":0.68},
  "mood_sad":        {"value":"not_sad",     "probability":0.95},
  "timbre":          {"value":"dark",        "probability":0.89}
}}`

func parse(t *testing.T, raw string) document {
	t.Helper()
	var doc document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("fixture does not parse: %v", err)
	}
	return doc
}

// The probability has to be read together with the label. "not_danceable at
// p=0.75" and "danceable at p=0.25" are the same track, so trusting the label
// alone throws away most of the signal and trusting the probability alone
// inverts half the catalogue.
func TestProbabilityIsSignedByTheLabel(t *testing.T) {
	ballad := toFeatures(parse(t, dieAHappyMan))
	heavy := toFeatures(parse(t, godsCountry))

	if got := ballad.Danceability; math.Abs(got-0.25) > 0.001 {
		t.Errorf("ballad danceability = %.3f, want 0.25 (not_danceable at p=0.75)", got)
	}
	if got := heavy.Danceability; math.Abs(got-0.71) > 0.001 {
		t.Errorf("heavy danceability = %.3f, want 0.71 (danceable at p=0.71)", got)
	}
}

// The point of the whole change: two real tracks must come out different enough
// to move the palette.
func TestRealTracksAreDiscriminated(t *testing.T) {
	ballad := toFeatures(parse(t, dieAHappyMan))
	heavy := toFeatures(parse(t, godsCountry))

	if !(ballad.Acousticness > heavy.Acousticness) {
		t.Errorf("the acoustic ballad (%.2f) should read more acoustic than the heavy track (%.2f)",
			ballad.Acousticness, heavy.Acousticness)
	}
	if !(heavy.Energy > ballad.Energy) {
		t.Errorf("the heavy track (%.2f) should read more energetic than the ballad (%.2f)",
			heavy.Energy, ballad.Energy)
	}
	if !(heavy.Danceability > ballad.Danceability) {
		t.Errorf("the heavy track (%.2f) should read more danceable than the ballad (%.2f)",
			heavy.Danceability, ballad.Danceability)
	}
}

// These classifiers were wrong on the very samples above: a vocal ballad called
// instrumental, and a male singer called female. Nothing may start reading them
// without this failing first.
func TestUnreliableClassifiersAreIgnored(t *testing.T) {
	f := toFeatures(parse(t, dieAHappyMan))

	if f.Instrumentalness != 0 {
		t.Errorf("instrumentalness = %.2f, want 0: voice_instrumental called this vocal track instrumental",
			f.Instrumentalness)
	}
	if f.Liveness != 0 || f.Speechiness != 0 {
		t.Errorf("liveness=%.2f speechiness=%.2f, want 0: nothing here measures them",
			f.Liveness, f.Speechiness)
	}
}

// A recording that exists but classified nothing must not average in as
// silence, which is exactly the bug this whole change is fixing.
func TestNothingMeasuredIsNotPresent(t *testing.T) {
	for _, raw := range []string{`{"highlevel":{}}`, `{}`, `{"highlevel":{"gender":{"value":"male","probability":0.9}}}`} {
		if f := toFeatures(parse(t, raw)); f.Present {
			t.Errorf("%s reported Present with nothing usable measured", raw)
		}
	}
}

// A partial document should still contribute what it has.
func TestPartialDocumentsStillCount(t *testing.T) {
	f := toFeatures(parse(t, `{"highlevel":{"mood_acoustic":{"value":"acoustic","probability":0.80}}}`))

	if !f.Present {
		t.Fatal("a document with one usable classifier should be Present")
	}
	if math.Abs(f.Acousticness-0.80) > 0.001 {
		t.Errorf("acousticness = %.3f, want 0.80", f.Acousticness)
	}
	if f.Energy != 0 || f.Valence != 0 {
		t.Errorf("absent classifiers should stay 0, got energy=%.2f valence=%.2f", f.Energy, f.Valence)
	}
}
