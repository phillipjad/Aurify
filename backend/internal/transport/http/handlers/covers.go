package handlers

import (
	"net/http"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command/deletecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/generatecover"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcoverimage"
	"github.com/phillipjad/aurify/backend/internal/app/query/listcovers"
	"github.com/phillipjad/aurify/backend/internal/domain"
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

// Image serves the bytes behind a cover's imageUrl (query).
// GET /api/v1/covers/{id}/image
//
// This route is anonymous, and deliberately so. It is what an <img> tag fetches,
// and a tag loading cross-origin (the app on :5173, the API on :8080) does not
// send credentials, so an authenticated route simply would not render. Cover ids
// are UUIDv4, which is the unguessable-name trade
// docs/adr/0014-hosted-generation-apis.md already accepted for its public
// bucket.
//
// Because the route is AllowAnonymous, mux skips the authentication middleware
// entirely and currentUser(c) would be empty even for a signed-in caller. Do not
// add an ownership check here expecting it to work.
func (h *Covers) Image(c mux.RouteContext) {
	id, ok := c.Params().String("id")
	if !ok {
		c.BadRequest("missing id", "path parameter 'id' is required")
		return
	}

	image, err := h.app.Queries.GetCoverImage.Handle(c, getcoverimage.Query{CoverID: id})
	if err != nil {
		respondError(c, err)
		return
	}

	w := c.Response()
	w.Header().Set("Content-Type", image.ContentType)
	// The bytes for a given id never change: regenerating a playlist creates a
	// new cover with a new id.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Bytes)

	// Content-Length is deliberately not set here. The router compresses
	// responses, so the bytes on the wire are not len(image.Bytes), and a
	// hand-set length made every browser abort the image with
	// ERR_CONTENT_LENGTH_MISMATCH while curl fetched it happily: curl does not
	// ask for gzip by default, browsers always do. Whatever writes the body last
	// is what knows how long it is.
}

// Delete removes one of the current user's covers (command).
// DELETE /api/v1/covers/{id}
func (h *Covers) Delete(c mux.RouteContext) {
	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}

	id, ok := c.Params().String("id")
	if !ok {
		c.BadRequest("missing id", "path parameter 'id' is required")
		return
	}

	if err := h.app.Commands.DeleteCover.Handle(c, deletecover.Command{
		CoverID: id,
		UserID:  userID,
	}); err != nil {
		respondError(c, err)
		return
	}
	c.NoContent()
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
	status, _ := c.Query().String("status")

	covers, err := h.app.Queries.ListCovers.Handle(c, listcovers.Query{
		UserID: userID,
		Status: domain.CoverStatus(status),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.OK(dto.NewCoverList(covers))
}
