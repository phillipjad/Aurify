// Package listcovers reads the covers a user has generated.
package listcovers

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// validStatuses is the set of lifecycle states a caller may filter by. Anything
// else (including the empty string) means "all statuses".
var validStatuses = map[domain.CoverStatus]bool{
	domain.CoverStatusPending:    true,
	domain.CoverStatusAnalyzing:  true,
	domain.CoverStatusGenerating: true,
	domain.CoverStatusReady:      true,
	domain.CoverStatusFailed:     true,
}

// Query is a paginated request for a user's covers, optionally filtered by
// status. After is the keyset position; its zero value is the first page.
type Query struct {
	UserID string
	Status domain.CoverStatus
	Limit  int
	After  domain.CoverCursor
}

// Handler executes the ListCovers query.
type Handler struct {
	covers ports.CoverRepository
}

// NewHandler constructs a ListCovers handler.
func NewHandler(covers ports.CoverRepository) *Handler {
	return &Handler{covers: covers}
}

// Handle returns the user's covers, clamping pagination to sane bounds. An
// unrecognized status is treated as "all" so a bad query parameter widens the
// result rather than erroring.
func (h *Handler) Handle(ctx context.Context, q Query) ([]domain.Cover, error) {
	if q.Limit <= 0 || q.Limit > maxLimit {
		q.Limit = defaultLimit
	}
	// Half a cursor is not a position. Falling back to the first page beats
	// paging from an arbitrary one, and it is what a client that sent only a
	// timestamp meant anyway.
	if q.After.UpdatedAt.IsZero() || q.After.ID == "" {
		q.After = domain.CoverCursor{}
	}

	status := ""
	if validStatuses[q.Status] {
		status = string(q.Status)
	}
	return h.covers.ListByUser(ctx, q.UserID, status, q.Limit, q.After)
}
