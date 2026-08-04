//go:build integration

package payments_test

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
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
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
// payments consumes nothing from billing directly (they only share the
// invoices table), but the fixtures need a real billing.Service to issue the
// invoices payments settles.
func newIntegrationDeps(t *testing.T) (*payments.Service, *billing.Service, *gorm.DB) {
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
	return paymentsSvc, billingSvc, db
}

// seedStudentWithSessions creates one class + student + enrollment under
// contactID, with sessionCount held+confirmed sessions each worth the
// fixture's fixed 100 000 đồng unit price, all inside the calendar month
// classStart falls in. Returns the new student id.
func seedStudentWithSessions(t *testing.T, db *gorm.DB, teacherID, contactID uuid.UUID, className string, classStart time.Time, sessionCount int) uuid.UUID {
	t.Helper()
	class := testutil.Class(t, db, teacherID, testutil.WithClassName(className), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, teacherID, contactID, testutil.WithStudentFullName(className+"-student"))
	enrollment := testutil.Enrollment(t, db, teacherID, student.ID, class.ID, classStart)
	for i := 0; i < sessionCount; i++ {
		sess := testutil.Session(t, db, teacherID, class.ID, classStart.AddDate(0, 0, 7*i+1),
			testutil.WithSessionAttendanceConfirmed(time.Now()))
		testutil.AttendanceRecord(t, db, teacherID, sess.ID, student.ID, enrollment.ID)
	}
	return student.ID
}

// getInvoice loads the one invoice a fixture student owns.
func getInvoice(t *testing.T, db *gorm.DB, teacherID, studentID uuid.UUID) billing.Invoice {
	t.Helper()
	var inv billing.Invoice
	require.NoError(t, db.Where("teacher_id = ? AND student_id = ?", teacherID, studentID).Take(&inv).Error)
	return inv
}

// assertLedgerInvariant proves the two independently-maintained views of what
// a teacher has been paid never drift apart: every invoice's paid_amount
// (recomputed by recalcInvoicePaidQuery) must equal the ledger itself — the
// sum of every non-reversal allocation minus every reversal allocation,
// across every payment this teacher has ever recorded.
func assertLedgerInvariant(t *testing.T, db *gorm.DB, teacherID uuid.UUID) {
	t.Helper()
	var invoiceTotal int64
	require.NoError(t, db.Table("invoices").
		Where("teacher_id = ?", teacherID).
		Select("COALESCE(SUM(paid_amount), 0)").Scan(&invoiceTotal).Error)

	var ledgerTotal int64
	require.NoError(t, db.Table("payment_allocations AS pa").
		Joins("JOIN payments p ON p.id = pa.payment_id").
		Where("pa.teacher_id = ?", teacherID).
		Select("COALESCE(SUM(CASE WHEN p.reverses_payment_id IS NULL THEN pa.amount ELSE -pa.amount END), 0)").
		Scan(&ledgerTotal).Error)

	require.Equal(t, invoiceTotal, ledgerTotal,
		"sum(invoices.paid_amount) must equal sum(non-reversal allocations) - sum(reversal allocations)")
}

// TestRecordExactPaymentAcrossTwoChildrenBothPaid proves the base case: one
// contact, two children each with their own invoice, an exact payment clears
// both — two allocation rows, paid_amount matches total_due on each.
func TestRecordExactPaymentAcrossTwoChildrenBothPaid(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)

	studentA := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "A", date("2026-01-01"), 1)
	studentB := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "B", date("2026-01-01"), 2)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invA := getInvoice(t, db, teacher.ID, studentA)
	invB := getInvoice(t, db, teacher.ID, studentB)
	require.EqualValues(t, 100_000, invA.TotalDue)
	require.EqualValues(t, 200_000, invB.TotalDue)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 300_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, detail.UnallocatedAmount)
	require.Len(t, detail.Allocations, 2)

	reloadedA := getInvoice(t, db, teacher.ID, studentA)
	reloadedB := getInvoice(t, db, teacher.ID, studentB)
	require.EqualValues(t, reloadedA.TotalDue, reloadedA.PaidAmount)
	require.Equal(t, billing.InvoicePaid, reloadedA.Status)
	require.EqualValues(t, reloadedB.TotalDue, reloadedB.PaidAmount)
	require.Equal(t, billing.InvoicePaid, reloadedB.Status)
}

