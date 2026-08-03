package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/shared/authctx"
)

// TokenIssuer mints access JWTs and opaque refresh tokens.
type TokenIssuer struct {
	cfg config.JWTConfig
	// now is injectable for tests.
	now func() time.Time
}

// NewTokenIssuer builds a TokenIssuer from the JWT config.
func NewTokenIssuer(cfg config.JWTConfig) *TokenIssuer {
	return &TokenIssuer{cfg: cfg, now: time.Now}
}

// IssueAccess signs an HS256 access token: sub = user id, custom role claim,
// expiry from config.
func (i *TokenIssuer) IssueAccess(userID uuid.UUID, role string) (string, error) {
	now := i.now()
	claims := authctx.AccessClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.cfg.AccessTTL)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(i.cfg.Secret))
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// AccessTTL exposes the configured access-token lifetime.
func (i *TokenIssuer) AccessTTL() time.Duration { return i.cfg.AccessTTL }

// RefreshTTL exposes the configured refresh-token lifetime.
func (i *TokenIssuer) RefreshTTL() time.Duration { return i.cfg.RefreshTTL }

// NewRefreshToken mints a 256-bit opaque token, returning the plaintext
// (sent to the client once) and its sha256 hex hash (stored at rest).
func (i *TokenIssuer) NewRefreshToken() (plaintext, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashToken(plaintext), nil
}

// HashToken returns the sha256 hex digest used to look up stored tokens.
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
