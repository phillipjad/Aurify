package postgres_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/pgtest"
)

// newStore resets the schema and returns a store, a cover, and one finished run
// of it. Images hang off the revision, not the cover: cover_images is keyed by
// revision_id so each run keeps its own bytes.
func newStore(t *testing.T) (store *postgres.Store, coverID, revisionID string) {
	t.Helper()
	ctx := t.Context()
	store, _ = pgtest.Reset(t)

	user := &domain.User{Email: "cover-images@aurify.test", CreatedAt: time.Now().UTC()}
	if err := store.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	cover := &domain.Cover{
		UserID:     user.ID,
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
		Status:     domain.CoverStatusGenerating,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.Covers().Save(ctx, cover); err != nil {
		t.Fatalf("save cover: %v", err)
	}
	rev := &domain.CoverRevision{CoverID: cover.ID, Status: domain.CoverStatusReady}
	if err := store.Covers().SaveRevision(ctx, rev); err != nil {
		t.Fatalf("save revision: %v", err)
	}
	return store, cover.ID, rev.ID
}

// BYTEA is the one column type in this schema where a driver-level encoding
// mistake produces plausible-looking but corrupt data, so the bytes are compared
// exactly rather than by length.
func TestCoverImageRoundTrip(t *testing.T) {
	store, _, revisionID := newStore(t)
	ctx := t.Context()

	// Bytes chosen to break on anything that treats them as text: a NUL, a
	// backslash, invalid UTF-8, and a high byte.
	original := domain.GeneratedImage{
		Bytes:       []byte{0xFF, 0xD8, 0x00, '\\', 0xC3, 0x28, 0x7F, 0xFE},
		ContentType: "image/jpeg",
	}

	if err := store.CoverImages().Put(ctx, revisionID, original); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.CoverImages().Find(ctx, revisionID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if !bytes.Equal(got.Bytes, original.Bytes) {
		t.Errorf("bytes = %x, want %x", got.Bytes, original.Bytes)
	}
	if got.ContentType != original.ContentType {
		t.Errorf("content type = %q, want %q", got.ContentType, original.ContentType)
	}
}

// A retried store writes to the same revision id, so a plain INSERT would fail
// on the primary key and lose the new image.
func TestPutReplacesAnExistingImage(t *testing.T) {
	store, _, revisionID := newStore(t)
	ctx := t.Context()

	first := domain.GeneratedImage{Bytes: []byte{0x01, 0x02}, ContentType: "image/png"}
	if err := store.CoverImages().Put(ctx, revisionID, first); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second := domain.GeneratedImage{Bytes: []byte{0x03, 0x04, 0x05}, ContentType: "image/jpeg"}
	if err := store.CoverImages().Put(ctx, revisionID, second); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	got, err := store.CoverImages().Find(ctx, revisionID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if !bytes.Equal(got.Bytes, second.Bytes) || got.ContentType != second.ContentType {
		t.Errorf("got %x/%s, want the second write %x/%s",
			got.Bytes, got.ContentType, second.Bytes, second.ContentType)
	}
}

// The route turns this into a 404. Any other error would become a 500 and read
// as a broken server rather than a cover that has no image yet.
func TestFindingAMissingImageIsErrNotFound(t *testing.T) {
	store, _, _ := newStore(t)

	_, err := store.CoverImages().Find(t.Context(), "0f8fad5b-d9cb-469f-a165-70867728950e")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

// Deleting a cover must take every run's image with it, through two cascades:
// covers -> cover_revisions -> cover_images. Otherwise the bytes outlive the row
// that referenced them and nothing will ever collect them.
func TestDeletingACoverRemovesItsImage(t *testing.T) {
	store, coverID, revisionID := newStore(t)
	ctx := t.Context()

	if err := store.CoverImages().Put(ctx, revisionID,
		domain.GeneratedImage{Bytes: []byte{0x01}, ContentType: "image/png"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	cover, err := store.Covers().FindByID(ctx, coverID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if err := store.Covers().Delete(ctx, coverID, cover.UserID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := store.CoverImages().Find(ctx, revisionID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want the image to have been cascaded away", err)
	}
}

// Deleting one run takes only that run's bytes, which is what makes "delete this
// run" different from "delete this playlist".
func TestDeletingARevisionRemovesOnlyItsImage(t *testing.T) {
	store, coverID, revisionID := newStore(t)
	ctx := t.Context()

	kept := &domain.CoverRevision{CoverID: coverID, Status: domain.CoverStatusReady}
	if err := store.Covers().SaveRevision(ctx, kept); err != nil {
		t.Fatalf("save second revision: %v", err)
	}
	for _, id := range []string{revisionID, kept.ID} {
		if err := store.CoverImages().Put(ctx, id,
			domain.GeneratedImage{Bytes: []byte{0x01}, ContentType: "image/png"}); err != nil {
			t.Fatalf("Put(%s): %v", id, err)
		}
	}

	cover, err := store.Covers().FindByID(ctx, coverID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if err := store.Covers().DeleteRevision(ctx, coverID, revisionID, cover.UserID); err != nil {
		t.Fatalf("DeleteRevision: %v", err)
	}

	if _, err := store.CoverImages().Find(ctx, revisionID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want the deleted run's image to have cascaded away", err)
	}
	if _, err := store.CoverImages().Find(ctx, kept.ID); err != nil {
		t.Fatalf("the other run's image went with it: %v", err)
	}
}
