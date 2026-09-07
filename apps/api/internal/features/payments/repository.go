package payments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows the payment list.
type ListFilter struct {
	ContactID uuid.UUID
	// PeriodID filters to payments with at least one allocation landing on
	// an invoice of this billing period.
	PeriodID     uuid.UUID
	ReceivedFrom *time.Time
	ReceivedTo   *time.Time
}

// AllocationRow is one payment_allocations row joined with the target
// invoice's identifying and money fields — everything AllocationResponse
// needs so no client ever has to reimplement the D8 rule to know which child
// a đồng landed on.
type AllocationRow struct {
	PaymentID   uuid.UUID
	InvoiceID   uuid.UUID
	StudentID   uuid.UUID
	StudentName string
	PeriodID    uuid.UUID
	Amount      int64
	AllocatedBy string
	TotalDue    int64
	PaidAmount  int64
}

// InvoiceRow is the slice of an invoice row Reallocate's validation and
// AutoAllocateRemainder's target search need: enough to check tenancy,
// contact ownership, status, and remaining room without importing billing's
// Invoice model (the two features deliberately share only the invoices
// table, never a Go type).
type InvoiceRow struct {
	ID         uuid.UUID
	ContactID  uuid.UUID
	Status     string
	TotalDue   int64
	PaidAmount int64
}

