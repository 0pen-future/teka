package classstaff

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// Repository is the persistence contract for staff assignments. Every method
// is center-scoped through the caller's center id only — never the owner flag
// (the owner gate lives in the service, enforced by scoping_guard_test).
type Repository interface {
	// ClassInCenter reports whether a live (non-deleted) class with this id
	// exists in the caller's center — the existence half of the read gate.
	ClassInCenter(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
	// CallerHasAssignment reports whether the caller holds ANY stint on the
	// class, active or ended — an ended stint still grants history reads.
	CallerHasAssignment(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
	// ListByClass returns the class's stints (active and ended) with teacher
	// display names: active first, then by start time.
	ListByClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]StaffRow, error)
	// GetRow loads one stint of the class with its teacher name; ErrNotFound
	// also covers another class's or another center's stint.
	GetRow(ctx context.Context, sc authctx.Scope, classID, staffID uuid.UUID) (*StaffRow, error)
	// HasActiveAssignment reports whether the teacher holds an active stint on
	// the class — the friendly 409 pre-check; uq_class_staff_active backs it
	// against races.
	HasActiveAssignment(ctx context.Context, sc authctx.Scope, classID, teacherID uuid.UUID) (bool, error)
	// Create inserts a stint. A racing duplicate loses to
	// uq_class_staff_active and surfaces as gorm.ErrDuplicatedKey.
	Create(ctx context.Context, a *Assignment) error
	// Close stamps ended_at on an ACTIVE stint of the center; ErrNotFound when
	// the stint is missing or already ended.
	Close(ctx context.Context, sc authctx.Scope, staffID uuid.UUID) error
	// Delete hard-deletes a stint of the center regardless of state — the
	// void path revoking a mistaken grant; ErrNotFound when missing.
	Delete(ctx context.Context, sc authctx.Scope, staffID uuid.UUID) error

	// RolesByClass returns the teacher's ACTIVE role keys per class for the
	// given class ids, one batch query. Ended stints are excluded: the result
	// describes what the caller currently is (drives my_staff_roles and write
	// affordances), not what they may still read.
	RolesByClass(ctx context.Context, teacherID, centerID uuid.UUID, classIDs []uuid.UUID) (map[uuid.UUID][]string, error)

	// SyncPrimaryTeacher enforces the dual-write invariant for one class:
	// after it returns, exactly one active giao_vien stint exists and belongs
	// to teacherID. It closes drifted giao_vien stints of other teachers,
	// closes the target's active non-giao_vien stint (one active stint per
	// person per class), and inserts the giao_vien stint idempotently. Callers
	// run it inside the transaction that establishes classes.teacher_id — the
	// classes create hook and the handoff — and it deliberately never fails on
	// drift: it repairs whatever state it finds. A target without a LIVE
	// center membership makes the whole call a no-op: a kicked member must
	// never regain class access through this primitive, and the current
	// stint must not be closed in their favor.
	SyncPrimaryTeacher(ctx context.Context, classID, centerID, teacherID uuid.UUID) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ClassInCenter(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM classes
			WHERE id = ? AND center_id = ? AND deleted_at IS NULL)`,
			classID, sc.CenterID).
		Scan(&exists).Error
	return exists, err
}

func (r *gormRepository) CallerHasAssignment(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM class_staff
			WHERE class_id = ? AND center_id = ? AND teacher_id = ?)`,
			classID, sc.CenterID, sc.TeacherID).
		Scan(&exists).Error
	return exists, err
}

