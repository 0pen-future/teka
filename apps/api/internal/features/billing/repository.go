package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classscope"
	"teka/apps/api/internal/shared/pagination"
)

var (
	// ErrPeriodNotFound covers both a missing billing period and another
	// tenant's — the caller cannot tell them apart, by design.
	ErrPeriodNotFound = errors.New("billing period not found")
	// ErrDuplicatePeriod is the uq_billing_periods unique violation: a period
	// already exists for this (teacher, year, month).
	ErrDuplicatePeriod = errors.New("billing period already exists")
	// ErrTeacherNotFound means the teacher id has no row in teachers — should
	// not happen for an authenticated caller, but resolved defensively rather
	// than assumed.
	ErrTeacherNotFound = errors.New("teacher not found")
	// ErrInvoiceNotDraft is UpsertInvoice's refusal to touch an invoice whose
	// status has already moved past draft (issued and later are immutable —
	// D7). The caller maps this onto a 409 and the whole draft transaction
	// rolls back, so a concurrent issue never gets silently overwritten.
	ErrInvoiceNotDraft = errors.New("invoice is not a draft")
	// ErrInvoiceNotFound covers both a missing invoice and another tenant's.
	ErrInvoiceNotFound = errors.New("invoice not found")
	// errPeriodStatusChanged is a defense-in-depth invariant check: ClosePeriod's
	// guarded UPDATE (WHERE status='open') affected no row even though LockPeriod
	// already serialised concurrent closers with SELECT ... FOR UPDATE. It should
	// be unreachable in practice; surfacing it as a plain (non-AppError) error
	// makes Close abort the transaction rather than silently succeed on a period
	// that was not actually open.
	errPeriodStatusChanged = errors.New("billing period status changed unexpectedly during close")
	// ErrSessionNotFound covers both a missing session and another tenant's
	// for SessionMeta — the direct class_sessions lookup ReconcileSession
	// uses to resolve which class and date it is reacting to.
	ErrSessionNotFound = errors.New("session not found")
	// ErrStudentNotFound covers both a missing student and another
	// tenant's for StudentSnapshot.
	ErrStudentNotFound = errors.New("student not found")
	// ErrClassNotFound covers both a missing class and another tenant's for
	// ClassReadable.
	ErrClassNotFound = errors.New("class not found")
)

// CarriedDebtStudent is a previous-period debtor considered for the current
// period's opening balance even when they have no attendance at all this
// period — a student who quit mid-cycle but still owes money (PRD "học sinh
// nghỉ hẳn giữa chu kỳ giữ lại nợ"). Contact/name fields resolve against the
// student's *current* contact, matching the documented draft-follows-live-
// contact behaviour (phase 2 Architecture).
type CarriedDebtStudent struct {
	StudentID   uuid.UUID
	ContactID   uuid.UUID
	StudentName string
	ContactName string
	Outstanding int64
}

// AttendanceSource is the slice of the attendance feature billing needs: the
// batched billable/absent/present tally per enrollment for a date window.
// *attendance.Service satisfies this. Declared here — a consumer-defined
// interface, the same pattern attendance and sessions use for their own
// upstream dependencies — so billing depends on attendance's public service
// contract, never its repository type.
type AttendanceSource interface {
	TallyByEnrollment(ctx context.Context, sc authctx.Scope, from, to time.Time) ([]attendance.EnrollmentTally, error)
	// SessionAttendance returns the attendance rows already recorded for one
	// session — plan 04's entry point for discovering which students a
	// post-close reconciliation must consider. It performs no aggregation;
	// LiveBillableCounts (built on TallyByEnrollment) remains the sole
	// counting query.
	SessionAttendance(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]attendance.Record, error)
}

// PeriodWithTeacher is one billing period plus its owning teacher's display
// name, produced in one query by the period read endpoints so a center-wide
// (oversight) listing can group by teacher without an N+1.
type PeriodWithTeacher struct {
	Period      `gorm:"embedded"`
	TeacherName string
}

