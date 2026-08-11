package postgres_test

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/pgtest"
)

// coverFixture is a user with one cover, which is the starting point for every
// test below.
func coverFixture(t *testing.T, playlistID string) (*postgres.Store, *domain.Cover) {
	t.Helper()
	ctx := t.Context()
	store, _ := pgtest.Reset(t)

	user := &domain.User{Email: "revisions@aurify.test", CreatedAt: time.Now().UTC()}
	if err := store.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	cover := &domain.Cover{
		UserID:     user.ID,
		Platform:   domain.PlatformSpotify,
		PlaylistID: playlistID,
		Status:     domain.CoverStatusReady,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("save cover: %v", err)
	}
	return store, cover
}

// finish records a terminal run, backdated so ordering between runs is explicit
// rather than a race on the clock.
func finish(t *testing.T, store *postgres.Store, coverID string, status domain.CoverStatus, ago time.Duration) *domain.CoverRevision {
	t.Helper()
	rev := &domain.CoverRevision{
		CoverID:     coverID,
		Status:      status,
		Prompt:      "prompt for the " + string(status) + " run",
		CompletedAt: time.Now().UTC().Add(-ago),
	}
	if err := store.Covers().SaveRevision(t.Context(), rev); err != nil {
		t.Fatalf("SaveRevision(%s): %v", status, err)
	}
	return rev
}

// The behaviour the backfill applies retroactively, and the one a naive "take
// the newest row" collapse gets wrong: a playlist whose most recent run failed
// still shows the artwork from the last one that worked.
func TestAFailedNewestRunKeepsTheLastSuccessfulArtwork(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	finish(t, store, cover.ID, domain.CoverStatusReady, 3*time.Hour)
	wanted := finish(t, store, cover.ID, domain.CoverStatusReady, 2*time.Hour)
	finish(t, store, cover.ID, domain.CoverStatusFailed, time.Hour)

	// The newest run's outcome is what the cover carries, and what the gallery
	// filter reads.
	cover.Status = domain.CoverStatusFailed
	cover.Error = "the image service refused the prompt"
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != domain.CoverStatusFailed {
		t.Errorf("status = %q, want the newest run's outcome", got.Status)
	}
	if want := "/api/v1/covers/" + wanted.ID + "/image"; got.ImageURL != want {
		t.Errorf("imageURL = %q, want the last successful run's %q", got.ImageURL, want)
	}
	// Failed runs are recorded but do not count as artwork.
	if got.RunCount != 2 {
		t.Errorf("runCount = %d, want the 2 successful runs", got.RunCount)
	}
}

// "Regenerating does not blank the tile" is half of what this change is for, and
// it is invisible to anything that does not go through a real read: the cover
// goes non-terminal while the previous run's artwork has to stay put.
func TestARegenerationInFlightKeepsTheOldArtwork(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	previous := finish(t, store, cover.ID, domain.CoverStatusReady, time.Hour)

	// A regeneration is a save of the same cover, back to a non-terminal status.
	cover.Status = domain.CoverStatusAnalyzing
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != domain.CoverStatusAnalyzing {
		t.Errorf("status = %q, want the run in flight", got.Status)
	}
	if want := "/api/v1/covers/" + previous.ID + "/image"; got.ImageURL != want {
		t.Errorf("imageURL = %q, want the previous run's %q — the tile blanked", got.ImageURL, want)
	}
}

