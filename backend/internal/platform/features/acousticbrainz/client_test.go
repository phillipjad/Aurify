package acousticbrainz

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// memoryCache is a ports.TrackFeatureRepository that records what it was asked.
type memoryCache struct {
	mu      sync.Mutex
	entries map[string]domain.CachedFeatures
	saved   []domain.CachedFeatures
	findErr error
}

func newCache() *memoryCache {
	return &memoryCache{entries: map[string]domain.CachedFeatures{}}
}

func (c *memoryCache) FindMany(_ context.Context, keys []string) (map[string]domain.CachedFeatures, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.findErr != nil {
		return nil, c.findErr
	}
	out := map[string]domain.CachedFeatures{}
	for _, k := range keys {
		if e, ok := c.entries[k]; ok {
			out[k] = e
		}
	}
	return out, nil
}

func (c *memoryCache) SaveMany(_ context.Context, entries []domain.CachedFeatures) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saved = append(c.saved, entries...)
	for _, e := range entries {
		c.entries[e.Key] = e
	}
	return nil
}

// stub serves both upstreams and counts what was asked of each.
type stub struct {
	server   *httptest.Server
	mbCalls  int
	abCalls  int
	mu       sync.Mutex
	mbScore  int
	abData   bool
	lastPath string
}

func newStub(t *testing.T, score int, withData bool) *stub {
	t.Helper()
	s := &stub{mbScore: score, abData: withData}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.lastPath = r.URL.Path + "?" + r.URL.RawQuery
		switch {
		case strings.HasPrefix(r.URL.Path, "/ws/2/recording"):
			s.mbCalls++
		case strings.HasPrefix(r.URL.Path, "/api/v1/high-level"):
			s.abCalls++
		}
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/ws/2/recording") {
			fmt.Fprintf(w, `{"recordings":[{"id":"mbid-1","score":%d},{"id":"mbid-2","score":%d}]}`,
				s.mbScore, s.mbScore)
			return
		}
		if !s.abData {
			fmt.Fprint(w, `{"mbid_mapping":{}}`)
			return
		}
		fmt.Fprint(w, `{"mbid-1":{"0":`+godsCountry+`}}`)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func clientFor(s *stub, cache *memoryCache) *Client {
	c := New(cache, "test")
	c.musicBrainzURL = s.server.URL
	c.acousticBrainzURL = s.server.URL
	// The real one-second limit would make this suite take minutes.
	c.limit = newLimiter(time.Millisecond)
	return c
}

func tracks(n int) []domain.Track {
	out := make([]domain.Track, n)
	for i := range out {
		out[i] = domain.Track{Title: fmt.Sprintf("song %d", i), Artists: []string{"an artist"}}
	}
	return out
}

func TestLooksUpAndCachesFeatures(t *testing.T) {
	cache, s := newCache(), newStub(t, 100, true)
	got := clientFor(s, cache).Lookup(context.Background(), tracks(2))

	if len(got) != 2 || !got[0].Present || !got[1].Present {
		t.Fatalf("features = %+v, want both present", got)
	}
	if len(cache.saved) != 2 {
		t.Errorf("cached %d entries, want 2", len(cache.saved))
	}
}

// Without a negative entry, a library full of non-music (this one is full of car
// repair videos) re-pays a second of MusicBrainz rate limit per track on every
// single generation, forever.
func TestMissesAreCachedToo(t *testing.T) {
	cache, s := newCache(), newStub(t, 100, false)
	got := clientFor(s, cache).Lookup(context.Background(), tracks(1))

	if got[0].Present {
		t.Error("a recording with no submitted analysis should not be Present")
	}
	if len(cache.saved) != 1 || cache.saved[0].Features.Present {
		t.Fatalf("the miss was not cached: %+v", cache.saved)
	}
}

