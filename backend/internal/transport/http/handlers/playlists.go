package handlers

import (
	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/query/listplaylists"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/transport/http/dto"
)

// Playlists serves the read side for a user's DSP playlists.
type Playlists struct {
	app *app.App
}

// NewPlaylists constructs the playlists handler.
func NewPlaylists(a *app.App) *Playlists { return &Playlists{app: a} }

// List returns the current user's playlists for a platform.
// GET /api/v1/playlists?platform=spotify
func (h *Playlists) List(c mux.RouteContext) {
	platform, ok := c.Query().String("platform")
	if !ok {
		c.BadRequest("missing platform", "query parameter 'platform' is required")
		return
	}

	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}

	search, _ := c.Query().String("q")
	sortKey, _ := c.Query().String("sort")
	limit, _ := c.Query().Int("limit")
	offset, _ := c.Query().Int("offset")

	playlists, err := h.app.Queries.ListPlaylists.Handle(c, listplaylists.Query{
		UserID:   userID,
		Platform: domain.DSPPlatform(platform),
		Search:   search,
		Sort:     listplaylists.SortKey(sortKey),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		respondError(c, err)
		return
	}

	c.OK(dto.NewPlaylistList(playlists))
}
