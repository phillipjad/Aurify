package lyrics

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// incompressible returns n bytes that flate cannot shrink, so a test can reason
// about sizes. Real lyrics compress about 2.7x, which is exactly why the budget
// counts compressed bytes, and equally why size-sensitive tests cannot use
// repetitive filler: a thousand identical characters costs almost nothing.
func incompressible(n int) string {
	r := rand.New(rand.NewSource(1))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return string(b)
}

func entry(key, text string) domain.CachedLyrics {
	return domain.CachedLyrics{Key: key, Lyrics: text, Found: text != "", FetchedAt: time.Now()}
}

func TestRoundTripsThroughCompression(t *testing.T) {
	c := newMemoryCache(DefaultMemoryBudget)

	for name, text := range map[string]string{
		"plain":      "hello\nworld",
		"empty":      "",
		"unicode":    "作詞: 誰か\nLa vie en rose — naïve café",
		"repetitive": strings.Repeat("chorus line\n", 500),
	} {
		c.put(entry(name, text))
		got, ok := c.get(name)
		if !ok {
			t.Fatalf("%s: expected a hit", name)
		}
		if got.Lyrics != text {
			t.Errorf("%s: round trip changed the text (%d bytes in, %d out)", name, len(text), len(got.Lyrics))
		}
	}
}

// A hit whose answer is "no lyrics exist" is still a hit, and must not send the
// caller back to the provider.
func TestNegativeEntryIsAHit(t *testing.T) {
	c := newMemoryCache(DefaultMemoryBudget)
	c.put(domain.CachedLyrics{Key: "k", Found: false, FetchedAt: time.Now()})

	got, ok := c.get("k")
	if !ok {
		t.Fatal("expected a hit for a cached negative")
	}
	if got.Found || got.Lyrics != "" {
		t.Fatalf("got %+v, want an empty, not-found entry", got)
	}
}

// The budget is bytes, not entries: the same cache should hold far more short
// entries than long ones, which an entry count cannot express.
func TestEvictsByBytesNotCount(t *testing.T) {
	// Deliberately tiny so eviction is reachable in a test.
	c := newMemoryCache(8 << 10)

	for i := range 200 {
		c.put(entry(string(rune('a'+i%26))+string(rune(i)), incompressible(400)))
	}
	if c.used > c.budget {
		t.Fatalf("used %d bytes over a budget of %d", c.used, c.budget)
	}
	if len(c.entries) == 0 {
		t.Fatal("evicted everything")
	}
	if len(c.entries) >= 200 {
		t.Fatalf("kept %d entries with an 8KB budget: nothing was evicted", len(c.entries))
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := newMemoryCache(4 << 10)
	body := incompressible(600)

	c.put(entry("first", body))
	c.put(entry("second", body))
	// Touch the first so the second becomes the eviction candidate.
	if _, ok := c.get("first"); !ok {
		t.Fatal("setup: first should be present")
	}
	for i := range 20 {
		c.put(entry("filler"+string(rune('a'+i)), body))
	}

	if _, ok := c.get("second"); ok {
		t.Error("the least recently used entry survived")
	}
}

// One pathological result must not be able to evict everything to make room for
// itself.
func TestRefusesAnOversizedEntry(t *testing.T) {
	c := newMemoryCache(DefaultMemoryBudget)
	c.put(entry("keep", "something small"))

	// Genuinely incompressible, since the limit is on the compressed size: three
	// megabytes of repetitive text would shrink under it and be admitted.
	c.put(entry("huge", incompressible(maxEntryBytes*2)))

	if _, ok := c.get("huge"); ok {
		t.Error("an oversized entry was admitted")
	}
	if _, ok := c.get("keep"); !ok {
		t.Error("admitting an oversized entry disturbed the existing ones")
	}
}

// Overwriting a key must not leak its old size, or the budget drifts until the
// cache holds far less than it should.
func TestAccountingSurvivesOverwrite(t *testing.T) {
	c := newMemoryCache(DefaultMemoryBudget)
	body := incompressible(2000)

	c.put(entry("k", body))
	first := c.used
	for range 50 {
		c.put(entry("k", body))
	}

	if c.used != first {
		t.Fatalf("used drifted from %d to %d across overwrites of one key", first, c.used)
	}
	if len(c.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(c.entries))
	}
}
