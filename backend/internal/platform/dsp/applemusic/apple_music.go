// Package applemusic implements the Apple Music DSP provider.
//
// SCAFFOLD: Apple Music uses MusicKit with a developer JWT plus a user "Music
// User Token" rather than a standard OAuth code exchange, so AuthURL/Exchange
// will look different from the other providers. All methods are stubbed.
package applemusic

import (
	"context"
	"errors"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

var errNotImplemented = errors.New("applemusic: not implemented in scaffold")

// Provider is the Apple Music implementation of ports.DSPProvider.
type Provider struct {
	cfg dsp.OAuthConfig
}

var _ ports.DSPProvider = (*Provider)(nil)

// New constructs an Apple Music provider.
func New(cfg dsp.OAuthConfig) *Provider { return &Provider{cfg: cfg} }

// Platform returns the platform identifier.
func (p *Provider) Platform() domain.DSPPlatform { return domain.PlatformAppleMusic }

// AuthURL would point at the MusicKit authorization flow. TODO: implement.
func (p *Provider) AuthURL(state string) string {
	_ = state
	return ""
}

// Exchange validates the MusicKit user token. TODO: implement.
func (p *Provider) Exchange(ctx context.Context, code string) (domain.DSPConnection, error) {
	_ = ctx
	_ = code
	return domain.DSPConnection{}, errNotImplemented
}

// RefreshConnection has nothing to do here: a Music User Token cannot be renewed
// without the user, so an expired one is re-obtained through MusicKit in the
// browser rather than refreshed server-side.
func (p *Provider) RefreshConnection(
	ctx context.Context,
	conn domain.DSPConnection,
) (domain.DSPConnection, bool, error) {
	_ = ctx
	return conn, false, nil
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