// TestRecordUnderpaymentSettlesEarlierClassStartInvoiceFirst proves D8's
// tie-break: two invoices in the same billing period (so period_start is
// equal) are ordered by their earliest class start date, older first.
func TestRecordUnderpaymentSettlesEarlierClassStartInvoiceFirst(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)

	earlierStudent := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Earlier", date("2026-01-01"), 1)
	laterStudent := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Later", date("2026-01-10"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	earlierInvoice := getInvoice(t, db, teacher.ID, earlierStudent)
	laterInvoice := getInvoice(t, db, teacher.ID, laterStudent)
	require.EqualValues(t, 100_000, earlierInvoice.TotalDue)
	require.EqualValues(t, 100_000, laterInvoice.TotalDue)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 150_000, Method: payments.MethodTransfer, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, detail.UnallocatedAmount)

	reloadedEarlier := getInvoice(t, db, teacher.ID, earlierStudent)
	reloadedLater := getInvoice(t, db, teacher.ID, laterStudent)
	require.Equal(t, billing.InvoicePaid, reloadedEarlier.Status, "the earlier-class invoice must be fully paid first")
	require.EqualValues(t, 100_000, reloadedEarlier.PaidAmount)
	require.Equal(t, billing.InvoicePartiallyPaid, reloadedLater.Status)
	require.EqualValues(t, 50_000, reloadedLater.PaidAmount, "only the remainder settles the later-class invoice")
}

// TestRecordOverpaymentReturnsUnallocatedAndCapsAtOutstanding proves an
// overpayment never drives an invoice's paid_amount past total_due; the
// surplus comes back as unallocated_amount instead.
func TestRecordOverpaymentReturnsUnallocatedAndCapsAtOutstanding(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Only", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invoice := getInvoice(t, db, teacher.ID, student)
	require.EqualValues(t, 100_000, invoice.TotalDue)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 150_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 50_000, detail.UnallocatedAmount)
	require.Len(t, detail.Allocations, 1)
	require.EqualValues(t, 100_000, detail.Allocations[0].Amount, "no allocation may exceed the invoice's outstanding")

	reloaded := getInvoice(t, db, teacher.ID, student)
	require.EqualValues(t, 100_000, reloaded.PaidAmount)
	require.Equal(t, billing.InvoicePaid, reloaded.Status)
}

// TestRecordNeverAllocatesToDraftInvoice proves money cannot settle a bill
// that has not been issued yet — the candidate query's status filter excludes
// draft.
func TestRecordNeverAllocatesToDraftInvoice(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Draft", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Draft(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invoice := getInvoice(t, db, teacher.ID, student)
	require.Equal(t, billing.InvoiceDraft, invoice.Status)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 100_000, detail.UnallocatedAmount)
	require.Empty(t, detail.Allocations)

	reloaded := getInvoice(t, db, teacher.ID, student)
	require.Equal(t, billing.InvoiceDraft, reloaded.Status, "a draft invoice's status must never move")
	require.EqualValues(t, 0, reloaded.PaidAmount)
}

// TestRecordNeverAllocatesToVoidInvoice proves the same exclusion for a
// voided invoice, regardless of its total_due.
func TestRecordNeverAllocatesToVoidInvoice(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Void", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invoice := getInvoice(t, db, teacher.ID, student)
	voidReason := "test fixture void"
	voidedAt := time.Now()
	require.NoError(t, db.Table("invoices").Where("id = ?", invoice.ID).Updates(map[string]any{
		"status": billing.InvoiceVoid, "void_reason": voidReason, "voided_at": voidedAt,
	}).Error)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 100_000, detail.UnallocatedAmount)
	require.Empty(t, detail.Allocations)

	reloaded := getInvoice(t, db, teacher.ID, student)
	require.Equal(t, billing.InvoiceVoid, reloaded.Status)
	require.EqualValues(t, 0, reloaded.PaidAmount)
}

// TestRecordAgainstAnotherTeachersContactIsNotFoundAndWritesNothing proves the
// service-level ownership check, not just the composite FK, is what a
// cross-teacher request hits — a 404, not a 500, and zero rows written.
func TestRecordAgainstAnotherTeachersContactIsNotFoundAndWritesNothing(t *testing.T) {
	t.Parallel()
	paymentsSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, stranger := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, owner.ID)

	_, err := paymentsSvc.Record(ctx, stranger.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	var count int64
	require.NoError(t, db.Table("payments").Where("contact_id = ?", contact.ID).Count(&count).Error)
	require.EqualValues(t, 0, count, "a rejected record must write nothing")
}

