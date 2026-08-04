// Package statements generates per-contact billing statements for a closed
// period and the tokenised links parents use to view them. Generation is
// teacher-initiated and every read here is teacher-scoped; the public,
// unauthenticated view a parent opens with the plaintext token is a later
// phase.
package statements

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Statement is one contact's billing summary for one period, plus the
// bookkeeping behind its tokenised link: a hash of the plaintext token (the
// plaintext itself is never stored, only derived on demand — see token.go),
// its expiry, and view/revocation tracking.
type Statement struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	ContactID uuid.UUID
	PeriodID  uuid.UUID
	TokenHash []byte
	ExpiresAt time.Time
	TotalDue  int64

	FirstViewedAt *time.Time
	LastViewedAt  *time.Time
	ViewCount     int

	RevokedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Statement) TableName() string { return "statements" }
