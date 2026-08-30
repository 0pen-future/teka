package zalo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/classscope"
)

// ErrAccountNotFound reports that a teacher has no linked Zalo account — the
// normal state for every teacher who has not scanned a QR code yet, not a
// failure.
var ErrAccountNotFound = errors.New("zalo: account not found")

// ErrConsentVersionRequired rejects an Upsert that carries no acknowledged
// consent version. Linking hands a teacher's own Zalo session to this system,
// so a stored row must always name the consent text they agreed to.
var ErrConsentVersionRequired = errors.New("zalo: consent version is required")

// Repository is the persistence contract for linked Zalo accounts; callers
// depend on this interface, tests supply a fake.
type Repository interface {
	// Upsert writes the teacher's linked account, replacing any previous one —
	// including a soft-deleted row, which relinking revives. acc.ConsentVersion
	// must be set. Status, ConsentAt, and LinkedAt default when left zero.
	Upsert(ctx context.Context, acc *Account) error
	// GetByTeacher returns the teacher's linked account, or ErrAccountNotFound
	// when there is none. A soft-deleted account counts as none.
	GetByTeacher(ctx context.Context, teacherID uuid.UUID) (*Account, error)
	// Delete soft-deletes the teacher's account, unlinking it. It returns
	// ErrAccountNotFound when there was nothing to unlink.
	Delete(ctx context.Context, teacherID uuid.UUID) error
	// UpdateStatus moves a live account between StatusLinked and StatusExpired.
	// It returns ErrAccountNotFound when the teacher has no live account.
	UpdateStatus(ctx context.Context, teacherID uuid.UUID, status string) error
	// MarkVerified stamps last_verified_at, recording that Zalo accepted the
	// stored credentials just now. It returns ErrAccountNotFound when the
	// teacher has no live account.
	MarkVerified(ctx context.Context, teacherID uuid.UUID) error
	// ListLinked returns the teachers whose account is currently StatusLinked —
	// the set the health probe sweeps.
	ListLinked(ctx context.Context) ([]uuid.UUID, error)
	// HasActiveHocVu reports whether the teacher holds an active hoc_vu stint
	// on any live class in the center — the gate that opens the scoped
	// friend-match to non-oversight staff.
	HasActiveHocVu(ctx context.Context, teacherID, centerID uuid.UUID) (bool, error)
	// ReachableContactPhones returns the phones of every contact the teacher's
	// active hoc_vu stints make phone-visible (the classscope contact rule).
	// These are the only phones a non-oversight caller may send to Zalo.
	ReachableContactPhones(ctx context.Context, teacherID, centerID uuid.UUID) ([]string, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Upsert(ctx context.Context, acc *Account) error {
	if acc.ConsentVersion == "" {
		return ErrConsentVersionRequired
	}
	// The column defaults in the migration document intent but cannot fire:
	// GORM sends every mapped field, so a zero time.Time would be written as
	// year 1 rather than now(). Fill them here instead.
	now := time.Now()
	if acc.Status == "" {
		acc.Status = StatusLinked
	}
	if acc.ConsentAt.IsZero() {
		acc.ConsentAt = now
	}
	if acc.LinkedAt.IsZero() {
		acc.LinkedAt = now
	}
	// Clearing deleted_at is what turns a re-link into a revival rather than a
	// primary-key collision against an unlinked row.
	acc.DeletedAt = gorm.DeletedAt{}

	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "teacher_id"}},
			UpdateAll: true,
		}).
		Create(acc).Error
}

func (r *gormRepository) GetByTeacher(ctx context.Context, teacherID uuid.UUID) (*Account, error) {
	var acc Account
	err := database.FromContext(ctx, r.db).
		Where("teacher_id = ?", teacherID).
		First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *gormRepository) Delete(ctx context.Context, teacherID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).
		Where("teacher_id = ?", teacherID).
		Delete(&Account{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, teacherID uuid.UUID, status string) error {
	return r.update(ctx, teacherID, map[string]any{
		"status":     status,
		"updated_at": gorm.Expr("now()"),
	})
}

func (r *gormRepository) MarkVerified(ctx context.Context, teacherID uuid.UUID) error {
	return r.update(ctx, teacherID, map[string]any{
		"last_verified_at": gorm.Expr("now()"),
		"updated_at":       gorm.Expr("now()"),
	})
}

func (r *gormRepository) update(ctx context.Context, teacherID uuid.UUID, values map[string]any) error {
	res := database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("teacher_id = ? AND deleted_at IS NULL", teacherID).
		Updates(values)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (r *gormRepository) HasActiveHocVu(ctx context.Context, teacherID, centerID uuid.UUID) (bool, error) {
	var ok bool
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM class_staff cs
			JOIN classes c ON c.id = cs.class_id
			  AND c.center_id = cs.center_id
			  AND c.deleted_at IS NULL
			WHERE cs.teacher_id = ?
			  AND cs.center_id = ?
			  AND cs.role_key = 'hoc_vu'
			  AND cs.ended_at IS NULL)`,
		teacherID, centerID).Scan(&ok).Error
	return ok, err
}

func (r *gormRepository) ReachableContactPhones(ctx context.Context, teacherID, centerID uuid.UUID) ([]string, error) {
	frag, _ := classscope.PhoneVisibleViaContact("contacts.id")
	var phones []string
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT contacts.phone FROM contacts
		WHERE contacts.center_id = ?
		  AND contacts.deleted_at IS NULL
		  AND `+frag,
		centerID, teacherID, centerID).Scan(&phones).Error
	if err != nil {
		return nil, err
	}
	return phones, nil
}

func (r *gormRepository) ListLinked(ctx context.Context) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("status = ?", StatusLinked).
		Order("teacher_id").
		Pluck("teacher_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}
