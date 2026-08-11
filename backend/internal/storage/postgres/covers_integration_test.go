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

	// A playlist each: covers are unique on (user, platform, playlist) now, so
	// three saves against one playlist would be three writes to one row.
	save := func(playlistID string, status domain.CoverStatus) *domain.Cover {
		cover := &domain.Cover{
			UserID: user.ID, Platform: domain.PlatformSpotify,
			PlaylistID: playlistID, Status: status,
		}
		if err := store.Covers().Save(ctx, cover); err != nil {
			t.Fatalf("Save(%s): %v", status, err)
		}
		return cover
	}

	stuck := save("PL1", domain.CoverStatusAnalyzing)
	live := save("PL2", domain.CoverStatusGenerating)
	done := save("PL3", domain.CoverStatusReady)

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

// A heartbeat is what keeps the sweep off a run that is merely slow. It must
// move the clock and nothing else: a beat that also wrote status would race the
// pipeline it runs beside and could reinstate a stage the run had left.
func TestTouchMarksARunAliveWithoutChangingIt(t *testing.T) {
	store, db := pgtest.Reset(t)
	ctx := t.Context()

	user := &domain.User{Email: "touch@aurify.test", CreatedAt: time.Now().UTC()}
	if err := store.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	cover := &domain.Cover{
		UserID: user.ID, Platform: domain.PlatformSpotify,
		PlaylistID: "PL1", Status: domain.CoverStatusAnalyzing,
	}
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := db.Exec(ctx,
		"UPDATE covers SET updated_at = now() - interval '1 hour' WHERE id = $1", cover.ID,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	before, err := store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if err := store.Covers().Touch(ctx, cover.ID); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	after, err := store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("updatedAt %v did not move past %v, so the sweep would still reclaim it",
			after.UpdatedAt, before.UpdatedAt)
	}
	if after.Status != domain.CoverStatusAnalyzing {
		t.Errorf("status = %q, want the run's own stage untouched", after.Status)
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Error("a heartbeat moved createdAt")
	}

	// And it is enough to keep the sweep off it.
	n, err := store.Covers().FailStuck(ctx, time.Now().UTC().Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("FailStuck: %v", err)
	}
	if n != 0 {
		t.Errorf("the sweep failed %d beating runs, want 0", n)
	}
}

// What a restart does: the service runs one instance, so nothing that was
// generating before it started is generating after, however recent its
// timestamp. A cutoff of "now" is how the reap says that.
func TestReapingAtStartupFailsEveryRunInFlight(t *testing.T) {
	store, _ := pgtest.Reset(t)
	ctx := t.Context()

	user := &domain.User{Email: "reap@aurify.test", CreatedAt: time.Now().UTC()}
	if err := store.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	stages := []domain.CoverStatus{
		domain.CoverStatusPending, domain.CoverStatusAnalyzing,
		domain.CoverStatusGenerating, domain.CoverStatusReady,
	}
	ids := make(map[domain.CoverStatus]string, len(stages))
	for i, status := range stages {
		cover := &domain.Cover{
			UserID: user.ID, Platform: domain.PlatformSpotify,
			PlaylistID: "PL" + string(rune('1'+i)), Status: status,
		}
		if err := store.Covers().Save(ctx, cover); err != nil {
			t.Fatalf("Save(%s): %v", status, err)
		}
		ids[status] = cover.ID
	}

	// No backdating: these were all written milliseconds ago, which is exactly
	// the case the old ten-minute threshold could not answer.
	n, err := store.Covers().FailStuck(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("FailStuck: %v", err)
	}
	if n != 3 {
		t.Fatalf("reaped %d covers, want the 3 non-terminal ones", n)
	}

	for _, status := range stages {
		got, err := store.Covers().FindByID(ctx, ids[status])
		if err != nil {
			t.Fatalf("FindByID(%s): %v", status, err)
		}
		want := domain.CoverStatusFailed
		if status == domain.CoverStatusReady {
			want = domain.CoverStatusReady // a finished run is not interrupted
		}
		if got.Status != want {
			t.Errorf("a %s cover became %q, want %q", status, got.Status, want)
		}
	}

	// And the reap releases the claim, so those playlists can be generated again.
	if err := store.Covers().StartRun(ctx, &domain.Cover{
		UserID: user.ID, Platform: domain.PlatformSpotify, PlaylistID: "PL2",
		Status: domain.CoverStatusPending, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("StartRun after the reap: %v", err)
	}
}
