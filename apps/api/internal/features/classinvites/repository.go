package classinvites

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// Filter narrows List. A nil pointer means "any".
type Filter struct {
	Status    string
	ClassID   *uuid.UUID
	TeacherID *uuid.UUID
}

// Repository is the persistence contract for class invitations. Every read
// and state change is bound to the caller's center id; visibility inside the
// center (owner vs invitee) is decided by the service.
type Repository interface {
	// Create inserts a pending invitation; id and timestamps come from the
	// table defaults. A racing duplicate loses to uq_class_invitations_pending
	// and surfaces as gorm.ErrDuplicatedKey.
	Create(ctx context.Context, inv *Invitation) error
	// Get loads one invitation of the center with its display names.
	Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error)
	// List returns the center's invitations matching the filter, newest first.
	List(ctx context.Context, sc authctx.Scope, f Filter) ([]Row, error)
	// HasPending reports whether the teacher already has a pending
	// invitation on the class — the friendly 409 pre-check.
	HasPending(ctx context.Context, sc authctx.Scope, classID, teacherID uuid.UUID) (bool, error)
	// Transition moves the invitation from one of the `from` statuses to
	// `to`, stamping responded_at (accepted/declined) or assigned_at
	// (assigned). ErrNotFound when no row of the center is in a `from`
	// state — the caller distinguishes "missing" from "changed" with Get.
	Transition(ctx context.Context, sc authctx.Scope, id uuid.UUID, from []string, to string) error
	// Remind stamps reminded_at on a pending invitation; ErrNotFound
	// otherwise.
	Remind(ctx context.Context, sc authctx.Scope, id uuid.UUID) error
	// CancelOpenForMember cancels every pending or accepted invitation of the
	// teacher in the center, returning how many changed. It runs inside the
	// centers remove-member transaction.
	CancelOpenForMember(ctx context.Context, centerID, teacherID uuid.UUID) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

const rowSelect = `SELECT ci.id, ci.center_id, ci.class_id, ci.teacher_id, ci.role_key, ci.status,
	ci.invited_by, ci.message, ci.sent_at, ci.reminded_at, ci.responded_at, ci.assigned_at,
	ci.created_at, ci.updated_at,
	c.name AS class_name, co.name AS course_name, t.full_name AS teacher_name, ib.full_name AS invited_by_name
FROM class_invitations ci
JOIN classes c ON c.id = ci.class_id AND c.deleted_at IS NULL
LEFT JOIN courses co ON co.id = c.course_id AND co.deleted_at IS NULL
JOIN teachers t ON t.id = ci.teacher_id
JOIN teachers ib ON ib.id = ci.invited_by`

func (r *gormRepository) Create(ctx context.Context, inv *Invitation) error {
	err := database.FromContext(ctx, r.db).
		Raw(`INSERT INTO class_invitations (center_id, class_id, teacher_id, role_key, invited_by, message)
			VALUES (?, ?, ?, ?, ?, ?)
			RETURNING id, status, sent_at, created_at, updated_at`,
			inv.CenterID, inv.ClassID, inv.TeacherID, inv.RoleKey, inv.InvitedBy, inv.Message).
		Row().Scan(&inv.ID, &inv.Status, &inv.SentAt, &inv.CreatedAt, &inv.UpdatedAt)
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

func (r *gormRepository) Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	var row Row
	err := database.FromContext(ctx, r.db).
		Raw(rowSelect+` WHERE ci.id = ? AND ci.center_id = ?`, id, sc.CenterID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, f Filter) ([]Row, error) {
	query := rowSelect + ` WHERE ci.center_id = ?`
	args := []any{sc.CenterID}
	if f.Status != "" {
		query += ` AND ci.status = ?`
		args = append(args, f.Status)
	}
	if f.ClassID != nil {
		query += ` AND ci.class_id = ?`
		args = append(args, *f.ClassID)
	}
	if f.TeacherID != nil {
		query += ` AND ci.teacher_id = ?`
		args = append(args, *f.TeacherID)
	}
	query += ` ORDER BY ci.sent_at DESC, ci.id`
	rows := []Row{}
	err := database.FromContext(ctx, r.db).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) HasPending(ctx context.Context, sc authctx.Scope, classID, teacherID uuid.UUID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM class_invitations
			WHERE class_id = ? AND center_id = ? AND teacher_id = ? AND status = 'pending')`,
			classID, sc.CenterID, teacherID).
		Scan(&exists).Error
	return exists, err
}

func (r *gormRepository) Transition(ctx context.Context, sc authctx.Scope, id uuid.UUID, from []string, to string) error {
	stamp := ""
	switch to {
	case StatusAccepted, StatusDeclined:
		stamp = ", responded_at = now()"
	case StatusAssigned:
		stamp = ", assigned_at = now()"
	}
	res := database.FromContext(ctx, r.db).
		Exec(`UPDATE class_invitations SET status = ?, updated_at = now()`+stamp+`
			WHERE id = ? AND center_id = ? AND status IN ?`,
			to, id, sc.CenterID, from)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) Remind(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := database.FromContext(ctx, r.db).
		Exec(`UPDATE class_invitations SET reminded_at = now(), updated_at = now()
			WHERE id = ? AND center_id = ? AND status = 'pending'`,
			id, sc.CenterID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CancelOpenForMember(ctx context.Context, centerID, teacherID uuid.UUID) (int64, error) {
	res := database.FromContext(ctx, r.db).
		Exec(`UPDATE class_invitations SET status = 'cancelled', updated_at = now()
			WHERE center_id = ? AND teacher_id = ? AND status IN ('pending', 'accepted')`,
			centerID, teacherID)
	return res.RowsAffected, res.Error
}
