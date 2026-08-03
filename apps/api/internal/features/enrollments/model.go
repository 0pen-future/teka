// Package enrollments manages the join between a student and a class — the
// row that carries the unit price actually billed. PRD R1: "Đơn giá nằm ở bản
// ghi ghi danh, không nằm ở lớp." The price is copied from the class at
// enrollment time and never read live again, so raising a class's price can
// never rewrite what past students owed. started_on is stored exactly as
// given — mid-cycle joins are the product's whole reason for existing — and
// ended_on closes the row while keeping its history readable.
package enrollments

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Enrollment is one ghi danh. A student may hold many closed enrollments in
// the same class and at most one open one (uq_enrollments_active).
type Enrollment struct {
	// ID is a UUIDv7 generated in Go via id.New(); the column has no default.
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	StudentID uuid.UUID
	ClassID   uuid.UUID
	// StartedOn/EndedOn are Postgres DATE columns; only the date part is
	// meaningful. EndedOn NULL means the student is still enrolled.
	StartedOn time.Time
	EndedOn   *time.Time
	// UnitPrice is đồng per session, copied from classes.default_unit_price
	// at creation and frozen thereafter.
	UnitPrice int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Enrollment) TableName() string { return "enrollments" }
