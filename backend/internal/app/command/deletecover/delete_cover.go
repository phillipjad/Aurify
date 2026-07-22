// Package deletecover removes a cover a user owns.
package deletecover

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
)

// Command requests deletion of one of the user's covers.
type Command struct {
	CoverID string
	UserID  string
}

// Handler executes the DeleteCover command.
type Handler struct {
	covers ports.CoverRepository
}

// NewHandler constructs a DeleteCover handler.
func NewHandler(covers ports.CoverRepository) *Handler {
	return &Handler{covers: covers}
}

// Handle deletes the cover. The repository scopes the delete to the owner and
// returns domain.ErrNotFound when the cover does not exist for this user.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	return h.covers.Delete(ctx, cmd.CoverID, cmd.UserID)
}
