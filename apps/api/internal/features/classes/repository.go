package classes

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the class list. Status is one of StatusActive,
// StatusArchived, or "" for every non-deleted class regardless of status.
type ListFilter struct {
	Status string
}

// Repository is the persistence contract for classes and their schedules; the
// service depends on this interface, tests supply a fake.
type Repository interface {
	// CreateWithSchedules inserts the class and its schedule rows on the
	// context's transaction; the caller wraps it in WithinTx for atomicity.
	CreateWithSchedules(ctx context.Context, class *Class, schedules []Schedule) error
	GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Class, error)
	List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Class, int64, error)
	Update(ctx context.Context, class *Class) error
	Archive(ctx context.Context, teacherID, id uuid.UUID) error
	SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error
	CountOpenEnrollments(ctx context.Context, teacherID, classID uuid.UUID) (int64, error)

	AddSchedule(ctx context.Context, s *Schedule) error
	GetSchedule(ctx context.Context, teacherID, classID, scheduleID uuid.UUID) (*Schedule, error)
	UpdateSchedule(ctx context.Context, s *Schedule) error
	SoftDeleteSchedule(ctx context.Context, teacherID, classID, scheduleID uuid.UUID) error
	// ListEffectiveSchedules returns the class's schedule rows whose
	// [effective_from, effective_to] range intersects [from, to], treating a
	// NULL effective_to as open-ended. This is the contract session
	// generation (plan 03) consumes.
	ListEffectiveSchedules(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Schedule, error)
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

// preloadSchedules orders schedule rows deterministically for display.
func preloadSchedules(db *gorm.DB) *gorm.DB {
	return db.Order("effective_from, weekday, start_time")
}

func (r *gormRepository) CreateWithSchedules(ctx context.Context, class *Class, schedules []Schedule) error {
	db := database.FromContext(ctx, r.db)
	// Omit the association: schedule rows are inserted explicitly below with
	// ids and teacher ids already set.
	if err := db.Omit("Schedules").Create(class).Error; err != nil {
		return err
	}
	if len(schedules) == 0 {
		return nil
	}
	return db.Create(&schedules).Error
}

func (r *gormRepository) GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Class, error) {
	var class Class
	err := r.scoped(ctx, teacherID).
		Preload("Schedules", preloadSchedules).
		Take(&class, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &class, nil
}

func (r *gormRepository) List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	q := r.scoped(ctx, teacherID).Model(&Class{})
	if filter.Status != "" {
		// The default active-only list matches the idx_classes_teacher
		// partial-index predicate (deleted_at IS NULL AND status = 'active').
		q = q.Where("status = ?", filter.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Class
	err := q.Preload("Schedules", preloadSchedules).Scopes(p.Scope).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, class *Class) error {
	return database.FromContext(ctx, r.db).Omit("Schedules").Save(class).Error
}

func (r *gormRepository) Archive(ctx context.Context, teacherID, id uuid.UUID) error {
	res := r.scoped(ctx, teacherID).
		Model(&Class{}).
		Where("id = ?", id).
		Update("status", StatusArchived)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error {
	res := r.scoped(ctx, teacherID).Where("id = ?", id).Delete(&Class{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CountOpenEnrollments(ctx context.Context, teacherID, classID uuid.UUID) (int64, error) {
	var n int64
	err := database.FromContext(ctx, r.db).
		Table("enrollments").
		Where("teacher_id = ? AND class_id = ? AND ended_on IS NULL AND deleted_at IS NULL", teacherID, classID).
		Count(&n).Error
	return n, err
}

func (r *gormRepository) AddSchedule(ctx context.Context, s *Schedule) error {
	return database.FromContext(ctx, r.db).Create(s).Error
}

func (r *gormRepository) GetSchedule(ctx context.Context, teacherID, classID, scheduleID uuid.UUID) (*Schedule, error) {
	var s Schedule
	err := r.scoped(ctx, teacherID).
		Take(&s, "id = ? AND class_id = ?", scheduleID, classID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrScheduleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *gormRepository) UpdateSchedule(ctx context.Context, s *Schedule) error {
	return database.FromContext(ctx, r.db).Save(s).Error
}

func (r *gormRepository) SoftDeleteSchedule(ctx context.Context, teacherID, classID, scheduleID uuid.UUID) error {
	res := r.scoped(ctx, teacherID).
		Where("id = ? AND class_id = ?", scheduleID, classID).
		Delete(&Schedule{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *gormRepository) ListEffectiveSchedules(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	var rows []Schedule
	err := r.scoped(ctx, teacherID).
		Where("class_id = ?", classID).
		Where("effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)", to, from).
		Order("effective_from, weekday, start_time").
		Find(&rows).Error
	return rows, err
}
