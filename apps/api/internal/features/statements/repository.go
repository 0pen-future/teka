package statements

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/pagination"
)

// Row is a statement plus the contact display fields the teacher-facing
// endpoints need, produced in one query so listing a period's statements
// never becomes an N+1.
type Row struct {
	Statement       `gorm:"embedded"`
	ContactFullName string
	ContactPhone    string
}

// TargetContact is one contact eligible for a statement in a period: they
// have at least one non-void invoice there. Carries the display fields
// Generate's response needs so building it never requires a second,
// per-contact lookup.
type TargetContact struct {
	ContactID uuid.UUID
	FullName  string
	Phone     string
}

// Repository is the persistence contract for statements; the service depends
// on this interface, tests supply a fake.
//
// Every method takes teacherID and scopes by it, with one deliberate
// exception: GetByTokenHash. The public link a parent opens (a later phase)
// carries only the opaque token, never a teacher id, so it is the sole
// sanctioned way to resolve a statement without one — token_hash is globally
// unique (uq_statements_token), so the lookup stays exact-match, never a
// tenant-wide scan.
type Repository interface {
	// GetPeriodStatus reads one billing period's status, teacher-scoped —
	// Generate's closed-period precondition and List/Get's existence check
	// for a period id.
	GetPeriodStatus(ctx context.Context, teacherID, periodID uuid.UUID) (string, error)
	// TargetContacts returns every contact with at least one non-void
	// invoice in periodID — Generate's candidate set.
	TargetContacts(ctx context.Context, teacherID, periodID uuid.UUID) ([]TargetContact, error)
	// ContactTotals reads v_contact_balance's total_due per contact for
	// periodID — the money Generate writes onto each statement.
	ContactTotals(ctx context.Context, teacherID, periodID uuid.UUID) (map[uuid.UUID]int64, error)
	// UpsertStatement writes one row on the natural key uq_statements
	// (contact_id, period_id) among non-deleted rows: inserts stmt as given
	// when absent, or refreshes only total_due/updated_at when a
	// not-yet-revoked row already exists — token_hash is never touched by an
	// update, so re-running Generate never rotates a link already sent to a
	// parent. An existing, already-revoked row is left completely untouched
	// (skippedRevoked=true) rather than resurrected. On return, stmt is
	// refreshed (RETURNING) to the persisted row's real values; comparing
	// its id against the id it was called with is how the caller tells
	// created apart from refreshed.
	UpsertStatement(ctx context.Context, teacherID uuid.UUID, stmt *Statement) (created, skippedRevoked bool, err error)
	// ListByPeriod returns a page of one period's statements with contact
	// display fields.
	ListByPeriod(ctx context.Context, teacherID, periodID uuid.UUID, p pagination.Params) ([]Row, int64, error)
	// GetByID returns one statement with contact display fields.
	GetByID(ctx context.Context, teacherID, statementID uuid.UUID) (*Row, error)
	// GetByTokenHash resolves a statement by its token's hash. See the
	// Repository doc comment: the one method in this package with no
	// teacherID parameter.
	GetByTokenHash(ctx context.Context, tokenHash []byte) (*Statement, error)
	// Revoke sets revoked_at on one statement. Idempotent: revoking an
	// already-revoked statement succeeds without changing revoked_at.
	// Returns ErrNotFound only when the statement is missing or belongs to
	// another teacher.
	Revoke(ctx context.Context, teacherID, statementID uuid.UUID) error

	// InvoicesWithLines returns every non-void invoice for contactID in
	// periodID, one row per invoice_lines row (an invoice with none still
	// produces one row, its line fields null) — the public view's child and
	// class snapshot figures in a single round trip, independent of how many
	// children or classes the family has.
	InvoicesWithLines(ctx context.Context, teacherID, contactID, periodID uuid.UUID) ([]InvoiceLineRow, error)
	// LiveSessions returns, for every enrollment billed in periodID, its
	// attendance-confirmed sessions whose date falls inside the period's own
	// date range — read live off attendance_records/class_sessions, not off
	// the invoice_lines snapshot, so a correction made after the statement
	// was generated shows up on the next view without regenerating anything.
	LiveSessions(ctx context.Context, teacherID, contactID, periodID uuid.UUID) ([]LiveSessionRow, error)
	// PeriodInvoiceLines returns every non-void invoice's lines for periodID
	// across every contact, one row per invoice_lines row (or, for an invoice
	// with none, one null-line placeholder row) — the period-wide counterpart
	// to InvoicesWithLines. This is what lets Service.PeriodFigures assemble
	// every contact's message figures for a bulk send in one round trip
	// instead of one query per contact.
	PeriodInvoiceLines(ctx context.Context, teacherID, periodID uuid.UUID) ([]InvoiceLineRow, error)
	// Adjustments returns both kinds of adjustment a child's statement can
	// show: one posted directly on one of this period's own invoices, and
	// one posted on a later invoice but whose source session falls inside
	// this period's date range (a post-close correction billing.
	// ReconcileSession carried forward — see the Carried field). The
	// teacher-authored free-text reason never appears in this projection; it
	// must never reach a public payload.
	Adjustments(ctx context.Context, teacherID, contactID, periodID uuid.UUID) ([]AdjustmentRow, error)
	// TouchView records one open of a statement's public link: increments
	// view_count, sets first_viewed_at once, and refreshes last_viewed_at.
	TouchView(ctx context.Context, teacherID, statementID uuid.UUID) error
}

