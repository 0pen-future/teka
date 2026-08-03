package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
)

// closeFakeRepository extends previewFakeRepository with real (not stub)
// Close/Void-supporting behaviour: bulk void/issue by natural predicate and
// a guarded single-invoice void, so Close and VoidInvoice's own logic is
// meaningfully exercised without a real database. LockPeriod and ClosePeriod
// are inherited unchanged from fakeRepository — both already implement the
// real status-guarded semantics these tests need.
type closeFakeRepository struct {
	*previewFakeRepository
}

func newCloseFakeRepository() *closeFakeRepository {
	return &closeFakeRepository{previewFakeRepository: newPreviewFakeRepository()}
}

func (f *closeFakeRepository) IssueDraftInvoices(_ context.Context, teacherID, periodID uuid.UUID) (int64, error) {
	var n int64
	for _, inv := range f.invoices {
		if inv.TeacherID != teacherID || inv.PeriodID != periodID || inv.Status != InvoiceDraft {
			continue
		}
		inv.Status = InvoiceIssued
		n++
	}
	return n, nil
}

func (f *closeFakeRepository) VoidInvoices(_ context.Context, teacherID, periodID uuid.UUID) (int64, error) {
	var n int64
	now := time.Now()
	for _, inv := range f.invoices {
		if inv.TeacherID != teacherID || inv.PeriodID != periodID || inv.Status != InvoiceDraft {
			continue
		}
		if inv.CurrentCharge != 0 || inv.OpeningBalance != 0 || inv.AdjustmentTotal != 0 {
			continue
		}
		reason := emptyInvoiceVoidReason
		inv.Status = InvoiceVoid
		inv.VoidReason = &reason
		inv.VoidedAt = &now
		n++
	}
	return n, nil
}

func (f *closeFakeRepository) GetInvoice(_ context.Context, teacherID, invoiceID uuid.UUID) (*Invoice, error) {
	inv, ok := f.invoices[invoiceID]
	if !ok || inv.TeacherID != teacherID {
		return nil, ErrInvoiceNotFound
	}
	cp := *inv
	return &cp, nil
}

func (f *closeFakeRepository) LockInvoice(_ context.Context, teacherID, invoiceID uuid.UUID) (*Invoice, error) {
	inv, ok := f.invoices[invoiceID]
	if !ok || inv.TeacherID != teacherID {
		return nil, ErrInvoiceNotFound
	}
	cp := *inv
	return &cp, nil
}

func (f *closeFakeRepository) VoidInvoice(_ context.Context, teacherID, invoiceID uuid.UUID, reason string, at time.Time) error {
	inv, ok := f.invoices[invoiceID]
	if !ok || inv.TeacherID != teacherID {
		return ErrInvoiceNotFound
	}
	if inv.Status != InvoiceIssued && inv.Status != InvoicePartiallyPaid {
		return ErrInvoiceNotFound
	}
	inv.Status = InvoiceVoid
	inv.VoidReason = &reason
	inv.VoidedAt = &at
	return nil
}

// newCloseTestService builds a Service with a fixed now() so Close's
// teacher-timezone "today" resolution is deterministic across a test run.
func newCloseTestService(pending PendingSource, now time.Time) (*Service, *closeFakeRepository) {
	repo := newCloseFakeRepository()
	svc := &Service{repo: repo, tx: noopTx{}, pending: pending, now: func() time.Time { return now }}
	return svc, repo
}

