package payments

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// recalcTouched recomputes paid_amount/status for every invoice in set, in
// ascending invoice-id order. The order is not required for correctness once
// the caller has already locked these rows FOR UPDATE, but keeping every
// recompute loop on the same total order removes the last source of
// lock-acquisition divergence between the write paths. sc must be the
// touched payment's own owner scope, derived from the row Reallocate/Reverse/
// AutoAllocateRemainder just locked — never the raw acting caller's — so an
// owner's oversight recompute never silently widens to another teacher's
// invoice.
func (s *Service) recalcTouched(ctx context.Context, sc authctx.Scope, set map[uuid.UUID]bool) error {
	ids := make([]uuid.UUID, 0, len(set))
	for invID := range set {
		ids = append(ids, invID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, invID := range ids {
		if err := s.repo.RecalcInvoicePaid(ctx, sc, invID); err != nil {
			return err
		}
	}
	return nil
}

// reallocatableStatuses are the invoice statuses a reallocation may target —
// draft (not yet issued, nothing to settle) and void (already written off)
// are excluded.
var reallocatableStatuses = map[string]bool{
	"issued":         true,
	"partially_paid": true,
	"paid":           true,
}

// Reallocate replaces paymentID's split with a teacher-supplied set of
// amounts (a manual override of D8's automatic choice), validating every
// line before touching a row: the payment must exist, be within sc's
// tenancy, and be neither a reversal nor reversed; every target invoice must
// belong to the same teacher and the same contact as the payment and sit in
// a status that can still receive money; every amount must be positive; the
// sum must not exceed the payment; and no single invoice may be pushed past
// its total_due. All of that runs under FOR UPDATE inside the same
// transaction that performs the write, so a concurrent payment or
// reallocation for the same contact can never race it into an inconsistent
// split.
func (s *Service) Reallocate(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID, req ReallocateRequest) (*PaymentDetail, error) {
	if len(req.Allocations) == 0 {
		return nil, apperror.Invalid("validation failed", map[string]string{"allocations": "must include at least one line"})
	}

	amounts := make(map[uuid.UUID]int64, len(req.Allocations))
	invoiceIDs := make([]uuid.UUID, 0, len(req.Allocations))
	fields := map[string]string{}
	var sum int64
	for i, line := range req.Allocations {
		key := fmt.Sprintf("allocations[%d]", i)
		if line.Amount <= 0 {
			fields[key] = "amount must be greater than zero"
			continue
		}
		if _, dup := amounts[line.InvoiceID]; dup {
			fields[key] = "invoice_id is duplicated in this request"
			continue
		}
		amounts[line.InvoiceID] = line.Amount
		invoiceIDs = append(invoiceIDs, line.InvoiceID)
		sum += line.Amount
	}
	if len(fields) > 0 {
		return nil, apperror.Invalid("validation failed", fields)
	}

	err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		payment, err := s.repo.LockPayment(txCtx, sc, paymentID)
		if err != nil {
			return err
		}
		// paymentScope is the payment's own owner scope — never sc — so an
		// owner reallocating a member's payment recomputes only that
		// member's invoices, matching billing's periodScope precedent.
		paymentScope := authctx.Scope{TeacherID: payment.TeacherID, CenterID: payment.CenterID}
		if payment.ReversesPaymentID != nil {
			return apperror.Invalid("validation failed", map[string]string{"payment_id": "a reversal entry cannot be reallocated"})
		}
		if payment.ReversedAt != nil {
			return apperror.Invalid("validation failed", map[string]string{"payment_id": "a reversed payment cannot be reallocated"})
		}
		if sum > payment.Amount {
			return apperror.Invalid("validation failed", map[string]string{"allocations": "sum exceeds the payment amount"})
		}

		existing, err := s.repo.AllocationsByPayment(txCtx, paymentScope, paymentID)
		if err != nil {
			return err
		}
		existingByInvoice := make(map[uuid.UUID]int64, len(existing))
		touched := make(map[uuid.UUID]bool, len(existing)+len(invoiceIDs))
		for _, a := range existing {
			existingByInvoice[a.InvoiceID] = a.Amount
			touched[a.InvoiceID] = true
		}
		for _, invID := range invoiceIDs {
			touched[invID] = true
		}

		// Lock the union of the invoices this payment is leaving and the ones
		// it is landing on. An invoice dropped from the split still has its
		// paid_amount recomputed below, so it must be locked here too —
		// otherwise a concurrent reallocation could grab the two invoices in
		// the opposite order and deadlock. InvoicesByIDs locks in invoice-id
		// order, matching every other contact-scoped write path.
		lockIDs := make([]uuid.UUID, 0, len(touched))
		for invID := range touched {
			lockIDs = append(lockIDs, invID)
		}
		invoices, err := s.repo.InvoicesByIDs(txCtx, paymentScope, lockIDs)
		if err != nil {
			return err
		}
		byID := make(map[uuid.UUID]InvoiceRow, len(invoices))
		for _, inv := range invoices {
			byID[inv.ID] = inv
		}

		lineFields := map[string]string{}
		for i, invID := range invoiceIDs {
			key := fmt.Sprintf("allocations[%d]", i)
			inv, ok := byID[invID]
			if !ok {
				lineFields[key] = "invoice not found for this teacher"
				continue
			}
			if inv.ContactID != payment.ContactID {
				lineFields[key] = "invoice belongs to a different contact than the payment"
				continue
			}
			if !reallocatableStatuses[inv.Status] {
				lineFields[key] = "invoice status does not accept payment (must be issued, partially_paid, or paid)"
				continue
			}
			room := inv.TotalDue - (inv.PaidAmount - existingByInvoice[invID])
			if amounts[invID] > room {
				lineFields[key] = "amount exceeds the invoice's outstanding balance"
			}
		}
		if len(lineFields) > 0 {
			return apperror.Invalid("validation failed", lineFields)
		}

		if err := s.repo.DeleteAllocations(txCtx, paymentScope, paymentID); err != nil {
			return err
		}

		rows := make([]PaymentAllocation, 0, len(invoiceIDs))
		for _, invID := range invoiceIDs {
			rows = append(rows, PaymentAllocation{
				ID:          id.New(),
				TeacherID:   paymentScope.TeacherID,
				CenterID:    paymentScope.CenterID,
				PaymentID:   paymentID,
				InvoiceID:   invID,
				Amount:      amounts[invID],
				AllocatedBy: AllocatedManual,
			})
		}
		if err := s.repo.InsertAllocations(txCtx, rows); err != nil {
			return err
		}

		return s.recalcTouched(txCtx, paymentScope, touched)
	})
	if err != nil {
		return nil, translate(err)
	}

	return s.Get(ctx, sc, paymentID)
}

