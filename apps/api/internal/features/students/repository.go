package students

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the student list. ClassID filters through open
// enrollments — the attendance screen's view of a class roster. Unenrolled
// keeps only students with no open enrollment in any class — the roster's
// "Chưa ghi danh" tab.
type ListFilter struct {
	Query      string
	ContactID  uuid.UUID
	ClassID    uuid.UUID
	Unenrolled bool
}

// Row is a student joined with its contact's name and phone, so the roster
// screen needs no second call.
type Row struct {
	Student      `gorm:"embedded"`
	ContactName  string
	ContactPhone string
}

// Repository is the persistence contract for students; the service depends on
// this interface, tests supply a fake.
type Repository interface {
	Create(ctx context.Context, s *Student) error
	GetByID(ctx context.Context, teacherID, studentID uuid.UUID) (*Row, error)
	List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error)
	Update(ctx context.Context, s *Student) error
	// ContactExists reports whether the contact is live under this teacher —
	// the clean-422 check in front of the composite-FK safety net.
	ContactExists(ctx context.Context, teacherID, contactID uuid.UUID) (bool, error)
	// AnonymizeAndDelete scrubs PII and hides the row in one scoped UPDATE:
	// full_name becomes the placeholder, display_note goes NULL, and both
	// anonymized_at and deleted_at are stamped. The row itself survives so
	// financial foreign keys keep holding. Runs on the context transaction so
	// the caller can close enrollments atomically alongside it.
	AnonymizeAndDelete(ctx context.Context, teacherID, studentID uuid.UUID, placeholder string) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a query bound to one tenant. Every read is scoped to one
// teacher and skips soft-deleted rows (via gorm.DeletedAt on model queries —
// raw SQL and Table() queries must add deleted_at IS NULL by hand). The
// teacher_id column is qualified because reads here join contacts, which also
// carries one. Composite FKs stop cross-teacher writes; only this filter stops
// cross-teacher reads.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("students.teacher_id = ?", teacherID)
}

// withContact joins the owning contact and selects its name and phone
// alongside the student columns.
func withContact(q *gorm.DB) *gorm.DB {
	return q.
		Joins("JOIN contacts ON contacts.id = students.contact_id AND contacts.teacher_id = students.teacher_id").
		Select("students.*, contacts.full_name AS contact_name, contacts.phone AS contact_phone")
}

func (r *gormRepository) Create(ctx context.Context, s *Student) error {
	err := database.FromContext(ctx, r.db).Create(s).Error
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		// The composite FK (contact_id, teacher_id) refused the insert — the
		// contact is not this teacher's. Reached only when the pre-check raced.
		return ErrContactNotOwned
	}
	return err
}

func (r *gormRepository) GetByID(ctx context.Context, teacherID, studentID uuid.UUID) (*Row, error) {
	var row Row
	err := withContact(r.scoped(ctx, teacherID).Model(&Student{})).
		Where("students.id = ?", studentID).
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
	q := r.scoped(ctx, teacherID).Model(&Student{})
	if filter.Query != "" {
		q = q.Where("students.full_name ILIKE ?", "%"+filter.Query+"%")
	}
	if filter.ContactID != uuid.Nil {
		q = q.Where("students.contact_id = ?", filter.ContactID)
	}
	if filter.ClassID != uuid.Nil {
		// Open enrollment in the class; the partial unique index guarantees at
		// most one per student, so the join cannot duplicate rows.
		q = q.Joins(
			"JOIN enrollments ON enrollments.student_id = students.id"+
				" AND enrollments.class_id = ? AND enrollments.ended_on IS NULL AND enrollments.deleted_at IS NULL",
			filter.ClassID)
	}
	if filter.Unenrolled {
		q = q.Where("NOT EXISTS (SELECT 1 FROM enrollments" +
			" WHERE enrollments.student_id = students.id" +
			" AND enrollments.ended_on IS NULL AND enrollments.deleted_at IS NULL)")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	if err := withContact(q).Scopes(p.Scope).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, s *Student) error {
	err := database.FromContext(ctx, r.db).Save(s).Error
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return ErrContactNotOwned
	}
	return err
}

func (r *gormRepository) ContactExists(ctx context.Context, teacherID, contactID uuid.UUID) (bool, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("contacts").
		Where("id = ? AND teacher_id = ? AND deleted_at IS NULL", contactID, teacherID).
		Count(&n).Error
	return n > 0, err
}

func (r *gormRepository) AnonymizeAndDelete(ctx context.Context, teacherID, studentID uuid.UUID, placeholder string) error {
	res := database.FromContext(ctx, r.db).
		Model(&Student{}).
		Where("id = ? AND teacher_id = ?", studentID, teacherID).
		Updates(map[string]any{
			"full_name":     placeholder,
			"display_note":  nil,
			"anonymized_at": gorm.Expr("now()"),
			"deleted_at":    gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
