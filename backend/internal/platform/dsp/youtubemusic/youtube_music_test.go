package youtubemusic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

func TestParseISO8601Duration(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"PT3M20S", 200000, false},
		{"PT1H2M10S", 3730000, false},
		{"PT45S", 45000, false},
		{"PT0S", 0, false},
		{"P1DT1S", 86401000, false},
		{"", 0, true},
		{"garbage", 0, true},
	}
	for _, c := range cases {
		got, err := parseISO8601Duration(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseISO8601Duration(%q): expected error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseISO8601Duration(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseISO8601Duration(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMapTrack(t *testing.T) {
	// YouTube Music "Art Track": the owner channel is "<Artist> - Topic" and the
	// title is the clean song name.
	var topic playlistItemResource
	topic.Snippet.Title = "Yellow"
	topic.Snippet.VideoOwnerChannelTitle = "Coldplay - Topic"
	topic.ContentDetails.VideoID = "v1"

	got := mapTrack(topic, 273000)
	if got.Title != "Yellow" {
		t.Errorf("title = %q, want Yellow", got.Title)
	}
	if len(got.Artists) != 1 || got.Artists[0] != "Coldplay" {
		t.Errorf("artists = %v, want [Coldplay]", got.Artists)
	}
	if got.ID != "v1" || got.DurationMS != 273000 {
		t.Errorf("id/dur = %q/%d, want v1/273000", got.ID, got.DurationMS)
	}
	if got.Platform != domain.PlatformYouTubeMusic {
		t.Errorf("platform = %q, want youtube_music", got.Platform)
	}
	if got.Features.Present {
		t.Error("Features.Present must be false for YouTube Music (no audio-features endpoint)")
	}
	if got.Album != "" || got.ISRC != "" {
		t.Errorf("album/isrc should be empty, got %q/%q", got.Album, got.ISRC)
	}

	// A regular (non-Topic) channel: keep the channel as the artist and leave the
	// title untouched.
	var regular playlistItemResource
	regular.Snippet.Title = "Never Gonna Give You Up"
	regular.Snippet.VideoOwnerChannelTitle = "Rick Astley"
	regular.ContentDetails.VideoID = "v3"

	got2 := mapTrack(regular, 213000)
	if len(got2.Artists) != 1 || got2.Artists[0] != "Rick Astley" {
		t.Errorf("artists = %v, want [Rick Astley]", got2.Artists)
	}
	if got2.Title != "Never Gonna Give You Up" {
		t.Errorf("title changed: %q", got2.Title)
	}
}

func TestMapPlaylist(t *testing.T) {
	var p playlistResource
	p.ID = "PL1"
	p.Snippet.Title = "Chill"
	p.Snippet.Description = "easy listening"
	p.Snippet.Thumbnails = map[string]thumbnail{
		"medium": {URL: "http://img/med.jpg"},
		"high":   {URL: "http://img/high.jpg"},
	}
	p.ContentDetails.ItemCount = 12

	got := mapPlaylist(p)
	if got.ID != "PL1" || got.Name != "Chill" || got.Description != "easy listening" {
		t.Errorf("unexpected playlist: %+v", got)
	}
	if got.TrackCount != 12 {
		t.Errorf("trackcount = %d, want 12", got.TrackCount)
	}
	if got.ImageURL != "http://img/high.jpg" {
		t.Errorf("imageurl = %q, want the high thumbnail", got.ImageURL)
	}
	if got.Platform != domain.PlatformYouTubeMusic {
		t.Errorf("platform = %q, want youtube_music", got.Platform)
	}
}

func TestAuthURL(t *testing.T) {
	p := NewProvider(dsp.OAuthConfig{
		ClientID:    "cid",
		RedirectURL: "http://localhost:8080/api/v1/auth/youtube_music/callback",
	})
	u, err := url.Parse(p.AuthURL("xyz-state"))
	if err != nil {
		t.Fatalf("AuthURL is not a valid URL: %v", err)
	}
	if u.Host != "accounts.google.com" {
		t.Errorf("host = %q, want accounts.google.com", u.Host)
	}
	q := u.Query()
	want := map[string]string{
		"client_id":     "cid",
		"response_type": "code",
		"redirect_uri":  "http://localhost:8080/api/v1/auth/youtube_music/callback",
		"state":         "xyz-state",
		"access_type":   "offline",
		"prompt":        "consent",
	}
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("query %q = %q, want %q", k, got, v)
		}
	}
	if !strings.Contains(q.Get("scope"), "youtube.readonly") {
		t.Errorf("scope = %q, want it to contain youtube.readonly", q.Get("scope"))
	}
}

func TestExchange(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	p := testProvider(srv)

	conn, err := p.Exchange(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if conn.AccessToken != "at" || conn.RefreshToken != "rt" {
		t.Errorf("tokens = %q/%q, want at/rt", conn.AccessToken, conn.RefreshToken)
	}
	if conn.ProviderUserID != "UC_abc" {
		t.Errorf("providerUserID = %q, want UC_abc (from channels.list)", conn.ProviderUserID)
	}
	if conn.Platform != domain.PlatformYouTubeMusic {
		t.Errorf("platform = %q, want youtube_music", conn.Platform)
	}
	if conn.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be derived from expires_in")
	}
	if len(conn.Scopes) != 1 || !strings.Contains(conn.Scopes[0], "youtube.readonly") {
		t.Errorf("scopes = %v, want [youtube.readonly]", conn.Scopes)
	}
}

func TestListPlaylists(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	p := testProvider(srv)

	got, err := p.ListPlaylists(context.Background(), domain.DSPConnection{AccessToken: "at"})
	if err != nil {
		t.Fatalf("ListPlaylists: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d playlists, want 2 (second page must be followed)", len(got))
	}
	if got[0].ID != "PL1" || got[0].Name != "Chill" || got[0].TrackCount != 2 {
		t.Errorf("playlist[0] = %+v", got[0])
	}
	if got[0].ImageURL != "http://img/high.jpg" {
		t.Errorf("playlist[0].ImageURL = %q, want high thumbnail", got[0].ImageURL)
	}
	if got[1].ID != "PL2" {
		t.Errorf("playlist[1].ID = %q, want PL2", got[1].ID)
	}
}

func TestListTracks(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	p := testProvider(srv)

	got, err := p.ListTracks(context.Background(), domain.DSPConnection{AccessToken: "at"}, "PL1")
	if err != nil {
		t.Fatalf("ListTracks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tracks, want 2 (the private video must be skipped)", len(got))
	}

	byID := make(map[string]domain.Track, len(got))
	for _, tr := range got {
		byID[tr.ID] = tr
	}

	v1, ok := byID["v1"]
	if !ok {
		t.Fatal("missing track v1")
	}
	if v1.Title != "Yellow" || len(v1.Artists) != 1 || v1.Artists[0] != "Coldplay" {
		t.Errorf("v1 = %+v", v1)
	}
	if v1.DurationMS != 273000 {
		t.Errorf("v1.DurationMS = %d, want 273000 (from videos.list)", v1.DurationMS)
	}
	if _, exists := byID["v2"]; exists {
		t.Error("private video v2 should have been skipped")
	}
	v3 := byID["v3"]
	if len(v3.Artists) != 1 || v3.Artists[0] != "Rick Astley" || v3.DurationMS != 213000 {
		t.Errorf("v3 = %+v", v3)
	}
	for _, tr := range got {
		if tr.Features.Present {
			t.Errorf("track %s must have Features.Present=false", tr.ID)
		}
	}
}

// --- test helpers ---

// newTestServer stands in for Google's OAuth token endpoint and the YouTube
// Data API. Pagination is driven by the pageToken query parameter.
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"token_type":"Bearer"}`))
	})

	mux.HandleFunc("/channels", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":"UC_abc","snippet":{"title":"My Channel"}}]}`))
	})

	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write(
				[]byte(
					`{"nextPageToken":"P2","items":[{"id":"PL1","snippet":{"title":"Chill","description":"d","thumbnails":{"high":{"url":"http://img/high.jpg"},"default":{"url":"http://img/def.jpg"}}},"contentDetails":{"itemCount":2}}]}`,
				),
			)
			return
		}
		_, _ = w.Write(
			[]byte(
				`{"items":[{"id":"PL2","snippet":{"title":"Focus","thumbnails":{"medium":{"url":"http://img/med.jpg"}}},"contentDetails":{"itemCount":0}}]}`,
			),
		)
	})

	mux.HandleFunc("/playlistItems", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			// First item is a real Art Track; second is a private video (empty
			// owner channel) that must be filtered out.
			_, _ = w.Write(
				[]byte(
					`{"nextPageToken":"PI2","items":[{"snippet":{"title":"Yellow","videoOwnerChannelTitle":"Coldplay - Topic"},"contentDetails":{"videoId":"v1"}},{"snippet":{"title":"Private video","videoOwnerChannelTitle":""},"contentDetails":{"videoId":"v2"}}]}`,
				),
			)
			return
		}
		_, _ = w.Write(
			[]byte(
				`{"items":[{"snippet":{"title":"Never Gonna Give You Up","videoOwnerChannelTitle":"Rick Astley"},"contentDetails":{"videoId":"v3"}}]}`,
			),
		)
	})

	mux.HandleFunc("/videos", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(
			[]byte(
				`{"items":[{"id":"v1","contentDetails":{"duration":"PT4M33S"}},{"id":"v3","contentDetails":{"duration":"PT3M33S"}}]}`,
			),
		)
	})

	return httptest.NewServer(mux)
}

func testProvider(srv *httptest.Server) *Provider {
	p := NewProvider(dsp.OAuthConfig{
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURL:  "http://localhost/cb",
	})
	p.apiBaseURL = srv.URL
	p.httpClient = srv.Client()
	p.endpoint = oauth2.Endpoint{
		AuthURL:   srv.URL + "/auth",
		TokenURL:  srv.URL + "/token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	return p
}
