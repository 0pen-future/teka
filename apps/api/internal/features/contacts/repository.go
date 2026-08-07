package contacts

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the contact list. Query matches full_name or phone
// case-insensitively.
type ListFilter struct {
	Query string
}

// Row is a contact plus its live-student count, produced in a single statement
// so listing 150 contacts never becomes an N+1.
type Row struct {
	Contact      `gorm:"embedded"`
	StudentCount int64
}

// Repository is the persistence contract for contacts; the service depends on
// this interface, tests supply a fake.
type Repository interface {
	Create(ctx context.Context, c *Contact) error
	GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Row, error)
	List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error)
	Update(ctx context.Context, c *Contact) error
	SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error
	CountActiveStudents(ctx context.Context, teacherID, contactID uuid.UUID) (int64, error)
	// ListStudentNames returns up to limit names of live students referencing
	// the contact, alphabetically, for the delete-blocked error message.
	ListStudentNames(ctx context.Context, teacherID, contactID uuid.UUID, limit int) ([]string, error)
	// UpdateZaloMapping binds the contact to one Zalo friend; both columns are
	// written together. ErrNotFound when the contact is missing, deleted, or
	// another teacher's.
	UpdateZaloMapping(ctx context.Context, teacherID, contactID uuid.UUID, zaloUserID, zaloName string) error
	// ClearZaloMapping nulls both mapping columns. Clearing an unmapped
	// contact succeeds; a missing contact is still ErrNotFound.
	ClearZaloMapping(ctx context.Context, teacherID, contactID uuid.UUID) error
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
// raw SQL and Table() queries must add deleted_at IS NULL by hand). Composite
// FKs stop cross-teacher writes; only this filter stops cross-teacher reads.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("teacher_id = ?", teacherID)
}

// withStudentCount selects contacts joined against a grouped subquery of live
// students, exposing student_count without a per-row query.
func (r *gormRepository) withStudentCount(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	counts := database.FromContext(ctx, r.db).
		Table("students").
		Select("contact_id, COUNT(*) AS n").
		Where("teacher_id = ? AND deleted_at IS NULL", teacherID).
		Group("contact_id")
	return r.scoped(ctx, teacherID).
		Model(&Contact{}).
		Select("contacts.*, COALESCE(sc.n, 0) AS student_count").
		Joins("LEFT JOIN (?) sc ON sc.contact_id = contacts.id", counts)
}

func (r *gormRepository) Create(ctx context.Context, c *Contact) error {
	return translateError(database.FromContext(ctx, r.db).Create(c).Error)
}

func (r *gormRepository) GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Row, error) {
	var row Row
	err := r.withStudentCount(ctx, teacherID).Where("contacts.id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	q := r.withStudentCount(ctx, teacherID)
	base := r.scoped(ctx, teacherID).Model(&Contact{})
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		const cond = "(contacts.full_name ILIKE ? OR contacts.phone ILIKE ?)"
		q = q.Where(cond, like, like)
		base = base.Where(cond, like, like)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	if err := q.Scopes(p.Scope).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, c *Contact) error {
	return translateError(database.FromContext(ctx, r.db).Save(c).Error)
}

func (r *gormRepository) SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error {
	res := r.scoped(ctx, teacherID).Where("id = ?", id).Delete(&Contact{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CountActiveStudents(ctx context.Context, teacherID, contactID uuid.UUID) (int64, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("students").
		Where("teacher_id = ? AND contact_id = ? AND deleted_at IS NULL", teacherID, contactID).
		Count(&n).Error
	return n, err
}

func (r *gormRepository) ListStudentNames(ctx context.Context, teacherID, contactID uuid.UUID, limit int) ([]string, error) {
	var names []string
	err := database.FromContext(ctx, r.db).
		Table("students").
		Where("teacher_id = ? AND contact_id = ? AND deleted_at IS NULL", teacherID, contactID).
		Order("full_name").
		Limit(limit).
		Pluck("full_name", &names).Error
	return names, err
}

func (r *gormRepository) UpdateZaloMapping(ctx context.Context, teacherID, contactID uuid.UUID, zaloUserID, zaloName string) error {
	return r.setZaloMapping(ctx, teacherID, contactID, &zaloUserID, &zaloName)
}

func (r *gormRepository) ClearZaloMapping(ctx context.Context, teacherID, contactID uuid.UUID) error {
	return r.setZaloMapping(ctx, teacherID, contactID, nil, nil)
}

// setZaloMapping writes both mapping columns in one tenant-scoped UPDATE;
// RowsAffected 0 means the contact is missing, deleted, or another teacher's.
func (r *gormRepository) setZaloMapping(ctx context.Context, teacherID, contactID uuid.UUID, zaloUserID, zaloName *string) error {
	res := r.scoped(ctx, teacherID).
		Model(&Contact{}).
		Where("id = ?", contactID).
		Updates(map[string]any{"zalo_user_id": zaloUserID, "zalo_name": zaloName})
	if res.Error != nil {
		return translateError(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// translateError maps unique index violations onto domain errors so callers
// stay driver-agnostic. Two unique indexes live on contacts, so the constraint
// name decides which; the phone index stays the default because callers that
// insert or save a contact can only trip that one.
func translateError(err error) error {
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "uq_contacts_zalo_user" {
		return ErrDuplicateZaloMapping
	}
	return ErrDuplicatePhone
}
