package auth

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if err := VerifyPassword(encoded, "correct horse battery staple"); err != nil {
		t.Fatalf("verify correct password: %v", err)
	}
	if err := VerifyPassword(encoded, "wrong password"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("verify wrong password: err = %v, want ErrPasswordMismatch", err)
	}
}

// The salt is per-password, so the same plaintext must never produce the same
// stored hash. Without this, identical passwords are visibly identical in a
// leaked database.
func TestHashPasswordIsSaltedPerCall(t *testing.T) {
	first, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	second, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if first == second {
		t.Fatal("two hashes of the same password are identical, salt is not being applied")
	}
	if err := VerifyPassword(second, "same password"); err != nil {
		t.Fatalf("verify second hash: %v", err)
	}
}

func TestHashPasswordUsesConfiguredParameters(t *testing.T) {
	encoded, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	want := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$", argonVersion, argonMemoryKiB, argonTime, argonLanes)
	if !strings.HasPrefix(encoded, want) {
		t.Fatalf("encoded = %q, want prefix %q", encoded, want)
	}
}

func TestNeedsRehash(t *testing.T) {
	current, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if NeedsRehash(current) {
		t.Fatal("a freshly created hash should not need rehashing")
	}

	// A hash stored under weaker parameters must be flagged so sign-in can
	// upgrade it while it still holds the plaintext.
	weaker := strings.Replace(current, fmt.Sprintf("m=%d", argonMemoryKiB), "m=8192", 1)
	if !NeedsRehash(weaker) {
		t.Fatal("a hash with less memory should need rehashing")
	}
	fewerPasses := strings.Replace(current, fmt.Sprintf("t=%d", argonTime), "t=1", 1)
	if !NeedsRehash(fewerPasses) {
		t.Fatal("a hash with fewer passes should need rehashing")
	}

	if !NeedsRehash("not a phc string") {
		t.Fatal("a malformed hash should need rehashing")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	valid, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	cases := map[string]string{
		"empty":            "",
		"not phc":          "plaintext",
		"wrong algorithm":  strings.Replace(valid, "argon2id", "argon2i", 1),
		"wrong version":    strings.Replace(valid, fmt.Sprintf("v=%d", argonVersion), "v=16", 1),
		"missing segments": "$argon2id$v=19$m=19456,t=2,p=1",
		"bad salt base64":  strings.Replace(valid, "$argon2id$", "$argon2id$", 1)[:strings.LastIndex(valid, "$")] + "$!!!",
	}
	for name, encoded := range cases {
		if err := VerifyPassword(encoded, "pw"); err == nil {
			t.Fatalf("%s: expected an error, got nil", name)
		}
	}
}

// VerifyDecoy exists so an unknown email costs the same as a known one. It only
// has to run without panicking; the timing property itself is not unit-testable
// in a way that would be stable on CI.
func TestVerifyDecoyRuns(t *testing.T) {
	VerifyDecoy("anything")
}

func TestNewOpaqueTokenIsUniqueAndHashable(t *testing.T) {
	first, firstDigest, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	second, _, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}

	if first == second {
		t.Fatal("two generated tokens are identical")
	}
	if first == "" {
		t.Fatal("token is empty")
	}
	if len(firstDigest) != 32 {
		t.Fatalf("digest length = %d, want 32", len(firstDigest))
	}

	// The digest must be reproducible from the token, since that is how lookups
	// find the stored row.
	rehashed := HashToken(first)
	if string(rehashed) != string(firstDigest) {
		t.Fatal("HashToken did not reproduce the digest returned by NewOpaqueToken")
	}
	if string(HashToken(second)) == string(firstDigest) {
		t.Fatal("different tokens produced the same digest")
	}
}
