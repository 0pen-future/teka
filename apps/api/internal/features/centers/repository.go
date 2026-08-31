package centers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/authctx"
)

// Sentinel errors the service translates into API errors.
var (
	ErrNotFound = errors.New("centers: not found")
	// ErrStaleVersion is a CAS miss on a permission-assignment write: the
	// caller's assignment_version no longer matches the row, so another write
	// landed since their read. Nothing was mutated.
	ErrStaleVersion = errors.New("centers: stale assignment version")
	// ErrOwnerHasLiveCenter is the uq_centers_owner violation: the teacher
	// already owns a live center.
	ErrOwnerHasLiveCenter = errors.New("centers: owner already has a live center")
	// ErrActiveMembershipExists is the uq_center_members_active violation: a
	// concurrent transaction opened a membership for the same teacher first.
	ErrActiveMembershipExists = errors.New("centers: teacher already has a live membership")
)

// ScopeRow is the per-request tenant context read straight from the database.
// The permission lists arrive comma-joined (string_agg) rather than as
// Postgres arrays — the stdlib pgx driver GORM sits on cannot scan text[]
// into []string; keys are dot-separated identifiers, so ',' never collides.
type ScopeRow struct {
	CenterID  uuid.UUID
	IsOwner   bool
	RolePerms string
	Grants    string
	Denies    string
}

// OwnerRow is the current center owner of one teacher, and whether that
// teacher IS the owner.
type OwnerRow struct {
	OwnerID uuid.UUID
	IsOwner bool
}

// MemberRow is one member of a center joined with their account phone.
// CanSendReports is computed from the member's effective reports.send
// permission — the roster's JSON contract predates the permission catalog and
// keeps exposing the flag.
type MemberRow struct {
	ID             uuid.UUID
	FullName       string
	Phone          string
	IsOwner        bool
	CanSendReports bool
}

// RoleRow is one center role with its permission keys, comma-joined like
// ScopeRow's lists (same text[] scanning limitation).
type RoleRow struct {
	ID                uuid.UUID
	Key               string
	Name              string
	Perms             string
	AssignmentVersion int64
}

// MemberRBACRow is one live non-owner member's RBAC state: assigned role plus
// comma-joined grant/deny override keys. RoleKey is "" for a role-less stint.
type MemberRBACRow struct {
	TeacherID         uuid.UUID
	FullName          string
	RoleID            *uuid.UUID
	RoleKey           string
	Grants            string
	Denies            string
	AssignmentVersion int64
}

// TeacherRow is the slice of a teachers row the remove flow needs.
type TeacherRow struct {
	ID       uuid.UUID
	FullName string
}

// TeacherStatsRow is one dashboard roster row with activity counts.
type TeacherStatsRow struct {
	ID             uuid.UUID
	FullName       string
	Phone          string
	IsOwner        bool
	ActiveClasses  int
	ActiveStudents int
}

// OverviewClassRow is one class's raw monthly aggregates; the service turns
// the counters into rates.
type OverviewClassRow struct {
	ClassID          uuid.UUID
	ClassName        string
	TeacherID        uuid.UUID
	TeacherName      string
	HeldSessions     int
	AttendanceTotal  int
	PresentCount     int
	EstimatedRevenue int64
	FirstDayCount    int
	RetainedCount    int
}

// SessionStatsRow is one session's attendance counters.
type SessionStatsRow struct {
	Total     int
	Present   int
	Estimated int64
}

