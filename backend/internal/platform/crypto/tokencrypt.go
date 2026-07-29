// Package crypto encrypts the short-lived third-party secrets Aurify has to
// store — currently DSP access and refresh tokens.
//
// The threat it addresses is a copy of the database, or a backup, reaching
// somebody who should not have it: a leaked dump of dsp_connections used to hand
// over live access to every connected music library. It is not protection against
// a compromised application process, which necessarily holds the key.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// envelopePrefix tags values this package produced. It is a version marker, so a
// future scheme can be introduced without guessing at what a stored value is,
// and it is what distinguishes ciphertext from a row written before encryption
// existed.
const envelopePrefix = "v1:"

// ErrCipherKey reports a key that is not usable.
var ErrCipherKey = errors.New("crypto: cipher key must be 32 bytes")

// Cipher encrypts and decrypts stored secrets with AES-256-GCM.
type Cipher struct{ aead cipher.AEAD }

// DeriveKey derives the storage key from the Ed25519 authentication seed, so a
// deployment has one secret to manage rather than two.
//
// The consequence is worth stating plainly: rotating the authentication key makes
// every stored token undecryptable, and users have to reconnect their accounts.
// A dedicated key is the upgrade path if that becomes unacceptable.
func DeriveKey(seed []byte) []byte {
	h := sha256.New()
	h.Write([]byte("aurify/dsp-token/v1"))
	h.Write(seed)
	return h.Sum(nil)
}

// NewCipher builds a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w, got %d", ErrCipherKey, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals a value for storage. An empty string stays empty: an absent
// refresh token is absent, and writing ciphertext for it would only obscure that.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return envelopePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a stored value.
//
// A value with no envelope prefix is returned as-is. That is deliberate, and it
// is the one concession here: rows written before encryption existed hold
// plaintext, and refusing them would lock users out of connections they made
// earlier. Each is replaced with ciphertext the next time it is saved. Remove
// this path once no plaintext rows remain.
func (c *Cipher) Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	encoded, ok := strings.CutPrefix(stored, envelopePrefix)
	if !ok {
		return stored, nil
	}

	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: decode envelope: %w", err)
	}
	if len(sealed) < c.aead.NonceSize() {
		return "", errors.New("crypto: envelope is too short to contain a nonce")
	}
	nonce, ct := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: open envelope: %w", err)
	}
	return string(plaintext), nil
}
