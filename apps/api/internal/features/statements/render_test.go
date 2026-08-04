package statements

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(dateLayout, s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return tm
}

func ptr[T any](v T) *T { return &v }

func TestBuildPublicStatementTwoChildrenTwoClassesEach(t *testing.T) {
	studentA, studentB := uuid.New(), uuid.New()
	invoiceA, invoiceB := uuid.New(), uuid.New()
	enrollA1, enrollA2, enrollB1, enrollB2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	invoiceRows := []InvoiceLineRow{
		{
			InvoiceID: invoiceA, StudentID: studentA, StudentName: "Con A", ContactName: "Chị Hoa",
			OpeningBalance: 0, CurrentCharge: 200_000, AdjustmentTotal: 0, TotalDue: 200_000, PaidAmount: 100_000,
			PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enrollA1, ClassName: ptr("Toán"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(100_000)), LineAmount: ptr(int64(100_000)),
		},
		{
			InvoiceID: invoiceA, StudentID: studentA, StudentName: "Con A", ContactName: "Chị Hoa",
			OpeningBalance: 0, CurrentCharge: 200_000, AdjustmentTotal: 0, TotalDue: 200_000, PaidAmount: 100_000,
			PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enrollA2, ClassName: ptr("Văn"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(100_000)), LineAmount: ptr(int64(100_000)),
		},
		{
			InvoiceID: invoiceB, StudentID: studentB, StudentName: "Con B", ContactName: "Chị Hoa",
			OpeningBalance: 0, CurrentCharge: 300_000, AdjustmentTotal: 0, TotalDue: 300_000, PaidAmount: 0,
			PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enrollB1, ClassName: ptr("Anh"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(150_000)), LineAmount: ptr(int64(150_000)),
		},
		{
			InvoiceID: invoiceB, StudentID: studentB, StudentName: "Con B", ContactName: "Chị Hoa",
			OpeningBalance: 0, CurrentCharge: 300_000, AdjustmentTotal: 0, TotalDue: 300_000, PaidAmount: 0,
			PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enrollB2, ClassName: ptr("Lý"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(150_000)), LineAmount: ptr(int64(150_000)),
		},
	}

	got := buildPublicStatement(invoiceRows, nil, nil)

	if got.ContactName != "Chị Hoa" {
		t.Fatalf("contact_name = %q, want %q", got.ContactName, "Chị Hoa")
	}
	if got.Period != "08/2026" {
		t.Fatalf("period = %q, want %q", got.Period, "08/2026")
	}
	if len(got.Children) != 2 {
		t.Fatalf("want 2 children, got %d", len(got.Children))
	}
	if len(got.Children[0].Classes) != 2 || len(got.Children[1].Classes) != 2 {
		t.Fatalf("each child must have 2 classes, got %d and %d", len(got.Children[0].Classes), len(got.Children[1].Classes))
	}
	if got.Totals.TotalDue != 500_000 {
		t.Fatalf("totals.total_due = %d, want 500000", got.Totals.TotalDue)
	}
	if got.Totals.Paid != 100_000 || got.Totals.Outstanding != 400_000 {
		t.Fatalf("paid/outstanding = %d/%d, want 100000/400000", got.Totals.Paid, got.Totals.Outstanding)
	}
	if got.Payments.TotalPaid != 100_000 || len(got.Payments.ByInvoice) != 2 {
		t.Fatalf("payments = %+v, want total_paid 100000 and 2 by_invoice entries", got.Payments)
	}
}

