package postgres_test

import (
	"math"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/pgtest"
)

func TestTrackFeaturesRoundTrip(t *testing.T) {
	store, _ := pgtest.Reset(t)
	ctx := t.Context()

	entries := []domain.CachedFeatures{{
		Key: "blake shelton\ngod's country",
		Features: domain.AudioFeatures{
			Danceability: 0.71, Acousticness: 0.03, Energy: 0.62, Valence: 0.44,
			// Deliberately outside [0,1] and outside every other column's range:
			// the save is a positional unnest() and the read is a positional
			// scan, so a column added in the wrong slot has to be visible here
			// rather than passing as a plausible-looking probability.
			OnsetRate: 5.28,
			Present:   true,
		},
		FetchedAt: time.Now().UTC(),
	}, {
		// A track that could not be matched. Storing the absence is the point of
		// the table: without it, every non-song in the library re-pays a second
		// of MusicBrainz rate limit on every generation.
		Key:       "superfastmatt\ndiy brake lines the easy way",
		Features:  domain.AudioFeatures{Present: false},
		FetchedAt: time.Now().UTC(),
	}}

	if err := store.TrackFeatures().SaveMany(ctx, entries); err != nil {
		t.Fatalf("SaveMany: %v", err)
	}

	got, err := store.TrackFeatures().FindMany(ctx, []string{entries[0].Key, entries[1].Key, "absent\nkey"})
	if err != nil {
		t.Fatalf("FindMany: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d entries, want 2 (a key with no row must be absent, not an error)", len(got))
	}

	hit := got[entries[0].Key].Features
	if !hit.Present || math.Abs(hit.Danceability-0.71) > 1e-9 || math.Abs(hit.Energy-0.62) > 1e-9 {
		t.Errorf("features round-tripped wrong: %+v", hit)
	}
	if math.Abs(hit.OnsetRate-5.28) > 1e-9 {
		t.Errorf("onset rate = %v, want 5.28", hit.OnsetRate)
	}
	if math.Abs(got[entries[0].Key].FetchedAt.Sub(entries[0].FetchedAt).Seconds()) > 1 {
		t.Errorf("fetched_at = %v, want %v: the new column displaced a later one",
			got[entries[0].Key].FetchedAt, entries[0].FetchedAt)
	}
	if miss := got[entries[1].Key].Features; miss.Present {
		t.Errorf("the negative entry came back Present: %+v", miss)
	}
}

// Rows predating the current extractor have to come back marked as such, which
// is what migration 00010 exists for. Written as raw SQL because SaveMany
// deliberately cannot produce one: it stamps the current version.
func TestTrackFeaturesCarryTheirVersion(t *testing.T) {
	store, conn := pgtest.Reset(t)
	ctx := t.Context()
	key := "an older build\na cached song"

	_, err := conn.Exec(ctx, `
		INSERT INTO track_features (track_key, present, energy, features_version, fetched_at)
		VALUES ($1, true, 0.5, 0, now())`, key)
	if err != nil {
		t.Fatalf("seed a stale row: %v", err)
	}

	got, err := store.TrackFeatures().FindMany(ctx, []string{key})
	if err != nil {
		t.Fatalf("FindMany: %v", err)
	}
	entry := got[key]
	if entry.Version != 0 {
		t.Errorf("Version = %d, want the stored 0", entry.Version)
	}
	if entry.Fresh(time.Now().UTC()) {
		t.Error("a row from an older extractor is not fresh, however Present it looks")
	}

	// And the repository stamps the current version on the way back out, so the
	// re-fetch that follows is not stale again immediately.
	if err := store.TrackFeatures().SaveMany(ctx, []domain.CachedFeatures{{
		Key:       key,
		Features:  domain.AudioFeatures{Energy: 0.5, OnsetRate: 3, Present: true},
		FetchedAt: time.Now().UTC(),
		// Claiming an absurd version to prove the caller does not get a say.
		Version: 99,
	}}); err != nil {
		t.Fatalf("SaveMany: %v", err)
	}

	got, err = store.TrackFeatures().FindMany(ctx, []string{key})
	if err != nil {
		t.Fatalf("FindMany: %v", err)
	}
	if v := got[key].Version; v != domain.FeaturesVersion {
		t.Errorf("Version = %d, want the stamped %d", v, domain.FeaturesVersion)
	}
	if !got[key].Fresh(time.Now().UTC()) {
		t.Error("a row this build just wrote must be fresh")
	}
}

// A negative entry has to be able to become positive once MusicBrainz gains the
// release, so the upsert overwrites rather than skipping.
func TestTrackFeaturesUpsertOverwrites(t *testing.T) {
	store, _ := pgtest.Reset(t)
	ctx := t.Context()
	key := "a band\na song"

	for _, f := range []domain.AudioFeatures{
		{Present: false},
		{Energy: 0.8, Present: true},
	} {
		if err := store.TrackFeatures().SaveMany(ctx, []domain.CachedFeatures{
			{Key: key, Features: f, FetchedAt: time.Now().UTC()},
		}); err != nil {
			t.Fatalf("SaveMany: %v", err)
		}
	}

	got, err := store.TrackFeatures().FindMany(ctx, []string{key})
	if err != nil {
		t.Fatalf("FindMany: %v", err)
	}
	if f := got[key].Features; !f.Present || math.Abs(f.Energy-0.8) > 1e-9 {
		t.Errorf("features = %+v, want the second write to have replaced the first", f)
	}
}

// PostgreSQL refuses an ON CONFLICT DO UPDATE that touches the same row twice in
// one statement. A playlist holding the same song twice is ordinary, and this
// exact failure lost a whole batch of lyric writes once already.
func TestTrackFeaturesToleratesDuplicateKeysInOneBatch(t *testing.T) {
	store, _ := pgtest.Reset(t)
	ctx := t.Context()
	key := "a band\na song"

	err := store.TrackFeatures().SaveMany(ctx, []domain.CachedFeatures{
		{Key: key, Features: domain.AudioFeatures{Energy: 0.5, Present: true}, FetchedAt: time.Now().UTC()},
		{Key: key, Features: domain.AudioFeatures{Energy: 0.5, Present: true}, FetchedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("a batch with a duplicate key failed: %v", err)
	}

	got, err := store.TrackFeatures().FindMany(ctx, []string{key})
	if err != nil {
		t.Fatalf("FindMany: %v", err)
	}
	if !got[key].Features.Present {
		t.Error("the entry was not written")
	}
}
