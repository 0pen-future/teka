package classes

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
	GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Class, error)
	List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error)
	Update(ctx context.Context, class *Class) error
	Archive(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	CountOpenEnrollments(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error)

	AddSchedule(ctx context.Context, s *Schedule) error
	GetSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) (*Schedule, error)
	UpdateSchedule(ctx context.Context, s *Schedule) error
	SoftDeleteSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error
	// ListEffectiveSchedules returns the class's schedule rows whose
	// [effective_from, effective_to] range intersects [from, to], treating a
	// NULL effective_to as open-ended. This is the contract session
	// generation (plan 03) consumes.
	ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error)
	// FindActiveByName resolves a live, active class by its exact name within
	// the scope. Bulk flows need it to decide create-or-reuse; there is no
	// unique index on classes.name, so this is a lookup, not a constraint.
	FindActiveByName(ctx context.Context, sc authctx.Scope, name string) (*Class, error)
	// ScheduleExists reports whether the class already carries this exact
	// weekly slot, including its effective_from.
	ScheduleExists(ctx context.Context, sc authctx.Scope, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a classes query bound to one center. An owner sees every
// class in their center; a member sees only the rows they created themselves.
// Composite FKs stop cross-center writes; only this filter stops cross-tenant
// reads.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("classes.center_id = ?", sc.CenterID)
	if !sc.IsOwner {
		q = q.Where("classes.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// scopedSchedules is scoped's counterpart for the class_schedules table.
func (r *gormRepository) scopedSchedules(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("class_schedules.center_id = ?", sc.CenterID)
	if !sc.IsOwner {
		q = q.Where("class_schedules.teacher_id = ?", sc.TeacherID)
	}
	return q
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

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Class, error) {
	var class Class
	err := r.scoped(ctx, sc).
		Preload("Schedules", preloadSchedules).
		Take(&class, "classes.id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &class, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	q := r.scoped(ctx, sc).Model(&Class{})
	if filter.Status != "" {
		// The default active-only list matches the idx_classes_teacher
		// partial-index predicate (deleted_at IS NULL AND status = 'active').
		q = q.Where("classes.status = ?", filter.Status)
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

func (r *gormRepository) Archive(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.scoped(ctx, sc).
		Model(&Class{}).
		Where("classes.id = ?", id).
		Update("status", StatusArchived)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.scoped(ctx, sc).Where("classes.id = ?", id).Delete(&Class{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CountOpenEnrollments(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error) {
	var n int64
	q := database.FromContext(ctx, r.db).
		Table("enrollments").
		Where("center_id = ? AND class_id = ? AND ended_on IS NULL AND deleted_at IS NULL", sc.CenterID, classID)
	if !sc.IsOwner {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	err := q.Count(&n).Error
	return n, err
}

func (r *gormRepository) AddSchedule(ctx context.Context, s *Schedule) error {
	return database.FromContext(ctx, r.db).Create(s).Error
}

func (r *gormRepository) GetSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) (*Schedule, error) {
	var s Schedule
	err := r.scopedSchedules(ctx, sc).
		Take(&s, "class_schedules.id = ? AND class_schedules.class_id = ?", scheduleID, classID).Error
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

func (r *gormRepository) SoftDeleteSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error {
	res := r.scopedSchedules(ctx, sc).
		Where("class_schedules.id = ? AND class_schedules.class_id = ?", scheduleID, classID).
		Delete(&Schedule{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *gormRepository) ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	var rows []Schedule
	err := r.scopedSchedules(ctx, sc).
		Where("class_schedules.class_id = ?", classID).
		Where("class_schedules.effective_from <= ? AND (class_schedules.effective_to IS NULL OR class_schedules.effective_to >= ?)", to, from).
		Order("effective_from, weekday, start_time").
		Find(&rows).Error
	return rows, err
}

// FindActiveByName resolves a class by exact name inside the scope. status is
// part of the match: an archived class still has deleted_at IS NULL, so a
// name-only lookup would hand a bulk importer a class the teacher closed last
// term and quietly enrol this year's students into it.
func (r *gormRepository) FindActiveByName(ctx context.Context, sc authctx.Scope, name string) (*Class, error) {
	var class Class
	err := r.scoped(ctx, sc).
		Where("classes.name = ? AND classes.status = ?", name, StatusActive).
		Take(&class).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &class, nil
}

// ScheduleExists reports whether an identical live slot is already on the
// class. effective_from is part of the identity because the same weekday and
// time can legitimately recur after a timetable change closes the old row.
func (r *gormRepository) ScheduleExists(ctx context.Context, sc authctx.Scope, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error) {
	var count int64
	err := r.scopedSchedules(ctx, sc).Model(&Schedule{}).
		Where("class_schedules.class_id = ? AND class_schedules.weekday = ?", classID, weekday).
		Where("class_schedules.start_time = ? AND class_schedules.effective_from = ?", startTime, effectiveFrom).
		Count(&count).Error
	return count > 0, err
}
