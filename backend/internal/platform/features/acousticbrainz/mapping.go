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

// lowLevelDocument is the shape of one recording's low-level data, of which
// Aurify reads two numbers. The rest of the document is the raw Essentia dump
// and is an order of magnitude larger than everything else in this package.
type lowLevelDocument struct {
	Rhythm struct {
		// OnsetRate is onsets per second: how many note or percussive events
		// the track fires. See docs/adr/0023-pace-from-onset-rate.md for why
		// this and not rhythm.bpm.
		OnsetRate float64 `json:"onset_rate"`
	} `json:"rhythm"`
	Tonal struct {
		// KeyScale is "major" or "minor". A measurement of the audio rather
		// than a trained classifier, and near-binary per track, so its mean
		// over a playlist is a proportion and survives the averaging that
		// flattens valence (docs/adr/0025-palette-from-measured-signals.md).
		KeyScale string `json:"key_scale"`
	} `json:"tonal"`
}

// tonality maps a key scale onto the signed axis the palette reads. An
// unrecognized or absent scale is 0, which is "nothing to say" rather than
// "minor".
func tonality(keyScale string) float64 {
	switch keyScale {
	case "major":
		return 1
	case "minor":
		return -1
	default:
		return 0
	}
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
// Only the descriptors that survived inspection are used. The genre and gender
// classifiers are still ignored: gender answered "female" at p=0.94 on a male
// country vocal, and genre_electronic called both that track and Radiohead's
// "Creep" ambient.
//
// voice_instrumental is read again, which ADR 0018 rejected on the evidence of
// that one country ballad it called instrumental at p=0.72. Over 24 recordings
// whose vocal status is not in dispute it was right about 71% of them and
// separated the two groups by 0.442, which is a floor rather than an accuracy:
// the labels are per song and the data is per recording, so an instrumental
// cover in the candidate list is scored as the song being wrong. It is
// near-binary per track, so its mean is a proportion of the playlist that is
// instrumental. Argued in docs/adr/0025-palette-from-measured-signals.md.
//
// Liveness and speechiness are left at zero because nothing here measures them
// honestly, and no dimension reads them any more.
func toFeatures(doc document) domain.AudioFeatures {
	hl := doc.HighLevel
	if len(hl) == 0 {
		return domain.AudioFeatures{Present: false}
	}

	danceability, hasDance := score(hl, "danceability", "danceable")
	acoustic, hasAcoustic := score(hl, "mood_acoustic", "acoustic")
	instrumental, hasInstrumental := score(hl, "voice_instrumental", "instrumental")

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
	present := hasDance || hasAcoustic || hasEnergy || hasValence || hasInstrumental
	return domain.AudioFeatures{
		Danceability:     danceability,
		Acousticness:     acoustic,
		Energy:           energy,
		Valence:          valence,
		Instrumentalness: instrumental,
		Present:          present,
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
