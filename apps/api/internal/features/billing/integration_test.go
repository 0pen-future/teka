//go:build integration

package billing_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// newIntegrationDeps wires the real dependency chain router.go uses: billing
// consumes attendance (batched per-enrollment tallies) through
// attendance.Service, and joins its own enrollment/student/contact/class
// metadata directly — it never imports enrollments' or attendance's
// repository types.
func newIntegrationDeps(t *testing.T) (*billing.Service, billing.Repository, *enrollments.Service, *sessions.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	billingRepo := billing.NewRepository(db, attendanceSvc)
	billingSvc := billing.NewService(billingRepo, txMgr, sessionsSvc, enrollmentsSvc)
	return billingSvc, billingRepo, enrollmentsSvc, sessionsSvc, db
}

// TestTallyAttendancePlan03ContractExclusions seeds one teacher, one contact,
// one student, one class with two held+confirmed sessions, then adds a
// cancelled session, an unconfirmed session, and a soft-deleted attendance
// record — each with a stray attendance_records row attached directly
// (bypassing the Confirm workflow) — and asserts TallyAttendance's counts
// reflect only the two legitimate rows.
//
// These are plan 03 contract assertions, not tests of billing's own SQL:
// billing writes no aggregate over attendance_records, so a failure here
// means attendance.Service.TallyByEnrollment's predicate changed, not that
// billing needs its own filter.
func TestTallyAttendancePlan03ContractExclusions(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	// Property: two held+confirmed sessions inside the period are the
	// baseline billable count plan 03 hands billing.
	held1 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	held2 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-13"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, held1.ID, student.ID, enrollment.ID)
	testutil.AttendanceRecord(t, db, teacher.ID, held2.ID, student.ID, enrollment.ID,
		testutil.WithAttendanceStatus(attendance.StatusAbsent))

	// Property 1: a cancelled session never contributes, even with a stray
	// attendance row against it (PRD §5 edge; the CHECK at
	// docs/schema_design.sql:212 already forbids a confirmed cancelled
	// session — this fixture proves the counting predicate also excludes it).
	cancelled := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-20"),
		testutil.WithSessionStatus(sessions.StatusCancelled), testutil.WithSessionCancelReason("nghỉ lễ"))
	testutil.AttendanceRecord(t, db, teacher.ID, cancelled.ID, student.ID, enrollment.ID)

	// Property 2: an unconfirmed session never contributes
	// (docs/schema_design.sql:203).
	unconfirmed := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-22"))
	testutil.AttendanceRecord(t, db, teacher.ID, unconfirmed.ID, student.ID, enrollment.ID)

	// Property 3: a soft-deleted attendance record never contributes (schema
	// note (j), docs/schema_design.sql:526).
	heldButRecordDeleted := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-27"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	softDeleted := testutil.AttendanceRecord(t, db, teacher.ID, heldButRecordDeleted.ID, student.ID, enrollment.ID)
	require.NoError(t, db.Delete(softDeleted).Error)

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)

	tallies, err := repo.TallyAttendance(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.Len(t, tallies, 1, "one enrollment must produce exactly one tally row")

	tally := tallies[0]
	require.Equal(t, enrollment.ID, tally.EnrollmentID)
	require.Equal(t, 2, tally.BillableCount,
		"only the two held+confirmed rows count; cancelled, unconfirmed, and soft-deleted rows must not")
	require.Equal(t, 1, tally.AbsentCount)
	require.Equal(t, 1, tally.PresentCount)
	require.Equal(t, student.ID, tally.StudentID)
	require.Equal(t, contact.ID, tally.ContactID)
	require.Equal(t, class.Name, tally.ClassName)
	require.Equal(t, enrollment.UnitPrice, tally.UnitPrice)
}

// TestMidPeriodJoinerNeedsNoRosterDateFilter proves Architecture property 4: a
// student who joins mid-period is billed starting from their first confirmed
// session only because plan 03's Confirm only ever writes attendance_records
// for enrollments enrollments.ActiveOn returns as of the session date — never
// because billing adds its own enrollment date-range comparison.
func TestMidPeriodJoinerNeedsNoRosterDateFilter(t *testing.T) {
	t.Parallel()
	svc, repo, enrollmentsSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	// Joins on the 15th, mid-period.
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-15"))

	before := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-08"))
	after := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-15"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, after.ID, student.ID, enrollment.ID)

	// Verify roster membership independently through the sanctioned query:
	// the joiner is not on the roster for the session before their join date,
	// and is on it (inclusive boundary) for the session on their join date.
	sc := testutil.ScopeFor(t, db, teacher.ID)
	rosterBefore, err := enrollmentsSvc.ActiveOn(ctx, sc, class.ID, before.SessionDate)
	require.NoError(t, err)
	require.Empty(t, rosterBefore, "the joiner must not be on the roster before their enrollment begins")
	rosterOnStart, err := enrollmentsSvc.ActiveOn(ctx, sc, class.ID, after.SessionDate)
	require.NoError(t, err)
	require.Len(t, rosterOnStart, 1)
	require.Equal(t, enrollment.ID, rosterOnStart[0].ID)

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	tallies, err := repo.TallyAttendance(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	require.Equal(t, 1, tallies[0].BillableCount,
		"only the single post-join confirmed session bills; no attendance row exists for the earlier session because plan 03 never creates one, not because billing filters by date")
}

// TestEnsurePeriodIdempotentAcrossCalls proves POST /billing-periods'
// idempotency contract at the repository/service layer: two calls for the
// same (teacher, year, month) converge on one row, not a 409 or a duplicate.
func TestEnsurePeriodIdempotentAcrossCalls(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	first, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 4)
	require.NoError(t, err)
	second, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 4)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	var count int64
	require.NoError(t, db.Table("billing_periods").
		Where("teacher_id = ? AND year = ? AND month = ? AND deleted_at IS NULL", teacher.ID, 2026, 4).
		Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// TestGetPeriodCrossTenantIsNotFound proves no cross-teacher existence leak:
// another teacher's billing period must read as 404, not 403 or 200.
func TestGetPeriodCrossTenantIsNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	stranger, _ := testutil.Teacher(t, db)

	period, err := svc.EnsurePeriod(ctx, owner.ID, 2026, 5)
	require.NoError(t, err)

	_, err = svc.GetPeriod(ctx, stranger.ID, period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"another teacher's period must read as 404, not 403 or 200")
}

// seededDraftFixture is one teacher with one student billable for two
// confirmed sessions inside an open January 2026 period — the common
// starting point every draft test below builds on.
type seededDraftFixture struct {
	teacher    *teachers.Teacher
	student    *students.Student
	enrollment *enrollments.Enrollment
	period     *billing.Period
}

