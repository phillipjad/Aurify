// Package getcover reads a single generated cover by id.
package getcover

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Query identifies a cover and the user requesting it (for ownership checks).
type Query struct {
	CoverID string
	UserID  string
	// WithRevisions loads the run history. Off by default because the SSE
	// stream re-reads the cover on every notification and heartbeat, and
	// resending an unchanged history with each snapshot buys nothing.
	WithRevisions bool
	// IncludeFailedRevisions widens that history to runs that produced no
	// artwork. They are recorded because their error is what explains a
	// failure to the client, and their prompt is what explains it to us, but
	// they are not what the history is for.
	IncludeFailedRevisions bool
}

// Handler executes the GetCover query.
type Handler struct {
	covers ports.CoverRepository
}

// NewHandler constructs a GetCover handler.
func NewHandler(covers ports.CoverRepository) *Handler {
	return &Handler{covers: covers}
}

// Handle returns the cover, enforcing ownership when a UserID is supplied.
func (h *Handler) Handle(ctx context.Context, q Query) (*domain.Cover, error) {
	cover, err := h.covers.FindByID(ctx, q.CoverID)
	if err != nil {
		return nil, err
	}
	if q.UserID != "" && cover.UserID != q.UserID {
		return nil, domain.ErrNotFound
	}
	if q.WithRevisions {
		revisions, rerr := h.covers.ListRevisions(ctx, cover.ID, q.IncludeFailedRevisions)
		if rerr != nil {
			return nil, rerr
		}
		cover.Revisions = revisions
	}
	return cover, nil
}
