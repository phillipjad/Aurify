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
	return cover, nil
}
