package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
)

// ErrRunNotFound reports that a period has no notification run (or none the
// caller's teacher owns) — the snapshot endpoint turns it into {active: false}
// rather than a 404.
var ErrRunNotFound = errors.New("notification run not found")

// ErrRunActive reports that the database refused a second running run for the
// teacher (partial unique index on notification_runs). The in-memory
// reservation already blocks this within one process; this error is the
// cross-process backstop surfacing as a conflict instead of a server error.
var ErrRunActive = errors.New("a notification run is already running for this teacher")

// ListRow is one notifications row plus the contact display fields a teacher
// needs to read the ledger — joined through statement_id -> statements ->
// contacts in one query, never a second round trip per row.
type ListRow struct {
	ID          uuid.UUID
	StatementID uuid.UUID
	ContactID   uuid.UUID
	ContactName string
	Phone       string
	Channel     string
	Purpose     string
	Status      string
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
	// of how many contacts are queued. rows must already carry TeacherID.
	InsertBatch(ctx context.Context, rows []*Notification) error
	// ListByPeriod returns one billing period's notification ledger,
	// teacher-scoped, joined with contact display fields, optionally
	// narrowed by filter. deleted_at IS NULL on every row returned.
	ListByPeriod(ctx context.Context, teacherID, periodID uuid.UUID, filter ListFilter) ([]ListRow, error)
	// MarkSent sets status=sent and sent_at=now() for every id in ids that
	// currently belongs to teacherID and is still queued. An id already sent,
	// unknown, or belonging to another teacher is silently skipped — the
	// caller cannot distinguish "already sent" from "not found" from this
	// call alone, which is what makes repeated calls idempotent rather than
	// erroring on the second attempt.
	MarkSent(ctx context.Context, teacherID uuid.UUID, ids []uuid.UUID) error

	// CreateRun inserts one notification_runs record. Called inside the same
	// transaction as the run's InsertBatch so a failed queue never leaves a
	// headless run behind.
	CreateRun(ctx context.Context, run *Run) error
	// HasActiveRun reports whether the teacher has a run still in
	// RunStatusRunning — the one-active-run-per-teacher guard BulkSend
	// enforces with a 409.
	HasActiveRun(ctx context.Context, teacherID uuid.UUID) (bool, error)
	// LatestRunByPeriod returns the period's most recently created run,
	// teacher-scoped, or ErrRunNotFound when the period never had one.
	LatestRunByPeriod(ctx context.Context, teacherID, periodID uuid.UUID) (*Run, error)
	// UpdateRunStatus moves a run to status. A terminal status stamps
	// finished_at; moving back to RunStatusRunning (manual resume) clears it,
	// so finished_at is always "when this run last stopped", never a stale
	// leftover.
	UpdateRunStatus(ctx context.Context, teacherID, runID uuid.UUID, status string) error
	// RunCounts derives a run's progress by counting its rows. Counters are
	// deliberately never stored on the run record — a COUNT cannot drift from
	// the rows the way an incremented column can after a crash.
	RunCounts(ctx context.Context, teacherID, runID uuid.UUID) (RunCounts, error)
	// MarkOutcome records one row's final verdict. Only a row still queued
	// (and owned by teacherID) moves: a sent row is final, so a late or
	// duplicate outcome is silently skipped, mirroring MarkSent's idempotency.
	// StatusSent stamps sent_at; any outcome may carry a provider message id
	// or an error message.
	MarkOutcome(ctx context.Context, teacherID, id uuid.UUID, status string, providerMsgID, errorMessage *string) error
	// FailQueuedInRun fails every still-queued row of the run with reason —
	// the expired-mid-run sweep. Rows already sent or failed keep their
	// verdicts.
	FailQueuedInRun(ctx context.Context, teacherID, runID uuid.UUID, reason string) error
	// QueuedRunRows lists the run's still-queued rows oldest-first, each with
	// its statement and contact, so a resume can re-render exactly the
	// messages that never went out.
	QueuedRunRows(ctx context.Context, teacherID, runID uuid.UUID) ([]QueuedRunRow, error)
	// ZaloMappings returns contactID -> zalo_user_id for the given contacts,
	// covering only live (non-deleted) contacts of teacherID that are mapped.
	// A contact absent from the result is unmapped and falls back to the
	// manual channel.
	ZaloMappings(ctx context.Context, teacherID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]string, error)
	// MarkInterrupted moves every RunStatusRunning run to
	// RunStatusInterrupted and returns how many moved. Called once at boot:
	// a run still "running" when no process is sending is a run the previous
	// process died under. Its rows stay queued for a manual resume.
	MarkInterrupted(ctx context.Context) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) InsertBatch(ctx context.Context, rows []*Notification) error {
	if len(rows) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

const listByPeriodQuery = `
	SELECT n.id AS id, n.statement_id AS statement_id, s.contact_id AS contact_id,
	       c.full_name AS contact_name, c.phone AS phone,
	       n.channel AS channel, n.purpose AS purpose, n.status AS status,
	       n.error_message AS error_message, n.run_id AS run_id,
	       n.sent_at AS sent_at, n.created_at AS created_at
	FROM notifications n
	JOIN statements s ON s.id = n.statement_id AND s.teacher_id = n.teacher_id
	JOIN contacts c   ON c.id = s.contact_id AND c.teacher_id = s.teacher_id
	WHERE n.teacher_id = ? AND s.period_id = ? AND n.deleted_at IS NULL
	  AND (? = '' OR n.purpose = ?)
	  AND (? = '' OR n.status = ?)
	ORDER BY n.created_at DESC
`

func (r *gormRepository) ListByPeriod(ctx context.Context, teacherID, periodID uuid.UUID, filter ListFilter) ([]ListRow, error) {
	var rows []ListRow
	err := database.FromContext(ctx, r.db).
		Raw(listByPeriodQuery, teacherID, periodID, filter.Purpose, filter.Purpose, filter.Status, filter.Status).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) MarkSent(ctx context.Context, teacherID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).
		Model(&Notification{}).
		Where("teacher_id = ? AND id IN ? AND status = ? AND deleted_at IS NULL", teacherID, ids, StatusQueued).
		Updates(map[string]any{
			"status":     StatusSent,
			"sent_at":    gorm.Expr("now()"),
			"updated_at": gorm.Expr("now()"),
		}).Error
}

