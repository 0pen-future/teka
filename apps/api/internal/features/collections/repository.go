package collections

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// Filter narrows either view. Status must be "", StatusUnpaid, StatusPartial,
// or StatusPaid. ClassID is only honoured by ClassCollections, and required
// there — Service enforces that. Query substring-matches a contact's name in
// the contact view, or a student's or contact's name in the class view.
type Filter struct {
	Status  string
	ClassID *uuid.UUID
	Query   string
}

// Repository is the read-only data access surface behind the collection
// board. No method mutates a row: every query here scopes strictly by center,
// further narrowed to one teacher's own rows unless the caller owns the
// center, and never opens a transaction.
type Repository interface {
	PeriodExists(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (bool, error)
	ContactBalances(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter Filter, p pagination.Params) ([]ContactBalanceRow, int64, error)
	ClassCollections(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter Filter, p pagination.Params) ([]ClassCollectionRow, int64, error)
	PeriodSummary(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*SummaryResponse, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository builds the gorm-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// PeriodExists reports whether periodID belongs to sc's center — and, unless
// sc is the center's owner, to sc's own teacher — the sole tenant check every
// handler in this package runs before touching any reporting query. An owner
// matches any teacher's period in the center, mirroring the oversight rule
// every other scoped query in this package applies.
func (r *gormRepository) PeriodExists(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (bool, error) {
	var count int64
	q := database.FromContext(ctx, r.db).
		Table("billing_periods").
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", periodID, sc.CenterID)
	if !sc.IsOwner {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	err := q.Count(&count).Error
	return count > 0, err
}

// contactScanRow is the wire shape's exact projection off v_contact_balance
// joined to contacts, plus contacts.deleted_at to derive contact_archived.
// DeletedAt is scanned as a plain nullable timestamp rather than
// gorm.DeletedAt: that type makes GORM treat the destination struct as
// soft-delete-enabled and silently inject its own "deleted_at IS NULL"
// clause against this query's table alias, which breaks the one query in
// this package that must NOT filter it.
type contactScanRow struct {
	ContactID    uuid.UUID
	FullName     string
	Phone        string
	DeletedAt    *time.Time
	StudentCount int64
	TotalDue     int64
	TotalPaid    int64
	Outstanding  int64
}

// contactBalanceQuery builds the shared base for both the count and the data
// pass over v_contact_balance: the view already excludes void invoices, so
// this joins straight to contacts for the display name.
//
// vcb.center_id anchors the tenant filter unconditionally; vcb.teacher_id is
// only added when sc is not the center's owner, so an owner's read resolves
// every member's rows in one query instead of narrowing to their own.
//
// The contacts join deliberately omits "AND c.deleted_at IS NULL" — a
// contact who still owes money for a period must keep showing up here even
// after being archived, or a teacher chasing this period's collections would
// silently lose track of that debt. contact_archived flags the row instead
// of hiding it. Every other join in this file keeps its deleted_at filter.
func (r *gormRepository) contactBalanceQuery(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter Filter) *gorm.DB {
	q := database.FromContext(ctx, r.db).
		Table("v_contact_balance AS vcb").
		Joins("JOIN contacts c ON c.id = vcb.contact_id AND c.center_id = vcb.center_id").
		Where("vcb.center_id = ? AND vcb.period_id = ?", sc.CenterID, periodID)
	if !sc.IsOwner {
		q = q.Where("vcb.teacher_id = ?", sc.TeacherID)
	}
	if filter.Status != "" {
		q = q.Where(paymentStatusExpr("vcb.total_due", "vcb.total_paid")+" = ?", filter.Status)
	}
	if filter.Query != "" {
		q = q.Where("c.full_name ILIKE ?", "%"+filter.Query+"%")
	}
	return q
}

// contactSortColumns whitelists the by-contact view's sort key.
var contactSortColumns = map[string]string{
	"outstanding": "vcb.outstanding",
	"full_name":   "c.full_name",
	"total_due":   "vcb.total_due",
}

// ContactSortColumns exposes the by-contact sort whitelist to the handler.
func ContactSortColumns() map[string]string { return contactSortColumns }

// ContactBalances returns one row per contact who has at least one non-void
// invoice in periodID, merging every one of their children into a single
// balance — the default view for a teacher (or the center owner) chasing
// money by family.
func (r *gormRepository) ContactBalances(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter Filter, p pagination.Params) ([]ContactBalanceRow, int64, error) {
	var total int64
	if err := r.contactBalanceQuery(ctx, sc, periodID, filter).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var scanned []contactScanRow
	err := r.contactBalanceQuery(ctx, sc, periodID, filter).
		Select(`vcb.contact_id AS contact_id, c.full_name AS full_name, c.phone AS phone,
			c.deleted_at AS deleted_at, vcb.student_count AS student_count,
			vcb.total_due AS total_due, vcb.total_paid AS total_paid, vcb.outstanding AS outstanding`).
		Scopes(p.Scope).
		Find(&scanned).Error
	if err != nil {
		return nil, 0, err
	}

	contactIDs := make([]uuid.UUID, len(scanned))
	for i, sr := range scanned {
		contactIDs[i] = sr.ContactID
	}
	invoicesByContact, err := r.childInvoicesByContact(ctx, sc, periodID, contactIDs)
	if err != nil {
		return nil, 0, err
	}

	rows := make([]ContactBalanceRow, len(scanned))
	for i, sr := range scanned {
		invoices := invoicesByContact[sr.ContactID]
		if invoices == nil {
			invoices = []ContactChildInvoiceRow{}
		}
		rows[i] = ContactBalanceRow{
			ContactID:       sr.ContactID,
			FullName:        sr.FullName,
			Phone:           sr.Phone,
			ContactArchived: sr.DeletedAt != nil,
			StudentCount:    sr.StudentCount,
			TotalDue:        sr.TotalDue,
			TotalPaid:       sr.TotalPaid,
			Outstanding:     sr.Outstanding,
			PaymentStatus:   derivePaymentStatus(sr.TotalDue, sr.TotalPaid),
			Invoices:        invoices,
		}
	}
	return rows, total, nil
}

// childInvoicesByContact batch-loads every non-void invoice for the given
// contacts in one query, avoiding an N+1 round trip per page row.
func (r *gormRepository) childInvoicesByContact(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]ContactChildInvoiceRow, error) {
	out := make(map[uuid.UUID][]ContactChildInvoiceRow, len(contactIDs))
	if len(contactIDs) == 0 {
		return out, nil
	}

	type scanRow struct {
		ContactID   uuid.UUID
		InvoiceID   uuid.UUID
		StudentName string
		TotalDue    int64
		PaidAmount  int64
	}
	q := database.FromContext(ctx, r.db).
		Table("invoices").
		Select("contact_id AS contact_id, id AS invoice_id, student_name AS student_name, total_due AS total_due, paid_amount AS paid_amount").
		Where("center_id = ? AND period_id = ? AND contact_id IN ? AND status <> ?", sc.CenterID, periodID, contactIDs, statusVoid)
	if !sc.IsOwner {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	var rows []scanRow
	err := q.Order("student_name").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, sr := range rows {
		out[sr.ContactID] = append(out[sr.ContactID], ContactChildInvoiceRow{
			InvoiceID:   sr.InvoiceID,
			StudentName: sr.StudentName,
			TotalDue:    sr.TotalDue,
			PaidAmount:  sr.PaidAmount,
			Outstanding: sr.TotalDue - sr.PaidAmount,
		})
	}
	return out, nil
}

// classScanRow is the wire shape's exact projection for the by-class view.
type classScanRow struct {
	InvoiceID             uuid.UUID
	StudentID             uuid.UUID
	StudentName           string
	ContactID             uuid.UUID
	ContactName           string
	ClassName             string
	BillableCount         int
	AbsentCount           int
	LineAmount            int64
	InvoiceOpeningBalance int64
	InvoiceTotalDue       int64
	InvoicePaidAmount     int64
	InvoiceOutstanding    int64
}

// classSortColumns whitelists the by-class view's sort key.
var classSortColumns = map[string]string{
	"student_name": "i.student_name",
	"outstanding":  "invoice_outstanding",
}

// ClassSortColumns exposes the by-class sort whitelist to the handler.
func ClassSortColumns() map[string]string { return classSortColumns }

// classCollectionsQuery builds the shared base for both the count and the
// data pass over one class's billing lines for the period. filter.ClassID is
// required by the caller (Service enforces it); this still guards against a
// nil pointer for a repository used directly.
//
// i.center_id anchors the tenant filter unconditionally; i.teacher_id is
// only added when sc is not the center's owner, matching contactBalanceQuery.
func (r *gormRepository) classCollectionsQuery(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter Filter) *gorm.DB {
	q := database.FromContext(ctx, r.db).
		Table("invoice_lines AS il").
		Joins("JOIN invoices i ON i.id = il.invoice_id AND i.center_id = il.center_id").
		Joins("JOIN enrollments e ON e.id = il.enrollment_id AND e.center_id = il.center_id AND e.deleted_at IS NULL").
		Where("i.center_id = ? AND i.period_id = ? AND i.status <> ?", sc.CenterID, periodID, statusVoid)
	if !sc.IsOwner {
		q = q.Where("i.teacher_id = ?", sc.TeacherID)
	}
	if filter.ClassID != nil {
		q = q.Where("e.class_id = ?", *filter.ClassID)
	}
	if filter.Status != "" {
		q = q.Where(paymentStatusExpr("i.total_due", "i.paid_amount")+" = ?", filter.Status)
	}
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		q = q.Where("(i.student_name ILIKE ? OR i.contact_name ILIKE ?)", like, like)
	}
	return q
}

// ClassCollections returns one row per invoice line for the requested class
// within periodID — the view a teacher (or the center owner) standing in
// front of one room uses to see which child is short and by how much.
func (r *gormRepository) ClassCollections(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter Filter, p pagination.Params) ([]ClassCollectionRow, int64, error) {
	var total int64
	if err := r.classCollectionsQuery(ctx, sc, periodID, filter).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var scanned []classScanRow
	err := r.classCollectionsQuery(ctx, sc, periodID, filter).
		Select(`i.id AS invoice_id, i.student_id AS student_id, i.student_name AS student_name,
			i.contact_id AS contact_id, i.contact_name AS contact_name,
			il.class_name AS class_name, il.billable_count AS billable_count, il.absent_count AS absent_count,
			il.amount AS line_amount, i.opening_balance AS invoice_opening_balance,
			i.total_due AS invoice_total_due, i.paid_amount AS invoice_paid_amount,
			(i.total_due - i.paid_amount) AS invoice_outstanding`).
		Scopes(p.Scope).
		Find(&scanned).Error
	if err != nil {
		return nil, 0, err
	}

	rows := make([]ClassCollectionRow, len(scanned))
	for i, sr := range scanned {
		rows[i] = ClassCollectionRow{
			InvoiceID:             sr.InvoiceID,
			StudentID:             sr.StudentID,
			StudentName:           sr.StudentName,
			ContactID:             sr.ContactID,
			ContactName:           sr.ContactName,
			ClassName:             sr.ClassName,
			BillableCount:         sr.BillableCount,
			AbsentCount:           sr.AbsentCount,
			LineAmount:            sr.LineAmount,
			InvoiceOpeningBalance: sr.InvoiceOpeningBalance,
			InvoiceTotalDue:       sr.InvoiceTotalDue,
			InvoicePaidAmount:     sr.InvoicePaidAmount,
			InvoiceOutstanding:    sr.InvoiceOutstanding,
			PaymentStatus:         derivePaymentStatus(sr.InvoiceTotalDue, sr.InvoicePaidAmount),
		}
	}
	return rows, total, nil
}

// PeriodSummary aggregates periodID's non-void invoices, the paid/unpaid/
// partial split of its contacts, and the period's contacts' unallocated
// credit — money already received that no allocation has touched.
func (r *gormRepository) PeriodSummary(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*SummaryResponse, error) {
	var totals struct {
		StudentCount     int64
		ContactCount     int64
		TotalDue         int64
		TotalPaid        int64
		TotalOutstanding int64
	}
	totalsQ := database.FromContext(ctx, r.db).
		Table("invoices").
		Select(`COUNT(DISTINCT student_id) AS student_count, COUNT(DISTINCT contact_id) AS contact_count,
			COALESCE(SUM(total_due),0) AS total_due, COALESCE(SUM(paid_amount),0) AS total_paid,
			COALESCE(SUM(total_due - paid_amount),0) AS total_outstanding`).
		Where("center_id = ? AND period_id = ? AND status <> ?", sc.CenterID, periodID, statusVoid)
	if !sc.IsOwner {
		totalsQ = totalsQ.Where("teacher_id = ?", sc.TeacherID)
	}
	if err := totalsQ.Take(&totals).Error; err != nil {
		return nil, err
	}

	var statusCounts struct {
		Paid    int64
		Unpaid  int64
		Partial int64
	}
	statusQ := database.FromContext(ctx, r.db).
		Table("v_contact_balance").
		Select(`COUNT(*) FILTER (WHERE outstanding <= 0) AS paid,
			COUNT(*) FILTER (WHERE outstanding > 0 AND total_paid = 0) AS unpaid,
			COUNT(*) FILTER (WHERE outstanding > 0 AND total_paid > 0) AS partial`).
		Where("center_id = ? AND period_id = ?", sc.CenterID, periodID)
	if !sc.IsOwner {
		statusQ = statusQ.Where("teacher_id = ?", sc.TeacherID)
	}
	if err := statusQ.Take(&statusCounts).Error; err != nil {
		return nil, err
	}

	// unallocated_credit sums, over every contact who has an invoice in this
	// period, amount minus allocated per payment — restricted to payments
	// that are neither reversed nor themselves a reversal entry, the same
	// pair of flags Reverse() uses to keep a corrected payment from counting
	// twice (repository.go's RecalcInvoicePaid in the payments package nets
	// a reversal's allocations the same way for the identical reason).
	//
	// Every fragment below repeats the (? OR x.teacher_id = ?) pair so
	// sc.IsOwner short-circuits the teacher_id check via SQL OR — the same
	// trick payments' candidateInvoicesQuery and billing's
	// RecalcInvoiceTotals use — because this single statement joins a
	// subquery and a correlated subquery, each independently tenant-scoped,
	// and building that out of conditional Go query fragments would mean
	// three near-duplicate raw strings instead of one.
	var unallocated int64
	row := database.FromContext(ctx, r.db).
		Table("payments AS p").
		Select("COALESCE(SUM(p.amount - COALESCE(alloc.total, 0)), 0)").
		Joins(`LEFT JOIN (
			SELECT payment_id, SUM(amount) AS total FROM payment_allocations
			WHERE center_id = ? AND (? OR teacher_id = ?) GROUP BY payment_id
		) alloc ON alloc.payment_id = p.id`, sc.CenterID, sc.IsOwner, sc.TeacherID).
		Where(`p.center_id = ? AND (? OR p.teacher_id = ?) AND p.reversed_at IS NULL AND p.reverses_payment_id IS NULL
			AND p.contact_id IN (
				SELECT DISTINCT contact_id FROM v_contact_balance
				WHERE center_id = ? AND (? OR teacher_id = ?) AND period_id = ?
			)`,
			sc.CenterID, sc.IsOwner, sc.TeacherID,
			sc.CenterID, sc.IsOwner, sc.TeacherID, periodID).
		Row()
	if err := row.Scan(&unallocated); err != nil {
		return nil, err
	}

	return &SummaryResponse{
		StudentCount:        totals.StudentCount,
		ContactCount:        totals.ContactCount,
		TotalDue:            totals.TotalDue,
		TotalPaid:           totals.TotalPaid,
		TotalOutstanding:    totals.TotalOutstanding,
		PaidContactCount:    statusCounts.Paid,
		UnpaidContactCount:  statusCounts.Unpaid,
		PartialContactCount: statusCounts.Partial,
		UnallocatedCredit:   unallocated,
	}, nil
}
