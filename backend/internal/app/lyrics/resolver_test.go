package lyrics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// --- fakes ---

type fakeStore struct {
	mu       sync.Mutex
	entries  map[string]domain.CachedLyrics
	findCall int
	saveCall int
	saved    []domain.CachedLyrics
	findErr  error
	saveErr  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{entries: map[string]domain.CachedLyrics{}}
}

func (s *fakeStore) FindMany(_ context.Context, keys []string) (map[string]domain.CachedLyrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findCall++
	if s.findErr != nil {
		return nil, s.findErr
	}
	out := map[string]domain.CachedLyrics{}
	for _, k := range keys {
		if e, ok := s.entries[k]; ok {
			out[k] = e
		}
	}
	return out, nil
}

func (s *fakeStore) SaveMany(_ context.Context, entries []domain.CachedLyrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCall++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, entries...)
	for _, e := range entries {
		s.entries[e.Key] = e
	}
	return nil
}

type fakeClient struct {
	mu       sync.Mutex
	calls    int
	inFlight int32
	peak     int32
	delay    func(title string) time.Duration
	answer   func(title string) (string, error)
}

func (c *fakeClient) Fetch(_ context.Context, track domain.Track) (string, error) {
	now := atomic.AddInt32(&c.inFlight, 1)
	for {
		peak := atomic.LoadInt32(&c.peak)
		if now <= peak || atomic.CompareAndSwapInt32(&c.peak, peak, now) {
			break
		}
	}
	defer atomic.AddInt32(&c.inFlight, -1)

	c.mu.Lock()
	c.calls++
	c.mu.Unlock()

	if c.delay != nil {
		time.Sleep(c.delay(track.Title))
	}
	if c.answer != nil {
		return c.answer(track.Title)
	}
	return "lyrics for " + track.Title, nil
}

func tracksNamed(names ...string) []domain.Track {
	out := make([]domain.Track, len(names))
	for i, n := range names {
		out[i] = domain.Track{ID: n, Title: n, Artists: []string{"artist"}}
	}
	return out
}

// --- tests ---

// The whole reason the resolver exists: a playlist costs one read and one write,
// not one of each per track.
func TestResolveUsesOneQueryAndOneWriteForAWholePlaylist(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{}
	r := NewResolver(store, client)

	tracks := tracksNamed("a", "b", "c", "d", "e", "f", "g", "h")
	got := r.Resolve(context.Background(), tracks)

	if len(got) != len(tracks) {
		t.Fatalf("got %d results for %d tracks", len(got), len(tracks))
	}
	if store.findCall != 1 {
		t.Errorf("repository reads = %d, want exactly 1 for the whole playlist", store.findCall)
	}
	if store.saveCall != 1 {
		t.Errorf("repository writes = %d, want exactly 1 for the whole playlist", store.saveCall)
	}
	if client.calls != len(tracks) {
		t.Errorf("provider calls = %d, want one per track on a cold cache", client.calls)
	}
}

func TestResolveIsIndexAlignedWhenLookupsFinishOutOfOrder(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{
		// Odd positions answer late, so completion order differs from input order.
		delay: func(title string) time.Duration {
			if len(title)%2 == 1 {
				return 40 * time.Millisecond
			}
			return 0
		},
	}
	r := NewResolver(store, client)

	tracks := tracksNamed("a", "bb", "ccc", "dddd", "eeeee", "ffffff", "g", "hh")
	got := r.Resolve(context.Background(), tracks)

	for i, track := range tracks {
		want := "lyrics for " + track.Title
		if got[i] != want {
			t.Fatalf("index %d = %q, want %q: results are not aligned with tracks", i, got[i], want)
		}
	}
}

func TestResolveBoundsConcurrency(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{delay: func(string) time.Duration { return 20 * time.Millisecond }}
	r := NewResolver(store, client)

	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("track-%d", i)
	}
	r.Resolve(context.Background(), tracksNamed(names...))

	if peak := atomic.LoadInt32(&client.peak); peak > fetchConcurrency {
		t.Fatalf("peak concurrent provider calls = %d, want at most %d", peak, fetchConcurrency)
	}
}

func TestSecondResolveIsServedFromMemory(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{}
	r := NewResolver(store, client)
	tracks := tracksNamed("a", "b", "c")

	r.Resolve(context.Background(), tracks)
	callsAfterFirst := client.calls
	findsAfterFirst := store.findCall

	r.Resolve(context.Background(), tracks)

	if client.calls != callsAfterFirst {
		t.Errorf("provider calls rose to %d on a repeat resolve", client.calls)
	}
	if store.findCall != findsAfterFirst {
		t.Errorf("repository reads rose to %d: memory should have answered", store.findCall)
	}
}

// A cold process with a warm database must not go back to the provider.
func TestResolveIsServedFromTheRepositoryWithoutTouchingTheProvider(t *testing.T) {
	store := newFakeStore()
	store.entries["artist\na"] = domain.CachedLyrics{
		Key: "artist\na", Lyrics: "stored", Found: true, FetchedAt: time.Now(),
	}
	client := &fakeClient{}
	r := NewResolver(store, client)

	got := r.Resolve(context.Background(), tracksNamed("a"))

	if got[0] != "stored" {
		t.Fatalf("got %q, want the stored lyrics", got[0])
	}
	if client.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 when the repository had the answer", client.calls)
	}
}