// Repository is the persistence contract for centers and memberships; the
// service depends on this interface, tests supply a fake.
type Repository interface {
	// ResolveScope returns the live scope for an active, non-deleted account
	// whose center is itself alive; ErrNotFound covers every dead variant.
	ResolveScope(ctx context.Context, teacherID uuid.UUID) (*ScopeRow, error)
	// CenterOwner returns the owner teacher id of teacherID's current center,
	// and whether teacherID IS that owner. Unlike ResolveScope it does not
	// filter on the account's own status: it deliberately answers for a
	// disabled account too, since ForgotPassword's owner-exclusion check must
	// run before it knows whether the account is active.
	CenterOwner(ctx context.Context, teacherID uuid.UUID) (*OwnerRow, error)
	GetCenter(ctx context.Context, centerID uuid.UUID) (*Center, error)
	ListMembers(ctx context.Context, centerID uuid.UUID) ([]MemberRow, error)
	// GetTeacherInCenter loads a live teacher currently belonging to the
	// center; ErrNotFound also for members of other centers (no existence
	// leak).
	GetTeacherInCenter(ctx context.Context, centerID, teacherID uuid.UUID) (*TeacherRow, error)
	Rename(ctx context.Context, centerID uuid.UUID, name string) error
	CreateCenter(ctx context.Context, c *Center) error
	// OpenMembership inserts the live stint, reopening a closed row from an
	// earlier stint in the same center instead of duplicating the key. The
	// caller must have closed the previous live stint first in the same
	// transaction — uq_center_members_active is checked per statement.
	OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) (joinedAt time.Time, err error)
	// CloseMembership stamps left_at on the live stint; ErrNotFound when a
	// concurrent transaction already closed it.
	CloseMembership(ctx context.Context, teacherID, centerID uuid.UUID) error
	// ListRoles returns the center's roles with their permission sets, in
	// stable key order.
	ListRoles(ctx context.Context, centerID uuid.UUID) ([]RoleRow, error)
	// GetRole loads one role of the center; ErrNotFound also for another
	// center's role (the path id is the target, tenancy comes from centerID).
	GetRole(ctx context.Context, centerID, roleID uuid.UUID) (*RoleRow, error)
	// ReplaceRolePermissions swaps the role's permission set for keys. The
	// caller resolves the role to its center first (GetRole) inside the same
	// transaction. expectedVersion is the CAS guard: 0 skips the check
	// (pre-CAS clients), otherwise a mismatch returns ErrStaleVersion with
	// nothing mutated. The version row-lock also serializes concurrent swaps.
	ReplaceRolePermissions(ctx context.Context, roleID uuid.UUID, keys []string, expectedVersion int64) error
	// ListMemberRBAC returns the live non-owner members with their role and
	// override lists (the owner sits outside the role system).
	ListMemberRBAC(ctx context.Context, centerID uuid.UUID) ([]MemberRBACRow, error)
	// GetMemberRBAC loads one live non-owner member's RBAC state; ErrNotFound
	// covers a left member, another center's member, and the owner.
	GetMemberRBAC(ctx context.Context, centerID, teacherID uuid.UUID) (*MemberRBACRow, error)
	// SetMemberRole assigns the live stint's role; ErrNotFound for every
	// refused variant (left member, other center, owner).
	SetMemberRole(ctx context.Context, centerID, teacherID, roleID uuid.UUID) error
	// ReplaceMemberOverrides swaps the member's override rows for the given
	// grant/deny lists. The caller verifies the target via GetMemberRBAC in
	// the same transaction. expectedVersion is the CAS guard, same contract
	// as ReplaceRolePermissions.
	ReplaceMemberOverrides(ctx context.Context, centerID, teacherID uuid.UUID, grants, denies []string, expectedVersion int64) error
	// SwitchTeacherCenter moves teachers.center_id to centerID
	// unconditionally; ErrNotFound when the teacher is unknown or
	// soft-deleted.
	SwitchTeacherCenter(ctx context.Context, teacherID, centerID uuid.UUID) error
	// WasEverMember reports whether the teacher is a current member of the
	// center or ever had a membership stint in it. Drill-down authorization
	// uses this so a removed teacher's left-behind data stays reachable.
	WasEverMember(ctx context.Context, centerID, teacherID uuid.UUID) (bool, error)
	// DashboardTeacherStats returns the current roster with per-teacher
	// activity counts, owner first.
	DashboardTeacherStats(ctx context.Context, centerID uuid.UUID) ([]TeacherStatsRow, error)
	// OverviewClassStats aggregates one row per live class of the center over
	// [from, to], ordered by teacher name then class name.
	OverviewClassStats(ctx context.Context, centerID uuid.UUID, from, to time.Time) ([]OverviewClassRow, error)
	// InvoicedByClass sums the given closed month's non-void invoice lines
	// (attributed to a class via their enrollment) and session-sourced
	// adjustments per class. Classes without invoiced money are absent.
	InvoicedByClass(ctx context.Context, centerID uuid.UUID, year, month int) (map[uuid.UUID]int64, error)
	// ClosedPeriodTeachers returns the teachers whose billing period for the
	// month is closed — the only ones whose invoiced numbers are final.
	ClosedPeriodTeachers(ctx context.Context, centerID uuid.UUID, year, month int) (map[uuid.UUID]struct{}, error)
	// SessionStats returns attendance counters per session id; sessions
	// without live records are absent from the map.
	SessionStats(ctx context.Context, centerID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]SessionStatsRow, error)
	// SessionInvoiced computes one session's invoiced revenue, or nil while
	// no closed billing period covers its date; ErrNotFound when the session
	// is not a live session of the center.
	SessionInvoiced(ctx context.Context, centerID, sessionID uuid.UUID) (*int64, error)
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
	// Still one round trip: the three permission lists ride along as
	// correlated subqueries. On a NULL cm (no live stint) they all collapse
	// to '' — an empty permission set.
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT t.center_id, (c.owner_id = t.id) AS is_owner,
			COALESCE((SELECT string_agg(rp.permission_key, ',')
				FROM center_role_permissions rp
				WHERE rp.role_id = cm.role_id), '') AS role_perms,
			COALESCE((SELECT string_agg(mp.permission_key, ',')
				FROM center_member_permissions mp
				WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
					AND mp.allowed), '') AS grants,
			COALESCE((SELECT string_agg(mp.permission_key, ',')
				FROM center_member_permissions mp
				WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
					AND NOT mp.allowed), '') AS denies
		FROM teachers t
		JOIN centers c ON c.id = t.center_id AND c.deleted_at IS NULL
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = ?
		LEFT JOIN center_members cm ON cm.teacher_id = t.id
			AND cm.center_id = t.center_id AND cm.left_at IS NULL
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

