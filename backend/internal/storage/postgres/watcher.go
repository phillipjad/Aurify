package postgres

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CoverWatcher fans PostgreSQL cover-update notifications out to in-process
// subscribers. One instance holds one LISTEN connection; a second API instance
// runs its own watcher against the same channel, so nothing here needs to be
// shared across instances (see docs/adr/0019-async-cover-generation.md).
//
// Signals are coalescing and carry no payload: a subscriber is told "your cover
// changed" and re-reads the row, so a missed or merged notification can never
// deliver stale state.
type CoverWatcher struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

// WatchCovers starts listening for cover updates until ctx is cancelled.
//
// The listener runs on a dedicated connection acquired from the pool; if it
// drops, it reconnects with backoff. Subscribers are not told about the gap,
// which is safe because every SSE stream also re-reads its cover on a heartbeat
// tick, so a lost notification delays an update rather than losing it.
func (s *Store) WatchCovers(ctx context.Context) *CoverWatcher {
	w := &CoverWatcher{subs: make(map[string]map[chan struct{}]struct{})}
	go w.listen(ctx, s)
	return w
}

// Subscribe registers interest in one cover. The returned channel receives a
// coalesced signal whenever the cover's row changes; the returned func cancels
// the subscription and must be called.
func (w *CoverWatcher) Subscribe(coverID string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	if w.subs[coverID] == nil {
		w.subs[coverID] = make(map[chan struct{}]struct{})
	}
	w.subs[coverID][ch] = struct{}{}
	w.mu.Unlock()

	return ch, func() {
		w.mu.Lock()
		delete(w.subs[coverID], ch)
		if len(w.subs[coverID]) == 0 {
			delete(w.subs, coverID)
		}
		w.mu.Unlock()
	}
}

func (w *CoverWatcher) notify(coverID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.subs[coverID] {
		select {
		case ch <- struct{}{}:
		default: // already signalled; the subscriber re-reads the row anyway
		}
	}
}

func (w *CoverWatcher) listen(ctx context.Context, s *Store) {
	for ctx.Err() == nil {
		if err := w.listenOnce(ctx, s); err != nil && ctx.Err() == nil {
			slog.Warn("cover listener lost its connection, reconnecting", "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}
}

// listenOnce holds one connection out of the pool for the lifetime of the
// LISTEN, which is the price of push: notifications arrive on a session, not a
// pool.
func (w *CoverWatcher) listenOnce(ctx context.Context, s *Store) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN cover_updates"); err != nil {
		return err
	}
	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		w.notify(notification.Payload)
	}
}
