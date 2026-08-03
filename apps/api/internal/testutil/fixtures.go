package testutil

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// DefaultPassword is the plaintext behind every fixture account's hash unless
// WithPassword overrides it.
const DefaultPassword = "password-123"

// JWTSecret signs access tokens in integration tests; shared so tests can mint
// tokens against the same key the service under test verifies with.
const JWTSecret = "integration-test-secret-0123456789abcdef"

// TeacherOption customizes a fixture teacher before insertion.
type TeacherOption func(acct *teachers.Account, t *teachers.Teacher, password *string)

// WithPhone sets the fixture account's phone (stored verbatim — pass E.164).
func WithPhone(phone string) TeacherOption {
	return func(acct *teachers.Account, _ *teachers.Teacher, _ *string) { acct.Phone = phone }
}

// WithFullName sets the fixture teacher's display name.
func WithFullName(name string) TeacherOption {
	return func(_ *teachers.Account, t *teachers.Teacher, _ *string) { t.FullName = name }
}

// WithStatus sets the fixture account's status.
func WithStatus(status string) TeacherOption {
	return func(acct *teachers.Account, _ *teachers.Teacher, _ *string) { acct.Status = status }
}

// WithPassword sets the plaintext the stored hash is derived from.
func WithPassword(plaintext string) TeacherOption {
	return func(_ *teachers.Account, _ *teachers.Teacher, pw *string) { *pw = plaintext }
}

// Teacher inserts a user_accounts + teachers row pair directly (bypassing the
// service) and returns both. Passwords hash at bcrypt.MinCost so fixtures stay
// fast; phones default to a unique random +84 number so tests never collide.
func Teacher(t *testing.T, db *gorm.DB, opts ...TeacherOption) (*teachers.Account, *teachers.Teacher) {
	t.Helper()
	accountID := id.New()
	acct := &teachers.Account{
		ID:     accountID,
		Role:   authctx.RoleTeacher,
		Phone:  randomPhone(),
		Status: teachers.StatusActive,
	}
	teacher := &teachers.Teacher{
		ID:       accountID,
		FullName: "Fixture Teacher",
		Timezone: teachers.DefaultTimezone,
	}
	password := DefaultPassword
	for _, opt := range opts {
		opt(acct, teacher, &password)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	hashStr := string(hash)
	acct.PasswordHash = &hashStr
	if err := db.Create(acct).Error; err != nil {
		t.Fatalf("insert fixture account %s: %v", acct.Phone, err)
	}
	if err := db.Create(teacher).Error; err != nil {
		t.Fatalf("insert fixture teacher %s: %v", acct.Phone, err)
	}
	return acct, teacher
}

// randomPhone derives a valid-shaped, effectively unique +849xxxxxxxx number
// from random UUID bytes.
func randomPhone() string {
	u := uuid.New()
	return fmt.Sprintf("+849%08d", binary.BigEndian.Uint32(u[0:4])%100000000)
}