func (r *gormRepository) CenterOwner(ctx context.Context, teacherID uuid.UUID) (*OwnerRow, error) {
	var row OwnerRow
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT c.owner_id, (c.owner_id = t.id) AS is_owner
		FROM teachers t
		JOIN centers c ON c.id = t.center_id AND c.deleted_at IS NULL
		WHERE t.id = ? AND t.deleted_at IS NULL`,
		teacherID).Scan(&row)
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

func (r *gormRepository) ListMembers(ctx context.Context, centerID uuid.UUID) ([]MemberRow, error) {
	var rows []MemberRow
	// Offboarding (RemoveMember) is the only path onto status='disabled', and
	// it always pairs with closing the membership — so a disabled account is
	// a removed member and drops off the live roster exactly like a
	// soft-deleted one. teachers.center_id is not the source of truth here on
	// its own: it stays pointing at a removed member's last center until they
	// are reactivated elsewhere.
	// can_send_reports is the member's effective reports.send: a member
	// override grant or a role-held grant, minus a deny override — the same
	// algebra ResolveScope's BuildPermSet applies, evaluated in SQL for the
	// roster projection.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT t.id, t.full_name, ua.phone, (c.owner_id = t.id) AS is_owner,
			(cm.teacher_id IS NOT NULL
				AND (EXISTS (SELECT 1 FROM center_member_permissions mp
						WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
							AND mp.permission_key = @perm AND mp.allowed)
					OR EXISTS (SELECT 1 FROM center_role_permissions rp
						WHERE rp.role_id = cm.role_id AND rp.permission_key = @perm))
				AND NOT EXISTS (SELECT 1 FROM center_member_permissions mp
					WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
						AND mp.permission_key = @perm AND NOT mp.allowed)
			) AS can_send_reports
		FROM teachers t
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = @status
		JOIN centers c ON c.id = t.center_id
		LEFT JOIN center_members cm ON cm.teacher_id = t.id
			AND cm.center_id = t.center_id AND cm.left_at IS NULL
		WHERE t.center_id = @cid AND t.deleted_at IS NULL
		ORDER BY is_owner DESC, t.full_name, t.id`,
		map[string]any{"perm": authctx.PermReportsSend, "status": teachers.StatusActive, "cid": centerID}).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) GetTeacherInCenter(ctx context.Context, centerID, teacherID uuid.UUID) (*TeacherRow, error) {
	var row TeacherRow
	// Same active-only membership rule as ListMembers: an already-removed
	// (disabled) teacher is not found here even though their teachers.center_id
	// still points at this center.
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT t.id, t.full_name FROM teachers t
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = ?
		WHERE t.id = ? AND t.center_id = ? AND t.deleted_at IS NULL`,
		teachers.StatusActive, teacherID, centerID).Scan(&row)
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

func (r *gormRepository) CreateCenter(ctx context.Context, c *Center) error {
	if err := translateError(database.FromContext(ctx, r.db).Create(c).Error); err != nil {
		return err
	}
	// Every center is born with its three system roles; members default to
	// giao_vien on join, the owner stays outside the role system entirely.
	if err := database.FromContext(ctx, r.db).Exec(`
		INSERT INTO center_roles (id, center_id, key, name)
		VALUES (gen_random_uuid(), @cid, 'giao_vien', 'Giáo viên'),
			(gen_random_uuid(), @cid, 'hoc_vu', 'Học vụ'),
			(gen_random_uuid(), @cid, 'tro_giang', 'Trợ giảng')`,
		map[string]any{"cid": c.ID}).Error; err != nil {
		return err
	}
	// New roles start from the operational default baseline, mirroring the
	// backfill of pre-catalog centers: membership used to grant all
	// operational access, so a fresh role denying everything would make new
	// centers behave differently from existing ones.
	return database.FromContext(ctx, r.db).Exec(`
		INSERT INTO center_role_permissions (role_id, permission_key)
		SELECT cr.id, k
		FROM center_roles cr
		CROSS JOIN unnest(string_to_array(?, ',')) AS k
		WHERE cr.center_id = ?`,
		strings.Join(authctx.DefaultRoleKeys(), ","), c.ID).Error
}

func (r *gormRepository) OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) (time.Time, error) {
	var joinedAt time.Time
	// Permissions reset on reopen: they belong to a stint, not the person —
	// a re-invited member must be granted afresh. The override delete rides
	// in a data-modifying CTE so reset + reopen stay one atomic statement
	// even outside a caller transaction. The role subquery yields NULL for
	// the center owner (owner_id check) and for centers without seeded roles
	// (raw-SQL fixtures) — NULL role_id is defined as "no role permissions".
	// staff_closed is the same defence-in-depth as cleared: class assignments
	// belong to the previous stint, so a reopened membership must start with
	// none active — CloseMembership already ends them, this catches stints a
	// non-standard close path left behind. Active-ness is the write guard
	// only: the ended rows persist, and any stint (ended included) still
	// grants class READS, so a re-invited member resumes reading every class
	// they were ever assigned to. That is deliberate — removal from a class
	// keeps history readable — and revoking reads is the owner's explicit
	// act: voiding the assignment (hard delete) instead of soft-closing it.
	err := database.FromContext(ctx, r.db).Raw(`
		WITH cleared AS (
			DELETE FROM center_member_permissions
			WHERE teacher_id = @tid AND center_id = @cid
		), staff_closed AS (
			UPDATE class_staff SET ended_at = now()
			WHERE teacher_id = @tid AND center_id = @cid AND ended_at IS NULL
		)
		INSERT INTO center_members (teacher_id, center_id, role_id)
		VALUES (@tid, @cid, (
			SELECT cr.id FROM center_roles cr
			JOIN centers c ON c.id = cr.center_id AND c.owner_id <> @tid
			WHERE cr.center_id = @cid AND cr.key = 'giao_vien'
		))
		ON CONFLICT (teacher_id, center_id)
		DO UPDATE SET left_at = NULL, joined_at = now(), role_id = EXCLUDED.role_id
		RETURNING joined_at`,
		map[string]any{"tid": teacherID, "cid": centerID}).Scan(&joinedAt).Error
	if err != nil {
		return time.Time{}, translateError(err)
	}
	return joinedAt, nil
}

func (r *gormRepository) CloseMembership(ctx context.Context, teacherID, centerID uuid.UUID) error {
	// Defence in depth alongside OpenMembership's reset: a closed stint never
	// keeps permissions, so no code path can resurrect them from stale rows.
	// The override delete is unconditional on purpose — orphaned rows of an
	// already-closed stint are equally dead — and the CTE keeps delete +
	// close one atomic statement.
	// Class-staff stints soft-close with the membership: like permissions they
	// belong to the stint, but unlike permissions the rows stay (ended, so
	// history reads survive — including across a later re-invite) rather than
	// being deleted. An owner who means to revoke reads voids the assignment
	// (hard delete) rather than removing the member.
	res := database.FromContext(ctx, r.db).Exec(`
		WITH cleared AS (
			DELETE FROM center_member_permissions
			WHERE teacher_id = @tid AND center_id = @cid
		), staff_closed AS (
			UPDATE class_staff SET ended_at = now()
			WHERE teacher_id = @tid AND center_id = @cid AND ended_at IS NULL
		)
		UPDATE center_members SET left_at = now()
		WHERE teacher_id = @tid AND center_id = @cid AND left_at IS NULL`,
		map[string]any{"tid": teacherID, "cid": centerID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ListRoles(ctx context.Context, centerID uuid.UUID) ([]RoleRow, error) {
	var rows []RoleRow
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT cr.id, cr.key, cr.name, cr.assignment_version,
			COALESCE((SELECT string_agg(rp.permission_key, ',' ORDER BY rp.permission_key)
				FROM center_role_permissions rp
				WHERE rp.role_id = cr.id), '') AS perms
		FROM center_roles cr
		WHERE cr.center_id = ?
		ORDER BY cr.key`, centerID).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) GetRole(ctx context.Context, centerID, roleID uuid.UUID) (*RoleRow, error) {
	var row RoleRow
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT cr.id, cr.key, cr.name, cr.assignment_version,
			COALESCE((SELECT string_agg(rp.permission_key, ',' ORDER BY rp.permission_key)
				FROM center_role_permissions rp
				WHERE rp.role_id = cr.id), '') AS perms
		FROM center_roles cr
		WHERE cr.id = ? AND cr.center_id = ?`, roleID, centerID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *gormRepository) ReplaceRolePermissions(ctx context.Context, roleID uuid.UUID, keys []string, expectedVersion int64) error {
	db := database.FromContext(ctx, r.db)
	// CAS bump first: the UPDATE takes the row lock, so concurrent
	// replacements of the same role serialize here and the loser's re-check
	// of assignment_version fails cleanly. The caller resolved the role via
	// GetRole in this transaction, so zero rows can only mean a version
	// mismatch. expectedVersion 0 is a pre-CAS client: bump without checking
	// (last write wins, the legacy behavior).
	bump := db.Exec(`
		UPDATE center_roles SET assignment_version = assignment_version + 1, updated_at = now()
		WHERE id = @rid AND (@expected = 0 OR assignment_version = @expected)`,
		map[string]any{"rid": roleID, "expected": expectedVersion})
	if bump.Error != nil {
		return bump.Error
	}
	if bump.RowsAffected == 0 {
		return ErrStaleVersion
	}
	if err := db.Exec(`DELETE FROM center_role_permissions WHERE role_id = ?`, roleID).Error; err != nil {
		return err
	}
	// One statement per key: the set is bounded by the registry (single
	// digits), and the surrounding transaction keeps the swap atomic.
	for _, key := range keys {
		if err := db.Exec(`
			INSERT INTO center_role_permissions (role_id, permission_key)
			VALUES (?, ?)`, roleID, key).Error; err != nil {
			return err
		}
	}
	return nil
}

// memberRBACSelect is the shared projection of one live non-owner stint's
// RBAC state; the owner is excluded — they sit outside the role system
// entirely.
const memberRBACSelect = `
	SELECT cm.teacher_id, t.full_name, cm.role_id, cm.assignment_version,
		COALESCE(cr.key, '') AS role_key,
		COALESCE((SELECT string_agg(mp.permission_key, ',' ORDER BY mp.permission_key)
			FROM center_member_permissions mp
			WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
				AND mp.allowed), '') AS grants,
		COALESCE((SELECT string_agg(mp.permission_key, ',' ORDER BY mp.permission_key)
			FROM center_member_permissions mp
			WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
				AND NOT mp.allowed), '') AS denies
	FROM center_members cm
	JOIN centers c ON c.id = cm.center_id AND c.owner_id <> cm.teacher_id
	JOIN teachers t ON t.id = cm.teacher_id AND t.deleted_at IS NULL
	JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = ?
	LEFT JOIN center_roles cr ON cr.id = cm.role_id`

func (r *gormRepository) ListMemberRBAC(ctx context.Context, centerID uuid.UUID) ([]MemberRBACRow, error) {
	var rows []MemberRBACRow
	// Same live-roster semantics as ListMembers: a disabled account is a
	// removed member.
	err := database.FromContext(ctx, r.db).Raw(memberRBACSelect+`
		WHERE cm.center_id = ? AND cm.left_at IS NULL
		ORDER BY t.full_name, t.id`, teachers.StatusActive, centerID).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) GetMemberRBAC(ctx context.Context, centerID, teacherID uuid.UUID) (*MemberRBACRow, error) {
	var row MemberRBACRow
	res := database.FromContext(ctx, r.db).Raw(memberRBACSelect+`
		WHERE cm.center_id = ? AND cm.teacher_id = ? AND cm.left_at IS NULL`,
		teachers.StatusActive, centerID, teacherID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *gormRepository) SetMemberRole(ctx context.Context, centerID, teacherID, roleID uuid.UUID) error {
	// Owner exclusion at the SQL level: a role on the owner's stint would be
	// dead weight at best and confusing at worst.
	res := database.FromContext(ctx, r.db).Exec(`
		UPDATE center_members cm SET role_id = @rid
		FROM centers c
		WHERE cm.teacher_id = @tid AND cm.center_id = @cid AND cm.left_at IS NULL
			AND c.id = cm.center_id AND c.owner_id <> cm.teacher_id`,
		map[string]any{"rid": roleID, "tid": teacherID, "cid": centerID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ReplaceMemberOverrides(ctx context.Context, centerID, teacherID uuid.UUID, grants, denies []string, expectedVersion int64) error {
	db := database.FromContext(ctx, r.db)
	// CAS bump first, before the row swap, so the version row lock serializes
	// concurrent replacements. The caller verified the live stint via
	// GetMemberRBAC in this transaction, so zero rows means the version went
	// stale, not that the member vanished. expectedVersion 0 is a pre-CAS
	// client: bump without checking.
	bump := db.Exec(`
		UPDATE center_members
		SET assignment_version = assignment_version + 1
		WHERE teacher_id = @tid AND center_id = @cid AND left_at IS NULL
			AND (@expected = 0 OR assignment_version = @expected)`,
		map[string]any{"tid": teacherID, "cid": centerID, "expected": expectedVersion})
	if bump.Error != nil {
		return bump.Error
	}
	if bump.RowsAffected == 0 {
		return ErrStaleVersion
	}
	if err := db.Exec(`
		DELETE FROM center_member_permissions
		WHERE teacher_id = ? AND center_id = ?`, teacherID, centerID).Error; err != nil {
		return err
	}
	for _, key := range grants {
		if err := db.Exec(`
			INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
			VALUES (?, ?, ?, TRUE)`, teacherID, centerID, key).Error; err != nil {
			return err
		}
	}
	for _, key := range denies {
		if err := db.Exec(`
			INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
			VALUES (?, ?, ?, FALSE)`, teacherID, centerID, key).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *gormRepository) SwitchTeacherCenter(ctx context.Context, teacherID, centerID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).Exec(`
		UPDATE teachers SET center_id = ?, updated_at = now()
		WHERE id = ? AND deleted_at IS NULL`,
		centerID, teacherID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) WasEverMember(ctx context.Context, centerID, teacherID uuid.UUID) (bool, error) {
	var ok bool
	// Every membership — current or ended — leaves a center_members row; the
	// teachers check additionally covers the owner of a fresh center whose
	// stint row and current pointer must agree.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM center_members WHERE center_id = @cid AND teacher_id = @tid
			UNION ALL
			SELECT 1 FROM teachers WHERE id = @tid AND center_id = @cid AND deleted_at IS NULL
		)`, map[string]any{"cid": centerID, "tid": teacherID}).Scan(&ok).Error
	return ok, err
}