func (r *gormRepository) CreateRun(ctx context.Context, run *Run) error {
	return translateRunError(database.FromContext(ctx, r.db).Create(run).Error)
}

func (r *gormRepository) HasActiveRun(ctx context.Context, teacherID uuid.UUID) (bool, error) {
	var count int64
	err := database.FromContext(ctx, r.db).
		Model(&Run{}).
		Where("teacher_id = ? AND status = ?", teacherID, RunStatusRunning).
		Count(&count).Error
	return count > 0, err
}

func (r *gormRepository) LatestRunByPeriod(ctx context.Context, teacherID, periodID uuid.UUID) (*Run, error) {
	var run Run
	err := database.FromContext(ctx, r.db).
		Where("teacher_id = ? AND billing_period_id = ?", teacherID, periodID).
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

func (r *gormRepository) UpdateRunStatus(ctx context.Context, teacherID, runID uuid.UUID, status string) error {
	finishedAt := any(gorm.Expr("now()"))
	if status == RunStatusRunning {
		finishedAt = nil
	}
	return translateRunError(database.FromContext(ctx, r.db).
		Model(&Run{}).
		Where("id = ? AND teacher_id = ?", runID, teacherID).
		Updates(map[string]any{
			"status":      status,
			"finished_at": finishedAt,
		}).Error)
}

// translateRunError maps the one unique constraint run writes can trip — the
// single-running-run index — onto ErrRunActive. Runs have no other unique key
// a caller could violate, so any duplicate-key error here is that index.
func translateRunError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrRunActive
	}
	return err
}

func (r *gormRepository) RunCounts(ctx context.Context, teacherID, runID uuid.UUID) (RunCounts, error) {
	var counts RunCounts
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT count(*) AS total,
		            count(*) FILTER (WHERE status = ?) AS sent,
		            count(*) FILTER (WHERE status = ?) AS failed
		     FROM notifications
		     WHERE run_id = ? AND teacher_id = ? AND deleted_at IS NULL`,
			StatusSent, StatusFailed, runID, teacherID).
		Scan(&counts).Error
	return counts, err
}

func (r *gormRepository) MarkOutcome(ctx context.Context, teacherID, id uuid.UUID, status string, providerMsgID, errorMessage *string) error {
	updates := map[string]any{
		"status":          status,
		"provider_msg_id": providerMsgID,
		"error_message":   errorMessage,
		"updated_at":      gorm.Expr("now()"),
	}
	if status == StatusSent {
		updates["sent_at"] = gorm.Expr("now()")
	}
	return database.FromContext(ctx, r.db).
		Model(&Notification{}).
		Where("id = ? AND teacher_id = ? AND status = ? AND deleted_at IS NULL", id, teacherID, StatusQueued).
		Updates(updates).Error
}

func (r *gormRepository) FailQueuedInRun(ctx context.Context, teacherID, runID uuid.UUID, reason string) error {
	return database.FromContext(ctx, r.db).
		Model(&Notification{}).
		Where("run_id = ? AND teacher_id = ? AND status = ? AND deleted_at IS NULL", runID, teacherID, StatusQueued).
		Updates(map[string]any{
			"status":        StatusFailed,
			"error_message": reason,
			"updated_at":    gorm.Expr("now()"),
		}).Error
}

const queuedRunRowsQuery = `
	SELECT n.id AS notification_id, n.statement_id AS statement_id, s.contact_id AS contact_id
	FROM notifications n
	JOIN statements s ON s.id = n.statement_id AND s.teacher_id = n.teacher_id
	WHERE n.run_id = ? AND n.teacher_id = ? AND n.status = ? AND n.deleted_at IS NULL
	ORDER BY n.created_at, n.id
`

func (r *gormRepository) QueuedRunRows(ctx context.Context, teacherID, runID uuid.UUID) ([]QueuedRunRow, error) {
	var rows []QueuedRunRow
	err := database.FromContext(ctx, r.db).
		Raw(queuedRunRowsQuery, runID, teacherID, StatusQueued).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) ZaloMappings(ctx context.Context, teacherID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID]string, error) {
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
		Where("teacher_id = ? AND id IN ? AND zalo_user_id IS NOT NULL AND deleted_at IS NULL", teacherID, contactIDs).
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
