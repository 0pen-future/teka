package payments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// PaymentDetail bundles a payment with its allocation breakdown — the shape
// every read response returns so a client never has to reimplement D8 to
// know which child a đồng landed on.
type PaymentDetail struct {
	Payment           Payment
	Allocations       []AllocationRow
	UnallocatedAmount int64
}

// Service owns payment recording and the D8 auto-allocation flow.
type Service struct {
	repo Repository
	tx   database.TxManager
	// now is overridden in tests so Reverse's "today" (the reversal row's
	// received_on) is deterministic; production always gets time.Now. Same
	// seam billing.Service uses for its Close cutoff.
	now func() time.Time
}

// NewService builds the payments service.
func NewService(repo Repository, tx database.TxManager) *Service {
	return &Service{repo: repo, tx: tx, now: time.Now}
}

// Record validates the contact, inserts the payment, allocates it across
// that contact's outstanding invoices per D8, and recomputes every touched
// invoice's paid_amount/status — all inside one transaction. A failure at
// any step leaves zero rows written.
//
// The payment is anchored on the CONTACT's own owning teacher and center,
// not necessarily sc: an owner recording a member's payment must not
// silently reassign it to the owner (the same parent-anchor precedent
// billing's periodScope and attendance's roster checks apply). For a
// non-owner sc these are always the same value — ResolveContactScope already
// refuses a contact outside sc's own tenancy in that case.
func (s *Service) Record(ctx context.Context, sc authctx.Scope, req RecordPaymentRequest) (*PaymentDetail, error) {
	ownerScope, ok, err := s.repo.ResolveContactScope(ctx, sc, req.ContactID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if !ok {
		return nil, apperror.NotFound("contact")
	}

	receivedOn, err := time.Parse(dateLayout, req.ReceivedOn)
	if err != nil {
		return nil, apperror.Invalid("received_on must be a date in YYYY-MM-DD form", map[string]string{"received_on": "is invalid"})
	}

	payment := &Payment{
		ID:            id.New(),
		TeacherID:     ownerScope.TeacherID,
		CenterID:      ownerScope.CenterID,
		ContactID:     req.ContactID,
		Amount:        req.Amount,
		Method:        req.Method,
		ReceivedOn:    receivedOn,
		ReferenceCode: req.ReferenceCode,
		Note:          req.Note,
	}

	var unallocated int64
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreatePayment(txCtx, payment); err != nil {
			return err
		}

		candidates, err := s.repo.CandidateInvoices(txCtx, ownerScope, req.ContactID)
		if err != nil {
			return err
		}

		var allocs []Allocation
		allocs, unallocated = Allocate(req.Amount, candidates)

		rows := make([]PaymentAllocation, 0, len(allocs))
		for _, a := range allocs {
			rows = append(rows, PaymentAllocation{
				ID:          id.New(),
				TeacherID:   ownerScope.TeacherID,
				CenterID:    ownerScope.CenterID,
				PaymentID:   payment.ID,
				InvoiceID:   a.InvoiceID,
				Amount:      a.Amount,
				AllocatedBy: AllocatedAuto,
			})
		}
		if err := s.repo.InsertAllocations(txCtx, rows); err != nil {
			return err
		}

		for _, a := range allocs {
			if err := s.repo.RecalcInvoicePaid(txCtx, ownerScope, a.InvoiceID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, translate(err)
	}

	allocRows, err := s.repo.ListAllocations(ctx, ownerScope, payment.ID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return &PaymentDetail{Payment: *payment, Allocations: allocRows, UnallocatedAmount: unallocated}, nil
}

// Get returns one payment with its allocation breakdown.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) (*PaymentDetail, error) {
	p, err := s.repo.GetPayment(ctx, sc, paymentID)
	if err != nil {
		return nil, translate(err)
	}
	allocRows, err := s.repo.ListAllocations(ctx, sc, paymentID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return &PaymentDetail{Payment: *p, Allocations: allocRows, UnallocatedAmount: sumUnallocated(p.Amount, allocRows)}, nil
}

// List returns a page of payments, each with its allocation breakdown
// fetched in one batched query rather than one round trip per payment.
func (s *Service) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]PaymentDetail, int64, error) {
	rows, total, err := s.repo.ListPayments(ctx, sc, filter, p)
	if err != nil {
		return nil, 0, apperror.Internal(err)
	}
	if len(rows) == 0 {
		return []PaymentDetail{}, total, nil
	}

	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	allocRows, err := s.repo.ListAllocationsForPayments(ctx, sc, ids)
	if err != nil {
		return nil, 0, apperror.Internal(err)
	}
	byPayment := make(map[uuid.UUID][]AllocationRow, len(rows))
	for _, a := range allocRows {
		byPayment[a.PaymentID] = append(byPayment[a.PaymentID], a)
	}

	details := make([]PaymentDetail, len(rows))
	for i, r := range rows {
		allocs := byPayment[r.ID]
		details[i] = PaymentDetail{Payment: r, Allocations: allocs, UnallocatedAmount: sumUnallocated(r.Amount, allocs)}
	}
	return details, total, nil
}

// sumUnallocated re-derives unallocated_amount for a read (as opposed to
// Record, which already knows it) as payment.amount minus the sum of its
// allocations.
func sumUnallocated(amount int64, allocs []AllocationRow) int64 {
	sum := amount
	for _, a := range allocs {
		sum -= a.Amount
	}
	if sum < 0 {
		sum = 0
	}
	return sum
}

// translate maps domain errors onto the API error contract.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrPaymentNotFound):
		return apperror.NotFound("payment")
	default:
		return apperror.From(err)
	}
}