// InvoiceLineRow is one invoice_lines row (or, for an invoice with none, one
// null-line placeholder row) joined with its parent invoice, that invoice's
// billing period label, and the student's display note — the exact
// projection buildPublicStatement needs to assemble one child's classes
// without a second round trip per line. Line* fields are nil together: the
// LEFT JOIN either matched a real line or none at all.
type InvoiceLineRow struct {
	InvoiceID uuid.UUID
	// ContactID identifies which family this invoice belongs to. Unused by
	// InvoicesWithLines' own caller (already contact-scoped by its WHERE
	// clause) but required by PeriodInvoiceLines, the period-wide sibling
	// query that groups rows across every contact in one pass — see
	// Service.PeriodFigures.
	ContactID       uuid.UUID
	StudentID       uuid.UUID
	StudentName     string
	ContactName     string
	DisplayNote     *string
	OpeningBalance  int64
	CurrentCharge   int64
	AdjustmentTotal int64
	TotalDue        int64
	PaidAmount      int64
	PeriodYear      int
	PeriodMonth     int

	LineID        *uuid.UUID
	EnrollmentID  *uuid.UUID
	ClassName     *string
	BillableCount *int
	AbsentCount   *int
	UnitPrice     *int64
	LineAmount    *int64
}

// LiveSessionRow is one attendance-confirmed session for one billed
// enrollment, read live rather than off the invoice_lines snapshot.
type LiveSessionRow struct {
	EnrollmentID     uuid.UUID
	SessionDate      time.Time
	SessionStatus    string
	AttendanceStatus string
	Billable         bool
}

// AdjustmentRow is one invoice_adjustments row relevant to a child's
// statement: either posted directly on one of this period's own invoices
// (Carried=false), or posted on a later invoice but sourced from a session
// that falls inside this period's date range (Carried=true, SessionDate
// set) — see the Repository.Adjustments doc comment.
type AdjustmentRow struct {
	StudentID       uuid.UUID
	Amount          int64
	SourceSessionID *uuid.UUID
	Carried         bool
	SessionDate     *time.Time
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// scoped returns a statements query bound to one tenant.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Model(&Statement{}).Where("statements.teacher_id = ?", teacherID)
}

// withContact joins in the contact display fields ListByPeriod/GetByID
// return alongside each statement.
func (r *gormRepository) withContact(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return r.scoped(ctx, teacherID).
		Select(`statements.*, contacts.full_name AS contact_full_name, contacts.phone AS contact_phone`).
		Joins("JOIN contacts ON contacts.id = statements.contact_id AND contacts.teacher_id = statements.teacher_id")
}

func (r *gormRepository) GetPeriodStatus(ctx context.Context, teacherID, periodID uuid.UUID) (string, error) {
	var statuses []string
	err := database.FromContext(ctx, r.db).
		Table("billing_periods").
		Where("id = ? AND teacher_id = ? AND deleted_at IS NULL", periodID, teacherID).
		Pluck("status", &statuses).Error
	if err != nil {
		return "", err
	}
	if len(statuses) == 0 {
		return "", ErrPeriodNotFound
	}
	return statuses[0], nil
}

