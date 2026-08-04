package notifications

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
)

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
	SentAt      *time.Time
	CreatedAt   time.Time
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