func seedDraftFixture(t *testing.T, db *gorm.DB, svc *billing.Service) seededDraftFixture {
	t.Helper()
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	held1 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	held2 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-13"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, held1.ID, student.ID, enrollment.ID)
	testutil.AttendanceRecord(t, db, teacher.ID, held2.ID, student.ID, enrollment.ID)

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)

	return seededDraftFixture{teacher: teacher, student: student, enrollment: enrollment, period: period}
}

// TestDraftTwiceProducesNoDuplicateRows proves R4's idempotency contract at
// the database level: drafting an unchanged period twice converges on the
// same invoice and invoice_line rows rather than inserting a second copy.
func TestDraftTwiceProducesNoDuplicateRows(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	first, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.Len(t, first.Invoices, 1)
	require.NotNil(t, first.Invoices[0].InvoiceID)

	second, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.Len(t, second.Invoices, 1)
	require.Equal(t, *first.Invoices[0].InvoiceID, *second.Invoices[0].InvoiceID,
		"re-drafting must keep the same invoice id")

	var invoiceCount, lineCount int64
	require.NoError(t, db.Table("invoices").Where("period_id = ?", fx.period.ID).Count(&invoiceCount).Error)
	require.NoError(t, db.Table("invoice_lines").
		Where("invoice_id = ?", *first.Invoices[0].InvoiceID).Count(&lineCount).Error)
	require.EqualValues(t, 1, invoiceCount, "must not duplicate the invoice row")
	require.EqualValues(t, 1, lineCount, "must not duplicate the invoice_line row")
}

// TestDraftPreservesManualAdjustmentAndFoldsItIntoTotalDue proves an
// invoice_adjustments row created between two drafts is never touched by
// UpsertInvoice/UpsertInvoiceLine, and that its amount is folded into
// total_due on the next draft — the CHECK
// total_due = opening_balance + current_charge + adjustment_total must hold.
func TestDraftPreservesManualAdjustmentAndFoldsItIntoTotalDue(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	first, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	invoiceID := *first.Invoices[0].InvoiceID
	require.EqualValues(t, 200_000, first.Invoices[0].TotalDue)

	adjustment := &billing.InvoiceAdjustment{
		ID: id.New(), TeacherID: fx.teacher.ID, InvoiceID: invoiceID,
		Amount: -20_000, Reason: "hoàn tiền do nghỉ có phép",
	}
	require.NoError(t, db.Create(adjustment).Error)

	second, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, invoiceID, *second.Invoices[0].InvoiceID)
	require.EqualValues(t, -20_000, second.Invoices[0].AdjustmentTotal)
	require.EqualValues(t, 180_000, second.Invoices[0].TotalDue,
		"total_due must fold the surviving adjustment: 200000-20000")

	var adjustmentCount int64
	require.NoError(t, db.Table("invoice_adjustments").
		Where("invoice_id = ? AND deleted_at IS NULL", invoiceID).Count(&adjustmentCount).Error)
	require.EqualValues(t, 1, adjustmentCount, "re-drafting must never delete or duplicate the adjustment row")
}

// TestDraftZeroesLineWhenAttendanceRecordIsSoftDeletedInsteadOfDeletingIt
// proves ZeroUnmatchedLines never hard-deletes an invoice_line — it zeroes
// counts/amount so the invoice's own history stays explainable — and that
// the invoice's total_due keeps tracking SUM(lines.amount).
func TestDraftZeroesLineWhenAttendanceRecordIsSoftDeletedInsteadOfDeletingIt(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	first, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	invoiceID := *first.Invoices[0].InvoiceID
	require.Len(t, first.Invoices[0].Lines, 1)
	lineEnrollmentID := first.Invoices[0].Lines[0].EnrollmentID

	var records []attendance.Record
	require.NoError(t, db.Where("enrollment_id = ?", fx.enrollment.ID).Find(&records).Error)
	require.NotEmpty(t, records)
	for _, r := range records {
		require.NoError(t, db.Delete(&r).Error)
	}

	second, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, invoiceID, *second.Invoices[0].InvoiceID)
	require.Empty(t, second.Invoices[0].Lines,
		"a line with billable_count=0 and absent_count=0 must be omitted from the response")
	require.EqualValues(t, 0, second.Invoices[0].CurrentCharge)
	require.EqualValues(t, 0, second.Invoices[0].TotalDue)

	var line billing.InvoiceLine
	require.NoError(t, db.Where("invoice_id = ? AND enrollment_id = ?", invoiceID, lineEnrollmentID).
		First(&line).Error, "the line row must still exist, only zeroed, not hard-deleted")
	require.Equal(t, 0, line.BillableCount)
	require.Equal(t, 0, line.AbsentCount)
	require.EqualValues(t, 0, line.Amount)

	var lineCount int64
	require.NoError(t, db.Table("invoice_lines").Where("invoice_id = ?", invoiceID).Count(&lineCount).Error)
	require.EqualValues(t, 1, lineCount, "zeroing must not create a second row")

	var sum int64
	require.NoError(t, db.Table("invoice_lines").
		Where("invoice_id = ?", invoiceID).Select("COALESCE(SUM(amount), 0)").Row().Scan(&sum))
	require.EqualValues(t, sum, second.Invoices[0].CurrentCharge,
		"total_due's current_charge must keep matching SUM(invoice_lines.amount)")
}

// TestDraftOnClosedPeriodIsConflictAndWritesNothing proves a closed period
// refuses Draft with 409 before ComputePeriod ever reaches a write.
func TestDraftOnClosedPeriodIsConflictAndWritesNothing(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	require.NoError(t, db.Table("billing_periods").Where("id = ?", fx.period.ID).
		Update("status", billing.PeriodClosed).Error)

	_, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var invoiceCount int64
	require.NoError(t, db.Table("invoices").Where("period_id = ?", fx.period.ID).Count(&invoiceCount).Error)
	require.EqualValues(t, 0, invoiceCount, "a refused draft must write nothing")
}

