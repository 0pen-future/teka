package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
)

// ErrRunNotFound reports that a period has no notification run visible to the
// caller's scope — the snapshot endpoint turns it into {active: false} rather
// than a 404.
var ErrRunNotFound = errors.New("notification run not found")

// ErrRunActive reports that the database refused a second running run — either
// the per-teacher or the per-period partial unique index on notification_runs.
// The in-memory reservation and the HasActiveRunForPeriod pre-check already
// block both within one process; this error is the cross-process backstop
// surfacing as a conflict instead of a server error.
var ErrRunActive = errors.New("a notification run is already running")

// ErrPeriodNotFound reports that the billing period does not exist in the
// caller's center — surfaced as a neutral not-found, never distinguishing
// "someone else's" from "nonexistent".
var ErrPeriodNotFound = errors.New("billing period not found")

// ListRow is one notifications row plus the contact display fields a teacher
// needs to read the ledger — joined through statement_id -> statements ->
// contacts in one query, never a second round trip per row.
type ListRow struct {
	ID          uuid.UUID
	StatementID uuid.UUID
	ContactID   uuid.UUID
	ContactName string
	Phone       string
	// PhoneVisible is the phone-privacy derived column: whether the caller
	// holds an active hoc_vu stint over one of the contact's actively
	// enrolled students. Owner/oversight bypass happens in the service via
	// Scope.PhoneVisible.
	PhoneVisible bool
	Channel      string
	Purpose      string
	Status       string
	// ErrorMessage carries a failed row's teacher-facing reason; nil on any
	// row that has not failed.
	ErrorMessage *string
	// RunID is the paced run this row was queued into; nil on manual rows.
	RunID     *uuid.UUID
	SentAt    *time.Time
	CreatedAt time.Time
}

// ListFilter narrows GET .../notifications; a zero value matches every row.
type ListFilter struct {
	Purpose string
	Status  string
}

