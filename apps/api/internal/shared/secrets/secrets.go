// Package secrets seals values that must survive at rest but must not be
// readable from a stolen database row. It is a thin, deliberately small
// envelope over AES-256-GCM: one process-wide key, a fresh random nonce per
// seal, and no logging of anything it touches.
//
// The package is not named crypto so that a caller can import it alongside
// crypto/rand and friends without aliasing either.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// MinKeyLen is the smallest key, in bytes, New accepts. 32 bytes (256 bits)
// matches the AES-256 key the envelope is built on.
const MinKeyLen = 32

// ErrKeyTooShort reports a key with less than MinKeyLen bytes of material.
var ErrKeyTooShort = errors.New("secrets: key must be at least 32 bytes")

// ErrCiphertextTooShort reports a value shorter than the nonce it must begin
// with, so it cannot have come from Seal.
var ErrCiphertextTooShort = errors.New("secrets: ciphertext is shorter than the nonce")

// Cipher seals and opens values under one key. It is safe for concurrent use.
// Construct it once, at startup, from the configured key.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from raw key material of at least MinKeyLen bytes.
//
// The AES-256 key is SHA-256 of the supplied bytes rather than the bytes
// themselves: configuration hands over whatever the operator set (hex,
// base64, or a long passphrase), and AES needs exactly 32. Hashing accepts
// every one of those shapes, is stable across restarts, and — unlike
// truncating to the first 32 bytes — keeps the entropy of a longer key.
func New(key []byte) (*Cipher, error) {
	if len(key) < MinKeyLen {
		return nil, ErrKeyTooShort
	}
	derived := sha256.Sum256(key)
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, fmt.Errorf("secrets: build aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: build gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts plaintext and returns nonce || ciphertext || tag. The nonce is
// drawn fresh from crypto/rand on every call; reusing one under the same key
// would break GCM's confidentiality outright.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secrets: read nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open reverses Seal. It returns an error for any value that was truncated,
// extended, modified, or sealed under a different key — GCM authenticates, so
// there is no partial or best-effort decryption. The error never carries the
// ciphertext.
func (c *Cipher) Open(sealed []byte) ([]byte, error) {
	nonceSize := c.aead.NonceSize()
	if len(sealed) <= nonceSize {
		return nil, ErrCiphertextTooShort
	}
	plaintext, err := c.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return nil, errors.New("secrets: open failed: ciphertext is not authentic for this key")
	}
	return plaintext, nil
}
