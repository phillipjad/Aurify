// Package google implements Sign in with Google as an OpenID Connect
// authorization-code flow with PKCE.
//
// It is deliberately separate from internal/platform/dsp/youtubemusic even
// though both speak OAuth to Google. A DSP connection grants Aurify access to a
// music library; this establishes who the user is. Keeping them apart is what
// stops a data connection from being mistaken for a login
// (see docs/adr/0011-authentication-and-sessions.md).
package google

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Google's OIDC endpoints. They are constants rather than discovery-document
// lookups: a fixed pair cannot be repointed by a compromised or spoofed
// discovery response, and they have been stable for the life of the API.
const (
	defaultAuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenEndpoint = "https://oauth2.googleapis.com/token"
)

// issuer values Google is permitted to claim. Both spellings are legitimate and
// which one appears depends on the endpoint, so both are accepted and nothing
// else is.
var validIssuers = []string{"https://accounts.google.com", "accounts.google.com"}

// scopes are the minimum needed to identify the user: no Drive, no calendar, no
// long-lived data access. A login should ask for a login and nothing more.
const scopes = "openid email profile"

// clockLeeway absorbs small clock differences with Google. It matches the
// leeway used for our own access tokens and is deliberately tight.
const clockLeeway = 60 * time.Second

// exchangeTimeout bounds the token-endpoint call so a hung provider cannot pin
// a request goroutine open indefinitely.
const exchangeTimeout = 10 * time.Second

// Errors returned by this package. The transport layer collapses all of them
// into one generic failure: telling a caller precisely which check rejected
// their token is free reconnaissance.
var (
	// ErrNotConfigured means no Google client credentials are set, so the
	// feature is off.
	ErrNotConfigured = errors.New("google: sign-in is not configured")
	// ErrExchange covers a failed or unreadable token-endpoint call.
	ErrExchange = errors.New("google: code exchange failed")
	// ErrClaims covers an ID token whose claims do not check out.
	ErrClaims = errors.New("google: id token claims are invalid")
)

// Config holds the OAuth client credentials.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// AuthEndpoint and TokenEndpoint default to Google's. They exist so tests
	// can point at a stub, and are not configuration the deployment sets.
	AuthEndpoint  string
	TokenEndpoint string
}

// Claims are the identity assertions taken from a validated ID token.
type Claims struct {
	// Subject is Google's stable id for the account, and the only value the
	// application keys an identity on.
	Subject string
	Email   string
	// EmailVerified is Google's assertion that the user controls the address.
	// Account linking hinges on it.
	EmailVerified bool
	Name          string
}

// AuthRequest is one authorization attempt. State, Nonce and CodeVerifier are
// per-attempt secrets that must survive the redirect to Google and come back
// intact, so the caller has to stash them somewhere the callback can read.
type AuthRequest struct {
	// State is echoed back by Google and compared, which is what ties the
	// callback to a flow this browser actually started (CSRF on the callback).
	State string
	// Nonce is embedded in the ID token by Google and compared, which is what
	// stops an ID token captured from another flow being replayed into this one.
	Nonce string
	// CodeVerifier is held back until the token exchange. Google only ever sees
	// its SHA-256 digest until then, so intercepting the authorization request
	// does not yield enough to redeem the code.
	CodeVerifier string
	URL          string
}

// Provider performs the Google OIDC handshake.
type Provider struct {
	cfg    Config
	client *http.Client
}

// NewProvider constructs a Provider. Empty credentials are not an error: they
// mean the feature is switched off, and Enabled reports that.
func NewProvider(cfg Config) (*Provider, error) {
	if cfg.AuthEndpoint == "" {
		cfg.AuthEndpoint = defaultAuthEndpoint
	}
	if cfg.TokenEndpoint == "" {
		cfg.TokenEndpoint = defaultTokenEndpoint
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: exchangeTimeout}}, nil
}

// Enabled reports whether Google sign-in is configured. A nil Provider counts as
// disabled, so a caller that was wired without one degrades to a 501 rather than
// panicking on the first request.
func (p *Provider) Enabled() bool {
	return p != nil && p.cfg.ClientID != "" && p.cfg.ClientSecret != "" && p.cfg.RedirectURL != ""
}

// NewAuthRequest mints the per-attempt secrets and builds the authorization URL.
func (p *Provider) NewAuthRequest() (AuthRequest, error) {
	if !p.Enabled() {
		return AuthRequest{}, ErrNotConfigured
	}

	state, err := randomString()
	if err != nil {
		return AuthRequest{}, err
	}
	nonce, err := randomString()
	if err != nil {
		return AuthRequest{}, err
	}
	verifier, err := randomString()
	if err != nil {
		return AuthRequest{}, err
	}

	digest := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"client_id":             {p.cfg.ClientID},
		"redirect_uri":          {p.cfg.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {scopes},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(digest[:])},
		"code_challenge_method": {"S256"},
	}
	return AuthRequest{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		URL:          p.cfg.AuthEndpoint + "?" + q.Encode(),
	}, nil
}