// TestConcurrentPaymentsForSameContactNeverOverpayInvoice proves the
// candidate invoices' FOR UPDATE lock serialises two payments racing for the
// same contact's single invoice: their combined allocation never exceeds
// total_due, however the goroutines interleave. Run with -race.
func TestConcurrentPaymentsForSameContactNeverOverpayInvoice(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Contested", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invoice := getInvoice(t, db, teacher.ID, student)
	require.EqualValues(t, 100_000, invoice.TotalDue)

	const perPayment = 80_000
	details := make([]*payments.PaymentDetail, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			details[i], errs[i] = paymentsSvc.Record(context.Background(), teacher.ID, payments.RecordPaymentRequest{
				ContactID: contact.ID, Amount: perPayment, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
			})
		}(i)
	}
	wg.Wait()

	var totalAllocated, totalUnallocated int64
	for i, err := range errs {
		require.NoError(t, err)
		for _, a := range details[i].Allocations {
			totalAllocated += a.Amount
		}
		totalUnallocated += details[i].UnallocatedAmount
	}
	require.EqualValues(t, 2*perPayment, totalAllocated+totalUnallocated,
		"no đồng from either payment may be lost or invented")
	require.EqualValues(t, 100_000, totalAllocated,
		"the invoice's outstanding is exactly what both payments together may allocate to it")

	reloaded := getInvoice(t, db, teacher.ID, student)
	require.LessOrEqual(t, reloaded.PaidAmount, reloaded.TotalDue, "paid_amount must never exceed total_due")
	require.EqualValues(t, 100_000, reloaded.PaidAmount)
	require.Equal(t, billing.InvoicePaid, reloaded.Status)
}

// TestConcurrentReallocationsOnSameContactDoNotDeadlock drives two payments
// that each touch the same two invoices, then reallocates both at once in
// opposite directions. Both reallocations lock the same pair of invoices, so
// unless every contact-scoped write path acquires those locks in one agreed
// order they can grab them in opposite orders and deadlock — which Postgres
// surfaces as a rolled-back transaction (a 500 to the teacher), not a wrong
// number. The assertion is therefore that neither call fails with an internal
// error and the ledger still balances afterwards, whichever order won.
func TestConcurrentReallocationsOnSameContactDoNotDeadlock(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	studentEarly := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Early", date("2026-01-01"), 1)
	studentLate := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Late", date("2026-01-10"), 3)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invEarly := getInvoice(t, db, teacher.ID, studentEarly) // total_due 100 000
	invLate := getInvoice(t, db, teacher.ID, studentLate)   // total_due 300 000

	// Two payments, each manually spread 40 000 / 40 000 across both invoices,
	// so both later reallocations must lock invEarly and invLate together.
	p1, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 80_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	_, err = paymentsSvc.Reallocate(ctx, teacher.ID, p1.Payment.ID, payments.ReallocateRequest{
		Allocations: []payments.ReallocationLine{{InvoiceID: invEarly.ID, Amount: 40_000}, {InvoiceID: invLate.ID, Amount: 40_000}},
	})
	require.NoError(t, err)

	p2, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 80_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	_, err = paymentsSvc.Reallocate(ctx, teacher.ID, p2.Payment.ID, payments.ReallocateRequest{
		Allocations: []payments.ReallocationLine{{InvoiceID: invEarly.ID, Amount: 40_000}, {InvoiceID: invLate.ID, Amount: 40_000}},
	})
	require.NoError(t, err)

	reqs := []payments.ReallocateRequest{
		{Allocations: []payments.ReallocationLine{{InvoiceID: invEarly.ID, Amount: 30_000}, {InvoiceID: invLate.ID, Amount: 50_000}}},
		{Allocations: []payments.ReallocationLine{{InvoiceID: invLate.ID, Amount: 30_000}, {InvoiceID: invEarly.ID, Amount: 50_000}}},
	}
	ids := []uuid.UUID{p1.Payment.ID, p2.Payment.ID}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = paymentsSvc.Reallocate(context.Background(), teacher.ID, ids[i], reqs[i])
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			require.NotEqual(t, apperror.CodeInternal, apperror.From(err).Code,
				"a reallocation must never fail with a deadlock/internal error")
		}
	}
	assertLedgerInvariant(t, db, teacher.ID)
}

