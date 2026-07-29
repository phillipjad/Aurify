package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command/federatedsignin"
	"github.com/phillipjad/aurify/backend/internal/app/lockout"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/identity/google"
	"github.com/phillipjad/aurify/backend/internal/transport/http/dto"
)

// Federated handles Sign in with Google.
//
// It is separate from Auth, which handles the DSP OAuth flow. The two look alike
// on the wire and are emphatically not the same concern: this one establishes
// who the user is, the other grants Aurify access to a music library. Keeping
// them apart is what stops a YouTube Music data connection from implying a login
// (see docs/adr/0011-authentication-and-sessions.md).
type Federated struct {
	app          *app.App
	google       *google.Provider
	cookies      *CookieWriter
	guard        *lockout.Guard
	appBaseURL   string
	supportEmail string
}

// NewFederated constructs the federated sign-in handler.
func NewFederated(
	a *app.App,
	provider *google.Provider,
	cookies *CookieWriter,
	guard *lockout.Guard,
	appBaseURL, supportEmail string,
) *Federated {
	return &Federated{
		app:          a,
		google:       provider,
		cookies:      cookies,
		guard:        guard,
		appBaseURL:   appBaseURL,
		supportEmail: supportEmail,
	}
}

// GoogleStart begins the sign-in flow.
// GET /api/v1/auth/federated/google/start?return=/covers
//
// It answers with a redirect rather than a JSON authorization URL, so the whole
// flow is one top-level navigation the browser drives. That is also why the
// cookies involved are SameSite=Lax: Strict would drop them on the way back.
func (h *Federated) GoogleStart(c mux.RouteContext) {
	if !h.google.Enabled() {
		h.unavailable(c)
		return
	}

	req, err := h.google.NewAuthRequest()
	if err != nil {
		c.ServerError("internal error", err.Error())
		return
	}

	// The return path is stashed server-side in the flow cookie rather than
	// round-tripped through the provider's state parameter, so a caller cannot
	// smuggle a destination back into the callback.
	returnTo, _ := c.Query().String("return")

	if err := setFlowState(c, flowCookieName(h.cookies.Secure()), flowState{
		State:    req.State,
		Nonce:    req.Nonce,
		Verifier: req.CodeVerifier,
		Expires:  time.Now().Add(flowTTL).Unix(),
		Return:   returnTo,
	}, h.cookies.Secure()); err != nil {
		c.ServerError("internal error", err.Error())
		return
	}

	c.Redirect(http.StatusFound, req.URL)
}

// GoogleCallback completes the flow and starts a session.
// GET /api/v1/auth/federated/google/callback?code=...&state=...
//
// Every failure ends in a redirect back to the sign-in page carrying a short
// error code, never a JSON body: this is a top-level navigation, so the user is
// looking at whatever this returns. The codes are deliberately coarse — the
// specific reason a handshake failed is reconnaissance.
func (h *Federated) GoogleCallback(c mux.RouteContext) {
	if !h.google.Enabled() {
		h.unavailable(c)
		return
	}
	secure := h.cookies.Secure()
	ip := clientIP(c.Request())

	// A blocked pair is turned away before the exchange, so a locked-out caller
	// cannot use this route to make us talk to Google on their behalf.
	decision, err := h.guard.Check(c, ip, "")
	if err != nil {
		c.ServerError("internal error", err.Error())
		return
	}
	if decision.Blocked {
		h.fail(c, secure, "blocked")
		return
	}

	// The provider reports a user who declined consent this way. It is a normal
	// outcome, not a failure worth counting.
	if providerErr, ok := c.Query().String("error"); ok && providerErr != "" {
		h.fail(c, secure, "cancelled")
		return
	}

	flow, err := readFlowState(c, flowCookieName(secure))
	// The nonce and verifier are required here rather than in decodeFlowState,
	// which only insists on the fields every flow has. Without them the ID token
	// could not be bound to this attempt, so a cookie missing either is not a
	// sign-in flow this handler can complete.
	if err != nil || flow.Nonce == "" || flow.Verifier == "" {
		// A missing or stale flow cookie is usually a bookmarked callback, a
		// double submit, or a user who took longer than the window. Benign, so it
		// does not count against the lockout.
		h.fail(c, secure, "expired")
		return
	}

	code, _ := c.Query().String("code")
	echoedState, _ := c.Query().String("state")

	// A state mismatch means this callback does not belong to a flow this browser
	// started, which is a forged callback rather than an accident.
	if !matchesState(flow.State, echoedState) || code == "" {
		h.recordFailure(c, ip, secure, "state")
		return
	}

	claims, err := h.google.Exchange(c, code, flow.Nonce, flow.Verifier)
	if err != nil {
		// A rejected exchange or a token whose claims do not check out is a
		// tampered or replayed handshake, so it counts.
		slog.Warn("google sign-in exchange failed", "error", err, "ip", ip)
		h.recordFailure(c, ip, secure, "exchange")
		return
	}

	tokens, err := h.app.Commands.FederatedSignIn.Handle(c, federatedsignin.Command{
		Provider:      domain.ProviderGoogle,
		Subject:       claims.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		DisplayName:   claims.Name,
		UserAgent:     c.Request().UserAgent(),
		IP:            ip,
	})
	if err != nil {
		clearFlowState(c, flowCookieName(secure), secure)
		// These two are the user's own account state, not an attack: they need to
		// be told what to do rather than counted against a lockout.
		switch {
		case errors.Is(err, domain.ErrLinkRequiresVerification):
			h.redirect(c, "/sign-in?error=verify_existing_account")
		case errors.Is(err, domain.ErrEmailNotVerified):
			h.redirect(c, "/sign-in?error=google_email_unverified")
		default:
			slog.Error("google sign-in failed", "error", err)
			h.redirect(c, "/sign-in?error=signin_failed")
		}
		return
	}

	h.guard.Reset(ip, "")
	clearFlowState(c, flowCookieName(secure), secure)
	if _, err := h.cookies.Issue(c, tokens); err != nil {
		c.ServerError("internal error", err.Error())
		return
	}
	// safeRedirect confines the destination to this app's origin. Without it the
	// return parameter would be an open redirect carrying a browser that has just
	// been handed session cookies.
	c.Redirect(http.StatusFound, safeRedirect(h.appBaseURL, flow.Return))
}

// recordFailure counts a failed handshake and redirects.
func (h *Federated) recordFailure(c mux.RouteContext, ip string, secure bool, reason string) {
	decision, err := h.guard.RecordFailure(c, ip, "")
	if err != nil {
		c.ServerError("internal error", err.Error())
		return
	}
	if decision.Blocked {
		h.fail(c, secure, "blocked")
		return
	}
	h.fail(c, secure, reason)
}

// fail clears the half-finished flow and sends the browser back to sign-in.
func (h *Federated) fail(c mux.RouteContext, secure bool, reason string) {
	clearFlowState(c, flowCookieName(secure), secure)
	h.redirect(c, "/sign-in?error="+url.QueryEscape(reason))
}

func (h *Federated) redirect(c mux.RouteContext, path string) {
	c.Redirect(http.StatusFound, safeRedirect(h.appBaseURL, path))
}

// unavailable answers when no Google credentials are configured. It is a 501
// rather than a 404: the route exists, the deployment just has not enabled it.
func (h *Federated) unavailable(c mux.RouteContext) {
	c.JSON(http.StatusNotImplemented, dto.MessageResponse{
		Message: "Google sign-in is not configured on this deployment.",
	})
}
