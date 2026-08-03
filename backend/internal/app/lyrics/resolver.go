// Package lyrics resolves lyric text for a set of tracks, through a memory
// cache, a durable cache, and finally the provider.
//
// It exists because ports.LyricsClient is deliberately single-track, so anything
// wrapped around it inherits one lookup per track. Resolving a playlist at a
// time is what turns hundreds of round trips into two, and it is also the only
// place that knows enough to fetch several misses at once.
package lyrics

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// fetchConcurrency bounds simultaneous provider requests.
//
// Enough to turn a sequential half-minute into single digits, restrained enough
// to be a reasonable neighbour: lrclib is a free community service with no
// documented rate limit, so the restraint has to come from this side. With a
// warm cache most generations never reach the network at all.
const fetchConcurrency = 4

// Resolver turns tracks into lyric text, remembering what it learns.
type Resolver struct {
	memory *memoryCache
	store  ports.LyricsRepository
	client ports.LyricsClient
	now    func() time.Time
}

// NewResolver constructs a Resolver with the default memory budget.
func NewResolver(store ports.LyricsRepository, client ports.LyricsClient) *Resolver {
	return &Resolver{
		memory: newMemoryCache(DefaultMemoryBudget),
		store:  store,
		client: client,
		now:    time.Now,
	}
}

// Resolve returns lyric text for each track, index-aligned with the input.
//
// It never returns an error, and that is deliberate rather than lazy: a lyric is
// a best-effort signal, the pipeline already treats absence as neutral, and
// there is no outcome here worth failing a generation the user asked for.
// Failures are logged and become empty strings.
func (r *Resolver) Resolve(ctx context.Context, tracks []domain.Track) []string {
	out := make([]string, len(tracks))
	if len(tracks) == 0 {
		return out
	}

	keys := make([]string, len(tracks))
	for i, track := range tracks {
		keys[i] = domain.LyricsKey(track)
	}
	now := r.now()

	// 1. Memory. Free, and the only tier that can answer while the durable one
	//    is unreachable.
	missing := make([]int, 0, len(tracks))
	for i, key := range keys {
		if entry, ok := r.memory.get(key); ok && entry.Fresh(now) {
			out[i] = entry.Lyrics
			continue
		}
		missing = append(missing, i)
	}

	// 2. The durable cache, in one query for everything memory did not answer.
	missing = r.fillFromStore(ctx, keys, missing, out, now)

	// 3. The provider, for genuine misses only.
	r.fetchMissing(ctx, tracks, keys, missing, out, now)

	return out
}

// fillFromStore resolves what it can from the repository and returns the indexes
// still unanswered.
func (r *Resolver) fillFromStore(
	ctx context.Context,
	keys []string,
	missing []int,
	out []string,
	now time.Time,
) []int {
	if len(missing) == 0 {
		return missing
	}

	wanted := make([]string, 0, len(missing))
	for _, i := range missing {
		wanted = append(wanted, keys[i])
	}

	found, err := r.store.FindMany(ctx, wanted)
	if err != nil {
		// A cache that cannot be read is a slow day, not a failed generation.
		slog.WarnContext(ctx, "lyrics cache read failed, falling through to the provider", "error", err)
		return missing
	}

	stillMissing := make([]int, 0, len(missing))
	for _, i := range missing {
		entry, ok := found[keys[i]]
		if !ok || !entry.Fresh(now) {
			stillMissing = append(stillMissing, i)
			continue
		}
		out[i] = entry.Lyrics
		r.memory.put(entry)
	}
	return stillMissing
}

// fetchMissing asks the provider for what neither cache held, with bounded
// concurrency, and writes what it learns back to both tiers.
func (r *Resolver) fetchMissing(
	ctx context.Context,
	tracks []domain.Track,
	keys []string,
	missing []int,
	out []string,
	now time.Time,
) {
	if len(missing) == 0 {
		return
	}

	// Group the outstanding indexes by key first. A playlist can hold the same
	// song twice, and distinct tracks can normalize to one key, so without this a
	// duplicate would be fetched once per occurrence, and the batched upsert would
	// be handed the same key twice, which PostgreSQL rejects outright with
	// "ON CONFLICT DO UPDATE command cannot affect row a second time" — losing the
	// whole batch, not just the duplicate.
	order := make([]string, 0, len(missing))
	byKey := make(map[string][]int, len(missing))
	for _, index := range missing {
		key := keys[index]
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], index)
	}

	// Pre-sized and written by slot. Appending from these goroutines would
	// scramble the alignment with tracks that the analysis step depends on, and
	// nothing downstream would notice.
	fetched := make([]string, len(order))
	failed := make([]bool, len(order))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(fetchConcurrency)
	for slot, key := range order {
		slot, track := slot, tracks[byKey[key][0]]
		group.Go(func() error {
			text, err := r.client.Fetch(groupCtx, track)
			if err != nil {
				// Marked rather than returned: one unlucky track must not cancel
				// the rest, and a failure is not a cacheable answer.
				failed[slot] = true
				return nil
			}
			fetched[slot] = text
			return nil
		})
	}
	// No error is ever returned above, so this only surfaces a cancelled context.
	if err := group.Wait(); err != nil {
		slog.WarnContext(ctx, "lyrics lookup interrupted", "error", err)
	}

	entries := make([]domain.CachedLyrics, 0, len(order))
	for slot, key := range order {
		if failed[slot] {
			// Deliberately not cached. Recording a failure as "no lyrics exist"
			// would let one bad spell at the provider mark a whole library as
			// lyric-less for thirty days.
			continue
		}
		for _, index := range byKey[key] {
			out[index] = fetched[slot]
		}
		entry := domain.CachedLyrics{
			Key:       key,
			Lyrics:    fetched[slot],
			Found:     fetched[slot] != "",
			FetchedAt: now,
		}
		entries = append(entries, entry)
		r.memory.put(entry)
	}

	if len(entries) == 0 {
		return
	}
	if err := r.store.SaveMany(ctx, entries); err != nil {
		slog.WarnContext(ctx, "lyrics cache write failed", "error", err, "entries", len(entries))
	}
}
