//go:build integration

package notifications_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// testTokenKey is a fixed, non-secret key for these tests only — it never
// protects anything real, so reusing a literal here carries none of the risk
// a production key would.
var testTokenKey = []byte("integration-test-notifications-key-32b")

func date(s string) time.Time {
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return tm
}

// deps is the real dependency chain router.go wires for notifications, plus
// everything the fixtures need to produce closed periods, payments, and
// voided invoices to seed against.
type deps struct {
	notifications *notifications.Service
	statements    *statements.Service
	billing       *billing.Service
	payments      *payments.Service
	db            *gorm.DB
}

func newDeps(t *testing.T) *deps {
	t.Helper()
	db := testutil.StartPostgres(t)
	return newDepsOnDB(t, db)
}

// newDepsOnDB wires the same dependency chain against an already-started db
// handle, so the scale test can attach a query counter to the handle before
// any service is constructed. The Zalo side is a benign fake — these tests
// exercise the copy-paste channels, which never touch it.
func newDepsOnDB(t *testing.T, db *gorm.DB) *deps {
	t.Helper()
	return newDepsWithZalo(t, db, &fakeZaloSender{})
}

// newDepsWithZalo wires the chain with a caller-controlled Zalo fake, for
// tests driving the zalo_personal channel.
func newDepsWithZalo(t *testing.T, db *gorm.DB, zaloSender notifications.ZaloSender) *deps {
	t.Helper()
	return newDepsWithZaloAndRunCap(t, db, zaloSender, 50)
}

func newDepsWithZaloAndRunCap(t *testing.T, db *gorm.DB, zaloSender notifications.ZaloSender, maxRunSize int) *deps {
	t.Helper()
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	billingSvc := billing.NewService(billing.NewRepository(db, attendanceSvc), txMgr, sessionsSvc, enrollmentsSvc)
	statementsSvc := statements.NewService(statements.NewRepository(db), txMgr, config.StatementsConfig{
		TokenKey:      testTokenKey,
		PublicBaseURL: "https://parent.example.com",
	}, statements.BankConfig{}, statements.NewQRBuilder())
	paymentsSvc := payments.NewService(payments.NewRepository(db), txMgr)
	notificationsSvc := notifications.NewService(notifications.NewRepository(db), txMgr, statementsSvc, zaloSender,
		slog.New(slog.DiscardHandler), config.NotificationsConfig{
			DefaultChannel: notifications.ChannelZaloManual,
			MaxMessageLen:  1000,
			// Zero pacing: integration runs must finish in milliseconds, not
			// minutes — the gap arithmetic itself is pinned by unit tests.
			PaceMinSeconds: 0,
			PaceMaxSeconds: 0,
			MaxRunSize:     maxRunSize,
		})
	t.Cleanup(notificationsSvc.Close)
	return &deps{
		notifications: notificationsSvc,
		statements:    statementsSvc,
		billing:       billingSvc,
		payments:      paymentsSvc,
		db:            db,
	}
}

// seedChild is one class+student+enrollment fixture with sessionCount
// held+confirmed sessions (100 000 đồng each) under contactID — mirrors
// statements' own seedChild helper (unexported there, so duplicated here).
func seedChild(t *testing.T, db *gorm.DB, teacherID, contactID uuid.UUID, name string, classStart time.Time, sessionCount int) {
	t.Helper()
	class := testutil.Class(t, db, teacherID, testutil.WithClassName(name), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, teacherID, contactID, testutil.WithStudentFullName(name+"-student"))
	enrollment := testutil.Enrollment(t, db, teacherID, student.ID, class.ID, classStart)
	for i := 0; i < sessionCount; i++ {
		sess := testutil.Session(t, db, teacherID, class.ID, classStart.AddDate(0, 0, 7*i+1),
			testutil.WithSessionAttendanceConfirmed(time.Now()))
		testutil.AttendanceRecord(t, db, teacherID, sess.ID, student.ID, enrollment.ID)
	}
}

