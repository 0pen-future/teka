package payments

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// --- fake Repository ---

// fakeInvoice is the subset of an invoice row RecalcInvoicePaid and
// ListAllocations need — enough to exercise the service's orchestration
// without a database. The real recompute SQL (recalcInvoicePaidQuery) is
// exercised only against Postgres, in integration_test.go.
type fakeInvoice struct {
	StudentID   uuid.UUID
	StudentName string
	PeriodID    uuid.UUID
	TotalDue    int64
	PaidAmount  int64
}

type fakeRepository struct {
	// contacts maps contactID -> owning teacherID.
	contacts map[uuid.UUID]uuid.UUID
	// candidates maps contactID -> the invoices CandidateInvoices returns for it.
	candidates  map[uuid.UUID][]Candidate
	invoices    map[uuid.UUID]*fakeInvoice
	payments    map[uuid.UUID]Payment
	allocations []PaymentAllocation
	recalcCalls []uuid.UUID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		contacts:   map[uuid.UUID]uuid.UUID{},
		candidates: map[uuid.UUID][]Candidate{},
		invoices:   map[uuid.UUID]*fakeInvoice{},
		payments:   map[uuid.UUID]Payment{},
	}
}

func (f *fakeRepository) CreatePayment(_ context.Context, _ uuid.UUID, p *Payment) error {
	f.payments[p.ID] = *p
	return nil
}

func (f *fakeRepository) GetPayment(_ context.Context, teacherID, paymentID uuid.UUID) (*Payment, error) {
	p, ok := f.payments[paymentID]
	if !ok || p.TeacherID != teacherID {
		return nil, ErrPaymentNotFound
	}
	cp := p
	return &cp, nil
}

func (f *fakeRepository) ListPayments(_ context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Payment, int64, error) {
	var out []Payment
	for _, row := range f.payments {
		if row.TeacherID != teacherID {
			continue
		}
		if filter.ContactID != uuid.Nil && row.ContactID != filter.ContactID {
			continue
		}
		out = append(out, row)
	}
	total := int64(len(out))
	if p.PerPage > 0 && p.Offset() < len(out) {
		end := min(len(out), p.Offset()+p.PerPage)
		out = out[p.Offset():end]
	}
	return out, total, nil
}

func (f *fakeRepository) CandidateInvoices(_ context.Context, _, contactID uuid.UUID) ([]Candidate, error) {
	return f.candidates[contactID], nil
}

func (f *fakeRepository) InsertAllocations(_ context.Context, _ uuid.UUID, rows []PaymentAllocation) error {
	f.allocations = append(f.allocations, rows...)
	return nil
}

func (f *fakeRepository) RecalcInvoicePaid(_ context.Context, _, invoiceID uuid.UUID) error {
	f.recalcCalls = append(f.recalcCalls, invoiceID)
	inv, ok := f.invoices[invoiceID]
	if !ok {
		return nil
	}
	var paid int64
	for _, a := range f.allocations {
		if a.InvoiceID != invoiceID {
			continue
		}
		if src := f.payments[a.PaymentID]; src.ReversesPaymentID == nil {
			paid += a.Amount
		} else {
			paid -= a.Amount
		}
	}
	inv.PaidAmount = paid
	return nil
}

func (f *fakeRepository) ContactExists(_ context.Context, teacherID, contactID uuid.UUID) (bool, error) {
	owner, ok := f.contacts[contactID]
	return ok && owner == teacherID, nil
}

func (f *fakeRepository) ListAllocations(_ context.Context, teacherID, paymentID uuid.UUID) ([]AllocationRow, error) {
	var out []AllocationRow
	for _, a := range f.allocations {
		if a.TeacherID != teacherID || a.PaymentID != paymentID {
			continue
		}
		out = append(out, f.rowFor(a))
	}
	return out, nil
}

