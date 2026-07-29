package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fgrzl/mux"
)

// oauthFlowCookie carries the per-attempt secrets of a federated sign-in across
// the redirect to the provider and back. dspFlowCookie does the same for a DSP
// account link.
//
// They are deliberately separate cookies. The two flows look alike on the wire
// and are not the same concern — one establishes who the user is, the other
// grants access to a music library — so a half-finished sign-in must not be
// able to satisfy a DSP callback, or the reverse.
//
// Both use the __Host- prefix, which the browser enforces: the cookie must be
// Secure, must have Path=/ and must carry no Domain, which pins it to exactly
// this origin. That matters more here than the narrow path would, because a
// subdomain able to overwrite this cookie could plant its own state and nonce
// and complete a flow the user never started — logging the victim into an
// account the attacker controls.
const (
	oauthFlowCookie = "__Host-aurify_oauth"
	dspFlowCookie   = "__Host-aurify_dsp"
)

// flowTTL bounds how long a half-finished flow stays completable. It only has
// to cover a trip through the provider's consent screen.
const flowTTL = 10 * time.Minute

// errFlowState covers a missing, malformed or expired flow cookie.
var errFlowState = errors.New("handlers: oauth flow state is missing or invalid")

// flowState is what the cookie holds between the authorization redirect and the
// callback.
type flowState struct {
	// State is compared against the state query parameter the provider echoes
	// back, which is what ties the callback to a flow this browser started.
	State string `json:"s"`
	// Nonce is compared against the nonce claim inside the ID token, which is
	// what stops a token minted for another flow being replayed into this one.
	Nonce string `json:"n"`
	// Verifier is the PKCE code verifier, held back until the token exchange.
	Verifier string `json:"v"`
	// Platform names the DSP a connect flow was started for. The callback checks
	// it against the platform in its own path, so a flow started for one provider
	// cannot be completed as another.
	Platform string `json:"p,omitempty"`
	// Expires is a unix timestamp. The cookie's own Max-Age already bounds this,
	// but a client controls its cookie jar and we do not, so the deadline is
	// checked server-side too.
	Expires int64 `json:"e"`
	// Return is where to send the browser once the flow completes.
	Return string `json:"r,omitempty"`
}

func encodeFlowState(f flowState) (string, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeFlowState parses and expiry-checks the cookie value.
//
// The encoding is not authenticated, and does not need to be: the __Host- prefix
// means only this origin can have written the cookie, and every secret inside it
// is compared against something the provider supplies. A forged value fails the
// state comparison, and a tampered verifier fails PKCE at Google's token
// endpoint.
//
// Only State and Expires are required here, because they are the two fields
// every flow has. Nonce and Verifier belong to the OIDC sign-in alone; that
// callback requires them itself.
func decodeFlowState(encoded string) (flowState, error) {
	if encoded == "" {
		return flowState{}, errFlowState
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return flowState{}, errFlowState
	}
	var f flowState
	if err := json.Unmarshal(raw, &f); err != nil {
		return flowState{}, errFlowState
	}
	if f.State == "" {
		return flowState{}, errFlowState
	}
	if f.Expires == 0 || time.Now().After(time.Unix(f.Expires, 0)) {
		return flowState{}, errFlowState
	}
	return f, nil
}

// randomState returns a 256-bit URL-safe random value for use as an OAuth state
// parameter.
func randomState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("handlers: read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// setFlowState writes the named flow cookie.
func setFlowState(c mux.RouteContext, name string, f flowState, secure bool) error {
	encoded, err := encodeFlowState(f)
	if err != nil {
		return err
	}
	// SameSite must be Lax, not Strict. The callback arrives as a cross-site
	// top-level navigation from the provider, and a Strict cookie is not sent on
	// one — the state, nonce and verifier would simply be missing.
	c.Cookies().Set(
		name, encoded,
		int(flowTTL.Seconds()),
		"/", "", secure, true, http.SameSiteLaxMode,
	)
	return nil
}

// clearFlowState removes the named flow cookie. It is called on both the success
// and failure paths so a spent or abandoned attempt cannot be retried.
func clearFlowState(c mux.RouteContext, name string, secure bool) {
	c.Cookies().Set(name, "", -1, "/", "", secure, true, http.SameSiteLaxMode)
}

func readFlowState(c mux.RouteContext, name string) (flowState, error) {
	v, err := c.Cookies().Get(name)
	if err != nil {
		return flowState{}, errFlowState
	}
	return decodeFlowState(v)
}

// flowCookieName and dspFlowCookieName drop the __Host- prefix for local HTTP
// development, where a browser would reject a prefixed cookie that is not marked
// Secure and the flow would never complete.
func flowCookieName(secure bool) string {
	if secure {
		return oauthFlowCookie
	}
	return "aurify_oauth"
}

func dspFlowCookieName(secure bool) string {
	if secure {
		return dspFlowCookie
	}
	return "aurify_dsp"
}

// matchesState compares the echoed state against the stored one in constant
// time. Two empty values deliberately do not match: a missing state must never
// satisfy the check.
func matchesState(stored, echoed string) bool {
	if stored == "" || echoed == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(echoed)) == 1
}

// safeRedirect resolves a post-sign-in destination against the app's own origin,
// falling back to the app root for anything that would leave it.
//
// The callback hands control back to the browser at a moment when it has just
// been given session cookies, so an unchecked destination here is an open
// redirect with a freshly authenticated victim attached.
func safeRedirect(appBaseURL, target string) string {
	root := strings.TrimRight(appBaseURL, "/") + "/"
	if target == "" {
		return root
	}
	// Reject anything that could name a different host: an absolute URL, a
	// scheme-relative "//host" and a backslash variant browsers normalise to one.
	if strings.Contains(target, ":") || strings.HasPrefix(target, "//") || strings.HasPrefix(target, `\\`) {
		return root
	}
	if !strings.HasPrefix(target, "/") {
		return root
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return root
	}
	return strings.TrimRight(appBaseURL, "/") + parsed.String()
}
