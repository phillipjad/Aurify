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
// The store builds it because the store is what decides where bytes live: a
// bucket-backed implementation would return its own public URL here instead, and
// nothing above this line would change. It is relative on purpose, so covers
// generated against localhost still resolve once the API has a real origin.
const imageURLFormat = "/api/v1/covers/%s/image"

// CoverImageRepository is the PostgreSQL-backed ports.ImageStore.
type CoverImageRepository struct {
	q *db.Queries
}

var _ ports.ImageStore = (*CoverImageRepository)(nil)

// Put stores the image for a cover and returns the URL that serves it.
func (r *CoverImageRepository) Put(
	ctx context.Context,
	coverID string,
	image domain.GeneratedImage,
) (string, error) {
	err := r.q.SaveCoverImage(ctx, db.SaveCoverImageParams{
		CoverID:     coverID,
		Bytes:       image.Bytes,
		ContentType: image.ContentType,
		CreatedAt:   tsFromTime(time.Now().UTC()),
	})
	if err != nil {
		return "", err
	}
	return coverImageURL(coverID), nil
}

// Find returns a cover's stored image, mapping a miss to domain.ErrNotFound.
func (r *CoverImageRepository) Find(
	ctx context.Context,
	coverID string,
) (domain.GeneratedImage, error) {
	row, err := r.q.FindCoverImage(ctx, coverID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.GeneratedImage{}, domain.ErrNotFound
		}
		return domain.GeneratedImage{}, err
	}
	return domain.GeneratedImage{Bytes: row.Bytes, ContentType: row.ContentType}, nil
}

func coverImageURL(coverID string) string {
	return fmt.Sprintf(imageURLFormat, coverID)
}
