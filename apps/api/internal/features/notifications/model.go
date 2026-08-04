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
	ChannelZaloManual = "zalo_manual"
	ChannelZaloZNS    = "zalo_zns"
	ChannelSMS        = "sms"
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

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot
// silently break the mapping.
func (Notification) TableName() string { return "notifications" }