// Reverse creates a new payments row that cancels paymentID: same contact,
// amount, and method, received_on = today, reverses_payment_id pointing at
// the original, and an allocation mirror of the original's split so
// RecalcInvoicePaid's sign convention (subtract a reversal's allocations)
// returns every touched invoice to its exact pre-payment paid_amount and
// status. The original row is stamped reversed_at, never deleted. Refuses a
// payment that is already reversed or is itself a reversal — a reversal
// cannot be undone by reversing it again; recording a fresh payment is the
// correct fix for a wrong reversal (YAGNI: no partial reversal in V1).
func (s *Service) Reverse(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID, req ReverseRequest) (*PaymentDetail, error) {
	var reversalID uuid.UUID
	err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		original, err := s.repo.LockPayment(txCtx, sc, paymentID)
		if err != nil {
			return err
		}
		// paymentScope anchors the reversal entry and every downstream call on
		// the ORIGINAL payment's own owner, not sc — an owner reversing a
		// member's payment must not silently reassign the reversal (and its
		// allocation mirror) to the owner.
		paymentScope := authctx.Scope{TeacherID: original.TeacherID, CenterID: original.CenterID}
		if original.ReversedAt != nil || original.ReversesPaymentID != nil {
			return apperror.Conflict("payment has already been reversed, or is itself a reversal entry")
		}

		allocs, err := s.repo.AllocationsByPayment(txCtx, paymentScope, paymentID)
		if err != nil {
			return err
		}

		// Lock the invoices this reversal will recompute, in invoice-id order,
		// before writing — the same lock discipline recording and reallocation
		// use, so a reversal never deadlocks against a concurrent write on the
		// same contact.
		affected := make([]uuid.UUID, 0, len(allocs))
		for _, a := range allocs {
			affected = append(affected, a.InvoiceID)
		}
		if _, err := s.repo.InvoicesByIDs(txCtx, paymentScope, affected); err != nil {
			return err
		}

		reason := req.Reason
		reversal := &Payment{
			ID:                id.New(),
			TeacherID:         paymentScope.TeacherID,
			CenterID:          paymentScope.CenterID,
			ContactID:         original.ContactID,
			Amount:            original.Amount,
			Method:            original.Method,
			ReceivedOn:        today(s.now()),
			ReversesPaymentID: &original.ID,
			Note:              &reason,
		}
		if err := s.repo.CreatePayment(txCtx, reversal); err != nil {
			return err
		}
		reversalID = reversal.ID

		mirrored := make([]PaymentAllocation, 0, len(allocs))
		touched := make(map[uuid.UUID]bool, len(allocs))
		for _, a := range allocs {
			mirrored = append(mirrored, PaymentAllocation{
				ID:          id.New(),
				TeacherID:   paymentScope.TeacherID,
				CenterID:    paymentScope.CenterID,
				PaymentID:   reversal.ID,
				InvoiceID:   a.InvoiceID,
				Amount:      a.Amount,
				AllocatedBy: a.AllocatedBy,
			})
			touched[a.InvoiceID] = true
		}
		if err := s.repo.InsertAllocations(txCtx, mirrored); err != nil {
			return err
		}

		if err := s.repo.MarkReversed(txCtx, paymentScope, paymentID, s.now()); err != nil {
			return err
		}

		return s.recalcTouched(txCtx, paymentScope, touched)
	})
	if err != nil {
		return nil, translate(err)
	}

	return s.Get(ctx, sc, reversalID)
}

