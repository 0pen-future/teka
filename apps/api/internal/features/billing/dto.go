package billing

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// dateLayout is the wire form of every DATE field in this package.
const dateLayout = "2006-01-02"

// EnsurePeriodRequest opens (or fetches, if it already exists) a billing
// period for one calendar month.
type EnsurePeriodRequest struct {
	Year  int `json:"year" binding:"required,min=2020,max=2100"`
	Month int `json:"month" binding:"required,min=1,max=12"`
}

// PeriodResponse is the public billing_periods shape.
type PeriodResponse struct {
	ID          uuid.UUID  `json:"id"`
	Year        int16      `json:"year"`
	Month       int16      `json:"month"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	Status      string     `json:"status"`
	ClosedAt    *time.Time `json:"closed_at"`
}

// FromPeriodModel maps one billing period onto its wire response.
func FromPeriodModel(p *Period) PeriodResponse {
	return PeriodResponse{
		ID:          p.ID,
		Year:        p.Year,
		Month:       p.Month,
		PeriodStart: p.PeriodStart.Format(dateLayout),
		PeriodEnd:   p.PeriodEnd.Format(dateLayout),
		Status:      p.Status,
		ClosedAt:    p.ClosedAt,
	}
}

// FromPeriodModels maps a page of billing periods onto their wire responses.
func FromPeriodModels(rows []Period) []PeriodResponse {
	out := make([]PeriodResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromPeriodModel(&rows[i]))
	}
	return out
}

// PreviewLine is one enrollment's charge line inside a PreviewInvoice.
// class_id and present_count come from the compute step's attendance tally,
// not the persisted invoice_lines row, which carries neither column.
type PreviewLine struct {
	EnrollmentID  uuid.UUID `json:"enrollment_id"`
	ClassID       uuid.UUID `json:"class_id"`
	ClassName     string    `json:"class_name"`
	BillableCount int       `json:"billable_count"`
	AbsentCount   int       `json:"absent_count"`
	PresentCount  int       `json:"present_count"`
	UnitPrice     int64     `json:"unit_price"`
	Amount        int64     `json:"amount"`
}

// PreviewInvoice is one student's computed fee for the period. InvoiceID is
// null until Draft has persisted it.
type PreviewInvoice struct {
	InvoiceID       *uuid.UUID    `json:"invoice_id"`
	StudentID       uuid.UUID     `json:"student_id"`
	ContactID       uuid.UUID     `json:"contact_id"`
	StudentName     string        `json:"student_name"`
	ContactName     string        `json:"contact_name"`
	Lines           []PreviewLine `json:"lines"`
	OpeningBalance  int64         `json:"opening_balance"`
	CurrentCharge   int64         `json:"current_charge"`
	AdjustmentTotal int64         `json:"adjustment_total"`
	TotalDue        int64         `json:"total_due"`
}

// PreviewTotals summarises a PreviewResponse across every student in it.
type PreviewTotals struct {
	StudentCount    int   `json:"student_count"`
	TotalOpening    int64 `json:"total_opening"`
	TotalCharge     int64 `json:"total_charge"`
	TotalAdjustment int64 `json:"total_adjustment"`
	TotalDue        int64 `json:"total_due"`
}

// PreviewResponse is the chốt sổ review screen payload (R4): every student in
// the period with their per-class session counts, amounts, carried debt, and
// a grand total. GET /preview returns it unpersisted; POST /draft returns
// the same shape after writing it, with invoice_id populated.
type PreviewResponse struct {
	Invoices []PreviewInvoice `json:"invoices"`
	Totals   PreviewTotals    `json:"totals"`
}

// buildPreviewResponse renders one PeriodCompute as the wire response both
// Preview and Draft return, so the two endpoints never diverge on shape or
// rounding. invoiceIDByStudent is nil for Preview (invoice_id always null)
// and populated per student for Draft. Lines with zero billable_count and
// zero absent_count are omitted from the rendered list — they stay in the
// database (a zeroed line, never a deleted one) but add nothing a teacher
// needs to review.
func buildPreviewResponse(compute *PeriodCompute, invoiceIDByStudent map[uuid.UUID]uuid.UUID) *PreviewResponse {
	invoices := make([]PreviewInvoice, 0, len(compute.Invoices))
	var totals PreviewTotals
	for _, ci := range compute.Invoices {
		lines := make([]PreviewLine, 0, len(ci.Lines))
		for _, l := range ci.Lines {
			if l.BillableCount == 0 && l.AbsentCount == 0 {
				continue
			}
			tally := compute.TalliesByEnrollment[l.EnrollmentID]
			lines = append(lines, PreviewLine{
				EnrollmentID:  l.EnrollmentID,
				ClassID:       tally.ClassID,
				ClassName:     l.ClassName,
				BillableCount: l.BillableCount,
				AbsentCount:   l.AbsentCount,
				PresentCount:  tally.PresentCount,
				UnitPrice:     l.UnitPrice,
				Amount:        l.Amount,
			})
		}

		var invoiceID *uuid.UUID
		if resolvedID, ok := invoiceIDByStudent[ci.StudentID]; ok {
			idCopy := resolvedID
			invoiceID = &idCopy
		}

		invoices = append(invoices, PreviewInvoice{
			InvoiceID:       invoiceID,
			StudentID:       ci.StudentID,
			ContactID:       ci.ContactID,
			StudentName:     ci.StudentName,
			ContactName:     ci.ContactName,
			Lines:           lines,
			OpeningBalance:  ci.OpeningBalance,
			CurrentCharge:   ci.CurrentCharge,
			AdjustmentTotal: ci.AdjustmentTotal,
			TotalDue:        ci.TotalDue,
		})
		totals.StudentCount++
		totals.TotalOpening += ci.OpeningBalance
		totals.TotalCharge += ci.CurrentCharge
		totals.TotalAdjustment += ci.AdjustmentTotal
		totals.TotalDue += ci.TotalDue
	}

	sort.Slice(invoices, func(i, j int) bool {
		if invoices[i].StudentName != invoices[j].StudentName {
			return invoices[i].StudentName < invoices[j].StudentName
		}
		return invoices[i].StudentID.String() < invoices[j].StudentID.String()
	})

	return &PreviewResponse{Invoices: invoices, Totals: totals}
}

// UnconfirmedSession is one past-or-future session without confirmed
// attendance, as reported both in a 409 close-blocked response and in a
// successful close's future_unconfirmed_sessions warning.
type UnconfirmedSession struct {
	SessionID   uuid.UUID `json:"session_id"`
	ClassID     uuid.UUID `json:"class_id"`
	ClassName   string    `json:"class_name"`
	SessionDate string    `json:"session_date"`
	Status      string    `json:"status"`
}

// CloseWarnings holds non-blocking information a successful close still
// wants the teacher to see.
type CloseWarnings struct {
	// FutureUnconfirmedSessions are sessions dated after today but still
	// inside the period without confirmed attendance — closing early is
	// legal (Architecture), so these do not block, but attendance confirmed
	// on them later must flow through a phase 4 adjustment on the next
	// period rather than this one.
	FutureUnconfirmedSessions []UnconfirmedSession `json:"future_unconfirmed_sessions"`
}

// CloseResponse is POST /billing-periods/:id/close's success payload:
// closed period plus how many invoices landed in each terminal state and
// what they add up to.
type CloseResponse struct {
	Period      PeriodResponse `json:"period"`
	IssuedCount int64          `json:"issued_count"`
	VoidedCount int64          `json:"voided_count"`
	// TotalDue sums total_due across every non-void invoice this close
	// produced — the money now owed for the period.
	TotalDue int64         `json:"total_due"`
	Warnings CloseWarnings `json:"warnings"`
}

// VoidInvoiceRequest is POST /invoices/:id/void's body. Reason mirrors the
// non-blank discipline invoice_adjustments' CHECK enforces
// (docs/schema_design.sql:351) — not itself DB-enforced on invoices, but
// applied here by the same convention.
type VoidInvoiceRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

// InvoiceResponse is the public invoices shape returned by void. A fuller
// detail endpoint (with lines) is a later phase's concern; this is
// deliberately the bare row.
type InvoiceResponse struct {
	ID              uuid.UUID  `json:"id"`
	PeriodID        uuid.UUID  `json:"period_id"`
	StudentID       uuid.UUID  `json:"student_id"`
	ContactID       uuid.UUID  `json:"contact_id"`
	StudentName     string     `json:"student_name"`
	ContactName     string     `json:"contact_name"`
	OpeningBalance  int64      `json:"opening_balance"`
	CurrentCharge   int64      `json:"current_charge"`
	AdjustmentTotal int64      `json:"adjustment_total"`
	TotalDue        int64      `json:"total_due"`
	PaidAmount      int64      `json:"paid_amount"`
	Status          string     `json:"status"`
	VoidReason      *string    `json:"void_reason"`
	VoidedAt        *time.Time `json:"voided_at"`
}

// FromInvoiceModel maps one invoice onto its wire response.
func FromInvoiceModel(inv *Invoice) InvoiceResponse {
	return InvoiceResponse{
		ID:              inv.ID,
		PeriodID:        inv.PeriodID,
		StudentID:       inv.StudentID,
		ContactID:       inv.ContactID,
		StudentName:     inv.StudentName,
		ContactName:     inv.ContactName,
		OpeningBalance:  inv.OpeningBalance,
		CurrentCharge:   inv.CurrentCharge,
		AdjustmentTotal: inv.AdjustmentTotal,
		TotalDue:        inv.TotalDue,
		PaidAmount:      inv.PaidAmount,
		Status:          inv.Status,
		VoidReason:      inv.VoidReason,
		VoidedAt:        inv.VoidedAt,
	}
}

// AdjustmentRequest is POST /invoices/:id/adjustments's body: a signed manual
// correction plus its mandatory reason. Reason mirrors the non-blank
// discipline invoice_adjustments' CHECK enforces
// (docs/schema_design.sql:351); Amount has no min/max tag — zero is rejected
// by AddAdjustment itself with a field-specific message, and there is no
// business ceiling on a correction's size.
type AdjustmentRequest struct {
	Amount int64  `json:"amount" binding:"required"`
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

// AdjustmentResponse is the public invoice_adjustments shape: one row of the
// append-only audit trail R4 exposes via GET /invoices/:id/adjustments.
type AdjustmentResponse struct {
	ID              uuid.UUID  `json:"id"`
	InvoiceID       uuid.UUID  `json:"invoice_id"`
	Amount          int64      `json:"amount"`
	Reason          string     `json:"reason"`
	SourceSessionID *uuid.UUID `json:"source_session_id"`
	CreatedAt       time.Time  `json:"created_at"`
}

// FromAdjustmentModel maps one invoice_adjustments row onto its wire
// response.
func FromAdjustmentModel(adj *InvoiceAdjustment) AdjustmentResponse {
	return AdjustmentResponse{
		ID:              adj.ID,
		InvoiceID:       adj.InvoiceID,
		Amount:          adj.Amount,
		Reason:          adj.Reason,
		SourceSessionID: adj.SourceSessionID,
		CreatedAt:       adj.CreatedAt,
	}
}

// FromAdjustmentModels maps an invoice's adjustment audit trail onto its
// wire response.
func FromAdjustmentModels(rows []InvoiceAdjustment) []AdjustmentResponse {
	out := make([]AdjustmentResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromAdjustmentModel(&rows[i]))
	}
	return out
}

// ReconciliationEntry is one student's carried adjustment inside a
// ReconciliationResponse.
type ReconciliationEntry struct {
	StudentID uuid.UUID `json:"student_id"`
	PeriodID  uuid.UUID `json:"period_id"`
	InvoiceID uuid.UUID `json:"invoice_id"`
	Amount    int64     `json:"amount"`
}

// ReconciliationResponse is what a post-close reconciliation produced: zero
// or more carried adjustments. Reserved for a future endpoint that exposes a
// reconciliation result directly; today it is surfaced only indirectly, via
// attendance.Response's warning field when the automatic reconciliation
// attempt inside Confirm fails.
type ReconciliationResponse struct {
	Adjustments []ReconciliationEntry `json:"adjustments"`
}
