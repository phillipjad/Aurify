// Package setplaylistcover writes a generated cover back onto the playlist it
// was made from, on the DSP it came from.
//
// A write, so it lives here rather than in query/, and it is the first thing
// Aurify sends *to* a provider: everything before this only ever read.
package setplaylistcover

import (
	"context"
	"fmt"

	"github.com/phillipjad/aurify/backend/internal/app/dspconn"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Command requests that a cover be pushed to its playlist.
type Command struct {
	CoverID string
	UserID  string
}

// Handler executes the SetPlaylistCover command.
type Handler struct {
	covers      ports.CoverRepository
	images      ports.ImageStore
	connections *dspconn.Resolver
}

// NewHandler constructs a SetPlaylistCover handler.
func NewHandler(
	covers ports.CoverRepository,
	images ports.ImageStore,
	connections *dspconn.Resolver,
) *Handler {
	return &Handler{covers: covers, images: images, connections: connections}
}

// Handle pushes the cover's current artwork to the playlist.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	cover, err := h.covers.FindByID(ctx, cmd.CoverID)
	if err != nil {
		return err
	}
	// FindByID is not scoped to an owner, so the check is here. Answering
	// ErrNotFound rather than ErrUnauthorized keeps someone else's cover ids
	// unconfirmable.
	if cover.UserID != cmd.UserID {
		return domain.ErrNotFound
	}

	// Newest first, failures excluded, so this is the artwork the detail page is
	// showing. A cover whose only runs failed has nothing to push.
	revisions, err := h.covers.ListRevisions(ctx, cover.ID, false)
	if err != nil {
		return err
	}
	if len(revisions) == 0 {
		return fmt.Errorf("setplaylistcover: cover %s has no successful run: %w", cmd.CoverID, domain.ErrNotFound)
	}

	image, err := h.images.Find(ctx, revisions[0].ID)
	if err != nil {
		return err
	}

	provider, conn, err := h.connections.Resolve(ctx, cmd.UserID, cover.Platform)
	if err != nil {
		return err
	}
	setter, ok := provider.(ports.PlaylistCoverSetter)
	if !ok {
		return fmt.Errorf("setplaylistcover: %s cannot set playlist art: %w", cover.Platform, domain.ErrUnsupportedPlatform)
	}

	return setter.SetPlaylistCover(ctx, conn, cover.PlaylistID, image)
}
