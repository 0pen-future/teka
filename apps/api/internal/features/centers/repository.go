package centers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/teachers"
)

// Sentinel errors the service translates into API errors.
var (
	ErrNotFound = errors.New("centers: not found")
	// ErrOwnerHasLiveCenter is the uq_centers_owner violation: the teacher
	// already owns a live center.
	ErrOwnerHasLiveCenter = errors.New("centers: owner already has a live center")
	// ErrActiveMembershipExists is the uq_center_members_active violation: a
	// concurrent transaction opened a membership for the same teacher first.
	ErrActiveMembershipExists = errors.New("centers: teacher already has a live membership")
)

// ScopeRow is the per-request tenant context read straight from the database.
type ScopeRow struct {
	CenterID uuid.UUID
	IsOwner  bool
}

// MemberRow is one member of a center joined with their account phone.
type MemberRow struct {
	ID       uuid.UUID
	FullName string
	Phone    string
	IsOwner  bool
}

// TeacherRow is the slice of a teachers row the remove flow needs.
type TeacherRow struct {
	ID       uuid.UUID
	FullName string
}

// Repository is the persistence contract for centers and memberships; the
// service depends on this interface, tests supply a fake.
type Repository interface {
	// ResolveScope returns the live scope for an active, non-deleted account
	// whose center is itself alive; ErrNotFound covers every dead variant.
	ResolveScope(ctx context.Context, teacherID uuid.UUID) (*ScopeRow, error)
	GetCenter(ctx context.Context, centerID uuid.UUID) (*Center, error)
	GetLiveCenterByOwner(ctx context.Context, ownerID uuid.UUID) (*Center, error)
	ListMembers(ctx context.Context, centerID uuid.UUID) ([]MemberRow, error)
	// GetTeacherInCenter loads a live teacher currently belonging to the
	// center; ErrNotFound also for members of other centers (no existence
	// leak).
	GetTeacherInCenter(ctx context.Context, centerID, teacherID uuid.UUID) (*TeacherRow, error)
	Rename(ctx context.Context, centerID uuid.UUID, name string) error
	// LockLiveCenter takes a FOR UPDATE row lock on a live center so
	// concurrent membership moves serialize; ErrNotFound when the center is
	// already retired. Only meaningful inside a transaction.
	LockLiveCenter(ctx context.Context, centerID uuid.UUID) error
	// CountOtherMembers counts the center's live members besides the given
	// teacher. A teacher whose account was soft-deleted is gone for good and
	// does not count.
	CountOtherMembers(ctx context.Context, centerID, exceptTeacherID uuid.UUID) (int64, error)
	// CountBusinessRows tallies the live rows of the three root business
	// tables (classes, students, contacts); every other business table hangs
	// off one of them, so zero here means the center is empty.
	CountBusinessRows(ctx context.Context, centerID uuid.UUID) (int64, error)
	CreateCenter(ctx context.Context, c *Center) error
	// OpenMembership inserts the live stint, reopening a closed row from an
	// earlier stint in the same center instead of duplicating the key. The
	// caller must have closed the previous live stint first in the same
	// transaction — uq_center_members_active is checked per statement.
	OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) (joinedAt time.Time, err error)
	// CloseMembership stamps left_at on the live stint; ErrNotFound when a
	// concurrent transaction already closed it.
	CloseMembership(ctx context.Context, teacherID, centerID uuid.UUID) error
	// SwitchTeacherCenter moves teachers.center_id from exactly `from` to
	// `to`; ErrNotFound when a concurrent move won the race (from no longer
	// matches).
	SwitchTeacherCenter(ctx context.Context, teacherID, from, to uuid.UUID) error
	// SoftDeleteCenter retires a center; membership history and the data
	// anchored on it stay. It refuses (ErrNotFound) while any live teacher
	// still has the center as their current one — retiring it under them
	// would leave scopes that can never resolve again.
	SoftDeleteCenter(ctx context.Context, centerID, ownerID uuid.UUID) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ResolveScope(ctx context.Context, teacherID uuid.UUID) (*ScopeRow, error) {
	var row ScopeRow
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT t.center_id, (c.owner_id = t.id) AS is_owner
		FROM teachers t
		JOIN centers c ON c.id = t.center_id AND c.deleted_at IS NULL
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = ?
		WHERE t.id = ? AND t.deleted_at IS NULL`,
		teachers.StatusActive, teacherID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *gormRepository) GetCenter(ctx context.Context, centerID uuid.UUID) (*Center, error) {
	var c Center
	err := database.FromContext(ctx, r.db).First(&c, "id = ?", centerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *gormRepository) GetLiveCenterByOwner(ctx context.Context, ownerID uuid.UUID) (*Center, error) {
	var c Center
	err := database.FromContext(ctx, r.db).First(&c, "owner_id = ?", ownerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *gormRepository) ListMembers(ctx context.Context, centerID uuid.UUID) ([]MemberRow, error) {
	var rows []MemberRow
	// Disabled accounts stay on the roster (a temporary lock is still a
	// member); soft-deleted accounts are gone for good and drop off.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT t.id, t.full_name, ua.phone, (c.owner_id = t.id) AS is_owner
		FROM teachers t
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL
		JOIN centers c ON c.id = t.center_id
		WHERE t.center_id = ? AND t.deleted_at IS NULL
		ORDER BY is_owner DESC, t.full_name, t.id`, centerID).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) GetTeacherInCenter(ctx context.Context, centerID, teacherID uuid.UUID) (*TeacherRow, error) {
	var row TeacherRow
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT id, full_name FROM teachers
		WHERE id = ? AND center_id = ? AND deleted_at IS NULL`,
		teacherID, centerID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *gormRepository) Rename(ctx context.Context, centerID uuid.UUID, name string) error {
	res := database.FromContext(ctx, r.db).
		Model(&Center{}).
		Where("id = ?", centerID).
		Update("name", name)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CountOtherMembers(ctx context.Context, centerID, exceptTeacherID uuid.UUID) (int64, error) {
	var n int64
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT COUNT(*)
		FROM teachers t
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL
		WHERE t.center_id = ? AND t.id <> ? AND t.deleted_at IS NULL`,
		centerID, exceptTeacherID).Scan(&n).Error
	return n, err
}

