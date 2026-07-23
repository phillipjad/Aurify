package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// algEdDSA is the only algorithm this package will ever verify with.
//
// The `alg` header is read for one purpose: to reject anything that is not this
// value. It never selects a key or an algorithm. Letting the token choose its
// own verification algorithm is the classic JWT break, in two flavours: "none",
// which asks the verifier to skip the signature entirely, and algorithm
// confusion, where an RSA public key is replayed as an HMAC secret. Pinning the
// algorithm server-side makes both impossible by construction.
const algEdDSA = "EdDSA"

// clockLeeway absorbs small clock differences between the signer and verifier.
// It is deliberately tight: a generous window extends the life of a token we
// believe to be expired.
const clockLeeway = 60 * time.Second

// Token errors. Verify returns these unwrapped so callers can branch, but the
// HTTP layer must collapse all of them into a plain 401: telling a client
// exactly why its token failed is free reconnaissance.
var (
	ErrTokenMalformed = errors.New("auth: token is malformed")
	ErrTokenAlgorithm = errors.New("auth: unexpected token algorithm")
	ErrTokenSignature = errors.New("auth: token signature is invalid")
	ErrTokenExpired   = errors.New("auth: token is expired")
	ErrTokenClaims    = errors.New("auth: token claims are invalid")
)

