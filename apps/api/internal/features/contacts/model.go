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
	// UserID stays NULL for all of V1 — parents do not log in, they open a
	// token link. Modelled but never written.
	UserID    *uuid.UUID
	FullName  string
	Phone     string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Contact) TableName() string { return "contacts" }