func (r *gormRepository) TargetContacts(ctx context.Context, teacherID, periodID uuid.UUID) ([]TargetContact, error) {
	var rows []TargetContact
	err := database.FromContext(ctx, r.db).
		Table("invoices").
		Select(`DISTINCT invoices.contact_id AS contact_id, contacts.full_name AS full_name, contacts.phone AS phone`).
		Joins("JOIN contacts ON contacts.id = invoices.contact_id AND contacts.teacher_id = invoices.teacher_id").
		Where("invoices.teacher_id = ? AND invoices.period_id = ? AND invoices.status <> ?", teacherID, periodID, invoiceStatusVoid).
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ContactTotals(ctx context.Context, teacherID, periodID uuid.UUID) (map[uuid.UUID]int64, error) {
	type row struct {
		ContactID uuid.UUID
		TotalDue  int64
	}
	var rows []row
	err := database.FromContext(ctx, r.db).
		Table("v_contact_balance").
		Select("contact_id, total_due").
		Where("teacher_id = ? AND period_id = ?", teacherID, periodID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int64, len(rows))
	for _, rr := range rows {
		out[rr.ContactID] = rr.TotalDue
	}
	return out, nil
}

func (r *gormRepository) UpsertStatement(ctx context.Context, teacherID uuid.UUID, stmt *Statement) (created, skippedRevoked bool, err error) {
	stmt.TeacherID = teacherID
	candidateID := stmt.ID

	res := database.FromContext(ctx, r.db).
		Clauses(
			clause.OnConflict{
				Columns: []clause.Column{{Name: "contact_id"}, {Name: "period_id"}},
				// Matches uq_statements' partial index — a soft-deleted row
				// never conflicts with a fresh insert.
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "statements.deleted_at IS NULL"},
				}},
				DoUpdates: clause.Assignments(map[string]any{
					"total_due":  gorm.Expr("excluded.total_due"),
					"updated_at": gorm.Expr("now()"),
				}),
				// Only a not-yet-revoked row may be refreshed — a revoked
				// statement's link must stay dead even after its contact
				// re-appears in a later Generate run for the same period.
				Where: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "statements.revoked_at IS NULL"},
				}},
			},
			// Empty Returning requests RETURNING * so stmt is refreshed with
			// the row's real id/token_hash/timestamps after the statement —
			// required when the conflict resolves to a pre-existing row
			// whose id and token differ from the candidate this call was
			// invoked with.
			clause.Returning{},
		).
		Create(stmt)
	if res.Error != nil {
		return false, false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, true, nil
	}
	return stmt.ID == candidateID, false, nil
}