// notificationCount counts every non-deleted notifications row for periodID,
// joined through statement_id -> statements.period_id since notifications
// carries no period_id column of its own.
func notificationCount(t *testing.T, db *gorm.DB, periodID uuid.UUID) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Table("notifications").
		Joins("JOIN statements s ON s.id = notifications.statement_id").
		Where("s.period_id = ? AND notifications.deleted_at IS NULL", periodID).
		Count(&count).Error)
	return count
}

// sentAtOf reads one notification row's sent_at column directly.
func sentAtOf(t *testing.T, db *gorm.DB, notificationID uuid.UUID) *time.Time {
	t.Helper()
	var row struct {
		SentAt *time.Time
	}
	require.NoError(t, db.Table("notifications").Select("sent_at").
		Where("id = ?", notificationID).Take(&row).Error)
	return row.SentAt
}

// publicRouter mounts only the public, unauthenticated statement route group
// against svc — the endpoint a bulk-sent message's URL must resolve against.
func publicRouter(svc *statements.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	statements.RegisterPublicRoutes(r, statements.NewPublicHandler(svc))
	return r
}

// fetchStatementURL extracts the token from a bulk-sent message's trailing
// URL and resolves it against the real public route, returning the response
// status — the "working statement URL" the phase's own acceptance criterion
// requires.
func fetchStatementURL(t *testing.T, svc *statements.Service, statementURL string) int {
	t.Helper()
	const prefix = "https://parent.example.com/s/"
	require.True(t, strings.HasPrefix(statementURL, prefix), "unexpected statement url shape: %s", statementURL)
	token := statementURL[len(prefix):]

	req := httptest.NewRequest(http.MethodGet, "/public/statements/"+token, nil)
	rec := httptest.NewRecorder()
	publicRouter(svc).ServeHTTP(rec, req)
	return rec.Code
}

