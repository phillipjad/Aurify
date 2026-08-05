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
			Danceability: 0.71, Acousticness: 0.03, Energy: 0.62, Valence: 0.44, Present: true,
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
	if miss := got[entries[1].Key].Features; miss.Present {
		t.Errorf("the negative entry came back Present: %+v", miss)
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