// Repository is the persistence contract for notifications; Service depends
// on this interface, tests supply a fake.
type Repository interface {
	// InsertBatch writes every row in one multi-row INSERT — the property
	// Service.BulkSend's scale requirement depends on: one query regardless
	// of how many contacts are queued. rows must already carry TeacherID and
	// CenterID.
	InsertBatch(ctx context.Context, rows []*Notification) error
	// ListByPeriod returns one billing period's notification ledger, scoped
	// to sc's center and to periods sc's own teacher owns unless sc holds
	// reports oversight (owner or reports.send). Period ownership, not
	// row ownership, is what gates visibility: after a delegated send the
	// period's own teacher must see the secretary's rows in their ledger, or
	// they would resend and double-DM every parent. Joined with contact
	// display fields, optionally narrowed by filter. deleted_at IS NULL on
	// every row returned.
	ListByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter ListFilter) ([]ListRow, error)
	// MarkSent sets status=sent and sent_at=now() for every id in ids that is
	// visible to sc (center-scoped, and teacher-scoped unless sc is the
	// center's owner) and still queued. An id already sent, unknown, or out
	// of scope is silently skipped — the caller cannot distinguish "already
	// sent" from "not found" from this call alone, which is what makes
	// repeated calls idempotent rather than erroring on the second attempt.
	MarkSent(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) error

	// CreateRun inserts one notification_runs record. Called inside the same
	// transaction as the run's InsertBatch so a failed queue never leaves a
	// headless run behind. run must already carry TeacherID and CenterID.
	CreateRun(ctx context.Context, run *Run) error
	// HasActiveRun reports whether sc's own teacher has a run still in
	// RunStatusRunning — the one-active-run-per-teacher guard BulkSend
	// enforces with a 409. Never owner-bypassed: a run's send slot belongs to
	// the acting teacher's own Zalo session, not the whole center.
	HasActiveRun(ctx context.Context, sc authctx.Scope) (bool, error)
	// LatestRunByPeriod returns the period's most recently created run in one
	// dimension: classID nil resolves the latest FAMILY run (class_id IS
	// NULL), scoped to sc's center and to periods sc's own teacher owns
	// unless sc holds reports oversight — period ownership, like
	// ListByPeriod, so the period's teacher can watch a delegated run on
	// their own period. classID set resolves the latest run for that class
	// copy, center-scoped only: the caller has already passed the class-send
	// gate, and period ownership is irrelevant to a class run (the class's
	// students can be billed under any teacher's period). Returns
	// ErrRunNotFound when none is visible.
	LatestRunByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, classID *uuid.UUID) (*Run, error)
	// LatestOwnRunByPeriod returns the period's most recently created run in
	// the classID dimension (as in LatestRunByPeriod) among runs sc's own
	// teacher created, ignoring oversight entirely, or ErrRunNotFound.
	// ResumeRun resolves through this: a resume re-occupies the acting
	// teacher's own Zalo session, so it must never resolve a run someone
	// else is responsible for.
	LatestOwnRunByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, classID *uuid.UUID) (*Run, error)
	// HasActiveRunForPeriod reports whether ANY teacher has a run still in
	// RunStatusRunning for this period in the classID dimension — the
	// single-running-run-per-dimension guard (the partial unique indexes on
	// notification_runs) that keeps two senders from double-DMing the same
	// parents. Dimensions are independent by design: a family run never
	// blocks a class run or vice versa (that overlap is warned, not
	// blocked), and two different classes' runs never block each other.
	HasActiveRunForPeriod(ctx context.Context, centerID, periodID uuid.UUID, classID *uuid.UUID) (bool, error)
	// HasStatementSendForPeriod reports whether the period already has any
	// non-failed notification in the OTHER dimension: classScoped false
	// checks family-statement sends, true checks class-copy sends. Backs the
	// overlap warning a send/preview response carries when a family and a
	// class send would reach the same parents in one period — a warning
	// because overlap can be intentional (plan decision), never a block.
	HasStatementSendForPeriod(ctx context.Context, centerID, periodID uuid.UUID, classScoped bool) (bool, error)
	// ClassSendAllowed reports whether the teacher may still send for the
	// class right now: an active sending-role stint on it, an effective
	// reports.send permission on their live membership, or being the
	// center's owner. A deleted class reads as false. This is the mid-run
	// revocation probe for a class-scoped run — the class counterpart of
	// CanSendReports.
	ClassSendAllowed(ctx context.Context, centerID, teacherID, classID uuid.UUID) (bool, error)
	// PeriodTeacher returns the owning teacher of one billing period in
	// centerID, or ErrPeriodNotFound. Backs the pre-send preview's
	// cross-teacher check without a statements refresh.
	PeriodTeacher(ctx context.Context, centerID, periodID uuid.UUID) (uuid.UUID, error)
	// CanSendReports reports whether the teacher currently holds an
	// effective reports.send permission on a live membership in centerID. A
	// missing or closed membership reads as false — removal revokes like an
	// explicit deny does.
	CanSendReports(ctx context.Context, centerID, teacherID uuid.UUID) (bool, error)
	// UpdateRunStatus moves a run to status. A terminal status stamps
	// finished_at; moving back to RunStatusRunning (manual resume) clears it,
	// so finished_at is always "when this run last stopped", never a stale
	// leftover. Never owner-bypassed: only the run's own teacher may move it.
	UpdateRunStatus(ctx context.Context, sc authctx.Scope, runID uuid.UUID, status string) error
	// RunCounts derives a run's progress by counting its rows. Counters are
	// deliberately never stored on the run record — a COUNT cannot drift from
	// the rows the way an incremented column can after a crash. Callers pass
	// the run's own owning scope (not necessarily the requesting caller's),
	// since this call is never owner-bypassed.
	RunCounts(ctx context.Context, sc authctx.Scope, runID uuid.UUID) (RunCounts, error)
	// MarkOutcome records one row's final verdict. Only a row still queued
	// (and visible to sc) moves: a sent row is final, so a late or duplicate
	// outcome is silently skipped, mirroring MarkSent's idempotency. Never
	// owner-bypassed. StatusSent stamps sent_at; any outcome may carry a
	// provider message id or an error message.
	MarkOutcome(ctx context.Context, sc authctx.Scope, id uuid.UUID, status string, providerMsgID, errorMessage *string) error
	// FailQueuedInRun fails every still-queued row of the run with reason —
	// the expired-mid-run sweep. Rows already sent or failed keep their
	// verdicts. Never owner-bypassed.
	FailQueuedInRun(ctx context.Context, sc authctx.Scope, runID uuid.UUID, reason string) error
	// QueuedRunRows lists the run's still-queued rows oldest-first, each with
	// its statement and contact, so a resume can re-render exactly the
	// messages that never went out. Never owner-bypassed.
	QueuedRunRows(ctx context.Context, sc authctx.Scope, runID uuid.UUID) ([]QueuedRunRow, error)
	// ZaloMappings returns contactID -> zalo_user_id for the given contacts,
	// covering live (non-deleted) contacts sc's own teacher owns — widened to
	// the whole center when sc holds reports oversight, so a delegated sender
	// can resolve the period teacher's mappings. The mapping stays the period
	// owner's consent artifact: this read never rewrites it. A contact absent
	// from the result is unmapped (or out of scope) and falls back to the
	// manual channel. An owner sending a member's period never reaches here
	// on the personal channel — the cross-teacher 409 fires first.
	ZaloMappings(ctx context.Context, sc authctx.Scope, contactIDs []uuid.UUID) (map[uuid.UUID]string, error)
	// ZaloMappingsClass is ZaloMappings for a class-scoped send: instead of
	// the owner/oversight teacher filter, a contact's mapping resolves when
	// one of their students holds a STILL-ACTIVE enrollment in classID — the
	// same liveness rule that targets the class copies. The mapping remains
	// whichever teacher's consent artifact it is; this read never rewrites
	// it. The caller must have passed the class-send gate first.
	ZaloMappingsClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]string, error)
	// MarkInterrupted moves every RunStatusRunning run to
	// RunStatusInterrupted and returns how many moved. Called once at boot,
	// with no tenant caller: a run still "running" when no process is sending
	// is a run the previous process died under, regardless of which center or
	// teacher it belongs to. Its rows stay queued for a manual resume.
	MarkInterrupted(ctx context.Context) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped binds a notifications-table query to sc's center, further narrowed
