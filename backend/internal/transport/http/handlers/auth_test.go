package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/fgrzl/claims"
	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command"
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

const testAppBaseURL = "http://localhost:5173"

// --- fakes ---

// fakeProvider stands in for a DSP. Its exchanges counter is the spy that says
// whether a callback got far enough to link an account.
type fakeProvider struct {
	exchanges int
	err       error
}

var _ ports.DSPProvider = (*fakeProvider)(nil)

func (p *fakeProvider) Platform() domain.DSPPlatform { return domain.PlatformYouTubeMusic }

func (p *fakeProvider) AuthURL(state string) string {
	return "https://provider.test/authorize?state=" + url.QueryEscape(state)
}

func (p *fakeProvider) Exchange(_ context.Context, _ string) (domain.DSPConnection, error) {
	p.exchanges++
	if p.err != nil {
		return domain.DSPConnection{}, p.err
	}
	return domain.DSPConnection{Platform: domain.PlatformYouTubeMusic, AccessToken: "tok"}, nil
}

func (p *fakeProvider) RefreshConnection(
	_ context.Context,
	conn domain.DSPConnection,
) (domain.DSPConnection, bool, error) {
	return conn, false, nil
}

func (p *fakeProvider) ListPlaylists(context.Context, domain.DSPConnection) ([]domain.Playlist, error) {
	return nil, nil
}

func (p *fakeProvider) ListTracks(context.Context, domain.DSPConnection, string) ([]domain.Track, error) {
	return nil, nil
}

type fakeRegistry struct{ provider ports.DSPProvider }

func (r fakeRegistry) Get(platform domain.DSPPlatform) (ports.DSPProvider, error) {
	if platform != domain.PlatformYouTubeMusic {
		return nil, fmt.Errorf("%w: %q", domain.ErrUnsupportedPlatform, platform)
	}
	return r.provider, nil
}

func (r fakeRegistry) Platforms() []domain.DSPPlatform {
	return []domain.DSPPlatform{domain.PlatformYouTubeMusic}
}

type fakeUsers struct{ user *domain.User }

var _ ports.UserRepository = (*fakeUsers)(nil)

func (u *fakeUsers) Save(_ context.Context, user *domain.User) error {
	u.user = user
	return nil
}
func (u *fakeUsers) Create(_ context.Context, user *domain.User) error {
	return u.Save(context.TODO(), user)
}
func (u *fakeUsers) FindByID(context.Context, string) (*domain.User, error) {
	return u.user, nil
}
func (u *fakeUsers) FindByEmail(context.Context, string) (*domain.User, error) {
	return u.user, nil
}

// --- harness ---

// newAuthTest wires the DSP routes onto a bare router. The authentication
// middleware is replaced by one that plants the given subject, so these tests
// exercise the flow rather than token verification. An empty userID means the
// caller is anonymous.
func newAuthTest(t *testing.T, userID string) (*mux.Router, *fakeProvider) {
	t.Helper()

	provider := &fakeProvider{}
	users := &fakeUsers{user: &domain.User{ID: "user-1", Email: "a@b.test"}}
	application := &app.App{
		Commands: &command.Bus{
			ConnectDSP: connectdsp.NewHandler(users, fakeRegistry{provider: provider}),
		},
	}

	// Secure=false: these requests are plain HTTP, and a __Host- cookie would be
	// dropped by the recorder round trip the same way a browser would drop it.
	h := NewAuth(application, fakeRegistry{provider: provider}, NewCookieWriter(false), testAppBaseURL)

	router := mux.NewRouter()
	router.Use(mux.MiddlewareFunc(func(c mux.MutableRouteContext, next mux.HandlerFunc) {
		if userID != "" {
			c.SetUser(claims.NewPrincipal(claims.NewClaimsSet(userID)))
		}
		next(c)
	}))
	err := router.Configure(func(r *mux.Router) {
		api := r.Group("/api/v1")
		api.GET("/auth/{platform}/login", h.Login).AllowAnonymous()
		api.GET("/auth/{platform}/callback", h.Callback).AllowAnonymous()
	})
	if err != nil {
		t.Fatalf("configure router: %v", err)
	}
	return router, provider
}

