package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id cost parameters, at the OWASP-recommended minimum: 19 MiB of memory,
// two passes, one lane. Memory hardness is what makes the hash expensive to
// attack on GPUs and ASICs, so memoryKiB is the knob to raise first when these
// are retuned. Raising any of them is safe for existing users: NeedsRehash
// reports stored hashes that used weaker parameters so sign-in can upgrade them.
const (
	argonMemoryKiB uint32 = 19 * 1024
	argonTime      uint32 = 2
	argonLanes     uint8  = 1
	argonSaltLen          = 16
	argonKeyLen    uint32 = 32
)

// argonVersion is the Argon2 version embedded in the encoded hash. x/crypto
// implements 0x13 (19).
const argonVersion = argon2.Version

// Password errors. ErrPasswordMismatch is deliberately the only signal a caller
// gets for a bad password, so nothing upstream can accidentally distinguish
// "wrong password" from "malformed stored hash" in a user-facing message.
var (
	ErrPasswordMismatch = errors.New("auth: password does not match")
	ErrHashMalformed    = errors.New("auth: stored password hash is malformed")
)

// decoySalt backs VerifyDecoy. It is a fixed, non-secret value: the decoy hash
// is never compared against anything, it exists purely to burn the same CPU and
// memory as a real verification.
var decoySalt = []byte("aurify-decoy-salt")

// HashPassword derives an Argon2id hash and returns it in the standard PHC
// string format, which carries the parameters and salt alongside the digest:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
//
// Encoding the parameters is what makes them changeable later; a bare digest
// would pin us to today's cost forever.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonLanes, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion, argonMemoryKiB, argonTime, argonLanes,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword recomputes the hash using the parameters stored in encoded and
// reports whether it matches. It returns nil on success and ErrPasswordMismatch
// on a wrong password.
//
// The comparison is constant-time: a byte-wise early exit would leak how much of
// the digest matched, which is enough to reconstruct it one byte at a time.
func VerifyPassword(encoded, password string) error {
	p, salt, want, err := parsePHC(encoded)
	if err != nil {
		return err
	}

	got := argon2.IDKey([]byte(password), salt, p.time, p.memoryKiB, p.lanes, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// VerifyDecoy performs an Argon2id derivation with the same cost as
// VerifyPassword and discards the result.
//
// Sign-in calls this when the email address is unknown. Without it, a missing
// account returns in microseconds while a real account pays ~19 MiB of hashing,
// and that timing difference alone tells an attacker which addresses are
// registered.
func VerifyDecoy(password string) {
	argon2.IDKey([]byte(password), decoySalt, argonTime, argonMemoryKiB, argonLanes, argonKeyLen)
}

// NeedsRehash reports whether a stored hash was produced with weaker parameters
// than we now use, so sign-in can transparently upgrade it while it holds the
// plaintext. A malformed hash also reports true: it needs replacing either way.
func NeedsRehash(encoded string) bool {
	p, _, _, err := parsePHC(encoded)
	if err != nil {
		return true
	}
	return p.memoryKiB < argonMemoryKiB || p.time < argonTime || p.lanes != argonLanes
}

type argonParams struct {
	memoryKiB uint32
	time      uint32
	lanes     uint8
}

// parsePHC splits a PHC-format Argon2id string into its parameters, salt and
// digest. It is strict on purpose: anything it does not fully understand is
// rejected rather than guessed at, since a silently mis-parsed hash would
// compare against the wrong bytes.
func parsePHC(encoded string) (argonParams, []byte, []byte, error) {
	// Leading empty field comes from the leading '$'.
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrHashMalformed
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argonVersion {
		return argonParams{}, nil, nil, ErrHashMalformed
	}

	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memoryKiB, &p.time, &p.lanes); err != nil {
		return argonParams{}, nil, nil, ErrHashMalformed
	}
	if p.memoryKiB == 0 || p.time == 0 || p.lanes == 0 {
		return argonParams{}, nil, nil, ErrHashMalformed
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return argonParams{}, nil, nil, ErrHashMalformed
	}
	digest, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(digest) == 0 {
		return argonParams{}, nil, nil, ErrHashMalformed
	}
	return p, salt, digest, nil
}
