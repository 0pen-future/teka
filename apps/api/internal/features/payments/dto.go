package payments

import (
	"time"

	"github.com/google/uuid"
)

// dateLayout is the wire form for date-only fields — YYYY-MM-DD, matching
// billing's convention (billing/dto.go) so the two features never disagree
// about how a DATE column travels over JSON.
const dateLayout = "2006-01-02"

// RecordPaymentRequest is the POST /payments body.
type RecordPaymentRequest struct {
	ContactID     uuid.UUID `json:"contact_id" binding:"required"`
	Amount        int64     `json:"amount" binding:"required,gt=0"`
	Method        string    `json:"method" binding:"required,oneof=cash transfer other"`
	ReceivedOn    string    `json:"received_on" binding:"required,datetime=2006-01-02"`
	ReferenceCode *string   `json:"reference_code" binding:"omitempty,max=50"`
	Note          *string   `json:"note" binding:"omitempty,max=1000"`
}

// ReallocationLine is one invoice's new amount in a ReallocateRequest.
type ReallocationLine struct {
	InvoiceID uuid.UUID `json:"invoice_id" binding:"required"`
	Amount    int64     `json:"amount" binding:"required,gt=0"`
}

// ReallocateRequest is the PUT /payments/:id/allocations body — a teacher's
// override of the split D8 chose automatically. Business validation (same
// contact, invoice status, per-invoice room, the payment total) happens in
// Service.Reallocate; binding only enforces the request's own shape.
type ReallocateRequest struct {
	Allocations []ReallocationLine `json:"allocations" binding:"required,min=1,dive"`
}

// ReverseRequest is the POST /payments/:id/reverse body. Reason is stored on
// the reversal row's note column — payments has no dedicated reason field.
type ReverseRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=1000"`
}

// AllocationResponse is one invoice's slice of a payment, plus that invoice's
// money fields after the allocation was applied. Every payment read response
// carries this breakdown — the frontend must never reimplement the D8
// oldest-debt-first rule itself; the server states plainly which child each
// đồng landed on (web plan 08's hard contract requirement).
type AllocationResponse struct {
	InvoiceID   uuid.UUID `json:"invoice_id"`
	StudentID   uuid.UUID `json:"student_id"`
	StudentName string    `json:"student_name"`
	PeriodID    uuid.UUID `json:"period_id"`
	Amount      int64     `json:"amount"`
	AllocatedBy string    `json:"allocated_by"`
	TotalDue    int64     `json:"total_due"`
	PaidAmount  int64     `json:"paid_amount"`
	Outstanding int64     `json:"outstanding"`
}

// PaymentResponse is the wire shape of a recorded payment, always including
// its allocation breakdown.
type PaymentResponse struct {
	ID                uuid.UUID            `json:"id"`
	ContactID         uuid.UUID            `json:"contact_id"`
	Amount            int64                `json:"amount"`
	Method            string               `json:"method"`
	ReceivedOn        string               `json:"received_on"`
	ReferenceCode     *string              `json:"reference_code,omitempty"`
	Note              *string              `json:"note,omitempty"`
	ReversesPaymentID *uuid.UUID           `json:"reverses_payment_id,omitempty"`
	ReversedAt        *time.Time           `json:"reversed_at,omitempty"`
	Allocations       []AllocationResponse `json:"allocations"`
	UnallocatedAmount int64                `json:"unallocated_amount"`
	CreatedAt         time.Time            `json:"created_at"`
}

// FromDetail maps a PaymentDetail (payment + its allocation rows) onto the
// wire response.
func FromDetail(d PaymentDetail) PaymentResponse {
	allocs := make([]AllocationResponse, 0, len(d.Allocations))
	for _, a := range d.Allocations {
		allocs = append(allocs, AllocationResponse{
			InvoiceID:   a.InvoiceID,
			StudentID:   a.StudentID,
			StudentName: a.StudentName,
			PeriodID:    a.PeriodID,
			Amount:      a.Amount,
			AllocatedBy: a.AllocatedBy,
			TotalDue:    a.TotalDue,
			PaidAmount:  a.PaidAmount,
			Outstanding: a.TotalDue - a.PaidAmount,
		})
	}

	return PaymentResponse{
		ID:                d.Payment.ID,
		ContactID:         d.Payment.ContactID,
		Amount:            d.Payment.Amount,
		Method:            d.Payment.Method,
		ReceivedOn:        d.Payment.ReceivedOn.Format(dateLayout),
		ReferenceCode:     d.Payment.ReferenceCode,
		Note:              d.Payment.Note,
		ReversesPaymentID: d.Payment.ReversesPaymentID,
		ReversedAt:        d.Payment.ReversedAt,
		Allocations:       allocs,
		UnallocatedAmount: d.UnallocatedAmount,
		CreatedAt:         d.Payment.CreatedAt,
	}
}
