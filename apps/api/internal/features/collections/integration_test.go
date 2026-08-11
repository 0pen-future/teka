//go:build integration

package collections_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/collections"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// newIntegrationDeps wires the real dependency chain router.go uses:
// collections consumes nothing from billing or payments directly — they only
// share tables — but the fixtures need real billing and payments services to
// produce invoices and settle them.
func newIntegrationDeps(t *testing.T) (*collections.Service, *billing.Service, *payments.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	billingSvc := billing.NewService(billing.NewRepository(db, attendanceSvc), txMgr, sessionsSvc, enrollmentsSvc)
	paymentsSvc := payments.NewService(payments.NewRepository(db), txMgr)
	collectionsSvc := collections.NewService(collections.NewRepository(db))
	return collectionsSvc, billingSvc, paymentsSvc, db
}

// seededChild is one class+student+enrollment fixture, along with the ids a
// by-class query needs.
type seededChild struct {
	ClassID   uuid.UUID
	StudentID uuid.UUID
}

// seedChild creates one class with sessionCount held+confirmed sessions
// (100 000 đồng each) and one student under contactID enrolled in it.
func seedChild(t *testing.T, db *gorm.DB, teacherID, contactID uuid.UUID, name string, classStart time.Time, sessionCount int) seededChild {
	t.Helper()
	class := testutil.Class(t, db, teacherID, testutil.WithClassName(name), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, teacherID, contactID, testutil.WithStudentFullName(name+"-student"))
	enrollment := testutil.Enrollment(t, db, teacherID, student.ID, class.ID, classStart)
	for i := 0; i < sessionCount; i++ {
		sess := testutil.Session(t, db, teacherID, class.ID, classStart.AddDate(0, 0, 7*i+1),
			testutil.WithSessionAttendanceConfirmed(time.Now()))
		testutil.AttendanceRecord(t, db, teacherID, sess.ID, student.ID, enrollment.ID)
	}
	return seededChild{ClassID: class.ID, StudentID: student.ID}
}

// getInvoiceByStudent loads the one invoice a fixture student owns.
func getInvoiceByStudent(t *testing.T, db *gorm.DB, teacherID, studentID uuid.UUID) billing.Invoice {
	t.Helper()
	var inv billing.Invoice
	require.NoError(t, db.Where("teacher_id = ? AND student_id = ?", teacherID, studentID).Take(&inv).Error)
	return inv
}

// listParams builds pagination params the way a handler would, from a raw
// query string.
func listParams(t *testing.T, allowed map[string]string, defaultSort, query string) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	target := "/"
	if query != "" {
		target += "?" + query
	}
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return pagination.Parse(c, defaultSort, allowed)
}

func contactParams(t *testing.T, query string) pagination.Params {
	return listParams(t, collections.ContactSortColumns(), "-outstanding", query)
}

func classParams(t *testing.T, query string) pagination.Params {
	return listParams(t, collections.ClassSortColumns(), "student_name", query)
}

func contactIDs(rows []collections.ContactBalanceRow) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ContactID
	}
	return ids
}

