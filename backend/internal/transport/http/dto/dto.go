// Package dto defines the JSON request/response shapes for the HTTP API and
// the mapping to/from domain types. Keeping transport DTOs separate from domain
// models lets the wire format evolve independently and keeps JSON tags out of
// the domain.
package dto

import (
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// GenerateCoverRequest is the POST /api/v1/covers body.
type GenerateCoverRequest struct {
	Platform   string `json:"platform"`
	PlaylistID string `json:"playlistId"`
}

// PlaylistResponse is the API representation of a playlist.
type PlaylistResponse struct {
	ID          string `json:"id"`
	Platform    string `json:"platform"`
	Name        string `json:"name"`
	Description string `json:"description"`
	TrackCount  int    `json:"trackCount"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// ColorWeightResponse is one entry of a cover's derived palette.
type ColorWeightResponse struct {
	Dimension string  `json:"dimension"`
	HexColor  string  `json:"hexColor"`
	Weight    float64 `json:"weight"`
}

// CoverResponse is the API representation of a generated cover.
type CoverResponse struct {
	ID         string                `json:"id"`
	Status     string                `json:"status"`
	Platform   string                `json:"platform"`
	PlaylistID string                `json:"playlistId"`
	ImageURL   string                `json:"imageUrl,omitempty"`
	Prompt     string                `json:"prompt,omitempty"`
	Palette    []ColorWeightResponse `json:"palette,omitempty"`
	CreatedAt  string                `json:"createdAt"`
}

// NewPlaylistList maps domain playlists to their API representation.
func NewPlaylistList(playlists []domain.Playlist) []PlaylistResponse {
	out := make([]PlaylistResponse, 0, len(playlists))
	for _, p := range playlists {
		out = append(out, PlaylistResponse{
			ID:          p.ID,
			Platform:    string(p.Platform),
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
		Status:     string(c.Status),
		Platform:   string(c.Platform),
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