// One run at a time per cover. A second generation is refused while the first is
// still moving, whichever non-terminal stage it is in.
func TestStartRunIsRefusedWhileOneIsInFlight(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	claim := func() error {
		return store.Covers().StartRun(ctx, &domain.Cover{
			UserID: cover.UserID, Platform: cover.Platform, PlaylistID: cover.PlaylistID,
			Status: domain.CoverStatusPending, CreatedAt: time.Now().UTC(),
		})
	}

	// The fixture is ready, so the first claim takes it.
	if err := claim(); err != nil {
		t.Fatalf("first claim on an idle cover: %v", err)
	}

	for _, stage := range []domain.CoverStatus{
		domain.CoverStatusPending, domain.CoverStatusAnalyzing, domain.CoverStatusGenerating,
	} {
		cover.Status = stage
		if err := store.Covers().Save(ctx, cover); err != nil {
			t.Fatalf("Save(%s): %v", stage, err)
		}
		if err := claim(); !errors.Is(err, domain.ErrGenerationInFlight) {
			t.Errorf("claim at %s = %v, want ErrGenerationInFlight", stage, err)
		}
	}

	// Reaching a terminal status releases it, so a regeneration can start.
	cover.Status = domain.CoverStatusReady
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("Save(ready): %v", err)
	}
	if err := claim(); err != nil {
		t.Fatalf("claim after the run finished: %v", err)
	}
}

// The guarantee that matters: the exclusion has to survive genuinely concurrent
// callers, which is what a read-then-write in Go would not. Every claim races
// the same playlist and exactly one may win.
//
// Both directions are covered. When no cover exists the losers collide on the
// unique index; when one does they collide on its row.
func TestOnlyOneConcurrentClaimWins(t *testing.T) {
	store, existing := coverFixture(t, "PL1")
	ctx := t.Context()

	for _, tc := range []struct{ name, playlistID string }{
		{"a cover that already exists", existing.PlaylistID},
		{"a playlist generated for the first time", "PL-brand-new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const claimants = 12
			var start sync.WaitGroup
			var done sync.WaitGroup
			var won atomic.Int64
			start.Add(1)

			for range claimants {
				done.Add(1)
				go func() {
					defer done.Done()
					// Released together, so the claims genuinely overlap.
					start.Wait()
					err := store.Covers().StartRun(ctx, &domain.Cover{
						UserID: existing.UserID, Platform: existing.Platform,
						PlaylistID: tc.playlistID,
						Status:     domain.CoverStatusPending, CreatedAt: time.Now().UTC(),
					})
					switch {
					case err == nil:
						won.Add(1)
					case errors.Is(err, domain.ErrGenerationInFlight):
					default:
						t.Errorf("StartRun: %v", err)
					}
				}()
			}
			start.Done()
			done.Wait()

			if n := won.Load(); n != 1 {
				t.Fatalf("%d of %d concurrent claims won, want exactly 1", n, claimants)
			}
		})
	}
}

// The root cause of duplicate tiles: there was no unique constraint, so a second
// generation for one playlist created a second cover. It must now update the
// first, and hand back the id that already existed.
func TestRegeneratingUpdatesTheSameCover(t *testing.T) {
	store, first := coverFixture(t, "PL1")
	ctx := t.Context()

	second := &domain.Cover{
		UserID:       first.UserID,
		Platform:     first.Platform,
		PlaylistID:   first.PlaylistID,
		PlaylistName: "Renamed since",
		Status:       domain.CoverStatusPending,
		CreatedAt:    time.Now().UTC(),
	}
	// Through the accept path, since that is where a regeneration begins.
	if err := store.Covers().StartRun(ctx, second); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second generation got id %q, want the existing cover %q", second.ID, first.ID)
	}

	covers, err := store.Covers().ListByUser(ctx, first.UserID, "", 20, domain.CoverCursor{})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(covers) != 1 {
		t.Fatalf("%d tiles for one playlist, want 1", len(covers))
	}
	if covers[0].PlaylistName != "Renamed since" {
		t.Errorf("playlist name = %q, want the newest run's", covers[0].PlaylistName)
	}
}

// The name lookup is best effort, so a run that could not fetch one must not
// erase the name an earlier run found — the tile would go untitled.
func TestARunWithoutANameKeepsThePreviousOne(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	cover.PlaylistName = "Chill Stratovarius"
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("Save: %v", err)
	}

	nameless := &domain.Cover{
		UserID: cover.UserID, Platform: cover.Platform, PlaylistID: cover.PlaylistID,
		Status: domain.CoverStatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := store.Covers().StartRun(ctx, nameless); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	got, err := store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.PlaylistName != "Chill Stratovarius" {
		t.Errorf("playlist name = %q, want it kept", got.PlaylistName)
	}
}

