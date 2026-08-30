package statements

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"

	"github.com/google/uuid"
)

// deriveToken computes the plaintext token for statementID under key. The
// token is never stored — only hashToken's digest of it is — so it can only
// ever be reconstructed by someone who holds key, which is exactly what lets
// a teacher-authenticated response recompute a parent's link on demand
// without a token column to read it back from.
//
// classID is the statement's own class scope: a class-scoped statement's
// token binds the class into the MAC, so its URL can only ever resolve to
// that class's content. A nil classID (family statement) contributes nothing
// to the MAC — every family token minted before class scoping existed keeps
// resolving byte-identically.
func deriveToken(key []byte, statementID uuid.UUID, classID *uuid.UUID) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(statementID[:])
	if classID != nil {
		mac.Write(classID[:])
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// hashToken digests a plaintext token (as presented on the public link) into
// the 32-byte value stored in statements.token_hash — what GetByTokenHash
// looks a statement up by, so the plaintext itself never touches the
// database.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