func TestCachedTracksNeverReachTheNetwork(t *testing.T) {
	cache, s := newCache(), newStub(t, 100, true)
	cache.entries[domain.LyricsKey(tracks(1)[0])] = domain.CachedFeatures{
		Key:      domain.LyricsKey(tracks(1)[0]),
		Features: domain.AudioFeatures{Energy: 0.5, Present: true},
	}

	got := clientFor(s, cache).Lookup(context.Background(), tracks(1))

	if s.mbCalls != 0 || s.abCalls != 0 {
		t.Errorf("hit the network despite a cache hit: mb=%d ab=%d", s.mbCalls, s.abCalls)
	}
	if got[0].Energy != 0.5 {
		t.Errorf("energy = %.2f, want the cached 0.5", got[0].Energy)
	}
}

// At one MusicBrainz request per second, an uncapped 194-track playlist would
// add three minutes to a synchronous generation.
func TestUncachedLookupsAreCapped(t *testing.T) {
	cache, s := newCache(), newStub(t, 100, true)
	clientFor(s, cache).Lookup(context.Background(), tracks(maxLookupsPerRun+15))

	if s.mbCalls != maxLookupsPerRun {
		t.Errorf("made %d MusicBrainz calls, want the cap of %d", s.mbCalls, maxLookupsPerRun)
	}
}

// MusicBrainz always answers with something. A fuzzy hit on a car repair video
// would attach a stranger's audio features to it.
func TestWeakMatchesAreDiscarded(t *testing.T) {
	cache, s := newCache(), newStub(t, 55, true)
	got := clientFor(s, cache).Lookup(context.Background(), tracks(1))

	if got[0].Present {
		t.Error("a score-55 match was accepted; only near-exact hits should be")
	}
	if s.abCalls != 0 {
		t.Error("asked AcousticBrainz about a match that should have been discarded")
	}
}

// The same song twice in a playlist is common and must not cost two lookups.
func TestDuplicateTracksAreLookedUpOnce(t *testing.T) {
	cache, s := newCache(), newStub(t, 100, true)
	same := []domain.Track{
		{Title: "one song", Artists: []string{"an artist"}},
		{Title: "One  Song", Artists: []string{"An Artist"}},
	}

	got := clientFor(s, cache).Lookup(context.Background(), same)

	if s.mbCalls != 1 {
		t.Errorf("made %d lookups for the same song, want 1", s.mbCalls)
	}
	if !got[0].Present || !got[1].Present {
		t.Errorf("both copies should carry the answer, got %+v", got)
	}
}

// An outage remembered as "this track has no features" would be permanent.
func TestNetworkFailuresAreNotCached(t *testing.T) {
	cache := newCache()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer dead.Close()

	c := New(cache, "test")
	c.musicBrainzURL, c.acousticBrainzURL = dead.URL, dead.URL
	c.limit = newLimiter(time.Millisecond)

	got := c.Lookup(context.Background(), tracks(1))

	if got[0].Present {
		t.Error("a failed lookup should not report features")
	}
	if len(cache.saved) != 0 {
		t.Errorf("cached %d entries from a failing upstream", len(cache.saved))
	}
}

// A generation must survive an unreadable cache rather than fail.
func TestAnUnreadableCacheDegradesToLookups(t *testing.T) {
	cache, s := newCache(), newStub(t, 100, true)
	cache.findErr = fmt.Errorf("database is down")

	got := clientFor(s, cache).Lookup(context.Background(), tracks(1))

	if !got[0].Present {
		t.Error("should have looked the track up despite the cache read failing")
	}
}

// MusicBrainz blocks callers who exceed one request per second, so this is
// enforced rather than hoped for.
func TestTheRateLimiterSerializes(t *testing.T) {
	l := newLimiter(20 * time.Millisecond)
	start := time.Now()
	for range 3 {
		if err := l.wait(context.Background()); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	// Three calls means two enforced gaps.
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("three calls took %v, want at least two 20ms gaps", elapsed)
	}
}

func TestTheRateLimiterHonoursCancellation(t *testing.T) {
	l := newLimiter(time.Hour)
	if err := l.wait(context.Background()); err != nil {
		t.Fatalf("first wait: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.wait(ctx); err == nil {
		t.Error("a cancelled context should abandon the wait, not block for an hour")
	}
}