// TestDraftAgainstIssuedInvoiceIsConflictAndLeavesItUntouched proves the
// upsert's WHERE invoices.status = 'draft' guard refuses to touch an invoice
// that already moved past draft, and that the transaction rolls back the
// whole request rather than partially applying it.
func TestDraftAgainstIssuedInvoiceIsConflictAndLeavesItUntouched(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	issued := &billing.Invoice{
		ID: id.New(), TeacherID: fx.teacher.ID, PeriodID: fx.period.ID,
		StudentID: fx.student.ID, ContactID: fx.student.ContactID,
		StudentName: fx.student.FullName, ContactName: "Fixture Contact",
		CurrentCharge: 200_000, TotalDue: 200_000, Status: billing.InvoiceIssued,
	}
	require.NoError(t, db.Create(issued).Error)

	_, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var reloaded billing.Invoice
	require.NoError(t, db.Where("id = ?", issued.ID).First(&reloaded).Error)
	require.Equal(t, billing.InvoiceIssued, reloaded.Status, "the issued invoice's status must be untouched")
	require.EqualValues(t, 200_000, reloaded.TotalDue, "the issued invoice's total_due must be untouched")

	var invoiceCount int64
	require.NoError(t, db.Table("invoices").Where("period_id = ?", fx.period.ID).Count(&invoiceCount).Error)
	require.EqualValues(t, 1, invoiceCount, "nothing else must have been written")

	var lineCount int64
	require.NoError(t, db.Table("invoice_lines").Where("invoice_id = ?", issued.ID).Count(&lineCount).Error)
	require.EqualValues(t, 0, lineCount, "no lines must have been attached to the untouched invoice")
}

// hcmToday resolves "today" the same way Close and ListPending do: the
// current instant converted into the teacher's timezone (fixtures always use
// teachers.DefaultTimezone), then re-expressed as a DATE at UTC midnight.
// Close/void tests use it to build periods and sessions relative to real
// time instead of a fixed calendar month, so a past-vs-future split inside
// one period never depends on which month the suite happens to run in.
func hcmToday(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	require.NoError(t, err)
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// insertOpenPeriod writes a billing_periods row directly, bypassing
// EnsurePeriod's calendar-month constraint, so a test can set an open period
// window relative to real time (e.g. spanning both past and future days)
// without waiting for an actual month boundary.
func insertOpenPeriod(t *testing.T, db *gorm.DB, teacherID uuid.UUID, start, end time.Time) *billing.Period {
	t.Helper()
	p := &billing.Period{
		ID:          id.New(),
		TeacherID:   teacherID,
		Year:        int16(start.Year()), //nolint:gosec // calendar year, always in range
		Month:       int16(start.Month()),
		PeriodStart: start,
		PeriodEnd:   end,
		Status:      billing.PeriodOpen,
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

// TestCloseBlocksOnPastUnconfirmedSessionAndLeavesPeriodOpen proves R4's hard
// block (Architecture step 2): a past session without confirmed attendance —
// whether still 'planned' or already 'held' — refuses close with 409 and
// writes zero invoices, regardless of which of the two statuses it is in.
func TestCloseBlocksOnPastUnconfirmedSessionAndLeavesPeriodOpen(t *testing.T) {
	t.Parallel()
	for _, status := range []string{sessions.StatusHeld, sessions.StatusPlanned} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			svc, repo, _, _, db := newIntegrationDeps(t)
			ctx := context.Background()
			_, teacher := testutil.Teacher(t, db)
			contact := testutil.Contact(t, db, teacher.ID)
			today := hcmToday(t)
			periodStart := today.AddDate(0, 0, -10)
			periodEnd := today.AddDate(0, 0, 10)
			class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(periodStart.AddDate(0, 0, -30)))
			student := testutil.Student(t, db, teacher.ID, contact.ID)
			testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, periodStart.AddDate(0, 0, -30))
			period := insertOpenPeriod(t, db, teacher.ID, periodStart, periodEnd)

			overdue := testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -3),
				testutil.WithSessionStatus(status))

			_, err := svc.Close(ctx, teacher.ID, period.ID)
			require.Error(t, err)
			var blocked *billing.ErrUnconfirmedSessions
			require.ErrorAs(t, err, &blocked)
			require.Len(t, blocked.Sessions, 1)
			require.Equal(t, overdue.ID, blocked.Sessions[0].SessionID)
			require.Equal(t, status, blocked.Sessions[0].Status)

			reloaded, err := repo.GetPeriod(ctx, teacher.ID, period.ID)
			require.NoError(t, err)
			require.Equal(t, billing.PeriodOpen, reloaded.Status, "a blocked close must leave the period open")

			var invoiceCount int64
			require.NoError(t, db.Table("invoices").Where("period_id = ?", period.ID).Count(&invoiceCount).Error)
			require.EqualValues(t, 0, invoiceCount, "a blocked close must write zero invoices")
		})
	}
}

// TestCloseBlockedSessionsAgreeWithPendingFeed proves the by-construction
// guarantee from close.go: the session set a blocked close reports is
// exactly what GET /sessions/pending (the dashboard's own feed) returns over
// the same window, because both ultimately share
// sessions.Service.ListUnconfirmedInWindow's predicate.
func TestCloseBlockedSessionsAgreeWithPendingFeed(t *testing.T) {
	t.Parallel()
	svc, _, _, sessionsSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	today := hcmToday(t)
	periodStart := today.AddDate(0, 0, -10)
	periodEnd := today.AddDate(0, 0, 10)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(periodStart.AddDate(0, 0, -30)))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, periodStart.AddDate(0, 0, -30))
	period := insertOpenPeriod(t, db, teacher.ID, periodStart, periodEnd)

	held := testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -5),
		testutil.WithSessionStatus(sessions.StatusHeld))
	planned := testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -2))
	// Neither of these belongs in either feed.
	testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -4),
		testutil.WithSessionStatus(sessions.StatusCancelled), testutil.WithSessionCancelReason("nghỉ lễ"))
	testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -6),
		testutil.WithSessionAttendanceConfirmed(time.Now()))

	_, err := svc.Close(ctx, teacher.ID, period.ID)
	require.Error(t, err)
	var blocked *billing.ErrUnconfirmedSessions
	require.ErrorAs(t, err, &blocked)

	feed, err := sessionsSvc.ListPending(ctx, testutil.ScopeFor(t, db, teacher.ID), &periodStart, &periodEnd, 1000)
	require.NoError(t, err)

	closeIDs := make(map[uuid.UUID]struct{}, len(blocked.Sessions))
	for _, s := range blocked.Sessions {
		closeIDs[s.SessionID] = struct{}{}
	}
	feedIDs := make(map[uuid.UUID]struct{}, len(feed.Items))
	for _, item := range feed.Items {
		feedIDs[item.SessionID] = struct{}{}
	}
	require.Equal(t, feedIDs, closeIDs, "close's blocked-session set must agree with the dashboard's own pending feed")
	require.Len(t, closeIDs, 2)
	require.Contains(t, closeIDs, held.ID)
	require.Contains(t, closeIDs, planned.ID)
}

