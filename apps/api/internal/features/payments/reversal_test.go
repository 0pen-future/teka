package payments

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
)

// --- fake repository extension ---
//
// service_test.go's fakeInvoice fixture carries no contact_id or status —
// Record's own tests never needed either. Reallocate's validation needs both,
// so this file keeps that per-invoice metadata in a side table keyed by the
// owning *fakeRepository instance rather than adding fields to fakeInvoice or
// fakeRepository, which only service_test.go may edit. Go allows adding new
// methods to an existing type from another file in the same package, so the
// five methods phase 2 adds to Repository are implemented here directly on
// *fakeRepository, keeping every existing service_test.go test compiling and
// passing unchanged.

type invoiceMeta struct {
	ContactID uuid.UUID
	Status    string
}

var invoiceMetaByRepo = map[*fakeRepository]map[uuid.UUID]invoiceMeta{}

func setInvoiceMeta(f *fakeRepository, invoiceID, contactID uuid.UUID, status string) {
	m := invoiceMetaByRepo[f]
	if m == nil {
		m = map[uuid.UUID]invoiceMeta{}
		invoiceMetaByRepo[f] = m
	}
	m[invoiceID] = invoiceMeta{ContactID: contactID, Status: status}
}

func (f *fakeRepository) LockPayment(ctx context.Context, teacherID, paymentID uuid.UUID) (*Payment, error) {
	return f.GetPayment(ctx, teacherID, paymentID)
}

func (f *fakeRepository) InvoicesByIDs(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]InvoiceRow, error) {
	meta := invoiceMetaByRepo[f]
	var rows []InvoiceRow
	for _, invID := range ids {
		inv, ok := f.invoices[invID]
		if !ok {
			continue
		}
		m := meta[invID]
		rows = append(rows, InvoiceRow{ID: invID, ContactID: m.ContactID, Status: m.Status, TotalDue: inv.TotalDue, PaidAmount: inv.PaidAmount})
	}
	return rows, nil
}

func (f *fakeRepository) DeleteAllocations(_ context.Context, teacherID, paymentID uuid.UUID) error {
	out := f.allocations[:0:0]
	for _, a := range f.allocations {
		if a.TeacherID == teacherID && a.PaymentID == paymentID {
			continue
		}
		out = append(out, a)
	}
	f.allocations = out
	return nil
}

