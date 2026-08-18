// Package youtubemusic implements the YouTube Music DSP provider on top of the
// official YouTube Data API v3.
//
// There is no first-party YouTube Music API; the Data API's standard OAuth 2.0
// authorization-code web flow is the supported path and maps directly onto
// ports.DSPProvider. YouTube Music exposes no audio-features endpoint, so every
// track is normalized with domain.AudioFeatures.Present == false and the
// pipeline leans on lyric sentiment instead (see docs/adr/0005 and 0009).
package youtubemusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

const (
	defaultAPIBaseURL    = "https://www.googleapis.com/youtube/v3"
	youtubeReadonlyScope = "https://www.googleapis.com/auth/youtube.readonly"
	// maxPageSize is the YouTube Data API's per-call maximum for list endpoints.
	maxPageSize = 50
	// likedMusicPlaylistID is YouTube's well-known id for the auto-generated
	// "Liked Music" playlist.
	//
	// It is reachable but not discoverable: playlists.list?mine=true does not
	// return it, and on accounts whose playlists were created inside YouTube
	// Music that call answers totalResults=1 with an empty items array, so the
	// listing comes back empty even though there is music to read. Looking the id
	// up directly resolves the playlist, and playlistItems reads it normally.
	likedMusicPlaylistID = "LM"
)

// Provider is the YouTube Music implementation of ports.DSPProvider.
type Provider struct {
	cfg        dsp.OAuthConfig
	httpClient *http.Client
	// apiBaseURL and endpoint default to the real Google services and are only
	// overridden in tests (white-box, same package).
	apiBaseURL string
	endpoint   oauth2.Endpoint
}

var _ ports.DSPProvider = (*Provider)(nil)

// NewProvider constructs a YouTube Music provider.
func NewProvider(cfg dsp.OAuthConfig) *Provider {
	return &Provider{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBaseURL: defaultAPIBaseURL,
		endpoint:   google.Endpoint,
	}
}

// Platform returns the platform identifier.
func (p *Provider) Platform() domain.DSPPlatform { return domain.PlatformYouTubeMusic }

func (p *Provider) oauthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		RedirectURL:  p.cfg.RedirectURL,
		Scopes:       []string{youtubeReadonlyScope},
		Endpoint:     p.endpoint,
	}
}

// withHTTPClient makes oauth2 — and the authenticated API calls it wraps — use
// the provider's *http.Client. This is the seam tests use to point the provider
// at an httptest server.
func (p *Provider) withHTTPClient(ctx context.Context) context.Context {
	if p.httpClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, p.httpClient)
}

// AuthURL builds the Google OAuth authorization URL. access_type=offline plus
// prompt=consent ensure a refresh token is issued.
func (p *Provider) AuthURL(state string) string {
	return p.oauthConfig().AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
}

// Exchange swaps an authorization code for tokens and resolves the user's
// channel id as the provider identity.
func (p *Provider) Exchange(ctx context.Context, code string) (domain.DSPConnection, error) {
	ctx = p.withHTTPClient(ctx)
	cfg := p.oauthConfig()

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return domain.DSPConnection{}, fmt.Errorf("youtubemusic: token exchange: %w", err)
	}

	conn := domain.DSPConnection{
		Platform:     domain.PlatformYouTubeMusic,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.Expiry,
		Scopes:       cfg.Scopes,
	}

	// Identity is best-effort: a lookup failure must not fail an otherwise
	// successful account link.
	if id, err := p.fetchChannelID(ctx, cfg.Client(ctx, tok)); err == nil {
		conn.ProviderUserID = id
	}

	return conn, nil
}

// RefreshConnection renews the access token when it has expired, returning the
// updated connection so the caller can store it.
//
// oauth2's token source refreshes on demand during a call and keeps the result
// to itself, which left the stored token permanently stale. Forcing the refresh
// here, before the API calls, means what the database holds is what the next
// request will use.
func (p *Provider) RefreshConnection(
	ctx context.Context,
	conn domain.DSPConnection,
) (domain.DSPConnection, bool, error) {
	// Without a refresh token there is nothing to renew with; the connection has
	// to be re-authorized by the user instead.
	if conn.RefreshToken == "" {
		return conn, false, nil
	}

	ctx = p.withHTTPClient(ctx)
	tok, err := p.oauthConfig().TokenSource(ctx, &oauth2.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		Expiry:       conn.ExpiresAt,
	}).Token()
	if err != nil {
		// A RetrieveError is Google answering the token endpoint and refusing:
		// invalid_grant for a refresh token revoked, expired, or aged out of a
		// testing-mode project. Nothing retries its way past that, so it is
		// reported as needing re-authorization. A transport failure is not: the
		// grant may be perfectly good and the network merely down, and telling
		// someone to reconnect over a dropped packet costs them their tokens.
		var retrieve *oauth2.RetrieveError
		if errors.As(err, &retrieve) {
			return conn, false, fmt.Errorf(
				"%w: youtubemusic: refresh token: %w", domain.ErrDSPReauthRequired, err,
			)
		}
		return conn, false, fmt.Errorf("youtubemusic: refresh token: %w", err)
	}
	if tok.AccessToken == conn.AccessToken {
		return conn, false, nil
	}

	conn.AccessToken = tok.AccessToken
	// Google only returns a new refresh token when it rotates one; an empty value
	// means keep the one we have, and overwriting it with "" would strand the
	// connection with no way to renew.
	if tok.RefreshToken != "" {
		conn.RefreshToken = tok.RefreshToken
	}
	conn.ExpiresAt = tok.Expiry
	return conn, true, nil
}

