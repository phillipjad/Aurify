package auth

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	testIssuer   = "aurify"
	testAudience = "aurify-api"
)

func testPair(t *testing.T) (*Signer, *Verifier, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := NewSigner(priv, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	verifier, err := NewVerifier(pub, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return signer, verifier, priv
}

// mintRaw signs an arbitrary payload with the given header, bypassing Signer so
// tests can forge tokens that our own issuer would never produce.
func mintRaw(t *testing.T, priv ed25519.PrivateKey, header jwtHeader, payload jwtPayload) string {
	t.Helper()
	h, err := encodeSegment(header)
	if err != nil {
		t.Fatalf("encode header: %v", err)
	}
	p, err := encodeSegment(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	input := h + "." + p
	sig := ed25519.Sign(priv, []byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func validPayload() jwtPayload {
	now := time.Now().UTC()
	exp := now.Add(15 * time.Minute).Unix()
	return jwtPayload{
		Sub: "user-1",
		Sid: "session-1",
		Iss: testIssuer,
		Aud: testAudience,
		Iat: now.Unix(),
		Nbf: now.Unix(),
		Exp: &exp,
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	signer, verifier, _ := testPair(t)

	token, err := signer.Sign("user-1", "session-1", 15*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-1" || claims.SessionID != "session-1" {
		t.Fatalf("claims = %+v, want sub=user-1 sid=session-1", claims)
	}
	if claims.Issuer != testIssuer || claims.Audience != testAudience {
		t.Fatalf("iss/aud = %q/%q, want %q/%q", claims.Issuer, claims.Audience, testIssuer, testAudience)
	}
}

// The "none" algorithm asks the verifier to skip signature checking entirely.
// It must be refused even though the rest of the token is well-formed.
func TestVerifyRejectsAlgNone(t *testing.T) {
	_, verifier, priv := testPair(t)

	token := mintRaw(t, priv, jwtHeader{Alg: "none", Typ: "JWT"}, validPayload())
	if _, err := verifier.Verify(token); !errors.Is(err, ErrTokenAlgorithm) {
		t.Fatalf("alg=none: err = %v, want ErrTokenAlgorithm", err)
	}

	// Also reject it with the signature stripped, which is how "none" tokens are
	// usually presented in the wild.
	parts := strings.Split(token, ".")
	if _, err := verifier.Verify(parts[0] + "." + parts[1] + "."); !errors.Is(err, ErrTokenAlgorithm) {
		t.Fatalf("alg=none unsigned: err = %v, want ErrTokenAlgorithm", err)
	}
}

// Algorithm confusion: an attacker who knows our Ed25519 public key signs an
// HS256 token using that key as the HMAC secret. A verifier that trusted the
// token's `alg` header would validate it. Ours must not.
func TestVerifyRejectsAlgorithmConfusion(t *testing.T) {
	_, verifier, priv := testPair(t)
	pub := priv.Public().(ed25519.PublicKey)

	header, err := encodeSegment(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		t.Fatalf("encode header: %v", err)
	}
	payload, err := encodeSegment(validPayload())
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	input := header + "." + payload

	mac := hmac.New(sha256.New, pub)
	mac.Write([]byte(input))
	forged := input + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, err := verifier.Verify(forged); !errors.Is(err, ErrTokenAlgorithm) {
		t.Fatalf("HS256-with-public-key: err = %v, want ErrTokenAlgorithm", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	signer, verifier, _ := testPair(t)

	token, err := signer.Sign("user-1", "session-1", 15*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	parts := strings.Split(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload jwtPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	payload.Sub = "someone-else"
	swapped, err := encodeSegment(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	tampered := parts[0] + "." + swapped + "." + parts[2]
	if _, err := verifier.Verify(tampered); !errors.Is(err, ErrTokenSignature) {
		t.Fatalf("tampered sub: err = %v, want ErrTokenSignature", err)
	}
}

func TestVerifyRejectsForeignKey(t *testing.T) {
	_, verifier, _ := testPair(t)

	_, attackerKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	token := mintRaw(t, attackerKey, jwtHeader{Alg: algEdDSA, Typ: "JWT"}, validPayload())

	if _, err := verifier.Verify(token); !errors.Is(err, ErrTokenSignature) {
		t.Fatalf("foreign key: err = %v, want ErrTokenSignature", err)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	signer, verifier, _ := testPair(t)

	// Sign in the past so the token is expired well beyond the clock leeway.
	signer.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	token, err := signer.Sign("user-1", "session-1", time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := verifier.Verify(token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expired: err = %v, want ErrTokenExpired", err)
	}
}

// A token with no `exp` never expires, so it is a permanent credential. Refuse
// it even though it carries our own valid signature.
func TestVerifyRejectsMissingExpiry(t *testing.T) {
	_, verifier, priv := testPair(t)

	payload := validPayload()
	payload.Exp = nil
	token := mintRaw(t, priv, jwtHeader{Alg: algEdDSA, Typ: "JWT"}, payload)

	if _, err := verifier.Verify(token); !errors.Is(err, ErrTokenClaims) {
		t.Fatalf("missing exp: err = %v, want ErrTokenClaims", err)
	}
}

func TestVerifyRejectsWrongIssuerOrAudience(t *testing.T) {
	_, verifier, priv := testPair(t)

	wrongIssuer := validPayload()
	wrongIssuer.Iss = "evil"
	if _, err := verifier.Verify(
		mintRaw(t, priv, jwtHeader{Alg: algEdDSA, Typ: "JWT"}, wrongIssuer),
	); !errors.Is(
		err,
		ErrTokenClaims,
	) {
		t.Fatalf("wrong iss: err = %v, want ErrTokenClaims", err)
	}

	wrongAudience := validPayload()
	wrongAudience.Aud = "another-service"
	if _, err := verifier.Verify(
		mintRaw(t, priv, jwtHeader{Alg: algEdDSA, Typ: "JWT"}, wrongAudience),
	); !errors.Is(
		err,
		ErrTokenClaims,
	) {
		t.Fatalf("wrong aud: err = %v, want ErrTokenClaims", err)
	}
}

func TestVerifyRejectsFutureNotBefore(t *testing.T) {
	_, verifier, priv := testPair(t)

	payload := validPayload()
	payload.Nbf = time.Now().Add(time.Hour).Unix()
	token := mintRaw(t, priv, jwtHeader{Alg: algEdDSA, Typ: "JWT"}, payload)

	if _, err := verifier.Verify(token); !errors.Is(err, ErrTokenClaims) {
		t.Fatalf("future nbf: err = %v, want ErrTokenClaims", err)
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	signer, verifier, _ := testPair(t)
	good, err := signer.Sign("user-1", "session-1", 15*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parts := strings.Split(good, ".")

	cases := map[string]string{
		"empty":         "",
		"two segments":  parts[0] + "." + parts[1],
		"four segments": good + ".extra",
		"not base64":    "!!!.???.***",
		"padded base64": base64.URLEncoding.EncodeToString(
			[]byte(`{"alg":"EdDSA"}`),
		) + "." + parts[1] + "." + parts[2],
		"header not json": base64.RawURLEncoding.EncodeToString([]byte("not json")) + "." + parts[1] + "." + parts[2],
	}
	for name, token := range cases {
		if _, err := verifier.Verify(token); err == nil {
			t.Fatalf("%s: expected an error, got nil", name)
		}
	}
}

func TestParsePrivateKeySeedRoundTrip(t *testing.T) {
	seed, err := GenerateKeySeed()
	if err != nil {
		t.Fatalf("generate seed: %v", err)
	}
	key, err := ParsePrivateKeySeed(seed)
	if err != nil {
		t.Fatalf("parse seed: %v", err)
	}
	if len(key) != ed25519.PrivateKeySize {
		t.Fatalf("key size = %d, want %d", len(key), ed25519.PrivateKeySize)
	}

	if _, err := ParsePrivateKeySeed("short"); err == nil {
		t.Fatal("expected an error for a short seed")
	}
}
