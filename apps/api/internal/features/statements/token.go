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
func deriveToken(key []byte, statementID uuid.UUID) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(statementID[:])
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
