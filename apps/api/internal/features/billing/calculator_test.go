package billing

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/id"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// assertChecks verifies both DB CHECK identities hold for a computed
// invoice: amount = billable_count * unit_price (docs/schema_design.sql:334)
// on every line, and total_due = opening_balance + current_charge +
// adjustment_total (docs/schema_design.sql:308) on the invoice itself.
func assertChecks(t *testing.T, inv ComputedInvoice) {
	t.Helper()
	var sumAmount int64
	for _, line := range inv.Lines {
		wantAmount := int64(line.BillableCount) * line.UnitPrice
		if line.Amount != wantAmount {
			t.Fatalf("line amount check failed (docs/schema_design.sql:334): got %d, want billable_count(%d)*unit_price(%d)=%d",
				line.Amount, line.BillableCount, line.UnitPrice, wantAmount)
		}
		sumAmount += line.Amount
	}
	if inv.CurrentCharge != sumAmount {
		t.Fatalf("current_charge must equal sum(line.amount): got %d, want %d", inv.CurrentCharge, sumAmount)
	}
	wantTotal := inv.OpeningBalance + inv.CurrentCharge + inv.AdjustmentTotal
	if inv.TotalDue != wantTotal {
		t.Fatalf("total_due check failed (docs/schema_design.sql:308): got %d, want opening_balance(%d)+current_charge(%d)+adjustment_total(%d)=%d",
			inv.TotalDue, inv.OpeningBalance, inv.CurrentCharge, inv.AdjustmentTotal, wantTotal)
	}
}

func tallyFixture(studentID, contactID, enrollmentID, classID uuid.UUID, className string, classStart time.Time, unitPrice int64, billable, absent, present int) AttendanceTally {
	return AttendanceTally{
		EnrollmentID:   enrollmentID,
		StudentID:      studentID,
		ContactID:      contactID,
		StudentName:    "An",
		ContactName:    "Mẹ An",
		ClassID:        classID,
		ClassName:      className,
		ClassStartDate: classStart,
		UnitPrice:      unitPrice,
		BillableCount:  billable,
		AbsentCount:    absent,
		PresentCount:   present,
	}
}

func TestComputeSingleLine(t *testing.T) {
	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	tallies := []AttendanceTally{
		tallyFixture(studentID, contactID, enrollmentID, classID, "Toán 8", date("2026-01-01"), 150_000, 8, 1, 7),
	}

	out := Compute(tallies, nil, nil)
	if len(out) != 1 {
		t.Fatalf("want 1 invoice, got %d", len(out))
	}
	inv := out[0]
	assertChecks(t, inv)
	if len(inv.Lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(inv.Lines))
	}
	if inv.Lines[0].Amount != 1_200_000 {
		t.Fatalf("amount = 8*150000, got %d", inv.Lines[0].Amount)
	}
	if inv.CurrentCharge != 1_200_000 || inv.TotalDue != 1_200_000 {
		t.Fatalf("current_charge/total_due mismatch: %+v", inv)
	}
}

// TestComputeTwoLinesOneStudent is R1's acceptance case: one student enrolled
// in two classes gets one invoice with two lines.
func TestComputeTwoLinesOneStudent(t *testing.T) {
	studentID, contactID := id.New(), id.New()
	mathEnrollment, mathClass := id.New(), id.New()
	englishEnrollment, englishClass := id.New(), id.New()

	// Insert in reverse chronological order to prove sorting actually
	// reorders, rather than happening to preserve insertion order.
	tallies := []AttendanceTally{
		tallyFixture(studentID, contactID, englishEnrollment, englishClass, "Tiếng Anh", date("2026-02-01"), 200_000, 4, 0, 4),
		tallyFixture(studentID, contactID, mathEnrollment, mathClass, "Toán 8", date("2026-01-01"), 150_000, 8, 1, 7),
	}

	out := Compute(tallies, nil, nil)
	if len(out) != 1 {
		t.Fatalf("one student must produce one invoice, got %d", len(out))
	}
	inv := out[0]
	assertChecks(t, inv)
	if len(inv.Lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(inv.Lines))
	}
	if inv.Lines[0].ClassName != "Toán 8" || inv.Lines[1].ClassName != "Tiếng Anh" {
		t.Fatalf("lines must sort by class_start_date, got order %s, %s", inv.Lines[0].ClassName, inv.Lines[1].ClassName)
	}
	wantCharge := int64(8)*150_000 + int64(4)*200_000
	if inv.CurrentCharge != wantCharge {
		t.Fatalf("current_charge = sum of both lines, got %d want %d", inv.CurrentCharge, wantCharge)
	}
	if inv.TotalDue != wantCharge {
		t.Fatalf("total_due with no opening/adjustment must equal current_charge, got %d want %d", inv.TotalDue, wantCharge)
	}
}

