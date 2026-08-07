package postgres_test

import (
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/pgtest"
)

// The sweep exists for covers orphaned by a crash or scale-down (ADR 0019):
// non-terminal and untouched past the cutoff means no worker is attached. It
// must fail exactly those — not live jobs, not terminal covers.
func TestFailStuckFailsOnlyAbandonedCovers(t *testing.T) {
	store, db := pgtest.Reset(t)
	ctx := t.Context()

	user := &domain.User{Email: "covers@aurify.test", CreatedAt: time.Now().UTC()}
	if err := store.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	save := func(status domain.CoverStatus) *domain.Cover {
		cover := &domain.Cover{
			UserID: user.ID, Platform: domain.PlatformSpotify,
			PlaylistID: "PL1", Status: status,
		}
		if err := store.Covers().Save(ctx, cover); err != nil {
			t.Fatalf("Save(%s): %v", status, err)
		}
		return cover
	}

	stuck := save(domain.CoverStatusAnalyzing)
	live := save(domain.CoverStatusGenerating)
	done := save(domain.CoverStatusReady)

	// Save stamps updated_at with now, so the abandoned cover is backdated to
	// before the cutoff; ready is backdated too, proving terminal rows are
	// exempt no matter how old.
	for _, id := range []string{stuck.ID, done.ID} {
		if _, err := db.Exec(ctx,
			"UPDATE covers SET updated_at = now() - interval '1 hour' WHERE id = $1", id,
		); err != nil {
			t.Fatalf("backdate %s: %v", id, err)
		}
	}

	n, err := store.Covers().FailStuck(ctx, time.Now().UTC().Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("FailStuck: %v", err)
	}
	if n != 1 {
		t.Fatalf("FailStuck failed %d covers, want exactly the abandoned one", n)
	}

	assertStatus := func(id string, want domain.CoverStatus) {
		t.Helper()
		got, err := store.Covers().FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID(%s): %v", id, err)
		}
		if got.Status != want {
			t.Errorf("cover %s status = %q, want %q", id, got.Status, want)
		}
	}
	assertStatus(stuck.ID, domain.CoverStatusFailed)
	assertStatus(live.ID, domain.CoverStatusGenerating)
	assertStatus(done.ID, domain.CoverStatusReady)

	failed, err := store.Covers().FindByID(ctx, stuck.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if failed.Error == "" {
		t.Error("a swept cover carries no error, so the gallery cannot explain the failure")
	}
}
