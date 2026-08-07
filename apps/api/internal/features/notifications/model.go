// Package notifications queues and tracks the parent-facing sends a teacher
// triggers for a billing period's statements: one row per contact per send
// attempt, never a stored copy of the message text itself (see the model's
// own doc comment for why).
package notifications

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Channel values, matching notifications.channel's CHECK constraint exactly
// (docs/schema_design.sql:438-439). Every call site uses these constants,
// never a string literal.
const (
	ChannelZaloManual   = "zalo_manual"
	ChannelZaloZNS      = "zalo_zns"
	ChannelSMS          = "sms"
	ChannelZaloPersonal = "zalo_personal"
)

// Purpose values, matching notifications.purpose's CHECK constraint exactly
// (docs/schema_design.sql:440-441). The database only ever stores the plural
// PurposeStatements — see dto.go for how a request's singular "statement"
// spelling is normalised to it before it ever reaches this package's
// internals.
const (
	PurposeStatements = "statements"
	PurposeReminder   = "reminder"
)

// Status values, matching notifications.status's CHECK constraint exactly
// (docs/schema_design.sql:442-443). Under ChannelZaloManual (the only wired
// sender) a row only ever moves StatusQueued -> StatusSent; StatusDelivered
// is never set by this codebase — no delivery receipt exists to justify it.
const (
	StatusQueued    = "queued"
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// Notification is one row of notifications, mapped onto exactly the columns
// the frozen schema defines (docs/schema_design.sql:434-451) — no
// message_text, no contact_id. The message text a bulk send produces is
// never persisted: it is rendered fresh from the same invoice snapshot
// PeriodFigures reads and returned only in the API response (see
// Service.BulkSend). A notification's contact is reached indirectly, through
// statement_id -> statements.contact_id.
type Notification struct {
	ID            uuid.UUID `gorm:"primaryKey"`
	TeacherID     uuid.UUID
	StatementID   uuid.UUID
	Channel       string
	Purpose       string
	Status        string
	ProviderMsgID *string
	ErrorMessage  *string
	SentAt        *time.Time
	// RunID groups the zalo_personal rows one bulk send queued into a paced
	// background run. Rows of the copy-paste channels never carry it, and the
	// FK clears it (SET NULL) if the run record is ever deleted — the ledger
	// row outlives its run.
	RunID *uuid.UUID

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot
// silently break the mapping.
func (Notification) TableName() string { return "notifications" }

// Run status values, matching notification_runs.status's CHECK constraint
// exactly. Running is the only non-terminal status: Completed and Expired are
// the run's own verdicts, Interrupted means the process died underneath it
// and its remaining rows are still queued, awaiting a manual resume.
const (
	RunStatusRunning     = "running"
	RunStatusCompleted   = "completed"
	RunStatusInterrupted = "interrupted"
	RunStatusExpired     = "expired"
)

// Run is one row of notification_runs: a single paced zalo_personal sending
// pass over a billing period. Progress counters are never stored here — they
// are derived by counting the notifications rows that carry this run's ID, so
// a crash can never leave a counter lying about the rows.
type Run struct {
	ID              uuid.UUID `gorm:"primaryKey"`
	TeacherID       uuid.UUID
	BillingPeriodID uuid.UUID
	Purpose         string
	Status          string
	CreatedAt       time.Time
	FinishedAt      *time.Time
}

// TableName pins the table explicitly so a later model rename cannot
// silently break the mapping.
func (Run) TableName() string { return "notification_runs" }

// RunCounts is a run's DB-derived progress snapshot. Total minus Sent minus
// Failed is the queued remainder.
type RunCounts struct {
	Total  int
	Sent   int
	Failed int
}

// QueuedRunRow is one still-queued notification of a run, joined with the
// statement's contact so a resume can re-render its message without a second
// lookup per row.
type QueuedRunRow struct {
	NotificationID uuid.UUID
	StatementID    uuid.UUID
	ContactID      uuid.UUID
}