// Claims is the payload Aurify puts in an access token. It is deliberately
// small: the token is a bearer credential that travels on every request, so it
// carries identity and nothing else. Anything mutable (email, display name,
// verification state) is read from the database, where a change takes effect
// immediately rather than at the next token refresh.
type Claims struct {
	// Subject is the Aurify user id.
	Subject string
	// SessionID ties the access token to the session that minted it, so a
	// revoked session can be recognised before its access token expires.
	SessionID string
	Issuer    string
	Audience  string
	IssuedAt  time.Time
	NotBefore time.Time
	ExpiresAt time.Time
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtPayload mirrors the registered claim names. Exp is a pointer so an absent
// `exp` is distinguishable from `exp: 0`; a token with no expiry must be
// rejected rather than treated as expiring at the Unix epoch by luck.
type jwtPayload struct {
	Sub string `json:"sub"`
	Sid string `json:"sid"`
	Iss string `json:"iss"`
	Aud string `json:"aud"`
	Iat int64  `json:"iat"`
	Nbf int64  `json:"nbf"`
	Exp *int64 `json:"exp"`
}

// Signer mints access tokens.
type Signer struct {
	key      ed25519.PrivateKey
	issuer   string
	audience string
	now      func() time.Time
}

// NewSigner builds a Signer. It rejects a wrong-sized key rather than letting
// ed25519.Sign panic at request time.
func NewSigner(key ed25519.PrivateKey, issuer, audience string) (*Signer, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("auth: ed25519 private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(key))
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("auth: signer requires an issuer and an audience")
	}
	return &Signer{key: key, issuer: issuer, audience: audience, now: time.Now}, nil
}

// Sign returns a signed access token for the given user and session.
func (s *Signer) Sign(userID, sessionID string, ttl time.Duration) (string, error) {
	if userID == "" || sessionID == "" {
		return "", errors.New("auth: sign requires a user id and a session id")
	}
	if ttl <= 0 {
		return "", errors.New("auth: sign requires a positive ttl")
	}

	now := s.now().UTC()
	exp := now.Add(ttl).Unix()
	payload := jwtPayload{
		Sub: userID,
		Sid: sessionID,
		Iss: s.issuer,
		Aud: s.audience,
		Iat: now.Unix(),
		Nbf: now.Unix(),
		Exp: &exp,
	}

	headerPart, err := encodeSegment(jwtHeader{Alg: algEdDSA, Typ: "JWT"})
	if err != nil {
		return "", err
	}
	payloadPart, err := encodeSegment(payload)
	if err != nil {
		return "", err
	}

	signingInput := headerPart + "." + payloadPart
	sig := ed25519.Sign(s.key, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verifier validates access tokens.
type Verifier struct {
	key      ed25519.PublicKey
	issuer   string
	audience string
	now      func() time.Time
}

// NewVerifier builds a Verifier, rejecting a wrong-sized key up front because
// ed25519.Verify panics on one.
func NewVerifier(key ed25519.PublicKey, issuer, audience string) (*Verifier, error) {
	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("auth: ed25519 public key must be %d bytes, got %d", ed25519.PublicKeySize, len(key))
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("auth: verifier requires an issuer and an audience")
	}
	return &Verifier{key: key, issuer: issuer, audience: audience, now: time.Now}, nil
}

// Verify checks a token's signature and claims and returns the payload.
//
// Order matters: the signature is checked before the payload is unmarshalled or
// any claim is read, so no attacker-controlled field is ever acted on before we
// know the token is genuinely ours.
func (v *Verifier) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrTokenMalformed
	}

	// RawURLEncoding is unpadded base64url per RFC 7515. Using the strict
	// decoder means a token with padding or non-URL alphabet is rejected rather
	// than silently normalised into something that might collide.
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrTokenMalformed
	}
	var header jwtHeader
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		return Claims{}, ErrTokenMalformed
	}
	// Pin the algorithm. This single comparison rejects "none" along with every
	// other value we did not issue.
	if header.Alg != algEdDSA {
		return Claims{}, ErrTokenAlgorithm
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrTokenMalformed
	}
	if !ed25519.Verify(v.key, []byte(parts[0]+"."+parts[1]), sig) {
		return Claims{}, ErrTokenSignature
	}

	// Signature verified: the payload is now trustworthy enough to parse.
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrTokenMalformed
	}
	var payload jwtPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return Claims{}, ErrTokenMalformed
	}

	if payload.Exp == nil {
		// A token that never expires is a permanent credential. Refuse it even
		// though we signed it, so a bug in issuance cannot mint one.
		return Claims{}, ErrTokenClaims
	}
	if payload.Sub == "" || payload.Sid == "" {
		return Claims{}, ErrTokenClaims
	}
	if payload.Iss != v.issuer || payload.Aud != v.audience {
		// Stops a token minted for another service, or by another issuer that
		// happens to share our key, from being accepted here.
		return Claims{}, ErrTokenClaims
	}

	now := v.now().UTC()
	expiresAt := time.Unix(*payload.Exp, 0).UTC()
	if !now.Add(-clockLeeway).Before(expiresAt) {
		return Claims{}, ErrTokenExpired
	}
	notBefore := time.Unix(payload.Nbf, 0).UTC()
	if now.Add(clockLeeway).Before(notBefore) {
		return Claims{}, ErrTokenClaims
	}
	issuedAt := time.Unix(payload.Iat, 0).UTC()
	if now.Add(clockLeeway).Before(issuedAt) {
		return Claims{}, ErrTokenClaims
	}

	return Claims{
		Subject:   payload.Sub,
		SessionID: payload.Sid,
		Issuer:    payload.Iss,
		Audience:  payload.Aud,
		IssuedAt:  issuedAt,
		NotBefore: notBefore,
		ExpiresAt: expiresAt,
	}, nil
}

func encodeSegment(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("auth: encode token segment: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ParsePrivateKeySeed builds an Ed25519 private key from a base64-encoded
// 32-byte seed, which is how the signing key is supplied by configuration.
// Seeds are used rather than full keys because the public half is derivable, so
// there is only one secret to store and no way for the two to disagree.
func ParsePrivateKeySeed(encoded string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("auth: decode key seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("auth: key seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// GenerateKeySeed returns a fresh base64-encoded signing seed, for provisioning
// a deployment or a developer's local environment.
func GenerateKeySeed() (string, error) {
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", fmt.Errorf("auth: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key.Seed()), nil
}
