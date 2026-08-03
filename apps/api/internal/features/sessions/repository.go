package sessions

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
)

// Row is a session joined with the class name the responses display.
type Row struct {
	Session   `gorm:"embedded"`
	ClassName string
}

// Repository is the persistence contract for sessions; the service depends on
// this interface, tests supply a fake.
type Repository interface {
	// BulkInsertIgnoreConflicts inserts rows, silently skipping any whose
	// (class_id, session_date) already exists among non-deleted rows — the
	// idempotency guarantee behind on-demand generation. Two concurrent
	// generations racing the same range both submit the same candidate rows;
	// Postgres, not application logic, decides which one lands.
	BulkInsertIgnoreConflicts(ctx context.Context, rows []Session) error
	// Create inserts a single ad-hoc session, translating the partial unique
	// index violation into ErrSessionExists so the caller sees a clean 409
	// instead of a silently dropped insert.
	Create(ctx context.Context, s *Session) error
	ListByClassAndRange(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Row, error)
	GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Row, error)
	// UpdateStatus transitions a session's status and cancel_reason in one
	// statement; a nil cancelReason clears the column. Every current caller
	// supplies the full desired value, so there is no "leave untouched" case.
	UpdateStatus(ctx context.Context, teacherID, id uuid.UUID, status string, cancelReason *string) error
	SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error
	// MarkHeldAndConfirmed transitions a session to held and stamps
	// attendance_confirmed_at in one statement — the transition attendance
	// confirmation performs implicitly, so confirming attendance never
	// requires a second button press.
	MarkHeldAndConfirmed(ctx context.Context, teacherID, id uuid.UUID, at time.Time) error
	// ListPending returns sessions strictly before `before` that are still
	// unconfirmed and planned or held (cancelled sessions never qualify),
	// optionally bounded by [from, to] (both inclusive), newest first. total
	// is the unlimited count; the returned rows respect limit. The expected
	// student count comes from one grouped join over enrollments, never a
	// per-row lookup — see pending.go.
	ListPending(ctx context.Context, teacherID uuid.UUID, before time.Time, from, to *time.Time, limit int) ([]PendingRow, int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a query bound to one tenant, qualified because list queries
// join classes, which carries the same teacher_id column name.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("class_sessions.teacher_id = ?", teacherID)
}

// withClassName joins the display name onto a session query. The same-teacher
// join condition keeps the composite-key discipline even though the FK
// already guarantees it.
func withClassName(q *gorm.DB) *gorm.DB {
	return q.
		Joins("JOIN classes ON classes.id = class_sessions.class_id AND classes.teacher_id = class_sessions.teacher_id").
		Select("class_sessions.*, classes.name AS class_name")
}

func (r *gormRepository) BulkInsertIgnoreConflicts(ctx context.Context, rows []Session) error {
	if len(rows) == 0 {
		return nil
	}
	// TargetWhere reproduces the partial index's predicate
	// (uq_class_sessions_per_day is WHERE deleted_at IS NULL) as the ON
	// CONFLICT target; without it Postgres cannot match the index and the
	// insert fails outright instead of skipping the conflicting rows.
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "class_id"}, {Name: "session_date"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
			DoNothing:   true,
		}).
		Create(&rows).Error
}

func (r *gormRepository) Create(ctx context.Context, s *Session) error {
	err := database.FromContext(ctx, r.db).Create(s).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrSessionExists
	}
	return err
}

func (r *gormRepository) ListByClassAndRange(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Row, error) {
	var rows []Row
	err := withClassName(r.scoped(ctx, teacherID).Model(&Session{})).
		Where("class_sessions.class_id = ?", classID).
		Where("class_sessions.session_date BETWEEN ? AND ?", from, to).
		Order("class_sessions.session_date").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetByID(ctx context.Context, teacherID, id uuid.UUID) (*Row, error) {
	var row Row
	err := withClassName(r.scoped(ctx, teacherID).Model(&Session{})).
		Where("class_sessions.id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, teacherID, id uuid.UUID, status string, cancelReason *string) error {
	res := r.scoped(ctx, teacherID).
		Model(&Session{}).
		Where("class_sessions.id = ?", id).
		Updates(map[string]any{"status": status, "cancel_reason": cancelReason})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDelete(ctx context.Context, teacherID, id uuid.UUID) error {
	res := r.scoped(ctx, teacherID).Where("class_sessions.id = ?", id).Delete(&Session{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) MarkHeldAndConfirmed(ctx context.Context, teacherID, id uuid.UUID, at time.Time) error {
	res := r.scoped(ctx, teacherID).
		Model(&Session{}).
		Where("class_sessions.id = ?", id).
		Updates(map[string]any{"status": StatusHeld, "attendance_confirmed_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
