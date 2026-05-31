package handlers

import (
	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command/generatecover"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/app/query/listcovers"
	"github.com/phillipjad/aurify/backend/internal/transport/http/dto"
)

// Covers serves both the write side (generate) and read side (get/list) of
// covers.
type Covers struct {
	app *app.App
}

// NewCovers constructs the covers handler.
func NewCovers(a *app.App) *Covers { return &Covers{app: a} }

// Generate kicks off cover generation for a playlist (command).
// POST /api/v1/covers
func (h *Covers) Generate(c mux.RouteContext) {
	var req dto.GenerateCoverRequest
	if err := c.Bind(&req); err != nil {
		c.BadRequest("invalid body", err.Error())
		return
	}

	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}

	id, err := h.app.Commands.GenerateCover.Handle(c, generatecover.Command{
		UserID:     userID,
		Platform:   req.Platform,
		PlaylistID: req.PlaylistID,
	})
	if err != nil {
		respondError(c, err)
		return
	}

	cover, err := h.app.Queries.GetCover.Handle(c, getcover.Query{CoverID: id, UserID: userID})
	if err != nil {
		respondError(c, err)
		return
	}
	c.Created(dto.NewCoverResponse(cover))
}

// Get returns a single cover by id (query).
// GET /api/v1/covers/{id}
func (h *Covers) Get(c mux.RouteContext) {
	id, ok := c.Params().String("id")
	if !ok {
		c.BadRequest("missing id", "path parameter 'id' is required")
		return
	}

	cover, err := h.app.Queries.GetCover.Handle(c, getcover.Query{
		CoverID: id,
		UserID:  currentUser(c),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.OK(dto.NewCoverResponse(cover))
}

// List returns the current user's covers (query).
// GET /api/v1/covers?limit=&offset=
func (h *Covers) List(c mux.RouteContext) {
	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}

	limit, _ := c.Query().Int("limit")
	offset, _ := c.Query().Int("offset")

	covers, err := h.app.Queries.ListCovers.Handle(c, listcovers.Query{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.OK(dto.NewCoverList(covers))
}
