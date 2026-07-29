package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// testFlowKey stands in for the key derived from the deployment's auth seed.
var testFlowKey = DeriveFlowKey([]byte("test-seed"))

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
	h := NewAuth(application, fakeRegistry{provider: provider}, NewCookieWriter(false), testAppBaseURL, testFlowKey)

	router := mux.NewRouter()
	router.Use(mux.MiddlewareFunc(func(c mux.MutableRouteContext, next mux.HandlerFunc) {
		if userID != "" {
			c.SetUser(claims.NewPrincipal(claims.NewClaimsSet(userID)))
		}
		next(c)
	}))
	err := router.Configure(func(r *mux.Router) {
		api := r.Group("/api/v1")
		// login is authenticated in the real router; this harness plants the
		// principal itself, so AllowAnonymous here only stops mux rejecting the
		// request before the handler runs. The callback is anonymous for real: it
		// authenticates itself from the signed flow cookie.
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
// The callback deliberately does not consult the session: consent can outlast a
// fifteen minute access token, and requiring one threw away an authorization the
// user had just granted. The signed cookie is what identifies them.
func TestDSPCallbackCompletesWithNoLiveSession(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")
	state, cookie := startFlow(t, router, "youtube_music")

	// A second router with no principal at all stands in for a session that
	// expired while the user was on the consent screen.
	anonymous, provider2 := newAuthTest(t, "")
	_ = provider2
	rec := callback(anonymous, "youtube_music", state, "the-code", cookie)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	want := testAppBaseURL + "/playlists?connected=youtube_music"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	if provider.exchanges != 0 {
		t.Fatalf("the first router should not have exchanged anything, got %d", provider.exchanges)
	}
	if provider2.exchanges != 1 {
		t.Fatalf("exchanges on the anonymous router = %d, want 1", provider2.exchanges)
	}
}

// The user id is the one value in the cookie that nothing external corroborates,
// so it has to be tamper-evident: without the signature a user could rewrite it
// and file their DSP account against somebody else's account.
func TestDSPCallbackRejectsATamperedUserID(t *testing.T) {
	router, provider := newAuthTest(t, "user-1")
	state, cookie := startFlow(t, router, "youtube_music")

	payload, _, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		t.Fatal("flow cookie is not signed: expected payload.signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var forged flowState
	if err := json.Unmarshal(raw, &forged); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if forged.UserID != "user-1" {
		t.Fatalf("flow user id = %q, want it recorded at login", forged.UserID)
	}
	forged.UserID = "victim"
	forgedPayload, err := encodeFlowState(forged)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Payload swapped, original signature kept — the MAC no longer matches.
	cookie.Value = forgedPayload + "." + strings.SplitN(cookie.Value, ".", 2)[1]

	rec := callback(router, "youtube_music", state, "the-code", cookie)

	assertConnectError(t, rec, "youtube_music", "expired")
	if provider.exchanges != 0 {
		t.Fatalf("exchanges = %d, want 0: a forged user id reached the exchange", provider.exchanges)
	}
}

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