// Repository is the persistence contract for billing periods and the
// attendance-to-money assembler; the service depends on this interface,
// tests supply a fake.
type Repository interface {
	CreatePeriod(ctx context.Context, p *Period) error
	GetPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*Period, error)
	// GetPeriodRead / ListPeriodsRead back the period read ENDPOINTS only:
	// center-scoped with reports oversight (owner or reports.send
	// holder), joined with the owning teacher's name. Write paths (draft,
	// close, tally) keep the owner-gated GetPeriod.
	GetPeriodRead(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*PeriodWithTeacher, error)
	ListPeriodsRead(ctx context.Context, sc authctx.Scope, p pagination.Params) ([]PeriodWithTeacher, int64, error)
	// ClassReadable reports whether the caller holds any class_staff stint on
	// the class — ended stints included, matching every other class-data read.
	// ErrClassNotFound when the class does not exist in the caller's center
	// (or is soft-deleted). The oversight bypass lives in the service.
	ClassReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
	// ListPeriodsClassRead lists the center's periods carrying at least one
	// non-void invoice line billed to one of classID's enrollments —
	// deliberately NOT own-teacher-filtered, because after a handoff the
	// class's charges live under another teacher's period and it is the
	// caller's stint on the class (checked by the service via ClassReadable)
	// that entitles the read.
	ListPeriodsClassRead(ctx context.Context, sc authctx.Scope, classID uuid.UUID, p pagination.Params) ([]PeriodWithTeacher, int64, error)
	GetPeriodByYearMonth(ctx context.Context, sc authctx.Scope, year, month int16) (*Period, error)
	// PreviousClosedPeriod returns the most recently closed period whose
	// period_end is strictly before `before` — the R3 carry-over source. A
	// nil result with a nil error means there is no such period: the
	// student's very first billing cycle, where opening_balance is
	// legitimately zero, not a lookup failure.
	PreviousClosedPeriod(ctx context.Context, sc authctx.Scope, before time.Time) (*Period, error)
	// TallyAttendance assembles []AttendanceTally for one period: one call
	// into attendance.Service.TallyByEnrollment for the counts, zipped on
	// enrollment_id with billing's own enrollment/student/contact/class
	// metadata join. It writes no aggregate over attendance_records and adds
	// no enrollment date-range filter of its own — the counting and
	// roster-membership rules belong to plan 03, not here.
	TallyAttendance(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) ([]AttendanceTally, error)
	// OpeningBalances reads the outstanding (total_due - paid_amount, clamped
	// to 0) of each student's non-void invoice in prevPeriodID. A student
	// with no invoice there is simply absent from the returned map — the
	// caller treats a missing entry as zero.
	OpeningBalances(ctx context.Context, sc authctx.Scope, prevPeriodID uuid.UUID, studentIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	// TeacherTimezone reads the IANA zone EnsurePeriod resolves
	// period_start/period_end in. Keyed by a raw teacher id, not a scope: it
	// is a direct FK lookup on an id the caller already resolved (self, or a
	// period/session owner), not a tenancy-filtered query.
	TeacherTimezone(ctx context.Context, teacherID uuid.UUID) (string, error)
	// AdjustmentTotals sums invoice_adjustments.amount (deleted_at IS NULL)
	// per invoice for every invoice in periodID, keyed by invoice_id. An
	// invoice absent from the result has no adjustments.
	AdjustmentTotals(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]int64, error)
	// CarriedDebtStudents returns every student with outstanding debt
	// (total_due - paid_amount > 0) on a non-void invoice in prevPeriodID —
	// the set ComputePeriod must fold into Compute's input even when
	// TallyAttendance has no row for them.
	CarriedDebtStudents(ctx context.Context, sc authctx.Scope, prevPeriodID uuid.UUID) ([]CarriedDebtStudent, error)
	// UpsertInvoice writes one row on the natural key uq_invoices
	// (period_id, student_id): inserts when absent, refreshes the
	// contact/name snapshots and money columns when the existing row is
	// still status='draft', and returns ErrInvoiceNotDraft without writing
	// anything when it is not — an issued invoice never gets silently
	// rewritten. inv.ID is set to the persisted row's actual id on return
	// (RETURNING), which matters when a caller resolved the wrong candidate
	// id for what turned out to be a pre-existing row. Not scope-filtered:
	// inv already carries the period's own TeacherID/CenterID, stamped by
	// the caller before this is invoked.
	UpsertInvoice(ctx context.Context, inv *Invoice) error
	// UpsertInvoiceLine writes one row on the natural key uq_invoice_line
	// (invoice_id, enrollment_id): inserts when absent, refreshes counts,
	// price, and amount when present. Never deletes.
	UpsertInvoiceLine(ctx context.Context, line *InvoiceLine) error
	// ZeroUnmatchedLines sets billable_count=0, absent_count=0, amount=0 on
	// every line of invoiceID whose enrollment_id is not in
	// keepEnrollmentIDs — the row survives so the invoice total always
	// reconciles against its own detail (schema note (i)).
	ZeroUnmatchedLines(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID, keepEnrollmentIDs []uuid.UUID) error
	// ListInvoices returns every invoice already stored for periodID.
	ListInvoices(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) ([]Invoice, error)
	// GetInvoiceWithLines returns one invoice and its lines. Reserved for a
	// later phase's detail endpoint; not called by Preview or Draft, which
	// build their response from the in-memory compute result instead (the
	// bare rows here carry no class_id or present_count).
	GetInvoiceWithLines(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, []InvoiceLine, error)
	// LockPeriod issues SELECT ... FOR UPDATE on billing_periods, center-scoped.
	// Close calls it first so two concurrent close requests for the same period
	// serialise instead of double-issuing invoices.
	LockPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*Period, error)
	// IssueDraftInvoices bulk-updates every remaining status='draft' invoice of
	// periodID to status='issued', returning the number of rows changed. One
	// statement regardless of student count.
	IssueDraftInvoices(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (int64, error)
	// VoidInvoices bulk-updates every status='draft' invoice of periodID whose
	// current_charge, opening_balance, and adjustment_total are all zero to
	// status='void' with a fixed void_reason and voided_at=now(), returning the
	// number of rows changed. Must run before IssueDraftInvoices, whose blanket
	// WHERE status='draft' would otherwise also issue these.
	VoidInvoices(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (int64, error)
	// ClosePeriod sets status='closed', closed_at, updated_at, guarded by
	// WHERE status='open'. Returns errPeriodStatusChanged if RowsAffected != 1 —
	// the LockPeriod row lock should make this unreachable in normal operation.
	ClosePeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, closedAt time.Time) error
	// GetInvoice returns one bare invoice row (no lines).
	GetInvoice(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, error)
	// LockInvoice issues SELECT ... FOR UPDATE on one invoice row,
	// center-scoped, returning the freshly-read row. A post-close
	// reconciliation locks the closed-period invoice — the natural key for one
	// (student, period) pair — before it reads the already-carried adjustment
	// total, so two reconciliations of the same student's closed period
	// serialise instead of both computing the same carry against a stale
	// already_adj and double-posting it.
	LockInvoice(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, error)
	// VoidInvoice sets status='void', voided_at, void_reason on one invoice
	// currently status IN (issued, partially_paid). Returns ErrInvoiceNotFound
	// if no such row (missing, wrong tenant, or not in a voidable status).
	VoidInvoice(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID, reason string, at time.Time) error

	// CreateAdjustment inserts one invoice_adjustments row. reason must
	// already be validated non-empty by the caller — the DB CHECK is a
	// backstop, never the first line of defense.
	CreateAdjustment(ctx context.Context, adj *InvoiceAdjustment) error
	// ListAdjustments returns invoiceID's adjustment audit trail, oldest
	// first, filtered to deleted_at IS NULL — cancelling an adjustment in V1
	// always posts an opposite-signed row rather than soft-deleting the
	// original, so this is a complete, append-only history.
	ListAdjustments(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) ([]InvoiceAdjustment, error)
	// AdjustmentsBySourcePeriod sums invoice_adjustments.amount
	// (deleted_at IS NULL) for one student whose source_session_id belongs to
	// a session inside periodID's date window, regardless of which invoice
	// each adjustment actually landed on. This is the already_adj term that
	// stops a second attendance edit on the same closed period from
	// double-counting the first edit's carry.
	AdjustmentsBySourcePeriod(ctx context.Context, sc authctx.Scope, studentID, periodID uuid.UUID) (int64, error)
	// RecalcInvoiceTotals re-derives adjustment_total, total_due, and status
	// from invoiceID's own current_charge/opening_balance/paid_amount plus a
	// fresh SUM over its non-deleted adjustments, in one UPDATE ... FROM so
	// the total_due CHECK can never observe a partial write. status only ever
	// moves among issued/partially_paid/paid here — draft and void are left
	// untouched, owned by Draft and VoidInvoice respectively.
	RecalcInvoiceTotals(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) error
	// PeriodContainingDate returns the closed billing period whose window
	// contains `on`, or nil (not an error) when none is closed yet — the
	// signal ReconcileSession uses to no-op for a still-open period. sc must
	// be the session's own owner scope, never the acting caller's — this is
	// a date-range search with no id anchor.
	PeriodContainingDate(ctx context.Context, sc authctx.Scope, on time.Time) (*Period, error)
	// NextOpenPeriod returns the earliest open period whose period_start is
	// strictly after afterPeriodEnd, or nil (not an error) when none exists —
	// ensureAdjustmentTarget's first resolution step. sc must be the closed
	// period's own owner scope, never the acting caller's.
	NextOpenPeriod(ctx context.Context, sc authctx.Scope, afterPeriodEnd time.Time) (*Period, error)
	// LiveBillableCounts filters attendance's batched per-enrollment tally for
	// period's window down to enrollmentIDs, keyed by enrollment id. An
	// enrollment id with no confirmed billable attendance in the window is
	// simply absent from the result — callers treat a missing entry as zero.
	// Scope is derived internally from period's own TeacherID/CenterID, never
	// taken as a parameter — period already carries the only anchor this
	// lookup may use. Gap: plan 03 exposes no per-enrollment single-count
	// method, only the batched tally, so this wraps it rather than
	// re-querying attendance_records — the one counting query a
	// reconciliation and a draft/close alike ultimately share.
	LiveBillableCounts(ctx context.Context, enrollmentIDs []uuid.UUID, period *Period) (map[uuid.UUID]int, error)
	// SessionMeta resolves one class_sessions row's class_id, class name,
	// session_date, and its own owner scope directly against the table — the
	// same direct-table-read precedent TallyAttendance and CarriedDebtStudents
	// already use for another feature's metadata, kept out of a
	// sessions.Service dependency so this package's constructor signature
	// does not grow for one lookup. sessionScope is the session's own
	// TeacherID/CenterID, needed by ReconcileSession to search
	// PeriodContainingDate under the session's own tenancy rather than the
	// acting caller's.
	SessionMeta(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (classID uuid.UUID, className string, sessionDate time.Time, sessionScope authctx.Scope, err error)
	// StudentSnapshot resolves one student's live contact id and
	// student/contact display names — the same students+contacts join
	// CarriedDebtStudents already performs, reused by ensureAdjustmentTarget
	// to seed a brand-new draft invoice's name snapshots.
	StudentSnapshot(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (contactID uuid.UUID, studentName, contactName string, err error)
	// SessionAttendance passes through AttendanceSource.SessionAttendance —
	// plan 04's entry point for discovering which students a post-close
	// reconciliation must consider.
	SessionAttendance(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]attendance.Record, error)
}

type gormRepository struct {
	db         *gorm.DB
	attendance AttendanceSource
}

// NewRepository returns the GORM-backed Repository. attendanceSvc is the
// sanctioned counting source TallyAttendance calls into; billing never
// aggregates attendance_records itself.
func NewRepository(db *gorm.DB, attendanceSvc AttendanceSource) Repository {
	return &gormRepository{db: db, attendance: attendanceSvc}
}

// scoped returns a billing_periods query bound to one center, further
// narrowed to one teacher's own rows unless the caller is the center's
// owner. Columns are table-qualified because callers join enrollments,
// students, contacts, and classes, which carry the same
// center_id/teacher_id column names.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("billing_periods.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("billing_periods.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// scopedRead is scoped()'s read-only sibling: the teacher filter is lifted
// for anyone with reports oversight (owner or reports.send holder), not
// just the owner. It backs ONLY the period read endpoints (GetPeriodRead,
// ListPeriodsRead) — every write keeps scoped(), so the delegated permission
// never reaches draft/close/void/adjustment paths.
func (r *gormRepository) scopedRead(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("billing_periods.center_id = ?", sc.CenterID)
	if !sc.ReportsOversight() {
		q = q.Where("billing_periods.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// invoiceScoped mirrors scoped for the invoices table.
func (r *gormRepository) invoiceScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	q := database.FromContext(ctx, r.db).Where("invoices.center_id = ?", sc.CenterID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("invoices.teacher_id = ?", sc.TeacherID)
	}
	return q
}

// getInvoiceRow is the shared bare-invoice read GetInvoice and
// GetInvoiceWithLines both need.
func (r *gormRepository) getInvoiceRow(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, error) {
	var inv Invoice
	err := r.invoiceScoped(ctx, sc).Where("invoices.id = ?", invoiceID).Take(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) CreatePeriod(ctx context.Context, p *Period) error {
	err := database.FromContext(ctx, r.db).Create(p).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicatePeriod
	}
	return err
}

func (r *gormRepository) GetPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*Period, error) {
	var p Period
	err := r.scoped(ctx, sc).Take(&p, "billing_periods.id = ?", periodID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPeriodNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) GetPeriodRead(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*PeriodWithTeacher, error) {
	var row PeriodWithTeacher
	err := r.scopedRead(ctx, sc).Model(&Period{}).
		Select("billing_periods.*, teachers.full_name AS teacher_name").
		Joins("JOIN teachers ON teachers.id = billing_periods.teacher_id").
		Take(&row, "billing_periods.id = ?", periodID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPeriodNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) ListPeriodsRead(ctx context.Context, sc authctx.Scope, p pagination.Params) ([]PeriodWithTeacher, int64, error) {
	q := r.scopedRead(ctx, sc).Model(&Period{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []PeriodWithTeacher
	// The id tie-breaker keeps pagination stable when several teachers hold
	// periods for the same month (identical period_start). It must run as a
	// scope after p.Scope: scopes apply at Find time, so a chained Order here
	// would land before the pagination sort and hijack the primary order.
	err := q.
		Select("billing_periods.*, teachers.full_name AS teacher_name").
		Joins("JOIN teachers ON teachers.id = billing_periods.teacher_id").
		Scopes(p.Scope, func(db *gorm.DB) *gorm.DB { return db.Order("billing_periods.id") }).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) ClassReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	readFrag, _ := classscope.ReadExists("classes.id")
	var rows []struct{ Readable bool }
	err := database.FromContext(ctx, r.db).
		Table("classes").
		Select(readFrag+" AS readable", sc.TeacherID, sc.CenterID).
		Where("classes.id = ? AND classes.center_id = ? AND classes.deleted_at IS NULL", classID, sc.CenterID).
		Find(&rows).Error
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, ErrClassNotFound
	}
	return rows[0].Readable, nil
}

// classInvoiceLinesExist reaches a period's class charges the same way
// statements' class totals do: invoice_lines carry no class_id, so the class
// is resolved through the billed enrollment. Void invoices don't count — a
// period whose only class charges were voided has nothing left to send.
const classInvoiceLinesExist = `EXISTS (
	SELECT 1 FROM invoices i
	JOIN invoice_lines il ON il.invoice_id = i.id AND il.center_id = i.center_id
	JOIN enrollments e    ON e.id = il.enrollment_id AND e.center_id = il.center_id
	WHERE i.period_id = billing_periods.id
	  AND i.status <> ?
	  AND e.class_id = ?)`

func (r *gormRepository) ListPeriodsClassRead(ctx context.Context, sc authctx.Scope, classID uuid.UUID, p pagination.Params) ([]PeriodWithTeacher, int64, error) {
	q := database.FromContext(ctx, r.db).Model(&Period{}).
		Where("billing_periods.center_id = ?", sc.CenterID).
		Where(classInvoiceLinesExist, InvoiceVoid, classID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []PeriodWithTeacher
	err := q.
		Select("billing_periods.*, teachers.full_name AS teacher_name").
		Joins("JOIN teachers ON teachers.id = billing_periods.teacher_id").
		Scopes(p.Scope, func(db *gorm.DB) *gorm.DB { return db.Order("billing_periods.id") }).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetPeriodByYearMonth resolves the CALLER'S OWN period for a month. It backs
// EnsurePeriod's duplicate branch, where the conflict comes from the
// (teacher_id, year, month) unique index — so the existing row is by
// definition the caller's own. The teacher filter is pinned unconditionally:
// scoped() would lift it for an owner, and in a center holding several
// teachers' periods for the same month the owner would get an arbitrary
// teacher's period back instead of their own.
func (r *gormRepository) GetPeriodByYearMonth(ctx context.Context, sc authctx.Scope, year, month int16) (*Period, error) {
	var p Period
	err := database.FromContext(ctx, r.db).
		Where("billing_periods.center_id = ?", sc.CenterID).
		Where("billing_periods.teacher_id = ?", sc.TeacherID).
		Take(&p, "billing_periods.year = ? AND billing_periods.month = ?", year, month).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPeriodNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) PreviousClosedPeriod(ctx context.Context, sc authctx.Scope, before time.Time) (*Period, error) {
	var p Period
	err := r.scoped(ctx, sc).
		Where("billing_periods.status = ?", PeriodClosed).
		Where("billing_periods.period_end < ?", before).
		Order("billing_periods.period_end DESC").
		Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// tallyMetadataRow is the enrollment/student/contact/class join billing owns
// for TallyAttendance — who the enrollment belongs to and what to charge,
// never a re-derivation of whether a session was billable.
type tallyMetadataRow struct {
	EnrollmentID   uuid.UUID
	StudentID      uuid.UUID
	ContactID      uuid.UUID
	StudentName    string
	ContactName    string
	ClassID        uuid.UUID
	ClassName      string
	ClassStartDate time.Time
	UnitPrice      int64
}

func (r *gormRepository) TallyAttendance(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) ([]AttendanceTally, error) {
	period, err := r.GetPeriod(ctx, sc, periodID)
	if err != nil {
		return nil, err
	}

	counts, err := r.attendance.TallyByEnrollment(ctx, sc, period.PeriodStart, period.PeriodEnd)
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return nil, nil
	}

	enrollmentIDs := make([]uuid.UUID, len(counts))
	countByEnrollment := make(map[uuid.UUID]attendance.EnrollmentTally, len(counts))
	for i, c := range counts {
		enrollmentIDs[i] = c.EnrollmentID
		countByEnrollment[c.EnrollmentID] = c
	}

	// No deleted_at filter on enrollments/students/contacts/classes: a
	// student or class that was later removed must still resolve for a
	// period where they were billed, mirroring
	// attendance.Repository.StudentNames's same deliberate omission.
	// center_id (not teacher_id) join conditions let an owner's recompute
	// still resolve names for a member's roster.
	q := database.FromContext(ctx, r.db).
		Table("enrollments").
		Select(`enrollments.id AS enrollment_id,
			enrollments.student_id AS student_id,
			students.contact_id AS contact_id,
			students.full_name AS student_name,
			contacts.full_name AS contact_name,
			enrollments.class_id AS class_id,
			classes.name AS class_name,
			classes.start_date AS class_start_date,
			enrollments.unit_price AS unit_price`).
		Joins("JOIN students ON students.id = enrollments.student_id AND students.center_id = enrollments.center_id").
		Joins("JOIN contacts ON contacts.id = students.contact_id AND contacts.center_id = students.center_id").
		Joins("JOIN classes ON classes.id = enrollments.class_id AND classes.center_id = enrollments.center_id").
		Where("enrollments.center_id = ? AND enrollments.id IN ?", sc.CenterID, enrollmentIDs)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("enrollments.teacher_id = ?", sc.TeacherID)
	}
	var meta []tallyMetadataRow
	err = q.Find(&meta).Error
	if err != nil {
		return nil, err
	}

	tallies := make([]AttendanceTally, 0, len(meta))
	for _, m := range meta {
		c := countByEnrollment[m.EnrollmentID]
		tallies = append(tallies, AttendanceTally{
			EnrollmentID:   m.EnrollmentID,
			StudentID:      m.StudentID,
			ContactID:      m.ContactID,
			StudentName:    m.StudentName,
			ContactName:    m.ContactName,
			ClassID:        m.ClassID,
			ClassName:      m.ClassName,
			ClassStartDate: m.ClassStartDate,
			UnitPrice:      m.UnitPrice,
			BillableCount:  c.BillableCount,
			AbsentCount:    c.AbsentCount,
			PresentCount:   c.PresentCount,
		})
	}
	return tallies, nil
}

func (r *gormRepository) OpeningBalances(ctx context.Context, sc authctx.Scope, prevPeriodID uuid.UUID, studentIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(studentIDs))
	if len(studentIDs) == 0 {
		return out, nil
	}
	type row struct {
		StudentID  uuid.UUID
		TotalDue   int64
		PaidAmount int64
	}
	q := database.FromContext(ctx, r.db).
		Table("invoices").
		Select("student_id, total_due, paid_amount").
		Where("center_id = ? AND period_id = ? AND student_id IN ? AND status <> ?", sc.CenterID, prevPeriodID, studentIDs, InvoiceVoid)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	var rows []row
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, rr := range rows {
		outstanding := rr.TotalDue - rr.PaidAmount
		if outstanding < 0 {
			outstanding = 0
		}
		out[rr.StudentID] = outstanding
	}
	return out, nil
}

func (r *gormRepository) TeacherTimezone(ctx context.Context, teacherID uuid.UUID) (string, error) {
	var timezones []string
	err := database.FromContext(ctx, r.db).
		Table("teachers").
		Where("id = ? AND deleted_at IS NULL", teacherID).
		Pluck("timezone", &timezones).Error
	if err != nil {
		return "", err
	}
	if len(timezones) == 0 {
		return "", ErrTeacherNotFound
	}
	return timezones[0], nil
}

func (r *gormRepository) AdjustmentTotals(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]int64, error) {
	type row struct {
		InvoiceID uuid.UUID
		Total     int64
	}
	q := database.FromContext(ctx, r.db).
		Table("invoice_adjustments").
		Select("invoice_adjustments.invoice_id AS invoice_id, SUM(invoice_adjustments.amount) AS total").
		Joins("JOIN invoices ON invoices.id = invoice_adjustments.invoice_id AND invoices.center_id = invoice_adjustments.center_id").
		Where("invoice_adjustments.center_id = ? AND invoices.period_id = ? AND invoice_adjustments.deleted_at IS NULL", sc.CenterID, periodID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("invoice_adjustments.teacher_id = ?", sc.TeacherID)
	}
	var rows []row
	if err := q.Group("invoice_adjustments.invoice_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int64, len(rows))
	for _, rr := range rows {
		out[rr.InvoiceID] = rr.Total
	}
	return out, nil
}

func (r *gormRepository) CarriedDebtStudents(ctx context.Context, sc authctx.Scope, prevPeriodID uuid.UUID) ([]CarriedDebtStudent, error) {
	// Joins to students/contacts (not invoices.contact_id/student_name) so
	// the carried balance follows the student's live contact, matching the
	// documented draft-time behaviour: only an issued invoice freezes the
	// contact snapshot.
	q := database.FromContext(ctx, r.db).
		Table("invoices").
		Select(`invoices.student_id AS student_id,
			students.contact_id AS contact_id,
			students.full_name AS student_name,
			contacts.full_name AS contact_name,
			(invoices.total_due - invoices.paid_amount) AS outstanding`).
		Joins("JOIN students ON students.id = invoices.student_id AND students.center_id = invoices.center_id").
		Joins("JOIN contacts ON contacts.id = students.contact_id AND contacts.center_id = students.center_id").
		Where("invoices.center_id = ? AND invoices.period_id = ? AND invoices.status <> ? AND (invoices.total_due - invoices.paid_amount) > 0",
			sc.CenterID, prevPeriodID, InvoiceVoid)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("invoices.teacher_id = ?", sc.TeacherID)
	}
	var rows []CarriedDebtStudent
	err := q.Find(&rows).Error
	return rows, err
}

// invoiceUpsertColumns are the invoice columns Draft is allowed to overwrite
// on an existing row: contact/name snapshots and every money column. status,
// paid_amount, void_reason, voided_at, and created_at never appear here — a
// re-draft never touches collection state or an invoice's issued/void
// history.
var invoiceUpsertColumns = []string{
	"contact_id", "student_name", "contact_name",
	"opening_balance", "current_charge", "adjustment_total", "total_due",
}

func (r *gormRepository) UpsertInvoice(ctx context.Context, inv *Invoice) error {
	assignments := make(map[string]any, len(invoiceUpsertColumns)+1)
	for _, col := range invoiceUpsertColumns {
		assignments[col] = gorm.Expr("excluded." + col)
	}
	assignments["updated_at"] = gorm.Expr("now()")

	res := database.FromContext(ctx, r.db).
		Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "period_id"}, {Name: "student_id"}},
				DoUpdates: clause.Assignments(assignments),
				// Only a still-draft row may be overwritten (D7: issued and
				// later are immutable). Guards the same invariant even under
				// a race with a concurrent issue happening mid-transaction.
				Where: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "invoices.status = ?", Vars: []interface{}{InvoiceDraft}},
				}},
			},
			// Empty Returning requests RETURNING * so inv is refreshed with
			// the row's real id/status after the statement — required when
			// the conflict resolves to a pre-existing row whose id differs
			// from the candidate this call was invoked with.
			clause.Returning{},
		).
		Create(inv)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInvoiceNotDraft
	}
	return nil
}