// to sc's own rows unless sc sees center-wide data — the standard
// owner-oversight template for the plain reads (List, MarkSent).
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("notifications.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermNotificationsViewAll) {
		q = q.Where("notifications.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// runsPeriodScoped binds a notification_runs-table query to sc's center,
// further narrowed to runs on periods sc's own teacher owns unless sc holds
// reports oversight. The run's own teacher_id is deliberately NOT the filter:
// a delegated run carries the secretary's teacher_id on the period teacher's
// period, and the period teacher must still see it (RunSnapshot) or they
// would judge the period unsent and resend.
func (r *gormRepository) runsPeriodScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("notification_runs.center_id = ?", sc.CenterID)
	if !sc.ReportsOversight() {
		q = q.Where("notification_runs.billing_period_id IN (SELECT id FROM billing_periods WHERE teacher_id = ? AND center_id = ?)",
			sc.TeacherID, sc.CenterID)
	}
	return q
}

// runsOwnScoped binds strictly to sc's own center and teacher, ignoring
// IsOwner entirely. A run's zalo_personal send slot, its occupancy checks,
// and the row-level writes that drive it belong to the acting teacher's own
// Zalo session — an owner has no bypass here, only over the read-only
// progress snapshot (runsScoped). The same unqualified center_id/teacher_id
// predicate is reused for both the notification_runs table and the
// notifications table: every caller of this helper chains a single-table
// Model/Table with no join, so the column names never collide.
func (r *gormRepository) runsOwnScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Where("center_id = ? AND teacher_id = ?", sc.CenterID, sc.TeacherID)
}