// TestContactViewMergesFamiliesAndClassViewShowsPerChildStatus is the
// scenario proving both views of the collection board agree with each other
// and with the ledger: contact A paid in full across two children in two
// classes, contact B underpaid so one child absorbed the shortfall, contact C
// never paid, contact D is archived but still owes money, and a fifth
// contact's only invoice was voided — which must make that contact and its
// invoice disappear from every view and every total.
func TestContactViewMergesFamiliesAndClassViewShowsPerChildStatus(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, paymentsSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	scope := testutil.ScopeFor(t, db, teacher.ID)

	contactA := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact A"))
	childA1 := seedChild(t, db, teacher.ID, contactA.ID, "A1", date("2026-01-01"), 1)
	childA2 := seedChild(t, db, teacher.ID, contactA.ID, "A2", date("2026-01-02"), 2)

	contactB := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact B"))
	childB1 := seedChild(t, db, teacher.ID, contactB.ID, "B1", date("2026-01-01"), 1)
	childB2 := seedChild(t, db, teacher.ID, contactB.ID, "B2", date("2026-01-10"), 1)

	contactC := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact C"))
	seedChild(t, db, teacher.ID, contactC.ID, "C1", date("2026-01-01"), 1)

	contactD := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact D"))
	seedChild(t, db, teacher.ID, contactD.ID, "D1", date("2026-01-01"), 1)

	contactE := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact E"))
	childE1 := seedChild(t, db, teacher.ID, contactE.ID, "E1", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, scope, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, scope, period.ID)
	require.NoError(t, err)

	// A overpays by 20 000 — the surplus has nothing left to settle in this
	// period, so it must surface as summary.unallocated_credit rather than
	// vanish.
	_, err = paymentsSvc.Record(ctx, scope, payments.RecordPaymentRequest{
		ContactID: contactA.ID, Amount: 320_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)

	// B pays enough to clear the earlier-starting class but not the later
	// one — D8's tie-break settles B1 in full and leaves B2 short by 50 000.
	_, err = paymentsSvc.Record(ctx, scope, payments.RecordPaymentRequest{
		ContactID: contactB.ID, Amount: 150_000, Method: payments.MethodTransfer, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)

	// C and D receive no payment at all.

	// D's last active child has left the roster; their debt must stay
	// visible and collectable.
	require.NoError(t, db.Delete(contactD).Error)

	// E's invoice is voided outright — it must vanish from every view and
	// every total, not just read as zero.
	voidInvoice := getInvoiceByStudent(t, db, teacher.ID, childE1.StudentID)
	require.NoError(t, db.Table("invoices").Where("id = ?", voidInvoice.ID).Updates(map[string]any{
		"status": billing.InvoiceVoid, "void_reason": "test fixture void", "voided_at": time.Now(),
	}).Error)

	// --- default (contact) view merges every family into one row ---
	listAll, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.NoError(t, err)
	require.Len(t, listAll.ContactRows, 4, "contact E's only invoice is void and must not appear")

	byContact := make(map[uuid.UUID]collections.ContactBalanceRow, len(listAll.ContactRows))
	for _, row := range listAll.ContactRows {
		byContact[row.ContactID] = row
	}

	rowA := byContact[contactA.ID]
	require.EqualValues(t, 2, rowA.StudentCount)
	require.EqualValues(t, 300_000, rowA.TotalDue)
	require.EqualValues(t, 300_000, rowA.TotalPaid)
	require.EqualValues(t, 0, rowA.Outstanding)
	require.Equal(t, collections.StatusPaid, rowA.PaymentStatus)
	require.False(t, rowA.ContactArchived)
	require.Len(t, rowA.Invoices, 2)

	rowB := byContact[contactB.ID]
	require.EqualValues(t, 200_000, rowB.TotalDue)
	require.EqualValues(t, 150_000, rowB.TotalPaid)
	require.EqualValues(t, 50_000, rowB.Outstanding)
	require.Equal(t, collections.StatusPartial, rowB.PaymentStatus)

	rowC := byContact[contactC.ID]
	require.EqualValues(t, 100_000, rowC.TotalDue)
	require.EqualValues(t, 0, rowC.TotalPaid)
	require.Equal(t, collections.StatusUnpaid, rowC.PaymentStatus)

	rowD := byContact[contactD.ID]
	require.EqualValues(t, 100_000, rowD.TotalDue)
	require.EqualValues(t, 0, rowD.TotalPaid)
	require.Equal(t, collections.StatusUnpaid, rowD.PaymentStatus)
	require.True(t, rowD.ContactArchived, "a soft-deleted contact with debt must stay visible and flagged")

	// --- by-class view: A's two children each read paid in their own room ---
	resultA1, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewClass,
		collections.Filter{ClassID: &childA1.ClassID}, classParams(t, ""))
	require.NoError(t, err)
	require.Len(t, resultA1.ClassRows, 1)
	require.Equal(t, collections.StatusPaid, resultA1.ClassRows[0].PaymentStatus)
	require.EqualValues(t, 0, resultA1.ClassRows[0].InvoiceOutstanding)

	resultA2, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewClass,
		collections.Filter{ClassID: &childA2.ClassID}, classParams(t, ""))
	require.NoError(t, err)
	require.Len(t, resultA2.ClassRows, 1)
	require.Equal(t, collections.StatusPaid, resultA2.ClassRows[0].PaymentStatus)
	require.EqualValues(t, 200_000, resultA2.ClassRows[0].LineAmount)
	require.EqualValues(t, 0, resultA2.ClassRows[0].InvoiceOutstanding)

	// --- by-class view: B's shortfall is visible on the specific child ---
	resultB1, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewClass,
		collections.Filter{ClassID: &childB1.ClassID}, classParams(t, ""))
	require.NoError(t, err)
	require.Len(t, resultB1.ClassRows, 1)
	require.Equal(t, collections.StatusPaid, resultB1.ClassRows[0].PaymentStatus)
	require.EqualValues(t, 100_000, resultB1.ClassRows[0].InvoicePaidAmount)
	require.EqualValues(t, 0, resultB1.ClassRows[0].InvoiceOutstanding)

	resultB2, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewClass,
		collections.Filter{ClassID: &childB2.ClassID}, classParams(t, ""))
	require.NoError(t, err)
	require.Len(t, resultB2.ClassRows, 1)
	require.Equal(t, collections.StatusPartial, resultB2.ClassRows[0].PaymentStatus)
	require.EqualValues(t, 100_000, resultB2.ClassRows[0].InvoiceTotalDue)
	require.EqualValues(t, 50_000, resultB2.ClassRows[0].InvoicePaidAmount)
	require.EqualValues(t, 50_000, resultB2.ClassRows[0].InvoiceOutstanding, "B2 must show it is short by exactly 50 000")

	// --- E's voided invoice is absent from its own classroom's view too ---
	resultE, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewClass,
		collections.Filter{ClassID: &childE1.ClassID}, classParams(t, ""))
	require.NoError(t, err)
	require.Empty(t, resultE.ClassRows)

	// --- status filter: paid isolates A, partial isolates B, unpaid isolates C and D ---
	//
	// The by-contact status derivation (payment_status.go) reads "unpaid" as
	// total_paid == 0. B has a nonzero total_paid (150 000, split across two
	// children by D8), so it derives to "partial" — not "unpaid" — despite
	// one of its children individually reading partially_paid. Filtered here
	// against that derivation rather than against the "B, C, D" grouping a
	// contact-level "underpaid" description might suggest.
	paidOnly, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewContact,
		collections.Filter{Status: collections.StatusPaid}, contactParams(t, "status=paid"))
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{contactA.ID}, contactIDs(paidOnly.ContactRows))

	partialOnly, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewContact,
		collections.Filter{Status: collections.StatusPartial}, contactParams(t, "status=partial"))
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{contactB.ID}, contactIDs(partialOnly.ContactRows))

	unpaidOnly, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewContact,
		collections.Filter{Status: collections.StatusUnpaid}, contactParams(t, "status=unpaid"))
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{contactC.ID, contactD.ID}, contactIDs(unpaidOnly.ContactRows))

	// --- summary reconciles with the contact view's own row totals ---
	summary, err := collectionsSvc.Summary(ctx, scope, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 6, summary.StudentCount)
	require.EqualValues(t, 4, summary.ContactCount)
	require.EqualValues(t, 1, summary.PaidContactCount)
	require.EqualValues(t, 1, summary.PartialContactCount)
	require.EqualValues(t, 2, summary.UnpaidContactCount)
	require.EqualValues(t, 20_000, summary.UnallocatedCredit, "A's 20 000 surplus has nothing left to settle in this period")

	var sumDue, sumPaid, sumOutstanding int64
	for _, row := range listAll.ContactRows {
		sumDue += row.TotalDue
		sumPaid += row.TotalPaid
		sumOutstanding += row.Outstanding
	}
	require.Equal(t, sumDue, summary.TotalDue)
	require.Equal(t, sumPaid, summary.TotalPaid)
	require.Equal(t, sumOutstanding, summary.TotalOutstanding)

	// --- a period id belonging to another teacher, in another center, is not found ---
	_, stranger := testutil.Teacher(t, db)
	strangerScope := testutil.ScopeFor(t, db, stranger.ID)
	_, err = collectionsSvc.List(ctx, strangerScope, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = collectionsSvc.Summary(ctx, strangerScope, period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// TestListRejectsUnknownViewAndMissingClassID proves the two guards service.List
// owns: an unrecognised view value and a class view with no class_id both fail
// closed as 422s rather than falling back to a default.
func TestListRejectsUnknownViewAndMissingClassID(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	scope := testutil.ScopeFor(t, db, teacher.ID)
	period, err := billingSvc.EnsurePeriod(ctx, scope, 2026, 1)
	require.NoError(t, err)

	_, err = collectionsSvc.List(ctx, scope, period.ID, "bogus", collections.Filter{}, contactParams(t, ""))
	require.Error(t, err)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	_, err = collectionsSvc.List(ctx, scope, period.ID, collections.ViewClass, collections.Filter{}, classParams(t, ""))
	require.Error(t, err)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
}

// TestContactViewFiltersUnpaidWithin500msAt150Students proves R7's filtering
// interaction feels instant at the scale a full-time teacher's roster
// reaches: 150 students across 5 classes, none paid.
func TestContactViewFiltersUnpaidWithin500msAt150Students(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	scope := testutil.ScopeFor(t, db, teacher.ID)

	const classCount = 5
	const studentsPerClass = 30
	for c := 0; c < classCount; c++ {
		class := testutil.Class(t, db, teacher.ID,
			testutil.WithClassName(fmt.Sprintf("Perf-%d", c)), testutil.WithClassStartDate(date("2026-01-01")))
		// One session per class per day — a class only ever holds one
		// session on a given date — with every student in that class
		// attending the same session.
		sess := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-02"),
			testutil.WithSessionAttendanceConfirmed(time.Now()))
		for i := 0; i < studentsPerClass; i++ {
			contact := testutil.Contact(t, db, teacher.ID)
			student := testutil.Student(t, db, teacher.ID, contact.ID)
			enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))
			testutil.AttendanceRecord(t, db, teacher.ID, sess.ID, student.ID, enrollment.ID)
		}
	}

	period, err := billingSvc.EnsurePeriod(ctx, scope, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, scope, period.ID)
	require.NoError(t, err)

	started := time.Now()
	result, err := collectionsSvc.List(ctx, scope, period.ID, collections.ViewContact,
		collections.Filter{Status: collections.StatusUnpaid}, contactParams(t, "status=unpaid&per_page=100"))
	elapsed := time.Since(started)

	require.NoError(t, err)
	require.NotEmpty(t, result.ContactRows)
	require.EqualValues(t, classCount*studentsPerClass, result.Total)
	require.Less(t, elapsed, 500*time.Millisecond, "the unpaid filter must return within 500ms at 150 students")
}

