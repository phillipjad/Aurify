package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app/sessions"
)

// Cookie names.
//
// The prefixes are enforced by the browser, not by us:
//   - __Host- requires Secure, Path=/ and no Domain, which pins the cookie to
//     exactly this origin. A subdomain cannot overwrite it, so it is the right
//     choice for the credential sent on every request.
//   - __Secure- only requires Secure, which is what the refresh cookie needs
//     because it is deliberately scoped to a narrow Path and __Host- forbids
//     that.
//
// The CSRF cookie carries no prefix and is intentionally readable by JavaScript:
// the browser has to hand its value back in a header for the double-submit
// check to mean anything.
const (
	accessCookie  = "__Host-aurify_at"
	refreshCookie = "__Secure-aurify_rt"
	csrfCookie    = "aurify_csrf"

	// refreshCookiePath keeps the long-lived credential off every ordinary API
	// request, so it is only ever exposed on the routes that actually use it.
	refreshCookiePath = "/api/v1/auth"

	csrfHeader = "X-CSRF-Token"
)

// devCookiePrefixNote: when CookieSecure is false (local HTTP development) the
// __Host- and __Secure- prefixes are dropped, because a browser rejects a
// prefixed cookie that is not marked Secure and the user would simply never be
// signed in.
func cookieName(name string, secure bool) string {
	if secure {
		return name
	}
	switch name {
	case accessCookie:
		return "aurify_at"
	case refreshCookie:
		return "aurify_rt"
	default:
		return name
	}
}

// CookieWriter issues and clears the auth cookie set.
type CookieWriter struct {
	secure bool
}

// NewCookieWriter builds a CookieWriter. secure should only be false for local
// HTTP development.
func NewCookieWriter(secure bool) *CookieWriter {
	return &CookieWriter{secure: secure}
}

// Issue writes the access, refresh and CSRF cookies for a new token pair and
// returns the CSRF token so the response body can carry it too.
//
// SameSite is Lax rather than Strict throughout. Strict would drop these
// cookies on the cross-site top-level navigation that returns from a federated
// sign-in, which silently breaks the Google flow added in the next phase.
func (w *CookieWriter) Issue(c mux.RouteContext, tokens sessions.Tokens) (string, error) {
	now := time.Now()

	c.Cookies().Set(
		cookieName(accessCookie, w.secure), tokens.AccessToken,
		maxAge(tokens.AccessExpiresAt, now),
		"/", "", w.secure, true, http.SameSiteLaxMode,
	)
	c.Cookies().Set(
		cookieName(refreshCookie, w.secure), tokens.RefreshToken,
		maxAge(tokens.RefreshExpiresAt, now),
		refreshCookiePath, "", w.secure, true, http.SameSiteLaxMode,
	)

	csrf, err := newCSRFToken()
	if err != nil {
		return "", err
	}
	// httpOnly is false on purpose: the SPA must read this value to echo it in
	// the X-CSRF-Token header. It is not a secret in the way the session is,
	// since an attacker on another origin cannot read it.
	c.Cookies().Set(
		csrfCookie, csrf,
		maxAge(tokens.RefreshExpiresAt, now),
		"/", "", w.secure, false, http.SameSiteLaxMode,
	)
	return csrf, nil
}

// Clear removes every auth cookie. Paths must match the ones used to set them,
// or the browser keeps the original cookie alongside the deletion.
func (w *CookieWriter) Clear(c mux.RouteContext) {
	c.Cookies().Set(cookieName(accessCookie, w.secure), "", -1, "/", "", w.secure, true, http.SameSiteLaxMode)
	c.Cookies().Set(cookieName(refreshCookie, w.secure), "", -1, refreshCookiePath, "", w.secure, true, http.SameSiteLaxMode)
	c.Cookies().Set(csrfCookie, "", -1, "/", "", w.secure, false, http.SameSiteLaxMode)
}

// AccessToken reads the access token cookie.
func (w *CookieWriter) AccessToken(c mux.RouteContext) string {
	v, _ := c.Cookies().Get(cookieName(accessCookie, w.secure))
	return v
}

// RefreshToken reads the refresh token cookie.
func (w *CookieWriter) RefreshToken(c mux.RouteContext) string {
	v, _ := c.Cookies().Get(cookieName(refreshCookie, w.secure))
	return v
}

// CheckCSRF validates the double-submit pair on a state-changing request.
//
// The defence works because an attacker on another origin can cause the browser
// to *send* our cookies but cannot read them, so they cannot populate the
// matching header. Comparison is constant-time to avoid leaking the expected
// value a byte at a time.
func CheckCSRF(c mux.RouteContext) bool {
	cookie, err := c.Cookies().Get(csrfCookie)
	if err != nil || cookie == "" {
		return false
	}
	header := c.Request().Header.Get(csrfHeader)
	if header == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) == 1
}

func newCSRFToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// maxAge converts an absolute expiry into a cookie Max-Age, never returning
// zero: a zero Max-Age would make it a session cookie that outlives the token.
func maxAge(expiresAt, now time.Time) int {
	seconds := int(expiresAt.Sub(now).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
