package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

// WatchCover signals on every change to one cover, and cancels via the returned
// func. Satisfied by postgres.CoverWatcher.Subscribe, wired in cmd/api.
type WatchCover func(coverID string) (<-chan struct{}, func())

// Covers serves the write side (generate), the read side (get/list), and the
// live side (events) of covers.
type Covers struct {
	app *app.App
	// The stream's own dependencies: change signals, and the revocation check
	// re-run mid-stream because a stream outlives its one middleware pass.
	watch    WatchCover
	sessions SessionCheck
}

// NewCovers constructs the covers handler.
func NewCovers(a *app.App, watch WatchCover, sessions SessionCheck) *Covers {
	return &Covers{app: a, watch: watch, sessions: sessions}
}

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

// Each beat proves the stream alive, re-checks revocation, and re-reads the
// cover, which is what recovers a notification lost while reconnecting.
const heartbeatInterval = 15 * time.Second

// Events streams a cover's lifecycle over SSE until terminal (query, long-lived).
// GET /api/v1/covers/{id}/events
//
// Every event is a complete snapshot, which is what makes reconnects trivial:
// the first event catches a client up, so there is no replay log to maintain.
// EventSource cannot set headers, so it authenticates by cookie like every
// other route, and revocation is re-checked on each heartbeat.
func (h *Covers) Events(c mux.RouteContext) {
	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}
	sessionID := c.User().CustomClaimValue(SessionClaim)

	id, ok := c.Params().String("id")
	if !ok {
		c.BadRequest("missing id", "path parameter 'id' is required")
		return
	}

	// Before the stream opens, so someone else's id gets a plain 404.
	cover, err := h.app.Queries.GetCover.Handle(c, getcover.Query{CoverID: id, UserID: userID})
	if err != nil {
		respondError(c, err)
		return
	}

	// Events go through the raw writer, the only one that can flush; headers go
	// through the context's so the access log still sees them.
	w := streamWriter(c)
	rc := http.NewResponseController(w)
	// The server's 10s WriteTimeout would sever this, cleared per-connection.
	_ = rc.SetWriteDeadline(time.Time{})

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().WriteHeader(http.StatusOK)

	send := func(cover *domain.Cover) bool {
		payload, merr := json.Marshal(dto.NewCoverResponse(cover))
		if merr != nil {
			return false
		}
		if _, werr := fmt.Fprintf(w, "event: cover\ndata: %s\n\n", payload); werr != nil {
			return false
		}
		// Unflushed events buffer until close, which is worse than no stream.
		return rc.Flush() == nil
	}

	if !send(cover) || cover.Status.Terminal() {
		return
	}

	updates, cancel := h.watch(id)
	defer cancel()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	ctx := c.Request().Context()
	lastSent := cover.UpdatedAt
	for {
		select {
		case <-ctx.Done():
			return
		case <-updates:
		case <-heartbeat.C:
			if h.sessions(ctx, sessionID) != nil {
				return
			}
		}

		cover, err = h.app.Queries.GetCover.Handle(c, getcover.Query{CoverID: id, UserID: userID})
		if err != nil {
			return
		}
		if cover.UpdatedAt.Equal(lastSent) {
			// Nothing new: a comment line satisfies proxies without waking the
			// client, which a full event would (parse, cache write, re-render).
			if _, werr := fmt.Fprint(w, ": ping\n\n"); werr != nil || rc.Flush() != nil {
				return
			}
			continue
		}
		lastSent = cover.UpdatedAt
		if !send(cover) || cover.Status.Terminal() {
			return
		}
	}
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
