package http

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command"
	"github.com/phillipjad/aurify/backend/internal/app/query"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcoverimage"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
	"github.com/phillipjad/aurify/backend/internal/platform/identity/google"
	"github.com/phillipjad/aurify/backend/internal/transport/http/handlers"
)

// fakeDSP is enough of a provider for the router to build an authorization URL.
type fakeDSP struct{}

func (fakeDSP) Platform() domain.DSPPlatform { return domain.PlatformYouTubeMusic }
func (fakeDSP) AuthURL(state string) string  { return "https://provider.test/authorize?state=" + state }
func (fakeDSP) Exchange(context.Context, string) (domain.DSPConnection, error) {
	return domain.DSPConnection{}, nil
}
func (fakeDSP) RefreshConnection(
	_ context.Context,
	conn domain.DSPConnection,
) (domain.DSPConnection, bool, error) {
	return conn, false, nil
}
func (fakeDSP) GetPlaylist(context.Context, domain.DSPConnection, string) (domain.Playlist, error) {
	return domain.Playlist{}, nil
}
func (fakeDSP) ListPlaylists(context.Context, domain.DSPConnection) ([]domain.Playlist, error) {
	return nil, nil
}
func (fakeDSP) ListTracks(context.Context, domain.DSPConnection, string) ([]domain.Track, error) {
	return nil, nil
}

// knownCoverID is the only cover the fake image store knows about.
const knownCoverID = "0f8fad5b-d9cb-469f-a165-70867728950e"

var knownCoverBytes = []byte{0x89, 'P', 'N', 'G', 0x00, 0xFF}

// storedImage is a fake ports.ImageStore holding exactly one image.
type storedImage struct{}

func (storedImage) Put(context.Context, string, domain.GeneratedImage) (string, error) {
	return "", nil
}

func (storedImage) Find(_ context.Context, coverID string) (domain.GeneratedImage, error) {
	if coverID != knownCoverID {
		return domain.GeneratedImage{}, domain.ErrNotFound
	}
	return domain.GeneratedImage{Bytes: knownCoverBytes, ContentType: "image/png"}, nil
}

// newTestRouter builds the real router, so route configuration is under test
// rather than stubbed. It returns the router and a signed access token for
// "user-1". Options mutate the Deps before the router is built.
func newTestRouter(t *testing.T, opts ...func(*Deps)) (http.Handler, string) {
	t.Helper()

	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := auth.NewSigner(key, "aurify", "aurify-api")
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	verifier, err := auth.NewVerifier(key.Public().(ed25519.PublicKey), "aurify", "aurify-api")
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	token, err := signer.Sign("user-1", "session-1", time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	googleProvider, err := google.NewProvider(google.Config{})
	if err != nil {
		t.Fatalf("google provider: %v", err)
	}

	deps := Deps{
		Application: &app.App{
			Commands: &command.Bus{},
			Queries: &query.Bus{
				GetCoverImage: getcoverimage.NewHandler(storedImage{}),
			},
		},
		Providers:    dsp.NewRegistry(fakeDSP{}),
		Version:      "test",
		Verifier:     verifier,
		Cookies:      handlers.NewCookieWriter(false),
		SessionCheck: func(context.Context, string) error { return nil },
		Google:       googleProvider,
		AppBaseURL:   "http://localhost:5173",
		// A stream that is never signalled. Tests that drive the SSE route
		// replace this with one they control.
		WatchCover: func(string) (<-chan struct{}, func()) { return make(chan struct{}), func() {} },
	}
	for _, opt := range opts {
		opt(&deps)
	}
	router, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}
	return router, token
}

// The DSP routes must not be AllowAnonymous. That flag does not merely tolerate
// an anonymous caller: mux skips the authentication middleware entirely, so the
// principal is never populated and the handler cannot tell who is linking the
// account. With it set, this request answered a redirect to the sign-in page
// while holding a perfectly valid session, and the connect flow could never
// complete for anyone.
func TestDSPLoginAuthenticatesTheSessionCookie(t *testing.T) {
	router, token := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/youtube_music/login", nil)
	req.AddCookie(&http.Cookie{Name: handlers.NewCookieWriter(false).AccessCookieName(), Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusFound, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "https://provider.test/authorize") {
		t.Fatalf("Location = %q, want the provider's authorization URL", location)
	}
	if strings.Contains(location, "/sign-in") {
		t.Fatal("a signed-in caller was sent to sign in: the route is not authenticating its cookie")
	}
}

// The same route must still refuse an anonymous caller, since there would be no
// user to attach the connection to.
func TestDSPLoginRejectsAnonymousCallers(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/youtube_music/login", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// The image route must serve an anonymous caller. An <img> tag fetching this
// cross-origin (app on :5173, API on :8080) sends no credentials, so requiring a
// session would mean every cover in the gallery renders as a broken image.
func TestCoverImageServesAnonymousCallers(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/api/v1/covers/"+knownCoverID+"/image", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, knownCoverBytes) {
		t.Errorf("body = %x, want the stored bytes %x", got, knownCoverBytes)
	}
	// Without this the browser guesses, and a sniffed type is what turns a valid
	// image into a download prompt.
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
}

// A cover with no stored image is a 404, not a 500: it is an ordinary state
// while a generation is still running.
func TestCoverImageIsNotFoundWhenAbsent(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/api/v1/covers/11111111-2222-3333-4444-555555555555/image", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

// Browsers always send Accept-Encoding: gzip and the router compresses, so a
// hand-set Content-Length describes the uncompressed body and never matches the
// bytes on the wire. Chrome aborts that with ERR_CONTENT_LENGTH_MISMATCH, and
// every cover in the gallery renders broken; curl hides it, because curl does
// not ask for compression by default.
func TestCoverImageLengthMatchesTheCompressedBody(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+knownCoverID+"/image", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	declared := rec.Header().Get("Content-Length")
	if declared == "" {
		return // Nothing claimed, so nothing to contradict.
	}
	want, err := strconv.Atoi(declared)
	if err != nil {
		t.Fatalf("Content-Length = %q, not a number", declared)
	}
	if got := rec.Body.Len(); got != want {
		t.Errorf("Content-Length says %d but the body is %d bytes (encoding %q)",
			want, got, rec.Header().Get("Content-Encoding"))
	}
}