func (f *fakeRepository) ListAllocationsForPayments(_ context.Context, teacherID uuid.UUID, paymentIDs []uuid.UUID) ([]AllocationRow, error) {
	want := make(map[uuid.UUID]bool, len(paymentIDs))
	for _, id := range paymentIDs {
		want[id] = true
	}
	var out []AllocationRow
	for _, a := range f.allocations {
		if a.TeacherID != teacherID || !want[a.PaymentID] {
			continue
		}
		out = append(out, f.rowFor(a))
	}
	return out, nil
}

func (f *fakeRepository) rowFor(a PaymentAllocation) AllocationRow {
	inv := f.invoices[a.InvoiceID]
	row := AllocationRow{
		PaymentID:   a.PaymentID,
		InvoiceID:   a.InvoiceID,
		Amount:      a.Amount,
		AllocatedBy: a.AllocatedBy,
	}
	if inv != nil {
		row.StudentID = inv.StudentID
		row.StudentName = inv.StudentName
		row.PeriodID = inv.PeriodID
		row.TotalDue = inv.TotalDue
		row.PaidAmount = inv.PaidAmount
	}
	return row
}

// --- noopTx ---

type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	return NewService(repo, noopTx{}), repo
}

func addContact(repo *fakeRepository, teacherID uuid.UUID) uuid.UUID {
	contactID := id.New()
	repo.contacts[contactID] = teacherID
	return contactID
}

func addCandidate(repo *fakeRepository, contactID uuid.UUID, inv fakeInvoice, opening int64) uuid.UUID {
	invoiceID := id.New()
	repo.invoices[invoiceID] = &inv
	repo.candidates[contactID] = append(repo.candidates[contactID], Candidate{
		InvoiceID:      invoiceID,
		PeriodStart:    testPeriodStart,
		OpeningBalance: opening,
		TotalDue:       inv.TotalDue,
		PaidAmount:     inv.PaidAmount,
	})
	return invoiceID
}

var testPeriodStart = mustParseDate("2026-01-01")

func mustParseDate(s string) time.Time {
	parsed, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestRecordUnknownContactIsNotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	teacherID := id.New()

	_, err := svc.Record(ctx, teacherID, RecordPaymentRequest{
		ContactID:  id.New(),
		Amount:     100_000,
		Method:     MethodCash,
		ReceivedOn: "2026-01-15",
	})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("unknown contact must be 404, got %v", err)
	}
}

func TestRecordAnotherTeachersContactIsNotFound(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	owner, stranger := id.New(), id.New()
	contactID := addContact(repo, owner)

	_, err := svc.Record(ctx, stranger, RecordPaymentRequest{
		ContactID:  contactID,
		Amount:     100_000,
		Method:     MethodCash,
		ReceivedOn: "2026-01-15",
	})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant contact must be 404, got %v", err)
	}
	if len(repo.payments) != 0 {
		t.Fatalf("rejected record must write nothing, got %d payments", len(repo.payments))
	}
}

