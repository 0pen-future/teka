package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// previewFakeRepository extends the base fakeRepository (service_test.go)
// with in-memory storage that actually reproduces upsert-on-natural-key
// semantics for invoices/invoice_lines, so Draft's idempotency and conflict
// guards are meaningfully exercised without a real database.
type previewFakeRepository struct {
	*fakeRepository
	tallies         map[uuid.UUID][]AttendanceTally    // by period id
	carriedDebt     map[uuid.UUID][]CarriedDebtStudent // by prev period id
	openingBalances map[uuid.UUID]int64                // by student id
	invoices        map[uuid.UUID]*Invoice             // by invoice id
	lines           map[uuid.UUID]*InvoiceLine         // by line id
	adjustments     map[uuid.UUID]int64                // by invoice id, test-seeded directly
}

func newPreviewFakeRepository() *previewFakeRepository {
	return &previewFakeRepository{
		fakeRepository:  newFakeRepository(),
		tallies:         map[uuid.UUID][]AttendanceTally{},
		carriedDebt:     map[uuid.UUID][]CarriedDebtStudent{},
		openingBalances: map[uuid.UUID]int64{},
		invoices:        map[uuid.UUID]*Invoice{},
		lines:           map[uuid.UUID]*InvoiceLine{},
		adjustments:     map[uuid.UUID]int64{},
	}
}

func (f *previewFakeRepository) TallyAttendance(_ context.Context, _ authctx.Scope, periodID uuid.UUID) ([]AttendanceTally, error) {
	return f.tallies[periodID], nil
}

func (f *previewFakeRepository) OpeningBalances(_ context.Context, _ authctx.Scope, _ uuid.UUID, studentIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, sid := range studentIDs {
		if v, ok := f.openingBalances[sid]; ok {
			out[sid] = v
		}
	}
	return out, nil
}

func (f *previewFakeRepository) AdjustmentTotals(_ context.Context, _ authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, inv := range f.invoices {
		if inv.PeriodID != periodID {
			continue
		}
		if total, ok := f.adjustments[inv.ID]; ok {
			out[inv.ID] = total
		}
	}
	return out, nil
}

func (f *previewFakeRepository) CarriedDebtStudents(_ context.Context, _ authctx.Scope, prevPeriodID uuid.UUID) ([]CarriedDebtStudent, error) {
	return f.carriedDebt[prevPeriodID], nil
}

func (f *previewFakeRepository) UpsertInvoice(_ context.Context, inv *Invoice) error {
	for _, existing := range f.invoices {
		if existing.PeriodID != inv.PeriodID || existing.StudentID != inv.StudentID {
			continue
		}
		if existing.Status != InvoiceDraft {
			return ErrInvoiceNotDraft
		}
		inv.ID = existing.ID
		inv.Status = existing.Status
		inv.CreatedAt = existing.CreatedAt
		cp := *inv
		f.invoices[inv.ID] = &cp
		return nil
	}
	cp := *inv
	f.invoices[inv.ID] = &cp
	return nil
}

func (f *previewFakeRepository) UpsertInvoiceLine(_ context.Context, line *InvoiceLine) error {
	for _, existing := range f.lines {
		if existing.InvoiceID != line.InvoiceID || existing.EnrollmentID != line.EnrollmentID {
			continue
		}
		line.ID = existing.ID
		cp := *line
		f.lines[line.ID] = &cp
		return nil
	}
	cp := *line
	f.lines[line.ID] = &cp
	return nil
}

func (f *previewFakeRepository) ZeroUnmatchedLines(_ context.Context, _ authctx.Scope, invoiceID uuid.UUID, keepEnrollmentIDs []uuid.UUID) error {
	keep := make(map[uuid.UUID]bool, len(keepEnrollmentIDs))
	for _, eid := range keepEnrollmentIDs {
		keep[eid] = true
	}
	for _, line := range f.lines {
		if line.InvoiceID != invoiceID || keep[line.EnrollmentID] {
			continue
		}
		line.BillableCount, line.AbsentCount, line.Amount = 0, 0, 0
	}
	return nil
}

