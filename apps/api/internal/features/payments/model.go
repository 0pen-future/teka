// Package payments owns the write side of PRD R7: recording money against a
// contact and splitting it across that family's invoices. debt is recorded
// per student (billing), money is received per contact (payments) —
// payment_allocations is the bridge between the two axes
// (docs/schema_design.sql:27).
package payments

import (
	"time"

	"github.com/google/uuid"
)

// Payment method values — mirrors payments.method CHECK
// (docs/schema_design.sql:366).
const (
	MethodCash     = "cash"
	MethodTransfer = "transfer"
	MethodOther    = "other"
)

// Allocation source values — mirrors payment_allocations.allocated_by CHECK
// (docs/schema_design.sql:391). 'auto' is the D8 default rule; a teacher
// override rewrites the rows as 'manual' (phase 2).
const (
	AllocatedAuto   = "auto"
	AllocatedManual = "manual"
)

// Payment is one amount a contact handed over. Deliberately has no
// gorm.DeletedAt: this table has no deleted_at column — a mistaken payment is
// reversed with a counter-entry (reverses_payment_id), never deleted (schema
// note (i), docs/schema_design.sql:512).
type Payment struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	// CenterID anchors the row in the center it was created in; it never
	// changes, even when the creating teacher later moves centers.
	CenterID          uuid.UUID
	ContactID         uuid.UUID
	Amount            int64
	Method            string
	ReceivedOn        time.Time
	ReferenceCode     *string
	Note              *string
	ReversesPaymentID *uuid.UUID
	ReversedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Payment) TableName() string { return "payments" }

// PaymentAllocation is the slice of one payment that settled one invoice —
// the bridge row realizing Q8's allocation rule. Deliberately has no
// gorm.DeletedAt: this table has no deleted_at column either.
type PaymentAllocation struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	// CenterID inherits its payment's anchors, never the acting caller's.
	CenterID    uuid.UUID
	PaymentID   uuid.UUID
	InvoiceID   uuid.UUID
	Amount      int64
	AllocatedBy string
	CreatedAt   time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (PaymentAllocation) TableName() string { return "payment_allocations" }
