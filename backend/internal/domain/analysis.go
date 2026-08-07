package domain

// Sentiment is the NLP result for a track's lyrics.
//
// Polarity is in [-1,1] (negative..positive) and Subjectivity is in [0,1]
// (objective..subjective). HasLyrics reports whether lyrics were found and
// analyzed at all.
//
// The json tags fix the encoding of the JSONB analysis payload that PlaylistAnalysis
// is serialized into (see docs/adr/0010-postgresql-storage.md); keep them stable.
type Sentiment struct {
	Polarity     float64 `json:"polarity"`
	Subjectivity float64 `json:"subjectivity"`
	HasLyrics    bool    `json:"has_lyrics"`
}

// PlaylistAnalysis is the aggregated result of analyzing every track in a
// playlist. It combines normalized audio features, lyric sentiment, and the
// derived color palette that drives cover generation. It is persisted as the
// JSONB `analysis` column on covers.
type PlaylistAnalysis struct {
	PlaylistID string `json:"playlist_id"`
	TrackCount int    `json:"track_count"`
	// AnalyzedCount is how many tracks carried real, measured features. With
	// TrackCount it is also what any stored analysis needs to reconstruct how
	// much of its palette was measured rather than guessed
	// (see analysis.CoverageWeight).
	AnalyzedCount int           `json:"analyzed_count"`
	MeanFeatures  AudioFeatures `json:"mean_features"`
	MeanSentiment Sentiment     `json:"mean_sentiment"`
	Palette       []ColorWeight `json:"palette"`
	// FeaturesEstimated records that a guess from the tracks and their lyrics
	// contributed to MeanFeatures, which happens whenever too little of the
	// playlist could be matched to a features source to trust the mean
	// (docs/adr/0020-coverage-weighted-features.md). Kept so an estimate is
	// never mistaken for a measurement later; omitempty so existing stored
	// analyses are unaffected.
	FeaturesEstimated bool `json:"features_estimated,omitempty"`
}

// ColorWeight maps an emotional/sonic dimension to a weighted color in the
// generated cover's palette. Weight is normalized so that the sum of all
// weights in a palette is 1.
type ColorWeight struct {
	Dimension string  `json:"dimension"`
	HexColor  string  `json:"hex_color"`
	Weight    float64 `json:"weight"`
}
