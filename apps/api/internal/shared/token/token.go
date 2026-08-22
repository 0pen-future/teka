// Package token mints opaque single-use tokens and hashes them for storage.
// Refresh tokens, invitations, and password-reset links all follow the same
// shape — a 256-bit random plaintext handed out once, only its sha256-hex
// digest stored at rest — so the primitive lives here rather than inside any
// one feature (invitations must not import the auth feature to reuse it).
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// New mints a 256-bit opaque token, returning the base64url plaintext (sent to
// the client once, never stored) and its sha256 hex hash (stored at rest and
// used for lookup).
func New() (plaintext, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, Hash(plaintext), nil
}

// Hash returns the sha256 hex digest used to look up a stored token. The
// digest is 64 hex characters, matching the token_hash columns.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
