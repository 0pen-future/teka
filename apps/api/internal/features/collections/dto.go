package collections

import "github.com/google/uuid"

// ContactBalanceRow is one family's merged balance for the requested period
// — the by-contact view's wire shape, returned directly by the repository
// since this reporting package has no separate internal model to map from.
type ContactBalanceRow struct {
	ContactID uuid.UUID `json:"contact_id"`
	FullName  string    `json:"full_name"`
	// Phone is null unless the caller may see the contact's phone (owner,
	// reports oversight, or an active hoc_vu stint over one of the contact's
	// enrolled students). The repository always fills it; the service nils it
	// by PhoneVisible before the row leaves.
	Phone *string `json:"phone"`
	// PhoneVisible is the caller's derived hoc_vu grant for this contact —
	// service-internal input to the mask, never serialized.
	PhoneVisible    bool                     `json:"-"`
	ContactArchived bool                     `json:"contact_archived"`
	StudentCount    int64                    `json:"student_count"`
	TotalDue        int64                    `json:"total_due"`
	TotalPaid       int64                    `json:"total_paid"`
	Outstanding     int64                    `json:"outstanding"`
	PaymentStatus   string                   `json:"payment_status"`
	Invoices        []ContactChildInvoiceRow `json:"invoices"`
}

// ContactChildInvoiceRow is one child's invoice nested under its contact's
// balance row.
type ContactChildInvoiceRow struct {
	InvoiceID   uuid.UUID `json:"invoice_id"`
	StudentName string    `json:"student_name"`
	TotalDue    int64     `json:"total_due"`
	PaidAmount  int64     `json:"paid_amount"`
	Outstanding int64     `json:"outstanding"`
}

// ClassCollectionRow is one invoice line for the requested class — the
// by-class view's wire shape. LineAmount is the line's own charge; the
// Invoice* fields describe the whole invoice that line belongs to, kept
// distinctly named so a client never confuses a line's charge with its
// invoice's total.
type ClassCollectionRow struct {
	InvoiceID             uuid.UUID `json:"invoice_id"`
	StudentID             uuid.UUID `json:"student_id"`
	StudentName           string    `json:"student_name"`
	ContactID             uuid.UUID `json:"contact_id"`
	ContactName           string    `json:"contact_name"`
	ClassName             string    `json:"class_name"`
	BillableCount         int       `json:"billable_count"`
	AbsentCount           int       `json:"absent_count"`
	LineAmount            int64     `json:"line_amount"`
	InvoiceOpeningBalance int64     `json:"invoice_opening_balance"`
	InvoiceTotalDue       int64     `json:"invoice_total_due"`
	InvoicePaidAmount     int64     `json:"invoice_paid_amount"`
	InvoiceOutstanding    int64     `json:"invoice_outstanding"`
	PaymentStatus         string    `json:"payment_status"`
}

// SummaryResponse aggregates the whole period unconditionally — no filter,
// no pagination, one row.
type SummaryResponse struct {
	StudentCount        int64 `json:"student_count"`
	ContactCount        int64 `json:"contact_count"`
	TotalDue            int64 `json:"total_due"`
	TotalPaid           int64 `json:"total_paid"`
	TotalOutstanding    int64 `json:"total_outstanding"`
	PaidContactCount    int64 `json:"paid_contact_count"`
	UnpaidContactCount  int64 `json:"unpaid_contact_count"`
	PartialContactCount int64 `json:"partial_contact_count"`
	UnallocatedCredit   int64 `json:"unallocated_credit"`
}