// startFlow runs the login leg and returns the state the provider was sent and
// the flow cookie the browser would hold.
func startFlow(t *testing.T, router *mux.Router, platform string) (string, *http.Cookie) {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/"+platform+"/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want %d", rec.Code, http.StatusFound)
	}
	authURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	state := authURL.Query().Get("state")
	if state == "" {
		t.Fatal("login sent the provider no state")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login set no flow cookie")
	}
	return state, cookies[0]
}

func callback(router *mux.Router, platform, state, code string, cookie *http.Cookie) *httptest.ResponseRecorder {
	target := fmt.Sprintf("/api/v1/auth/%s/callback?code=%s&state=%s", platform, code, url.QueryEscape(state))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// --- tests ---

func TestDSPLoginRedirectsWithAFreshRandomState(t *testing.T) {
	router, _ := newAuthTest(t, "user-1")

	first, cookie := startFlow(t, router, "youtube_music")
	second, _ := startFlow(t, router, "youtube_music")

	// The state used to be the platform name: constant, public, and therefore no
	// defence at all against a forged callback.
	if first == "youtube_music" || len(first) < 32 {
		t.Fatalf("state = %q, want a long random value", first)
	}
	if first == second {
		t.Fatal("two flows reused the same state")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("flow cookie = %+v, want HttpOnly and SameSite=Lax", cookie)
	}
}

// This harness plants the principal itself, so it cannot see route
// configuration; the real router's authentication is covered in
// transport/http/router_test.go. What it does pin is that the handler refuses
// to start a flow it could not attribute to anybody.
func TestDSPLoginRefusesWithoutAPrincipal(t *testing.T) {
	router, _ := newAuthTest(t, "")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/youtube_music/login", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDSPCallbackLinksTheAccount(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")
	state, cookie := startFlow(t, router, "youtube_music")

	rec := callback(router, "youtube_music", state, "the-code", cookie)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	want := testAppBaseURL + "/playlists?connected=youtube_music"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	if provider.exchanges != 1 {
		t.Fatalf("provider exchanges = %d, want 1", provider.exchanges)
	}
}

// The check that makes the state parameter worth having: a callback carrying a
// state this browser never issued must not link anything.
func TestDSPCallbackRejectsAMismatchedState(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")
	_, cookie := startFlow(t, router, "youtube_music")

	rec := callback(router, "youtube_music", "forged-state", "the-code", cookie)

	assertConnectError(t, rec, "youtube_music", "state")
	if provider.exchanges != 0 {
		t.Fatalf("provider exchanges = %d, want 0: a forged callback reached the exchange", provider.exchanges)
	}
}

// A flow started for one platform must not complete as another, or a user
// consenting to YouTube Music could have the result filed under Spotify.
func TestDSPCallbackRejectsAnotherPlatformsFlow(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")
	state, cookie := startFlow(t, router, "youtube_music")

	rec := callback(router, "spotify", state, "the-code", cookie)

	assertConnectError(t, rec, "spotify", "state")
	if provider.exchanges != 0 {
		t.Fatalf("provider exchanges = %d, want 0", provider.exchanges)
	}
}

func TestDSPCallbackWithoutAFlowCookieIsExpired(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")

	rec := callback(router, "youtube_music", "some-state", "the-code", nil)

	assertConnectError(t, rec, "youtube_music", "expired")
	if provider.exchanges != 0 {
		t.Fatalf("provider exchanges = %d, want 0", provider.exchanges)
	}
}

// A user who declines consent is a normal outcome, reported as its own reason so
// the UI can say something kinder than "that failed".
func TestDSPCallbackReportsDeclinedConsent(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")
	_, cookie := startFlow(t, router, "youtube_music")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/youtube_music/callback?error=access_denied", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertConnectError(t, rec, "youtube_music", "cancelled")
	if provider.exchanges != 0 {
		t.Fatalf("provider exchanges = %d, want 0", provider.exchanges)
	}
}

// assertConnectError also pins the platform on the redirect: the UI attributes
// the failure from it, so a reason that travels without one is how a failed
// YouTube Music attempt ends up reported against Spotify.
func assertConnectError(t *testing.T, rec *httptest.ResponseRecorder, platform, reason string) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	want := testAppBaseURL + "/playlists?connect_error=" + reason + "&platform=" + platform
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}
