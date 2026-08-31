package statements

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
	"teka/apps/api/internal/shared/pagination"
)

// Row is a statement plus the contact display fields the teacher-facing
// endpoints need, produced in one query so listing a period's statements
// never becomes an N+1. PhoneVisible is the phone-privacy derived column:
// whether the reading caller holds an active hoc_vu stint over one of the
// contact's actively enrolled students (owner/oversight bypass happens in
// the service via Scope.PhoneVisible).
type Row struct {
	Statement       `gorm:"embedded"`
	ContactFullName string
	ContactPhone    string
	PhoneVisible    bool
}

// TargetContact is one contact eligible for a statement in a period: they
// have at least one non-void invoice there. Carries the display fields
// Generate's response needs so building it never requires a second,
// per-contact lookup. PhoneVisible is computed against the calling teacher
// (see TargetContacts).
type TargetContact struct {
	ContactID    uuid.UUID
	FullName     string
	Phone        string
	PhoneVisible bool
}

// PeriodInfo is one billing period's status plus its own owning teacher —
// GetPeriodStatus's result. TeacherID is what Generate/PeriodFigures anchor
// their periodScope on: the period's own teacher, never necessarily the
// caller's, so an owner acting on a member's period never reassigns the
// member's rows to itself.
type PeriodInfo struct {
	Status    string
	TeacherID uuid.UUID
}