func (r *gormRepository) ListByPeriod(ctx context.Context, teacherID, periodID uuid.UUID, p pagination.Params) ([]Row, int64, error) {
	var total int64
	if err := r.scoped(ctx, teacherID).Where("statements.period_id = ?", periodID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	err := r.withContact(ctx, teacherID).
		Where("statements.period_id = ?", periodID).
		Scopes(p.Scope).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) GetByID(ctx context.Context, teacherID, statementID uuid.UUID) (*Row, error) {
	var row Row
	err := r.withContact(ctx, teacherID).Where("statements.id = ?", statementID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) GetByTokenHash(ctx context.Context, tokenHash []byte) (*Statement, error) {
	var stmt Statement
	err := database.FromContext(ctx, r.db).Take(&stmt, "token_hash = ?", tokenHash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &stmt, nil
}

func (r *gormRepository) Revoke(ctx context.Context, teacherID, statementID uuid.UUID) error {
	res := r.scoped(ctx, teacherID).
		Where("statements.id = ? AND statements.revoked_at IS NULL", statementID).
		Update("revoked_at", gorm.Expr("now()"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	// The guarded UPDATE affects no row both when the statement does not
	// exist (or belongs to another teacher) and when it is already revoked —
	// revoking an already-revoked statement must stay a no-op, not an error,
	// so retrying the same request is always safe. Only the missing case is
	// an actual failure.
	var count int64
	if err := r.scoped(ctx, teacherID).Where("statements.id = ?", statementID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// invoicesWithLinesQuery is scoped by teacher_id AND contact_id AND
// period_id in its WHERE clause — every one of the public view's reads
// carries all three, never contact_id or period_id alone, so a resolved
// statement can only ever pull its own family's own period.
const invoicesWithLinesQuery = `
	SELECT i.id AS invoice_id, i.contact_id AS contact_id, i.student_id AS student_id, i.student_name AS student_name,
	       i.contact_name AS contact_name, s.display_note AS display_note,
	       i.opening_balance AS opening_balance, i.current_charge AS current_charge,
	       i.adjustment_total AS adjustment_total, i.total_due AS total_due, i.paid_amount AS paid_amount,
	       bp.year AS period_year, bp.month AS period_month,
	       il.id AS line_id, il.enrollment_id AS enrollment_id, il.class_name AS class_name,
	       il.billable_count AS billable_count, il.absent_count AS absent_count,
	       il.unit_price AS unit_price, il.amount AS line_amount
	FROM invoices i
	JOIN billing_periods bp ON bp.id = i.period_id AND bp.teacher_id = i.teacher_id
	LEFT JOIN invoice_lines il ON il.invoice_id = i.id AND il.teacher_id = i.teacher_id
	LEFT JOIN students s       ON s.id = i.student_id AND s.teacher_id = i.teacher_id
	WHERE i.teacher_id = ? AND i.contact_id = ? AND i.period_id = ? AND i.status <> ?
	ORDER BY i.student_name, i.id, il.created_at
`

func (r *gormRepository) InvoicesWithLines(ctx context.Context, teacherID, contactID, periodID uuid.UUID) ([]InvoiceLineRow, error) {
	var rows []InvoiceLineRow
	err := database.FromContext(ctx, r.db).
		Raw(invoicesWithLinesQuery, teacherID, contactID, periodID, invoiceStatusVoid).
		Scan(&rows).Error
	return rows, err
}

// periodInvoiceLinesQuery is invoicesWithLinesQuery's period-wide sibling:
// scoped by teacher_id AND period_id only, never a single contact_id, and
// ordered by contact_id first so PeriodInvoiceLines' caller can group every
// contact's rows in one linear pass — see Service.PeriodFigures.
const periodInvoiceLinesQuery = `
	SELECT i.id AS invoice_id, i.contact_id AS contact_id, i.student_id AS student_id, i.student_name AS student_name,
	       i.contact_name AS contact_name, s.display_note AS display_note,
	       i.opening_balance AS opening_balance, i.current_charge AS current_charge,
	       i.adjustment_total AS adjustment_total, i.total_due AS total_due, i.paid_amount AS paid_amount,
	       bp.year AS period_year, bp.month AS period_month,
	       il.id AS line_id, il.enrollment_id AS enrollment_id, il.class_name AS class_name,
	       il.billable_count AS billable_count, il.absent_count AS absent_count,
	       il.unit_price AS unit_price, il.amount AS line_amount
	FROM invoices i
	JOIN billing_periods bp ON bp.id = i.period_id AND bp.teacher_id = i.teacher_id
	LEFT JOIN invoice_lines il ON il.invoice_id = i.id AND il.teacher_id = i.teacher_id
	LEFT JOIN students s       ON s.id = i.student_id AND s.teacher_id = i.teacher_id
	WHERE i.teacher_id = ? AND i.period_id = ? AND i.status <> ?
	ORDER BY i.contact_id, i.student_name, i.id, il.created_at
`

func (r *gormRepository) PeriodInvoiceLines(ctx context.Context, teacherID, periodID uuid.UUID) ([]InvoiceLineRow, error) {
	var rows []InvoiceLineRow
	err := database.FromContext(ctx, r.db).
		Raw(periodInvoiceLinesQuery, teacherID, periodID, invoiceStatusVoid).
		Scan(&rows).Error
	return rows, err
}

// liveSessionsQuery reads attendance live off attendance_records/
// class_sessions rather than off the invoice_lines snapshot, so a correction
// made after the statement's period closed is reflected the moment it is
// viewed. deleted_at IS NULL on both joined tables excludes a session or
// attendance record removed since — soft deletes here are corrections, not
// history to keep showing a parent.
const liveSessionsQuery = `
	SELECT il.enrollment_id AS enrollment_id, cs.session_date AS session_date,
	       cs.status AS session_status, a.status AS attendance_status, a.billable AS billable
	FROM invoice_lines il
	JOIN invoices i           ON i.id = il.invoice_id AND i.teacher_id = il.teacher_id
	JOIN attendance_records a ON a.enrollment_id = il.enrollment_id AND a.teacher_id = il.teacher_id
	JOIN class_sessions cs    ON cs.id = a.session_id AND cs.teacher_id = a.teacher_id
	JOIN billing_periods bp   ON bp.id = i.period_id AND bp.teacher_id = i.teacher_id
	WHERE i.teacher_id = ? AND i.contact_id = ? AND i.period_id = ? AND i.status <> ?
	  AND a.deleted_at IS NULL
	  AND cs.deleted_at IS NULL
	  AND cs.session_date BETWEEN bp.period_start AND bp.period_end
	ORDER BY il.enrollment_id, cs.session_date
`

func (r *gormRepository) LiveSessions(ctx context.Context, teacherID, contactID, periodID uuid.UUID) ([]LiveSessionRow, error) {
	var rows []LiveSessionRow
	err := database.FromContext(ctx, r.db).
		Raw(liveSessionsQuery, teacherID, contactID, periodID, invoiceStatusVoid).
		Scan(&rows).Error
	return rows, err
}

// adjustmentsQuery is a single UNION ALL rather than two round trips: its
// first arm reads adjustments posted directly on one of this period's own
// invoices; its second reads adjustments posted on a later invoice whose
// source_session_id falls, by session_date, inside this period's own date
// range — exactly the carried-forward corrections billing.ReconcileSession
// produces when an attendance correction lands inside an already-closed
// period. Both arms exclude i.status = 'void' and
// ia.deleted_at — a voided invoice's or a reversed adjustment's figures must
// never appear on a public statement.
const adjustmentsQuery = `
	SELECT i.student_id AS student_id, ia.amount AS amount,
	       ia.source_session_id AS source_session_id, false AS carried, NULL::date AS session_date
	FROM invoice_adjustments ia
	JOIN invoices i ON i.id = ia.invoice_id AND i.teacher_id = ia.teacher_id
	WHERE ia.teacher_id = ? AND i.contact_id = ? AND i.period_id = ?
	  AND ia.deleted_at IS NULL AND i.status <> ?

	UNION ALL

	SELECT i2.student_id AS student_id, ia.amount AS amount,
	       ia.source_session_id AS source_session_id, true AS carried, cs.session_date AS session_date
	FROM invoice_adjustments ia
	JOIN invoices i2        ON i2.id = ia.invoice_id AND i2.teacher_id = ia.teacher_id
	JOIN class_sessions cs  ON cs.id = ia.source_session_id AND cs.teacher_id = ia.teacher_id
	JOIN billing_periods bp ON bp.id = ? AND bp.teacher_id = ?
	WHERE ia.teacher_id = ? AND i2.contact_id = ? AND ia.source_session_id IS NOT NULL
	  AND ia.deleted_at IS NULL AND i2.period_id <> ? AND i2.status <> ?
	  AND cs.session_date BETWEEN bp.period_start AND bp.period_end
`

func (r *gormRepository) Adjustments(ctx context.Context, teacherID, contactID, periodID uuid.UUID) ([]AdjustmentRow, error) {
	var rows []AdjustmentRow
	err := database.FromContext(ctx, r.db).
		Raw(adjustmentsQuery,
			teacherID, contactID, periodID, invoiceStatusVoid,
			periodID, teacherID,
			teacherID, contactID, periodID, invoiceStatusVoid,
		).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) TouchView(ctx context.Context, teacherID, statementID uuid.UUID) error {
	return database.FromContext(ctx, r.db).
		Exec(`UPDATE statements
		      SET view_count = view_count + 1,
		          first_viewed_at = COALESCE(first_viewed_at, now()),
		          last_viewed_at = now()
		      WHERE id = ? AND teacher_id = ?`, statementID, teacherID).Error
}
