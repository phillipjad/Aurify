package google

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// idToken builds an ID token with the given claims. The signature is deliberate
// nonsense: this flow takes the token straight from the token endpoint over an
// authenticated TLS channel and does not verify the signature, so a test that
// signed one would be asserting a check we do not (and need not) make. See the
// comment on Exchange.
func idToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".not-verified"
}

func validClaims(nonce string) map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            "test-client-id",
		"sub":            "google-sub-123",
		"email":          "person@example.com",
		"email_verified": true,
		"name":           "A Person",
		"nonce":          nonce,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	}
}

// newProvider wires a provider at a stub token endpoint. The stub records the
// form it was posted so tests can assert on the exchange itself.
func newProvider(t *testing.T, claims func(form url.Values) map[string]any) (*Provider, *url.Values) {
	t.Helper()

	posted := &url.Values{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		*posted = r.PostForm

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "provider-access-token",
			"token_type":   "Bearer",
			"id_token":     idToken(t, claims(r.PostForm)),
		})
	}))
	t.Cleanup(server.Close)

	p, err := NewProvider(Config{
		ClientID:      "test-client-id",
		ClientSecret:  "test-client-secret",
		RedirectURL:   "https://aurify.test/api/v1/auth/google/callback",
		TokenEndpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return p, posted
}

func TestNewAuthRequestBuildsAPKCEAuthorizationURL(t *testing.T) {
	p, _ := newProvider(t, validClaims2)

	req, err := p.NewAuthRequest()
	if err != nil {
		t.Fatalf("new auth request: %v", err)
	}

	parsed, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := parsed.Query()

	for field, want := range map[string]string{
		"client_id":             "test-client-id",
		"redirect_uri":          "https://aurify.test/api/v1/auth/google/callback",
		"response_type":         "code",
		"code_challenge_method": "S256",
		"state":                 req.State,
		"nonce":                 req.Nonce,
	} {
		if got := q.Get(field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if q.Get("scope") == "" {
		t.Error("no scope requested")
	}

	// The challenge must be the S256 hash of the verifier, never the verifier
	// itself: sending the plain value would let anyone who intercepts the
	// authorization request replay the code.
	sum := sha256.Sum256([]byte(req.CodeVerifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if got := q.Get("code_challenge"); got != want {
		t.Errorf("code_challenge = %q, want the S256 digest %q", got, want)
	}
	if q.Get("code_challenge") == req.CodeVerifier {
		t.Error("the verifier was sent as the challenge")
	}
}

// State, nonce and verifier are per-attempt secrets. Reusing any of them across
// requests would defeat the thing it protects.
func TestNewAuthRequestIsUniquePerCall(t *testing.T) {
	p, _ := newProvider(t, validClaims2)

	first, err := p.NewAuthRequest()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := p.NewAuthRequest()
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first.State == second.State {
		t.Error("state was reused")
	}
	if first.Nonce == second.Nonce {
		t.Error("nonce was reused")
	}
	if first.CodeVerifier == second.CodeVerifier {
		t.Error("code verifier was reused")
	}
}

func TestExchangeSendsTheVerifierAndClientCredentials(t *testing.T) {
	p, posted := newProvider(t, validClaims2)

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "the-verifier"); err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if got := posted.Get("code"); got != "auth-code" {
		t.Errorf("code = %q, want %q", got, "auth-code")
	}
	// Without the verifier the token endpoint cannot bind the code to the client
	// that started the flow, which is the entire point of PKCE.
	if got := posted.Get("code_verifier"); got != "the-verifier" {
		t.Errorf("code_verifier = %q, want %q", got, "the-verifier")
	}
	if got := posted.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q, want %q", got, "authorization_code")
	}
	if got := posted.Get("client_secret"); got != "test-client-secret" {
		t.Errorf("client_secret = %q, want it sent", got)
	}
}

func TestExchangeAcceptsAWellFormedToken(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any { return validClaims("the-nonce") })

	claims, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "the-verifier")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if claims.Subject != "google-sub-123" {
		t.Errorf("subject = %q, want %q", claims.Subject, "google-sub-123")
	}
	if claims.Email != "person@example.com" {
		t.Errorf("email = %q, want %q", claims.Email, "person@example.com")
	}
	if !claims.EmailVerified {
		t.Error("email_verified was not carried through")
	}
	if claims.Name != "A Person" {
		t.Errorf("name = %q, want %q", claims.Name, "A Person")
	}
}

// A mismatched nonce means this ID token was minted for a different
// authorization request, so it is a replay.
func TestExchangeRejectsMismatchedNonce(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any { return validClaims("some-other-nonce") })

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "v"); !errors.Is(err, ErrClaims) {
		t.Fatalf("err = %v, want ErrClaims", err)
	}
}

// A token minted for another client must not authenticate anyone here, even
// though Google signed it.
func TestExchangeRejectsWrongAudience(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any {
		c := validClaims("the-nonce")
		c["aud"] = "someone-elses-client-id"
		return c
	})

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "v"); !errors.Is(err, ErrClaims) {
		t.Fatalf("err = %v, want ErrClaims", err)
	}
}

func TestExchangeRejectsWrongIssuer(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any {
		c := validClaims("the-nonce")
		c["iss"] = "https://evil.example.com"
		return c
	})

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "v"); !errors.Is(err, ErrClaims) {
		t.Fatalf("err = %v, want ErrClaims", err)
	}
}

func TestExchangeRejectsExpiredToken(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any {
		c := validClaims("the-nonce")
		c["exp"] = time.Now().Add(-2 * time.Hour).Unix()
		return c
	})

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "v"); !errors.Is(err, ErrClaims) {
		t.Fatalf("err = %v, want ErrClaims", err)
	}
}

// A token with no expiry is a permanent credential, so it is refused even when
// everything else about it checks out.
func TestExchangeRejectsTokenWithoutExpiry(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any {
		c := validClaims("the-nonce")
		delete(c, "exp")
		return c
	})

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "v"); !errors.Is(err, ErrClaims) {
		t.Fatalf("err = %v, want ErrClaims", err)
	}
}

func TestExchangeRejectsMissingSubject(t *testing.T) {
	p, _ := newProvider(t, func(url.Values) map[string]any {
		c := validClaims("the-nonce")
		delete(c, "sub")
		return c
	})

	if _, err := p.Exchange(t.Context(), "auth-code", "the-nonce", "v"); !errors.Is(err, ErrClaims) {
		t.Fatalf("err = %v, want ErrClaims", err)
	}
}

func TestProviderIsDisabledWithoutCredentials(t *testing.T) {
	p, err := NewProvider(Config{})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if p.Enabled() {
		t.Fatal("a provider with no client id reports itself enabled")
	}
	if _, err := p.NewAuthRequest(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func validClaims2(url.Values) map[string]any { return validClaims("the-nonce") }