// ListPlaylists returns the authenticated user's playlists.
func (p *Provider) ListPlaylists(ctx context.Context, conn domain.DSPConnection) ([]domain.Playlist, error) {
	ctx = p.withHTTPClient(ctx)
	client := p.authedClient(ctx, conn)

	var out []domain.Playlist
	pageToken := ""
	for {
		q := url.Values{}
		q.Set("part", "snippet,contentDetails")
		q.Set("mine", "true")
		q.Set("maxResults", strconv.Itoa(maxPageSize))
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}

		var resp playlistListResponse
		if err := p.getJSON(ctx, client, "/playlists", q, &resp); err != nil {
			return nil, err
		}
		for _, item := range resp.Items {
			out = append(out, mapPlaylist(item))
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	// Liked Music has to be asked for by id; see likedMusicPlaylistID. Best
	// effort, like the identity lookup in Exchange: an account with nothing
	// liked, or a lookup that fails, must not empty out the rest of the list.
	if liked, err := p.fetchPlaylistByID(ctx, client, likedMusicPlaylistID); err == nil {
		out = append(out, liked)
	}

	return out, nil
}

// fetchPlaylistByID resolves a single playlist by its id.
func (p *Provider) fetchPlaylistByID(
	ctx context.Context,
	client *http.Client,
	id string,
) (domain.Playlist, error) {
	q := url.Values{}
	q.Set("part", "snippet,contentDetails")
	q.Set("id", id)

	var resp playlistListResponse
	if err := p.getJSON(ctx, client, "/playlists", q, &resp); err != nil {
		return domain.Playlist{}, err
	}
	if len(resp.Items) == 0 {
		return domain.Playlist{}, fmt.Errorf("youtubemusic: playlist %q not found", id)
	}
	return mapPlaylist(resp.Items[0]), nil
}

// GetPlaylist reads a single playlist by id.
func (p *Provider) GetPlaylist(
	ctx context.Context,
	conn domain.DSPConnection,
	playlistID string,
) (domain.Playlist, error) {
	ctx = p.withHTTPClient(ctx)
	return p.fetchPlaylistByID(ctx, p.authedClient(ctx, conn), playlistID)
}

// ListTracks returns the normalized tracks of a playlist. Durations require a
// second videos.list call since playlistItems.list does not expose them.
func (p *Provider) ListTracks(
	ctx context.Context,
	conn domain.DSPConnection,
	playlistID string,
) ([]domain.Track, error) {
	ctx = p.withHTTPClient(ctx)
	client := p.authedClient(ctx, conn)

	// 1. Page through the playlist's items.
	var items []playlistItemResource
	pageToken := ""
	for {
		q := url.Values{}
		q.Set("part", "snippet,contentDetails")
		q.Set("playlistId", playlistID)
		q.Set("maxResults", strconv.Itoa(maxPageSize))
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}

		var resp playlistItemsResponse
		if err := p.getJSON(ctx, client, "/playlistItems", q, &resp); err != nil {
			return nil, err
		}
		items = append(items, resp.Items...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	// 2. Collect playable video ids (private/deleted items carry no owner).
	var ids []string
	for _, item := range items {
		if playableVideoID(item) != "" {
			ids = append(ids, item.ContentDetails.VideoID)
		}
	}

	// 3. Durations from videos.list (batched 50 ids per call).
	durations, err := p.fetchDurations(ctx, client, ids)
	if err != nil {
		return nil, err
	}

	// 4. Normalize, skipping the unplayable items.
	tracks := make([]domain.Track, 0, len(ids))
	for _, item := range items {
		if playableVideoID(item) == "" {
			continue
		}
		tracks = append(tracks, mapTrack(item, durations[item.ContentDetails.VideoID]))
	}
	return tracks, nil
}

// authedClient returns an *http.Client that attaches conn's token and refreshes
// it transparently when expired.
func (p *Provider) authedClient(ctx context.Context, conn domain.DSPConnection) *http.Client {
	tok := &oauth2.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		Expiry:       conn.ExpiresAt,
	}
	return p.oauthConfig().Client(ctx, tok)
}

func (p *Provider) fetchChannelID(ctx context.Context, client *http.Client) (string, error) {
	q := url.Values{}
	q.Set("part", "id")
	q.Set("mine", "true")

	var resp channelListResponse
	if err := p.getJSON(ctx, client, "/channels", q, &resp); err != nil {
		return "", err
	}
	if len(resp.Items) == 0 {
		return "", fmt.Errorf("youtubemusic: no channel for authenticated user")
	}
	return resp.Items[0].ID, nil
}

func (p *Provider) fetchDurations(ctx context.Context, client *http.Client, ids []string) (map[string]int, error) {
	durations := make(map[string]int, len(ids))
	for start := 0; start < len(ids); start += maxPageSize {
		end := start + maxPageSize
		if end > len(ids) {
			end = len(ids)
		}

		q := url.Values{}
		q.Set("part", "contentDetails")
		q.Set("id", strings.Join(ids[start:end], ","))
		q.Set("maxResults", strconv.Itoa(maxPageSize))

		var resp videosResponse
		if err := p.getJSON(ctx, client, "/videos", q, &resp); err != nil {
			return nil, err
		}
		for _, v := range resp.Items {
			if ms, err := parseISO8601Duration(v.ContentDetails.Duration); err == nil {
				durations[v.ID] = ms
			}
		}
	}
	return durations, nil
}

func (p *Provider) getJSON(ctx context.Context, client *http.Client, path string, q url.Values, out any) error {
	endpoint := p.apiBaseURL + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("youtubemusic: GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("youtubemusic: GET %s: unexpected status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("youtubemusic: decode %s: %w", path, err)
	}
	return nil
}

// --- normalization ---

func mapPlaylist(p playlistResource) domain.Playlist {
	return domain.Playlist{
		ID:          p.ID,
		Platform:    domain.PlatformYouTubeMusic,
		Name:        p.Snippet.Title,
		Description: p.Snippet.Description,
		TrackCount:  p.ContentDetails.ItemCount,
		ImageURL:    bestThumbnail(p.Snippet.Thumbnails),
	}
}

func mapTrack(item playlistItemResource, durationMS int) domain.Track {
	channel := item.Snippet.VideoOwnerChannelTitle
	// Art Tracks come from auto-generated "<Artist> - Topic" channels, which
	// yields a clean artist; other uploads fall back to the channel title.
	artist := strings.TrimSpace(strings.TrimSuffix(channel, " - Topic"))

	var artists []string
	if artist != "" {
		artists = []string{artist}
	}

	return domain.Track{
		ID:         item.ContentDetails.VideoID,
		Platform:   domain.PlatformYouTubeMusic,
		Title:      item.Snippet.Title,
		Artists:    artists,
		DurationMS: durationMS,
		// No first-party audio features on YouTube Music (see ADR 0005).
		Features: domain.AudioFeatures{Present: false},
	}
}

// playableVideoID returns the video id for items Aurify can analyze, or "" for
// private/deleted entries (which have no owner channel and never resolve in
// videos.list).
func playableVideoID(item playlistItemResource) string {
	if item.ContentDetails.VideoID == "" || item.Snippet.VideoOwnerChannelTitle == "" {
		return ""
	}
	return item.ContentDetails.VideoID
}

func bestThumbnail(thumbs map[string]thumbnail) string {
	for _, size := range []string{"maxres", "standard", "high", "medium", "default"} {
		if t, ok := thumbs[size]; ok && t.URL != "" {
			return t.URL
		}
	}
	return ""
}

var iso8601DurationRE = regexp.MustCompile(`^P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// parseISO8601Duration converts a YouTube duration (e.g. "PT4M33S") to
// milliseconds. It returns an error for an empty or malformed value.
func parseISO8601Duration(s string) (int, error) {
	m := iso8601DurationRE.FindStringSubmatch(s)
	if s == "" || m == nil {
		return 0, fmt.Errorf("youtubemusic: invalid ISO-8601 duration %q", s)
	}
	atoi := func(v string) int {
		n, _ := strconv.Atoi(v)
		return n
	}
	weeks, days := atoi(m[1]), atoi(m[2])
	hours, mins, secs := atoi(m[3]), atoi(m[4]), atoi(m[5])
	total := ((((weeks*7+days)*24+hours)*60+mins)*60 + secs)
	return total * 1000, nil
}

// --- YouTube Data API response shapes (only the fields Aurify uses) ---

type thumbnail struct {
	URL string `json:"url"`
}

type playlistResource struct {
	ID      string `json:"id"`
	Snippet struct {
		Title       string               `json:"title"`
		Description string               `json:"description"`
		Thumbnails  map[string]thumbnail `json:"thumbnails"`
	} `json:"snippet"`
	ContentDetails struct {
		ItemCount int `json:"itemCount"`
	} `json:"contentDetails"`
}

type playlistListResponse struct {
	NextPageToken string             `json:"nextPageToken"`
	Items         []playlistResource `json:"items"`
}

type playlistItemResource struct {
	Snippet struct {
		Title                  string `json:"title"`
		VideoOwnerChannelTitle string `json:"videoOwnerChannelTitle"`
	} `json:"snippet"`
	ContentDetails struct {
		VideoID string `json:"videoId"`
	} `json:"contentDetails"`
}

type playlistItemsResponse struct {
	NextPageToken string                 `json:"nextPageToken"`
	Items         []playlistItemResource `json:"items"`
}

type videoResource struct {
	ID             string `json:"id"`
	ContentDetails struct {
		Duration string `json:"duration"`
	} `json:"contentDetails"`
}

type videosResponse struct {
	Items []videoResource `json:"items"`
}

type channelListResponse struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}
