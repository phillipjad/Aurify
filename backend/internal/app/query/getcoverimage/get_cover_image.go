// Package getcoverimage reads the stored bytes behind a cover's image URL.
package getcoverimage

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Query identifies the cover whose image is wanted.
//
// There is deliberately no UserID. This is what an <img> tag fetches, and a tag
// loading cross-origin does not send credentials, so there is no session to
// check against. Cover ids are UUIDv4, which is the same unguessable-name trade
// docs/adr/0014-hosted-generation-apis.md accepted for its public bucket.
type Query struct {
	CoverID string
}

// Handler executes the GetCoverImage query.
type Handler struct {
	images ports.ImageStore
}

// NewHandler constructs a GetCoverImage handler.
func NewHandler(images ports.ImageStore) *Handler {
	return &Handler{images: images}
}

// Handle returns the stored image, or domain.ErrNotFound.
func (h *Handler) Handle(ctx context.Context, q Query) (domain.GeneratedImage, error) {
	return h.images.Find(ctx, q.CoverID)
}
