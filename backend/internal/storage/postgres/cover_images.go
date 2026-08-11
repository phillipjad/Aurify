package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// imageURLFormat is the path that serves a stored image.
//
// The store owns it because the store is what decides where bytes live: a
// bucket-backed implementation would build its own public URL here instead, and
// nothing above this line would change. It is relative on purpose, so covers
// generated against localhost still resolve once the API has a real origin.
//
// The id in it is a *revision* id, not a cover id. A cover's artwork changes as
// it is regenerated, so a URL keyed by the cover could never be cached; keyed by
// the run that produced it, the bytes behind a URL never change at all.
const imageURLFormat = "/api/v1/covers/%s/image"

// CoverImageRepository is the PostgreSQL-backed ports.ImageStore.
type CoverImageRepository struct {
	q *db.Queries
}

var _ ports.ImageStore = (*CoverImageRepository)(nil)

// Put stores the image produced by one run. The revision row must already
// exist: the bytes are keyed by it and cascade with it.
func (r *CoverImageRepository) Put(
	ctx context.Context,
	revisionID string,
	image domain.GeneratedImage,
) error {
	return r.q.SaveCoverImage(ctx, db.SaveCoverImageParams{
		RevisionID:  revisionID,
		Bytes:       image.Bytes,
		ContentType: image.ContentType,
		CreatedAt:   tsFromTime(time.Now().UTC()),
	})
}

// Find returns a run's stored image, mapping a miss to domain.ErrNotFound.
func (r *CoverImageRepository) Find(
	ctx context.Context,
	revisionID string,
) (domain.GeneratedImage, error) {
	row, err := r.q.FindCoverImage(ctx, revisionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.GeneratedImage{}, domain.ErrNotFound
		}
		return domain.GeneratedImage{}, err
	}
	return domain.GeneratedImage{Bytes: row.Bytes, ContentType: row.ContentType}, nil
}

func coverImageURL(revisionID string) string {
	return fmt.Sprintf(imageURLFormat, revisionID)
}
