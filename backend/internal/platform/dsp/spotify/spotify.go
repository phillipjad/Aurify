// Package spotify implements the Spotify DSP provider.
//
// SCAFFOLD: the OAuth exchange and Web API calls are stubbed. The intended
// implementation is documented inline:
//   - Exchange:      POST https://accounts.spotify.com/api/token
//   - ListPlaylists: GET  https://api.spotify.com/v1/me/playlists
//   - ListTracks:    GET  https://api.spotify.com/v1/playlists/{id}/items
//
// There is no audio-features call to make. Spotify deprecated /v1/audio-features
// and /v1/audio-analysis on 2024-11-27 and they answer 403 for any client
// registered since; there is no replacement. Tracks therefore normalize with
// domain.AudioFeatures.Present == false, exactly as YouTube Music does, and the
// pipeline leans on lyric sentiment (see docs/adr/0005).
//
// The February 2026 Web API revision also renamed /v1/playlists/{id}/tracks to
// /v1/playlists/{id}/items, removed /v1/users/{id}/playlists, and trimmed fields
// off the user object. Check the changelog before implementing rather than
// trusting the endpoint names above:
// https://developer.spotify.com/documentation/web-api/references/changes
package spotify

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

const authBaseURL = "https://accounts.spotify.com/authorize"

// scopes are the OAuth scopes Aurify needs to read playlists and library data.
// TODO: re-check these against the February 2026 revision, which consolidated
// the library endpoints; the scope names may have moved with them.
var scopes = []string{"playlist-read-private", "playlist-read-collaborative", "user-library-read"}

var errNotImplemented = errors.New("spotify: not implemented in scaffold")

// Provider is the Spotify implementation of ports.DSPProvider.
type Provider struct {
	cfg    dsp.OAuthConfig
	client *http.Client
}

var _ ports.DSPProvider = (*Provider)(nil)

// New constructs a Spotify provider.
func New(cfg dsp.OAuthConfig) *Provider {
	return &Provider{cfg: cfg, client: http.DefaultClient}
}

// Platform returns the platform identifier.
func (p *Provider) Platform() domain.DSPPlatform { return domain.PlatformSpotify }

// AuthURL builds the Spotify authorization-code OAuth URL.
func (p *Provider) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", p.cfg.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", p.cfg.RedirectURL)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("state", state)
	return authBaseURL + "?" + q.Encode()
}

// Exchange swaps an authorization code for tokens. TODO: implement.
func (p *Provider) Exchange(ctx context.Context, code string) (domain.DSPConnection, error) {
	_ = ctx
	_ = code
	return domain.DSPConnection{}, errNotImplemented
}

// ListPlaylists returns the user's playlists. TODO: implement.
func (p *Provider) ListPlaylists(ctx context.Context, conn domain.DSPConnection) ([]domain.Playlist, error) {
	_ = ctx
	_ = conn
	return nil, errNotImplemented
}

// ListTracks returns the normalized tracks of a playlist. TODO: implement.
func (p *Provider) ListTracks(
	ctx context.Context,
	conn domain.DSPConnection,
	playlistID string,
) ([]domain.Track, error) {
	_ = ctx
	_ = conn
	_ = playlistID
	return nil, errNotImplemented
}