func TestRecordAllocatesAndRecalcsEveryTouchedInvoice(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	contactID := addContact(repo, teacherID)
	studentA, studentB := id.New(), id.New()
	invA := addCandidate(repo, contactID, fakeInvoice{StudentID: studentA, StudentName: "An", TotalDue: 100_000}, 0)
	invB := addCandidate(repo, contactID, fakeInvoice{StudentID: studentB, StudentName: "Bình", TotalDue: 150_000}, 0)

	detail, err := svc.Record(ctx, teacherID, RecordPaymentRequest{
		ContactID:  contactID,
		Amount:     250_000,
		Method:     MethodTransfer,
		ReceivedOn: "2026-01-15",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if detail.UnallocatedAmount != 0 {
		t.Fatalf("unallocated = %d, want 0", detail.UnallocatedAmount)
	}
	if len(detail.Allocations) != 2 {
		t.Fatalf("allocations = %+v, want 2 rows", detail.Allocations)
	}
	for _, a := range detail.Allocations {
		if a.AllocatedBy != AllocatedAuto {
			t.Fatalf("allocated_by = %s, want auto", a.AllocatedBy)
		}
		if a.TotalDue-a.PaidAmount != 0 {
			t.Fatalf("invoice %s left with outstanding %d after an exact payment", a.InvoiceID, a.TotalDue-a.PaidAmount)
		}
	}
	if len(repo.recalcCalls) != 2 {
		t.Fatalf("RecalcInvoicePaid must run once per touched invoice, got %d calls", len(repo.recalcCalls))
	}
	touched := map[uuid.UUID]bool{}
	for _, id := range repo.recalcCalls {
		touched[id] = true
	}
	if !touched[invA] || !touched[invB] {
		t.Fatalf("recalc must touch both invoices, got %+v", repo.recalcCalls)
	}
}

func TestRecordWithNoCandidatesLeavesEverythingUnallocated(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	contactID := addContact(repo, teacherID)

	detail, err := svc.Record(ctx, teacherID, RecordPaymentRequest{
		ContactID:  contactID,
		Amount:     50_000,
		Method:     MethodCash,
		ReceivedOn: "2026-01-15",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if detail.UnallocatedAmount != 50_000 {
		t.Fatalf("unallocated = %d, want 50000 (no invoices to settle)", detail.UnallocatedAmount)
	}
	if len(detail.Allocations) != 0 {
		t.Fatalf("allocations = %+v, want none", detail.Allocations)
	}
	if len(repo.recalcCalls) != 0 {
		t.Fatalf("recalc must not run when nothing was allocated, got %d calls", len(repo.recalcCalls))
	}
}

func TestRecordRejectsMalformedReceivedOn(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	contactID := addContact(repo, teacherID)

	_, err := svc.Record(ctx, teacherID, RecordPaymentRequest{
		ContactID:  contactID,
		Amount:     10_000,
		Method:     MethodCash,
		ReceivedOn: "15-01-2026",
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("malformed received_on must be a validation error, got %v", err)
	}
}

func TestGetPaymentNotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	_, err := svc.Get(ctx, id.New(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing payment must be 404, got %v", err)
	}
}

func TestGetPaymentCrossTenantIsNotFound(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	owner, stranger := id.New(), id.New()
	contactID := addContact(repo, owner)

	detail, err := svc.Record(ctx, owner, RecordPaymentRequest{
		ContactID:  contactID,
		Amount:     10_000,
		Method:     MethodCash,
		ReceivedOn: "2026-01-15",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	_, err = svc.Get(ctx, stranger, detail.Payment.ID)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %v", err)
	}
}

func TestListPaymentsScopedToTeacherAndBatchesAllocations(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherA, teacherB := id.New(), id.New()
	contactA := addContact(repo, teacherA)
	contactB := addContact(repo, teacherB)
	addCandidate(repo, contactA, fakeInvoice{StudentID: id.New(), StudentName: "An", TotalDue: 50_000}, 0)
	addCandidate(repo, contactB, fakeInvoice{StudentID: id.New(), StudentName: "Bình", TotalDue: 50_000}, 0)

	if _, err := svc.Record(ctx, teacherA, RecordPaymentRequest{ContactID: contactA, Amount: 50_000, Method: MethodCash, ReceivedOn: "2026-01-10"}); err != nil {
		t.Fatalf("record A: %v", err)
	}
	if _, err := svc.Record(ctx, teacherB, RecordPaymentRequest{ContactID: contactB, Amount: 50_000, Method: MethodCash, ReceivedOn: "2026-01-10"}); err != nil {
		t.Fatalf("record B: %v", err)
	}

	details, total, err := svc.List(ctx, teacherA, ListFilter{}, pagination.Params{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(details) != 1 {
		t.Fatalf("want exactly teacher A's own payment, got total=%d rows=%d", total, len(details))
	}
	if details[0].Payment.ContactID != contactA {
		t.Fatalf("list must not leak another teacher's payment, got %+v", details[0])
	}
	if len(details[0].Allocations) != 1 {
		t.Fatalf("allocations = %+v, want 1", details[0].Allocations)
	}
}
