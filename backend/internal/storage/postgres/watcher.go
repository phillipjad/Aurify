package postgres

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CoverWatcher fans cover-update notifications out to in-process subscribers,
// one LISTEN connection per instance (see docs/adr/0019-async-cover-generation.md).
type CoverWatcher struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

// WatchCovers listens for cover updates until ctx is cancelled, reconnecting
// on drop. A notification lost in that gap is recovered by the stream heartbeat.
func (s *Store) WatchCovers(ctx context.Context) *CoverWatcher {
	w := &CoverWatcher{subs: make(map[string]map[chan struct{}]struct{})}
	go w.listen(ctx, s)
	return w
}

// Subscribe signals on every change to one cover. The returned func must be called.
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

// listenOnce holds a pool connection for the lifetime of the LISTEN, since
// notifications arrive on a session rather than a pool.
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