func (r *gormRepository) InsertBatch(ctx context.Context, rows []*Notification) error {
	if len(rows) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

// listByPeriodQuery joins on center_id, not teacher_id: a notification's
// sender (owner or delegated secretary acting on a member's period) can
// differ from the statement's own teacher, but both always share a center —
// the same invariant the fk_notifications_statement_center composite FK
// enforces at the database level. Visibility is by PERIOD ownership:
// s.teacher_id is the period teacher's id (statements are always anchored on
// the period's own teacher), so the (? OR s.teacher_id = ?) pair — the same
// SQL-OR short-circuit trick payments/repository.go's CandidateInvoices uses
// — shows a period's full ledger to its own teacher and to reports-oversight
// callers, delegated rows included, and nothing to anyone else.
const listByPeriodQuery = `
	SELECT n.id AS id, n.statement_id AS statement_id, s.contact_id AS contact_id,
	       c.full_name AS contact_name, c.phone AS phone, %s AS phone_visible,
	       n.channel AS channel, n.purpose AS purpose, n.status AS status,
	       n.error_message AS error_message, n.run_id AS run_id,
	       n.sent_at AS sent_at, n.created_at AS created_at
	FROM notifications n
	JOIN statements s ON s.id = n.statement_id AND s.center_id = n.center_id
	JOIN contacts c   ON c.id = s.contact_id AND c.center_id = s.center_id
	WHERE n.center_id = ? AND (? OR s.teacher_id = ?) AND s.period_id = ? AND n.deleted_at IS NULL
	  AND (? = '' OR n.purpose = ?)
	  AND (? = '' OR n.status = ?)
	ORDER BY n.created_at DESC
`

func (r *gormRepository) ListByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter ListFilter) ([]ListRow, error) {
	// The phone_visible fragment sits in the SELECT clause, so its two bind
	// args (the CALLER's teacher, then center) come first in the Raw list.
	frag, _ := classscope.PhoneVisibleViaContact("s.contact_id")
	var rows []ListRow
	err := database.FromContext(ctx, r.db).
		Raw(fmt.Sprintf(listByPeriodQuery, frag),
			sc.TeacherID, sc.CenterID,
			sc.CenterID, sc.ReportsOversight(), sc.TeacherID, periodID, filter.Purpose, filter.Purpose, filter.Status, filter.Status).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) MarkSent(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return r.scoped(ctx, sc).
		Model(&Notification{}).
		Where("notifications.id IN ? AND notifications.status = ? AND notifications.deleted_at IS NULL", ids, StatusQueued).
		Updates(map[string]any{
			"status":     StatusSent,
			"sent_at":    gorm.Expr("now()"),
			"updated_at": gorm.Expr("now()"),
		}).Error
}

func (r *gormRepository) CreateRun(ctx context.Context, run *Run) error {
	return translateRunError(database.FromContext(ctx, r.db).Create(run).Error)
}

func (r *gormRepository) HasActiveRun(ctx context.Context, sc authctx.Scope) (bool, error) {
	var count int64
	err := r.runsOwnScoped(ctx, sc).
		Model(&Run{}).
		Where("status = ?", RunStatusRunning).
		Count(&count).Error
	return count > 0, err
}

// runClassDimension narrows a notification_runs query to one run dimension:
// family (class_id IS NULL) or one class's copies.
func runClassDimension(q *gorm.DB, classID *uuid.UUID) *gorm.DB {
	if classID == nil {
		return q.Where("class_id IS NULL")
	}
	return q.Where("class_id = ?", classID)
}

