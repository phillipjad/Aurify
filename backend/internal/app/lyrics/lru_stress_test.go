package lyrics

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// checkInvariants asserts what must be true of the cache at rest, whatever it
// has been through: the budget is respected, the map and the list describe the
// same set, and the running total matches the entries actually held.
//
// The last one is the one that rots quietly. An overwrite or eviction that
// forgets to adjust the total leaves the cache holding progressively less than
// it should, and nothing else would ever report it.
func checkInvariants(t *testing.T, c *memoryCache) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.used > c.budget {
		t.Fatalf("used %d bytes against a budget of %d", c.used, c.budget)
	}
	if len(c.entries) != c.order.Len() {
		t.Fatalf("map holds %d entries, list holds %d", len(c.entries), c.order.Len())
	}

	sum := 0
	for element := c.order.Front(); element != nil; element = element.Next() {
		entry := element.Value.(*memoryEntry)
		sum += entry.size
		if got, ok := c.entries[entry.key]; !ok {
			t.Fatalf("entry %q is in the list but not the map", entry.key)
		} else if got != element {
			t.Fatalf("entry %q maps to a different list element", entry.key)
		}
	}
	if sum != c.used {
		t.Fatalf("accounting drift: entries total %d bytes, used says %d", sum, c.used)
	}
}

func stressEntry(key string, size int) domain.CachedLyrics {
	return domain.CachedLyrics{Key: key, Lyrics: incompressible(size), Found: true, FetchedAt: time.Now()}
}

// The property that distinguishes least-recently-used from evicting whatever is
// oldest or whatever is convenient: a small working set that keeps being read
// must survive a flood of one-shot entries that individually dwarf the budget.
//
// A FIFO cache fails this, because the hot keys were inserted first and are
// evicted first regardless of how often they are read.
func TestHotEntriesSurviveAFloodOfColdOnes(t *testing.T) {
	// Roughly fifty entries fit, so 2,000 cold ones is forty times the capacity.
	c := newMemoryCache(64 << 10)

	const hotCount = 20
	hot := make([]string, hotCount)
	for i := range hot {
		hot[i] = fmt.Sprintf("hot-%02d", i)
		c.put(stressEntry(hot[i], 900))
	}

	r := rand.New(rand.NewSource(7))
	for i := range 2000 {
		// Keep the working set warm, then admit a cold entry that has to displace
		// something.
		for _, key := range hot {
			c.get(key)
		}
		c.put(stressEntry(fmt.Sprintf("cold-%04d", i), 900))
		_ = r
	}

	survivors := 0
	for _, key := range hot {
		if _, ok := c.get(key); ok {
			survivors++
		}
	}
	if survivors < hotCount {
		t.Fatalf("%d of %d repeatedly-read entries survived: eviction is not least-recently-used",
			survivors, hotCount)
	}
	checkInvariants(t, c)
}

// Eviction order is exact, not approximate: the least recently *used* entry goes
// first, which is not the same as the least recently added.
func TestEvictsInStrictUseOrder(t *testing.T) {
	c := newMemoryCache(4 << 10)

	// Four entries of about 900 compressed bytes each fill the budget.
	for _, key := range []string{"a", "b", "c", "d"} {
		c.put(stressEntry(key, 900))
	}
	checkInvariants(t, c)

	// Touch them so use order becomes b, a, d, c with c most recent.
	for _, key := range []string{"b", "a", "d", "c"} {
		if _, ok := c.get(key); !ok {
			t.Fatalf("setup: %q should be resident", key)
		}
	}

	// One more admission must evict exactly "b", the least recently used.
	c.put(stressEntry("e", 900))

	if _, ok := c.get("b"); ok {
		t.Error("the least recently used entry survived")
	}
	for _, key := range []string{"a", "c", "d", "e"} {
		if _, ok := c.get(key); !ok {
			t.Errorf("%q was evicted although it was used more recently than b", key)
		}
	}
	checkInvariants(t, c)
}

// Concurrency is the point: four fetches run at once, and several generations can
// overlap. Nothing here should race, deadlock, or drift.
func TestConcurrentUseKeepsTheCacheConsistent(t *testing.T) {
	c := newMemoryCache(128 << 10)

	const (
		workers   = 16
		perWorker = 400
		hotKeys   = 30
	)

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed)))
			for i := range perWorker {
				switch {
				case r.Intn(100) < 70:
					// Mostly reads of a shared working set, which is what a library
					// with overlapping playlists looks like.
					c.get(fmt.Sprintf("hot-%02d", r.Intn(hotKeys)))
				case r.Intn(100) < 90:
					c.put(stressEntry(fmt.Sprintf("hot-%02d", r.Intn(hotKeys)), 500+r.Intn(1500)))
				default:
					c.put(stressEntry(fmt.Sprintf("cold-%d-%d", seed, i), 500+r.Intn(3000)))
				}
			}
		}(w)
	}
	wg.Wait()

	checkInvariants(t, c)
	if len(c.entries) == 0 {
		t.Fatal("the cache emptied itself under concurrent load")
	}
}

// Repeatedly overwriting one key, and repeatedly filling and overflowing the
// cache, are the two paths where a size adjustment is easiest to get wrong.
func TestAccountingHoldsAcrossChurn(t *testing.T) {
	c := newMemoryCache(16 << 10)
	r := rand.New(rand.NewSource(3))

	for i := range 5000 {
		switch i % 3 {
		case 0:
			c.put(stressEntry("churn", 200+r.Intn(2000)))
		case 1:
			c.put(stressEntry(fmt.Sprintf("k-%d", i%50), 200+r.Intn(2000)))
		default:
			c.get(fmt.Sprintf("k-%d", r.Intn(50)))
		}
	}

	checkInvariants(t, c)
}

func BenchmarkMemoryCacheGet(b *testing.B) {
	c := newMemoryCache(DefaultMemoryBudget)
	for i := range 500 {
		c.put(stressEntry(fmt.Sprintf("k-%d", i), 1100))
	}
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		c.get(fmt.Sprintf("k-%d", i%500))
	}
}

func BenchmarkMemoryCachePut(b *testing.B) {
	c := newMemoryCache(DefaultMemoryBudget)
	body := incompressible(1100)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		c.put(domain.CachedLyrics{
			Key: fmt.Sprintf("k-%d", i), Lyrics: body, Found: true, FetchedAt: time.Now(),
		})
	}
}