// TestReverseRestoresInvoiceStateAndKeepsBothPaymentRows proves reversing a
// payment undoes its effect on every invoice it touched exactly — paid_amount
// and status return to their pre-payment values — while never deleting a row:
// the original stays with reversed_at stamped, and the reversal row exists in
// its own right with reverses_payment_id pointing back at it.
func TestReverseRestoresInvoiceStateAndKeepsBothPaymentRows(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	student := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Reversed", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	original, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)

	paid := getInvoice(t, db, teacher.ID, student)
	require.Equal(t, billing.InvoicePaid, paid.Status)
	require.EqualValues(t, 100_000, paid.PaidAmount)
	assertLedgerInvariant(t, db, teacher.ID)

	reversal, err := paymentsSvc.Reverse(ctx, teacher.ID, original.Payment.ID, payments.ReverseRequest{
		Reason: "recorded against the wrong contact",
	})
	require.NoError(t, err)
	require.NotNil(t, reversal.Payment.ReversesPaymentID)
	require.Equal(t, original.Payment.ID, *reversal.Payment.ReversesPaymentID)
	require.Len(t, reversal.Allocations, 1)
	require.EqualValues(t, 100_000, reversal.Allocations[0].Amount)

	restored := getInvoice(t, db, teacher.ID, student)
	require.Equal(t, billing.InvoiceIssued, restored.Status, "reversing the only payment must return the invoice to issued")
	require.EqualValues(t, 0, restored.PaidAmount)

	originalDetail, err := paymentsSvc.Get(ctx, teacher.ID, original.Payment.ID)
	require.NoError(t, err, "the original payment row must still exist after being reversed")
	require.NotNil(t, originalDetail.Payment.ReversedAt)

	reversalDetail, err := paymentsSvc.Get(ctx, teacher.ID, reversal.Payment.ID)
	require.NoError(t, err, "the reversal payment row must exist in its own right")
	require.NotNil(t, reversalDetail.Payment.ReversesPaymentID)
	require.Equal(t, original.Payment.ID, *reversalDetail.Payment.ReversesPaymentID)

	assertLedgerInvariant(t, db, teacher.ID)
}

