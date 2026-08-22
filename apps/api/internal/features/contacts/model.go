// Package contacts manages the người liên hệ roster: the parents/guardians
// who receive fee messages and pay them. Phone uniqueness is per teacher;
// students hang off contacts, so a contact with live students cannot be
// deleted.
package contacts

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Contact is the người liên hệ — the parent or guardian who receives the fee
// message and pays it (PRD R5/R7). Phone uniqueness is per-teacher, never
// global: a parent whose children study with several teachers is several
// independent rows, one per tenant.
type Contact struct {
	// ID is a UUIDv7 generated in Go via id.New(); the column has no default.
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	// CenterID anchors the row in the center it was created in. It never
	// moves the row when the creating teacher later changes centers — a
	// contact stays with the center that owned it at creation time.
	CenterID uuid.UUID
	// UserID stays NULL for all of V1 — parents do not log in, they open a
	// token link. Modelled but never written.
	UserID   *uuid.UUID
	FullName string
	Phone    string
	// ZaloUserID/ZaloName record which Zalo friend this contact is, chosen by
	// the teacher in the friend picker. ZaloName is the friend's name at
	// mapping time so lists render without refetching the live friend list.
	// Both NULL until mapped; always set and cleared together.
	ZaloUserID *string
	ZaloName   *string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Contact) TableName() string { return "contacts" }
