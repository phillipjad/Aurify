// Package auth holds the cryptographic building blocks behind Aurify's
// authentication: access-token signing, password hashing, and the random
// secrets used for refresh, email-verification and password-reset tokens.
//
// Everything here composes standard-library and x/crypto primitives. Nothing in
// this package implements a cryptographic algorithm itself, which is a
// deliberate boundary: the session and token design is ours, the maths is not.
// See docs/adr/0011-authentication-and-sessions.md.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// opaqueTokenBytes is the entropy behind every opaque token. 256 bits puts the
// value far outside guessing range and matches the SHA-256 digest we store.
const opaqueTokenBytes = 32

// NewOpaqueToken returns a URL-safe random token plus the digest to persist in
// its place.
//
// Only the digest is ever stored, so a database leak yields values that cannot
// be replayed as tokens. A bare SHA-256 is the correct choice here, unlike for
// passwords: the input already carries 256 bits of entropy, so there is no
// low-entropy secret for an attacker to brute-force and no reason to pay for a
// memory-hard hash on every request.
func NewOpaqueToken() (token string, digest []byte, err error) {
	raw := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: read random bytes: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken returns the digest stored in place of an opaque token. Callers look
// tokens up by this value; it is never reversible back to the token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
