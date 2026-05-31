package domain

// Playlist is a user's playlist on a DSP, normalized across providers.
type Playlist struct {
	ID          string      `bson:"id"`
	Platform    DSPPlatform `bson:"platform"`
	Name        string      `bson:"name"`
	Description string      `bson:"description"`
	TrackCount  int         `bson:"track_count"`
	ImageURL    string      `bson:"image_url"`
}

// Track is a single song, normalized into Aurify's internal model regardless
// of the source DSP.
type Track struct {
	ID         string        `bson:"id"`
	Platform   DSPPlatform   `bson:"platform"`
	Title      string        `bson:"title"`
	Artists    []string      `bson:"artists"`
	Album      string        `bson:"album"`
	ISRC       string        `bson:"isrc"`
	DurationMS int           `bson:"duration_ms"`
	Features   AudioFeatures `bson:"features"`
}

// AudioFeatures is the normalized acoustic feature model. Values are in the
// range [0,1] and are mapped from each platform's own feature endpoint (for
// example Spotify's /v1/audio-features). TempoBPM is in beats-per-minute.
//
// Present reports whether the source DSP actually supplied features for the
// track; the analysis pipeline ignores tracks where Present is false.
type AudioFeatures struct {
	Acousticness     float64 `bson:"acousticness"`
	Danceability     float64 `bson:"danceability"`
	Energy           float64 `bson:"energy"`
	Instrumentalness float64 `bson:"instrumentalness"`
	Liveness         float64 `bson:"liveness"`
	Loudness         float64 `bson:"loudness"`
	Speechiness      float64 `bson:"speechiness"`
	Valence          float64 `bson:"valence"`
	TempoBPM         float64 `bson:"tempo_bpm"`
	Present          bool    `bson:"present"`
}