// Caching the absence is the highest-value part of the table, so it has to be
// honoured rather than re-asked on every run.
func TestNegativeEntriesAreHonouredUntilTheyExpire(t *testing.T) {
	store := newFakeStore()
	store.entries["artist\nfresh"] = domain.CachedLyrics{
		Key: "artist\nfresh", Found: false, FetchedAt: time.Now(),
	}
	store.entries["artist\nstale"] = domain.CachedLyrics{
		Key:       "artist\nstale",
		Found:     false,
		FetchedAt: time.Now().Add(-domain.NegativeLyricsTTL - time.Hour),
	}
	client := &fakeClient{}
	r := NewResolver(store, client)

	r.Resolve(context.Background(), tracksNamed("fresh", "stale"))

	if client.calls != 1 {
		t.Fatalf("provider calls = %d, want 1: the fresh negative should be honoured and the expired one refetched", client.calls)
	}
}

// A failed lookup is not an answer. Recording it as "no lyrics exist" would let
// one bad spell at the provider mark a library as lyric-less for a month.
func TestFailuresAreNotCachedAsNegatives(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{
		answer: func(title string) (string, error) {
			if title == "broken" {
				return "", errors.New("provider is unwell")
			}
			return "lyrics for " + title, nil
		},
	}
	r := NewResolver(store, client)

	got := r.Resolve(context.Background(), tracksNamed("fine", "broken"))

	if got[0] == "" {
		t.Error("the healthy track lost its lyrics because a sibling failed")
	}
	if got[1] != "" {
		t.Errorf("failed track = %q, want empty", got[1])
	}
	for _, saved := range store.saved {
		if saved.Key == "artist\nbroken" {
			t.Fatal("a failed lookup was cached, which would persist an outage as fact")
		}
	}
}

// Found in live verification, and missed by the test above because that one
// exercised a client returning an *error*. A circuit breaker that skips a call
// used to answer with an empty success, which is indistinguishable from "this
// track has no lyrics", so an outage was written into the cache as 97 confirmed
// absences that would have stood for the whole negative TTL.
func TestSkippedLookupsAreNotCachedAsNegatives(t *testing.T) {
	store := newFakeStore()
	skipped := errors.New("circuit is open, nothing was asked")
	client := &fakeClient{answer: func(string) (string, error) { return "", skipped }}
	r := NewResolver(store, client)

	got := r.Resolve(context.Background(), tracksNamed("a", "b", "c"))

	for i, text := range got {
		if text != "" {
			t.Errorf("index %d = %q, want empty when nothing was asked", i, text)
		}
	}
	if len(store.saved) != 0 {
		t.Fatalf("%d entries cached from lookups that never happened", len(store.saved))
	}
}

// The caches are an optimisation. Losing one slows a generation down; it must
// not fail it.
func TestResolveSurvivesACacheThatIsDown(t *testing.T) {
	store := newFakeStore()
	store.findErr = errors.New("database is down")
	store.saveErr = errors.New("database is down")
	client := &fakeClient{}
	r := NewResolver(store, client)

	got := r.Resolve(context.Background(), tracksNamed("a", "b"))

	for i, text := range got {
		if text == "" {
			t.Fatalf("index %d came back empty despite the provider answering", i)
		}
	}
}

// Found in live verification: a playlist holding the same song twice, or two
// tracks normalizing to one key, produced a batch with duplicate keys. PostgreSQL
// rejects that outright with "ON CONFLICT DO UPDATE command cannot affect row a
// second time", losing the entire batch rather than just the duplicate, so
// nothing was cached at all. It also meant fetching the same track twice.
func TestDuplicateTracksAreFetchedOnceAndWrittenOnce(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{}
	r := NewResolver(store, client)

	// "a" appears three times, in non-adjacent positions.
	tracks := tracksNamed("a", "b", "a", "c", "a")
	got := r.Resolve(context.Background(), tracks)

	if client.calls != 3 {
		t.Errorf("provider calls = %d, want 3 for 3 distinct tracks among 5", client.calls)
	}

	seen := map[string]int{}
	for _, e := range store.saved {
		seen[e.Key]++
	}
	for key, n := range seen {
		if n > 1 {
			t.Errorf("key %q written %d times in one batch, which PostgreSQL rejects", key, n)
		}
	}

	// Every occurrence still gets its answer, not just the first.
	for i, track := range tracks {
		want := "lyrics for " + track.Title
		if got[i] != want {
			t.Errorf("index %d = %q, want %q", i, got[i], want)
		}
	}
}

func TestResolveHandlesNoTracks(t *testing.T) {
	store := newFakeStore()
	client := &fakeClient{}

	if got := NewResolver(store, client).Resolve(context.Background(), nil); len(got) != 0 {
		t.Fatalf("got %d results for no tracks", len(got))
	}
	if store.findCall != 0 || client.calls != 0 {
		t.Error("an empty playlist should touch nothing")
	}
}
