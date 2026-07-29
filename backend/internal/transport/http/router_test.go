package http

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command"
	"github.com/phillipjad/aurify/backend/internal/app/query"
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
func (fakeDSP) ListPlaylists(context.Context, domain.DSPConnection) ([]domain.Playlist, error) {
	return nil, nil
}
func (fakeDSP) ListTracks(context.Context, domain.DSPConnection, string) ([]domain.Track, error) {
	return nil, nil
}

// newTestRouter builds the real router, so route configuration is under test
// rather than stubbed. It returns the router and a signed access token for
// "user-1".
func newTestRouter(t *testing.T) (http.Handler, string) {
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

	router, err := NewRouter(Deps{
		Application:  &app.App{Commands: &command.Bus{}, Queries: &query.Bus{}},
		Providers:    dsp.NewRegistry(fakeDSP{}),
		Version:      "test",
		Verifier:     verifier,
		Cookies:      handlers.NewCookieWriter(false),
		SessionCheck: func(context.Context, string) error { return nil },
		Google:       googleProvider,
		AppBaseURL:   "http://localhost:5173",
	})
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