func (r *gormRepository) DashboardTeacherStats(ctx context.Context, centerID uuid.UUID) ([]TeacherStatsRow, error) {
	var rows []TeacherStatsRow
	// Same roster semantics as ListMembers (a disabled account is a removed
	// member and drops off same as soft-deleted), plus per-teacher activity
	// counts. "Active student" means an enrollment live today into a live
	// class. Removed teachers' historical classes/sessions stay reachable
	// through the drill-down endpoints below — only this top-level roster is
	// membership-filtered.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT t.id, t.full_name, ua.phone, (c.owner_id = t.id) AS is_owner,
			(SELECT COUNT(*) FROM classes cl
			 WHERE cl.teacher_id = t.id AND cl.center_id = t.center_id
			   AND cl.status = 'active' AND cl.deleted_at IS NULL) AS active_classes,
			(SELECT COUNT(DISTINCT e.student_id) FROM enrollments e
			 JOIN classes cl ON cl.id = e.class_id AND cl.center_id = e.center_id AND cl.deleted_at IS NULL
			 WHERE e.teacher_id = t.id AND e.center_id = t.center_id AND e.deleted_at IS NULL
			   AND e.started_on <= CURRENT_DATE
			   AND (e.ended_on IS NULL OR e.ended_on >= CURRENT_DATE)) AS active_students
		FROM teachers t
		JOIN user_accounts ua ON ua.id = t.id AND ua.deleted_at IS NULL AND ua.status = ?
		JOIN centers c ON c.id = t.center_id
		WHERE t.center_id = ? AND t.deleted_at IS NULL
		ORDER BY is_owner DESC, t.full_name, t.id`, teachers.StatusActive, centerID).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) OverviewClassStats(ctx context.Context, centerID uuid.UUID, from, to time.Time) ([]OverviewClassRow, error) {
	var rows []OverviewClassRow
	// One row per live class of the center, teacher left-joined for the name
	// only so classes of removed teachers keep reporting. Attendance counters
	// come from held sessions in range; estimated revenue additionally
	// requires the session confirmed and the record billable. Retention
	// counts enrollments active on the range's first day, and among those the
	// ones still active on its last day.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT c.id AS class_id, c.name AS class_name, c.teacher_id,
			COALESCE(t.full_name, '') AS teacher_name,
			st.held_sessions, st.attendance_total, st.present_count, st.estimated_revenue,
			re.first_day_count, re.retained_count
		FROM classes c
		LEFT JOIN teachers t ON t.id = c.teacher_id AND t.deleted_at IS NULL
		CROSS JOIN LATERAL (
			SELECT COUNT(DISTINCT s.id) AS held_sessions,
				COUNT(ar.id) AS attendance_total,
				COUNT(ar.id) FILTER (WHERE ar.status = 'present') AS present_count,
				COALESCE(SUM(e.unit_price) FILTER (
					WHERE ar.billable AND s.attendance_confirmed_at IS NOT NULL), 0) AS estimated_revenue
			FROM class_sessions s
			LEFT JOIN attendance_records ar ON ar.session_id = s.id AND ar.center_id = s.center_id
				AND ar.deleted_at IS NULL
			LEFT JOIN enrollments e ON e.id = ar.enrollment_id AND e.center_id = ar.center_id
				AND e.deleted_at IS NULL
			WHERE s.class_id = c.id AND s.center_id = c.center_id AND s.deleted_at IS NULL
				AND s.status = 'held' AND s.session_date BETWEEN @from AND @to
		) st
		CROSS JOIN LATERAL (
			SELECT COUNT(*) FILTER (
					WHERE en.started_on <= @from
					  AND (en.ended_on IS NULL OR en.ended_on >= @from)) AS first_day_count,
				COUNT(*) FILTER (
					WHERE en.started_on <= @from
					  AND (en.ended_on IS NULL OR en.ended_on >= @to)) AS retained_count
			FROM enrollments en
			WHERE en.class_id = c.id AND en.center_id = c.center_id AND en.deleted_at IS NULL
		) re
		WHERE c.center_id = @cid AND c.deleted_at IS NULL
		ORDER BY teacher_name, c.teacher_id, c.name, c.id`,
		map[string]any{"cid": centerID, "from": from, "to": to}).Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) InvoicedByClass(ctx context.Context, centerID uuid.UUID, year, month int) (map[uuid.UUID]int64, error) {
	var rows []struct {
		ClassID uuid.UUID
		Total   int64
	}
	// Lines attribute to a class through their enrollment; adjustments only
	// through an explicit source session — an adjustment without one belongs
	// to no class and is deliberately absent here. Void invoices never count;
	// invoices and lines have no deleted_at column.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT class_id, SUM(amount)::bigint AS total FROM (
			SELECT cl.id AS class_id, il.amount
			FROM invoice_lines il
			JOIN enrollments e ON e.id = il.enrollment_id AND e.center_id = il.center_id AND e.deleted_at IS NULL
			JOIN classes cl ON cl.id = e.class_id AND cl.center_id = e.center_id AND cl.deleted_at IS NULL
			JOIN invoices i ON i.id = il.invoice_id AND i.center_id = il.center_id AND i.status <> 'void'
			JOIN billing_periods p ON p.id = i.period_id AND p.center_id = i.center_id AND p.deleted_at IS NULL
				AND p.status = 'closed' AND p.year = @y AND p.month = @m
			WHERE il.center_id = @cid
			UNION ALL
			SELECT cl.id, a.amount
			FROM invoice_adjustments a
			JOIN class_sessions s ON s.id = a.source_session_id AND s.center_id = a.center_id AND s.deleted_at IS NULL
			JOIN classes cl ON cl.id = s.class_id AND cl.center_id = s.center_id AND cl.deleted_at IS NULL
			JOIN invoices i ON i.id = a.invoice_id AND i.center_id = a.center_id AND i.status <> 'void'
			JOIN billing_periods p ON p.id = i.period_id AND p.center_id = i.center_id AND p.deleted_at IS NULL
				AND p.status = 'closed' AND p.year = @y AND p.month = @m
			WHERE a.center_id = @cid AND a.deleted_at IS NULL
		) x GROUP BY class_id`,
		map[string]any{"cid": centerID, "y": year, "m": month}).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int64, len(rows))
	for _, row := range rows {
		out[row.ClassID] = row.Total
	}
	return out, nil
}

func (r *gormRepository) ClosedPeriodTeachers(ctx context.Context, centerID uuid.UUID, year, month int) (map[uuid.UUID]struct{}, error) {
	var rows []struct {
		TeacherID uuid.UUID
	}
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT teacher_id FROM billing_periods
		WHERE center_id = ? AND year = ? AND month = ? AND status = 'closed' AND deleted_at IS NULL`,
		centerID, year, month).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		out[row.TeacherID] = struct{}{}
	}
	return out, nil
}