func (r *gormRepository) ListByClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]StaffRow, error) {
	var rows []StaffRow
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT cs.id, cs.class_id, cs.center_id, cs.teacher_id, cs.role_key,
			cs.started_at, cs.ended_at, t.full_name AS teacher_name
		FROM class_staff cs
		JOIN teachers t ON t.id = cs.teacher_id
		WHERE cs.class_id = ? AND cs.center_id = ?
		ORDER BY (cs.ended_at IS NOT NULL), cs.started_at, cs.id`,
			classID, sc.CenterID).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) GetRow(ctx context.Context, sc authctx.Scope, classID, staffID uuid.UUID) (*StaffRow, error) {
	var row StaffRow
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT cs.id, cs.class_id, cs.center_id, cs.teacher_id, cs.role_key,
			cs.started_at, cs.ended_at, t.full_name AS teacher_name
		FROM class_staff cs
		JOIN teachers t ON t.id = cs.teacher_id
		WHERE cs.id = ? AND cs.class_id = ? AND cs.center_id = ?`,
			staffID, classID, sc.CenterID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) HasActiveAssignment(ctx context.Context, sc authctx.Scope, classID, teacherID uuid.UUID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM class_staff
			WHERE class_id = ? AND center_id = ? AND teacher_id = ? AND ended_at IS NULL)`,
			classID, sc.CenterID, teacherID).
		Scan(&exists).Error
	return exists, err
}

func (r *gormRepository) Create(ctx context.Context, a *Assignment) error {
	// Raw INSERT so the row's id and started_at come from the table defaults —
	// the DB clock stays the single time source, matching Close and
	// SyncPrimaryTeacher (a GORM Create would write the struct's zero values).
	err := database.FromContext(ctx, r.db).
		Raw(`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
			VALUES (?, ?, ?, ?)
			RETURNING id, started_at`,
			a.ClassID, a.CenterID, a.TeacherID, a.RoleKey).
		Row().Scan(&a.ID, &a.StartedAt)
	if err != nil && isUniqueViolation(err) {
		return gorm.ErrDuplicatedKey
	}
	return err
}

// isUniqueViolation reports a Postgres 23505; raw Row().Scan bypasses GORM's
// error translation, so the mapping to gorm.ErrDuplicatedKey happens here.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *gormRepository) Close(ctx context.Context, sc authctx.Scope, staffID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).
		Exec(`UPDATE class_staff SET ended_at = now()
			WHERE id = ? AND center_id = ? AND ended_at IS NULL`,
			staffID, sc.CenterID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) Delete(ctx context.Context, sc authctx.Scope, staffID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).
		Exec(`DELETE FROM class_staff WHERE id = ? AND center_id = ?`,
			staffID, sc.CenterID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) RolesByClass(ctx context.Context, teacherID, centerID uuid.UUID, classIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string, len(classIDs))
	if len(classIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ClassID uuid.UUID
		RoleKey string
	}
	err := database.FromContext(ctx, r.db).
		Table("class_staff").
		Select("class_id, role_key").
		Where("teacher_id = ? AND center_id = ? AND class_id IN ? AND ended_at IS NULL",
			teacherID, centerID, classIDs).
		Order("role_key").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ClassID] = append(out[row.ClassID], row.RoleKey)
	}
	return out, nil
}

func (r *gormRepository) SyncPrimaryTeacher(ctx context.Context, classID, centerID, teacherID uuid.UUID) error {
	db := database.FromContext(ctx, r.db)
	// Close whatever contradicts "teacherID is the one active giao_vien":
	// another teacher's active giao_vien stint (drift, or the pre-handoff
	// teacher) and the target's own active stint in a different role (one
	// active stint per person per class — uq_class_staff_active would block
	// the insert below otherwise).
	// Both statements are conditional on the target's LIVE membership: a
	// kicked member's sync must be a complete no-op — closing the current
	// stint without inserting theirs would leave the class with no active
	// giao_vien, and inserting would resurrect access their kick revoked.
	err := db.Exec(`UPDATE class_staff SET ended_at = now()
		WHERE class_id = ? AND center_id = ? AND ended_at IS NULL
		  AND ((role_key = 'giao_vien' AND teacher_id <> ?)
		    OR (teacher_id = ? AND role_key <> 'giao_vien'))
		  AND EXISTS (SELECT 1 FROM center_members cm
			WHERE cm.teacher_id = ? AND cm.center_id = class_staff.center_id
			  AND cm.left_at IS NULL)`,
		classID, centerID, teacherID, teacherID, teacherID).Error
	if err != nil {
		return err
	}
	return db.Exec(`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
		SELECT ?, ?, ?, 'giao_vien'
		WHERE EXISTS (SELECT 1 FROM center_members cm
			WHERE cm.teacher_id = ? AND cm.center_id = ? AND cm.left_at IS NULL)
		ON CONFLICT (class_id, teacher_id) WHERE ended_at IS NULL DO NOTHING`,
		classID, centerID, teacherID, teacherID, centerID).Error
}
