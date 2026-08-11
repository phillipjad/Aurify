// Package dto defines the JSON request/response shapes for the HTTP API and
// the mapping to/from domain types. Keeping transport DTOs separate from domain
// models lets the wire format evolve independently and keeps JSON tags out of
// the domain.
//
// Struct tags drive OpenAPI generation (via fgrzl/mux + fgrzl/json/jsonschema):
//   - binding:"required" marks a field as required in the generated schema.
//   - enum:"a,b,c" emits an enum, so the field generates a TypeScript union on
//     the client. The enum values mirror the domain constants in
//     internal/domain (DSPPlatform / CoverStatus) — keep them in sync.
package dto

import (
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// GenerateCoverRequest is the POST /api/v1/covers body.
type GenerateCoverRequest struct {
	Platform   domain.DSPPlatform `json:"platform"   binding:"required" enum:"spotify,apple_music,youtube_music"`
	PlaylistID string             `json:"playlistId" binding:"required"`
}

// PlaylistResponse is the API representation of a playlist.
type PlaylistResponse struct {
	ID          string             `json:"id"                 binding:"required"`
	Platform    domain.DSPPlatform `json:"platform"           binding:"required" enum:"spotify,apple_music,youtube_music"`
	Name        string             `json:"name"               binding:"required"`
	Description string             `json:"description"        binding:"required"`
	TrackCount  int                `json:"trackCount"         binding:"required"`
	ImageURL    string             `json:"imageUrl,omitempty"`
}

// ColorWeightResponse is one entry of a cover's derived palette.
type ColorWeightResponse struct {
	Dimension string  `json:"dimension" binding:"required"`
	HexColor  string  `json:"hexColor"  binding:"required"`
	Weight    float64 `json:"weight"    binding:"required"`
}

// CoverResponse is the API representation of one playlist's cover.
//
// Status describes the newest run, including one still in flight; ImageURL and
// Palette describe the current *artwork*, which during a regeneration is still
// the previous run's.
//
// The generation prompt is deliberately not here, on this or on a revision. It
// is an internal implementation detail of how the artwork gets made, kept in
// Postgres for diagnosing provider refusals and never handed to a client.
type CoverResponse struct {
	ID           string                `json:"id"                 binding:"required"`
	Status       domain.CoverStatus    `json:"status"             binding:"required" enum:"pending,analyzing,generating,ready,failed"`
	Platform     domain.DSPPlatform    `json:"platform"           binding:"required" enum:"spotify,apple_music,youtube_music"`
	PlaylistID   string                `json:"playlistId"         binding:"required"`
	PlaylistName string                `json:"playlistName"       binding:"required"`
	ImageURL     string                `json:"imageUrl,omitempty"`
	Palette      []ColorWeightResponse `json:"palette,omitempty"`
	// Error carries the failure reason for status="failed" so the client can
	// explain what went wrong instead of showing a dead tile.
	Error string `json:"error,omitempty"`
	// RunCount is how many successful generations this playlist has had, which
	// is what the tile shows and what Revisions defaults to listing.
	RunCount int `json:"runCount"  binding:"required"`
	// Revisions is the run history, newest first. Only the detail endpoint
	// fills it in; the list and the event stream leave it out.
	Revisions []CoverRevisionResponse `json:"revisions,omitempty"`
	CreatedAt string                  `json:"createdAt" binding:"required"`
	// UpdatedAt is both the keyset cursor for the next page and the identity of
	// the newest run: every stage of a generation stamps it, so a terminal
	// value names one run and a replayed snapshot of it repeats that value
	// exactly.
	UpdatedAt string `json:"updatedAt" binding:"required"`
}

// CoverRevisionResponse is one finished generation run. Rows exist only for runs
// that reached a terminal status, so completedAt is the finish time.
type CoverRevisionResponse struct {
	ID string `json:"id"                 binding:"required"`
	// Number is what the client labels the run: "Revision #28". Stable for the
	// life of the run, and with gaps where runs have been deleted, so it is safe
	// to put in a URL or read out to someone.
	Number      int                   `json:"number"             binding:"required"`
	Status      domain.CoverStatus    `json:"status"             binding:"required" enum:"ready,failed"`
	ImageURL    string                `json:"imageUrl,omitempty"`
	Palette     []ColorWeightResponse `json:"palette,omitempty"`
	Error       string                `json:"error,omitempty"`
	CompletedAt string                `json:"completedAt"        binding:"required"`
}

// NewPlaylistList maps domain playlists to their API representation.
func NewPlaylistList(playlists []domain.Playlist) []PlaylistResponse {
	out := make([]PlaylistResponse, 0, len(playlists))
	for _, p := range playlists {
		out = append(out, PlaylistResponse{
			ID:          p.ID,
			Platform:    p.Platform,
			Name:        p.Name,
			Description: p.Description,
			TrackCount:  p.TrackCount,
			ImageURL:    p.ImageURL,
		})
	}
	return out
}

// timeFormat is RFC 3339, which is what JavaScript's Date parses and what the
// keyset cursor is echoed back in.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// newPalette maps a stored analysis's palette to the wire shape.
func newPalette(analysis domain.PlaylistAnalysis) []ColorWeightResponse {
	palette := make([]ColorWeightResponse, 0, len(analysis.Palette))
	for _, cw := range analysis.Palette {
		palette = append(palette, ColorWeightResponse{
			Dimension: cw.Dimension,
			HexColor:  cw.HexColor,
			Weight:    cw.Weight,
		})
	}
	return palette
}

// NewCoverResponse maps a domain cover to its API representation.
func NewCoverResponse(c *domain.Cover) CoverResponse {
	revisions := make([]CoverRevisionResponse, 0, len(c.Revisions))
	for _, r := range c.Revisions {
		revisions = append(revisions, CoverRevisionResponse{
			ID:          r.ID,
			Number:      r.Number,
			Status:      r.Status,
			ImageURL:    r.ImageURL,
			Palette:     newPalette(r.Analysis),
			Error:       r.Error,
			CompletedAt: r.CompletedAt.Format(timeFormat),
		})
	}
	if len(revisions) == 0 {
		// omitempty on a nil slice, so a list response and an event stream do
		// not each carry an empty array per cover.
		revisions = nil
	}

	return CoverResponse{
		ID:           c.ID,
		Status:       c.Status,
		Platform:     c.Platform,
		PlaylistID:   c.PlaylistID,
		PlaylistName: c.PlaylistName,
		ImageURL:     c.ImageURL,
		Palette:      newPalette(c.Analysis),
		Error:        c.Error,
		RunCount:     c.RunCount,
		Revisions:    revisions,
		CreatedAt:    c.CreatedAt.Format(timeFormat),
		UpdatedAt:    c.UpdatedAt.Format(timeFormat),
	}
}

// NewCoverList maps a slice of domain covers to their API representation.
func NewCoverList(covers []domain.Cover) []CoverResponse {
	out := make([]CoverResponse, 0, len(covers))
	for i := range covers {
		out = append(out, NewCoverResponse(&covers[i]))
	}
	return out
}
