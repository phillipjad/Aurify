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
	"maps"
	"net/http"
	"slices"
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
	// generation. At one rate-limited search per second it is a stopwatch, not a
	// budget: roughly how many seconds the analyzing stage lasts on a cold
	// playlist. Measured and argued in
	// docs/adr/0020-coverage-weighted-features.md.
	maxLookupsPerRun = 100
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
	now := time.Now().UTC()
	for i, key := range keys {
		// Fresh is what enforces both expiries: the negative TTL, and the
		// version stamp that retires rows written before the extractor changed
		// shape. A row that fails it is a miss, so it is looked up again and
		// the upsert overwrites it in place.
		if entry, ok := cached[key]; ok && entry.Fresh(now) {
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
	for found := range c.resolveAll(ctx, tracks, pending) {
		features, err := c.features(ctx, found.ids)
		if err != nil {
			// Network trouble is not cached: an outage would otherwise be
			// remembered as "this track has no features", permanently.
			continue
		}
		out[found.index] = features
		fresh = append(fresh, domain.CachedFeatures{
			Key:       keys[found.index],
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

// candidates carries one track's MusicBrainz recording ids to the second stage.
type candidates struct {
	index int
	ids   []string
}

// resolveAll runs the rate-limited half of the lookup on its own goroutine, so
// the unlimited half can happen during the wait rather than after it. Only the
// MusicBrainz search is throttled, and it was not most of the cost; overlapping
// the two stages roughly halved the phase. Measured, and the reason there is no
// batch alternative, in docs/adr/0020-coverage-weighted-features.md.
//
// The channel is buffered for the whole run so searching never waits on
// fetching.
func (c *Client) resolveAll(
	ctx context.Context,
	tracks []domain.Track,
	pending []int,
) <-chan candidates {
	out := make(chan candidates, len(pending))
	go func() {
		defer close(out)
		for _, i := range pending {
			if err := c.limit.wait(ctx); err != nil {
				return
			}
			ids, err := c.resolve(ctx, tracks[i])
			if err != nil {
				// Not cached, for the same reason a failed fetch is not.
				continue
			}
			out <- candidates{index: i, ids: ids}
		}
	}()
	return out
}

// features turns one track's candidate recordings into its features.
//
// The two endpoints are independent, and only the MusicBrainz search upstream is
// rate limited, so they go out together rather than in sequence. That is what
// makes the pace signal free: the low-level fetch finishes inside the second the
// search is already spending, and the analyzing phase stays on the rate limit's
// floor. Measured in docs/adr/0023-pace-from-onset-rate.md.
func (c *Client) features(ctx context.Context, ids []string) (domain.AudioFeatures, error) {
	if len(ids) == 0 {
		// A real answer: nothing in MusicBrainz matches this closely enough.
		return domain.AudioFeatures{Present: false}, nil
	}

	var rate, tone float64
	done := make(chan struct{})
	go func() {
		defer close(done)
		// A failed low-level fetch is not a failed lookup. The classifiers are
		// most of the palette and stand on their own; a track that comes back
		// without a pace or a key contributes to no other dimension
		// differently.
		rate, tone, _ = c.lowLevel(ctx, ids)
	}()

	features, err := c.highLevel(ctx, ids)
	<-done
	if err != nil || !features.Present {
		return features, err
	}
	features.OnsetRate = rate
	features.Tonality = tone
	return features, nil
}

// highLevel takes the first candidate recording that carries usable classifiers.
// Present false with a nil error is a real answer: the recording matched, but
// nobody ever submitted an analysis for it.
func (c *Client) highLevel(ctx context.Context, ids []string) (domain.AudioFeatures, error) {
	var out domain.AudioFeatures
	err := c.eachDocument(ctx, "high-level", ids, func(raw json.RawMessage) bool {
		var doc document
		if err := json.Unmarshal(raw, &doc); err != nil {
			return false
		}
		features := toFeatures(doc)
		if !features.Present {
			return false
		}
		out = features
		return true
	})
	return out, err
}

// lowLevel takes the first candidate recording that was analyzed for rhythm,
// and reads both numbers Aurify wants out of that one submission.
//
// Both in one pass deliberately. eachDocument stops at the first submission the
// callback accepts, so two callbacks would be two independent searches and
// could take the pace from one recording and the key from another: a karaoke
// version's tempo against the original's key, presented as one track.
//
// Zero rate means nobody measured one, which analysis.normalizePace reads as no
// weight rather than as a motionless track. Zero tonality means the same for
// the bright/dark axis.
func (c *Client) lowLevel(ctx context.Context, ids []string) (float64, float64, error) {
	var rate, tone float64
	err := c.eachDocument(ctx, "low-level", ids, func(raw json.RawMessage) bool {
		var doc lowLevelDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return false
		}
		rate = doc.Rhythm.OnsetRate
		tone = tonality(doc.Tonal.KeyScale)
		return rate > 0
	})
	return rate, tone, err
}

// eachDocument asks one of the AcousticBrainz batch endpoints about every
// candidate recording at once, and hands each submission to use until it reports
// it has what it wanted.
func (c *Client) eachDocument(
	ctx context.Context,
	endpointPath string,
	ids []string,
	use func(json.RawMessage) bool,
) error {
	endpoint := fmt.Sprintf(
		"%s/api/v1/%s?recording_ids=%s", c.acousticBrainzURL, endpointPath, joinIDs(ids),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("acousticbrainz: status %d", resp.StatusCode)
	}

	// Keyed by recording id, then by submission offset. mbid_mapping is
	// bookkeeping rather than a recording.
	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}

	// Candidate order is MusicBrainz's relevance order, so preferring it over
	// map iteration order keeps the answer stable between runs.
	for _, id := range ids {
		raw, ok := body[id]
		if !ok {
			continue
		}
		var submissions map[string]json.RawMessage
		if err := json.Unmarshal(raw, &submissions); err != nil {
			continue
		}
		// Sorted because ranging a map is ranging in a random order, and this
		// picks which submission of a recording answers for it. Observed live
		// before this: the same Burial recording read 5.18 then 5.28 onsets per
		// second on consecutive runs. The order itself carries no meaning; that
		// it is the same order every time does.
		for _, offset := range slices.Sorted(maps.Keys(submissions)) {
			if use(submissions[offset]) {
				return nil
			}
		}
	}
	return nil
}