func TestComputeZeroBillableCount(t *testing.T) {
	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	tallies := []AttendanceTally{
		tallyFixture(studentID, contactID, enrollmentID, classID, "Toán 8", date("2026-01-01"), 150_000, 0, 0, 0),
	}

	out := Compute(tallies, nil, nil)
	if len(out) != 1 {
		t.Fatalf("want 1 invoice, got %d", len(out))
	}
	inv := out[0]
	assertChecks(t, inv)
	if inv.Lines[0].Amount != 0 {
		t.Fatalf("zero billable_count must produce zero amount, got %d", inv.Lines[0].Amount)
	}
	if inv.CurrentCharge != 0 || inv.TotalDue != 0 {
		t.Fatalf("zero billable_count invoice must be all zero, got %+v", inv)
	}
}

// TestComputeOpeningBalanceOnlyNoLines covers a student who carries debt into
// a period with no attendance at all — e.g. a zero-session class, or a
// student who left before any session in the new period.
func TestComputeOpeningBalanceOnlyNoLines(t *testing.T) {
	studentID := id.New()
	opening := map[uuid.UUID]int64{studentID: 500_000}

	out := Compute(nil, opening, nil)
	if len(out) != 1 {
		t.Fatalf("opening-only student must still get an invoice, got %d", len(out))
	}
	inv := out[0]
	assertChecks(t, inv)
	if len(inv.Lines) != 0 {
		t.Fatalf("opening-only invoice must have no lines, got %d", len(inv.Lines))
	}
	if inv.OpeningBalance != 500_000 || inv.CurrentCharge != 0 || inv.TotalDue != 500_000 {
		t.Fatalf("want opening_balance=500000, current_charge=0, total_due=500000, got %+v", inv)
	}
}

func TestComputeNegativeAdjustment(t *testing.T) {
	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	tallies := []AttendanceTally{
		tallyFixture(studentID, contactID, enrollmentID, classID, "Toán 8", date("2026-01-01"), 200_000, 5, 0, 5),
	}
	adjustments := map[uuid.UUID]int64{studentID: -300_000}

	out := Compute(tallies, nil, adjustments)
	if len(out) != 1 {
		t.Fatalf("want 1 invoice, got %d", len(out))
	}
	inv := out[0]
	assertChecks(t, inv)
	wantCharge := int64(5) * 200_000
	if inv.CurrentCharge != wantCharge {
		t.Fatalf("current_charge unaffected by adjustment, got %d want %d", inv.CurrentCharge, wantCharge)
	}
	if inv.AdjustmentTotal != -300_000 {
		t.Fatalf("adjustment_total must pass through unchanged, got %d", inv.AdjustmentTotal)
	}
	wantTotal := wantCharge - 300_000
	if inv.TotalDue != wantTotal {
		t.Fatalf("negative adjustment must reduce total_due, got %d want %d", inv.TotalDue, wantTotal)
	}
}

// TestComputeLargeValuesDoNotOverflow builds 150 separate students, each
// billed 30,000,000 VND, and sums their total_due manually in int64 — a
// magnitude (4,500,000,000) that exceeds math.MaxInt32, proving the
// calculator's arithmetic never narrows to a 32-bit type.
func TestComputeLargeValuesDoNotOverflow(t *testing.T) {
	const studentCount = 150
	const unitPrice = int64(30_000_000)

	tallies := make([]AttendanceTally, 0, studentCount)
	for i := 0; i < studentCount; i++ {
		studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
		tallies = append(tallies, tallyFixture(studentID, contactID, enrollmentID, classID, "Toán 8", date("2026-01-01"), unitPrice, 1, 0, 1))
	}

	out := Compute(tallies, nil, nil)
	if len(out) != studentCount {
		t.Fatalf("want %d invoices, got %d", studentCount, len(out))
	}

	var total int64
	for _, inv := range out {
		assertChecks(t, inv)
		if inv.CurrentCharge != unitPrice {
			t.Fatalf("each invoice must charge exactly unit_price, got %d", inv.CurrentCharge)
		}
		total += inv.TotalDue
	}
	wantTotal := int64(studentCount) * unitPrice
	if wantTotal <= 1<<31-1 {
		t.Fatalf("test setup error: expected magnitude to exceed math.MaxInt32, got %d", wantTotal)
	}
	if total != wantTotal {
		t.Fatalf("aggregate total_due must not overflow: got %d want %d", total, wantTotal)
	}
}