// TestCloseIgnoresCancelledAndWarnsOnFutureUnconfirmedSession proves two
// Architecture properties in one period: a cancelled session never blocks or
// bills (R4 edge), and a still-unconfirmed session dated after today but
// inside the period is reported as a non-blocking warning, not a 409 —
// closing early is legal.
func TestCloseIgnoresCancelledAndWarnsOnFutureUnconfirmedSession(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	today := hcmToday(t)
	periodStart := today.AddDate(0, 0, -10)
	periodEnd := today.AddDate(0, 0, 10)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(periodStart.AddDate(0, 0, -30)))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, periodStart.AddDate(0, 0, -30))
	period := insertOpenPeriod(t, db, teacher.ID, periodStart, periodEnd)

	confirmed := testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -5),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, confirmed.ID, student.ID, enrollment.ID)
	testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, -3),
		testutil.WithSessionStatus(sessions.StatusCancelled), testutil.WithSessionCancelReason("nghỉ lễ"))
	future := testutil.Session(t, db, teacher.ID, class.ID, today.AddDate(0, 0, 3))

	resp, err := svc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)
	require.EqualValues(t, 0, resp.VoidedCount)
	require.EqualValues(t, 100_000, resp.TotalDue, "only the single confirmed session bills; cancelled contributes nothing")
	require.Len(t, resp.Warnings.FutureUnconfirmedSessions, 1)
	require.Equal(t, future.ID, resp.Warnings.FutureUnconfirmedSessions[0].SessionID)

	reloaded, err := repo.GetPeriod(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.Equal(t, billing.PeriodClosed, reloaded.Status)
}

// TestCloseVariesTotalDuePerStudentAndClassWithNoSessionsAddsNoLine proves R3
// ("fee computed per student independently") end-to-end through Close: three
// students with different billable counts land on three different total_due
// values, and a second enrollment in a class that never gets a single
// session this period contributes no line and no charge.
func TestCloseVariesTotalDuePerStudentAndClassWithNoSessionsAddsNoLine(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	classB := testutil.Class(t, db, teacher.ID, testutil.WithClassName("B"), testutil.WithClassStartDate(date("2026-01-01")))

	// Each student gets their own class (a class meets at most once per day,
	// so several students cannot share one class's session rows here without
	// colliding on session_date) with a different session count, proving the
	// fee is computed per student independently rather than as a class total
	// split evenly.
	seed := func(className string, sessionCount int) uuid.UUID {
		class := testutil.Class(t, db, teacher.ID, testutil.WithClassName(className), testutil.WithClassStartDate(date("2026-01-01")))
		contact := testutil.Contact(t, db, teacher.ID)
		student := testutil.Student(t, db, teacher.ID, contact.ID)
		enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))
		for i := 0; i < sessionCount; i++ {
			sess := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-01").AddDate(0, 0, 7*i+5),
				testutil.WithSessionAttendanceConfirmed(time.Now()))
			testutil.AttendanceRecord(t, db, teacher.ID, sess.ID, student.ID, enrollment.ID)
		}
		return student.ID
	}

	student1ID := seed("student1-class", 1)
	student2ID := seed("student2-class", 2)
	student3ID := seed("student3-class", 3)
	// student3 also joins classB, which never gets a single session this
	// period.
	testutil.Enrollment(t, db, teacher.ID, student3ID, classB.ID, date("2026-01-01"))

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)

	resp, err := svc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, resp.IssuedCount)
	require.EqualValues(t, 0, resp.VoidedCount)
	require.EqualValues(t, 600_000, resp.TotalDue)

	invoices, err := repo.ListInvoices(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	byStudent := make(map[uuid.UUID]billing.Invoice, len(invoices))
	for _, inv := range invoices {
		byStudent[inv.StudentID] = inv
	}
	require.EqualValues(t, 100_000, byStudent[student1ID].TotalDue)
	require.EqualValues(t, 200_000, byStudent[student2ID].TotalDue)
	require.EqualValues(t, 300_000, byStudent[student3ID].TotalDue)

	_, lines, err := repo.GetInvoiceWithLines(ctx, teacher.ID, byStudent[student3ID].ID)
	require.NoError(t, err)
	require.Len(t, lines, 1, "the class with zero sessions this period must not add a second line")
	require.Equal(t, "student3-class", lines[0].ClassName)
}

// TestCloseStudentInTwoClassesProducesOneInvoiceWithTwoLines proves R1: a
// student enrolled in two classes gets exactly one invoice per period, with
// one invoice_line per class they were actually billed in.
func TestCloseStudentInTwoClassesProducesOneInvoiceWithTwoLines(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	classA := testutil.Class(t, db, teacher.ID, testutil.WithClassName("A"), testutil.WithClassStartDate(date("2026-01-01")))
	classB := testutil.Class(t, db, teacher.ID, testutil.WithClassName("B"), testutil.WithClassStartDate(date("2026-01-01")))
	enrollA := testutil.Enrollment(t, db, teacher.ID, student.ID, classA.ID, date("2026-01-01"))
	enrollB := testutil.Enrollment(t, db, teacher.ID, student.ID, classB.ID, date("2026-01-01"))

	sessA := testutil.Session(t, db, teacher.ID, classA.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, sessA.ID, student.ID, enrollA.ID)
	sessB := testutil.Session(t, db, teacher.ID, classB.ID, date("2026-01-08"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, sessB.ID, student.ID, enrollB.ID)

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)

	resp, err := svc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)
	require.EqualValues(t, 200_000, resp.TotalDue)

	invoices, err := repo.ListInvoices(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.Len(t, invoices, 1, "one student must produce exactly one invoice, never one per class")

	_, lines, err := repo.GetInvoiceWithLines(ctx, teacher.ID, invoices[0].ID)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	require.ElementsMatch(t, []string{"A", "B"}, []string{lines[0].ClassName, lines[1].ClassName})
}

// TestCloseVoidsInvoiceThatBecomesEmptyAfterAttendanceCorrection proves the
// void-empty rule fires not only for a never-billed student but for one whose
// only billable session was corrected away between an earlier draft and
// close: the invoice row survives (never hard-deleted), status flips to void.
func TestCloseVoidsInvoiceThatBecomesEmptyAfterAttendanceCorrection(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	sess := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	record := testutil.AttendanceRecord(t, db, teacher.ID, sess.ID, student.ID, enrollment.ID)

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)

	draft, err := svc.Draft(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.Len(t, draft.Invoices, 1)
	invoiceID := *draft.Invoices[0].InvoiceID
	require.EqualValues(t, 100_000, draft.Invoices[0].TotalDue)

	require.NoError(t, db.Delete(record).Error)

	resp, err := svc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, resp.IssuedCount)
	require.EqualValues(t, 1, resp.VoidedCount)
	require.EqualValues(t, 0, resp.TotalDue)

	voided, _, err := repo.GetInvoiceWithLines(ctx, teacher.ID, invoiceID)
	require.NoError(t, err)
	require.Equal(t, billing.InvoiceVoid, voided.Status)
	require.NotNil(t, voided.VoidReason)
	require.NotEmpty(t, *voided.VoidReason)
	require.NotNil(t, voided.VoidedAt)
}