func (f *fakeRepository) AllocationsByPayment(_ context.Context, teacherID, paymentID uuid.UUID) ([]PaymentAllocation, error) {
	var out []PaymentAllocation
	for _, a := range f.allocations {
		if a.TeacherID == teacherID && a.PaymentID == paymentID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepository) MarkReversed(_ context.Context, teacherID, paymentID uuid.UUID, at time.Time) error {
	p, ok := f.payments[paymentID]
	if !ok || p.TeacherID != teacherID {
		return ErrPaymentNotFound
	}
	p.ReversedAt = &at
	f.payments[paymentID] = p
	return nil
}

// --- test fixture helpers ---

func addInvoice(f *fakeRepository, contactID uuid.UUID, totalDue, paidAmount int64, status string) uuid.UUID {
	invoiceID := id.New()
	f.invoices[invoiceID] = &fakeInvoice{TotalDue: totalDue, PaidAmount: paidAmount}
	setInvoiceMeta(f, invoiceID, contactID, status)
	return invoiceID
}

func addPayment(f *fakeRepository, teacherID, contactID uuid.UUID, amount int64) *Payment {
	p := Payment{ID: id.New(), TeacherID: teacherID, ContactID: contactID, Amount: amount, Method: MethodCash, ReceivedOn: testPeriodStart}
	f.payments[p.ID] = p
	cp := p
	return &cp
}

func addAllocation(f *fakeRepository, teacherID, paymentID, invoiceID uuid.UUID, amount int64, allocatedBy string) {
	f.allocations = append(f.allocations, PaymentAllocation{
		ID: id.New(), TeacherID: teacherID, PaymentID: paymentID, InvoiceID: invoiceID,
		Amount: amount, AllocatedBy: allocatedBy,
	})
}

// --- Reallocate validation branches ---

func TestReallocateReversedPaymentIsInvalid(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 0, "issued")
	payment := addPayment(repo, teacherID, contactID, 100_000)
	reversedAt := time.Now()
	payment.ReversedAt = &reversedAt
	repo.payments[payment.ID] = *payment

	_, err := svc.Reallocate(ctx, teacherID, payment.ID, ReallocateRequest{
		Allocations: []ReallocationLine{{InvoiceID: invID, Amount: 100_000}},
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("reallocating a reversed payment must be 422, got %v", err)
	}
	if len(repo.allocations) != 0 {
		t.Fatalf("rejected reallocation must write nothing, got %d rows", len(repo.allocations))
	}
}

func TestReallocateReversalEntryIsInvalid(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 0, "issued")
	original := addPayment(repo, teacherID, contactID, 100_000)
	reversal := addPayment(repo, teacherID, contactID, 100_000)
	reversal.ReversesPaymentID = &original.ID
	repo.payments[reversal.ID] = *reversal

	_, err := svc.Reallocate(ctx, teacherID, reversal.ID, ReallocateRequest{
		Allocations: []ReallocationLine{{InvoiceID: invID, Amount: 100_000}},
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("reallocating a reversal entry must be 422, got %v", err)
	}
}

func TestReallocateToAnotherContactsInvoiceIsInvalid(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID, otherContactID := id.New(), id.New(), id.New()
	invID := addInvoice(repo, otherContactID, 100_000, 0, "issued")
	payment := addPayment(repo, teacherID, contactID, 100_000)

	_, err := svc.Reallocate(ctx, teacherID, payment.ID, ReallocateRequest{
		Allocations: []ReallocationLine{{InvoiceID: invID, Amount: 100_000}},
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("reallocating to another contact's invoice must be 422, got %v", err)
	}
	if len(repo.allocations) != 0 {
		t.Fatalf("rejected reallocation must write nothing, got %d rows", len(repo.allocations))
	}
}

func TestReallocateSumExceedingPaymentIsInvalid(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invA := addInvoice(repo, contactID, 200_000, 0, "issued")
	invB := addInvoice(repo, contactID, 200_000, 0, "issued")
	payment := addPayment(repo, teacherID, contactID, 100_000)

	_, err := svc.Reallocate(ctx, teacherID, payment.ID, ReallocateRequest{
		Allocations: []ReallocationLine{
			{InvoiceID: invA, Amount: 60_000},
			{InvoiceID: invB, Amount: 60_000},
		},
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("sum exceeding the payment amount must be 422, got %v", err)
	}
}

func TestReallocateAmountExceedingInvoiceRoomIsInvalid(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 30_000, "partially_paid")
	payment := addPayment(repo, teacherID, contactID, 100_000)

	_, err := svc.Reallocate(ctx, teacherID, payment.ID, ReallocateRequest{
		// room = total_due(100000) - (paid(30000) - existing-from-this-payment(0)) = 70000
		Allocations: []ReallocationLine{{InvoiceID: invID, Amount: 80_000}},
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("amount exceeding the invoice's room must be 422, got %v", err)
	}
}

func TestReallocateToDraftInvoiceIsInvalid(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 0, "draft")
	payment := addPayment(repo, teacherID, contactID, 100_000)

	_, err := svc.Reallocate(ctx, teacherID, payment.ID, ReallocateRequest{
		Allocations: []ReallocationLine{{InvoiceID: invID, Amount: 100_000}},
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("reallocating to a draft invoice must be 422, got %v", err)
	}
}

func TestReallocateSuccessRewritesToManualAndRecalcsUnion(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invA := addInvoice(repo, contactID, 100_000, 70_000, "partially_paid")
	invB := addInvoice(repo, contactID, 100_000, 30_000, "partially_paid")
	payment := addPayment(repo, teacherID, contactID, 100_000)
	addAllocation(repo, teacherID, payment.ID, invA, 70_000, AllocatedAuto)
	addAllocation(repo, teacherID, payment.ID, invB, 30_000, AllocatedAuto)

	detail, err := svc.Reallocate(ctx, teacherID, payment.ID, ReallocateRequest{
		Allocations: []ReallocationLine{{InvoiceID: invA, Amount: 100_000}},
	})
	if err != nil {
		t.Fatalf("reallocate: %v", err)
	}
	if len(detail.Allocations) != 1 || detail.Allocations[0].InvoiceID != invA {
		t.Fatalf("allocations = %+v, want exactly invA at 100000", detail.Allocations)
	}
	if detail.Allocations[0].AllocatedBy != AllocatedManual {
		t.Fatalf("allocated_by = %s, want manual", detail.Allocations[0].AllocatedBy)
	}
	touched := map[uuid.UUID]bool{}
	for _, invID := range repo.recalcCalls {
		touched[invID] = true
	}
	if !touched[invA] || !touched[invB] {
		t.Fatalf("recalc must cover the union of old and new invoices, got %+v", repo.recalcCalls)
	}
}

// --- Reverse ---

func TestReverseTwiceIsConflict(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 100_000, "paid")
	payment := addPayment(repo, teacherID, contactID, 100_000)
	addAllocation(repo, teacherID, payment.ID, invID, 100_000, AllocatedAuto)

	_, err := svc.Reverse(ctx, teacherID, payment.ID, ReverseRequest{Reason: "recorded twice by mistake"})
	if err != nil {
		t.Fatalf("first reverse: %v", err)
	}
	if len(repo.payments) != 2 {
		t.Fatalf("want original + one reversal row, got %d payments", len(repo.payments))
	}

	_, err = svc.Reverse(ctx, teacherID, payment.ID, ReverseRequest{Reason: "trying again"})
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("second reverse attempt must be 409, got %v", err)
	}
	if len(repo.payments) != 2 {
		t.Fatalf("a rejected second reversal must write no new row, got %d payments", len(repo.payments))
	}
}

func TestReverseUnknownPaymentIsNotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	_, err := svc.Reverse(ctx, id.New(), id.New(), ReverseRequest{Reason: "does not exist"})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("reversing an unknown payment must be 404, got %v", err)
	}
}

