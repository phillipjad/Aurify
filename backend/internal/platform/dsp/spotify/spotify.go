// Package spotify implements the Spotify DSP provider.
//
// SCAFFOLD: the OAuth exchange and Web API calls are stubbed. The intended
// implementation is documented inline:
//   - Exchange:      POST https://accounts.spotify.com/api/token
//   - ListPlaylists: GET  https://api.spotify.com/v1/me/playlists
//   - ListTracks:    GET  https://api.spotify.com/v1/playlists/{id}/tracks then
//     GET https://api.spotify.com/v1/audio-features, mapping the response onto
//     domain.AudioFeatures (this is the canonical example feature source).
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
