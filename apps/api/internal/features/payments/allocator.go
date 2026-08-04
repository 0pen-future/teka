package payments

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// Candidate is one invoice eligible to receive a slice of a payment — the
// Repository.CandidateInvoices projection Allocate consumes. It carries no
// behaviour of its own so the allocator stays a pure function over plain
// data.
type Candidate struct {
	InvoiceID uuid.UUID
	// PeriodStart is the invoice's billing period start date — the primary
	// D8 ordering key (older periods are older debt).
	PeriodStart time.Time
	// EarliestClassStart is the earliest class start date among the
	// invoice's lines, or nil when the invoice has no lines (pure
	// carry-over debt). nil sorts last within a period.
	EarliestClassStart *time.Time
	OpeningBalance     int64
	TotalDue           int64
	PaidAmount         int64
}

// Allocation is one invoice's slice of a payment.
type Allocation struct {
	InvoiceID uuid.UUID
	Amount    int64
}

// Allocate distributes amount across candidates per D8: the carried
// opening-balance portion of every candidate before any current charge, ties
// broken by earlier class start date then invoice id. It returns the
// allocations plus whatever could not be placed. Pure: no id generation, no
// clock reads, no errors — callers own persistence and validation.
//
// Ordering:
//  1. PeriodStart ascending — older periods are older debt.
//  2. EarliestClassStart ascending, nil last.
//  3. InvoiceID ascending — reproducible output.
//
// Two passes over that same order:
//
//	outstanding(c)    = c.TotalDue - c.PaidAmount                      (>= 0)
//	opening_unpaid(c) = max(0, min(c.OpeningBalance - c.PaidAmount, outstanding(c)))
//	rest_unpaid(c)    = outstanding(c) - opening_unpaid(c)
//
//	pass 1: for c in order: take = min(remaining, opening_unpaid(c))
//	pass 2: for c in order: take = min(remaining, rest_unpaid(c))
//
// Existing paid_amount is treated as having settled the opening balance
// first, which is what makes opening_unpaid shrink correctly across repeated
// part-payments and keeps a negative adjustment_total from ever producing a
// negative rest_unpaid. Both passes accumulate into a per-invoice map before
// any Allocation is built, so an invoice touched by both passes yields one
// row — required by uq_payment_allocations (docs/schema_design.sql:397).
func Allocate(amount int64, candidates []Candidate) (allocs []Allocation, unallocated int64) {
	ordered := make([]Candidate, len(candidates))
	copy(ordered, candidates)
	sort.SliceStable(ordered, func(i, j int) bool {
		return less(ordered[i], ordered[j])
	})

	openingUnpaid := make([]int64, len(ordered))
	restUnpaid := make([]int64, len(ordered))
	for i, c := range ordered {
		outstanding := c.TotalDue - c.PaidAmount
		if outstanding < 0 {
			outstanding = 0
		}
		opening := c.OpeningBalance - c.PaidAmount
		if opening < 0 {
			opening = 0
		}
		if opening > outstanding {
			opening = outstanding
		}
		openingUnpaid[i] = opening
		restUnpaid[i] = outstanding - opening
	}

	taken := make([]int64, len(ordered))
	remaining := amount

	for i := range ordered {
		if remaining <= 0 {
			break
		}
		take := min(remaining, openingUnpaid[i])
		taken[i] += take
		remaining -= take
	}
	for i := range ordered {
		if remaining <= 0 {
			break
		}
		take := min(remaining, restUnpaid[i])
		taken[i] += take
		remaining -= take
	}

	allocs = make([]Allocation, 0, len(ordered))
	for i, c := range ordered {
		if taken[i] > 0 {
			allocs = append(allocs, Allocation{InvoiceID: c.InvoiceID, Amount: taken[i]})
		}
	}
	return allocs, remaining
}

// less implements the D8 comparator: PeriodStart asc, EarliestClassStart asc
// (nil last), InvoiceID asc.
func less(a, b Candidate) bool {
	if !a.PeriodStart.Equal(b.PeriodStart) {
		return a.PeriodStart.Before(b.PeriodStart)
	}
	switch {
	case a.EarliestClassStart == nil && b.EarliestClassStart == nil:
		// fall through to the id tie-break below.
	case a.EarliestClassStart == nil:
		return false
	case b.EarliestClassStart == nil:
		return true
	case !a.EarliestClassStart.Equal(*b.EarliestClassStart):
		return a.EarliestClassStart.Before(*b.EarliestClassStart)
	}
	return a.InvoiceID.String() < b.InvoiceID.String()
}