// TestCloseCarriesForwardOpeningBalanceFromPriorClosedPeriod proves R4/R3's
// month-to-month link: a student who owes money from a closed period and has
// zero new sessions the next period still gets an issued invoice — carried
// debt alone is enough to keep the void-empty rule from firing.
func TestCloseCarriesForwardOpeningBalanceFromPriorClosedPeriod(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	jan := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, jan.ID, student.ID, enrollment.ID)

	janPeriod, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	janClose, err := svc.Close(ctx, teacher.ID, janPeriod.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, janClose.IssuedCount)
	require.EqualValues(t, 100_000, janClose.TotalDue)

	febPeriod, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 2)
	require.NoError(t, err)
	febClose, err := svc.Close(ctx, teacher.ID, febPeriod.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, febClose.IssuedCount, "carried debt alone must still issue, never void")
	require.EqualValues(t, 0, febClose.VoidedCount)
	require.EqualValues(t, 100_000, febClose.TotalDue)

	febInvoices, err := repo.ListInvoices(ctx, teacher.ID, febPeriod.ID)
	require.NoError(t, err)
	require.Len(t, febInvoices, 1)
	require.EqualValues(t, 100_000, febInvoices[0].OpeningBalance)
	require.EqualValues(t, 0, febInvoices[0].CurrentCharge)
	require.EqualValues(t, 100_000, febInvoices[0].TotalDue)
	require.Equal(t, billing.InvoiceIssued, febInvoices[0].Status)
}

// TestCloseThenDraftIsConflictAndWritesNothing proves the immutability
// guarantee (Architecture, Requirements): once a period is closed, POST
// .../draft must refuse with 409, never silently reopen or rewrite invoices.
func TestCloseThenDraftIsConflictAndWritesNothing(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	resp, err := svc.Close(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)

	var invoiceCount int64
	require.NoError(t, db.Table("invoices").Where("period_id = ?", fx.period.ID).Count(&invoiceCount).Error)

	_, err = svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var invoiceCountAfter int64
	require.NoError(t, db.Table("invoices").Where("period_id = ?", fx.period.ID).Count(&invoiceCountAfter).Error)
	require.Equal(t, invoiceCount, invoiceCountAfter, "a refused draft after close must write nothing")
}

// TestConcurrentCloseExactlyOneSucceeds proves LockPeriod's SELECT ... FOR
// UPDATE actually serializes two simultaneous closes on the same period:
// exactly one call succeeds, the other reads the row after the winner
// committed and refuses with 409 rather than double-issuing invoices. Run
// with -race to also catch any unsynchronized access in the close path
// itself.
func TestConcurrentCloseExactlyOneSucceeds(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Close(context.Background(), fx.teacher.ID, fx.period.ID)
		}(i)
	}
	wg.Wait()

	successCount, conflictCount := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successCount++
		case apperror.From(err).Code == apperror.CodeConflict:
			conflictCount++
		default:
			t.Fatalf("unexpected error from concurrent close: %v", err)
		}
	}
	require.Equal(t, 1, successCount, "exactly one concurrent close must succeed")
	require.Equal(t, 1, conflictCount, "the loser must see 409, not silently double-close")

	var invoiceCount int64
	require.NoError(t, db.Table("invoices").Where("period_id = ?", fx.period.ID).Count(&invoiceCount).Error)
	require.EqualValues(t, 1, invoiceCount, "a concurrent double-close must never double-issue invoices")

	reloaded, err := repo.GetPeriod(context.Background(), fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.Equal(t, billing.PeriodClosed, reloaded.Status)
}

// TestVoidInvoiceExcludedFromContactBalanceView proves R7's default
// contact-balance view stops counting a voided invoice entirely (schema note
// (i)) — not zeroed, not present at all.
func TestVoidInvoiceExcludedFromContactBalanceView(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	resp, err := svc.Close(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)

	invoices, err := repo.ListInvoices(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.Len(t, invoices, 1)
	invoiceID := invoices[0].ID

	type balanceRow struct {
		Outstanding int64
	}
	var before balanceRow
	require.NoError(t, db.Table("v_contact_balance").
		Where("teacher_id = ? AND period_id = ? AND contact_id = ?", fx.teacher.ID, fx.period.ID, fx.student.ContactID).
		Select("outstanding").Scan(&before).Error)
	require.EqualValues(t, 200_000, before.Outstanding)

	_, err = svc.VoidInvoice(ctx, fx.teacher.ID, invoiceID, "phụ huynh chuyển trường")
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Table("v_contact_balance").
		Where("teacher_id = ? AND period_id = ? AND contact_id = ?", fx.teacher.ID, fx.period.ID, fx.student.ContactID).
		Count(&count).Error)
	require.EqualValues(t, 0, count, "a voided invoice must leave no row behind in the contact balance view")
}

