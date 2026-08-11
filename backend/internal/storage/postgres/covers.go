package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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

// StartRun claims the playlist for a new generation, returning
// domain.ErrGenerationInFlight when one is already running.
//
// The exclusion lives in the SQL, not here: a read-then-write in Go would let
// two callers both see an idle cover and both start. The conditional upsert
// answers with no row when the claim is refused, which is the miss below.
func (r *CoverRepository) StartRun(ctx context.Context, cover *domain.Cover) error {
	r.stamp(cover)

	id, err := r.q.StartCoverRun(ctx, db.StartCoverRunParams(r.upsertArgs(cover)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrGenerationInFlight
		}
		return err
	}
	cover.ID = id
	return nil
}

// Save records a stage change for a run already claimed by StartRun. The id it
// writes back is the row's own, so a regeneration that arrived with a freshly
// minted id adopts the existing cover's instead of creating a second tile.
func (r *CoverRepository) Save(ctx context.Context, cover *domain.Cover) error {
	r.stamp(cover)

	id, err := r.q.UpsertCover(ctx, db.UpsertCoverParams(r.upsertArgs(cover)))
	if err != nil {
		return err
	}
	cover.ID = id
	return nil
}

// stamp fills in the fields the caller does not supply: a fresh id for a cover
// that has never been written, and the write time.
func (r *CoverRepository) stamp(cover *domain.Cover) {
	now := time.Now().UTC()
	cover.UpdatedAt = now
	if cover.ID == "" {
		cover.ID = uuid.NewString()
		if cover.CreatedAt.IsZero() {
			cover.CreatedAt = now
		}
	}
}

// upsertArgs is the column set both writes share. The two generated param
// structs are identical, so either conversion is free.
func (r *CoverRepository) upsertArgs(cover *domain.Cover) db.UpsertCoverParams {
	return db.UpsertCoverParams{
		ID:           cover.ID,
		UserID:       cover.UserID,
		Platform:     string(cover.Platform),
		PlaylistID:   cover.PlaylistID,
		PlaylistName: cover.PlaylistName,
		Status:       string(cover.Status),
		Error:        cover.Error,
		CreatedAt:    tsFromTime(cover.CreatedAt),
		UpdatedAt:    tsFromTime(cover.UpdatedAt),
	}
}

// Touch stamps a run as still alive, without touching anything else about it.
func (r *CoverRepository) Touch(ctx context.Context, coverID string) error {
	return r.q.TouchCover(ctx, coverID)
}

// FailStuck fails covers left non-terminal since before cutoff. Not on
// ports.CoverRepository: operational recovery, not an application command.
//
// A cutoff of "now" reaps every run in flight, which is what a restart wants:
// generation is in-process and the service runs a single instance, so nothing
// that was running before the restart is running after it.
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
	cover, err := toDomainCover(row.Cover, row.RevisionID, row.Analysis, row.RunCount)
	if err != nil {
		return nil, err
	}
	return &cover, nil
}

// ListByUser returns a user's covers, newest first, one keyset page at a time,
// optionally filtered to a single status (empty status = all).
func (r *CoverRepository) ListByUser(
	ctx context.Context,
	userID, status string,
	limit int,
	after domain.CoverCursor,
) ([]domain.Cover, error) {
	// A zero cursor is the first page, and NULL is what the query tests for.
	before := pgtype.Timestamptz{}
	if !after.UpdatedAt.IsZero() {
		before = tsFromTime(after.UpdatedAt)
	}

	rows, err := r.q.ListCoversByUser(ctx, db.ListCoversByUserParams{
		UserID:   userID,
		Status:   status,
		Before:   before,
		BeforeID: after.ID,
		RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}

	covers := make([]domain.Cover, 0, len(rows))
	for _, row := range rows {
		cover, err := toDomainCover(row.Cover, row.RevisionID, row.Analysis, row.RunCount)
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

// SaveRevision records a finished run, minting an id when it has none so the
// caller can key the image bytes by it.
func (r *CoverRepository) SaveRevision(ctx context.Context, rev *domain.CoverRevision) error {
	if rev.ID == "" {
		rev.ID = uuid.NewString()
	}
	if rev.CompletedAt.IsZero() {
		rev.CompletedAt = time.Now().UTC()
	}

	analysis, err := json.Marshal(rev.Analysis)
	if err != nil {
		return err
	}

	return r.q.UpsertCoverRevision(ctx, db.UpsertCoverRevisionParams{
		ID:          rev.ID,
		CoverID:     rev.CoverID,
		Status:      string(rev.Status),
		Prompt:      rev.Prompt,
		Analysis:    analysis,
		Error:       rev.Error,
		CompletedAt: tsFromTime(rev.CompletedAt),
	})
}

// ListRevisions returns a cover's run history, newest first.
func (r *CoverRepository) ListRevisions(
	ctx context.Context,
	coverID string,
	includeFailed bool,
) ([]domain.CoverRevision, error) {
	rows, err := r.q.ListCoverRevisions(ctx, db.ListCoverRevisionsParams{
		CoverID:       coverID,
		IncludeFailed: includeFailed,
	})
	if err != nil {
		return nil, err
	}

	out := make([]domain.CoverRevision, 0, len(rows))
	for _, row := range rows {
		rev, err := toDomainRevision(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, nil
}

// DeleteRevision removes one run. Scoped to the owning cover and user in SQL,
// for the same reason Delete is: a miss and someone else's run are the same
// answer.
func (r *CoverRepository) DeleteRevision(ctx context.Context, coverID, revisionID, userID string) error {
	n, err := r.q.DeleteCoverRevision(ctx, db.DeleteCoverRevisionParams{
		RevisionID: revisionID,
		CoverID:    coverID,
		UserID:     userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// toDomainCover maps a stored row plus its current revision to the domain
// aggregate. revisionID is empty until a run has succeeded, which is also what
// makes ImageURL empty: the tile then renders the placeholder.
func toDomainCover(
	row db.Cover,
	revisionID string,
	analysis []byte,
	runCount int64,
) (domain.Cover, error) {
	cover := domain.Cover{
		ID:           row.ID,
		UserID:       row.UserID,
		Platform:     domain.DSPPlatform(row.Platform),
		PlaylistID:   row.PlaylistID,
		PlaylistName: row.PlaylistName,
		Status:       domain.CoverStatus(row.Status),
		Error:        row.Error,
		CreatedAt:    timeFromTS(row.CreatedAt),
		UpdatedAt:    timeFromTS(row.UpdatedAt),
		RunCount:     int(runCount),
	}
	if revisionID != "" {
		cover.ImageURL = coverImageURL(revisionID)
	}
	if len(analysis) > 0 {
		if err := json.Unmarshal(analysis, &cover.Analysis); err != nil {
			return domain.Cover{}, err
		}
	}
	return cover, nil
}

// toDomainRevision maps one stored run. A failed run has no bytes, so it carries
// no image URL.
func toDomainRevision(row db.CoverRevision) (domain.CoverRevision, error) {
	rev := domain.CoverRevision{
		ID:          row.ID,
		CoverID:     row.CoverID,
		Number:      int(row.RevisionNumber),
		Status:      domain.CoverStatus(row.Status),
		Prompt:      row.Prompt,
		Error:       row.Error,
		CompletedAt: timeFromTS(row.CompletedAt),
	}
	if rev.Status == domain.CoverStatusReady {
		rev.ImageURL = coverImageURL(row.ID)
	}
	if len(row.Analysis) > 0 {
		if err := json.Unmarshal(row.Analysis, &rev.Analysis); err != nil {
			return domain.CoverRevision{}, err
		}
	}
	return rev, nil
}
