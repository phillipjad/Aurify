package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFlowStateRoundTrips(t *testing.T) {
	encoded, err := encodeFlowState(flowState{
		State:    "the-state",
		Nonce:    "the-nonce",
		Verifier: "the-verifier",
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := decodeFlowState(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.State != "the-state" || got.Nonce != "the-nonce" || got.Verifier != "the-verifier" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

// An expired flow must not be completable. Without this, a state/nonce pair
// captured from an abandoned attempt stays usable indefinitely.
func TestFlowStateRejectsExpired(t *testing.T) {
	encoded, err := encodeFlowState(flowState{
		State:    "the-state",
		Nonce:    "the-nonce",
		Verifier: "the-verifier",
		Expires:  time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if _, err := decodeFlowState(encoded); err == nil {
		t.Fatal("an expired flow state was accepted")
	}
}

// A DSP connect flow carries no nonce or verifier — those belong to the OIDC
// sign-in. decodeFlowState must still accept it, and must still insist on the
// two fields every flow has.
func TestFlowStateAcceptsADSPFlowWithoutOIDCFields(t *testing.T) {
	encoded, err := encodeFlowState(flowState{
		State:    "the-state",
		Platform: "youtube_music",
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := decodeFlowState(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Platform != "youtube_music" {
		t.Fatalf("platform = %q, want it carried through", got.Platform)
	}

	stateless, err := encodeFlowState(flowState{Expires: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := decodeFlowState(stateless); err == nil {
		t.Fatal("a flow state with no state value was accepted")
	}
}

// The two flows must not share a cookie: a half-finished sign-in satisfying a
// DSP callback (or the reverse) is exactly the confusion the separate handlers
// exist to prevent.
func TestFlowCookieNamesAreDistinct(t *testing.T) {
	for _, secure := range []bool{true, false} {
		if flowCookieName(secure) == dspFlowCookieName(secure) {
			t.Errorf("sign-in and DSP flows share a cookie name (secure=%v)", secure)
		}
	}
}

func TestFlowStateRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "not-base64!!", "aGVsbG8"} {
		if _, err := decodeFlowState(in); err == nil {
			t.Errorf("decodeFlowState(%q) accepted garbage", in)
		}
	}
}

// The encoding is reversible by design, so this is not a confidentiality claim:
// what protects the verifier and nonce is HttpOnly plus the __Host- prefix. The
// check is that neither appears verbatim, so a cookie caught in a log or a proxy
// trace does not hand them over at a glance.
func TestFlowStateDoesNotCarrySecretsVerbatim(t *testing.T) {
	encoded, err := encodeFlowState(flowState{
		State:    "the-state",
		Nonce:    "the-nonce",
		Verifier: "the-verifier",
		Expires:  time.Now().Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, secret := range []string{"the-nonce", "the-verifier"} {
		if strings.Contains(encoded, secret) {
			t.Errorf("encoded flow state leaks %q verbatim: %s", secret, encoded)
		}
	}
}

// The state parameter is compared in constant time against the cookie, and a
// mismatch is what stops a callback the browser never started.
func TestMatchesStateIsExact(t *testing.T) {
	if !matchesState("abc", "abc") {
		t.Error("identical states did not match")
	}
	if matchesState("abc", "abd") {
		t.Error("different states matched")
	}
	if matchesState("", "") {
		t.Error("an empty state matched; a missing value must never satisfy the check")
	}
}

func TestSafeRedirectStaysOnTheApp(t *testing.T) {
	base := "https://aurify.test"

	if got := safeRedirect(base, "/covers"); got != "https://aurify.test/covers" {
		t.Errorf("relative path = %q, want it joined onto the app origin", got)
	}
	// An open redirect here would let the callback bounce a freshly signed-in
	// user, cookies and all, to an attacker's page.
	for _, evil := range []string{
		"https://evil.example.com/steal",
		"//evil.example.com/steal",
		"http://evil.example.com",
	} {
		if got := safeRedirect(base, evil); got != base+"/" {
			t.Errorf("safeRedirect(%q) = %q, want the app root", evil, got)
		}
	}
}

func TestClientIPPrefersForwardedHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")

	if got := clientIP(r); got != "203.0.113.7" {
		t.Fatalf("clientIP = %q, want the first forwarded hop", got)
	}
}
