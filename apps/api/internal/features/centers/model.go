// Package centers owns the tenant of the system: the center a teacher's data
// lives in. Membership is a history, not a flag — center_members keeps one
// row per (teacher, center) pair, and the business tables' guard FKs anchor
// into it so a teacher's rows STAY in a center after they leave. Schema lives
// in migration 000007; models mirror it and are never auto-migrated.
package centers

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Center mirrors centers: the tenant. Every teacher is the owner of exactly
// one live center at registration (their personal center); joining another
// center retires the empty personal one (soft delete).
type Center struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name      string
	OwnerID   uuid.UUID `gorm:"type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table name explicitly.
func (Center) TableName() string { return "centers" }

// Member mirrors center_members: one row per (teacher, center) pair. The
// live membership has left_at NULL (at most one per teacher —
// uq_center_members_active); rejoining the same center reopens the closed
// row and refreshes joined_at, so only the latest stint per pair survives.
// A closed row is never deleted, because the business tables' guard FKs
// cascade off it and deleting it would take the teacher's historical data
// with it.
type Member struct {
	TeacherID uuid.UUID `gorm:"type:uuid;primaryKey"`
	CenterID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	JoinedAt  time.Time
	LeftAt    *time.Time
	// CanSendReports is the delegated report-sender permission, scoped to
	// this stint: closing or reopening the membership resets it to false.
	CanSendReports bool
	// RoleID is the stint's center role. NULL means "no role permissions" —
	// the owner's stint always, and raw-insert paths that predate role
	// seeding. Reopening a membership resets it to the center's default
	// giao_vien role.
	RoleID *uuid.UUID `gorm:"type:uuid"`
}

// TableName pins the table name explicitly.
func (Member) TableName() string { return "center_members" }
