package enrollments

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the enrollment list. Nil uuids mean unset; Active nil
// means both open and ended rows.
type ListFilter struct {
	StudentID uuid.UUID
	ClassID   uuid.UUID
	Active    *bool
}

// Row is an enrollment joined with the student and class names the list and
// detail responses display. The joins deliberately skip the deleted_at filter
// on students and classes: an anonymised student's history must stay readable
// under the placeholder name.
type Row struct {
	Enrollment  `gorm:"embedded"`
	StudentName string
	ClassName   string
}

// Repository is the persistence contract for enrollments; the service depends
// on this interface, tests supply a fake.
type Repository interface {
	Create(ctx context.Context, e *Enrollment) error
	GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error)
	// GetWritableByID resolves an enrollment through the write gate: the
	// owner center-wide, a member only via an active class_staff stint on
	// the enrollment's class whose role is in roles. ErrNotFound covers both
	// a missing row and a readable one the caller cannot manage — the
	// service disambiguates through the read port.
	GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, roles []string) (*Row, error)
	List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error)
	// FindByStudentAndClass returns the enrollment linking this student to
	// this class, open or already ended. Bulk flows need the ended case: it is
	// invisible to uq_enrollments_active, so a caller that only looked for
	// open rows would silently re-enrol a student who left.
	FindByStudentAndClass(ctx context.Context, sc authctx.Scope, studentID, classID uuid.UUID) (*Enrollment, error)
	// End stamps ended_on on one open enrollment; the row survives. Bounded
	// by the write gate: roles is the set of class_staff roles whose active
	// stint on the enrollment's class permits managing its roster.
	End(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID, endedOn time.Time) error
	SoftDelete(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID) error
	// ActiveOn returns the enrollments that should appear on a class's
	// attendance sheet for a given date: started on or before it, and not
	// ended before it. Both boundaries are inclusive — a student whose
	// started_on equals the date attends that session, and a student whose
	// ended_on equals it attends their last one. An exclusive boundary would
	// silently lose one session of revenue per student per departure.
	ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]Enrollment, error)
	// EndOpenEnrollments closes every open enrollment the student holds,
	// effective on the given date — the students feature calls this while
	// anonymising a deleted student.
	EndOpenEnrollments(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error
	// ClassDefaultPrice reads the class's default_unit_price — the value
	// copied onto new enrollments — returning ErrClassNotFound for a missing
	// or foreign class.
	ClassDefaultPrice(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error)
	StudentExists(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (bool, error)
	// ClassInCenter reports whether the class exists live in the caller's
	// center, regardless of who holds it — the existence gate that decides
	// 404 versus 403 for the picker.
	ClassInCenter(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
	// CallerClassStanding reports the caller's relationship to the class:
	// assigned is any class_staff stint (ended included, mirroring read
	// scoping), teaching is an ACTIVE giao_vien stint — the standing that
	// permits enrolling.
	CallerClassStanding(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (assigned, teaching bool, err error)
	// SearchEnrollableStudents finds live center students by name fragment
	// who are not already actively enrolled in classID,
	// sorted by name. Names only — the picker must never carry contact data.
	SearchEnrollableStudents(ctx context.Context, sc authctx.Scope, classID uuid.UUID, q string, limit int) ([]PickerStudent, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns an enrollments query bound to one center. An owner sees
// every enrollment in their center; a member sees only the rows they created
// themselves. Composite FKs stop cross-center writes; only this filter stops
// cross-tenant reads. The center_id column is qualified because list queries
// join students and classes, which carry the same column name.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("enrollments.center_id = ?", sc.CenterID)
	if !sc.CenterWide() {
		q = q.Where("enrollments.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// readScoped additionally lets a member read enrollments of classes they hold
// a class_staff stint on — the current teacher after a handoff (rows never
// move), and any other staff assignment, ended ones included. Reads only:
// managing an enrollment (end, delete) resolves through writeScoped.
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("enrollments.center_id = ?", sc.CenterID)
	if !sc.CenterWide() {
		frag, _ := classscope.ReadExists("enrollments.class_id")
		q = q.Where("(enrollments.teacher_id = ? OR "+frag+")",
			sc.TeacherID, sc.TeacherID, sc.CenterID)
	}
	return q
}

// writeScoped bounds enrollment mutations to the capability model: the owner
// center-wide, a member only through an ACTIVE class_staff stint on the
// enrollment's class whose role is in roles. The creator's teacher_id anchor
// grants nothing here — after a handoff the class's current giáo viên manages
// the roster, the creator does not.
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope, roles []string) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("enrollments.center_id = ?", sc.CenterID)
	if !sc.CenterWide() {
		frag, _ := classscope.WriteExists("enrollments.class_id")
		q = q.Where(frag, sc.TeacherID, sc.CenterID, roles)
	}
	return q
}

// withNames joins the display names onto an enrollment query. Same-center
// join conditions keep the composite-key discipline even though the FKs
// already guarantee it — matching on center_id (not teacher_id) lets an
// owner's enrollment for a member's student/class still resolve its names.
func withNames(q *gorm.DB) *gorm.DB {
	return q.
		Joins("JOIN students ON students.id = enrollments.student_id AND students.center_id = enrollments.center_id").
		Joins("JOIN classes ON classes.id = enrollments.class_id AND classes.center_id = enrollments.center_id").
		Select("enrollments.*, students.full_name AS student_name, classes.name AS class_name")
}

func (r *gormRepository) Create(ctx context.Context, e *Enrollment) error {
	err := database.FromContext(ctx, r.db).Create(e).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrAlreadyEnrolled
	}
	return err
}

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	var row Row
	err := withNames(r.readScoped(ctx, sc).Model(&Enrollment{})).
		Where("enrollments.id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	q := r.readScoped(ctx, sc).Model(&Enrollment{})
	if filter.StudentID != uuid.Nil {
		q = q.Where("enrollments.student_id = ?", filter.StudentID)
	}
	if filter.ClassID != uuid.Nil {
		q = q.Where("enrollments.class_id = ?", filter.ClassID)
	}
	if filter.Active != nil {
		if *filter.Active {
			q = q.Where("enrollments.ended_on IS NULL")
		} else {
			q = q.Where("enrollments.ended_on IS NOT NULL")
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	if err := withNames(q).Scopes(p.Scope).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, roles []string) (*Row, error) {
	var row Row
	err := withNames(r.writeScoped(ctx, sc, roles).Model(&Enrollment{})).
		Where("enrollments.id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) End(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID, endedOn time.Time) error {
	res := r.writeScoped(ctx, sc, roles).
		Model(&Enrollment{}).
		Where("enrollments.id = ? AND enrollments.ended_on IS NULL", id).
		Update("ended_on", endedOn)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// The row was not updated. Distinguish a genuinely absent (or foreign)
		// enrollment from one that a concurrent request already closed: under a
		// double-submit the loser must see 409 already-ended, not 404, so it
		// does not retry against a departure date that is already recorded.
		var count int64
		if err := r.writeScoped(ctx, sc, roles).
			Model(&Enrollment{}).
			Where("enrollments.id = ?", id).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrAlreadyEnded
		}
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, roles []string, id uuid.UUID) error {
	res := r.writeScoped(ctx, sc, roles).Delete(&Enrollment{}, "enrollments.id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	var rows []Enrollment
	err := r.readScoped(ctx, sc).
		Where("enrollments.class_id = ?", classID).
		Where("enrollments.started_on <= ? AND (enrollments.ended_on IS NULL OR enrollments.ended_on >= ?)", on, on).
		Order("enrollments.started_on, enrollments.id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *gormRepository) EndOpenEnrollments(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error {
	return r.scoped(ctx, sc).
		Model(&Enrollment{}).
		Where("enrollments.student_id = ? AND enrollments.ended_on IS NULL", studentID).
		Update("ended_on", on).Error
}

func (r *gormRepository) ClassDefaultPrice(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error) {
	var prices []int64
	q := database.FromContext(ctx, r.db).
		Table("classes").
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", classID, sc.CenterID)
	if !sc.CenterWide() {
		// The class anchor and the active giao_vien stint name the same
		// teacher whenever handoff has run cleanly; checking both states the
		// actual rule — the class's current teacher enrolls — and keeps the
		// gate correct if anchor and stint ever diverge.
		q = q.Where(`(teacher_id = ? OR EXISTS (
			SELECT 1 FROM class_staff cs
			WHERE cs.class_id = classes.id
			  AND cs.center_id = classes.center_id
			  AND cs.teacher_id = ?
			  AND cs.role_key = 'giao_vien'
			  AND cs.ended_at IS NULL))`, sc.TeacherID, sc.TeacherID)
	}
	err := q.Pluck("default_unit_price", &prices).Error
	if err != nil {
		return 0, err
	}
	if len(prices) == 0 {
		return 0, ErrClassNotFound
	}
	return prices[0], nil
}

// StudentExists is deliberately center-scoped for every caller: students are
// center assets anchored on the owner, so "one of your students" means one of
// the center's — an enrollment stamped with the caller's anchor may reference
// a student row carrying the owner's.
func (r *gormRepository) StudentExists(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (bool, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("students").
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", studentID, sc.CenterID).
		Count(&n).Error
	return n > 0, err
}

func (r *gormRepository) ClassInCenter(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("classes").
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", classID, sc.CenterID).
		Count(&n).Error
	return n > 0, err
}

func (r *gormRepository) CallerClassStanding(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (assigned, teaching bool, err error) {
	var row struct {
		Assigned bool
		Teaching bool
	}
	err = database.FromContext(ctx, r.db).Raw(`
		SELECT count(*) > 0 AS assigned,
		       count(*) FILTER (WHERE role_key = 'giao_vien' AND ended_at IS NULL) > 0 AS teaching
		FROM class_staff
		WHERE class_id = ? AND center_id = ? AND teacher_id = ?`,
		classID, sc.CenterID, sc.TeacherID).Scan(&row).Error
	return row.Assigned, row.Teaching, err
}

func (r *gormRepository) SearchEnrollableStudents(ctx context.Context, sc authctx.Scope, classID uuid.UUID, q string, limit int) ([]PickerStudent, error) {
	// Escape LIKE metacharacters so a literal % or _ in the query matches
	// itself instead of everything.
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	var rows []PickerStudent
	err := database.FromContext(ctx, r.db).
		Table("students").
		Where("center_id = ? AND deleted_at IS NULL", sc.CenterID).
		Where(`full_name ILIKE ? ESCAPE '\'`, "%"+esc+"%").
		// A student already actively enrolled in this class would only turn
		// into a 409 on pick — they are not enrollable, and each one shown
		// wastes a slot of the picker's small cap.
		Where(`NOT EXISTS (
			SELECT 1 FROM enrollments e
			WHERE e.student_id = students.id AND e.class_id = ?
			  AND e.deleted_at IS NULL AND e.ended_on IS NULL)`, classID).
		Order("full_name, id").
		Limit(limit).
		Select("id, full_name").
		Scan(&rows).Error
	return rows, err
}

// FindByStudentAndClass returns the newest enrollment for the pair regardless
// of whether it is still open. Ordering is explicit so a student who left and
// was re-admitted resolves to their current row rather than an arbitrary one.
func (r *gormRepository) FindByStudentAndClass(ctx context.Context, sc authctx.Scope, studentID, classID uuid.UUID) (*Enrollment, error) {
	var e Enrollment
	err := r.scoped(ctx, sc).
		Where("enrollments.student_id = ? AND enrollments.class_id = ?", studentID, classID).
		Order("enrollments.started_on DESC, enrollments.id DESC").
		Take(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