func TestBuildPublicStatementOneChildTwoClasses(t *testing.T) {
	student := uuid.New()
	invoice := uuid.New()
	enroll1, enroll2 := uuid.New(), uuid.New()

	invoiceRows := []InvoiceLineRow{
		{
			InvoiceID: invoice, StudentID: student, StudentName: "Con C", ContactName: "Anh Tuấn",
			TotalDue: 250_000, PaidAmount: 250_000, PeriodYear: 2026, PeriodMonth: 7,
			LineID: ptr(uuid.New()), EnrollmentID: &enroll1, ClassName: ptr("Toán"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(100_000)), LineAmount: ptr(int64(100_000)),
		},
		{
			InvoiceID: invoice, StudentID: student, StudentName: "Con C", ContactName: "Anh Tuấn",
			TotalDue: 250_000, PaidAmount: 250_000, PeriodYear: 2026, PeriodMonth: 7,
			LineID: ptr(uuid.New()), EnrollmentID: &enroll2, ClassName: ptr("Anh"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(150_000)), LineAmount: ptr(int64(150_000)),
		},
	}

	got := buildPublicStatement(invoiceRows, nil, nil)
	if len(got.Children) != 1 {
		t.Fatalf("want 1 child, got %d", len(got.Children))
	}
	if len(got.Children[0].Classes) != 2 {
		t.Fatalf("want 2 classes for the one child, got %d", len(got.Children[0].Classes))
	}
	if got.Totals.Outstanding != 0 {
		t.Fatalf("outstanding = %d, want 0 (fully paid)", got.Totals.Outstanding)
	}
}

func TestBuildPublicStatementAbsentSessionIsNotCounted(t *testing.T) {
	student := uuid.New()
	invoice := uuid.New()
	enroll := uuid.New()

	invoiceRows := []InvoiceLineRow{
		{
			InvoiceID: invoice, StudentID: student, StudentName: "Con D", ContactName: "Chị Lan",
			TotalDue: 150_000, PaidAmount: 0, PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enroll, ClassName: ptr("Toán"),
			BillableCount: ptr(1), AbsentCount: ptr(1), UnitPrice: ptr(int64(150_000)), LineAmount: ptr(int64(150_000)),
		},
	}
	sessionRows := []LiveSessionRow{
		{EnrollmentID: enroll, SessionDate: mustDate(t, "2026-08-03"), AttendanceStatus: "present", Billable: true},
		{EnrollmentID: enroll, SessionDate: mustDate(t, "2026-08-10"), AttendanceStatus: "absent", Billable: false},
	}

	got := buildPublicStatement(invoiceRows, sessionRows, nil)
	sessions := got.Children[0].Classes[0].Sessions
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	if sessions[1].Status != "absent" || sessions[1].Counted {
		t.Fatalf("second session = %+v, want status=absent counted=false", sessions[1])
	}
	if sessions[0].Status != "present" || !sessions[0].Counted {
		t.Fatalf("first session = %+v, want status=present counted=true", sessions[0])
	}
}

func TestBuildPublicStatementCarriedAdjustmentPresent(t *testing.T) {
	student := uuid.New()
	invoice := uuid.New()
	enroll := uuid.New()
	sourceSession := uuid.New()

	invoiceRows := []InvoiceLineRow{
		{
			InvoiceID: invoice, StudentID: student, StudentName: "Con E", ContactName: "Chị Mai",
			TotalDue: 100_000, PaidAmount: 0, PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enroll, ClassName: ptr("Toán"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(100_000)), LineAmount: ptr(int64(100_000)),
		},
	}
	adjustmentRows := []AdjustmentRow{
		{
			StudentID: student, Amount: 50_000, SourceSessionID: &sourceSession,
			Carried: true, SessionDate: ptr(mustDate(t, "2026-07-28")),
		},
	}

	got := buildPublicStatement(invoiceRows, nil, adjustmentRows)
	child := got.Children[0]
	if child.CarriedAdjustment == nil {
		t.Fatal("want a non-nil carried_adjustment")
	}
	if child.CarriedAdjustment.Amount != 50_000 {
		t.Fatalf("carried_adjustment.amount = %d, want 50000", child.CarriedAdjustment.Amount)
	}
	if len(child.CarriedAdjustment.SessionDates) != 1 || child.CarriedAdjustment.SessionDates[0] != "2026-07-28" {
		t.Fatalf("carried_adjustment.session_dates = %v, want [2026-07-28]", child.CarriedAdjustment.SessionDates)
	}
	if len(child.Adjustments) != 0 {
		t.Fatalf("a carried adjustment must not also appear in the direct adjustments list, got %+v", child.Adjustments)
	}
}