func (f *previewFakeRepository) ListInvoices(_ context.Context, _ authctx.Scope, periodID uuid.UUID) ([]Invoice, error) {
	var out []Invoice
	for _, inv := range f.invoices {
		if inv.PeriodID == periodID {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (f *previewFakeRepository) GetInvoiceWithLines(_ context.Context, _ authctx.Scope, invoiceID uuid.UUID) (*Invoice, []InvoiceLine, error) {
	inv, ok := f.invoices[invoiceID]
	if !ok {
		return nil, nil, ErrInvoiceNotFound
	}
	var lines []InvoiceLine
	for _, l := range f.lines {
		if l.InvoiceID == invoiceID {
			lines = append(lines, *l)
		}
	}
	cp := *inv
	return &cp, lines, nil
}

func newPreviewTestService() (*Service, *previewFakeRepository) {
	repo := newPreviewFakeRepository()
	return NewService(repo, noopTx{}, &fakePendingSource{}, nil), repo
}

// openPeriod inserts a period directly into the fake's map, bypassing
// EnsurePeriod so tests control period_start/period_end exactly.
func openPeriod(repo *previewFakeRepository, teacherID uuid.UUID, start, end time.Time, status string) Period {
	p := Period{
		ID: id.New(), TeacherID: teacherID, CenterID: id.New(), Year: int16(start.Year()), Month: int16(start.Month()), //nolint:gosec // test fixture, bounded input
		PeriodStart: start, PeriodEnd: end, Status: status,
	}
	repo.periods[p.ID] = p
	return p
}

func TestPreviewIncludesTallyStudentAndCarriedDebtOnlyStudent(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	teacherID := id.New()

	prev := openPeriod(repo, teacherID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), PeriodClosed)
	period := openPeriod(repo, teacherID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: period.CenterID}

	enrolledStudent := id.New()
	enrolledContact := id.New()
	enrollmentID := id.New()
	classID := id.New()
	repo.tallies[period.ID] = []AttendanceTally{{
		EnrollmentID: enrollmentID, StudentID: enrolledStudent, ContactID: enrolledContact,
		StudentName: "Nguyen An", ContactName: "Mother of An",
		ClassID: classID, ClassName: "Toán 5", ClassStartDate: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		UnitPrice: 100_000, BillableCount: 4, AbsentCount: 1, PresentCount: 3,
	}}

	// A student who quit before this period started, but still owes money.
	departedStudent := id.New()
	departedContact := id.New()
	repo.carriedDebt[prev.ID] = []CarriedDebtStudent{{
		StudentID: departedStudent, ContactID: departedContact,
		StudentName: "Tran Binh", ContactName: "Father of Binh", Outstanding: 250_000,
	}}
	repo.openingBalances[departedStudent] = 250_000

	resp, err := svc.Preview(ctx, sc, period.ID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(resp.Invoices) != 2 {
		t.Fatalf("want 2 invoices (enrolled + departed debtor), got %d", len(resp.Invoices))
	}
	if resp.Totals.StudentCount != 2 {
		t.Fatalf("totals.student_count = %d, want 2", resp.Totals.StudentCount)
	}

	// Sorted by student_name: "Nguyen An" < "Tran Binh".
	enrolled := resp.Invoices[0]
	if enrolled.StudentID != enrolledStudent {
		t.Fatalf("first invoice must be the enrolled student, got %s", enrolled.StudentID)
	}
	if len(enrolled.Lines) != 1 {
		t.Fatalf("enrolled student must have exactly one line, got %d", len(enrolled.Lines))
	}
	line := enrolled.Lines[0]
	if line.ClassID != classID {
		t.Fatalf("line.class_id = %s, want %s (must come from the tally, not invoice_lines)", line.ClassID, classID)
	}
	if line.PresentCount != 3 {
		t.Fatalf("line.present_count = %d, want 3 (must come from the tally, not invoice_lines)", line.PresentCount)
	}
	if enrolled.OpeningBalance != 0 {
		t.Fatalf("enrolled student has no prior debt, got opening_balance=%d", enrolled.OpeningBalance)
	}

	departed := resp.Invoices[1]
	if departed.StudentID != departedStudent {
		t.Fatalf("second invoice must be the departed debtor, got %s", departed.StudentID)
	}
	if len(departed.Lines) != 0 {
		t.Fatalf("a fully-departed debtor must have no lines, got %d", len(departed.Lines))
	}
	if departed.OpeningBalance != 250_000 {
		t.Fatalf("departed.opening_balance = %d, want 250000", departed.OpeningBalance)
	}
	if departed.CurrentCharge != 0 {
		t.Fatalf("departed.current_charge = %d, want 0", departed.CurrentCharge)
	}
	if departed.TotalDue != 250_000 {
		t.Fatalf("departed.total_due = %d, want 250000", departed.TotalDue)
	}
	if departed.ContactID != departedContact {
		t.Fatalf("departed.contact_id = %s, want the carried-debt contact %s, not zero", departed.ContactID, departedContact)
	}
	if departed.InvoiceID != nil {
		t.Fatalf("preview must never populate invoice_id, got %v", departed.InvoiceID)
	}
}

func TestPreviewOmitsZeroedLineFromResponse(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	teacherID := id.New()
	period := openPeriod(repo, teacherID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: period.CenterID}

	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	repo.tallies[period.ID] = []AttendanceTally{{
		EnrollmentID: enrollmentID, StudentID: studentID, ContactID: contactID,
		StudentName: "Le Chi", ContactName: "Mother of Chi",
		ClassID: classID, ClassName: "Văn 5", UnitPrice: 100_000,
		BillableCount: 0, AbsentCount: 0, PresentCount: 0,
	}}

	resp, err := svc.Preview(ctx, sc, period.ID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(resp.Invoices) != 1 {
		t.Fatalf("want 1 invoice, got %d", len(resp.Invoices))
	}
	if len(resp.Invoices[0].Lines) != 0 {
		t.Fatalf("a line with billable_count=0 and absent_count=0 must be omitted, got %+v", resp.Invoices[0].Lines)
	}
}

func TestPreviewCrossTenantIsNotFound(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	owner, stranger := id.New(), id.New()
	period := openPeriod(repo, owner, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)

	_, err := svc.Preview(ctx, authctx.Scope{TeacherID: stranger, CenterID: period.CenterID}, period.ID)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("another teacher's period must read as 404, got %v", err)
	}
}

func TestDraftRefusesClosedPeriod(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	teacherID := id.New()
	period := openPeriod(repo, teacherID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodClosed)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: period.CenterID}

	_, err := svc.Draft(ctx, sc, period.ID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("draft on a closed period must be 409, got %v", err)
	}
	if len(repo.invoices) != 0 {
		t.Fatalf("a refused draft must write nothing, got %d invoices", len(repo.invoices))
	}
}

func TestDraftIsIdempotentAcrossCalls(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	teacherID := id.New()
	period := openPeriod(repo, teacherID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: period.CenterID}

	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	repo.tallies[period.ID] = []AttendanceTally{{
		EnrollmentID: enrollmentID, StudentID: studentID, ContactID: contactID,
		StudentName: "Pham Dung", ContactName: "Mother of Dung",
		ClassID: classID, ClassName: "Anh 5", UnitPrice: 120_000,
		BillableCount: 3, AbsentCount: 0, PresentCount: 3,
	}}

	first, err := svc.Draft(ctx, sc, period.ID)
	if err != nil {
		t.Fatalf("first draft: %v", err)
	}
	second, err := svc.Draft(ctx, sc, period.ID)
	if err != nil {
		t.Fatalf("second draft: %v", err)
	}

	if len(repo.invoices) != 1 {
		t.Fatalf("a second draft must not create a second invoice, got %d", len(repo.invoices))
	}
	if len(repo.lines) != 1 {
		t.Fatalf("a second draft must not create a second line, got %d", len(repo.lines))
	}
	if first.Invoices[0].InvoiceID == nil || second.Invoices[0].InvoiceID == nil {
		t.Fatalf("draft must populate invoice_id")
	}
	if *first.Invoices[0].InvoiceID != *second.Invoices[0].InvoiceID {
		t.Fatalf("re-drafting must keep the same invoice id, got %s then %s",
			*first.Invoices[0].InvoiceID, *second.Invoices[0].InvoiceID)
	}
	if first.Invoices[0].TotalDue != 360_000 || second.Invoices[0].TotalDue != 360_000 {
		t.Fatalf("total_due must be 3*120000=360000, got %d then %d",
			first.Invoices[0].TotalDue, second.Invoices[0].TotalDue)
	}

	invoice, ok := repo.invoices[*first.Invoices[0].InvoiceID]
	if !ok {
		t.Fatalf("drafted invoice must be persisted")
	}
	if invoice.CenterID != sc.CenterID {
		t.Fatalf("drafted invoice must inherit the period's own center_id, got %s want %s", invoice.CenterID, sc.CenterID)
	}
}

func TestDraftRefusesWhenInvoiceAlreadyIssued(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	teacherID := id.New()
	period := openPeriod(repo, teacherID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: period.CenterID}

	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	repo.tallies[period.ID] = []AttendanceTally{{
		EnrollmentID: enrollmentID, StudentID: studentID, ContactID: contactID,
		StudentName: "Vo Em", ContactName: "Mother of Em",
		ClassID: classID, ClassName: "Lý 5", UnitPrice: 100_000,
		BillableCount: 2, AbsentCount: 0, PresentCount: 2,
	}}
	issued := &Invoice{
		ID: id.New(), TeacherID: teacherID, CenterID: period.CenterID, PeriodID: period.ID, StudentID: studentID, ContactID: contactID,
		StudentName: "Vo Em", ContactName: "Mother of Em",
		CurrentCharge: 200_000, TotalDue: 200_000, Status: InvoiceIssued,
	}
	repo.invoices[issued.ID] = issued

	_, err := svc.Draft(ctx, sc, period.ID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("draft against an issued invoice must be 409, got %v", err)
	}
	if len(repo.invoices) != 1 {
		t.Fatalf("nothing must be mutated on refusal, got %d invoices", len(repo.invoices))
	}
	if repo.invoices[issued.ID].Status != InvoiceIssued || repo.invoices[issued.ID].TotalDue != 200_000 {
		t.Fatalf("the existing issued invoice must be untouched, got %+v", repo.invoices[issued.ID])
	}
}

func TestDraftReflectsAdjustmentTotalIntoTotalDue(t *testing.T) {
	ctx := context.Background()
	svc, repo := newPreviewTestService()
	teacherID := id.New()
	period := openPeriod(repo, teacherID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: period.CenterID}

	studentID, contactID, enrollmentID, classID := id.New(), id.New(), id.New(), id.New()
	repo.tallies[period.ID] = []AttendanceTally{{
		EnrollmentID: enrollmentID, StudentID: studentID, ContactID: contactID,
		StudentName: "Do Giang", ContactName: "Mother of Giang",
		ClassID: classID, ClassName: "Hóa 5", UnitPrice: 100_000,
		BillableCount: 2, AbsentCount: 0, PresentCount: 2,
	}}

	first, err := svc.Draft(ctx, sc, period.ID)
	if err != nil {
		t.Fatalf("first draft: %v", err)
	}
	if first.Invoices[0].TotalDue != 200_000 {
		t.Fatalf("total_due before any adjustment = %d, want 200000", first.Invoices[0].TotalDue)
	}

	// Simulate an adjustment recorded directly against the drafted invoice,
	// bypassing phase 4's own service — the point being asserted is that a
	// re-draft never destroys it and folds it into total_due.
	invoiceID := *first.Invoices[0].InvoiceID
	repo.adjustments[invoiceID] = -20_000

	second, err := svc.Draft(ctx, sc, period.ID)
	if err != nil {
		t.Fatalf("second draft: %v", err)
	}
	if second.Invoices[0].AdjustmentTotal != -20_000 {
		t.Fatalf("adjustment_total = %d, want -20000", second.Invoices[0].AdjustmentTotal)
	}
	if second.Invoices[0].TotalDue != 180_000 {
		t.Fatalf("total_due = %d, want 200000-20000=180000", second.Invoices[0].TotalDue)
	}
	if len(repo.invoices) != 1 {
		t.Fatalf("re-draft must not create a second invoice, got %d", len(repo.invoices))
	}
}
