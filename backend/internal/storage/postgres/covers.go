package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres/db"
)

// CoverRepository is the PostgreSQL-backed ports.CoverRepository. The evolving
// PlaylistAnalysis is stored as JSONB; everything else is a relational column.
type CoverRepository struct {
	q *db.Queries
}

var _ ports.CoverRepository = (*CoverRepository)(nil)

// Save inserts a new cover or updates an existing one (upsert by id).
func (r *CoverRepository) Save(ctx context.Context, cover *domain.Cover) error {
	now := time.Now().UTC()
	cover.UpdatedAt = now
	if cover.ID == "" {
		cover.ID = uuid.NewString()
		if cover.CreatedAt.IsZero() {
			cover.CreatedAt = now
		}
	}

	analysis, err := json.Marshal(cover.Analysis)
	if err != nil {
		return err
	}

	return r.q.UpsertCover(ctx, db.UpsertCoverParams{
		ID:           cover.ID,
		UserID:       cover.UserID,
		Platform:     string(cover.Platform),
		PlaylistID:   cover.PlaylistID,
		PlaylistName: cover.PlaylistName,
		Status:       string(cover.Status),
		Prompt:       cover.Prompt,
		ImageUrl:     cover.ImageURL,
		Analysis:     analysis,
		Error:        cover.Error,
		CreatedAt:    tsFromTime(cover.CreatedAt),
		UpdatedAt:    tsFromTime(cover.UpdatedAt),
	})
}

// FailStuck fails covers left non-terminal since before cutoff. Not on
// ports.CoverRepository: operational recovery, not an application command.
func (r *CoverRepository) FailStuck(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.FailStuckCovers(ctx, tsFromTime(cutoff))
}

// FindByID looks up a cover by id, mapping a miss to domain.ErrNotFound.
func (r *CoverRepository) FindByID(ctx context.Context, id string) (*domain.Cover, error) {
	row, err := r.q.GetCoverByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	cover, err := toDomainCover(row)
	if err != nil {
		return nil, err
	}
	return &cover, nil
}

// ListByUser returns a user's covers, newest first, paginated, optionally
// filtered to a single status (empty status = all).
func (r *CoverRepository) ListByUser(
	ctx context.Context,
	userID, status string,
	limit, offset int,
) ([]domain.Cover, error) {
	rows, err := r.q.ListCoversByUser(ctx, db.ListCoversByUserParams{
		UserID:    userID,
		Status:    status,
		RowLimit:  int32(limit),
		RowOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	covers := make([]domain.Cover, 0, len(rows))
	for _, row := range rows {
		cover, err := toDomainCover(row)
		if err != nil {
			return nil, err
		}
		covers = append(covers, cover)
	}
	return covers, nil
}

// Delete removes a user's cover. The DELETE is scoped to the owner in SQL, so a
// zero rows-affected count means the cover either does not exist or belongs to
// someone else — both map to domain.ErrNotFound so callers can't probe for the
// existence of covers they don't own.
func (r *CoverRepository) Delete(ctx context.Context, id, userID string) error {
	n, err := r.q.DeleteCover(ctx, db.DeleteCoverParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// toDomainCover maps a stored row to the domain aggregate, decoding the JSONB
// analysis payload back into PlaylistAnalysis.
func toDomainCover(row db.Cover) (domain.Cover, error) {
	cover := domain.Cover{
		ID:           row.ID,
		UserID:       row.UserID,
		Platform:     domain.DSPPlatform(row.Platform),
		PlaylistID:   row.PlaylistID,
		PlaylistName: row.PlaylistName,
		Status:       domain.CoverStatus(row.Status),
		Prompt:       row.Prompt,
		ImageURL:     row.ImageUrl,
		Error:        row.Error,
		CreatedAt:    timeFromTS(row.CreatedAt),
		UpdatedAt:    timeFromTS(row.UpdatedAt),
	}
	if len(row.Analysis) > 0 {
		if err := json.Unmarshal(row.Analysis, &cover.Analysis); err != nil {
			return domain.Cover{}, err
		}
	}
	return cover, nil
}
