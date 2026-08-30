// Package statements generates per-contact billing statements for a closed
// period and the tokenised links parents use to view them. Generation and
// every authenticated read here are center-scoped, with owner oversight over
// every teacher's own statements in the center; the public, unauthenticated
// view a parent opens with the plaintext token derives its own scope from the
// resolved statement row instead.
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
	// CenterID is inherited from the statement's billing period at creation
	// (see Service.Generate's periodScope anchor), never the acting caller's
	// — the public, unauthenticated view derives its own read scope straight
	// from this row's TeacherID/CenterID, so a forged or guessed token can
	// never reach past its own family's own tenancy.
	CenterID  uuid.UUID
	ContactID uuid.UUID
	PeriodID  uuid.UUID
	// ClassID is nil on a family statement (the whole family's bill for the
	// period — the original unit) and set on a class-scoped statement, which
	// carries only that class's charges. The class is bound into the token
	// derivation, so one copy's link can never open the other's content.
	ClassID   *uuid.UUID
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
