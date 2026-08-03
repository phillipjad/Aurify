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

// newStore resets the schema and returns a store plus a cover row the images can
// hang off, since cover_images has a foreign key onto covers.
func newStore(t *testing.T) (*postgres.Store, string) {
	t.Helper()
	ctx := t.Context()
	store, _ := pgtest.Reset(t)

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
	return store, cover.ID
}

// BYTEA is the one column type in this schema where a driver-level encoding
// mistake produces plausible-looking but corrupt data, so the bytes are compared
// exactly rather than by length.
func TestCoverImageRoundTrip(t *testing.T) {
	store, coverID := newStore(t)
	ctx := t.Context()

	// Bytes chosen to break on anything that treats them as text: a NUL, a
	// backslash, invalid UTF-8, and a high byte.
	original := domain.GeneratedImage{
		Bytes:       []byte{0xFF, 0xD8, 0x00, '\\', 0xC3, 0x28, 0x7F, 0xFE},
		ContentType: "image/jpeg",
	}

	url, err := store.CoverImages().Put(ctx, coverID, original)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if want := "/api/v1/covers/" + coverID + "/image"; url != want {
		t.Errorf("url = %q, want %q", url, want)
	}

	got, err := store.CoverImages().Find(ctx, coverID)
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

// Regenerating a cover writes to the same id, so a plain INSERT would fail on
// the primary key and lose the new image.
func TestPutReplacesAnExistingImage(t *testing.T) {
	store, coverID := newStore(t)
	ctx := t.Context()

	first := domain.GeneratedImage{Bytes: []byte{0x01, 0x02}, ContentType: "image/png"}
	if _, err := store.CoverImages().Put(ctx, coverID, first); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second := domain.GeneratedImage{Bytes: []byte{0x03, 0x04, 0x05}, ContentType: "image/jpeg"}
	if _, err := store.CoverImages().Put(ctx, coverID, second); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	got, err := store.CoverImages().Find(ctx, coverID)
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
	store, _ := newStore(t)

	_, err := store.CoverImages().Find(t.Context(), "0f8fad5b-d9cb-469f-a165-70867728950e")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

// Deleting a cover must take its image with it, or the bytes outlive the row
// that referenced them and nothing will ever collect them.
func TestDeletingACoverRemovesItsImage(t *testing.T) {
	store, coverID := newStore(t)
	ctx := t.Context()

	if _, err := store.CoverImages().Put(ctx, coverID,
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

	if _, err := store.CoverImages().Find(ctx, coverID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want the image to have been cascaded away", err)
	}
}