// Repository is the persistence contract for statements; the service depends
// on this interface, tests supply a fake.
//
// Every method takes an authctx.Scope and scopes by it, with one deliberate
// exception: GetByTokenHash. The public link a parent opens carries only the
// opaque token, never a scope, so it is the sole sanctioned way to resolve a
// statement without one — token_hash is globally unique (uq_statements_token),
// so the lookup stays exact-match, never a tenant-wide scan.
type Repository interface {
	// GetPeriodStatus reads one billing period's status and owning teacher,
	// center-scoped by sc with owner oversight — Generate's closed-period
	// precondition and authorization check.
	GetPeriodStatus(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (PeriodInfo, error)
	// GetPeriodStatusRead is GetPeriodStatus with reports oversight instead
	// of owner oversight: a reports.send holder resolves any center
	// period, like the owner. Backs read paths (List, PeriodFigures) and the
	// delegated send's GenerateForSend — never the standalone generate route.
	GetPeriodStatusRead(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (PeriodInfo, error)
	// GetPeriodStatusCenter resolves any period in sc's center regardless of
	// who owns it or what the caller may oversee. It backs ONLY the
	// class-scoped paths, which run behind their own class-staff send gate
	// (ClassSendAccess): the class assignment, not period ownership, is what
	// authorizes a hoc_vu there — a class's students can be billed under any
	// teacher's period after a handoff.
	GetPeriodStatusCenter(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (PeriodInfo, error)
	// ClassSendAccess reports how sc relates to one live class in its center:
	// sendable — an ACTIVE class_staff stint whose role_key is in roles
	// (resolved by the service from the capability map); readable — any stint
	// at all, ended included. ErrClassNotFound when the class does not exist
	// (or is soft-deleted) in sc's center — the caller turns
	// readable-but-not-sendable into an honest 403 and everything else into a
	// neutral 404, mirroring classes.Service.GetWritable.
	ClassSendAccess(ctx context.Context, sc authctx.Scope, classID uuid.UUID, roles []string) (sendable, readable bool, err error)
	// TargetContacts returns every contact with at least one non-void
	// invoice in periodID — Generate's candidate set. periodScope must be
	// the period's own owner scope (see Service.Generate's periodScope);
	// viewer is the calling teacher's scope, which PhoneVisible is derived
	// for — the two differ whenever someone opens another teacher's period.
	TargetContacts(ctx context.Context, periodScope, viewer authctx.Scope, periodID uuid.UUID) ([]TargetContact, error)
	// ContactTotals reads v_contact_balance's total_due per contact for
	// periodID — the money Generate writes onto each statement. sc must be
	// the period's own owner scope, like TargetContacts.
	ContactTotals(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]int64, error)
	// TargetContactsClass narrows TargetContacts to contacts holding at least
	// one non-void invoice in periodID with a line billed to a STILL-ACTIVE
	// enrollment in classID — the class copy's candidate set. The active
	// filter applies only to targeting (who receives a class statement);
	// the money on the copy still counts every class line of the period, so
	// a family whose child just left the class stops receiving new class
	// links without any historical figure changing. periodScope/viewer as in
	// TargetContacts.
	TargetContactsClass(ctx context.Context, periodScope, viewer authctx.Scope, periodID, classID uuid.UUID) ([]TargetContact, error)
	// ContactClassTotals sums, per contact, the invoice_lines amounts billed
	// to classID's enrollments on periodID's non-void invoices — the class
	// copy's total_due. Deliberately NOT filtered by enrollment liveness:
	// class money is the period's class charges as invoiced, matching what
	// the public class render shows. sc must be the period's own owner scope.
	ContactClassTotals(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID) (map[uuid.UUID]int64, error)
	// UpsertStatement writes one row on the natural key uq_statements
	// (contact_id, period_id) among non-deleted rows: inserts stmt as given
	// when absent, or refreshes only total_due/updated_at when a
	// not-yet-revoked row already exists — token_hash is never touched by an
	// update, so re-running Generate never rotates a link already sent to a
	// parent. An existing, already-revoked row is left completely untouched
	// (skippedRevoked=true) rather than resurrected. On return, stmt is
	// refreshed (RETURNING) to the persisted row's real values; comparing
	// its id against the id it was called with is how the caller tells
	// created apart from refreshed. sc stamps TeacherID/CenterID onto stmt —
	// it must be the period's own owner scope, so an owner generating a
	// member's statements never reassigns them to itself.
	UpsertStatement(ctx context.Context, sc authctx.Scope, stmt *Statement) (created, skippedRevoked bool, err error)
	// ListByPeriod returns a page of one period's FAMILY statements
	// (class_id IS NULL) with contact display fields, center-scoped by sc
	// with reports oversight (owner or reports.send holder). Class
	// copies live under ListByPeriodClass so the family list never doubles
	// up after a class send.
	ListByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, p pagination.Params) ([]Row, int64, error)
	// ListByPeriodClass returns a page of one period's statements scoped to
	// one class copy (statements.class_id = classID). Visibility follows
	// scopedRead, whose class branch admits an active sending-role stint on
	// that class — the caller must still run ClassSendAccess first so a
	// non-staff caller gets a neutral 404 instead of an empty page.
	ListByPeriodClass(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID, p pagination.Params) ([]Row, int64, error)
	// GetByID returns one statement with contact display fields,
	// center-scoped by sc with reports oversight.
	GetByID(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) (*Row, error)
	// GetByTokenHash resolves a statement by its token's hash. See the
	// Repository doc comment: the one method in this package with no scope
	// parameter.
	GetByTokenHash(ctx context.Context, tokenHash []byte) (*Statement, error)
	// Revoke sets revoked_at on one statement, center-scoped by sc with
	// owner oversight. Idempotent: revoking an already-revoked statement
	// succeeds without changing revoked_at. Returns ErrNotFound only when
	// the statement is missing or outside sc's tenancy.
	Revoke(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) error

	// InvoicesWithLines returns every non-void invoice for contactID in
	// periodID, one row per invoice_lines row (an invoice with none still
	// produces one row, its line fields null) — the public view's child and
	// class snapshot figures in a single round trip, independent of how many
	// children or classes the family has. sc must be derived from the
	// resolved statement row on the public path, or the period's own owner
	// scope on the authenticated path.
	InvoicesWithLines(ctx context.Context, sc authctx.Scope, contactID, periodID uuid.UUID) ([]InvoiceLineRow, error)
	// LiveSessions returns, for every enrollment billed in periodID, its
	// attendance-confirmed sessions whose date falls inside the period's own
	// date range — read live off attendance_records/class_sessions, not off
	// the invoice_lines snapshot, so a correction made after the statement
	// was generated shows up on the next view without regenerating anything.
	LiveSessions(ctx context.Context, sc authctx.Scope, contactID, periodID uuid.UUID) ([]LiveSessionRow, error)
	// PeriodInvoiceLines returns every non-void invoice's lines for periodID
	// across every contact, one row per invoice_lines row (or, for an invoice
	// with none, one null-line placeholder row) — the period-wide counterpart
	// to InvoicesWithLines. This is what lets Service.PeriodFigures assemble
	// every contact's message figures for a bulk send in one round trip
	// instead of one query per contact. sc must be the period's own owner
	// scope.
	PeriodInvoiceLines(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) ([]InvoiceLineRow, error)
	// PeriodClassInvoiceLines is PeriodInvoiceLines narrowed to lines billed
	// to classID's enrollments (INNER JOIN — an invoice with no class line
	// contributes nothing). Like ContactClassTotals it carries no enrollment
	// liveness filter: these are the class copy's message figures, and money
	// stays as invoiced. sc must be the period's own owner scope.
	PeriodClassInvoiceLines(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID) ([]InvoiceLineRow, error)
	// Adjustments returns both kinds of adjustment a child's statement can
	// show: one posted directly on one of this period's own invoices, and
	// one posted on a later invoice but whose source session falls inside
	// this period's date range (a post-close correction billing.
	// ReconcileSession carried forward — see the Carried field). The
	// teacher-authored free-text reason never appears in this projection; it
	// must never reach a public payload.
	Adjustments(ctx context.Context, sc authctx.Scope, contactID, periodID uuid.UUID) ([]AdjustmentRow, error)
	// TouchView records one open of a statement's public link: increments
	// view_count, sets first_viewed_at once, and refreshes last_viewed_at. sc
	// must be derived from the resolved statement row, never the (absent)
	// caller's — this is reached only from the unauthenticated public path.
	TouchView(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) error
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
	// LineClassID is the class the line's enrollment belongs to, resolved
	// live from enrollments (invoice_lines snapshots only the class NAME).
	// Nil on a placeholder row or when the enrollment row is gone. This is
	// what the public class render filters a family's invoices down to one
	// class copy by.
	LineClassID *uuid.UUID
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

// scoped returns a statements query bound to one center, further narrowed to
// one teacher's own rows unless the caller is the center's owner.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Model(&Statement{}).Where("statements.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermStatementsViewAll) {
		q = q.Where("statements.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// scopedRead is scoped()'s read-only sibling: the teacher filter is lifted
// for anyone with reports oversight (owner or reports.send holder), not
// just the owner. A caller without oversight additionally sees a CLASS copy
// (class_id set) when they hold an active sending-role stint on that class —
// the copy exists exactly so class staff can work a period they don't own,
// and hiding it from them would orphan every row their own send created
// under the period owner's teacher_id. It backs ONLY read paths
// (ListByPeriod, ListByPeriodClass, GetByID) — writes like Revoke keep
// scoped(), so neither the delegated permission nor a class stint gains
// write reach here.
//
// The sending-role slice (statementSendRoles, resolved once in the service
// layer) is bound here rather than threaded through every read call: this is
// a scope helper, and its class branch is defined BY that capability, not by
// a per-call decision.
func (r *gormRepository) scopedRead(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Model(&Statement{}).Where("statements.center_id = ?", sc.CenterID)
	if !sc.ReportsOversight() {
		frag, _ := classscope.WriteExists("statements.class_id")
		q = q.Where("statements.teacher_id = ? OR (statements.class_id IS NOT NULL AND "+frag+")",
			sc.TeacherID, sc.TeacherID, sc.CenterID, statementSendRoles)
	}
	return q
}

// withContact joins in the contact display fields ListByPeriod/GetByID
// return alongside each statement. The join switches to center_id, not
// teacher_id: an oversight caller's row set spans every teacher in the
// center, so matching on teacher_id here would silently drop a member's own
// contacts from such a read.
func (r *gormRepository) withContact(ctx context.Context, sc authctx.Scope) *gorm.DB {
	frag, _ := classscope.PhoneVisibleViaContact("statements.contact_id")
	return r.scopedRead(ctx, sc).
		Select(`statements.*, contacts.full_name AS contact_full_name, contacts.phone AS contact_phone, `+frag+` AS phone_visible`,
			sc.TeacherID, sc.CenterID).
		Joins("JOIN contacts ON contacts.id = statements.contact_id AND contacts.center_id = statements.center_id")
}

func (r *gormRepository) GetPeriodStatus(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (PeriodInfo, error) {
	return r.periodStatus(ctx, sc, periodID, sc.CenterWideFor(authctx.PermStatementsViewAll))
}

func (r *gormRepository) GetPeriodStatusRead(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (PeriodInfo, error) {
	return r.periodStatus(ctx, sc, periodID, sc.ReportsOversight())
}

func (r *gormRepository) GetPeriodStatusCenter(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (PeriodInfo, error) {
	return r.periodStatus(ctx, sc, periodID, true)
}

func (r *gormRepository) ClassSendAccess(ctx context.Context, sc authctx.Scope, classID uuid.UUID, roles []string) (sendable, readable bool, err error) {
	writeFrag, _ := classscope.WriteExists("classes.id")
	readFrag, _ := classscope.ReadExists("classes.id")
	var rows []struct {
		Sendable bool
		Readable bool
	}
	err = database.FromContext(ctx, r.db).
		Table("classes").
		Select(writeFrag+" AS sendable, "+readFrag+" AS readable",
			sc.TeacherID, sc.CenterID, roles, sc.TeacherID, sc.CenterID).
		Where("classes.id = ? AND classes.center_id = ? AND classes.deleted_at IS NULL", classID, sc.CenterID).
		Find(&rows).Error
	if err != nil {
		return false, false, err
	}
	if len(rows) == 0 {
		return false, false, ErrClassNotFound
	}
	return rows[0].Sendable, rows[0].Readable, nil
}

func (r *gormRepository) periodStatus(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, centerWide bool) (PeriodInfo, error) {
	var rows []PeriodInfo
	q := database.FromContext(ctx, r.db).
		Table("billing_periods").
		Select("status, teacher_id").
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", periodID, sc.CenterID)
	if !centerWide {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return PeriodInfo{}, err
	}
	if len(rows) == 0 {
		return PeriodInfo{}, ErrPeriodNotFound
	}
	return rows[0], nil
}

// TargetContacts and ContactTotals below tenant-filter by the period's own
// owner scope (periodScope, resolved by GetPeriodStatus and derived by
// Service.Generate) rather than a general center/owner scope: a period
// belongs to exactly one teacher, so every invoice under it already carries
// that same teacher_id — no owner short-circuit is needed here, only the
// plain center_id+teacher_id match.

func (r *gormRepository) TargetContacts(ctx context.Context, periodScope, viewer authctx.Scope, periodID uuid.UUID) ([]TargetContact, error) {
	// phone_visible is derived for the CALLER (viewer), never the period's
	// teacher: the period lookup opens center-wide, so someone may reach
	// another teacher's period here, and the phone mask must follow what that
	// caller may see — running it on the period owner's scope would leak.
	frag, _ := classscope.PhoneVisibleViaContact("invoices.contact_id")
	var rows []TargetContact
	err := database.FromContext(ctx, r.db).
		Table("invoices").
		Select(`DISTINCT invoices.contact_id AS contact_id, contacts.full_name AS full_name, contacts.phone AS phone, `+frag+` AS phone_visible`,
			viewer.TeacherID, viewer.CenterID).
		Joins("JOIN contacts ON contacts.id = invoices.contact_id AND contacts.center_id = invoices.center_id").
		Where("invoices.center_id = ? AND invoices.teacher_id = ? AND invoices.period_id = ? AND invoices.status <> ?",
			periodScope.CenterID, periodScope.TeacherID, periodID, invoiceStatusVoid).
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) TargetContactsClass(ctx context.Context, periodScope, viewer authctx.Scope, periodID, classID uuid.UUID) ([]TargetContact, error) {
	frag, _ := classscope.PhoneVisibleViaContact("invoices.contact_id")
	var rows []TargetContact
	// The EXISTS is the targeting rule: at least one of the invoice's lines
	// is billed to a STILL-ACTIVE enrollment in the class. Money queries
	// (ContactClassTotals, PeriodClassInvoiceLines) deliberately drop the
	// liveness filter — see the interface doc comments.
	err := database.FromContext(ctx, r.db).
		Table("invoices").
		Select(`DISTINCT invoices.contact_id AS contact_id, contacts.full_name AS full_name, contacts.phone AS phone, `+frag+` AS phone_visible`,
			viewer.TeacherID, viewer.CenterID).
		Joins("JOIN contacts ON contacts.id = invoices.contact_id AND contacts.center_id = invoices.center_id").
		Where("invoices.center_id = ? AND invoices.teacher_id = ? AND invoices.period_id = ? AND invoices.status <> ?",
			periodScope.CenterID, periodScope.TeacherID, periodID, invoiceStatusVoid).
		Where(`EXISTS (
			SELECT 1 FROM invoice_lines il
			JOIN enrollments e ON e.id = il.enrollment_id AND e.center_id = il.center_id
			WHERE il.invoice_id = invoices.id AND il.center_id = invoices.center_id
			  AND e.class_id = ? AND e.deleted_at IS NULL AND e.ended_on IS NULL)`, classID).
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) ContactTotals(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]int64, error) {
	type row struct {
		ContactID uuid.UUID
		TotalDue  int64
	}
	var rows []row
	err := database.FromContext(ctx, r.db).
		Table("v_contact_balance").
		Select("contact_id, total_due").
		Where("center_id = ? AND teacher_id = ? AND period_id = ?", sc.CenterID, sc.TeacherID, periodID).
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

// contactClassTotalsQuery sums the class's own line amounts per contact —
// straight off invoice_lines, not v_contact_balance, because a class copy
// carries no opening balance or adjustments: its total IS its class charges.
// No enrollment liveness filter — money stays as invoiced (see the
// ContactClassTotals interface doc comment).
const contactClassTotalsQuery = `
	SELECT i.contact_id AS contact_id, SUM(il.amount) AS total_due
	FROM invoices i
	JOIN invoice_lines il ON il.invoice_id = i.id AND il.center_id = i.center_id
	JOIN enrollments e    ON e.id = il.enrollment_id AND e.center_id = il.center_id
	WHERE i.center_id = ? AND i.teacher_id = ? AND i.period_id = ? AND i.status <> ?
	  AND e.class_id = ?
	GROUP BY i.contact_id
`

func (r *gormRepository) ContactClassTotals(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID) (map[uuid.UUID]int64, error) {
	type row struct {
		ContactID uuid.UUID
		TotalDue  int64
	}
	var rows []row
	err := database.FromContext(ctx, r.db).
		Raw(contactClassTotalsQuery, sc.CenterID, sc.TeacherID, periodID, invoiceStatusVoid, classID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int64, len(rows))
	for _, rr := range rows {
		out[rr.ContactID] = rr.TotalDue
	}
	return out, nil
}

func (r *gormRepository) UpsertStatement(ctx context.Context, sc authctx.Scope, stmt *Statement) (created, skippedRevoked bool, err error) {
	stmt.TeacherID = sc.TeacherID
	stmt.CenterID = sc.CenterID
	candidateID := stmt.ID

	// The conflict target must name the exact partial unique index the row
	// lands under — Postgres only infers an index whose predicate the clause
	// matches. A family row (class_id nil) conflicts on uq_statements
	// (contact_id, period_id) WHERE class_id IS NULL; a class copy on
	// uq_statements_class (contact_id, period_id, class_id) WHERE class_id
	// IS NOT NULL. The two never collide with each other, so a class send
	// can never refresh (or resurrect) the family link and vice versa.
	conflictCols := []clause.Column{{Name: "contact_id"}, {Name: "period_id"}}
	conflictPredicate := "statements.class_id IS NULL AND statements.deleted_at IS NULL"
	if stmt.ClassID != nil {
		conflictCols = append(conflictCols, clause.Column{Name: "class_id"})
		conflictPredicate = "statements.class_id IS NOT NULL AND statements.deleted_at IS NULL"
	}

	res := database.FromContext(ctx, r.db).
		Clauses(
			clause.OnConflict{
				Columns: conflictCols,
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: conflictPredicate},
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

func (r *gormRepository) ListByPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, p pagination.Params) ([]Row, int64, error) {
	var total int64
	if err := r.scopedRead(ctx, sc).
		Where("statements.period_id = ? AND statements.class_id IS NULL", periodID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	err := r.withContact(ctx, sc).
		Where("statements.period_id = ? AND statements.class_id IS NULL", periodID).
		Scopes(p.Scope).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) ListByPeriodClass(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID, p pagination.Params) ([]Row, int64, error) {
	var total int64
	if err := r.scopedRead(ctx, sc).
		Where("statements.period_id = ? AND statements.class_id = ?", periodID, classID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Row
	err := r.withContact(ctx, sc).
		Where("statements.period_id = ? AND statements.class_id = ?", periodID, classID).
		Scopes(p.Scope).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) GetByID(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) (*Row, error) {
	var row Row
	err := r.withContact(ctx, sc).Where("statements.id = ?", statementID).Take(&row).Error
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

func (r *gormRepository) Revoke(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) error {
	res := r.scoped(ctx, sc).
		Where("statements.id = ? AND statements.revoked_at IS NULL", statementID).
		Update("revoked_at", gorm.Expr("now()"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	// The guarded UPDATE affects no row both when the statement does not
	// exist (or is outside sc's tenancy) and when it is already revoked —
	// revoking an already-revoked statement must stay a no-op, not an error,
	// so retrying the same request is always safe. Only the missing case is
	// an actual failure.
	var count int64
	if err := r.scoped(ctx, sc).Where("statements.id = ?", statementID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// invoicesWithLinesQuery is scoped by center_id AND teacher_id AND
// contact_id AND period_id in its WHERE clause — every one of the public
// view's reads carries all four, never contact_id or period_id alone, so a
// resolved statement can only ever pull its own family's own period. sc is
// always a narrow, single-teacher-derived scope here (periodScope or the
// statement row's own anchors), never a general owner-bypass scope, so a
// plain AND match is enough — no owner short-circuit needed.
const invoicesWithLinesQuery = `
	SELECT i.id AS invoice_id, i.contact_id AS contact_id, i.student_id AS student_id, i.student_name AS student_name,
	       i.contact_name AS contact_name, s.display_note AS display_note,
	       i.opening_balance AS opening_balance, i.current_charge AS current_charge,
	       i.adjustment_total AS adjustment_total, i.total_due AS total_due, i.paid_amount AS paid_amount,
	       bp.year AS period_year, bp.month AS period_month,
	       il.id AS line_id, il.enrollment_id AS enrollment_id, il.class_name AS class_name,
	       il.billable_count AS billable_count, il.absent_count AS absent_count,
	       il.unit_price AS unit_price, il.amount AS line_amount,
	       e.class_id AS line_class_id
	FROM invoices i
	JOIN billing_periods bp ON bp.id = i.period_id AND bp.center_id = i.center_id
	LEFT JOIN invoice_lines il ON il.invoice_id = i.id AND il.center_id = i.center_id
	LEFT JOIN enrollments e    ON e.id = il.enrollment_id AND e.center_id = il.center_id
	LEFT JOIN students s       ON s.id = i.student_id AND s.center_id = i.center_id
	WHERE i.center_id = ? AND i.teacher_id = ? AND i.contact_id = ? AND i.period_id = ? AND i.status <> ?
	ORDER BY i.student_name, i.id, il.created_at
`

func (r *gormRepository) InvoicesWithLines(ctx context.Context, sc authctx.Scope, contactID, periodID uuid.UUID) ([]InvoiceLineRow, error) {
	var rows []InvoiceLineRow
	err := database.FromContext(ctx, r.db).
		Raw(invoicesWithLinesQuery, sc.CenterID, sc.TeacherID, contactID, periodID, invoiceStatusVoid).
		Scan(&rows).Error
	return rows, err
}

// periodInvoiceLinesQuery is invoicesWithLinesQuery's period-wide sibling:
// scoped by center_id AND teacher_id AND period_id only, never a single
// contact_id, and ordered by contact_id first so PeriodInvoiceLines' caller
// can group every contact's rows in one linear pass — see
// Service.PeriodFigures.
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
	JOIN billing_periods bp ON bp.id = i.period_id AND bp.center_id = i.center_id
	LEFT JOIN invoice_lines il ON il.invoice_id = i.id AND il.center_id = i.center_id
	LEFT JOIN students s       ON s.id = i.student_id AND s.center_id = i.center_id
	WHERE i.center_id = ? AND i.teacher_id = ? AND i.period_id = ? AND i.status <> ?
	ORDER BY i.contact_id, i.student_name, i.id, il.created_at
`

func (r *gormRepository) PeriodInvoiceLines(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) ([]InvoiceLineRow, error) {
	var rows []InvoiceLineRow
	err := database.FromContext(ctx, r.db).
		Raw(periodInvoiceLinesQuery, sc.CenterID, sc.TeacherID, periodID, invoiceStatusVoid).
		Scan(&rows).Error
	return rows, err
}

// periodClassInvoiceLinesQuery narrows periodInvoiceLinesQuery to one class's
// lines with INNER JOINs: an invoice with no line in the class contributes no
// row at all — a class copy shows only its own charges, so there is no
// null-line placeholder here. No enrollment liveness filter, matching
// contactClassTotalsQuery.
const periodClassInvoiceLinesQuery = `
	SELECT i.id AS invoice_id, i.contact_id AS contact_id, i.student_id AS student_id, i.student_name AS student_name,
	       i.contact_name AS contact_name, s.display_note AS display_note,
	       i.opening_balance AS opening_balance, i.current_charge AS current_charge,
	       i.adjustment_total AS adjustment_total, i.total_due AS total_due, i.paid_amount AS paid_amount,
	       bp.year AS period_year, bp.month AS period_month,
	       il.id AS line_id, il.enrollment_id AS enrollment_id, il.class_name AS class_name,
	       il.billable_count AS billable_count, il.absent_count AS absent_count,
	       il.unit_price AS unit_price, il.amount AS line_amount,
	       e.class_id AS line_class_id
	FROM invoices i
	JOIN billing_periods bp ON bp.id = i.period_id AND bp.center_id = i.center_id
	JOIN invoice_lines il   ON il.invoice_id = i.id AND il.center_id = i.center_id
	JOIN enrollments e      ON e.id = il.enrollment_id AND e.center_id = il.center_id
	LEFT JOIN students s    ON s.id = i.student_id AND s.center_id = i.center_id
	WHERE i.center_id = ? AND i.teacher_id = ? AND i.period_id = ? AND i.status <> ?
	  AND e.class_id = ?
	ORDER BY i.contact_id, i.student_name, i.id, il.created_at
`

func (r *gormRepository) PeriodClassInvoiceLines(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID) ([]InvoiceLineRow, error) {
	var rows []InvoiceLineRow
	err := database.FromContext(ctx, r.db).
		Raw(periodClassInvoiceLinesQuery, sc.CenterID, sc.TeacherID, periodID, invoiceStatusVoid, classID).
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
	JOIN invoices i           ON i.id = il.invoice_id AND i.center_id = il.center_id
	JOIN attendance_records a ON a.enrollment_id = il.enrollment_id AND a.center_id = il.center_id
	JOIN class_sessions cs    ON cs.id = a.session_id AND cs.center_id = a.center_id
	JOIN billing_periods bp   ON bp.id = i.period_id AND bp.center_id = i.center_id
	WHERE i.center_id = ? AND i.teacher_id = ? AND i.contact_id = ? AND i.period_id = ? AND i.status <> ?
	  AND a.deleted_at IS NULL
	  AND cs.deleted_at IS NULL
	  AND cs.session_date BETWEEN bp.period_start AND bp.period_end
	ORDER BY il.enrollment_id, cs.session_date
`

func (r *gormRepository) LiveSessions(ctx context.Context, sc authctx.Scope, contactID, periodID uuid.UUID) ([]LiveSessionRow, error) {
	var rows []LiveSessionRow
	err := database.FromContext(ctx, r.db).
		Raw(liveSessionsQuery, sc.CenterID, sc.TeacherID, contactID, periodID, invoiceStatusVoid).
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
	JOIN invoices i ON i.id = ia.invoice_id AND i.center_id = ia.center_id
	WHERE ia.center_id = ? AND ia.teacher_id = ? AND i.contact_id = ? AND i.period_id = ?
	  AND ia.deleted_at IS NULL AND i.status <> ?

	UNION ALL

	SELECT i2.student_id AS student_id, ia.amount AS amount,
	       ia.source_session_id AS source_session_id, true AS carried, cs.session_date AS session_date
	FROM invoice_adjustments ia
	JOIN invoices i2        ON i2.id = ia.invoice_id AND i2.center_id = ia.center_id
	JOIN class_sessions cs  ON cs.id = ia.source_session_id AND cs.center_id = ia.center_id
	JOIN billing_periods bp ON bp.id = ? AND bp.center_id = ia.center_id
	WHERE ia.center_id = ? AND ia.teacher_id = ? AND i2.contact_id = ? AND ia.source_session_id IS NOT NULL
	  AND ia.deleted_at IS NULL AND i2.period_id <> ? AND i2.status <> ?
	  AND cs.session_date BETWEEN bp.period_start AND bp.period_end
`

func (r *gormRepository) Adjustments(ctx context.Context, sc authctx.Scope, contactID, periodID uuid.UUID) ([]AdjustmentRow, error) {
	var rows []AdjustmentRow
	err := database.FromContext(ctx, r.db).
		Raw(adjustmentsQuery,
			sc.CenterID, sc.TeacherID, contactID, periodID, invoiceStatusVoid,
			periodID,
			sc.CenterID, sc.TeacherID, contactID, periodID, invoiceStatusVoid,
		).
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) TouchView(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) error {
	return database.FromContext(ctx, r.db).
		Exec(`UPDATE statements
		      SET view_count = view_count + 1,
		          first_viewed_at = COALESCE(first_viewed_at, now()),
		          last_viewed_at = now()
		      WHERE id = ? AND teacher_id = ? AND center_id = ?`, statementID, sc.TeacherID, sc.CenterID).Error
}
