// Package listplaylists reads the playlists a user has on a connected DSP.
package listplaylists

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Query asks for a user's playlists on a specific platform.
type Query struct {
	UserID   string
	Platform domain.DSPPlatform
}

// Handler executes the ListPlaylists query.
type Handler struct {
	users     ports.UserRepository
	providers ports.DSPRegistry
}

// NewHandler constructs a ListPlaylists handler.
func NewHandler(users ports.UserRepository, providers ports.DSPRegistry) *Handler {
	return &Handler{users: users, providers: providers}
}

// Handle resolves the user's connection and asks the provider for playlists.
func (h *Handler) Handle(ctx context.Context, q Query) ([]domain.Playlist, error) {
	user, err := h.users.FindByID(ctx, q.UserID)
	if err != nil {
		return nil, err
	}

	conn, ok := user.Connections[q.Platform]
	if !ok {
		return nil, domain.ErrUnauthorized
	}

	provider, err := h.providers.Get(q.Platform)
	if err != nil {
		return nil, err
	}

	return provider.ListPlaylists(ctx, conn)
}