// TestVoidInvoiceWithPaidAmountIsConflict proves the guard against silently
// erasing a recorded payment: an invoice with paid_amount > 0 refuses to void
// with 409, leaving its status and amounts untouched.
func TestVoidInvoiceWithPaidAmountIsConflict(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	resp, err := svc.Close(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)

	invoices, err := repo.ListInvoices(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.Len(t, invoices, 1)
	invoiceID := invoices[0].ID

	require.NoError(t, db.Table("invoices").Where("id = ?", invoiceID).Update("paid_amount", 50_000).Error)

	_, err = svc.VoidInvoice(ctx, fx.teacher.ID, invoiceID, "phụ huynh chuyển trường")
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var reloaded billing.Invoice
	require.NoError(t, db.Where("id = ?", invoiceID).First(&reloaded).Error)
	require.Equal(t, billing.InvoiceIssued, reloaded.Status, "a refused void must leave the invoice untouched")
	require.EqualValues(t, 50_000, reloaded.PaidAmount)
}

// seedClosedJanuaryFixture is one teacher with one student billable for two
// confirmed January 2026 sessions, closed into a 200_000-đồng issued
// invoice — the common starting point every post-close reconciliation test
// below edits.
type seededClosedJanuaryFixture struct {
	teacher    *teachers.Teacher
	student    *students.Student
	class      *classes.Class
	enrollment *enrollments.Enrollment
	period     *billing.Period
	session1   *sessions.Session
	session2   *sessions.Session
	record1    *attendance.Record
	invoiceID  uuid.UUID
}

func seedClosedJanuaryFixture(t *testing.T, db *gorm.DB, svc *billing.Service) seededClosedJanuaryFixture {
	t.Helper()
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	session1 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	session2 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-13"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	record1 := testutil.AttendanceRecord(t, db, teacher.ID, session1.ID, student.ID, enrollment.ID)
	testutil.AttendanceRecord(t, db, teacher.ID, session2.ID, student.ID, enrollment.ID)

	period, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	resp, err := svc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)
	require.EqualValues(t, 200_000, resp.TotalDue)

	var invoiceID uuid.UUID
	require.NoError(t, db.Table("invoices").
		Where("period_id = ? AND student_id = ?", period.ID, student.ID).
		Select("id").Row().Scan(&invoiceID))

	return seededClosedJanuaryFixture{
		teacher: teacher, student: student, class: class, enrollment: enrollment,
		period: period, session1: session1, session2: session2, record1: record1, invoiceID: invoiceID,
	}
}

// currentCalendarPeriod resolves the (year, month) resolveTargetPeriod picks
// when no already-open period exists after the closed one: the real
// wall-clock month in the teacher's fixture timezone, mirroring hcmToday's
// rationale so these tests never hardcode "February" and rot as real time
// moves past it.
func currentCalendarPeriod(t *testing.T) (int, int) {
	today := hcmToday(t)
	return today.Year(), int(today.Month())
}

// TestReconcileSessionStatusChangeStillBillableIsNoOp proves Architecture's
// first reconciliation property: flipping a record's status (present <->
// absent) without touching its billable flag never moves the billable count
// V1 bills on, so ReconcileSession posts nothing and creates no target
// period.
func TestReconcileSessionStatusChangeStillBillableIsNoOp(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("status", attendance.StatusAbsent).Error)

	sc := testutil.ScopeFor(t, db, fx.teacher.ID)
	result, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)
	require.Empty(t, result.Adjustments, "present<->absent alone must not move the billable count")

	var adjustmentCount int64
	require.NoError(t, db.Table("invoice_adjustments").Count(&adjustmentCount).Error)
	require.EqualValues(t, 0, adjustmentCount)

	var periodCount int64
	require.NoError(t, db.Table("billing_periods").Where("teacher_id = ?", fx.teacher.ID).Count(&periodCount).Error)
	require.EqualValues(t, 1, periodCount, "a no-op reconciliation must never create a target period")
}

// TestReconcileSessionBillableFlipToFalsePostsNegativeDeltaWithSourceSessionID
// proves the money-moving half of Architecture's first property: flipping
// billable to false drops the live billable count by one, and the resulting
// negative delta lands on the next open period's invoice with
// source_session_id set to the edited session (docs/schema_design.sql:344).
func TestReconcileSessionBillableFlipToFalsePostsNegativeDeltaWithSourceSessionID(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("billable", false).Error)

	sc := testutil.ScopeFor(t, db, fx.teacher.ID)
	result, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)
	require.Len(t, result.Adjustments, 1)
	require.Equal(t, fx.student.ID, result.Adjustments[0].StudentID)
	require.EqualValues(t, -100_000, result.Adjustments[0].Amount)

	year, month := currentCalendarPeriod(t)
	targetPeriod, err := repo.GetPeriodByYearMonth(ctx, fx.teacher.ID, int16(year), int16(month)) //nolint:gosec // calendar year/month, always in range
	require.NoError(t, err)
	require.NotNil(t, targetPeriod, "the current calendar month's period must have been auto-created")
	require.Equal(t, targetPeriod.ID, result.Adjustments[0].PeriodID)

	var adj billing.InvoiceAdjustment
	require.NoError(t, db.Where("invoice_id = ?", result.Adjustments[0].InvoiceID).First(&adj).Error)
	require.EqualValues(t, -100_000, adj.Amount)
	require.NotNil(t, adj.SourceSessionID, "source_session_id must always be set on a reconciliation row")
	require.Equal(t, fx.session1.ID, *adj.SourceSessionID)
	require.NotEmpty(t, adj.Reason)

	targetInvoice, _, err := repo.GetInvoiceWithLines(ctx, fx.teacher.ID, result.Adjustments[0].InvoiceID)
	require.NoError(t, err)
	require.EqualValues(t, -100_000, targetInvoice.AdjustmentTotal)
	require.EqualValues(t, targetInvoice.OpeningBalance+targetInvoice.CurrentCharge+targetInvoice.AdjustmentTotal, targetInvoice.TotalDue)
	require.Equal(t, billing.InvoiceDraft, targetInvoice.Status, "the target invoice stays draft until the next draft/close recomputation")
}

// TestReconcileSessionLeavesClosedInvoiceByteIdentical proves the closed
// invoice the reconciled session belongs to is never itself written to —
// only the next open period's invoice is — by comparing every column before
// and after the edit.
func TestReconcileSessionLeavesClosedInvoiceByteIdentical(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	var before billing.Invoice
	require.NoError(t, db.Where("id = ?", fx.invoiceID).First(&before).Error)

	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("billable", false).Error)
	sc := testutil.ScopeFor(t, db, fx.teacher.ID)
	_, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)

	var after billing.Invoice
	require.NoError(t, db.Where("id = ?", fx.invoiceID).First(&after).Error)
	require.Equal(t, before, after, "the closed invoice's row must be byte-identical after a post-close reconciliation")

	var lineCount int64
	require.NoError(t, db.Table("invoice_lines").Where("invoice_id = ?", fx.invoiceID).Count(&lineCount).Error)
	require.EqualValues(t, 1, lineCount, "reconciliation must never add or remove a line on the closed invoice")
	var line billing.InvoiceLine
	require.NoError(t, db.Where("invoice_id = ?", fx.invoiceID).First(&line).Error)
	require.EqualValues(t, 2, line.BillableCount, "the closed invoice's own line must keep its frozen snapshot count")
	require.EqualValues(t, 200_000, line.Amount)
}

