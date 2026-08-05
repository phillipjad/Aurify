// Package acousticbrainz fills in acoustic features for tracks whose DSP
// supplies none, which on YouTube Music is every track.
//
// The data is real Essentia analysis of the audio, computed by the
// AcousticBrainz project and served free with no key. Aurify never sees the
// audio itself: getting it would mean downloading from YouTube against their
// terms, so measured-by-someone-else is as close to a measurement as this can
// honestly get (see docs/adr/0018-audio-features.md).
//
// Two properties of the source shape everything here. Lookups are by MusicBrainz
// recording id, so every track needs a name match first, and that search is rate
// limited to one request per second. And collection stopped in 2022, so recent
// releases simply are not in it.
package acousticbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

const (
	// defaultMusicBrainzURL and defaultAcousticBrainzURL are overridable only so
	// tests can point at an httptest server.
	defaultMusicBrainzURL    = "https://musicbrainz.org"
	defaultAcousticBrainzURL = "https://acousticbrainz.org"

	// idsPerTrack is how many candidate recording ids to carry forward per
	// track. A song has many recordings and only some were ever submitted, so
	// asking about several costs nothing extra: the lookup is one batched call
	// either way.
	idsPerTrack = 8

	// maxLookupsPerRun caps how many uncached tracks reach the network in one
	// generation.
	//
	// The palette is a mean, so a sample estimates it about as well as the whole
	// playlist, and at one MusicBrainz request per second an uncapped 194-track
	// playlist would add three minutes to a synchronous generation. Each run
	// widens the cache, so a playlist converges over a few generations.
	maxLookupsPerRun = 25
)

// Client looks features up through MusicBrainz and AcousticBrainz, in front of a
// cache.
type Client struct {
	cache             ports.TrackFeatureRepository
	musicBrainzURL    string
	acousticBrainzURL string
	userAgent         string
	limit             *limiter
	client            *http.Client
}

var _ ports.FeatureSource = (*Client)(nil)

// New constructs a feature source. An empty cache is not supported: without one
// every generation would re-pay the rate-limited matching.
func New(cache ports.TrackFeatureRepository, version string) *Client {
	return &Client{
		cache:             cache,
		musicBrainzURL:    defaultMusicBrainzURL,
		acousticBrainzURL: defaultAcousticBrainzURL,
		// MusicBrainz requires contact details and blocks generic agents.
		userAgent: fmt.Sprintf("Aurify/%s (https://github.com/phillipjad/Aurify)", version),
		limit:     newLimiter(mbInterval),
		client:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Lookup returns features index-aligned with tracks. Unmatched tracks come back
// with Present false, which the analysis engine already skips.
func (c *Client) Lookup(ctx context.Context, tracks []domain.Track) []domain.AudioFeatures {
	out := make([]domain.AudioFeatures, len(tracks))
	if len(tracks) == 0 {
		return out
	}

	keys := make([]string, len(tracks))
	for i, t := range tracks {
		keys[i] = domain.LyricsKey(t)
	}

	// One query for the whole playlist, as with lyrics: a per-track interface
	// would put hundreds of round trips in front of work that needs one.
	cached, err := c.cache.FindMany(ctx, keys)
	if err != nil {
		// A cache that cannot be read is not a reason to fail a generation, but
		// it does mean everything below looks like a miss.
		cached = map[string]domain.CachedFeatures{}
	}

	// Deduplicate: the same track can appear twice in a playlist, and asking
	// about it twice would spend the budget on a known answer.
	pending := make([]int, 0, len(tracks))
	seen := make(map[string]bool, len(tracks))
	for i, key := range keys {
		if entry, ok := cached[key]; ok {
			out[i] = entry.Features
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		pending = append(pending, i)
	}

	if len(pending) > maxLookupsPerRun {
		pending = pending[:maxLookupsPerRun]
	}

	fresh := make([]domain.CachedFeatures, 0, len(pending))
	for _, i := range pending {
		features, err := c.fetch(ctx, tracks[i])
		if err != nil {
			// Network trouble is not cached: an outage would otherwise be
			// remembered as "this track has no features", permanently.
			continue
		}
		out[i] = features
		fresh = append(fresh, domain.CachedFeatures{
			Key:       keys[i],
			Features:  features,
			FetchedAt: time.Now().UTC(),
		})
	}

	// Misses are written too. Most of this library will never match, and without
	// a negative entry each one would re-pay a second of rate limit on every
	// single generation.
	if len(fresh) > 0 {
		_ = c.cache.SaveMany(ctx, fresh)
	}

	// Duplicates skipped above still need their answer.
	for i, key := range keys {
		if !out[i].Present {
			for _, f := range fresh {
				if f.Key == key {
					out[i] = f.Features
					break
				}
			}
		}
	}
	return out
}

// fetch resolves one track to features, or to an absent result.
func (c *Client) fetch(ctx context.Context, track domain.Track) (domain.AudioFeatures, error) {
	if err := c.limit.wait(ctx); err != nil {
		return domain.AudioFeatures{}, err
	}
	ids, err := c.resolve(ctx, track)
	if err != nil {
		return domain.AudioFeatures{}, err
	}
	if len(ids) == 0 {
		// A real answer: nothing in MusicBrainz matches this closely enough.
		return domain.AudioFeatures{Present: false}, nil
	}
	return c.highLevel(ctx, ids)
}

// highLevel asks AcousticBrainz about every candidate recording at once and
// takes the first that carries usable data.
func (c *Client) highLevel(ctx context.Context, ids []string) (domain.AudioFeatures, error) {
	endpoint := fmt.Sprintf("%s/api/v1/high-level?recording_ids=%s", c.acousticBrainzURL, joinIDs(ids))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.AudioFeatures{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return domain.AudioFeatures{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return domain.AudioFeatures{}, fmt.Errorf("acousticbrainz: status %d", resp.StatusCode)
	}

	// Keyed by recording id, then by submission offset. mbid_mapping is
	// bookkeeping rather than a recording.
	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return domain.AudioFeatures{}, err
	}

	// Candidate order is MusicBrainz's relevance order, so preferring it over
	// map iteration order keeps the answer stable between runs.
	for _, id := range ids {
		raw, ok := body[id]
		if !ok {
			continue
		}
		var submissions map[string]document
		if err := json.Unmarshal(raw, &submissions); err != nil {
			continue
		}
		for _, doc := range submissions {
			if features := toFeatures(doc); features.Present {
				return features, nil
			}
		}
	}
	// Matched a recording, but nobody ever submitted an analysis for it.
	return domain.AudioFeatures{Present: false}, nil
}
