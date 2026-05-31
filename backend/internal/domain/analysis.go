package domain

// Sentiment is the NLP result for a track's lyrics.
//
// Polarity is in [-1,1] (negative..positive) and Subjectivity is in [0,1]
// (objective..subjective). HasLyrics reports whether lyrics were found and
// analyzed at all.
type Sentiment struct {
	Polarity     float64 `bson:"polarity"`
	Subjectivity float64 `bson:"subjectivity"`
	HasLyrics    bool    `bson:"has_lyrics"`
}

// PlaylistAnalysis is the aggregated result of analyzing every track in a
// playlist. It combines normalized audio features, lyric sentiment, and the
// derived color palette that drives cover generation.
type PlaylistAnalysis struct {
	PlaylistID    string        `bson:"playlist_id"`
	TrackCount    int           `bson:"track_count"`
	AnalyzedCount int           `bson:"analyzed_count"`
	MeanFeatures  AudioFeatures `bson:"mean_features"`
	MeanSentiment Sentiment     `bson:"mean_sentiment"`
	Palette       []ColorWeight `bson:"palette"`
}

// ColorWeight maps an emotional/sonic dimension to a weighted color in the
// generated cover's palette. Weight is normalized so that the sum of all
// weights in a palette is 1.
type ColorWeight struct {
	Dimension string  `bson:"dimension"`
	HexColor  string  `bson:"hex_color"`
	Weight    float64 `bson:"weight"`
}
