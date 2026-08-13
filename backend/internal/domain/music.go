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

// AudioFeatures is the normalized acoustic feature model. Most values are in
// the range [0,1] and are mapped from each platform's own feature endpoint (for
// example Spotify's /v1/audio-features). Two are not, and each says so on its
// own line below.
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
	// Valence is no longer read from the measured side: the palette's bright
	// and dark colors read Tonality instead, because a mean of mood
	// classifiers collapses toward the population mean over a whole playlist
	// (docs/adr/0025-palette-from-measured-signals.md). It is still measured,
	// stored and compared against, and on the estimated side it is the only
	// brightness the text model can give, which analysis.BlendFeatures maps
	// onto Tonality.
	Valence float64 `json:"valence"`
	// OnsetRate is onsets per second, unbounded above rather than in [0,1].
	// analysis.normalizePace is what turns it into a palette weight.
	OnsetRate float64 `json:"onset_rate"`
	// Tonality is the bright/dark axis in [-1,1]: +1 a track in a major key,
	// -1 a minor one, and 0 unmeasured. Over a playlist it is the mean of
	// those, so it reads as how lopsidedly major the playlist is.
	//
	// Signed rather than a [0,1] proportion so that unmeasured and evenly
	// split are the same number, and both mean "this axis has nothing to say"
	// rather than "every track is in a minor key".
	Tonality float64 `json:"tonality"`
	Present  bool    `json:"present"`
}
