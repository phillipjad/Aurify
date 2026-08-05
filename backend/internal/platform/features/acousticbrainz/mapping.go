package acousticbrainz

import "github.com/phillipjad/aurify/backend/internal/domain"

// highLevel is one AcousticBrainz classifier result: which class won, and how
// confident it was.
type highLevel struct {
	Value       string  `json:"value"`
	Probability float64 `json:"probability"`
}

// document is the shape of one recording's high-level data.
type document struct {
	HighLevel map[string]highLevel `json:"highlevel"`
}

// score converts one classifier into a [0,1] value.
//
// The classifiers are binary with a probability, so "not_danceable at p=0.75"
// and "danceable at p=0.25" describe the same track. Reading only the label
// would throw away three quarters of the signal, and reading only the
// probability would invert half the tracks.
func score(hl map[string]highLevel, key, positive string) (float64, bool) {
	d, ok := hl[key]
	if !ok {
		return 0, false
	}
	if d.Value == positive {
		return clamp01(d.Probability), true
	}
	return clamp01(1 - d.Probability), true
}

// mean averages the values that were actually present, so a missing classifier
// pulls nothing toward zero.
func mean(values ...float64) (float64, bool) {
	var sum float64
	var n int
	for _, v := range values {
		if v >= 0 {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// optional returns the value or -1, which mean() reads as absent.
func optional(v float64, ok bool) float64 {
	if !ok {
		return -1
	}
	return v
}

// toFeatures maps AcousticBrainz's classifiers onto Aurify's feature model.
//
// Only the descriptors that survived inspection are used. The genre and voice
// classifiers are deliberately ignored: on a vocal country ballad,
// voice_instrumental answered "instrumental" at p=0.72 and gender answered
// "female" at p=0.94, and genre_electronic called both that track and
// Radiohead's "Creep" ambient. The mood classifiers, danceability and timbre
// held up on the same samples, so those are what the palette is built from.
//
// Instrumentalness, liveness and speechiness are left at zero because nothing
// here measures them honestly. That keeps the introspective and intimate
// dimensions flat, which is a known and deliberate gap rather than an oversight.
func toFeatures(doc document) domain.AudioFeatures {
	hl := doc.HighLevel
	if len(hl) == 0 {
		return domain.AudioFeatures{Present: false}
	}

	danceability, hasDance := score(hl, "danceability", "danceable")
	acoustic, hasAcoustic := score(hl, "mood_acoustic", "acoustic")

	// Energy has no single classifier. Aggression and party-ness raise it and
	// relaxation lowers it, so it is the mean of what is available.
	energy, hasEnergy := mean(
		optional(score(hl, "mood_aggressive", "aggressive")),
		optional(score(hl, "mood_party", "party")),
		optional(invert(score(hl, "mood_relaxed", "relaxed"))),
	)

	// Valence likewise: happy raises it, sad lowers it, and a bright timbre
	// leans positive.
	valence, hasValence := mean(
		optional(score(hl, "mood_happy", "happy")),
		optional(invert(score(hl, "mood_sad", "sad"))),
		optional(score(hl, "timbre", "bright")),
	)

	// Present reports whether anything at all was measured. Without it a
	// recording that exists but classified nothing would average in as silence.
	present := hasDance || hasAcoustic || hasEnergy || hasValence
	return domain.AudioFeatures{
		Danceability: danceability,
		Acousticness: acoustic,
		Energy:       energy,
		Valence:      valence,
		Present:      present,
	}
}

func invert(v float64, ok bool) (float64, bool) {
	if !ok {
		return 0, false
	}
	return 1 - v, true
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