func TestCloseVoidsEmptyDraftsAndIssuesChargedOnes(t *testing.T) {
	ctx := context.Background()
	svc, repo := newCloseTestService(&fakePendingSource{}, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	teacherID := id.New()
	repo.setTimezone(teacherID, "UTC")
	period := openPeriod(repo.previewFakeRepository, teacherID,
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)

	chargedStudent, chargedEnrollment, chargedClass := id.New(), id.New(), id.New()
	// emptyStudent is enrolled (has a tally row) but every session this
	// period was cancelled: billable_count and absent_count both zero, no
	// carried debt — the void-empty rule's target case.
	emptyStudent, emptyEnrollment, emptyClass := id.New(), id.New(), id.New()
	repo.tallies[period.ID] = []AttendanceTally{
		{
			EnrollmentID: chargedEnrollment, StudentID: chargedStudent, ContactID: id.New(),
			StudentName: "Nguyen An", ContactName: "Mother of An",
			ClassID: chargedClass, ClassName: "Toán 5", ClassStartDate: period.PeriodStart,
			UnitPrice: 100_000, BillableCount: 4, AbsentCount: 0, PresentCount: 4,
		},
		{
			EnrollmentID: emptyEnrollment, StudentID: emptyStudent, ContactID: id.New(),
			StudentName: "Tran Binh", ContactName: "Mother of Binh",
			ClassID: emptyClass, ClassName: "Văn 5", ClassStartDate: period.PeriodStart,
			UnitPrice: 100_000, BillableCount: 0, AbsentCount: 0, PresentCount: 0,
		},
	}

	resp, err := svc.Close(ctx, teacherID, period.ID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if resp.IssuedCount != 1 {
		t.Fatalf("issued_count = %d, want 1", resp.IssuedCount)
	}
	if resp.VoidedCount != 1 {
		t.Fatalf("voided_count = %d, want 1", resp.VoidedCount)
	}
	if resp.TotalDue != 400_000 {
		t.Fatalf("total_due = %d, want 400000 (only the issued invoice)", resp.TotalDue)
	}
	if resp.Period.Status != PeriodClosed {
		t.Fatalf("period.status = %s, want closed", resp.Period.Status)
	}

	invoices, err := repo.ListInvoices(ctx, teacherID, period.ID)
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	byStudent := map[uuid.UUID]Invoice{}
	for _, inv := range invoices {
		byStudent[inv.StudentID] = inv
	}
	if byStudent[chargedStudent].Status != InvoiceIssued {
		t.Fatalf("charged student invoice status = %s, want issued", byStudent[chargedStudent].Status)
	}
	emptyInvoice := byStudent[emptyStudent]
	if emptyInvoice.Status != InvoiceVoid {
		t.Fatalf("empty student invoice status = %s, want void", emptyInvoice.Status)
	}
	if emptyInvoice.VoidedAt == nil {
		t.Fatal("voided invoice must have voided_at set")
	}
	if emptyInvoice.VoidReason == nil || *emptyInvoice.VoidReason == "" {
		t.Fatal("voided invoice must have a non-blank void_reason")
	}
}

func TestCloseBlocksOnUnconfirmedSessionsAndPeriodStaysOpen(t *testing.T) {
	ctx := context.Background()
	teacherID := id.New()
	sessionA, classA := id.New(), id.New()
	sessionB, classB := id.New(), id.New()

	// respond returns the two sessions deliberately out of session_date
	// order, so the test also asserts Close sorts them.
	pending := &fakePendingSource{respond: func(_, _ *time.Time, _ time.Time) (*sessions.PendingResponse, error) {
		return &sessions.PendingResponse{
			Total: 2,
			Items: []sessions.PendingSessionResponse{
				{SessionID: sessionB, ClassID: classB, ClassName: "Zzz", SessionDate: "2026-02-20", Status: "planned"},
				{SessionID: sessionA, ClassID: classA, ClassName: "Aaa", SessionDate: "2026-02-10", Status: "held"},
			},
		}, nil
	}}

	svc, repo := newCloseTestService(pending, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	repo.setTimezone(teacherID, "UTC")
	period := openPeriod(repo.previewFakeRepository, teacherID,
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)

	_, err := svc.Close(ctx, teacherID, period.ID)
	var blocked *ErrUnconfirmedSessions
	if !errors.As(err, &blocked) {
		t.Fatalf("close must be blocked with ErrUnconfirmedSessions, got %v", err)
	}
	if len(blocked.Sessions) != 2 {
		t.Fatalf("want 2 unconfirmed sessions in the block payload, got %d", len(blocked.Sessions))
	}
	if blocked.Sessions[0].SessionID != sessionA || blocked.Sessions[1].SessionID != sessionB {
		t.Fatalf("sessions must be ordered by session_date, got %+v", blocked.Sessions)
	}

	reloaded, err := repo.GetPeriod(ctx, teacherID, period.ID)
	if err != nil {
		t.Fatalf("get period: %v", err)
	}
	if reloaded.Status != PeriodOpen {
		t.Fatalf("a blocked close must leave the period open, got %s", reloaded.Status)
	}
	if len(repo.invoices) != 0 {
		t.Fatalf("a blocked close must write no invoices, got %d", len(repo.invoices))
	}
}

func TestCloseSucceedsWithFutureUnconfirmedSessionAsWarningOnly(t *testing.T) {
	ctx := context.Background()
	teacherID := id.New()
	futureSessionID, futureClassID := id.New(), id.New()
	today := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)

	pending := &fakePendingSource{respond: func(_, _ *time.Time, before time.Time) (*sessions.PendingResponse, error) {
		if before.Equal(today) {
			// The blocking call's before=today: nothing pending, so close
			// must not be refused.
			return &sessions.PendingResponse{}, nil
		}
		// The future-warnings call: one session past today but still in the
		// period, and Close must not block on it.
		return &sessions.PendingResponse{
			Total: 1,
			Items: []sessions.PendingSessionResponse{
				{SessionID: futureSessionID, ClassID: futureClassID, ClassName: "Toán 5", SessionDate: "2026-02-20", Status: "planned"},
			},
		}, nil
	}}

	svc, repo := newCloseTestService(pending, today)
	repo.setTimezone(teacherID, "UTC")
	period := openPeriod(repo.previewFakeRepository, teacherID,
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodOpen)

	resp, err := svc.Close(ctx, teacherID, period.ID)
	if err != nil {
		t.Fatalf("close must succeed despite a future unconfirmed session, got: %v", err)
	}
	if resp.Period.Status != PeriodClosed {
		t.Fatalf("period.status = %s, want closed", resp.Period.Status)
	}
	if len(resp.Warnings.FutureUnconfirmedSessions) != 1 {
		t.Fatalf("want 1 future warning, got %d", len(resp.Warnings.FutureUnconfirmedSessions))
	}
	if resp.Warnings.FutureUnconfirmedSessions[0].SessionID != futureSessionID {
		t.Fatalf("warning session id = %s, want %s", resp.Warnings.FutureUnconfirmedSessions[0].SessionID, futureSessionID)
	}
}

func TestCloseOnClosedPeriodIsConflict(t *testing.T) {
	ctx := context.Background()
	svc, repo := newCloseTestService(&fakePendingSource{}, time.Now())
	teacherID := id.New()
	repo.setTimezone(teacherID, "UTC")
	period := openPeriod(repo.previewFakeRepository, teacherID,
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), PeriodClosed)

	_, err := svc.Close(ctx, teacherID, period.ID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("closing an already-closed period must be 409, got %v", err)
	}
}

func TestVoidInvoiceGuards(t *testing.T) {
	ctx := context.Background()
	svc, repo := newCloseTestService(&fakePendingSource{}, time.Now())
	teacherID := id.New()

	notIssued := &Invoice{ID: id.New(), TeacherID: teacherID, Status: InvoiceDraft}
	repo.invoices[notIssued.ID] = notIssued
	if _, err := svc.VoidInvoice(ctx, teacherID, notIssued.ID, "mistake"); apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("voiding a draft invoice must be 409, got %v", err)
	}

	paidIssued := &Invoice{ID: id.New(), TeacherID: teacherID, Status: InvoiceIssued, PaidAmount: 50_000}
	repo.invoices[paidIssued.ID] = paidIssued
	if _, err := svc.VoidInvoice(ctx, teacherID, paidIssued.ID, "mistake"); apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("voiding an invoice with a recorded payment must be 409, got %v", err)
	}

	voidable := &Invoice{ID: id.New(), TeacherID: teacherID, Status: InvoiceIssued, TotalDue: 100_000}
	repo.invoices[voidable.ID] = voidable
	resp, err := svc.VoidInvoice(ctx, teacherID, voidable.ID, "double-billed by mistake")
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if resp.Status != InvoiceVoid {
		t.Fatalf("status = %s, want void", resp.Status)
	}
	if resp.VoidReason == nil || *resp.VoidReason != "double-billed by mistake" {
		t.Fatalf("void_reason = %v, want the given reason", resp.VoidReason)
	}
	if resp.VoidedAt == nil {
		t.Fatal("voided_at must be set")
	}

	if _, err := svc.VoidInvoice(ctx, teacherID, id.New(), "mistake"); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing invoice must be 404, got %v", err)
	}

	stranger := id.New()
	if _, err := svc.VoidInvoice(ctx, stranger, voidable.ID, "mistake"); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant void must be 404, got %v", err)
	}
}
