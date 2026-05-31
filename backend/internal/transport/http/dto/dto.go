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

// CoverResponse is the API representation of a generated cover.
type CoverResponse struct {
	ID         string                `json:"id"                 binding:"required"`
	Status     domain.CoverStatus    `json:"status"             binding:"required" enum:"pending,analyzing,generating,ready,failed"`
	Platform   domain.DSPPlatform    `json:"platform"           binding:"required" enum:"spotify,apple_music,youtube_music"`
	PlaylistID string                `json:"playlistId"         binding:"required"`
	ImageURL   string                `json:"imageUrl,omitempty"`
	Prompt     string                `json:"prompt,omitempty"`
	Palette    []ColorWeightResponse `json:"palette,omitempty"`
	CreatedAt  string                `json:"createdAt"          binding:"required"`
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

// NewCoverResponse maps a domain cover to its API representation.
func NewCoverResponse(c *domain.Cover) CoverResponse {
	palette := make([]ColorWeightResponse, 0, len(c.Analysis.Palette))
	for _, cw := range c.Analysis.Palette {
		palette = append(palette, ColorWeightResponse{
			Dimension: cw.Dimension,
			HexColor:  cw.HexColor,
			Weight:    cw.Weight,
		})
	}
	return CoverResponse{
		ID:         c.ID,
		Status:     c.Status,
		Platform:   c.Platform,
		PlaylistID: c.PlaylistID,
		ImageURL:   c.ImageURL,
		Prompt:     c.Prompt,
		Palette:    palette,
		CreatedAt:  c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
