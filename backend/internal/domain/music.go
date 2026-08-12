package domain

// Playlist is a user's playlist on a DSP, normalized across providers.
type Playlist struct {
	ID          string
	Platform    DSPPlatform
	Name        string
	Description string
	TrackCount  int
	ImageURL    string
}

// Track is a single song, normalized into Aurify's internal model regardless
// of the source DSP.
type Track struct {
	ID         string
	Platform   DSPPlatform
	Title      string
	Artists    []string
	Album      string
	ISRC       string
	DurationMS int
	Features   AudioFeatures
}

// AudioFeatures is the normalized acoustic feature model. Values are in the
// range [0,1] and are mapped from each platform's own feature endpoint (for
// example Spotify's /v1/audio-features). OnsetRate is the exception: it is
// onsets per second, unbounded above, and analysis.normalizePace is what turns
// it into a palette weight.
//
// Present reports whether the source DSP actually supplied features for the
// track; the analysis pipeline ignores tracks where Present is false.
//
// The json tags fix this type's encoding inside the JSONB analysis payload
// (it is PlaylistAnalysis.MeanFeatures); keep them stable.
type AudioFeatures struct {
	Acousticness     float64 `json:"acousticness"`
	Danceability     float64 `json:"danceability"`
	Energy           float64 `json:"energy"`
	Instrumentalness float64 `json:"instrumentalness"`
	Liveness         float64 `json:"liveness"`
	Loudness         float64 `json:"loudness"`
	Speechiness      float64 `json:"speechiness"`
	Valence          float64 `json:"valence"`
	OnsetRate        float64 `json:"onset_rate"`
	Present          bool    `json:"present"`
}