func (r *gormRepository) LatestRunByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, classID *uuid.UUID) (*Run, error) {
	q := r.runsPeriodScoped(ctx, sc)
	if classID != nil {
		// Class runs are center-scoped only — the service's class-send gate
		// already authorized this class, and period ownership carries no
		// meaning for a class run (see the interface doc comment).
		q = database.FromContext(ctx, r.db).Where("notification_runs.center_id = ?", sc.CenterID)
	}
	var run Run
	err := runClassDimension(q, classID).
		Where("billing_period_id = ?", periodID).
		Order("created_at DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *gormRepository) LatestOwnRunByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, classID *uuid.UUID) (*Run, error) {
	var run Run
	err := runClassDimension(r.runsOwnScoped(ctx, sc), classID).
		Where("billing_period_id = ?", periodID).
		Order("created_at DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *gormRepository) HasActiveRunForPeriod(ctx context.Context, centerID, periodID uuid.UUID, classID *uuid.UUID) (bool, error) {
	var count int64
	err := runClassDimension(
		database.FromContext(ctx, r.db).
			Model(&Run{}).
			Where("center_id = ? AND billing_period_id = ? AND status = ?", centerID, periodID, RunStatusRunning),
		classID,
	).Count(&count).Error
	return count > 0, err
}

func (r *gormRepository) HasStatementSendForPeriod(ctx context.Context, centerID, periodID uuid.UUID, classScoped bool) (bool, error) {
	dimension := "s.class_id IS NULL"
	if classScoped {
		dimension = "s.class_id IS NOT NULL"
	}
	var count int64
	err := database.FromContext(ctx, r.db).
		Table("notifications AS n").
		Joins("JOIN statements s ON s.id = n.statement_id AND s.center_id = n.center_id").
		Where("n.center_id = ? AND s.period_id = ? AND "+dimension, centerID, periodID).
		Where("n.purpose = ? AND n.status IN ? AND n.deleted_at IS NULL",
			PurposeStatements, []string{StatusQueued, StatusSent, StatusDelivered}).
		Count(&count).Error
	return count > 0, err
}

func (r *gormRepository) ClassSendAllowed(ctx context.Context, centerID, teacherID, classID uuid.UUID) (bool, error) {
	stintFrag, _ := classscope.WriteExists("classes.id")
	var rows []struct{ Allowed bool }
	// The delegation arm mirrors CanSendReports: effective reports.send on
	// the live stint (grant or role minus deny), never a closed one.
	err := database.FromContext(ctx, r.db).
		Table("classes").
		Select(stintFrag+` OR EXISTS (
			SELECT 1 FROM center_members cm
			WHERE cm.center_id = classes.center_id AND cm.teacher_id = ? AND cm.left_at IS NULL
				AND (EXISTS (SELECT 1 FROM center_member_permissions mp
						WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
							AND mp.permission_key = 'reports.send' AND mp.allowed)
					OR EXISTS (SELECT 1 FROM center_role_permissions rp
						WHERE rp.role_id = cm.role_id AND rp.permission_key = 'reports.send'))
				AND NOT EXISTS (SELECT 1 FROM center_member_permissions mp
					WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
						AND mp.permission_key = 'reports.send' AND NOT mp.allowed))
		OR EXISTS (
			SELECT 1 FROM centers ce
			WHERE ce.id = classes.center_id AND ce.owner_id = ?) AS allowed`,
			teacherID, centerID, classSendRoles, teacherID, teacherID).
		Where("classes.id = ? AND classes.center_id = ? AND classes.deleted_at IS NULL", classID, centerID).
		Find(&rows).Error
	if err != nil {
		return false, err
	}
	// Zero rows means the class is gone (deleted): nothing left to send for.
	return len(rows) == 1 && rows[0].Allowed, nil
}

func (r *gormRepository) PeriodTeacher(ctx context.Context, centerID, periodID uuid.UUID) (uuid.UUID, error) {
	var row struct{ TeacherID uuid.UUID }
	res := database.FromContext(ctx, r.db).
		Raw(`SELECT teacher_id FROM billing_periods WHERE id = ? AND center_id = ?`, periodID, centerID).
		Scan(&row)
	if res.Error != nil {
		return uuid.Nil, res.Error
	}
	if res.RowsAffected == 0 {
		return uuid.Nil, ErrPeriodNotFound
	}
	return row.TeacherID, nil
}

func (r *gormRepository) CanSendReports(ctx context.Context, centerID, teacherID uuid.UUID) (bool, error) {
	// Effective reports.send on the LIVE stint only: a member override grant
	// or a role-held grant, minus a deny override — the same algebra
	// BuildPermSet applies at scope resolution. left_at IS NULL is
	// load-bearing: closing a membership deletes the stint's override rows
	// but keeps role_id on the closed row, so without it a role-granted
	// reports.send would survive removal.
	var can bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM center_members cm
			WHERE cm.center_id = @cid AND cm.teacher_id = @tid AND cm.left_at IS NULL
				AND (EXISTS (SELECT 1 FROM center_member_permissions mp
						WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
							AND mp.permission_key = @perm AND mp.allowed)
					OR EXISTS (SELECT 1 FROM center_role_permissions rp
						WHERE rp.role_id = cm.role_id AND rp.permission_key = @perm))
				AND NOT EXISTS (SELECT 1 FROM center_member_permissions mp
					WHERE mp.teacher_id = cm.teacher_id AND mp.center_id = cm.center_id
						AND mp.permission_key = @perm AND NOT mp.allowed))`,
			map[string]any{"cid": centerID, "tid": teacherID, "perm": authctx.PermReportsSend}).
		Scan(&can).Error
	return can, err
}

func (r *gormRepository) UpdateRunStatus(ctx context.Context, sc authctx.Scope, runID uuid.UUID, status string) error {
	finishedAt := any(gorm.Expr("now()"))
	if status == RunStatusRunning {
		finishedAt = nil
	}
	return translateRunError(r.runsOwnScoped(ctx, sc).
		Model(&Run{}).
		Where("id = ?", runID).
		Updates(map[string]any{
			"status":      status,
			"finished_at": finishedAt,
		}).Error)
}

// translateRunError maps the unique constraints run writes can trip — the
// per-teacher and per-period single-running-run indexes — onto ErrRunActive.
// Runs have no other unique key a caller could violate, so any duplicate-key
// error here is one of those two, and both mean the same thing to a caller:
// something is already sending, come back later.
func translateRunError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrRunActive
	}
	return err
}

func (r *gormRepository) RunCounts(ctx context.Context, sc authctx.Scope, runID uuid.UUID) (RunCounts, error) {
	var counts RunCounts
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT count(*) AS total,
		            count(*) FILTER (WHERE status = ?) AS sent,
		            count(*) FILTER (WHERE status = ?) AS failed
		     FROM notifications
		     WHERE run_id = ? AND center_id = ? AND teacher_id = ? AND deleted_at IS NULL`,
			StatusSent, StatusFailed, runID, sc.CenterID, sc.TeacherID).
		Scan(&counts).Error
	return counts, err
}

