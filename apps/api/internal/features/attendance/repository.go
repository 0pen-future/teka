package attendance

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
)

// StudentName is the display shape the attendance sheet needs from the
// students table: full name plus the optional disambiguation note.
type StudentName struct {
	FullName    string
	DisplayNote *string
}

// EnrollmentTally is one enrollment's billable/absent/present counts over a
// date window — plan 04's entire entry point into attendance_records. It is
// the one place these counting rules live; nothing outside this package
// re-aggregates the table.
type EnrollmentTally struct {
	EnrollmentID  uuid.UUID
	BillableCount int
	AbsentCount   int
	PresentCount  int
}

// Repository is the persistence contract for attendance; the service depends
// on this interface, tests supply a fake.
type Repository interface {
	// UpsertMany writes one row per record, updating status, enrollment_id,
	// billable, note, and updated_at in place when a non-deleted row already
	// exists for (session_id, student_id) — the operation that makes
	// re-confirming a session safe and idempotent. Record ids stay stable
	// across edits; recorded_at is never touched by the update path, so it
	// keeps the timestamp attendance was first taken.
	UpsertMany(ctx context.Context, records []Record) error
	// ListBySession returns the non-deleted attendance rows already recorded
	// for a session — empty for a session never confirmed.
	ListBySession(ctx context.Context, teacherID, sessionID uuid.UUID) ([]Record, error)
	// SoftDeleteMissing soft-deletes a session's records for students not in
	// keepStudentIDs — the one legitimate soft delete this package performs
	// (a student removed from the roster since the last confirmation).
	// Absent students are never touched by this: they are kept in
	// keepStudentIDs by the caller because they are still on the roster,
	// merely marked status='absent'.
	SoftDeleteMissing(ctx context.Context, teacherID, sessionID uuid.UUID, keepStudentIDs []uuid.UUID) error
	// StudentNames resolves the display name and disambiguation note for a
	// set of student ids. The join skips the deleted_at filter on students
	// deliberately: an anonymised student's attendance history must stay
	// readable under the placeholder name.
	StudentNames(ctx context.Context, teacherID uuid.UUID, studentIDs []uuid.UUID) (map[uuid.UUID]StudentName, error)
	// TallyByEnrollment groups non-deleted attendance rows by enrollment for
	// every session within [from, to] whose attendance was actually
	// confirmed — one grouped query, teacher-scoped — into billable, absent,
	// and present counts. This is plan 04's sole entry point for pricing a
	// billing period: billing calls this once per period and joins its own
	// metadata on the result; it never aggregates attendance_records itself.
	TallyByEnrollment(ctx context.Context, teacherID uuid.UUID, from, to time.Time) ([]EnrollmentTally, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a query bound to one tenant, qualified because
// TallyByEnrollment joins class_sessions, which carries the same teacher_id
// column name.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("attendance_records.teacher_id = ?", teacherID)
}

func (r *gormRepository) UpsertMany(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	// TargetWhere reproduces the partial index's predicate
	// (uq_attendance_records is WHERE deleted_at IS NULL) as the ON CONFLICT
	// target; without it Postgres cannot match the index and the insert
	// fails outright instead of updating the conflicting row. updated_at is
	// set to a fresh now() on conflict rather than the excluded value, so a
	// batch upsert reflects one consistent edit time; recorded_at is
	// deliberately absent from the SET list so it survives every edit.
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "session_id"}, {Name: "student_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
			DoUpdates: clause.Assignments(map[string]any{
				"status":        gorm.Expr("excluded.status"),
				"enrollment_id": gorm.Expr("excluded.enrollment_id"),
				"billable":      gorm.Expr("excluded.billable"),
				"note":          gorm.Expr("excluded.note"),
				"updated_at":    gorm.Expr("now()"),
			}),
		}).
		Create(&records).Error
}

func (r *gormRepository) ListBySession(ctx context.Context, teacherID, sessionID uuid.UUID) ([]Record, error) {
	var records []Record
	err := r.scoped(ctx, teacherID).
		Where("attendance_records.session_id = ?", sessionID).
		Find(&records).Error
	return records, err
}

func (r *gormRepository) SoftDeleteMissing(ctx context.Context, teacherID, sessionID uuid.UUID, keepStudentIDs []uuid.UUID) error {
	q := r.scoped(ctx, teacherID).Where("attendance_records.session_id = ?", sessionID)
	if len(keepStudentIDs) > 0 {
		q = q.Where("attendance_records.student_id NOT IN ?", keepStudentIDs)
	}
	return q.Delete(&Record{}).Error
}

func (r *gormRepository) StudentNames(ctx context.Context, teacherID uuid.UUID, studentIDs []uuid.UUID) (map[uuid.UUID]StudentName, error) {
	out := make(map[uuid.UUID]StudentName, len(studentIDs))
	if len(studentIDs) == 0 {
		return out, nil
	}
	type row struct {
		ID          uuid.UUID
		FullName    string
		DisplayNote *string
	}
	var rows []row
	// No deleted_at filter: an anonymised student's history must stay
	// readable under the placeholder name, mirroring
	// enrollments/repository.go's withNames join.
	err := database.FromContext(ctx, r.db).
		Table("students").
		Select("id, full_name, display_note").
		Where("teacher_id = ? AND id IN ?", teacherID, studentIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, rr := range rows {
		out[rr.ID] = StudentName{FullName: rr.FullName, DisplayNote: rr.DisplayNote}
	}
	return out, nil
}

func (r *gormRepository) TallyByEnrollment(ctx context.Context, teacherID uuid.UUID, from, to time.Time) ([]EnrollmentTally, error) {
	var rows []EnrollmentTally
	err := r.scoped(ctx, teacherID).
		Model(&Record{}).
		Select(`attendance_records.enrollment_id AS enrollment_id,
			COUNT(*) FILTER (WHERE attendance_records.billable = true) AS billable_count,
			COUNT(*) FILTER (WHERE attendance_records.status = 'absent') AS absent_count,
			COUNT(*) FILTER (WHERE attendance_records.status = 'present') AS present_count`).
		Joins("JOIN class_sessions ON class_sessions.id = attendance_records.session_id AND class_sessions.teacher_id = attendance_records.teacher_id").
		Where("class_sessions.deleted_at IS NULL").
		Where("class_sessions.status = 'held'").
		Where("class_sessions.attendance_confirmed_at IS NOT NULL").
		Where("class_sessions.session_date BETWEEN ? AND ?", from, to).
		Group("attendance_records.enrollment_id").
		Find(&rows).Error
	return rows, err
}
