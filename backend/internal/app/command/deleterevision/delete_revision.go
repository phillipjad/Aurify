// Package deleterevision removes a single generation run from a cover the user
// owns, leaving the cover and its other runs alone.
//
// The sibling of deletecover: this one drops one run, that one drops the
// playlist and every run with it. Deleting the newest successful run falls back
// to the one before it, because the current artwork is simply "the newest
// revision that is ready"; deleting the last one leaves the tile on the
// placeholder.
package deleterevision

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
)

// Command requests deletion of one run of one of the user's covers.
type Command struct {
	CoverID    string
	RevisionID string
	UserID     string
}

// Handler executes the DeleteRevision command.
type Handler struct {
	covers ports.CoverRepository
}

// NewHandler constructs a DeleteRevision handler.
func NewHandler(covers ports.CoverRepository) *Handler {
	return &Handler{covers: covers}
}

// Handle deletes the run. The repository scopes the delete to the owning cover
// and user, and returns domain.ErrNotFound when either id does not resolve.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	return h.covers.DeleteRevision(ctx, cmd.CoverID, cmd.RevisionID, cmd.UserID)
}
