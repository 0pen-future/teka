package notifications

import (
	"time"

	"github.com/google/uuid"
)

// bulkSendPurposeValues accepts both the singular "statement" spelling and
// the database's own plural "statements" spelling in the request body — see
// normalizePurpose for where the singular form is mapped onto
// PurposeStatements before it reaches the service layer. The database's
// purpose CHECK constraint only ever accepts the plural form
// (docs/schema_design.sql:440-441); "statement" never reaches SQL.
const purposeStatementSingular = "statement"

// normalizePurpose maps a request's purpose spelling onto the Go/DB
// constant. Called only after binding's oneof tag has already rejected any
// value that is not one of the three accepted spellings.
func normalizePurpose(raw string) string {
	if raw == purposeStatementSingular {
		return PurposeStatements
	}
	return raw
}

// BulkSendRequest is the POST .../notifications/bulk body. Purpose accepts
// both "statement" and "statements" for the plural DB value — see
// normalizePurpose. Channel is optional; an empty value falls back to
// cfg.Notifications.DefaultChannel.
type BulkSendRequest struct {
	Purpose string `json:"purpose" binding:"required,oneof=statement statements reminder"`
	Channel string `json:"channel" binding:"omitempty,oneof=zalo_manual zalo_zns sms"`
}

// BulkSendRow is one contact's queued notification, with everything a
// teacher needs to copy and send it by hand under zalo_manual: the rendered
// text (never persisted — see model.go) and the statement link it ends with.
type BulkSendRow struct {
	NotificationID uuid.UUID `json:"notification_id"`
	ContactID      uuid.UUID `json:"contact_id"`
	ContactName    string    `json:"contact_name"`
	Phone          string    `json:"phone"`
	Channel        string    `json:"channel"`
	Purpose        string    `json:"purpose"`
	Status         string    `json:"status"`
	MessageText    string    `json:"message_text"`
	URL            string    `json:"url"`
	Collapsed      bool      `json:"collapsed"`
}

// BulkSendResponse is the result of one bulk send call.
type BulkSendResponse struct {
	QueuedCount      int `json:"queued_count"`
	SkippedPaidCount int `json:"skipped_paid_count"`
	CollapsedCount   int `json:"collapsed_count"`
	// BulkText joins every row's message, separated by a blank line, so a
	// teacher can copy once when pasting into a broadcast tool instead of
	// copying each contact's message individually.
	BulkText string        `json:"bulk_text"`
	Rows     []BulkSendRow `json:"rows"`
}

// MarkSentRequest is the POST /notifications/mark-sent body.
type MarkSentRequest struct {
	IDs []uuid.UUID `json:"ids" binding:"required,min=1,dive"`
}

// NotificationResponse is one ledger row's wire shape for
// GET .../notifications. There is no message_text field: the text is never
// persisted (see model.go), so a past send's text cannot be replayed here —
// only its delivery bookkeeping can.
type NotificationResponse struct {
	ID          uuid.UUID  `json:"id"`
	ContactID   uuid.UUID  `json:"contact_id"`
	ContactName string     `json:"contact_name"`
	Phone       string     `json:"phone"`
	Channel     string     `json:"channel"`
	Purpose     string     `json:"purpose"`
	Status      string     `json:"status"`
	SentAt      *time.Time `json:"sent_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// fromListRow maps a ledger row onto its wire DTO.
func fromListRow(r ListRow) NotificationResponse {
	return NotificationResponse{
		ID:          r.ID,
		ContactID:   r.ContactID,
		ContactName: r.ContactName,
		Phone:       r.Phone,
		Channel:     r.Channel,
		Purpose:     r.Purpose,
		Status:      r.Status,
		SentAt:      r.SentAt,
		CreatedAt:   r.CreatedAt,
	}
}
