package youtubemusic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

// The upload is a multipart/related body against a different host from the rest
// of the API, and every part of that is something Google rejects if it is wrong,
// so the test reads the request back apart rather than asserting a 200.
func TestSetPlaylistCover(t *testing.T) {
	var (
		gotPath, gotQuery, gotAuth string
		gotSnippet                 struct {
			Snippet struct {
				PlaylistID string `json:"playlistId"`
				Type       string `json:"type"`
			} `json:"snippet"`
		}
		gotImage       []byte
		gotImageHeader string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/related" {
			t.Errorf("Content-Type = %q, want multipart/related", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		for i := 0; ; i++ {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			body, _ := io.ReadAll(part)
			switch i {
			case 0:
				_ = json.Unmarshal(body, &gotSnippet)
			case 1:
				gotImage = body
				gotImageHeader = part.Header.Get("Content-Type")
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"youtube#playlistImage"}`))
	}))
	defer srv.Close()

	p := NewProvider(dsp.OAuthConfig{ClientID: "cid"})
	p.uploadBaseURL = srv.URL
	p.httpClient = srv.Client()

	err := p.SetPlaylistCover(
		context.Background(),
		domain.DSPConnection{AccessToken: "at"},
		"PL1",
		domain.GeneratedImage{Bytes: []byte("\xff\xd8\xff not really a jpeg"), ContentType: "image/jpeg"},
	)
	if err != nil {
		t.Fatalf("SetPlaylistCover: %v", err)
	}

	if gotPath != "/playlistImages" {
		t.Errorf("path = %q, want /playlistImages", gotPath)
	}
	// uploadType=multipart is what selects the two-part body; without it Google
	// reads the whole payload as raw image bytes.
	for _, want := range []string{"part=snippet", "uploadType=multipart"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query = %q, want it to contain %q", gotQuery, want)
		}
	}
	if gotAuth != "Bearer at" {
		t.Errorf("Authorization = %q, want the connection's token", gotAuth)
	}
	if gotSnippet.Snippet.PlaylistID != "PL1" {
		t.Errorf("snippet.playlistId = %q, want PL1", gotSnippet.Snippet.PlaylistID)
	}
	// The only value the API's discovery document allows.
	if gotSnippet.Snippet.Type != "hero" {
		t.Errorf("snippet.type = %q, want hero", gotSnippet.Snippet.Type)
	}
	if string(gotImage) != "\xff\xd8\xff not really a jpeg" {
		t.Errorf("image bytes did not survive: %q", gotImage)
	}
	if gotImageHeader != "image/jpeg" {
		t.Errorf("image part Content-Type = %q, want image/jpeg", gotImageHeader)
	}
}

// Google's message is the only thing that separates "this playlist cannot carry
// an image" from "you never granted the scope", and both need a person to act,
// so a failure has to carry it rather than flatten it to a status code.
func TestSetPlaylistCoverCarriesTheAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"The request is not properly authorized."}}`))
	}))
	defer srv.Close()

	p := NewProvider(dsp.OAuthConfig{ClientID: "cid"})
	p.uploadBaseURL = srv.URL
	p.httpClient = srv.Client()

	err := p.SetPlaylistCover(
		context.Background(),
		domain.DSPConnection{AccessToken: "at"},
		"PL1",
		domain.GeneratedImage{Bytes: []byte("x"), ContentType: "image/jpeg"},
	)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not properly authorized") {
		t.Errorf("error = %v, want Google's own message carried through", err)
	}
	// 403 is what a grant older than the write scope produces, and it is the
	// user's to fix. Unwrapped it reached the client as a 500 with this JSON in
	// the body.
	if !errors.Is(err, domain.ErrDSPReauthRequired) {
		t.Error("a 403 should ask the user to reconnect, not report a server fault")
	}
}

// A rejection that is not about the grant must not tell the user to reconnect:
// a playlist that cannot carry an image is not fixed by re-authorizing, and
// sending them through an OAuth round trip to find that out is worse than the
// raw error.
func TestSetPlaylistCoverDoesNotBlameTheGrantForEverything(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Playlist not found."}}`))
	}))
	defer srv.Close()

	p := NewProvider(dsp.OAuthConfig{ClientID: "cid"})
	p.uploadBaseURL = srv.URL
	p.httpClient = srv.Client()

	err := p.SetPlaylistCover(
		context.Background(),
		domain.DSPConnection{AccessToken: "at"},
		"PL1",
		domain.GeneratedImage{Bytes: []byte("x"), ContentType: "image/jpeg"},
	)
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, domain.ErrDSPReauthRequired) {
		t.Errorf("a 404 was reported as a reconnect: %v", err)
	}
}

// The ceiling is checked before the upload rather than after a rejection, since
// a cover over it is our bug and not the user's.
func TestSetPlaylistCoverRejectsOversizeLocally(t *testing.T) {
	p := NewProvider(dsp.OAuthConfig{ClientID: "cid"})
	p.uploadBaseURL = "http://127.0.0.1:1" // never reached

	err := p.SetPlaylistCover(
		context.Background(),
		domain.DSPConnection{AccessToken: "at"},
		"PL1",
		domain.GeneratedImage{Bytes: make([]byte, maxCoverBytes+1), ContentType: "image/jpeg"},
	)
	if err == nil || !strings.Contains(err.Error(), "over the") {
		t.Errorf("error = %v, want a local size rejection", err)
	}
}
