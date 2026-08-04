package statements

import (
	"time"

	"github.com/google/uuid"
)

// StatementResponse is the teacher-facing statement shape. It carries the
// parent-facing token/url — built fresh per response by Service.ToResponse,
// never read back from storage — so this DTO must only ever leave a
// teacher-authenticated endpoint.
type StatementResponse struct {
	ID            uuid.UUID  `json:"id"`
	ContactID     uuid.UUID  `json:"contact_id"`
	ContactName   string     `json:"contact_name"`
	Phone         string     `json:"phone"`
	TotalDue      int64      `json:"total_due"`
	URL           string     `json:"url"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	ViewCount     int        `json:"view_count"`
	FirstViewedAt *time.Time `json:"first_viewed_at"`
	LastViewedAt  *time.Time `json:"last_viewed_at"`
}

// GenerateResponse is the result of one POST .../statements/generate call.
type GenerateResponse struct {
	Created        int                 `json:"created"`
	Refreshed      int                 `json:"refreshed"`
	SkippedRevoked int                 `json:"skipped_revoked"`
	Statements     []StatementResponse `json:"statements"`
}

// dateLayout is the wire form of every DATE-only field the public payload
// carries — matches the layout billing and collections already use for the
// same DATE columns.
const dateLayout = "2006-01-02"

// Adjustment kinds a public statement's Adjustments field can carry. Never a
// third value, and never the teacher's free-text reason — see
// PublicAdjustment.
const (
	adjustmentKindManual     = "manual"
	adjustmentKindCorrection = "correction"
)

// PublicSession is one class session as it stands live right now —
// attendance corrected after the statement's period closed shows up here
// immediately, unlike the snapshot figures elsewhere in PublicChild.
type PublicSession struct {
	Date    string `json:"date"`
	Status  string `json:"status"`
	Counted bool   `json:"counted"`
}

// PublicClass is one child's one enrollment for the period: the snapshot
// charge (unit price, counts, amount) plus its live session list.
type PublicClass struct {
	ClassName     string          `json:"class_name"`
	UnitPrice     int64           `json:"unit_price"`
	BillableCount int             `json:"billable_count"`
	AbsentCount   int             `json:"absent_count"`
	Amount        int64           `json:"amount"`
	Sessions      []PublicSession `json:"sessions"`
}

// PublicAdjustment is one adjustment posted on this period's own invoice for
// this child. Kind is derived from whether the adjustment carries a
// source_session_id (correction) or not (manual) — never the teacher's
// free-text reason, which must never appear in a public payload.
type PublicAdjustment struct {
	Amount int64  `json:"amount"`
	Kind   string `json:"kind"`
}

// PublicCarriedAdjustment aggregates every correction that was posted on a
// later invoice but whose source session falls inside this period's date
// range — the "why does this total not match what I saw last month"
// explanation for a post-close attendance correction. SessionDates names
// which sessions drove it instead of repeating the teacher's private
// adjustment reason.
type PublicCarriedAdjustment struct {
	Amount       int64    `json:"amount"`
	SessionDates []string `json:"session_dates"`
}

// PublicChild is one student's billing summary for the period.
type PublicChild struct {
	StudentName       string                   `json:"student_name"`
	DisplayNote       *string                  `json:"display_note"`
	OpeningBalance    int64                    `json:"opening_balance"`
	Classes           []PublicClass            `json:"classes"`
	Adjustments       []PublicAdjustment       `json:"adjustments"`
	CarriedAdjustment *PublicCarriedAdjustment `json:"carried_adjustment"`
	Subtotal          int64                    `json:"subtotal"`
}

// PublicTotals is the family's period total, summed across every child's
// invoice — opening balance, current charge, and adjustments are snapshot
// figures; paid and outstanding are read from invoices.paid_amount, itself
// kept in sync by the payments package's own allocation summation rather
// than re-derived here.
type PublicTotals struct {
	OpeningBalance  int64 `json:"opening_balance"`
	CurrentCharge   int64 `json:"current_charge"`
	AdjustmentTotal int64 `json:"adjustment_total"`
	TotalDue        int64 `json:"total_due"`
	Paid            int64 `json:"paid"`
	Outstanding     int64 `json:"outstanding"`
}

// PublicInvoicePayment is one child's payment breakdown within the family
// total.
type PublicInvoicePayment struct {
	StudentName string `json:"student_name"`
	TotalDue    int64  `json:"total_due"`
	Paid        int64  `json:"paid"`
	Outstanding int64  `json:"outstanding"`
}

// PublicPayments is the family's payment breakdown for the period.
type PublicPayments struct {
	TotalPaid int64                  `json:"total_paid"`
	ByInvoice []PublicInvoicePayment `json:"by_invoice"`
}

// PublicQR is the family's payment QR block. Omitted (nil) entirely — never
// present with an empty image_url — when the teacher's bank details are not
// configured.
type PublicQR struct {
	ImageURL string `json:"image_url"`
	Amount   int64  `json:"amount"`
	Note     string `json:"note"`
}

// PublicStatement is the parent-facing statement payload: the one response
// shape this package's public, unauthenticated endpoint ever returns. Every
// money field is int64 (VND, no subunits) — see the package's qr.go and
// render.go for how it is assembled.
type PublicStatement struct {
	ContactName string         `json:"contact_name"`
	Period      string         `json:"period"`
	Children    []PublicChild  `json:"children"`
	Totals      PublicTotals   `json:"totals"`
	Payments    PublicPayments `json:"payments"`
	QR          *PublicQR      `json:"qr"`
}