func (r *gormRepository) SessionStats(ctx context.Context, centerID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]SessionStatsRow, error) {
	out := make(map[uuid.UUID]SessionStatsRow, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		SessionID uuid.UUID
		SessionStatsRow
	}
	// Estimated revenue only materialises once the session is confirmed;
	// billable absences still bill.
	err := database.FromContext(ctx, r.db).Raw(`
		SELECT s.id AS session_id,
			COUNT(ar.id) AS total,
			COUNT(ar.id) FILTER (WHERE ar.status = 'present') AS present,
			COALESCE(SUM(e.unit_price) FILTER (
				WHERE ar.billable AND s.attendance_confirmed_at IS NOT NULL), 0) AS estimated
		FROM class_sessions s
		LEFT JOIN attendance_records ar ON ar.session_id = s.id AND ar.center_id = s.center_id
			AND ar.deleted_at IS NULL
		LEFT JOIN enrollments e ON e.id = ar.enrollment_id AND e.center_id = ar.center_id
			AND e.deleted_at IS NULL
		WHERE s.center_id = ? AND s.id IN ? AND s.deleted_at IS NULL
		GROUP BY s.id`, centerID, sessionIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.SessionID] = row.SessionStatsRow
	}
	return out, nil
}

func (r *gormRepository) SessionInvoiced(ctx context.Context, centerID, sessionID uuid.UUID) (*int64, error) {
	var row struct {
		Covered bool
		Total   int64
	}
	// A record's unit price counts once its enrollment carries a non-void
	// line in a closed period covering the session's date. Session-sourced
	// adjustments are deliberately NOT added: they exist only as the
	// reconciler's post-close deltas, and the live records already embody the
	// edited state those deltas paid for — adding both would double-count.
	// No covering closed period for the session teacher's books → the number
	// is not final → nil.
	res := database.FromContext(ctx, r.db).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM billing_periods p
			WHERE p.teacher_id = s.teacher_id AND p.center_id = s.center_id
				AND p.status = 'closed' AND p.deleted_at IS NULL
				AND s.session_date BETWEEN p.period_start AND p.period_end
		) AS covered,
		(
			SELECT COALESCE(SUM(e.unit_price), 0)
			FROM attendance_records ar
			JOIN enrollments e ON e.id = ar.enrollment_id AND e.center_id = ar.center_id AND e.deleted_at IS NULL
			WHERE ar.session_id = s.id AND ar.center_id = s.center_id
				AND ar.deleted_at IS NULL AND ar.billable
				AND EXISTS (
					SELECT 1 FROM invoice_lines il
					JOIN invoices i ON i.id = il.invoice_id AND i.center_id = il.center_id AND i.status <> 'void'
					JOIN billing_periods p ON p.id = i.period_id AND p.center_id = i.center_id
						AND p.deleted_at IS NULL AND p.status = 'closed'
						AND s.session_date BETWEEN p.period_start AND p.period_end
					WHERE il.enrollment_id = e.id AND il.center_id = e.center_id
				)
		) AS total
		FROM class_sessions s
		WHERE s.id = ? AND s.center_id = ? AND s.deleted_at IS NULL`,
		sessionID, centerID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	if !row.Covered {
		return nil, nil
	}
	return &row.Total, nil
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
