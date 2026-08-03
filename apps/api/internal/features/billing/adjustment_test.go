package billing

import "testing"

// TestResolveDeltaFirstEditIsFullDiff: a session's first ever post-close edit
// has no prior carry (already_adj=0), so the whole gap between what live
// attendance now says and what the invoice actually billed must post in one
// shot.
func TestResolveDeltaFirstEditIsFullDiff(t *testing.T) {
	delta, shouldPost := resolveDelta(true, 300_000, 200_000, 0)
	if !shouldPost {
		t.Fatalf("a 100_000 gap must post")
	}
	if delta != 100_000 {
		t.Fatalf("delta = %d, want 100_000", delta)
	}
}

// TestResolveDeltaSecondEditIsIncrementalOnly: a second edit on top of an
// already-carried first one must only post the additional movement, not the
// first edit's amount again — already_adj is what prevents the double count
// (the single most likely money bug in this feature).
func TestResolveDeltaSecondEditIsIncrementalOnly(t *testing.T) {
	// First edit: live 220_000 vs billed 200_000, already_adj 0 -> +20_000.
	firstDelta, _ := resolveDelta(true, 220_000, 200_000, 0)
	if firstDelta != 20_000 {
		t.Fatalf("first delta = %d, want 20_000", firstDelta)
	}
	// Second edit: live moves further to 250_000; the invoice's own
	// current_charge never changes (it is frozen at close), and already_adj
	// now reflects the first carry.
	secondDelta, shouldPost := resolveDelta(true, 250_000, 200_000, firstDelta)
	if !shouldPost {
		t.Fatalf("a further 30_000 movement must post")
	}
	if secondDelta != 30_000 {
		t.Fatalf("second delta = %d, want 30_000 (incremental only), not %d", 30_000, secondDelta)
	}
}

// TestResolveDeltaRevertingEditSumsToZero: editing attendance and then
// reverting it back to the original state must leave the two posted
// adjustments summing to zero net effect.
func TestResolveDeltaRevertingEditSumsToZero(t *testing.T) {
	const billed = 200_000
	edit, _ := resolveDelta(true, 260_000, billed, 0)
	revert, shouldPost := resolveDelta(true, billed, billed, edit)
	if !shouldPost {
		t.Fatalf("reverting a 60_000 edit must still post the opposite -60_000")
	}
	if sum := edit + revert; sum != 0 {
		t.Fatalf("edit (%d) + revert (%d) = %d, want 0", edit, revert, sum)
	}
}

// TestResolveDeltaUnchangedSessionIsZero: an edit that never moved a
// billable count (e.g. flipping present<->present, or any change that does
// not affect the billable tally) must not post anything.
func TestResolveDeltaUnchangedSessionIsZero(t *testing.T) {
	delta, shouldPost := resolveDelta(true, 200_000, 200_000, 0)
	if shouldPost {
		t.Fatalf("an unchanged live charge must not post, got delta=%d", delta)
	}
	if delta != 0 {
		t.Fatalf("delta = %d, want 0", delta)
	}
}

// TestResolveDeltaSkipsStudentWithNoInvoice: a student with no non-void
// invoice in the closed period (Architecture: skip) must never post,
// regardless of what the other arguments say.
func TestResolveDeltaSkipsStudentWithNoInvoice(t *testing.T) {
	delta, shouldPost := resolveDelta(false, 999_999, 0, 0)
	if shouldPost {
		t.Fatalf("a student with no invoice must be skipped, got delta=%d", delta)
	}
	if delta != 0 {
		t.Fatalf("delta = %d, want 0", delta)
	}
}

func TestDeriveInvoiceStatusNeverTouchesDraftOrVoid(t *testing.T) {
	if got := deriveInvoiceStatus(InvoiceDraft, 0, 100); got != InvoiceDraft {
		t.Fatalf("draft must stay draft, got %s", got)
	}
	if got := deriveInvoiceStatus(InvoiceVoid, 500, 100); got != InvoiceVoid {
		t.Fatalf("void must stay void even when paid_amount exceeds total_due, got %s", got)
	}
}

func TestDeriveInvoiceStatusTransitionsAmongIssuedPartiallyPaidPaid(t *testing.T) {
	cases := []struct {
		name       string
		current    string
		paidAmount int64
		totalDue   int64
		want       string
	}{
		{"nothing paid stays issued", InvoiceIssued, 0, 100_000, InvoiceIssued},
		{"partial payment", InvoiceIssued, 40_000, 100_000, InvoicePartiallyPaid},
		{"paid in full", InvoicePartiallyPaid, 100_000, 100_000, InvoicePaid},
		{"overpaid still reads as paid", InvoicePaid, 120_000, 100_000, InvoicePaid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveInvoiceStatus(tc.current, tc.paidAmount, tc.totalDue)
			if got != tc.want {
				t.Fatalf("deriveInvoiceStatus(%s, %d, %d) = %s, want %s", tc.current, tc.paidAmount, tc.totalDue, got, tc.want)
			}
		})
	}
}

func TestNextCalendarMonthRollsYearOverAtDecember(t *testing.T) {
	year, month := nextCalendarMonth(2026, 12)
	if year != 2027 || month != 1 {
		t.Fatalf("nextCalendarMonth(2026, 12) = (%d, %d), want (2027, 1)", year, month)
	}
}

func TestNextCalendarMonthWithinYear(t *testing.T) {
	year, month := nextCalendarMonth(2026, 3)
	if year != 2026 || month != 4 {
		t.Fatalf("nextCalendarMonth(2026, 3) = (%d, %d), want (2026, 4)", year, month)
	}
}