// TestReconcileSessionRepeatedEditsDoNotDoubleCount proves the single most
// likely money bug (Risk Assessment): editing the same session's attendance
// twice after close must produce two adjustment rows whose sum equals the
// total live movement, never the full delta twice, and reconciling an
// unchanged edit a second time must post nothing at all.
func TestReconcileSessionRepeatedEditsDoNotDoubleCount(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	sc := testutil.ScopeFor(t, db, fx.teacher.ID)

	// First edit: session1 flips to non-billable (-100_000).
	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("billable", false).Error)
	first, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)
	require.Len(t, first.Adjustments, 1)
	require.EqualValues(t, -100_000, first.Adjustments[0].Amount)

	// Reconciling the same unchanged edit again must be a pure no-op:
	// already_adj now fully explains the gap.
	again, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)
	require.Empty(t, again.Adjustments, "reconciling an unchanged edit a second time must not double count")

	// Second real edit: session1 goes back to billable, reverting the first.
	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("billable", true).Error)
	second, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)
	require.Len(t, second.Adjustments, 1)
	require.EqualValues(t, 100_000, second.Adjustments[0].Amount, "reverting must post the equal and opposite delta")

	var rows []billing.InvoiceAdjustment
	require.NoError(t, db.Where("invoice_id = ?", first.Adjustments[0].InvoiceID).
		Order("created_at").Find(&rows).Error)
	require.Len(t, rows, 2, "two edits must produce exactly two adjustment rows, never one overwritten row")
	var sum int64
	for _, r := range rows {
		sum += r.Amount
		require.NotNil(t, r.SourceSessionID)
		require.Equal(t, fx.session1.ID, *r.SourceSessionID)
	}
	require.EqualValues(t, 0, sum, "reverting the edit must net the two adjustments to zero")

	trail, err := svc.ListAdjustments(ctx, fx.teacher.ID, first.Adjustments[0].InvoiceID)
	require.NoError(t, err)
	require.Len(t, trail, 2)
	require.False(t, trail[1].CreatedAt.Before(trail[0].CreatedAt), "the audit trail must be ordered oldest first")

	targetInvoice, _, err := repo.GetInvoiceWithLines(ctx, fx.teacher.ID, first.Adjustments[0].InvoiceID)
	require.NoError(t, err)
	require.EqualValues(t, 0, targetInvoice.AdjustmentTotal, "the two adjustments must net to zero on the target invoice")
}

// TestConcurrentReconcileSameStudentDoesNotDoubleCount proves two edits to a
// student's closed period, reconciled concurrently, carry the student's charge
// drop exactly once. Both sessions of the student are flipped to non-billable,
// so each reconciliation independently sees live_charge=0 against a frozen
// current_charge of 200_000; without serialisation both would read
// already_adj=0 and each post the full -200_000, over-crediting to -400_000.
// The closed-invoice row lock forces the second reconciliation to wait for the
// first to commit, then read an already_adj that leaves it a no-op.
func TestConcurrentReconcileSameStudentDoesNotDoubleCount(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	// Stage both edits before either reconciliation runs: the student is now
	// billable for zero sessions this closed period.
	require.NoError(t, db.Model(&attendance.Record{}).
		Where("session_id IN ?", []uuid.UUID{fx.session1.ID, fx.session2.ID}).
		Update("billable", false).Error)

	sc := testutil.ScopeFor(t, db, fx.teacher.ID)
	var wg sync.WaitGroup
	results := make([]attendance.Reconciliation, 2)
	errs := make([]error, 2)
	sessionIDs := []uuid.UUID{fx.session1.ID, fx.session2.ID}
	for i := range sessionIDs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.ReconcileSession(context.Background(), sc, sessionIDs[i])
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	// Exactly one of the two concurrent reconciliations posts the -200_000
	// carry; the other, seeing it already accounted for, posts nothing.
	posted := 0
	var targetInvoiceID uuid.UUID
	for _, r := range results {
		if len(r.Adjustments) > 0 {
			posted += len(r.Adjustments)
			targetInvoiceID = r.Adjustments[0].InvoiceID
		}
	}
	require.Equal(t, 1, posted, "concurrent reconciliation of the same student must post the carry exactly once")

	var rows []billing.InvoiceAdjustment
	require.NoError(t, db.Where("invoice_id = ?", targetInvoiceID).Find(&rows).Error)
	var sum int64
	for _, r := range rows {
		sum += r.Amount
	}
	require.EqualValues(t, -200_000, sum, "the concurrent carry must net to a single -200_000, never -400_000")

	targetInvoice, _, err := repo.GetInvoiceWithLines(ctx, fx.teacher.ID, targetInvoiceID)
	require.NoError(t, err)
	require.EqualValues(t, -200_000, targetInvoice.AdjustmentTotal, "the target invoice's adjustment_total must reflect a single carry")
}

// TestReconcileSessionCreatesNextPeriodAndDraftInvoiceThenCloseKeepsAdjustment
// proves ensureAdjustmentTarget's fallback (Architecture step 2): when no
// already-open period exists after the closed one, reconciliation creates
// the current calendar month's period and a draft invoice for it, and a
// subsequent close on that period recomputes current_charge/opening_balance
// while keeping the carried adjustment intact.
func TestReconcileSessionCreatesNextPeriodAndDraftInvoiceThenCloseKeepsAdjustment(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	var periodCountBefore int64
	require.NoError(t, db.Table("billing_periods").Where("teacher_id = ?", fx.teacher.ID).Count(&periodCountBefore).Error)
	require.EqualValues(t, 1, periodCountBefore, "only January exists before the edit")

	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("billable", false).Error)
	sc := testutil.ScopeFor(t, db, fx.teacher.ID)
	result, err := svc.ReconcileSession(ctx, sc, fx.session1.ID)
	require.NoError(t, err)
	require.Len(t, result.Adjustments, 1)

	year, month := currentCalendarPeriod(t)
	targetPeriod, err := repo.GetPeriodByYearMonth(ctx, fx.teacher.ID, int16(year), int16(month)) //nolint:gosec // calendar year/month, always in range
	require.NoError(t, err)
	require.NotNil(t, targetPeriod, "the current calendar month's period must have been auto-created")
	require.Equal(t, billing.PeriodOpen, targetPeriod.Status)

	closeResp, err := svc.Close(ctx, fx.teacher.ID, targetPeriod.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, closeResp.IssuedCount,
		"January's carried debt alone keeps the target invoice non-empty, so it must issue, not void")

	invoices, err := repo.ListInvoices(ctx, fx.teacher.ID, targetPeriod.ID)
	require.NoError(t, err)
	require.Len(t, invoices, 1)
	inv := invoices[0]
	require.EqualValues(t, 0, inv.CurrentCharge, "the target period has no sessions of its own")
	require.EqualValues(t, 200_000, inv.OpeningBalance, "January's unpaid 200_000 carries forward")
	require.EqualValues(t, -100_000, inv.AdjustmentTotal, "the carried reconciliation adjustment must survive the close-time recompute")
	require.EqualValues(t, 100_000, inv.TotalDue, "200_000 opening + 0 charge - 100_000 adjustment")
	require.Equal(t, billing.InvoiceIssued, inv.Status)

	var adjustmentCount int64
	require.NoError(t, db.Table("invoice_adjustments").Where("invoice_id = ?", inv.ID).Count(&adjustmentCount).Error)
	require.EqualValues(t, 1, adjustmentCount, "close must never duplicate the reconciliation adjustment row")
}