// AutoAllocateRemainder re-runs D8's Allocate over paymentID's unallocated
// remainder only — existing allocations are left untouched, and the fresh
// allocations merge into any row this same payment already holds on an
// invoice (InsertAllocations' ON CONFLICT). Candidates come from the same
// CandidateInvoices query Record uses, which already excludes any invoice
// this payment (or any other) has fully covered — its total_due > paid_amount
// filter is exactly that exclusion, so no extra filtering is needed here.
// Returns 409 when there is no remainder to place; refuses a payment that is
// reversed or is itself a reversal, the same guard Reverse and Reallocate
// enforce.
func (s *Service) AutoAllocateRemainder(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) (*PaymentDetail, error) {
	err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		payment, err := s.repo.LockPayment(txCtx, sc, paymentID)
		if err != nil {
			return err
		}
		// paymentScope is the payment's own owner scope — never sc — so an
		// owner auto-allocating a member's payment only ever touches that
		// member's own candidate invoices.
		paymentScope := authctx.Scope{TeacherID: payment.TeacherID, CenterID: payment.CenterID}
		if payment.ReversedAt != nil || payment.ReversesPaymentID != nil {
			return apperror.Conflict("a reversed payment or a reversal entry cannot be auto-allocated")
		}

		existing, err := s.repo.AllocationsByPayment(txCtx, paymentScope, paymentID)
		if err != nil {
			return err
		}
		var allocated int64
		for _, a := range existing {
			allocated += a.Amount
		}
		remainder := payment.Amount - allocated
		if remainder <= 0 {
			return apperror.Conflict("payment has no unallocated remainder")
		}

		candidates, err := s.repo.CandidateInvoices(txCtx, paymentScope, payment.ContactID)
		if err != nil {
			return err
		}
		allocs, _ := Allocate(remainder, candidates)
		if len(allocs) == 0 {
			// Nothing currently eligible to receive the remainder — not an
			// error, mirrors Record's own behaviour when a payment outruns
			// its contact's outstanding invoices.
			return nil
		}

		rows := make([]PaymentAllocation, 0, len(allocs))
		touched := make(map[uuid.UUID]bool, len(allocs))
		for _, a := range allocs {
			rows = append(rows, PaymentAllocation{
				ID:          id.New(),
				TeacherID:   paymentScope.TeacherID,
				CenterID:    paymentScope.CenterID,
				PaymentID:   paymentID,
				InvoiceID:   a.InvoiceID,
				Amount:      a.Amount,
				AllocatedBy: AllocatedAuto,
			})
			touched[a.InvoiceID] = true
		}
		if err := s.repo.InsertAllocations(txCtx, rows); err != nil {
			return err
		}

		return s.recalcTouched(txCtx, paymentScope, touched)
	})
	if err != nil {
		return nil, translate(err)
	}

	return s.Get(ctx, sc, paymentID)
}

// today resolves "now" as the calendar day at UTC midnight — V1's
// system-wide convention (adr.md, "chuẩn hôm nay dùng UTC midnight"), the
// same form RecordPaymentRequest.ReceivedOn parses into (dateLayout) and DATE
// columns compare against. Overridable indirectly through Service.now.
func today(now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
