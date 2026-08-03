package testutil

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/users"
)

// DefaultPassword is the plaintext behind every fixture user's hash unless
// WithPassword overrides it.
const DefaultPassword = "password-123"

// JWTSecret signs access tokens in integration tests; shared so tests can mint
// tokens against the same key the service under test verifies with.
const JWTSecret = "integration-test-secret-0123456789abcdef"

// UserOption customizes a fixture user before insertion.
type UserOption func(*users.User, *string)

// WithEmail sets the fixture user's email.
func WithEmail(email string) UserOption {
	return func(u *users.User, _ *string) { u.Email = email }
}

// WithName sets the fixture user's display name.
func WithName(name string) UserOption {
	return func(u *users.User, _ *string) { u.Name = name }
}

// WithRole sets the fixture user's role.
func WithRole(role string) UserOption {
	return func(u *users.User, _ *string) { u.Role = role }
}

// WithPassword sets the plaintext the stored hash is derived from.
func WithPassword(plaintext string) UserOption {
	return func(_ *users.User, pw *string) { *pw = plaintext }
}

// User inserts a user row directly (bypassing the service) and returns it.
// Passwords hash at bcrypt.MinCost so fixtures stay fast; emails default to a
// unique random address so tests never collide.
func User(t *testing.T, db *gorm.DB, opts ...UserOption) *users.User {
	t.Helper()
	u := &users.User{
		Email: fmt.Sprintf("user-%s@example.com", uuid.NewString()[:8]),
		Name:  "Fixture User",
		Role:  users.RoleUser,
	}
	password := DefaultPassword
	for _, opt := range opts {
		opt(u, &password)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	u.PasswordHash = string(hash)
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("insert fixture user %s: %v", u.Email, err)
	}
	return u
}
