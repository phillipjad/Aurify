package domain

import (
	"strings"
	"time"
)

// NegativeLyricsTTL is how long "lrclib has nothing for this track" is trusted.
//
// Positive entries never expire, because lyrics do not change. A negative one
// does, because a track absent from lrclib today may be contributed tomorrow,
// and without an expiry a single early miss would be permanent.
const NegativeLyricsTTL = 30 * 24 * time.Hour

// CachedLyrics is one remembered lyric lookup.
type CachedLyrics struct {
	// Key identifies the track independently of which platform it came from.
	Key string
	// Lyrics is the plain text, empty when Found is false.
	Lyrics string
	// Found records whether the provider had anything. False is a real answer
	// worth storing, not an absence of one: most lookups for a YouTube Music
	// library will never match, and re-asking every time is what this avoids.
	Found     bool
	FetchedAt time.Time
}

// Fresh reports whether an entry may still be used.
func (c CachedLyrics) Fresh(now time.Time) bool {
	if c.Found {
		return true
	}
	return now.Sub(c.FetchedAt) < NegativeLyricsTTL
}

// NegativeFeaturesTTL is how long "no features exist for this track" is trusted.
//
// Longer than the lyric equivalent because AcousticBrainz stopped collecting in
// 2022: a track absent today will still be absent next month, and re-asking
// costs a second of MusicBrainz rate limit each time. Not infinite, since the
// match itself may improve as MusicBrainz gains releases.
const NegativeFeaturesTTL = 90 * 24 * time.Hour

// CachedFeatures is one remembered feature lookup.
type CachedFeatures struct {
	// Key is domain.LyricsKey(track): the same normalized "artist\ntitle" the
	// lyric cache uses, so both caches agree on what counts as the same song.
	Key string
	// Features carries Present false for a track that could not be matched,
	// which is a real answer worth storing rather than an absence of one.
	Features  AudioFeatures
	FetchedAt time.Time
}

// Fresh reports whether an entry may still be used. Measured features never
// expire, because a recording's audio does not change.
func (c CachedFeatures) Fresh(now time.Time) bool {
	if c.Features.Present {
		return true
	}
	return now.Sub(c.FetchedAt) < NegativeFeaturesTTL
}

// LyricsKey builds the cache key for a track.
//
// Case and spacing are normalized so trivially different spellings share an
// entry. Duration is deliberately not part of it: the same song from two sources
// often reports slightly different lengths, and splitting the cache on that
// would defeat the point.
func LyricsKey(track Track) string {
	artist := ""
	if len(track.Artists) > 0 {
		artist = track.Artists[0]
	}
	return normalizeLyricsKeyPart(artist) + "\n" + normalizeLyricsKeyPart(track.Title)
}

func normalizeLyricsKeyPart(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