// Deleting the newest successful run falls back to the one before it, with no
// promotion step: the current artwork is only ever "the newest ready revision".
func TestDeletingTheNewestRunFallsBackToTheOneBefore(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	older := finish(t, store, cover.ID, domain.CoverStatusReady, 2*time.Hour)
	newest := finish(t, store, cover.ID, domain.CoverStatusReady, time.Hour)

	if err := store.Covers().DeleteRevision(ctx, cover.ID, newest.ID, cover.UserID); err != nil {
		t.Fatalf("DeleteRevision: %v", err)
	}

	got, err := store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if want := "/api/v1/covers/" + older.ID + "/image"; got.ImageURL != want {
		t.Errorf("imageURL = %q, want the run before it %q", got.ImageURL, want)
	}

	// And deleting the last one leaves the tile on the placeholder rather than
	// pointing at bytes that are gone.
	if err := store.Covers().DeleteRevision(ctx, cover.ID, older.ID, cover.UserID); err != nil {
		t.Fatalf("DeleteRevision: %v", err)
	}
	got, err = store.Covers().FindByID(ctx, cover.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ImageURL != "" {
		t.Errorf("imageURL = %q, want nothing left to point at", got.ImageURL)
	}
}

// numbersByAge returns each revision's number, newest first, which is the order
// ListRevisions hands them back in.
func numbersByAge(t *testing.T, store *postgres.Store, coverID string) []int {
	t.Helper()
	revs, err := store.Covers().ListRevisions(t.Context(), coverID, true)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	out := make([]int, 0, len(revs))
	for _, r := range revs {
		out = append(out, r.Number)
	}
	return out
}

// The number is what the client shows as "Revision #28", so it counts up in the
// order runs finished. Failed runs take one too: skipping them would mean a
// number depended on the outcome of the runs around it.
func TestRevisionNumbersCountUpIncludingFailedRuns(t *testing.T) {
	store, cover := coverFixture(t, "PL1")

	finish(t, store, cover.ID, domain.CoverStatusReady, 3*time.Hour)
	finish(t, store, cover.ID, domain.CoverStatusFailed, 2*time.Hour)
	finish(t, store, cover.ID, domain.CoverStatusReady, time.Hour)

	if got, want := numbersByAge(t, store, cover.ID), []int{3, 2, 1}; !slices.Equal(got, want) {
		t.Errorf("numbers newest first = %v, want %v", got, want)
	}
}

// The whole reason the number is stored rather than counted from the list:
// deleting a run must not renumber the ones around it, or a label on an open
// page and a ?rev= link would both start pointing at different artwork.
func TestDeletingARunNeverRenumbersTheOthers(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	finish(t, store, cover.ID, domain.CoverStatusReady, 3*time.Hour)
	middle := finish(t, store, cover.ID, domain.CoverStatusReady, 2*time.Hour)
	finish(t, store, cover.ID, domain.CoverStatusReady, time.Hour)

	if err := store.Covers().DeleteRevision(ctx, cover.ID, middle.ID, cover.UserID); err != nil {
		t.Fatalf("DeleteRevision: %v", err)
	}
	if got, want := numbersByAge(t, store, cover.ID), []int{3, 1}; !slices.Equal(got, want) {
		t.Errorf("numbers after deleting #2 = %v, want %v: a gap, not a renumbering", got, want)
	}

	// And the next run continues past the gap rather than reusing the number,
	// which would make #2 mean two different runs over the cover's life.
	finish(t, store, cover.ID, domain.CoverStatusReady, 0)
	if got, want := numbersByAge(t, store, cover.ID), []int{4, 3, 1}; !slices.Equal(got, want) {
		t.Errorf("numbers after a new run = %v, want %v", got, want)
	}
}

// Storing the image bytes is the last thing that can fail, and it happens after
// the revision row exists, so the failure path re-writes a run that was already
// recorded as ready. That is the same run correcting its outcome, and it has to
// keep the number the client is already showing.
func TestRewritingARunKeepsItsNumber(t *testing.T) {
	store, cover := coverFixture(t, "PL1")

	finish(t, store, cover.ID, domain.CoverStatusReady, 2*time.Hour)
	rev := finish(t, store, cover.ID, domain.CoverStatusReady, time.Hour)

	rev.Status = domain.CoverStatusFailed
	rev.Error = "storing the image failed"
	if err := store.Covers().SaveRevision(t.Context(), rev); err != nil {
		t.Fatalf("SaveRevision: %v", err)
	}

	if got, want := numbersByAge(t, store, cover.ID), []int{2, 1}; !slices.Equal(got, want) {
		t.Errorf("numbers after the rewrite = %v, want %v", got, want)
	}
}

// Neither id may be used to reach a run the caller does not own, and a miss and
// someone else's run have to be the same answer.
func TestDeletingAnotherUsersRunIsNotFound(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()
	rev := finish(t, store, cover.ID, domain.CoverStatusReady, time.Hour)

	if err := store.Covers().DeleteRevision(ctx, cover.ID, rev.ID, "someone-else"); err == nil {
		t.Fatal("another user's run was deleted")
	}
	if _, err := store.Covers().FindByID(ctx, cover.ID); err != nil {
		t.Fatalf("FindByID: %v", err)
	}
}

// Failed runs are recorded, because the prompt is the only thing that explains
// an image-provider refusal, but the history is otherwise about renders that
// exist.
func TestFailedRunsAreHiddenUnlessAskedFor(t *testing.T) {
	store, cover := coverFixture(t, "PL1")
	ctx := t.Context()

	finish(t, store, cover.ID, domain.CoverStatusReady, 2*time.Hour)
	finish(t, store, cover.ID, domain.CoverStatusFailed, time.Hour)

	shown, err := store.Covers().ListRevisions(ctx, cover.ID, false)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(shown) != 1 || shown[0].Status != domain.CoverStatusReady {
		t.Fatalf("default history = %+v, want the successful run alone", shown)
	}
	if shown[0].ImageURL == "" {
		t.Error("a successful run carries no image URL, so its artwork is unreachable")
	}

	all, err := store.Covers().ListRevisions(ctx, cover.ID, true)
	if err != nil {
		t.Fatalf("ListRevisions(includeFailed): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("full history has %d runs, want 2", len(all))
	}
	// Newest first, and the failed one keeps its prompt.
	if all[0].Status != domain.CoverStatusFailed || all[0].Prompt == "" {
		t.Errorf("newest run = %+v, want the failed one with its prompt kept", all[0])
	}
	if all[0].ImageURL != "" {
		t.Error("a failed run points at image bytes that were never stored")
	}
}

// Keyset paging is what survives a regeneration reordering the list mid-scroll.
// Walking the pages must visit each cover exactly once.
func TestKeysetPagingWalksEveryCoverOnce(t *testing.T) {
	store, first := coverFixture(t, "PL0")
	ctx := t.Context()

	for i := 1; i < 5; i++ {
		cover := &domain.Cover{
			UserID: first.UserID, Platform: first.Platform,
			PlaylistID: "PL" + string(rune('0'+i)),
			Status:     domain.CoverStatusReady, CreatedAt: time.Now().UTC(),
		}
		if err := store.Covers().Save(ctx, cover); err != nil {
			t.Fatalf("save PL%d: %v", i, err)
		}
	}

	seen := map[string]int{}
	cursor := domain.CoverCursor{}
	for pages := 0; pages < 10; pages++ {
		page, err := store.Covers().ListByUser(ctx, first.UserID, "", 2, cursor)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, c := range page {
			seen[c.ID]++
		}
		last := page[len(page)-1]
		cursor = domain.CoverCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}
	}

	if len(seen) != 5 {
		t.Fatalf("walked %d covers, want 5", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("cover %s appeared %d times across pages", id, n)
		}
	}
}
