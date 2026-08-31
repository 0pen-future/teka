package sessions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/authctx"
)

// defaultPendingLimit is what an unset (or non-positive) `limit` falls back
// to; maxPendingLimit caps it so an unbounded request cannot become an
// accidental full-table scan on the app's first screen.
const (
	defaultPendingLimit = 50
	maxPendingLimit     = 200
)

// PendingRow is a pending session joined with its class name and the size of
// the roster attendance confirmation would cover, populated by
// gormRepository.ListPending's single grouped join over enrollments — never
// a per-row lookup.
type PendingRow struct {
	Session              `gorm:"embedded"`
	ClassName            string
	ExpectedStudentCount int
}

// PendingSessionResponse is one row of the pending-attendance feed: a past
// session that has not had its attendance recorded yet.
type PendingSessionResponse struct {
	SessionID            uuid.UUID `json:"session_id"`
	ClassID              uuid.UUID `json:"class_id"`
	ClassName            string    `json:"class_name"`
	SessionDate          string    `json:"session_date"`
	StartTime            *string   `json:"start_time"`
	Status               string    `json:"status"`
	ExpectedStudentCount int       `json:"expected_student_count"`
	// DaysOverdue is computed from the teacher-timezone cutoff Service.ListPending
	// resolved, not stored — it is what makes the warning actionable.
	DaysOverdue int `json:"days_overdue"`
	// AttendanceSummary is always null here: the pending predicate is
	// attendance_confirmed_at IS NULL, and confirmation is the only writer of
	// attendance records. The field exists so all three session surfaces
	// (list, detail, pending) share one wire contract for calendar badges.
	AttendanceSummary *AttendanceSummary `json:"attendance_summary"`
}

// PendingResponse is the wire body of GET /sessions/pending: total is the
// unlimited count so the dashboard can render a badge without fetching
// everything; items respects the request's limit.
type PendingResponse struct {
	Total int64                    `json:"total"`
	Items []PendingSessionResponse `json:"items"`
}

// fromPendingRow maps a pending row onto the wire response. today is the
// same teacher-timezone cutoff the row was queried against, so DaysOverdue
// is always non-negative.
func fromPendingRow(row *PendingRow, today time.Time) PendingSessionResponse {
	var startTime *string
	if row.StartTime != nil {
		s := string(*row.StartTime)
		startTime = &s
	}
	return PendingSessionResponse{
		SessionID:            row.ID,
		ClassID:              row.ClassID,
		ClassName:            row.ClassName,
		SessionDate:          row.SessionDate.Format(dateLayout),
		StartTime:            startTime,
		Status:               row.Status,
		ExpectedStudentCount: row.ExpectedStudentCount,
		DaysOverdue:          int(today.Sub(row.SessionDate).Hours() / 24),
	}
}

// ListPending implements the pending-attendance predicate: center-scoped
// (narrowed to the caller's own rows when not an owner), session_date <
// before, attendance_confirmed_at IS NULL, status IN ('held','planned'),
// deleted_at IS NULL. from/to (both inclusive) narrow the range when set —
// the same predicate plan 04's period-closing gate reuses.
//
// total is counted with a plain scan over class_sessions (no joins, so it
// stays on the widened partial index); the limited rows carry the expected
// student count from one grouped LEFT JOIN over enrollments, keeping the
// whole feed at two statements regardless of result size — never a per-row
// roster lookup.
func (r *gormRepository) ListPending(ctx context.Context, sc authctx.Scope, before time.Time, from, to *time.Time, limit int) ([]PendingRow, int64, error) {
	base := func() *gorm.DB {
		q := r.scoped(ctx, sc).Model(&Session{}).
			Where("class_sessions.session_date < ?", before).
			Where("class_sessions.attendance_confirmed_at IS NULL").
			Where("class_sessions.status IN ?", []string{StatusHeld, StatusPlanned})
		if from != nil {
			q = q.Where("class_sessions.session_date >= ?", *from)
		}
		if to != nil {
			q = q.Where("class_sessions.session_date <= ?", *to)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []PendingRow
	err := base().
		Joins("JOIN classes ON classes.id = class_sessions.class_id AND classes.center_id = class_sessions.center_id").
		Joins(`LEFT JOIN enrollments ON enrollments.class_id = class_sessions.class_id
			AND enrollments.center_id = class_sessions.center_id
			AND enrollments.started_on <= class_sessions.session_date
			AND (enrollments.ended_on IS NULL OR enrollments.ended_on >= class_sessions.session_date)
			AND enrollments.deleted_at IS NULL`).
		Select("class_sessions.*, classes.name AS class_name, COUNT(enrollments.id)::int AS expected_student_count").
		Group("class_sessions.id, classes.id").
		Order("class_sessions.session_date DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