// Exchange redeems an authorization code and returns the identity Google
// asserts, having checked the ID token's claims against this flow.
//
// The ID token's signature is deliberately not verified, and this is the one
// place in Aurify where a JWT is trusted without one. The token arrives in the
// body of a direct, client-authenticated TLS POST to Google's token endpoint —
// not via the browser — so the TLS server identity already establishes that
// Google produced it. OIDC Core §3.1.3.7 explicitly permits substituting that
// for a signature check in exactly this flow. The alternative would be a JWKS
// fetch, cache and rotation path plus a second hand-written JWT verifier, whose
// key material would be trusted on the strength of the same TLS connection: more
// code, more to get wrong, no more assurance.
//
// The claims still have to be checked, because TLS says who sent the token, not
// who it was minted for or whether it belongs to this flow.
func (p *Provider) Exchange(ctx context.Context, code, nonce, verifier string) (Claims, error) {
	if !p.Enabled() {
		return Claims{}, ErrNotConfigured
	}

	form := url.Values{
		"code":          {code},
		"client_id":     {p.cfg.ClientID},
		"client_secret": {p.cfg.ClientSecret},
		"redirect_uri":  {p.cfg.RedirectURL},
		"grant_type":    {"authorization_code"},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, p.cfg.TokenEndpoint, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrExchange, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrExchange, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrExchange, err)
	}
	if resp.StatusCode != http.StatusOK {
		return Claims{}, fmt.Errorf("%w: token endpoint returned %d", ErrExchange, resp.StatusCode)
	}

	var payload struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrExchange, err)
	}
	if payload.IDToken == "" {
		return Claims{}, fmt.Errorf("%w: response carried no id_token", ErrExchange)
	}
	return p.claimsFrom(payload.IDToken, nonce)
}

// idTokenClaims is the subset of the ID token Aurify reads. Google sends more;
// anything not needed to identify the user is ignored on purpose.
type idTokenClaims struct {
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Subject  string `json:"sub"`
	Email    string `json:"email"`
	// EmailVerified is json.RawMessage because Google has historically sent it
	// both as a bool and as the strings "true"/"false". Decoding straight into a
	// bool fails on the string form, and a decode failure here would read as
	// "unverified" — which silently blocks legitimate sign-ins.
	EmailVerified json.RawMessage `json:"email_verified"`
	Name          string          `json:"name"`
	Nonce         string          `json:"nonce"`
	Expiry        int64           `json:"exp"`
	IssuedAt      int64           `json:"iat"`
}

// claimsFrom decodes the ID token payload and checks every claim that binds it
// to this client and this flow.
func (p *Provider) claimsFrom(token, nonce string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("%w: token is not a three-part JWT", ErrClaims)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: payload is not base64url", ErrClaims)
	}
	var c idTokenClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return Claims{}, fmt.Errorf("%w: payload is not JSON", ErrClaims)
	}

	if !slices.Contains(validIssuers, c.Issuer) {
		return Claims{}, fmt.Errorf("%w: unexpected issuer %q", ErrClaims, c.Issuer)
	}
	// A token minted for another client is still validly signed by Google. The
	// audience check is what stops one being replayed at us.
	if c.Audience != p.cfg.ClientID {
		return Claims{}, fmt.Errorf("%w: audience is not this client", ErrClaims)
	}
	// A token with no expiry is a permanent credential, so it is refused even
	// though Google would never issue one.
	if c.Expiry == 0 {
		return Claims{}, fmt.Errorf("%w: token has no expiry", ErrClaims)
	}
	now := time.Now()
	if now.After(time.Unix(c.Expiry, 0).Add(clockLeeway)) {
		return Claims{}, fmt.Errorf("%w: token is expired", ErrClaims)
	}
	if c.IssuedAt != 0 && now.Add(clockLeeway).Before(time.Unix(c.IssuedAt, 0)) {
		return Claims{}, fmt.Errorf("%w: token was issued in the future", ErrClaims)
	}
	// The nonce ties this token to the authorization request this browser
	// started. Without it, an ID token captured from any other flow for the same
	// client could be replayed here.
	if nonce == "" || c.Nonce != nonce {
		return Claims{}, fmt.Errorf("%w: nonce does not match this flow", ErrClaims)
	}
	if strings.TrimSpace(c.Subject) == "" {
		return Claims{}, fmt.Errorf("%w: token carries no subject", ErrClaims)
	}

	return Claims{
		Subject:       c.Subject,
		Email:         c.Email,
		EmailVerified: parseFlexibleBool(c.EmailVerified),
		Name:          c.Name,
	}, nil
}

// parseFlexibleBool reads a JSON value that may be a bool or a quoted bool.
// Anything else is false, which is the safe reading: an unrecognised
// email_verified must never be treated as an assertion that it is verified.
func parseFlexibleBool(raw json.RawMessage) bool {
	switch strings.Trim(string(raw), `"`) {
	case "true":
		return true
	default:
		return false
	}
}

// randomString returns a 256-bit URL-safe random value, used for state, nonce
// and the PKCE verifier alike. All three need to be unguessable and none of them
// needs structure.
func randomString() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("google: read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
