// Package billing owns billing_periods, invoices, invoice_lines, and
// invoice_adjustments: the monthly close and the money it produces. PRD R3:
// "fee = billable sessions in period × enrollment unit price + carried debt,
// computed per student independently."
package billing

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Billing period status values — mirrors billing_periods.status CHECK.
const (
	PeriodOpen   = "open"
	PeriodClosed = "closed"
)

// Invoice status values — mirrors invoices.status CHECK.
const (
	InvoiceDraft         = "draft"
	InvoiceIssued        = "issued"
	InvoicePartiallyPaid = "partially_paid"
	InvoicePaid          = "paid"
	InvoiceVoid          = "void"
)

// Period is one teacher's calendar month close. At most one row per
// (teacher_id, year, month) among non-deleted rows.
type Period struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	Year      int16
	Month     int16
	// PeriodStart/PeriodEnd are Postgres DATE columns: the first and last
	// calendar day of the month, resolved in the teacher's timezone.
	PeriodStart time.Time
	PeriodEnd   time.Time
	Status      string
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Period) TableName() string { return "billing_periods" }

// Invoice is one student's debt for one billing period — an immutable
// snapshot once the period closes.
//
// Deliberately has no gorm.DeletedAt: this table has no deleted_at column.
// Voiding an invoice uses status='void', never a soft delete — a soft-deleted
// financial row would make the money already reported to a parent
// unreconcilable (schema note (i), docs/schema_design.sql:512).
type Invoice struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	PeriodID  uuid.UUID
	StudentID uuid.UUID
	ContactID uuid.UUID
	// StudentName/ContactName are snapshots taken at close time so a hard
	// delete of students/contacts (retention) never orphans a historical
	// invoice.
	StudentName     string
	ContactName     string
	OpeningBalance  int64
	CurrentCharge   int64
	AdjustmentTotal int64
	TotalDue        int64
	PaidAmount      int64
	Status          string
	VoidReason      *string
	VoidedAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Invoice) TableName() string { return "invoices" }

// InvoiceLine is one enrollment's charge within an invoice — one row per
// enrollment, so a student in two classes gets two lines under one invoice.
//
// Deliberately has no gorm.DeletedAt and no UpdatedAt: soft-deleting a line
// would make the invoice total no longer explainable by its own detail rows.
// Corrections use invoice_adjustments instead, which carries a reason.
type InvoiceLine struct {
	ID            uuid.UUID `gorm:"primaryKey"`
	TeacherID     uuid.UUID
	InvoiceID     uuid.UUID
	EnrollmentID  uuid.UUID
	ClassName     string
	BillableCount int
	AbsentCount   int
	UnitPrice     int64
	Amount        int64
	CreatedAt     time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (InvoiceLine) TableName() string { return "invoice_lines" }

// InvoiceAdjustment is a manual correction to an invoice, always carrying a
// reason (R4). Reversing one never deletes it — a counter-signed adjustment
// is created instead; deleted_at exists only for a same-session data-entry
// mistake made before any notification went out.
type InvoiceAdjustment struct {
	ID              uuid.UUID `gorm:"primaryKey"`
	TeacherID       uuid.UUID
	InvoiceID       uuid.UUID
	Amount          int64
	Reason          string
	SourceSessionID *uuid.UUID
	CreatedAt       time.Time
	DeletedAt       gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (InvoiceAdjustment) TableName() string { return "invoice_adjustments" }