func (r *gormRepository) LockLiveCenter(ctx context.Context, centerID uuid.UUID) error {
	var one int
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT 1 FROM centers WHERE id = ? AND deleted_at IS NULL FOR UPDATE`,
		centerID).Scan(&one)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) CountBusinessRows(ctx context.Context, centerID uuid.UUID) (int64, error) {
	var n int64
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT (SELECT COUNT(*) FROM classes  WHERE center_id = @cid AND deleted_at IS NULL)
		     + (SELECT COUNT(*) FROM students WHERE center_id = @cid AND deleted_at IS NULL)
		     + (SELECT COUNT(*) FROM contacts WHERE center_id = @cid AND deleted_at IS NULL)`,
		map[string]any{"cid": centerID}).Scan(&n).Error
	return n, err
}

func (r *gormRepository) CreateCenter(ctx context.Context, c *Center) error {
	return translateError(database.FromContext(ctx, r.db).Create(c).Error)
}

func (r *gormRepository) OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) (time.Time, error) {
	var joinedAt time.Time
	err := database.FromContext(ctx, r.db).Raw(`
		INSERT INTO center_members (teacher_id, center_id) VALUES (?, ?)
		ON CONFLICT (teacher_id, center_id)
		DO UPDATE SET left_at = NULL, joined_at = now()
		RETURNING joined_at`, teacherID, centerID).Scan(&joinedAt).Error
	if err != nil {
		return time.Time{}, translateError(err)
	}
	return joinedAt, nil
}

func (r *gormRepository) CloseMembership(ctx context.Context, teacherID, centerID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).Exec(`
		UPDATE center_members SET left_at = now()
		WHERE teacher_id = ? AND center_id = ? AND left_at IS NULL`,
		teacherID, centerID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SwitchTeacherCenter(ctx context.Context, teacherID, from, to uuid.UUID) error {
	res := database.FromContext(ctx, r.db).Exec(`
		UPDATE teachers SET center_id = ?, updated_at = now()
		WHERE id = ? AND center_id = ? AND deleted_at IS NULL`,
		to, teacherID, from)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SoftDeleteCenter(ctx context.Context, centerID, ownerID uuid.UUID) error {
	// The NOT EXISTS clause is the last line of defense against retiring a
	// center someone still lives in; teachers whose accounts are soft-deleted
	// no longer count as living there.
	res := database.FromContext(ctx, r.db).Exec(`
		UPDATE centers SET deleted_at = now()
		WHERE id = ? AND owner_id = ? AND deleted_at IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM teachers t
			JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL
			WHERE t.center_id = centers.id AND t.deleted_at IS NULL)`,
		centerID, ownerID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// translateError maps constraint violations onto sentinel errors so callers
// stay driver-agnostic.
func translateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "uq_centers_owner":
			return ErrOwnerHasLiveCenter
		case "uq_center_members_active":
			return ErrActiveMembershipExists
		}
	}
	return err
}
