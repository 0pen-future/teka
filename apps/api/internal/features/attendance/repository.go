package attendance

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
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
	ListBySession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]Record, error)
	// SoftDeleteMissing soft-deletes a session's records for students not in
	// keepStudentIDs — the one legitimate soft delete this package performs
	// (a student removed from the roster since the last confirmation).
	// Absent students are never touched by this: they are kept in
	// keepStudentIDs by the caller because they are still on the roster,
	// merely marked status='absent'.
	SoftDeleteMissing(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, keepStudentIDs []uuid.UUID) error
	// StudentNames resolves the display name and disambiguation note for a
	// set of student ids. The join skips the deleted_at filter on students
	// deliberately: an anonymised student's attendance history must stay
	// readable under the placeholder name.
	StudentNames(ctx context.Context, sc authctx.Scope, studentIDs []uuid.UUID) (map[uuid.UUID]StudentName, error)
	// TallyByEnrollment groups non-deleted attendance rows by enrollment for
	// every session within [from, to] whose attendance was actually
	// confirmed — one grouped query, center-scoped — into billable, absent,
	// and present counts. This is plan 04's sole entry point for pricing a
	// billing period: billing calls this once per period and joins its own
	// metadata on the result; it never aggregates attendance_records itself.
	TallyByEnrollment(ctx context.Context, sc authctx.Scope, from, to time.Time) ([]EnrollmentTally, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// readScoped lets a member read their own attendance rows plus rows of
// sessions whose class they hold a class_staff stint on, ended stints
// included. Reads only: the write paths settle permission through the
// session write gate in the service, and their row filters never consult
// attendance_records.teacher_id (last-writer attribution, not ownership).
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("attendance_records.center_id = ?", sc.CenterID)
	if !sc.CenterWide() {
		frag, _ := classscope.ReadExists("s2.class_id")
		q = q.Where(`(attendance_records.teacher_id = ? OR EXISTS (
			SELECT 1 FROM class_sessions s2
			WHERE s2.id = attendance_records.session_id
			  AND s2.center_id = attendance_records.center_id
			  AND `+frag+`))`, sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	return q
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
	// teacher_id IS in the SET list: it is last-writer attribution, not a row
	// filter — an assistant editing a teacher's row takes the credit, and no
	// read that prices or deletes rows may ever filter on it.
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "session_id"}, {Name: "student_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
			DoUpdates: clause.Assignments(map[string]any{
				"status":        gorm.Expr("excluded.status"),
				"enrollment_id": gorm.Expr("excluded.enrollment_id"),
				"billable":      gorm.Expr("excluded.billable"),
				"note":          gorm.Expr("excluded.note"),
				"teacher_id":    gorm.Expr("excluded.teacher_id"),
				"updated_at":    gorm.Expr("now()"),
			}),
		}).
		Create(&records).Error
}

func (r *gormRepository) ListBySession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]Record, error) {
	var records []Record
	err := r.readScoped(ctx, sc).
		Where("attendance_records.session_id = ?", sessionID).
		Find(&records).Error
	return records, err
}

func (r *gormRepository) SoftDeleteMissing(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, keepStudentIDs []uuid.UUID) error {
	// Rows are selected by (center, session) alone — never by who recorded
	// them. The permission to touch this session's sheet was already settled
	// by the service's session write gate; filtering by recorder here would
	// leave another writer's off-roster rows behind as orphaned billable
	// records.
	q := database.FromContext(ctx, r.db).
		Where("attendance_records.center_id = ?", sc.CenterID).
		Where("attendance_records.session_id = ?", sessionID)
	if len(keepStudentIDs) > 0 {
		q = q.Where("attendance_records.student_id NOT IN ?", keepStudentIDs)
	}
	return q.Delete(&Record{}).Error
}

func (r *gormRepository) StudentNames(ctx context.Context, sc authctx.Scope, studentIDs []uuid.UUID) (map[uuid.UUID]StudentName, error) {
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
	// enrollments/repository.go's withNames join. center_id (not teacher_id)
	// so an owner can still resolve names for a member's roster.
	q := database.FromContext(ctx, r.db).
		Table("students").
		Select("id, full_name, display_note").
		Where("center_id = ? AND id IN ?", sc.CenterID, studentIDs)
	if !sc.CenterWide() {
		// Own students, plus students enrolled in a class the caller holds a
		// class_staff stint on (ended included): a handoff moves the class but
		// not the student rows, and the staff sheet must still show names. The
		// enrollment may be ended — history stays readable — but never
		// soft-deleted.
		frag, _ := classscope.ReadExistsViaEnrollment("students.id")
		q = q.Where("(teacher_id = ? OR "+frag+")", sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, rr := range rows {
		out[rr.ID] = StudentName{FullName: rr.FullName, DisplayNote: rr.DisplayNote}
	}
	return out, nil
}

func (r *gormRepository) TallyByEnrollment(ctx context.Context, sc authctx.Scope, from, to time.Time) ([]EnrollmentTally, error) {
	var rows []EnrollmentTally
	// A member scope means "this teacher's enrollments" — billing passes the
	// period teacher here — resolved through the enrollment join, NEVER
	// through attendance_records.teacher_id: that column is last-writer
	// attribution, and filtering on it would drop assistant-recorded rows
	// from the teacher's invoice.
	q := database.FromContext(ctx, r.db).
		Model(&Record{}).
		Where("attendance_records.center_id = ?", sc.CenterID).
		Select(`attendance_records.enrollment_id AS enrollment_id,
			COUNT(*) FILTER (WHERE attendance_records.billable = true) AS billable_count,
			COUNT(*) FILTER (WHERE attendance_records.status = 'absent') AS absent_count,
			COUNT(*) FILTER (WHERE attendance_records.status = 'present') AS present_count`).
		Joins("JOIN class_sessions ON class_sessions.id = attendance_records.session_id AND class_sessions.center_id = attendance_records.center_id").
		Where("class_sessions.deleted_at IS NULL").
		Where("class_sessions.status = 'held'").
		Where("class_sessions.attendance_confirmed_at IS NOT NULL").
		Where("class_sessions.session_date BETWEEN ? AND ?", from, to)
	if !sc.CenterWide() {
		q = q.Joins("JOIN enrollments ON enrollments.id = attendance_records.enrollment_id AND enrollments.center_id = attendance_records.center_id").
			Where("enrollments.teacher_id = ?", sc.TeacherID)
	}
	err := q.Group("attendance_records.enrollment_id").
		Find(&rows).Error
	return rows, err
}
