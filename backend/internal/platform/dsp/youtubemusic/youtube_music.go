// Package youtubemusic implements the YouTube Music DSP provider.
//
// SCAFFOLD: YouTube Music has no first-party audio-features endpoint, so the
// eventual implementation will leave domain.AudioFeatures.Present false and
// lean more heavily on lyric sentiment. All methods are stubbed.
package youtubemusic

import (
	"context"
	"errors"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

var errNotImplemented = errors.New("youtubemusic: not implemented in scaffold")

// Provider is the YouTube Music implementation of ports.DSPProvider.
type Provider struct {
	cfg dsp.OAuthConfig
}

var _ ports.DSPProvider = (*Provider)(nil)

// New constructs a YouTube Music provider.
func New(cfg dsp.OAuthConfig) *Provider { return &Provider{cfg: cfg} }

// Platform returns the platform identifier.
func (p *Provider) Platform() domain.DSPPlatform { return domain.PlatformYouTubeMusic }

// AuthURL builds the Google OAuth URL. TODO: implement.
func (p *Provider) AuthURL(state string) string {
	_ = state
	return ""
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
func (p *Provider) ListTracks(ctx context.Context, conn domain.DSPConnection, playlistID string) ([]domain.Track, error) {
	_ = ctx
	_ = conn
	_ = playlistID
	return nil, errNotImplemented
}
