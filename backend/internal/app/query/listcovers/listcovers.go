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

// Query is a paginated request for a user's covers.
type Query struct {
	UserID string
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

// Handle returns the user's covers, clamping pagination to sane bounds.
func (h *Handler) Handle(ctx context.Context, q Query) ([]domain.Cover, error) {
	if q.Limit <= 0 || q.Limit > maxLimit {
		q.Limit = defaultLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return h.covers.ListByUser(ctx, q.UserID, q.Limit, q.Offset)
}
