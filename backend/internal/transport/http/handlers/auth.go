package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// connectReturnPath is where both legs of the DSP flow send the browser. The
// connect button only exists on this page, so there is nothing to round-trip.
const connectReturnPath = "/playlists"

// Auth handles the DSP OAuth login + callback flow.
//
// Both legs are top-level navigations, mirroring Federated: the browser is
// looking at whatever these return, so they answer with redirects rather than
// JSON. The account link they establish is not a login — see the comment on
// Federated and docs/adr/0011-authentication-and-sessions.md.
type Auth struct {
	app        *app.App
	providers  ports.DSPRegistry
	cookies    *CookieWriter
	appBaseURL string
}

// NewAuth constructs the auth handler. It needs the provider registry directly
// because building the authorization-redirect URL is a transport concern, not a
// domain use case.
func NewAuth(a *app.App, providers ports.DSPRegistry, cookies *CookieWriter, appBaseURL string) *Auth {
	return &Auth{app: a, providers: providers, cookies: cookies, appBaseURL: appBaseURL}
}

// Login redirects to the provider's OAuth authorization page.
// GET /api/v1/auth/{platform}/login
func (h *Auth) Login(c mux.RouteContext) {
	platform, ok := c.Params().String("platform")
	if !ok {
		c.BadRequest("missing platform", "path parameter 'platform' is required")
		return
	}

	provider, err := h.providers.Get(domain.DSPPlatform(platform))
	if err != nil {
		respondError(c, err)
		return
	}

	// Belt and braces: the authentication middleware has already rejected an
	// anonymous caller, so this only fires if the route is ever marked
	// AllowAnonymous, which silently disables that middleware (see router.go).
	if currentUser(c) == "" {
		c.Unauthorized()
		return
	}

	// A random state, kept server-side in the flow cookie and compared against
	// what the provider echoes back. It replaced the platform name, which was
	// constant and public, so any page could forge a callback that linked an
	// attacker's DSP account to whoever was signed in.
	state, err := randomState()
	if err != nil {
		c.ServerError("internal error", err.Error())
		return
	}

	secure := h.cookies.Secure()
	if err := setFlowState(c, dspFlowCookieName(secure), flowState{
		State:    state,
		Platform: platform,
		Expires:  time.Now().Add(flowTTL).Unix(),
	}, secure); err != nil {
		c.ServerError("internal error", err.Error())
		return
	}

	c.Redirect(http.StatusFound, provider.AuthURL(state))
}

// Callback completes the OAuth flow and links the DSP account to the user.
// GET /api/v1/auth/{platform}/callback?code=...&state=...
//
// Every outcome is a redirect back to the playlists page carrying either
// connected=<platform> or a coarse connect_error code.
func (h *Auth) Callback(c mux.RouteContext) {
	secure := h.cookies.Secure()

	platform, ok := c.Params().String("platform")
	if !ok {
		c.BadRequest("missing platform", "path parameter 'platform' is required")
		return
	}

	// How a provider reports a user who declined consent. A normal outcome.
	if providerErr, ok := c.Query().String("error"); ok && providerErr != "" {
		h.fail(c, secure, platform, "cancelled")
		return
	}

	flow, err := readFlowState(c, dspFlowCookieName(secure))
	if err != nil {
		// Usually a bookmarked callback, a double submit, or a user who sat on the
		// consent screen for longer than the window.
		h.fail(c, secure, platform, "expired")
		return
	}

	code, _ := c.Query().String("code")
	echoedState, _ := c.Query().String("state")

	// A mismatch on either means this callback does not belong to a flow this
	// browser started for this platform, which is forged rather than accidental.
	if !matchesState(flow.State, echoedState) || flow.Platform != platform || code == "" {
		h.fail(c, secure, platform, "state")
		return
	}

	// ponytail: the link is attributed to whoever the session cookie names, so an
	// access token that expires during consent (a 15 minute TTL against a 10
	// minute flow window) loses the attempt and the user retries. Signing the user
	// id into the flow cookie is the upgrade path if that turns out to bite.
	userID := currentUser(c)
	if userID == "" {
		h.fail(c, secure, platform, "session")
		return
	}

	if err := h.app.Commands.ConnectDSP.Handle(c, connectdsp.Command{
		UserID:   userID,
		Platform: domain.DSPPlatform(platform),
		Code:     code,
	}); err != nil {
		slog.Error("dsp connect failed", "error", err, "platform", platform)
		h.fail(c, secure, platform, "exchange")
		return
	}

	clearFlowState(c, dspFlowCookieName(secure), secure)
	h.redirect(c, connectReturnPath+"?connected="+url.QueryEscape(platform))
}

// fail clears the half-finished flow and sends the browser back with a reason.
//
// The platform travels with the reason so the UI can attribute the failure to
// the account the user actually tried to link. Without it the page has to guess
// from whatever it happens to have selected, which is how a failed YouTube Music
// attempt used to report itself against Spotify.
func (h *Auth) fail(c mux.RouteContext, secure bool, platform, reason string) {
	clearFlowState(c, dspFlowCookieName(secure), secure)
	h.redirect(c, fmt.Sprintf(
		"%s?connect_error=%s&platform=%s",
		connectReturnPath, url.QueryEscape(reason), url.QueryEscape(platform),
	))
}

// redirect confines the destination to this app's own origin.
func (h *Auth) redirect(c mux.RouteContext, path string) {
	c.Redirect(http.StatusFound, safeRedirect(h.appBaseURL, path))
}
