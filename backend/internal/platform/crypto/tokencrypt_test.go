package crypto

import (
	"errors"
	"strings"
	"testing"
)

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := NewCipher(DeriveKey([]byte("test-seed")))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	c := newTestCipher(t)

	sealed, err := c.Encrypt("ya29.a0AfB_by-secret-token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// The whole point: what lands in the row must not be the token.
	if strings.Contains(sealed, "secret-token") {
		t.Fatalf("stored value leaks the plaintext: %s", sealed)
	}

	got, err := c.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "ya29.a0AfB_by-secret-token" {
		t.Fatalf("round trip = %q, want the original token", got)
	}
}

// Two encryptions of the same token must differ, or a dump would reveal which
// users share a value and make ciphertext comparable across rows.
func TestEncryptUsesAFreshNonce(t *testing.T) {
	c := newTestCipher(t)

	first, err := c.Encrypt("same-token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	second, err := c.Encrypt("same-token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if first == second {
		t.Fatal("identical plaintexts produced identical ciphertexts")
	}
}

// An absent refresh token stays absent rather than becoming ciphertext for "".
func TestEmptyValuesPassThrough(t *testing.T) {
	c := newTestCipher(t)

	sealed, err := c.Encrypt("")
	if err != nil || sealed != "" {
		t.Fatalf("Encrypt(\"\") = %q, %v; want empty", sealed, err)
	}
	got, err := c.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("Decrypt(\"\") = %q, %v; want empty", got, err)
	}
}

// Rows written before encryption existed hold plaintext and have no envelope
// prefix. They are returned as-is so an earlier connection keeps working; each is
// replaced with ciphertext the next time it is saved.
func TestUnprefixedValuesAreTreatedAsLegacyPlaintext(t *testing.T) {
	c := newTestCipher(t)

	got, err := c.Decrypt("plain-old-token")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "plain-old-token" {
		t.Fatalf("legacy value = %q, want it passed through", got)
	}
}

// Tampering must fail loudly rather than yield garbage, which is what the GCM tag
// is for.
func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	c := newTestCipher(t)

	sealed, err := c.Encrypt("token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip the last character of the envelope body.
	flipped := sealed[:len(sealed)-1] + map[bool]string{true: "A", false: "B"}[sealed[len(sealed)-1] != 'A']

	if _, err := c.Decrypt(flipped); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

// A value sealed under one key must not open under another, which is what makes
// the "rotating the auth seed forces a reconnect" consequence real rather than
// theoretical.
func TestDecryptFailsUnderADifferentKey(t *testing.T) {
	sealed, err := newTestCipher(t).Encrypt("token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	other, err := NewCipher(DeriveKey([]byte("a-different-seed")))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	if _, err := other.Decrypt(sealed); err == nil {
		t.Fatal("a value sealed under another key was accepted")
	}
}

func TestNewCipherRejectsAShortKey(t *testing.T) {
	if _, err := NewCipher([]byte("too-short")); !errors.Is(err, ErrCipherKey) {
		t.Fatalf("err = %v, want ErrCipherKey", err)
	}
}