// Repository is the persistence contract for payments and their allocations;
// the service depends on this interface, tests supply a fake.
type Repository interface {
	// CreatePayment writes p as given — not scope-filtered: p already carries
	// its own TeacherID/CenterID, stamped by the caller before this is
	// invoked.
	CreatePayment(ctx context.Context, p *Payment) error
	GetPayment(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) (*Payment, error)
	ListPayments(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Payment, int64, error)
	// LockPayment loads paymentID FOR UPDATE inside the caller's transaction —
	// the read every correction path (Reallocate, Reverse,
	// AutoAllocateRemainder) starts with, so two corrections racing on the
	// same payment serialise instead of both acting on a stale
	// reversed_at/reverses_payment_id.
	LockPayment(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) (*Payment, error)
	// CandidateInvoices returns contactID's invoices eligible for payment
	// (status issued/partially_paid, total_due > paid_amount), locked
	// FOR UPDATE so two concurrent payments for one contact serialise instead
	// of both reading the same stale paid_amount. a names the contact's own
	// owning teacher — Record's contact anchor, or Reallocate/Reverse/
	// AutoAllocateRemainder's payment anchor — never the caller, so the
	// candidate set can never widen to another teacher's invoices.
	CandidateInvoices(ctx context.Context, a authctx.Anchor, contactID uuid.UUID) ([]Candidate, error)
	// InvoicesByIDs loads every invoice in ids, FOR UPDATE, for Reallocate's
	// validation and write path — the same lock discipline CandidateInvoices
	// uses so a reallocation can never race a concurrent payment for the same
	// contact. Missing ids are simply absent from the result; the caller
	// decides whether that is a validation failure. a is always the
	// payment's own owner anchor, never the caller.
	InvoicesByIDs(ctx context.Context, a authctx.Anchor, ids []uuid.UUID) ([]InvoiceRow, error)
	// InsertAllocations bulk-inserts payment_allocations rows, merging into
	// an existing (payment_id, invoice_id) row by adding to its amount rather
	// than failing uq_payment_allocations — AutoAllocateRemainder relies on
	// this to top up an invoice this same payment already partially covers.
	// A no-op for an empty slice (a payment that allocated to nothing). Not
	// scope-filtered: every row already carries its own TeacherID/CenterID,
	// inherited from its payment.
	InsertAllocations(ctx context.Context, rows []PaymentAllocation) error
	// DeleteAllocations removes every payment_allocations row for paymentID —
	// the only delete anywhere in this package. It is safe because an
	// allocation is a link between two preserved facts (the payment row and
	// the invoice row), not a fact itself: replacing a split loses no
	// financial history, the payment amount and the invoice both stay
	// intact, and RecalcInvoicePaid always re-derives paid_amount from
	// whatever allocation rows exist at read time.
	// DeleteAllocations always runs under the payment's own owner anchor,
	// never the caller — see the invariant on the interface doc above.
	DeleteAllocations(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID) error
	// AllocationsByPayment returns paymentID's raw allocation rows (no
	// invoice join) — Reallocate uses it to know what a reallocation is
	// replacing, Reverse uses it to mirror a payment's split onto its
	// counter-entry, and AutoAllocateRemainder uses it to sum the payment's
	// already-allocated amount. a is always the payment's own owner anchor.
	AllocationsByPayment(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID) ([]PaymentAllocation, error)
	// MarkReversed stamps reversed_at on paymentID. a is the original
	// payment's own owner anchor, resolved by the caller's Scope after
	// LockPayment has already gated the write.
	MarkReversed(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID, at time.Time) error
	// RecalcInvoicePaid re-derives invoiceID's paid_amount and status from the
	// sum of its non-reversed allocations minus its reversed ones — never an
	// increment, so it can never drift and re-running it is a no-op. a is
	// always the touched payment's own owner anchor, never the caller's, so
	// an owner's oversight recompute never silently widens to another
	// teacher's invoice.
	RecalcInvoicePaid(ctx context.Context, a authctx.Anchor, invoiceID uuid.UUID) error
	// ResolveContactAnchor reports whether contactID belongs to sc's center —
	// any teacher's contact, because the right to record a payment is the
	// route permission (payments.create), not contact ownership — regardless
	// of soft-delete: a family whose last child left must still be able to
	// settle a carried debt (D4's soft-deleted-contact exception, extended
	// here from the collection board to the write path). On success it
	// returns the contact's own owning anchor (TeacherID from
	// contacts.teacher_id, CenterID always sc.CenterID) — Record uses this to
	// anchor a payment on the contact's own teacher, not the caller's.
	ResolveContactAnchor(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (authctx.Anchor, bool, error)
	// ListAllocations returns one payment's allocation breakdown, joined with
	// each target invoice's current money fields, narrowed to sc's own rows
	// unless sc sees payments center-wide.
	ListAllocations(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) ([]AllocationRow, error)
	// AllocationsOf is ListAllocations' anchor-only sibling: Record's
	// read-back after anchoring the payment on the contact's owning teacher,
	// never the recording caller's own visibility.
	AllocationsOf(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID) ([]AllocationRow, error)
	// ListAllocationsForPayments batches ListAllocations over several
	// payments in one round trip — the list endpoint's N+1 guard.
	ListAllocationsForPayments(ctx context.Context, sc authctx.Scope, paymentIDs []uuid.UUID) ([]AllocationRow, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// readScoped returns a payments READ query bound to one center, further
// narrowed to one teacher's own rows unless the caller sees payments
// center-wide (the owner, or a payments.view_all holder).
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("payments.center_id = ?", sc.CenterID)
	return r.readNarrow(q, sc, "payments.teacher_id")
}

// writeScoped is readScoped's WRITE sibling: only the owner reaches another
// teacher's payment. payments.view_all is a visibility key — it widens reads
// and nothing else — so lock and reverse resolve through here.
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("payments.center_id = ?", sc.CenterID)
	if !sc.WriteWide() {
		q = q.Where("payments.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// allocationAnchored pins a payment_allocations query to exactly a's center
// and teacher, never widened — every caller runs inside a reallocation or
// reversal transaction and passes the touched payment's own owner anchor, so
// there is no permission to consult here; the listing reads use readNarrow
// against the caller's Scope instead.
func (r *gormRepository) allocationAnchored(ctx context.Context, a authctx.Anchor) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Where("payment_allocations.center_id = ? AND payment_allocations.teacher_id = ?", a.CenterID, a.TeacherID)
}

// readNarrow appends the own-rows filter on col unless the caller sees
// payments center-wide. It is the READ template for the allocation and
// invoice joins; writes narrow on WriteWide instead and never call it.
func (r *gormRepository) readNarrow(q *gorm.DB, sc authctx.Scope, col string) *gorm.DB {
	if !sc.CenterWideFor(authctx.PermPaymentsViewAll) {
		q = q.Where(col+" = ?", sc.TeacherID)
	}
	return q
}

func (r *gormRepository) CreatePayment(ctx context.Context, p *Payment) error {
	return database.FromContext(ctx, r.db).Create(p).Error
}

func (r *gormRepository) GetPayment(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) (*Payment, error) {
	var p Payment
	err := r.readScoped(ctx, sc).Where("payments.id = ?", paymentID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) LockPayment(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) (*Payment, error) {
	var p Payment
	err := r.writeScoped(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("payments.id = ?", paymentID).
		Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) ListPayments(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Payment, int64, error) {
	q := r.readScoped(ctx, sc).Model(&Payment{})
	if filter.ContactID != uuid.Nil {
		q = q.Where("payments.contact_id = ?", filter.ContactID)
	}
	if filter.PeriodID != uuid.Nil {
		sub := database.FromContext(ctx, r.db).
			Table("payment_allocations pa").
			Select("1").
			Joins("JOIN invoices i ON i.id = pa.invoice_id AND i.center_id = pa.center_id").
			Where("pa.payment_id = payments.id AND pa.center_id = ? AND i.period_id = ?", sc.CenterID, filter.PeriodID)
		sub = r.readNarrow(sub, sc, "pa.teacher_id")
		q = q.Where("EXISTS (?)", sub)
	}
	if filter.ReceivedFrom != nil {
		q = q.Where("payments.received_on >= ?", *filter.ReceivedFrom)
	}
	if filter.ReceivedTo != nil {
		q = q.Where("payments.received_on <= ?", *filter.ReceivedTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Payment
	if err := q.Scopes(p.Scope).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// candidateInvoicesQuery is the D8 candidate set: issued/partially_paid
// invoices of one contact that still owe money, joined with their billing
// period's start date and the earliest class start date among their lines.
// FOR UPDATE OF i locks only the invoices table rows — the lateral join's
// aggregate lives inside its own derived table, so it never collides with
// Postgres's "FOR UPDATE cannot be applied to the nullable side of an outer
// join" restriction, which only concerns columns from the locked relation.
//
// The (? OR i.teacher_id = ?) pair keeps this one raw statement shared with
// callers that carry no widening authority at all: CandidateInvoices always
// runs under an Anchor, which has no owner/WriteWide concept, so the method
// below binds the OR arm's placeholder to a literal false and never widens —
// the SQL text itself never forks between an owner and a member caller.
//
// Rows are ordered by invoice id, not by the D8 sort keys: the caller re-sorts
// candidates through the D8 comparator before allocating, so this ORDER BY only
// governs the order locks are acquired. Locking every contact-scoped write path
// (recording, reallocation, reversal) in the same invoice-id order keeps two
// concurrent operations on one contact from grabbing the same two invoices in
// opposite orders and deadlocking.
const candidateInvoicesQuery = `
	SELECT i.id AS invoice_id,
	       bp.period_start AS period_start,
	       lc.earliest_class_start AS earliest_class_start,
	       i.opening_balance AS opening_balance,
	       i.total_due AS total_due,
	       i.paid_amount AS paid_amount
	FROM invoices i
	JOIN billing_periods bp ON bp.id = i.period_id AND bp.center_id = i.center_id
	LEFT JOIN LATERAL (
	    SELECT min(cl.start_date) AS earliest_class_start
	    FROM invoice_lines il
	    JOIN enrollments e ON e.id = il.enrollment_id
	    JOIN classes    cl ON cl.id = e.class_id
	    WHERE il.invoice_id = i.id
	) lc ON true
	WHERE i.center_id = ? AND (? OR i.teacher_id = ?) AND i.contact_id = ?
	  AND i.status IN ('issued', 'partially_paid')
	  AND i.total_due > i.paid_amount
	ORDER BY i.id
	FOR UPDATE OF i
`

func (r *gormRepository) CandidateInvoices(ctx context.Context, a authctx.Anchor, contactID uuid.UUID) ([]Candidate, error) {
	var rows []Candidate
	err := database.FromContext(ctx, r.db).
		Raw(candidateInvoicesQuery, a.CenterID, false, a.TeacherID, contactID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// insertAllocationsOnConflict merges an insert into an existing
// (payment_id, invoice_id) row by adding to its amount instead of failing
// uq_payment_allocations. Record's and Reallocate's inserts never actually
// collide (Record targets a brand-new payment id; Reallocate deletes the
// payment's old rows first), so the clause is a no-op for them and only
// AutoAllocateRemainder's top-up insert ever exercises the merge.
var insertAllocationsOnConflict = clause.OnConflict{
	Columns: []clause.Column{{Name: "payment_id"}, {Name: "invoice_id"}},
	DoUpdates: clause.Assignments(map[string]any{
		"amount": gorm.Expr("payment_allocations.amount + excluded.amount"),
	}),
}

func (r *gormRepository) InsertAllocations(ctx context.Context, rows []PaymentAllocation) error {
	if len(rows) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).
		Clauses(insertAllocationsOnConflict).
		Create(&rows).Error
}

func (r *gormRepository) InvoicesByIDs(ctx context.Context, a authctx.Anchor, ids []uuid.UUID) ([]InvoiceRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := database.FromContext(ctx, r.db).
		Table("invoices").
		Select("id, contact_id, status, total_due, paid_amount").
		Where("center_id = ? AND id IN ? AND teacher_id = ?", a.CenterID, ids, a.TeacherID)
	var rows []InvoiceRow
	err := q.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id").Find(&rows).Error
	return rows, err
}

// DeleteAllocations removes every payment_allocations row for paymentID —
// see the invariant documented on the Repository interface method: an
// allocation is a link, not a fact, so deleting it here loses no financial
// history.
func (r *gormRepository) DeleteAllocations(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID) error {
	return r.allocationAnchored(ctx, a).
		Where("payment_allocations.payment_id = ?", paymentID).
		Delete(&PaymentAllocation{}).Error
}

func (r *gormRepository) AllocationsByPayment(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID) ([]PaymentAllocation, error) {
	var rows []PaymentAllocation
	err := r.allocationAnchored(ctx, a).
		Where("payment_allocations.payment_id = ?", paymentID).
		Order("payment_allocations.invoice_id").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) MarkReversed(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID, at time.Time) error {
	return database.FromContext(ctx, r.db).
		Model(&Payment{}).
		Where("payments.center_id = ? AND payments.teacher_id = ? AND payments.id = ?", a.CenterID, a.TeacherID, paymentID).
		Update("reversed_at", at).Error
}

// recalcInvoicePaidQuery re-derives paid_amount from scratch — the sum of
// non-reversal allocations minus reversal allocations — and moves status
// among issued/partially_paid/paid using the same rule billing's own
// paid_amount-derived recompute uses, so the two packages can never disagree
// about what "paid" means. void and draft are left untouched.
//
// target pins a single-row anchor for invoiceID before the LEFT JOIN: an
// invoice that currently holds zero allocation rows (Reallocate dropped its
// only line) must still be matched and zeroed out, not skipped. A plain
// correlated subquery in FROM has no join predicate against i, so when it
// produces zero rows — exactly the all-allocations-deleted case — the UPDATE
// silently touches nothing and the invoice is left at its stale status.
//
// The trailing (? OR i.teacher_id = ?) mirrors candidateInvoicesQuery's
// shared-SQL trick: RecalcInvoicePaid always runs under an Anchor, so the Go
// method binds the OR arm to a literal false and never widens.
const recalcInvoicePaidQuery = `
	UPDATE invoices i SET
	  paid_amount = COALESCE(x.paid, 0),
	  status = CASE
	      WHEN i.status IN ('void', 'draft') THEN i.status
	      WHEN COALESCE(x.paid,0) >= i.total_due AND i.total_due > 0 THEN 'paid'
	      WHEN COALESCE(x.paid,0) > 0 THEN 'partially_paid'
	      ELSE 'issued'
	  END,
	  updated_at = now()
	FROM (SELECT ?::uuid AS invoice_id) target
	LEFT JOIN (
	  SELECT pa.invoice_id,
	         SUM(CASE WHEN p.reverses_payment_id IS NULL THEN pa.amount ELSE -pa.amount END) AS paid
	  FROM payment_allocations pa
	  JOIN payments p ON p.id = pa.payment_id
	  WHERE pa.invoice_id = ?
	  GROUP BY pa.invoice_id
	) x ON x.invoice_id = target.invoice_id
	WHERE i.id = target.invoice_id AND i.center_id = ? AND (? OR i.teacher_id = ?)
`

func (r *gormRepository) RecalcInvoicePaid(ctx context.Context, a authctx.Anchor, invoiceID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).
		Exec(recalcInvoicePaidQuery, invoiceID, invoiceID, a.CenterID, false, a.TeacherID)
	return res.Error
}

func (r *gormRepository) ResolveContactAnchor(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (authctx.Anchor, bool, error) {
	// Scanned into a struct field, never a bare uuid.UUID: GORM's raw-scalar
	// scan path treats a bare uuid.UUID destination as a 16-element array and
	// scans byte-by-byte instead of as one column.
	var rows []struct{ TeacherID uuid.UUID }
	// Center-only on purpose: recording a payment is gated by the route
	// permission (payments.create), and the payment itself anchors on the
	// contact's own teacher below, so which teacher created the contact does
	// not decide who may collect for it. No visibility key is consulted.
	q := database.FromContext(ctx, r.db).
		Table("contacts").
		Select("teacher_id").
		Where("center_id = ? AND id = ?", sc.CenterID, contactID)
	if err := q.Find(&rows).Error; err != nil {
		return authctx.Anchor{}, false, err
	}
	if len(rows) == 0 {
		return authctx.Anchor{}, false, nil
	}
	return sc.AnchorTo(rows[0].TeacherID), true, nil
}

// allocationRowSelect is the shared projection ListAllocations,
// AllocationsOf, and ListAllocationsForPayments join onto payment_allocations.
const allocationRowSelect = `pa.payment_id AS payment_id,
	pa.invoice_id AS invoice_id,
	i.student_id AS student_id,
	i.student_name AS student_name,
	i.period_id AS period_id,
	pa.amount AS amount,
	pa.allocated_by AS allocated_by,
	i.total_due AS total_due,
	i.paid_amount AS paid_amount`

// allocationsQuery is the shared base ListAllocations and AllocationsOf join
// onto payment_allocations for one payment; each caller narrows to its own
// teacher filter afterward — a caller's Scope through readNarrow, an Anchor
// through an exact equality.
func (r *gormRepository) allocationsQuery(ctx context.Context, centerID, paymentID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Table("payment_allocations pa").
		Select(allocationRowSelect).
		Joins("JOIN invoices i ON i.id = pa.invoice_id AND i.center_id = pa.center_id").
		Where("pa.center_id = ? AND pa.payment_id = ?", centerID, paymentID)
}

func (r *gormRepository) ListAllocations(ctx context.Context, sc authctx.Scope, paymentID uuid.UUID) ([]AllocationRow, error) {
	q := r.readNarrow(r.allocationsQuery(ctx, sc.CenterID, paymentID), sc, "pa.teacher_id")
	var rows []AllocationRow
	err := q.Order("i.student_name, pa.created_at").Find(&rows).Error
	return rows, err
}

func (r *gormRepository) AllocationsOf(ctx context.Context, a authctx.Anchor, paymentID uuid.UUID) ([]AllocationRow, error) {
	q := r.allocationsQuery(ctx, a.CenterID, paymentID).Where("pa.teacher_id = ?", a.TeacherID)
	var rows []AllocationRow
	err := q.Order("i.student_name, pa.created_at").Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ListAllocationsForPayments(ctx context.Context, sc authctx.Scope, paymentIDs []uuid.UUID) ([]AllocationRow, error) {
	if len(paymentIDs) == 0 {
		return nil, nil
	}
	q := database.FromContext(ctx, r.db).
		Table("payment_allocations pa").
		Select(allocationRowSelect).
		Joins("JOIN invoices i ON i.id = pa.invoice_id AND i.center_id = pa.center_id").
		Where("pa.center_id = ? AND pa.payment_id IN ?", sc.CenterID, paymentIDs)
	q = r.readNarrow(q, sc, "pa.teacher_id")
	var rows []AllocationRow
	err := q.Order("pa.payment_id, i.student_name, pa.created_at").Find(&rows).Error
	return rows, err
}
