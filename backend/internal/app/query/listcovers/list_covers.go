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
// status.
type Query struct {
	UserID string
	Status domain.CoverStatus
	Limit  int
	Offset int
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
	if q.Offset < 0 {
		q.Offset = 0
	}

	status := ""
	if validStatuses[q.Status] {
		status = string(q.Status)
	}
	return h.covers.ListByUser(ctx, q.UserID, status, q.Limit, q.Offset)
}