// TestReverseTwiceIsConflictAndWritesNoNewRow proves a reversal cannot itself
// be undone by reversing the already-reversed original a second time — the
// only correct fix for a wrong reversal is a fresh payment.
func TestReverseTwiceIsConflictAndWritesNoNewRow(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	seedStudentWithSessions(t, db, teacher.ID, contact.ID, "DoubleReverse", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	original, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)

	_, err = paymentsSvc.Reverse(ctx, teacher.ID, original.Payment.ID, payments.ReverseRequest{Reason: "first reversal"})
	require.NoError(t, err)

	var countBefore int64
	require.NoError(t, db.Table("payments").Where("contact_id = ?", contact.ID).Count(&countBefore).Error)
	require.EqualValues(t, 2, countBefore, "original plus one reversal")

	_, err = paymentsSvc.Reverse(ctx, teacher.ID, original.Payment.ID, payments.ReverseRequest{Reason: "second attempt"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var countAfter int64
	require.NoError(t, db.Table("payments").Where("contact_id = ?", contact.ID).Count(&countAfter).Error)
	require.EqualValues(t, countBefore, countAfter, "a rejected second reversal must write no new row")

	assertLedgerInvariant(t, db, teacher.ID)
}

// TestReallocateRebalancesATwoChildSplitOntoOneInvoice proves the manual
// override actually rewrites both sides of a prior D8 split: money moves off
// one invoice entirely (dropping it back to issued) and fully covers the
// other, and every surviving row is now allocated_by=manual.
func TestReallocateRebalancesATwoChildSplitOntoOneInvoice(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	// Earlier class start settles first under D8, so a 100 000 payment
	// clears the 30 000 invoice outright and leaves the 100 000 invoice
	// partially paid at 70 000 — the 70/30 split the reallocation rebalances.
	studentEarly := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Early", date("2026-01-01"), 1)
	studentLate := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Late", date("2026-01-10"), 3)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invEarly := getInvoice(t, db, teacher.ID, studentEarly)
	invLate := getInvoice(t, db, teacher.ID, studentLate)
	require.EqualValues(t, 100_000, invEarly.TotalDue)
	require.EqualValues(t, 300_000, invLate.TotalDue)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 130_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, detail.UnallocatedAmount)

	splitEarly := getInvoice(t, db, teacher.ID, studentEarly)
	splitLate := getInvoice(t, db, teacher.ID, studentLate)
	require.Equal(t, billing.InvoicePaid, splitEarly.Status)
	require.EqualValues(t, 100_000, splitEarly.PaidAmount)
	require.Equal(t, billing.InvoicePartiallyPaid, splitLate.Status)
	require.EqualValues(t, 30_000, splitLate.PaidAmount)
	assertLedgerInvariant(t, db, teacher.ID)

	reallocated, err := paymentsSvc.Reallocate(ctx, teacher.ID, detail.Payment.ID, payments.ReallocateRequest{
		Allocations: []payments.ReallocationLine{{InvoiceID: invLate.ID, Amount: 130_000}},
	})
	require.NoError(t, err)
	require.Len(t, reallocated.Allocations, 1)
	require.Equal(t, invLate.ID, reallocated.Allocations[0].InvoiceID)
	require.EqualValues(t, 130_000, reallocated.Allocations[0].Amount)
	require.Equal(t, payments.AllocatedManual, reallocated.Allocations[0].AllocatedBy)

	rebalancedEarly := getInvoice(t, db, teacher.ID, studentEarly)
	rebalancedLate := getInvoice(t, db, teacher.ID, studentLate)
	require.Equal(t, billing.InvoiceIssued, rebalancedEarly.Status, "the emptied invoice must drop back to issued")
	require.EqualValues(t, 0, rebalancedEarly.PaidAmount)
	require.Equal(t, billing.InvoicePartiallyPaid, rebalancedLate.Status)
	require.EqualValues(t, 130_000, rebalancedLate.PaidAmount)

	assertLedgerInvariant(t, db, teacher.ID)
}

// TestReallocateToAnotherContactsInvoiceIsRejectedAndWritesNothing proves the
// cross-contact guard runs before any row is touched — the payment's original
// allocation survives untouched.
func TestReallocateToAnotherContactsInvoiceIsRejectedAndWritesNothing(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contactA := testutil.Contact(t, db, teacher.ID)
	contactB := testutil.Contact(t, db, teacher.ID)
	seedStudentWithSessions(t, db, teacher.ID, contactA.ID, "OwnerA", date("2026-01-01"), 1)
	studentB := seedStudentWithSessions(t, db, teacher.ID, contactB.ID, "OwnerB", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, period.ID)
	require.NoError(t, err)

	invoiceB := getInvoice(t, db, teacher.ID, studentB)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contactA.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.Len(t, detail.Allocations, 1)

	_, err = paymentsSvc.Reallocate(ctx, teacher.ID, detail.Payment.ID, payments.ReallocateRequest{
		Allocations: []payments.ReallocationLine{{InvoiceID: invoiceB.ID, Amount: 100_000}},
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	unchanged, err := paymentsSvc.Get(ctx, teacher.ID, detail.Payment.ID)
	require.NoError(t, err)
	require.Len(t, unchanged.Allocations, 1, "a rejected reallocation must leave the payment's existing allocation untouched")
	require.Equal(t, detail.Allocations[0].InvoiceID, unchanged.Allocations[0].InvoiceID)

	assertLedgerInvariant(t, db, teacher.ID)
}

// TestAutoAllocateRemainderPlacesSurplusOnANewlyIssuedInvoice proves re-running
// D8 over a payment's leftover money reaches invoices that did not exist yet
// when the payment was first recorded — the surplus that came back as
// unallocated_amount is not stuck; the next period's invoice can absorb it.
func TestAutoAllocateRemainderPlacesSurplusOnANewlyIssuedInvoice(t *testing.T) {
	t.Parallel()
	paymentsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	studentJan := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Jan", date("2026-01-01"), 1)

	periodJan, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, periodJan.ID)
	require.NoError(t, err)

	invoiceJan := getInvoice(t, db, teacher.ID, studentJan)
	require.EqualValues(t, 100_000, invoiceJan.TotalDue)

	detail, err := paymentsSvc.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 150_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	require.EqualValues(t, 50_000, detail.UnallocatedAmount)
	assertLedgerInvariant(t, db, teacher.ID)

	studentFeb := seedStudentWithSessions(t, db, teacher.ID, contact.ID, "Feb", date("2026-02-01"), 1)
	periodFeb, err := billingSvc.EnsurePeriod(ctx, teacher.ID, 2026, 2)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, teacher.ID, periodFeb.ID)
	require.NoError(t, err)

	invoiceFeb := getInvoice(t, db, teacher.ID, studentFeb)
	require.Equal(t, billing.InvoiceIssued, invoiceFeb.Status)
	require.EqualValues(t, 100_000, invoiceFeb.TotalDue)

	reallocated, err := paymentsSvc.AutoAllocateRemainder(ctx, teacher.ID, detail.Payment.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, reallocated.UnallocatedAmount)
	require.Len(t, reallocated.Allocations, 2)

	settledFeb := getInvoice(t, db, teacher.ID, studentFeb)
	require.Equal(t, billing.InvoicePartiallyPaid, settledFeb.Status)
	require.EqualValues(t, 50_000, settledFeb.PaidAmount)

	// Re-running with nothing left to place is a conflict, not a silent no-op.
	_, err = paymentsSvc.AutoAllocateRemainder(ctx, teacher.ID, detail.Payment.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	assertLedgerInvariant(t, db, teacher.ID)
}
