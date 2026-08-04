// Package collections is the read-only reporting side of PRD R7's collection
// board: two views over one billing period's money — by contact (who to call)
// and by class (who in this room still owes). It owns no table of its own; it
// only projects invoices, invoice_lines, payments, and the v_contact_balance
// view onto the two shapes a teacher chasing money actually looks at. No
// writes, no transaction manager.
package collections

import "fmt"

// View query parameter values for GET /billing-periods/:id/collections.
// Contact is the default: one row per parent with every child's debt merged,
// which is what a teacher chasing money wants first. Class exists for the
// opposite moment — standing in front of one room, checking who is still
// short.
const (
	ViewContact = "contact"
	ViewClass   = "class"
)

// Derived payment_status values, computed the same way for both views: an
// outstanding balance at or below zero is paid regardless of how it got
// there; zero paid at all is unpaid; anything else is partial. A contact row
// with one paid child and one unpaid child reads partial — the honest
// family-level answer, matching how a teacher will actually describe it.
const (
	StatusUnpaid  = "unpaid"
	StatusPartial = "partial"
	StatusPaid    = "paid"
)

// validStatuses whitelists the status query parameter.
var validStatuses = map[string]bool{
	StatusUnpaid:  true,
	StatusPartial: true,
	StatusPaid:    true,
}

// statusVoid mirrors invoices.status='void' (docs/schema_design.sql:297).
// Declared locally rather than importing billing's InvoiceVoid constant —
// this package reads the invoices table directly, the same
// share-the-table-not-the-Go-type precedent payments already uses for the
// same table.
const statusVoid = "void"

// derivePaymentStatus computes one row's payment_status from its due/paid
// pair in Go — used after a query has already scanned the numbers, so the
// display value and the SQL filter (paymentStatusExpr) apply the identical
// rule without either duplicating the other's implementation.
func derivePaymentStatus(due, paid int64) string {
	switch {
	case due-paid <= 0:
		return StatusPaid
	case paid == 0:
		return StatusUnpaid
	default:
		return StatusPartial
	}
}

// paymentStatusExpr renders derivePaymentStatus's rule as a SQL CASE
// expression over the given due/paid column (or alias) expressions, for use
// in a WHERE clause filtering on the status query parameter. dueExpr and
// paidExpr must already be safe, non-user-controlled SQL fragments (a column
// or table-qualified column name), never request input.
func paymentStatusExpr(dueExpr, paidExpr string) string {
	return fmt.Sprintf(
		"CASE WHEN (%s - %s) <= 0 THEN '%s' WHEN %s = 0 THEN '%s' ELSE '%s' END",
		dueExpr, paidExpr, StatusPaid, paidExpr, StatusUnpaid, StatusPartial,
	)
}
