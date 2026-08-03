package lyrics

import (
	"bytes"
	"compress/flate"
	"container/list"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

const (
	// DefaultMemoryBudget is how much compressed lyric text is kept resident.
	//
	// A library of a thousand tracks is only a couple of megabytes uncompressed,
	// so this is generous for a single user and degrades by eviction rather than
	// by failing for many.
	DefaultMemoryBudget = 64 << 20

	// maxEntryBytes refuses any single oversized entry. Without it one
	// pathological result could evict everything else to make room for itself.
	maxEntryBytes = 1 << 20

	// entryOverhead approximates the map, list node and struct cost of an entry,
	// so the budget accounts for small entries rather than pretending they are
	// free.
	entryOverhead = 96
)

// memoryCache is a byte-budgeted LRU of lyric lookups.
//
// Bounded by bytes rather than entry count because lyrics range from a few
// hundred bytes to tens of kilobytes, and a count cannot express that: the same
// limit would be either far too small for a playlist or far too large for a
// process.
//
// Values are held compressed. Measured on real lyrics that is about 2.7x, which
// both stretches the budget and makes the accounting honest, since the charge is
// what is actually resident. Compression is stdlib flate: at roughly a kilobyte
// an input has too little internal history for a stronger algorithm to beat it,
// and a preset dictionary, which would roughly double the ratio, is not worth
// the machinery while the budget is already far larger than the data.
type memoryCache struct {
	mu      sync.Mutex
	budget  int
	used    int
	entries map[string]*list.Element
	order   *list.List // front is most recently used
}

type memoryEntry struct {
	key        string
	compressed []byte
	found      bool
	fetchedAt  time.Time
	size       int
}

func newMemoryCache(budget int) *memoryCache {
	if budget <= 0 {
		budget = DefaultMemoryBudget
	}
	return &memoryCache{
		budget:  budget,
		entries: make(map[string]*list.Element),
		order:   list.New(),
	}
}

// get returns a cached entry, decompressing it. The bool reports a hit, which is
// distinct from a hit whose answer is "no lyrics exist".
func (c *memoryCache) get(key string) (domain.CachedLyrics, bool) {
	c.mu.Lock()
	element, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return domain.CachedLyrics{}, false
	}
	c.order.MoveToFront(element)
	entry := element.Value.(*memoryEntry)
	compressed, found, fetchedAt := entry.compressed, entry.found, entry.fetchedAt
	c.mu.Unlock()

	// Decompression happens outside the lock: it is microseconds, but it is also
	// pure CPU that no other caller needs to wait behind.
	text, err := inflate(compressed)
	if err != nil {
		// A corrupt entry is treated as a miss rather than an error. The caller
		// can always fetch again, and there is nothing useful to report.
		return domain.CachedLyrics{}, false
	}
	return domain.CachedLyrics{Key: key, Lyrics: text, Found: found, FetchedAt: fetchedAt}, true
}

// put stores an entry, evicting least-recently-used ones until it fits.
func (c *memoryCache) put(entry domain.CachedLyrics) {
	compressed, err := deflate(entry.Lyrics)
	if err != nil {
		return
	}
	size := len(entry.Key) + len(compressed) + entryOverhead
	if size > maxEntryBytes || size > c.budget {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.entries[entry.Key]; ok {
		c.used -= existing.Value.(*memoryEntry).size
		c.order.Remove(existing)
		delete(c.entries, entry.Key)
	}

	element := c.order.PushFront(&memoryEntry{
		key:        entry.Key,
		compressed: compressed,
		found:      entry.Found,
		fetchedAt:  entry.FetchedAt,
		size:       size,
	})
	c.entries[entry.Key] = element
	c.used += size

	for c.used > c.budget {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		evicted := oldest.Value.(*memoryEntry)
		c.order.Remove(oldest)
		delete(c.entries, evicted.key)
		c.used -= evicted.size
	}
}

// Codecs are pooled because constructing one is far more expensive than using
// it: a flate writer allocates its whole compression window up front, which
// benchmarked at 816KB per call. At one call per cache write that is over a
// hundred megabytes of garbage for a single large playlist, to compress about a
// hundred kilobytes of text.
var (
	deflaters = sync.Pool{New: func() any {
		w, _ := flate.NewWriter(io.Discard, flate.DefaultCompression)
		return w
	}}
	inflaters = sync.Pool{New: func() any {
		return flate.NewReader(bytes.NewReader(nil))
	}}
)

func deflate(text string) ([]byte, error) {
	var buf bytes.Buffer
	w, _ := deflaters.Get().(*flate.Writer)
	defer deflaters.Put(w)

	w.Reset(&buf)
	if _, err := io.WriteString(w, text); err != nil {
		return nil, err
	}
	// Close flushes; the writer stays reusable afterwards through Reset.
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func inflate(compressed []byte) (string, error) {
	r, _ := inflaters.Get().(io.ReadCloser)
	defer inflaters.Put(r)

	resetter, ok := r.(flate.Resetter)
	if !ok {
		return "", errors.New("lyrics: flate reader is not resettable")
	}
	if err := resetter.Reset(bytes.NewReader(compressed), nil); err != nil {
		return "", err
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
