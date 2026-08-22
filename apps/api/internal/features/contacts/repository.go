package contacts

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
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
	GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error)
	List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error)
	Update(ctx context.Context, c *Contact) error
	SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	CountActiveStudents(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (int64, error)
	// ListStudentNames returns up to limit names of live students referencing
	// the contact, alphabetically, for the delete-blocked error message.
	ListStudentNames(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, limit int) ([]string, error)
	// UpdateZaloMapping binds the contact to one Zalo friend; both columns are
	// written together. ErrNotFound when the contact is missing, deleted, or
	// out of scope.
	UpdateZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, zaloUserID, zaloName string) error
	// ClearZaloMapping nulls both mapping columns. Clearing an unmapped
	// contact succeeds; a missing contact is still ErrNotFound.
	ClearZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) error
	// FindIDByPhone resolves a live contact by its exact E.164 phone within
	// the scope — the same shape as uq_contacts_phone(teacher_id, phone).
	FindIDByPhone(ctx context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a query bound to one center. An owner sees every contact in
// their center; a member sees only the rows they created themselves. Composite
// FKs stop cross-center writes; only this filter stops cross-tenant reads.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("contacts.center_id = ?", sc.CenterID)
	if !sc.IsOwner {
		q = q.Where("contacts.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// withStudentCount selects contacts joined against a grouped subquery of live
// students, exposing student_count without a per-row query.
func (r *gormRepository) withStudentCount(ctx context.Context, sc authctx.Scope) *gorm.DB {
	counts := database.FromContext(ctx, r.db).
		Table("students").
		Select("contact_id, COUNT(*) AS n")
	counts = counts.Where("center_id = ? AND deleted_at IS NULL", sc.CenterID)
	if !sc.IsOwner {
		counts = counts.Where("teacher_id = ?", sc.TeacherID)
	}
	counts = counts.Group("contact_id")
	return r.scoped(ctx, sc).
		Model(&Contact{}).
		Select("contacts.*, COALESCE(sc.n, 0) AS student_count").
		Joins("LEFT JOIN (?) sc ON sc.contact_id = contacts.id", counts)
}

func (r *gormRepository) Create(ctx context.Context, c *Contact) error {
	return translateError(database.FromContext(ctx, r.db).Create(c).Error)
}

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	var row Row
	err := r.withStudentCount(ctx, sc).Where("contacts.id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	q := r.withStudentCount(ctx, sc)
	base := r.scoped(ctx, sc).Model(&Contact{})
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

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.scoped(ctx, sc).Where("id = ?", id).Delete(&Contact{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CountActiveStudents(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (int64, error) {
	var n int64
	q := database.FromContext(ctx, r.db).
		Table("students").
		Where("center_id = ? AND contact_id = ? AND deleted_at IS NULL", sc.CenterID, contactID)
	if !sc.IsOwner {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	err := q.Count(&n).Error
	return n, err
}

func (r *gormRepository) ListStudentNames(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, limit int) ([]string, error) {
	var names []string
	q := database.FromContext(ctx, r.db).
		Table("students").
		Where("center_id = ? AND contact_id = ? AND deleted_at IS NULL", sc.CenterID, contactID)
	if !sc.IsOwner {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	err := q.Order("full_name").Limit(limit).Pluck("full_name", &names).Error
	return names, err
}

func (r *gormRepository) UpdateZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, zaloUserID, zaloName string) error {
	return r.setZaloMapping(ctx, sc, contactID, &zaloUserID, &zaloName)
}

func (r *gormRepository) ClearZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) error {
	return r.setZaloMapping(ctx, sc, contactID, nil, nil)
}

// setZaloMapping writes both mapping columns in one scope-bound UPDATE;
// RowsAffected 0 means the contact is missing, deleted, or out of scope.
func (r *gormRepository) setZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, zaloUserID, zaloName *string) error {
	res := r.scoped(ctx, sc).
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

// FindIDByPhone resolves a contact by exact phone. It is deliberately not the
// Query filter on List, which is an ILIKE '%...%' search built for a person
// typing into a search box: as an identity lookup that would match any contact
// whose number merely contains this one.
func (r *gormRepository) FindIDByPhone(ctx context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error) {
	// Scanning into a bare uuid.UUID would skip its sql.Scanner and hit
	// GORM's element-wise array path ([16]byte); the id has to land in a
	// struct field.
	var row struct{ ID uuid.UUID }
	err := r.scoped(ctx, sc).Model(&Contact{}).
		Where("contacts.phone = ?", phone).
		Limit(1).
		Select("contacts.id").
		Scan(&row).Error
	if err != nil {
		return uuid.Nil, false, err
	}
	return row.ID, row.ID != uuid.Nil, nil
}