func (r *gormRepository) MarkOutcome(ctx context.Context, sc authctx.Scope, id uuid.UUID, status string, providerMsgID, errorMessage *string) error {
	updates := map[string]any{
		"status":          status,
		"provider_msg_id": providerMsgID,
		"error_message":   errorMessage,
		"updated_at":      gorm.Expr("now()"),
	}
	if status == StatusSent {
		updates["sent_at"] = gorm.Expr("now()")
	}
	return r.runsOwnScoped(ctx, sc).
		Model(&Notification{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", id, StatusQueued).
		Updates(updates).Error
}

func (r *gormRepository) FailQueuedInRun(ctx context.Context, sc authctx.Scope, runID uuid.UUID, reason string) error {
	return r.runsOwnScoped(ctx, sc).
		Model(&Notification{}).
		Where("run_id = ? AND status = ? AND deleted_at IS NULL", runID, StatusQueued).
		Updates(map[string]any{
			"status":        StatusFailed,
			"error_message": reason,
			"updated_at":    gorm.Expr("now()"),
		}).Error
}

const queuedRunRowsQuery = `
	SELECT n.id AS notification_id, n.statement_id AS statement_id, s.contact_id AS contact_id
	FROM notifications n
	JOIN statements s ON s.id = n.statement_id AND s.center_id = n.center_id
	WHERE n.run_id = ? AND n.center_id = ? AND n.teacher_id = ? AND n.status = ? AND n.deleted_at IS NULL
	ORDER BY n.created_at, n.id
`

func (r *gormRepository) QueuedRunRows(ctx context.Context, sc authctx.Scope, runID uuid.UUID) ([]QueuedRunRow, error) {
	var rows []QueuedRunRow
	err := database.FromContext(ctx, r.db).
		Raw(queuedRunRowsQuery, runID, sc.CenterID, sc.TeacherID, StatusQueued).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) ZaloMappings(ctx context.Context, sc authctx.Scope, contactIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(contactIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	var rows []struct {
		ID         uuid.UUID
		ZaloUserID string
	}
	q := database.FromContext(ctx, r.db).
		Table("contacts").
		Select("id, zalo_user_id").
		Where("center_id = ? AND id IN ? AND zalo_user_id IS NOT NULL AND deleted_at IS NULL",
			sc.CenterID, contactIDs)
	if !sc.ReportsOversight() {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	err := q.Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	mappings := make(map[uuid.UUID]string, len(rows))
	for _, row := range rows {
		mappings[row.ID] = row.ZaloUserID
	}
	return mappings, nil
}

func (r *gormRepository) ZaloMappingsClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(contactIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	var rows []struct {
		ID         uuid.UUID
		ZaloUserID string
	}
	err := database.FromContext(ctx, r.db).
		Table("contacts").
		Select("id, zalo_user_id").
		Where("center_id = ? AND id IN ? AND zalo_user_id IS NOT NULL AND deleted_at IS NULL",
			sc.CenterID, contactIDs).
		Where(`EXISTS (
			SELECT 1 FROM students st
			JOIN enrollments e ON e.student_id = st.id AND e.center_id = st.center_id
			WHERE st.contact_id = contacts.id AND st.center_id = contacts.center_id
			  AND st.deleted_at IS NULL
			  AND e.class_id = ? AND e.deleted_at IS NULL AND e.ended_on IS NULL)`, classID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	mappings := make(map[uuid.UUID]string, len(rows))
	for _, row := range rows {
		mappings[row.ID] = row.ZaloUserID
	}
	return mappings, nil
}

func (r *gormRepository) MarkInterrupted(ctx context.Context) (int64, error) {
	res := database.FromContext(ctx, r.db).
		Model(&Run{}).
		Where("status = ?", RunStatusRunning).
		Updates(map[string]any{
			"status":      RunStatusInterrupted,
			"finished_at": gorm.Expr("now()"),
		})
	return res.RowsAffected, res.Error
}
