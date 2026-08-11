package enrollments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
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
	GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Row, error)
	List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error)
	// End stamps ended_on on one open enrollment; the row survives.
	End(ctx context.Context, teacherID, id uuid.UUID, endedOn time.Time) error
	SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error
	// ActiveOn returns the enrollments that should appear on a class's
	// attendance sheet for a given date: started on or before it, and not
	// ended before it. Both boundaries are inclusive — a student whose
	// started_on equals the date attends that session, and a student whose
	// ended_on equals it attends their last one. An exclusive boundary would
	// silently lose one session of revenue per student per departure.
	ActiveOn(ctx context.Context, teacherID, classID uuid.UUID, on time.Time) ([]Enrollment, error)
	// EndOpenEnrollments closes every open enrollment the student holds,
	// effective on the given date — the students feature calls this while
	// anonymising a deleted student.
	EndOpenEnrollments(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error
	// ClassDefaultPrice reads the class's default_unit_price — the value
	// copied onto new enrollments — returning ErrClassNotFound for a missing
	// or foreign class.
	ClassDefaultPrice(ctx context.Context, teacherID, classID uuid.UUID) (int64, error)
	StudentExists(ctx context.Context, teacherID, studentID uuid.UUID) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a query bound to one tenant. The teacher_id column is
// qualified because list queries join students and classes, which carry the
// same column name. Soft-deleted rows are skipped via gorm.DeletedAt on model
// queries — raw SQL and Table() queries must add deleted_at IS NULL by hand.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("enrollments.teacher_id = ?", teacherID)
}

// withNames joins the display names onto an enrollment query. Same-teacher
// join conditions keep the composite-key discipline even though the FKs
// already guarantee it.
func withNames(q *gorm.DB) *gorm.DB {
	return q.
		Joins("JOIN students ON students.id = enrollments.student_id AND students.teacher_id = enrollments.teacher_id").
		Joins("JOIN classes ON classes.id = enrollments.class_id AND classes.teacher_id = enrollments.teacher_id").
		Select("enrollments.*, students.full_name AS student_name, classes.name AS class_name")
}

func (r *gormRepository) Create(ctx context.Context, e *Enrollment) error {
	err := database.FromContext(ctx, r.db).Create(e).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrAlreadyEnrolled
	}
	return err
}

func (r *gormRepository) GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Row, error) {
	var row Row
	err := withNames(r.scoped(ctx, teacherID).Model(&Enrollment{})).
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

func (r *gormRepository) List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	q := r.scoped(ctx, teacherID).Model(&Enrollment{})
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

func (r *gormRepository) End(ctx context.Context, teacherID, id uuid.UUID, endedOn time.Time) error {
	res := r.scoped(ctx, teacherID).
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
		if err := r.scoped(ctx, teacherID).
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

func (r *gormRepository) SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error {
	res := r.scoped(ctx, teacherID).Delete(&Enrollment{}, "enrollments.id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ActiveOn(ctx context.Context, teacherID, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	var rows []Enrollment
	err := r.scoped(ctx, teacherID).
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
	q := database.FromContext(ctx, r.db).Where("enrollments.center_id = ?", sc.CenterID)
	if !sc.IsOwner {
		q = q.Where("enrollments.teacher_id = ?", sc.TeacherID)
	}
	return q.
		Model(&Enrollment{}).
		Where("enrollments.student_id = ? AND enrollments.ended_on IS NULL", studentID).
		Update("ended_on", on).Error
}

func (r *gormRepository) ClassDefaultPrice(ctx context.Context, teacherID, classID uuid.UUID) (int64, error) {
	var prices []int64
	err := database.FromContext(ctx, r.db).
		Table("classes").
		Where("id = ? AND teacher_id = ? AND deleted_at IS NULL", classID, teacherID).
		Pluck("default_unit_price", &prices).Error
	if err != nil {
		return 0, err
	}
	if len(prices) == 0 {
		return 0, ErrClassNotFound
	}
	return prices[0], nil
}

func (r *gormRepository) StudentExists(ctx context.Context, teacherID, studentID uuid.UUID) (bool, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("students").
		Where("id = ? AND teacher_id = ? AND deleted_at IS NULL", studentID, teacherID).
		Count(&n).Error
	return n > 0, err
}