func TestReverseMirrorsAllocationsAndStampsOriginal(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 100_000, "paid")
	payment := addPayment(repo, teacherID, contactID, 100_000)
	addAllocation(repo, teacherID, payment.ID, invID, 100_000, AllocatedAuto)

	detail, err := svc.Reverse(ctx, teacherID, payment.ID, ReverseRequest{Reason: "wrong contact"})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if detail.Payment.ReversesPaymentID == nil || *detail.Payment.ReversesPaymentID != payment.ID {
		t.Fatalf("reversal row must point reverses_payment_id at the original, got %+v", detail.Payment)
	}
	if len(detail.Allocations) != 1 || detail.Allocations[0].InvoiceID != invID || detail.Allocations[0].Amount != 100_000 {
		t.Fatalf("reversal must mirror the original's allocations exactly, got %+v", detail.Allocations)
	}

	original, err := repo.GetPayment(ctx, teacherID, payment.ID)
	if err != nil {
		t.Fatalf("get original: %v", err)
	}
	if original.ReversedAt == nil {
		t.Fatalf("original payment must have reversed_at stamped")
	}
}

// --- AutoAllocateRemainder ---

func TestAutoAllocateRemainderZeroIsConflict(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	invID := addInvoice(repo, contactID, 100_000, 100_000, "paid")
	payment := addPayment(repo, teacherID, contactID, 100_000)
	addAllocation(repo, teacherID, payment.ID, invID, 100_000, AllocatedAuto)

	_, err := svc.AutoAllocateRemainder(ctx, teacherID, payment.ID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("zero remainder must be 409, got %v", err)
	}
}

func TestAutoAllocateRemainderOnReversedPaymentIsConflict(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	teacherID, contactID := id.New(), id.New()
	payment := addPayment(repo, teacherID, contactID, 100_000)
	reversedAt := time.Now()
	payment.ReversedAt = &reversedAt
	repo.payments[payment.ID] = *payment

	_, err := svc.AutoAllocateRemainder(ctx, teacherID, payment.ID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("auto-allocating a reversed payment must be 409, got %v", err)
	}
}