// TestManualAdjustmentSurvivesCloseTimeRecomputeAndFoldsIntoTotalDue proves
// AddAdjustment's write is never touched by a later close-time recompute:
// posting a manual adjustment on a draft invoice, then closing the period,
// must fold the adjustment into total_due exactly once.
func TestManualAdjustmentSurvivesCloseTimeRecomputeAndFoldsIntoTotalDue(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	draft, err := svc.Draft(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	invoiceID := *draft.Invoices[0].InvoiceID
	require.EqualValues(t, 200_000, draft.Invoices[0].TotalDue)

	adjResp, invResp, err := svc.AddAdjustment(ctx, fx.teacher.ID, invoiceID, -30_000, "giảm giá học sinh cũ")
	require.NoError(t, err)
	require.EqualValues(t, -30_000, adjResp.Amount)
	require.EqualValues(t, 170_000, invResp.TotalDue)

	closeResp, err := svc.Close(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, closeResp.IssuedCount)
	require.EqualValues(t, 170_000, closeResp.TotalDue, "the manual adjustment must be folded exactly once into the close totals")

	invoices, err := repo.ListInvoices(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.Len(t, invoices, 1)
	require.EqualValues(t, invoiceID, invoices[0].ID)
	require.EqualValues(t, -30_000, invoices[0].AdjustmentTotal)
	require.EqualValues(t, 170_000, invoices[0].TotalDue)
	require.Equal(t, billing.InvoiceIssued, invoices[0].Status)

	var adjustmentCount int64
	require.NoError(t, db.Table("invoice_adjustments").Where("invoice_id = ?", invoiceID).Count(&adjustmentCount).Error)
	require.EqualValues(t, 1, adjustmentCount, "close must never duplicate the manual adjustment row")
}

// TestAddAdjustmentOnVoidInvoiceIsConflict proves Architecture's guard: a
// void invoice cannot grow a new adjustment.
func TestAddAdjustmentOnVoidInvoiceIsConflict(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	resp, err := svc.Close(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)

	invoices, err := repo.ListInvoices(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	invoiceID := invoices[0].ID

	_, err = svc.VoidInvoice(ctx, fx.teacher.ID, invoiceID, "phụ huynh chuyển trường")
	require.NoError(t, err)

	_, _, err = svc.AddAdjustment(ctx, fx.teacher.ID, invoiceID, 10_000, "sửa nhầm")
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var count int64
	require.NoError(t, db.Table("invoice_adjustments").Where("invoice_id = ?", invoiceID).Count(&count).Error)
	require.EqualValues(t, 0, count, "a refused adjustment on a void invoice must write nothing")
}

// TestAddAdjustmentOnPaidInvoiceIsConflict proves Architecture's other
// guard: a fully paid invoice refuses a new adjustment — the teacher is
// told to correct the next period instead.
func TestAddAdjustmentOnPaidInvoiceIsConflict(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	fx := seedDraftFixture(t, db, svc)
	ctx := context.Background()

	resp, err := svc.Close(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.IssuedCount)

	invoices, err := repo.ListInvoices(ctx, fx.teacher.ID, fx.period.ID)
	require.NoError(t, err)
	invoiceID := invoices[0].ID

	require.NoError(t, db.Table("invoices").Where("id = ?", invoiceID).
		Updates(map[string]any{"status": billing.InvoicePaid, "paid_amount": invoices[0].TotalDue}).Error)

	_, _, err = svc.AddAdjustment(ctx, fx.teacher.ID, invoiceID, -10_000, "hoàn tiền")
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var count int64
	require.NoError(t, db.Table("invoice_adjustments").Where("invoice_id = ?", invoiceID).Count(&count).Error)
	require.EqualValues(t, 0, count, "a refused adjustment on a paid invoice must write nothing")
}

// TestReconcileSessionIsNoOpForSessionInOpenPeriod proves ReconcileSession's
// documented no-op (Success Criteria): an edit to a session whose date falls
// inside a still-open period is Architecture's ordinary "next draft/close
// recomputation picks it up naturally" case, not a reconciliation concern.
func TestReconcileSessionIsNoOpForSessionInOpenPeriod(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))
	sess := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, sess.ID, student.ID, enrollment.ID)

	_, err := svc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	// Deliberately never Close this period: it stays open.

	result, err := svc.ReconcileSession(ctx, testutil.ScopeFor(t, db, teacher.ID), sess.ID)
	require.NoError(t, err)
	require.Empty(t, result.Adjustments, "a session inside a still-open period must never post a reconciliation adjustment")

	var count int64
	require.NoError(t, db.Table("invoice_adjustments").Count(&count).Error)
	require.EqualValues(t, 0, count)
}

// TestReconcileSessionIsNoOpWhenStudentHasNoInvoiceInClosedPeriod proves
// Architecture's "I = invoice of (P, student), status <> 'void' -- skip if
// absent": a student whose invoice in the closed period was voided (so no
// non-void invoice exists for them there) must be skipped even though the
// session's attendance was genuinely edited.
func TestReconcileSessionIsNoOpWhenStudentHasNoInvoiceInClosedPeriod(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	fx := seedClosedJanuaryFixture(t, db, svc)
	ctx := context.Background()

	_, err := svc.VoidInvoice(ctx, fx.teacher.ID, fx.invoiceID, "phụ huynh chuyển trường")
	require.NoError(t, err)

	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", fx.record1.ID).Update("billable", false).Error)

	result, err := svc.ReconcileSession(ctx, testutil.ScopeFor(t, db, fx.teacher.ID), fx.session1.ID)
	require.NoError(t, err)
	require.Empty(t, result.Adjustments, "a student with no non-void invoice in the closed period must be skipped")

	var count int64
	require.NoError(t, db.Table("invoice_adjustments").Count(&count).Error)
	require.EqualValues(t, 0, count)

	var periodCount int64
	require.NoError(t, db.Table("billing_periods").Where("teacher_id = ?", fx.teacher.ID).Count(&periodCount).Error)
	require.EqualValues(t, 1, periodCount, "skipping must never create a target period")
}
