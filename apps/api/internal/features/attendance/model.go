// Package attendance owns attendance_records: one row per student per
// session — including students who were present. PRD R2 ("điểm danh 1 chạm")
// makes this the data the entire product is a function of: "Mọi giá trị phía
// sau đều là hệ quả toán học của dữ liệu điểm danh."
package attendance

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Attendance status values — mirrors attendance_records.status CHECK.
const (
	StatusPresent = "present"
	StatusAbsent  = "absent"
	// StatusExcused (nghỉ có phép) is reserved for P1. The schema and the
	// billable column already support it, but V1's confirm endpoint never
	// writes it: present and absent are both billable = true.
	StatusExcused = "excused"
)

// Record is one student's attendance row for one session.
//
// Present students get rows too. The schema states why directly: "Lý do
// không chỉ lưu người vắng: cần phân biệt 'có mặt' với 'chưa điểm danh'."
// Without present rows an unconfirmed session and a fully-attended one are
// indistinguishable, which makes the G4 metric unmeasurable and the pending-
// attendance warning feed (phase 3) impossible.
//
// EnrollmentID is captured at confirmation time so plan 04 can price the
// session without re-deriving which enrollment was active on a past date —
// it stays correct even if the student later leaves and re-enrolls at a
// different price.
type Record struct {
	ID           uuid.UUID `gorm:"primaryKey"`
	TeacherID    uuid.UUID
	SessionID    uuid.UUID
	StudentID    uuid.UUID
	EnrollmentID uuid.UUID
	Status       string
	Billable     bool
	Note         *string
	RecordedAt   time.Time
	UpdatedAt    time.Time
	// DeletedAt is the one legitimate soft delete this package performs, and
	// only for one case: a student removed from the roster since the last
	// confirmation (added by mistake, or since left the class). The schema's
	// warning: "CẢNH BÁO: chỉ dùng khi học sinh bị thêm nhầm vào buổi. Học
	// sinh vắng KHÔNG phải xoá mềm — dùng status='absent'." An absent
	// student is never soft-deleted; doing so would change the billable
	// count and therefore money already reported to a parent.
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Record) TableName() string { return "attendance_records" }