// TestBulkSendStatementsTargetsUnpaidPaidAndOldDebtButSkipsVoided is the
// phase's core scenario: one closed period with contact A (two children,
// unpaid), contact B (one child, paid in full), contact C (one child whose
// only invoice was voided), and contact D carrying old debt from an earlier
// closed period. purpose=statement must reach A, B, and D — never C — with
// exactly one row per contact regardless of child count, the right total in
// A's message, the Nợ cũ line only where opening_balance is non-zero, and no
// cross-contact name leakage.
func TestBulkSendStatementsTargetsUnpaidPaidAndOldDebtButSkipsVoided(t *testing.T) {
	t.Parallel()
	d := newDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)

	// Contact D's old debt: one child in an earlier, already-closed period,
	// left unpaid so its outstanding balance carries forward as the target
	// period's opening_balance.
	contactD := testutil.Contact(t, d.db, teacher.ID, testutil.WithContactFullName("Debt Parent"))
	seedChild(t, d.db, teacher.ID, contactD.ID, "OldDebtChild", date("2026-01-05"), 1)
	periodJan, err := d.billing.EnsurePeriod(ctx, testutil.ScopeFor(t, d.db, teacher.ID), 2026, 1)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, testutil.ScopeFor(t, d.db, teacher.ID), periodJan.ID)
	require.NoError(t, err)

	contactA := testutil.Contact(t, d.db, teacher.ID, testutil.WithContactFullName("Alpha Parent"))
	seedChild(t, d.db, teacher.ID, contactA.ID, "AlphaChildOne", date("2026-02-02"), 1)
	seedChild(t, d.db, teacher.ID, contactA.ID, "AlphaChildTwo", date("2026-02-03"), 2)

	contactB := testutil.Contact(t, d.db, teacher.ID, testutil.WithContactFullName("Beta Parent"))
	seedChild(t, d.db, teacher.ID, contactB.ID, "BetaChild", date("2026-02-02"), 1)

	contactC := testutil.Contact(t, d.db, teacher.ID, testutil.WithContactFullName("Charlie Parent"))
	seedChild(t, d.db, teacher.ID, contactC.ID, "CharlieChild", date("2026-02-02"), 1)

	periodFeb, err := d.billing.EnsurePeriod(ctx, testutil.ScopeFor(t, d.db, teacher.ID), 2026, 2)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, testutil.ScopeFor(t, d.db, teacher.ID), periodFeb.ID)
	require.NoError(t, err)

	// Beta pays in full.
	var betaInvoice struct {
		ID       uuid.UUID
		TotalDue int64
	}
	require.NoError(t, d.db.Table("invoices").Select("id, total_due").
		Where("teacher_id = ? AND contact_id = ? AND period_id = ?", teacher.ID, contactB.ID, periodFeb.ID).
		Take(&betaInvoice).Error)
	_, err = d.payments.Record(ctx, teacher.ID, payments.RecordPaymentRequest{
		ContactID:  contactB.ID,
		Amount:     betaInvoice.TotalDue,
		Method:     "cash",
		ReceivedOn: "2026-02-20",
	})
	require.NoError(t, err)

	// Charlie's only invoice this period is voided.
	var charlieInvoice struct{ ID uuid.UUID }
	require.NoError(t, d.db.Table("invoices").Select("id").
		Where("teacher_id = ? AND contact_id = ? AND period_id = ?", teacher.ID, contactC.ID, periodFeb.ID).
		Take(&charlieInvoice).Error)
	_, err = d.billing.VoidInvoice(ctx, testutil.ScopeFor(t, d.db, teacher.ID), charlieInvoice.ID, "test fixture void")
	require.NoError(t, err)

	// purpose=statement: A, B, D only — never C.
	stmtResp, err := d.notifications.BulkSend(ctx, teacher.ID, periodFeb.ID, notifications.BulkSendRequest{Purpose: "statement"})
	require.NoError(t, err)
	require.Equal(t, 3, stmtResp.QueuedCount, "expected exactly one row each for A, B, D")
	require.Len(t, stmtResp.Rows, 3)

	byContact := make(map[uuid.UUID]notifications.BulkSendRow, len(stmtResp.Rows))
	for _, row := range stmtResp.Rows {
		byContact[row.ContactID] = row
	}
	_, hasCharlie := byContact[contactC.ID]
	require.False(t, hasCharlie, "a contact with only a voided invoice must receive no notification")

	rowA, ok := byContact[contactA.ID]
	require.True(t, ok, "contact A must be targeted")
	require.Contains(t, rowA.MessageText, "AlphaChildOne", "one message must name every child, not one per child")
	require.Contains(t, rowA.MessageText, "AlphaChildTwo")
	require.Contains(t, rowA.MessageText, "Tổng cộng: 300.000 đ", "total must equal the sum of both children's invoices")
	require.NotContains(t, rowA.MessageText, "Nợ cũ:", "a contact with no opening balance must show no old-debt line")

	rowD, ok := byContact[contactD.ID]
	require.True(t, ok, "contact D must be targeted")
	require.Contains(t, rowD.MessageText, "Nợ cũ: 100.000 đ", "old debt carried in opening_balance must appear")

	rowB, ok := byContact[contactB.ID]
	require.True(t, ok, "a fully-paid contact still receives a statement notification, distinct from a reminder")

	// No message may leak another contact's name.
	for _, row := range []notifications.BulkSendRow{rowA, rowB, rowD} {
		for _, other := range []string{"Alpha Parent", "Beta Parent", "Charlie Parent", "Debt Parent"} {
			if row.ContactName == other {
				continue
			}
			require.NotContains(t, row.MessageText, other, "message must never carry another contact's name")
		}
	}
	// Nor another family's child.
	require.NotContains(t, rowB.MessageText, "AlphaChildOne")
	require.NotContains(t, rowB.MessageText, "AlphaChildTwo")
	require.NotContains(t, rowB.MessageText, "OldDebtChild")
	require.NotContains(t, rowD.MessageText, "AlphaChildOne")
	require.NotContains(t, rowD.MessageText, "BetaChild")

	// Every message's URL must resolve to a live statement — except Beta's:
	// RenderPublic deliberately 404s a statement the instant its outstanding
	// balance reaches zero (statements/service.go), and Beta paid in full, so
	// its own link resolving 404 here is the correct behavior, not a bug.
	for _, row := range []notifications.BulkSendRow{rowA, rowD} {
		require.Equal(t, http.StatusOK, fetchStatementURL(t, d.statements, row.URL), "contact %s's statement link must resolve", row.ContactName)
	}

	// purpose=reminder: A and D only (outstanding > 0) — never B, which is
	// fully paid. A family with two children both in debt still gets exactly
	// one reminder row.
	reminderResp, err := d.notifications.BulkSend(ctx, teacher.ID, periodFeb.ID, notifications.BulkSendRequest{Purpose: "reminder"})
	require.NoError(t, err)
	require.Equal(t, 2, reminderResp.QueuedCount)
	require.Equal(t, 1, reminderResp.SkippedPaidCount, "the fully-paid contact must be counted as skipped, not queued")

	reminderContacts := make(map[uuid.UUID]bool, len(reminderResp.Rows))
	for _, row := range reminderResp.Rows {
		reminderContacts[row.ContactID] = true
		require.Equal(t, notifications.PurposeReminder, row.Purpose)
	}
	require.True(t, reminderContacts[contactA.ID])
	require.True(t, reminderContacts[contactD.ID])
	require.False(t, reminderContacts[contactB.ID])

	// Mark-sent is idempotent: a second call on an already-sent id changes
	// nothing.
	require.NoError(t, d.notifications.MarkSent(ctx, teacher.ID, []uuid.UUID{rowA.NotificationID}))
	firstSentAt := sentAtOf(t, d.db, rowA.NotificationID)
	require.NotNil(t, firstSentAt)

	require.NoError(t, d.notifications.MarkSent(ctx, teacher.ID, []uuid.UUID{rowA.NotificationID}))
	secondSentAt := sentAtOf(t, d.db, rowA.NotificationID)
	require.NotNil(t, secondSentAt)
	require.True(t, firstSentAt.Equal(*secondSentAt), "marking sent twice must not change sent_at")
}

