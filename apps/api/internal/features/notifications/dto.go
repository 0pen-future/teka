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
	Channel string `json:"channel" binding:"omitempty,oneof=zalo_manual zalo_zns sms zalo_personal"`
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
	// RunID names the background run auto-sending this call's zalo_personal
	// rows; poll GET .../notifications/run for its progress. Null when
	// nothing is being auto-sent — the copy-paste channels, or a personal
	// send whose every contact was unmapped.
	RunID *uuid.UUID `json:"run_id"`
	// PersonalQueuedCount and FallbackManualCount split a zalo_personal
	// send's rows: mapped contacts the run delivers itself vs unmapped
	// contacts left in BulkText for the teacher to copy-paste. Both zero for
	// other channels.
	PersonalQueuedCount int `json:"personal_queued_count"`
	FallbackManualCount int `json:"fallback_manual_count"`
	// BulkText joins every copy-paste row's message, separated by a blank
	// line, so a teacher can copy once when pasting into a broadcast tool
	// instead of copying each contact's message individually. Rows a
	// zalo_personal run delivers itself are excluded.
	BulkText string        `json:"bulk_text"`
	Rows     []BulkSendRow `json:"rows"`
}

// RunSnapshotResponse is the GET .../notifications/run body: the period's
// latest zalo_personal run with its progress derived by counting the run's
// rows. The zero value — Active false, RunID nil — means the period has never
// had a run, which is an ordinary answer here, not a 404.
type RunSnapshotResponse struct {
	Active  bool       `json:"active"`
	RunID   *uuid.UUID `json:"run_id"`
	Status  string     `json:"status,omitempty"`
	Purpose string     `json:"purpose,omitempty"`
	Total   int        `json:"total"`
	Sent    int        `json:"sent"`
	Failed  int        `json:"failed"`
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
	ID          uuid.UUID `json:"id"`
	ContactID   uuid.UUID `json:"contact_id"`
	ContactName string    `json:"contact_name"`
	Phone       string    `json:"phone"`
	Channel     string    `json:"channel"`
	Purpose     string    `json:"purpose"`
	Status      string    `json:"status"`
	// ErrorMessage is a failed row's teacher-facing reason — the ledger is
	// the only place a teacher can learn why a row was not delivered.
	ErrorMessage *string `json:"error_message,omitempty"`
	// RunID pins the row to the paced run that sent it, letting a client
	// show a run's failures without adopting older runs' rows.
	RunID     *uuid.UUID `json:"run_id,omitempty"`
	SentAt    *time.Time `json:"sent_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// fromListRow maps a ledger row onto its wire DTO.
func fromListRow(r ListRow) NotificationResponse {
	return NotificationResponse{
		ID:           r.ID,
		ContactID:    r.ContactID,
		ContactName:  r.ContactName,
		Phone:        r.Phone,
		Channel:      r.Channel,
		Purpose:      r.Purpose,
		Status:       r.Status,
		ErrorMessage: r.ErrorMessage,
		RunID:        r.RunID,
		SentAt:       r.SentAt,
		CreatedAt:    r.CreatedAt,
	}
}