// TestOwnerSeesMembersCollectionsWithFullContent proves center scope grants
// the center owner oversight of a member's billing period: the owner reads
// back the member's own contact balance and summary content, not merely a
// non-empty count — a blank result here would mean the owner bypass only
// widened the tenant filter without actually letting the query resolve the
// member's rows.
func TestOwnerSeesMembersCollectionsWithFullContent(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, paymentsSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	testutil.JoinCenter(t, db, member.ID, ownerScope.CenterID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	require.Equal(t, ownerScope.CenterID, memberScope.CenterID, "member must have joined the owner's center")

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactFullName("Member Contact"))
	seedChild(t, db, member.ID, contact.ID, "M1", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	_, err = paymentsSvc.Record(ctx, memberScope, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 60_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)

	// The owner reads the member's period id under their own (owning) scope.
	result, err := collectionsSvc.List(ctx, ownerScope, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.NoError(t, err, "the center owner must read a member's billing period")
	require.Len(t, result.ContactRows, 1, "owner's read-back must not come back blank")
	row := result.ContactRows[0]
	require.Equal(t, contact.ID, row.ContactID)
	require.Equal(t, "Member Contact", row.FullName)
	require.EqualValues(t, 100_000, row.TotalDue)
	require.EqualValues(t, 60_000, row.TotalPaid)
	require.EqualValues(t, 40_000, row.Outstanding)
	require.Len(t, row.Invoices, 1, "owner's read-back must resolve the member's child invoice")

	summary, err := collectionsSvc.Summary(ctx, ownerScope, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, summary.ContactCount)
	require.EqualValues(t, 100_000, summary.TotalDue)
	require.EqualValues(t, 60_000, summary.TotalPaid)
	require.EqualValues(t, 40_000, summary.TotalOutstanding)
}

// TestPeersInSameCenterCannotSeeEachOthersCollections proves center scope
// grants oversight to the center's owner only — a plain member gets no
// visibility into another member's billing period, even inside the same
// center.
func TestPeersInSameCenterCannotSeeEachOthersCollections(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	memberC, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, memberB.ID, ownerCenter)
	testutil.JoinCenter(t, db, memberC.ID, ownerCenter)
	scopeB := testutil.ScopeFor(t, db, memberB.ID)
	scopeC := testutil.ScopeFor(t, db, memberC.ID)
	require.False(t, scopeB.IsOwner)
	require.False(t, scopeC.IsOwner)

	contact := testutil.Contact(t, db, memberB.ID)
	seedChild(t, db, memberB.ID, contact.ID, "B1", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, scopeB, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, scopeB, period.ID)
	require.NoError(t, err)

	// B reads their own period fine.
	own, err := collectionsSvc.List(ctx, scopeB, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.NoError(t, err)
	require.Len(t, own.ContactRows, 1)

	// C, a non-owning peer in the same center, gets no oversight over B's
	// period — the period id resolves to not-found under C's own scope.
	_, err = collectionsSvc.List(ctx, scopeC, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.Error(t, err, "a peer must not read another member's billing period")
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = collectionsSvc.Summary(ctx, scopeC, period.ID)
	require.Error(t, err, "a peer must not read another member's period summary")
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// TestCrossCenterCollectionsAreNotFound proves a teacher entirely outside the
// center — not merely a non-owning peer inside it — is refused with the same
// not-found result, never a 403 that would leak the period's existence.
func TestCrossCenterCollectionsAreNotFound(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)
	require.NotEqual(t, scopeA.CenterID, scopeB.CenterID, "fixture teachers must start in separate centers")

	contact := testutil.Contact(t, db, teacherA.ID)
	seedChild(t, db, teacherA.ID, contact.ID, "A1", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, scopeA, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, scopeA, period.ID)
	require.NoError(t, err)

	_, err = collectionsSvc.List(ctx, scopeA, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.NoError(t, err, "teacher A must read their own period")

	_, err = collectionsSvc.List(ctx, scopeB, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = collectionsSvc.Summary(ctx, scopeB, period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// scopeB is not even an owner of their own center, but that must not
	// matter here: cross-center is invisible regardless of IsOwner.
	require.True(t, scopeB.IsOwner, "a fresh fixture teacher owns their own personal center")
}
