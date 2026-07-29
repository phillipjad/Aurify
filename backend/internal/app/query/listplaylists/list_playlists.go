// Package listplaylists reads the playlists a user has on a connected DSP.
package listplaylists

import (
	"context"
	"sort"
	"strings"

	"github.com/phillipjad/aurify/backend/internal/app/dspconn"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// SortKey selects the ordering applied to the playlist page.
type SortKey string

const (
	// SortName orders alphabetically by playlist name (the default).
	SortName SortKey = "name"
	// SortTracks orders by track count, largest first.
	SortTracks SortKey = "tracks"
)

// Query asks for a user's playlists on a specific platform, with an optional
// text filter, an ordering, and pagination. The DSP provider returns the full
// set today (network calls are stubbed); filtering/sorting/paging happen here so
// the client always talks a paginated contract, and a real provider can later
// push these down.
type Query struct {
	UserID   string
	Platform domain.DSPPlatform
	Search   string
	Sort     SortKey
	Limit    int
	Offset   int
}

// Handler executes the ListPlaylists query.
type Handler struct {
	connections *dspconn.Resolver
}

// NewHandler constructs a ListPlaylists handler.
func NewHandler(connections *dspconn.Resolver) *Handler {
	return &Handler{connections: connections}
}

// Handle resolves the user's connection, asks the provider for playlists, then
// applies the search filter, ordering, and pagination.
func (h *Handler) Handle(ctx context.Context, q Query) ([]domain.Playlist, error) {
	provider, conn, err := h.connections.Resolve(ctx, q.UserID, q.Platform)
	if err != nil {
		return nil, err
	}

	playlists, err := provider.ListPlaylists(ctx, conn)
	if err != nil {
		return nil, err
	}

	playlists = filterPlaylists(playlists, q.Search)
	sortPlaylists(playlists, q.Sort)
	return paginate(playlists, q.Limit, q.Offset), nil
}

// filterPlaylists keeps playlists whose name or description contains the search
// term (case-insensitive). An empty term keeps everything.
func filterPlaylists(playlists []domain.Playlist, search string) []domain.Playlist {
	term := strings.TrimSpace(strings.ToLower(search))
	if term == "" {
		return playlists
	}
	out := make([]domain.Playlist, 0, len(playlists))
	for _, p := range playlists {
		if strings.Contains(strings.ToLower(p.Name), term) ||
			strings.Contains(strings.ToLower(p.Description), term) {
			out = append(out, p)
		}
	}
	return out
}

func sortPlaylists(playlists []domain.Playlist, key SortKey) {
	switch key {
	case SortTracks:
		sort.SliceStable(playlists, func(i, j int) bool {
			return playlists[i].TrackCount > playlists[j].TrackCount
		})
	case SortName:
		fallthrough
	default:
		sort.SliceStable(playlists, func(i, j int) bool {
			return strings.ToLower(playlists[i].Name) < strings.ToLower(playlists[j].Name)
		})
	}
}

// paginate clamps the limit to sane bounds and returns the requested window.
func paginate(playlists []domain.Playlist, limit, offset int) []domain.Playlist {
	if limit <= 0 || limit > maxLimit {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(playlists) {
		return []domain.Playlist{}
	}
	end := offset + limit
	if end > len(playlists) {
		end = len(playlists)
	}
	return playlists[offset:end]
}