func (r *gormRepository) UpsertInvoiceLine(ctx context.Context, line *InvoiceLine) error {
	res := database.FromContext(ctx, r.db).
		Clauses(
			clause.OnConflict{
				Columns: []clause.Column{{Name: "invoice_id"}, {Name: "enrollment_id"}},
				DoUpdates: clause.Assignments(map[string]any{
					"class_name":     gorm.Expr("excluded.class_name"),
					"billable_count": gorm.Expr("excluded.billable_count"),
					"absent_count":   gorm.Expr("excluded.absent_count"),
					"unit_price":     gorm.Expr("excluded.unit_price"),
					"amount":         gorm.Expr("excluded.amount"),
				}),
			},
			clause.Returning{},
		).
		Create(line)
	return res.Error
}

func (r *gormRepository) ZeroUnmatchedLines(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID, keepEnrollmentIDs []uuid.UUID) error {
	q := database.FromContext(ctx, r.db).
		Model(&InvoiceLine{}).
		Where("center_id = ? AND invoice_id = ?", sc.CenterID, invoiceID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	if len(keepEnrollmentIDs) > 0 {
		q = q.Where("enrollment_id NOT IN ?", keepEnrollmentIDs)
	}
	return q.Updates(map[string]any{"billable_count": 0, "absent_count": 0, "amount": 0}).Error
}

func (r *gormRepository) ListInvoices(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) ([]Invoice, error) {
	q := database.FromContext(ctx, r.db).
		Where("center_id = ? AND period_id = ?", sc.CenterID, periodID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	var rows []Invoice
	err := q.Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetInvoiceWithLines(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, []InvoiceLine, error) {
	inv, err := r.getInvoiceRow(ctx, sc, invoiceID)
	if err != nil {
		return nil, nil, err
	}
	var lines []InvoiceLine
	// invoiceID already pins the exact row inv's own scope check just
	// validated; center_id here is a defense-in-depth belt, not a second
	// authorization gate.
	err = database.FromContext(ctx, r.db).
		Where("center_id = ? AND invoice_id = ?", inv.CenterID, invoiceID).
		Order("created_at").
		Find(&lines).Error
	if err != nil {
		return nil, nil, err
	}
	return inv, lines, nil
}

func (r *gormRepository) LockPeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*Period, error) {
	var p Period
	err := r.scoped(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Take(&p, "billing_periods.id = ?", periodID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPeriodNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) IssueDraftInvoices(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (int64, error) {
	q := database.FromContext(ctx, r.db).
		Model(&Invoice{}).
		Where("center_id = ? AND period_id = ? AND status = ?", sc.CenterID, periodID, InvoiceDraft)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	res := q.Updates(map[string]any{"status": InvoiceIssued, "updated_at": gorm.Expr("now()")})
	return res.RowsAffected, res.Error
}

// emptyInvoiceVoidReason is void_reason for step 5's void-empty rule: a
// drafted invoice with zero current_charge, zero opening_balance, and zero
// adjustment_total — a class with no billable sessions this period and no
// carried debt (PRD §5).
const emptyInvoiceVoidReason = "no charges or carried debt for this period"

func (r *gormRepository) VoidInvoices(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (int64, error) {
	q := database.FromContext(ctx, r.db).
		Model(&Invoice{}).
		Where("center_id = ? AND period_id = ? AND status = ? AND current_charge = 0 AND opening_balance = 0 AND adjustment_total = 0",
			sc.CenterID, periodID, InvoiceDraft)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	res := q.Updates(map[string]any{
		"status":      InvoiceVoid,
		"void_reason": emptyInvoiceVoidReason,
		"voided_at":   gorm.Expr("now()"),
		"updated_at":  gorm.Expr("now()"),
	})
	return res.RowsAffected, res.Error
}

func (r *gormRepository) ClosePeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, closedAt time.Time) error {
	res := r.scoped(ctx, sc).
		Model(&Period{}).
		Where("billing_periods.id = ? AND billing_periods.status = ?", periodID, PeriodOpen).
		Updates(map[string]any{"status": PeriodClosed, "closed_at": closedAt, "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errPeriodStatusChanged
	}
	return nil
}

func (r *gormRepository) GetInvoice(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, error) {
	return r.getInvoiceRow(ctx, sc, invoiceID)
}

func (r *gormRepository) LockInvoice(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) (*Invoice, error) {
	var inv Invoice
	err := r.invoiceScoped(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("invoices.id = ?", invoiceID).
		Take(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) VoidInvoice(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID, reason string, at time.Time) error {
	res := r.invoiceScoped(ctx, sc).
		Model(&Invoice{}).
		Where("invoices.id = ? AND invoices.status IN ?", invoiceID, []string{InvoiceIssued, InvoicePartiallyPaid}).
		Updates(map[string]any{
			"status":      InvoiceVoid,
			"void_reason": reason,
			"voided_at":   at,
			"updated_at":  gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrInvoiceNotFound
	}
	return nil
}

func (r *gormRepository) CreateAdjustment(ctx context.Context, adj *InvoiceAdjustment) error {
	return database.FromContext(ctx, r.db).Create(adj).Error
}

func (r *gormRepository) ListAdjustments(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) ([]InvoiceAdjustment, error) {
	q := database.FromContext(ctx, r.db).
		Where("center_id = ? AND invoice_id = ? AND deleted_at IS NULL", sc.CenterID, invoiceID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("teacher_id = ?", sc.TeacherID)
	}
	var rows []InvoiceAdjustment
	err := q.Order("created_at").Find(&rows).Error
	return rows, err
}

func (r *gormRepository) AdjustmentsBySourcePeriod(ctx context.Context, sc authctx.Scope, studentID, periodID uuid.UUID) (int64, error) {
	q := database.FromContext(ctx, r.db).
		Table("invoice_adjustments").
		Select("COALESCE(SUM(invoice_adjustments.amount), 0)").
		Joins("JOIN invoices ON invoices.id = invoice_adjustments.invoice_id AND invoices.center_id = invoice_adjustments.center_id").
		Joins("JOIN class_sessions ON class_sessions.id = invoice_adjustments.source_session_id AND class_sessions.center_id = invoice_adjustments.center_id").
		Joins("JOIN billing_periods ON billing_periods.id = ? AND billing_periods.center_id = invoice_adjustments.center_id", periodID).
		Where("invoice_adjustments.center_id = ? AND invoices.student_id = ? AND invoice_adjustments.deleted_at IS NULL", sc.CenterID, studentID).
		Where("class_sessions.session_date BETWEEN billing_periods.period_start AND billing_periods.period_end")
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("invoice_adjustments.teacher_id = ?", sc.TeacherID)
	}
	var total int64
	row := q.Row()
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *gormRepository) RecalcInvoiceTotals(ctx context.Context, sc authctx.Scope, invoiceID uuid.UUID) error {
	// The trailing (? OR invoices.teacher_id = ?) lets an owner's sc.CenterWideFor(authctx.PermBillingViewAll)
	// short-circuit the teacher_id check via SQL OR, mirroring scoped()'s Go
	// conditional in a statement that must stay one atomic UPDATE.
	res := database.FromContext(ctx, r.db).Exec(`
		UPDATE invoices
		SET adjustment_total = adj.total,
		    total_due = invoices.opening_balance + invoices.current_charge + adj.total,
		    status = CASE
		        WHEN invoices.status IN (?, ?) THEN invoices.status
		        WHEN invoices.paid_amount <= 0 THEN ?
		        WHEN invoices.paid_amount < (invoices.opening_balance + invoices.current_charge + adj.total) THEN ?
		        ELSE ?
		    END,
		    updated_at = now()
		FROM (
		    SELECT COALESCE(SUM(amount), 0) AS total
		    FROM invoice_adjustments
		    WHERE invoice_id = ? AND center_id = ? AND deleted_at IS NULL
		) adj
		WHERE invoices.id = ? AND invoices.center_id = ? AND (? OR invoices.teacher_id = ?)
	`,
		InvoiceDraft, InvoiceVoid, InvoiceIssued, InvoicePartiallyPaid, InvoicePaid,
		invoiceID, sc.CenterID,
		invoiceID, sc.CenterID, sc.CenterWideFor(authctx.PermBillingViewAll), sc.TeacherID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrInvoiceNotFound
	}
	return nil
}

func (r *gormRepository) PeriodContainingDate(ctx context.Context, sc authctx.Scope, on time.Time) (*Period, error) {
	var p Period
	err := r.scoped(ctx, sc).
		Where("billing_periods.status = ?", PeriodClosed).
		Where("billing_periods.period_start <= ? AND billing_periods.period_end >= ?", on, on).
		Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) NextOpenPeriod(ctx context.Context, sc authctx.Scope, afterPeriodEnd time.Time) (*Period, error) {
	var p Period
	err := r.scoped(ctx, sc).
		Where("billing_periods.status = ?", PeriodOpen).
		Where("billing_periods.period_start > ?", afterPeriodEnd).
		Order("billing_periods.period_start ASC").
		Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *gormRepository) LiveBillableCounts(ctx context.Context, enrollmentIDs []uuid.UUID, period *Period) (map[uuid.UUID]int, error) {
	out := make(map[uuid.UUID]int, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	want := make(map[uuid.UUID]bool, len(enrollmentIDs))
	for _, eid := range enrollmentIDs {
		want[eid] = true
	}
	// period's own anchors, never a caller-supplied scope: an owner
	// recomputing one member's closed period must see only that member's
	// attendance, not the whole center's.
	periodScope := authctx.Scope{TeacherID: period.TeacherID, CenterID: period.CenterID}
	tallies, err := r.attendance.TallyByEnrollment(ctx, periodScope, period.PeriodStart, period.PeriodEnd)
	if err != nil {
		return nil, err
	}
	for _, t := range tallies {
		if want[t.EnrollmentID] {
			out[t.EnrollmentID] = t.BillableCount
		}
	}
	return out, nil
}

func (r *gormRepository) SessionMeta(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (uuid.UUID, string, time.Time, authctx.Scope, error) {
	type row struct {
		ClassID     uuid.UUID
		ClassName   string
		SessionDate time.Time
		TeacherID   uuid.UUID
		CenterID    uuid.UUID
	}
	q := database.FromContext(ctx, r.db).
		Table("class_sessions").
		Select(`class_sessions.class_id AS class_id,
			classes.name AS class_name,
			class_sessions.session_date AS session_date,
			class_sessions.teacher_id AS teacher_id,
			class_sessions.center_id AS center_id`).
		Joins("JOIN classes ON classes.id = class_sessions.class_id AND classes.center_id = class_sessions.center_id").
		Where("class_sessions.center_id = ? AND class_sessions.id = ? AND class_sessions.deleted_at IS NULL", sc.CenterID, sessionID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("class_sessions.teacher_id = ?", sc.TeacherID)
	}
	var rows []row
	err := q.Find(&rows).Error
	if err != nil {
		return uuid.UUID{}, "", time.Time{}, authctx.Scope{}, err
	}
	if len(rows) == 0 {
		return uuid.UUID{}, "", time.Time{}, authctx.Scope{}, ErrSessionNotFound
	}
	sessionScope := authctx.Scope{TeacherID: rows[0].TeacherID, CenterID: rows[0].CenterID}
	return rows[0].ClassID, rows[0].ClassName, rows[0].SessionDate, sessionScope, nil
}

func (r *gormRepository) StudentSnapshot(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (uuid.UUID, string, string, error) {
	type row struct {
		ContactID   uuid.UUID
		StudentName string
		ContactName string
	}
	q := database.FromContext(ctx, r.db).
		Table("students").
		Select("students.contact_id AS contact_id, students.full_name AS student_name, contacts.full_name AS contact_name").
		Joins("JOIN contacts ON contacts.id = students.contact_id AND contacts.center_id = students.center_id").
		Where("students.center_id = ? AND students.id = ?", sc.CenterID, studentID)
	if !sc.CenterWideFor(authctx.PermBillingViewAll) {
		q = q.Where("students.teacher_id = ?", sc.TeacherID)
	}
	var rows []row
	err := q.Find(&rows).Error
	if err != nil {
		return uuid.UUID{}, "", "", err
	}
	if len(rows) == 0 {
		return uuid.UUID{}, "", "", ErrStudentNotFound
	}
	return rows[0].ContactID, rows[0].StudentName, rows[0].ContactName, nil
}

func (r *gormRepository) SessionAttendance(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]attendance.Record, error) {
	return r.attendance.SessionAttendance(ctx, sc, sessionID)
}