// TestBulkSendOnOpenPeriodFailsWithoutWritingAnything proves the 409 refusal
// leaves no notification (and, since Generate runs inside the same
// transaction, no statement) behind.
func TestBulkSendOnOpenPeriodFailsWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	d := newDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)

	contact := testutil.Contact(t, d.db, teacher.ID)
	seedChild(t, d.db, teacher.ID, contact.ID, "OpenPeriodChild", date("2026-03-01"), 1)

	period, err := d.billing.EnsurePeriod(ctx, testutil.ScopeFor(t, d.db, teacher.ID), 2026, 3)
	require.NoError(t, err)

	_, err = d.notifications.BulkSend(ctx, teacher.ID, period.ID, notifications.BulkSendRequest{Purpose: "statement"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	require.Zero(t, notificationCount(t, d.db, period.ID))

	var statementCount int64
	require.NoError(t, d.db.Table("statements").Where("period_id = ?", period.ID).Count(&statementCount).Error)
	require.Zero(t, statementCount, "a rejected bulk send must leave no statement behind either")
}

// TestBulkSendWithUnconfiguredChannelFailsWithoutWritingAnything proves a
// channel that cannot currently send (zalo_zns, until PRD Q1 is answered)
// aborts the whole call — including the statement refresh — rather than
// leaving partial state behind.
func TestBulkSendWithUnconfiguredChannelFailsWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	d := newDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)

	contact := testutil.Contact(t, d.db, teacher.ID)
	seedChild(t, d.db, teacher.ID, contact.ID, "ZnsChild", date("2026-04-01"), 1)

	period, err := d.billing.EnsurePeriod(ctx, testutil.ScopeFor(t, d.db, teacher.ID), 2026, 4)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, testutil.ScopeFor(t, d.db, teacher.ID), period.ID)
	require.NoError(t, err)

	_, err = d.notifications.BulkSend(ctx, teacher.ID, period.ID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloZNS,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeBadRequest, apperror.From(err).Code)
	require.Contains(t, err.Error(), "not configured")

	require.Zero(t, notificationCount(t, d.db, period.ID))
}

