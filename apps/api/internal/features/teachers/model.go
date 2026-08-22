// Package teachers owns the identity slice of the domain: the user_accounts
// login row and the teachers business profile. The two tables share one id —
// the JWT subject, the account id, and the teacher_id every downstream
// repository filters on are the same value. Schema lives in migration 000001;
// models mirror it and are never auto-migrated.
package teachers

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Account statuses; mirrored by the CHECK constraint on user_accounts.status.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// DefaultTimezone is the IANA zone assigned to new teacher profiles.
const DefaultTimezone = "Asia/Ho_Chi_Minh"

// Account mirrors user_accounts: the login identity. Ids are supplied by the
// caller (UUIDv7 from internal/shared/id) — the schema declares no DB-side
// default. PasswordHash is nullable for future OTP-only accounts; V1 always
// writes a bcrypt hash.
type Account struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Role         string
	Phone        string
	PasswordHash *string
	Status       string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt
}

// TableName pins the table; GORM would otherwise pluralise to "accounts".
func (Account) TableName() string { return "user_accounts" }

// Teacher mirrors the teachers business profile; Teacher.ID references
// user_accounts(id) and carries the same value. CenterID is the teacher's
// current (live) membership — always exactly one, backed by the deferrable
// fk_teachers_membership into center_members.
type Teacher struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	FullName  string
	Timezone  string
	CenterID  uuid.UUID `gorm:"type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table name explicitly.
func (Teacher) TableName() string { return "teachers" }

// Profile bundles the login account with its teacher business profile; the
// two rows share one id and are created in the same transaction.
type Profile struct {
	Account Account
	Teacher Teacher
}