func TestBuildPublicStatementDirectAdjustmentKindDerivedFromSourceSession(t *testing.T) {
	student := uuid.New()
	invoice := uuid.New()
	enroll := uuid.New()
	sourceSession := uuid.New()

	invoiceRows := []InvoiceLineRow{
		{
			InvoiceID: invoice, StudentID: student, StudentName: "Con F", ContactName: "Chị Nga",
			TotalDue: 120_000, PaidAmount: 0, PeriodYear: 2026, PeriodMonth: 8,
			LineID: ptr(uuid.New()), EnrollmentID: &enroll, ClassName: ptr("Toán"),
			BillableCount: ptr(1), AbsentCount: ptr(0), UnitPrice: ptr(int64(100_000)), LineAmount: ptr(int64(100_000)),
		},
	}
	adjustmentRows := []AdjustmentRow{
		{StudentID: student, Amount: 10_000, SourceSessionID: &sourceSession, Carried: false},
		{StudentID: student, Amount: 10_000, SourceSessionID: nil, Carried: false},
	}

	got := buildPublicStatement(invoiceRows, nil, adjustmentRows)
	adjustments := got.Children[0].Adjustments
	if len(adjustments) != 2 {
		t.Fatalf("want 2 direct adjustments, got %d", len(adjustments))
	}
	if adjustments[0].Kind != adjustmentKindCorrection {
		t.Fatalf("adjustment with a source session must be kind=correction, got %q", adjustments[0].Kind)
	}
	if adjustments[1].Kind != adjustmentKindManual {
		t.Fatalf("adjustment without a source session must be kind=manual, got %q", adjustments[1].Kind)
	}
}

// fakeQRBuilder lets attachQR's tests control Payload's ok result without a
// real EMVCo payload.
type fakeQRBuilder struct {
	ok      bool
	payload string
}

func (f fakeQRBuilder) Payload(_ BankConfig, _ int64, _ string) (string, bool) {
	return f.payload, f.ok
}
func (f fakeQRBuilder) Render(_ string) ([]byte, error) { return []byte("fake-png"), nil }

func TestAttachQRAbsentBankConfigOmitsQRBlock(t *testing.T) {
	payload := PublicStatement{ContactName: "Chị Hoa", Period: "08/2026", Totals: PublicTotals{Outstanding: 200_000}}
	raw := attachQR(&payload, fakeQRBuilder{ok: false}, BankConfig{}, "https://parent.example.com", "tok123")

	if raw != "" {
		t.Fatalf("raw payload = %q, want empty when the builder reports ok=false", raw)
	}
	if payload.QR != nil {
		t.Fatalf("QR = %+v, want nil when the bank config is absent", payload.QR)
	}
}

func TestAttachQRPresentBankConfigFillsQRBlock(t *testing.T) {
	payload := PublicStatement{ContactName: "Chị Hoa", Period: "08/2026", Totals: PublicTotals{Outstanding: 200_000}}
	raw := attachQR(&payload, fakeQRBuilder{ok: true, payload: "emv-payload"}, BankConfig{BankCode: "X"}, "https://parent.example.com", "tok123")

	if raw != "emv-payload" {
		t.Fatalf("raw payload = %q, want %q", raw, "emv-payload")
	}
	if payload.QR == nil {
		t.Fatal("QR = nil, want a filled block when the builder reports ok=true")
	}
	wantURL := "https://parent.example.com/public/statements/tok123/qr.png"
	if payload.QR.ImageURL != wantURL {
		t.Fatalf("QR.ImageURL = %q, want %q", payload.QR.ImageURL, wantURL)
	}
	if payload.QR.Amount != 200_000 {
		t.Fatalf("QR.Amount = %d, want 200000", payload.QR.Amount)
	}
	if payload.QR.Note != "HP Chị Hoa 08/2026" {
		t.Fatalf("QR.Note = %q, want %q", payload.QR.Note, "HP Chị Hoa 08/2026")
	}
}