// queryCounter counts every database round trip GORM issues on the handle it
// is attached to — query, row-scan, raw exec, create, update, and delete
// callback chains, the six chains a service call can ever go through. Built
// from scratch: no shared query-counting utility exists elsewhere in this
// codebase.
type queryCounter struct {
	n int64
}

func (qc *queryCounter) attach(db *gorm.DB) {
	inc := func(*gorm.DB) { atomic.AddInt64(&qc.n, 1) }
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(db.Callback().Query().Register("test:count_query", inc))
	must(db.Callback().Row().Register("test:count_row", inc))
	must(db.Callback().Raw().Register("test:count_raw", inc))
	must(db.Callback().Create().Register("test:count_create", inc))
	must(db.Callback().Update().Register("test:count_update", inc))
	must(db.Callback().Delete().Register("test:count_delete", inc))
}

func (qc *queryCounter) reset()       { atomic.StoreInt64(&qc.n, 0) }
func (qc *queryCounter) count() int64 { return atomic.LoadInt64(&qc.n) }

// TestBulkSendScalesToFiftyContactsUnderThreeSeconds seeds 50 contacts and 80
// students (30 with a second child) and proves one bulk send finishes well
// under R6's 10-minute-for-50-students bar and issues a query count that
// grows only with the number of contacts, never with the number of children
// or invoice lines — the property InsertBatch's single multi-row insert and
// PeriodFigures' single aggregation query exist to guarantee. Generate's own
// per-contact UpsertStatement call (statements/service.go) is a known,
// already-accepted linear cost from an earlier phase, so the bound below is
// a generous linear-in-contacts ceiling, not a constant: it is what proves
// notifications' own additions never regress to one query per child.
func TestBulkSendScalesToFiftyContactsUnderThreeSeconds(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	d := newDepsOnDB(t, db)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	const contactCount = 50
	const twoChildContacts = 30
	classStart := date("2026-05-04")
	for i := 0; i < contactCount; i++ {
		contact := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName(fmt.Sprintf("Scale Parent %02d", i)))
		seedChild(t, db, teacher.ID, contact.ID, fmt.Sprintf("ScaleChild%02dA", i), classStart, 1)
		if i < twoChildContacts {
			seedChild(t, db, teacher.ID, contact.ID, fmt.Sprintf("ScaleChild%02dB", i), classStart, 1)
		}
	}

	var studentCount int64
	require.NoError(t, db.Table("students").Where("teacher_id = ?", teacher.ID).Count(&studentCount).Error)
	require.EqualValues(t, contactCount+twoChildContacts, studentCount, "fixture must produce 80 students across 50 contacts")

	period, err := d.billing.EnsurePeriod(ctx, testutil.ScopeFor(t, d.db, teacher.ID), 2026, 5)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, testutil.ScopeFor(t, d.db, teacher.ID), period.ID)
	require.NoError(t, err)

	counter := &queryCounter{}
	counter.attach(db)
	counter.reset()

	start := time.Now()
	resp, err := d.notifications.BulkSend(ctx, teacher.ID, period.ID, notifications.BulkSendRequest{Purpose: "statement"})
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Equal(t, contactCount, resp.QueuedCount)

	require.Less(t, elapsed, 3*time.Second, "50 contacts must complete in one call under 3 seconds")

	got := counter.count()
	// A generous per-contact allowance (Generate's own per-contact upsert)
	// plus a small fixed overhead for the handful of period/aggregate/batch
	// queries the rest of the call issues.
	bound := int64(contactCount)*3 + 20
	require.Lessf(t, got, bound, "query count %d must stay roughly linear in contact count (50), not grow with the 80 students/children", got)
}
