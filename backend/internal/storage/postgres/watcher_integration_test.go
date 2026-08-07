package postgres_test

import (
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/pgtest"
)

// The push path end to end: a cover write fires the database trigger, the
// watcher's LISTEN picks it up, and the subscriber for that cover — and only
// that cover — is signalled.
func TestCoverWatcherSignalsSubscribersOnWrites(t *testing.T) {
	store, _ := pgtest.Reset(t)
	ctx := t.Context()

	user := &domain.User{Email: "watcher@aurify.test", CreatedAt: time.Now().UTC()}
	if err := store.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	cover := &domain.Cover{UserID: user.ID, Platform: domain.PlatformSpotify, PlaylistID: "PL1", Status: domain.CoverStatusPending}
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("save cover: %v", err)
	}

	watcher := store.WatchCovers(ctx)
	signals, cancel := watcher.Subscribe(cover.ID)
	defer cancel()

	// The LISTEN runs on a goroutine, so the first writes may race it. Keep
	// writing until a signal lands; each write is a real UPDATE, so any one of
	// them arriving proves trigger -> NOTIFY -> LISTEN -> subscriber.
	deadline := time.After(5 * time.Second)
	for signalled := false; !signalled; {
		cover.Status = domain.CoverStatusAnalyzing
		if err := store.Covers().Save(ctx, cover); err != nil {
			t.Fatalf("update cover: %v", err)
		}
		select {
		case <-signals:
			signalled = true
		case <-deadline:
			t.Fatal("no signal within 5s of writing the subscribed cover")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// A different cover must not signal this subscription.
	other := &domain.Cover{UserID: user.ID, Platform: domain.PlatformSpotify, PlaylistID: "PL2", Status: domain.CoverStatusPending}
	if err := store.Covers().Save(ctx, other); err != nil {
		t.Fatalf("save other cover: %v", err)
	}
	// Drain anything left over from the update loop first.
	for {
		select {
		case <-signals:
			continue
		case <-time.After(300 * time.Millisecond):
		}
		break
	}
	select {
	case <-signals:
		t.Fatal("a write to an unrelated cover signalled this subscription")
	case <-time.After(300 * time.Millisecond):
	}
}
