//go:build integration

package statements_test

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// testTokenKey is a fixed, non-secret key for these tests only — it never
// protects anything real, so reusing a literal here carries none of the risk
// a production key would.
var testTokenKey = []byte("integration-test-token-key-32-bytes")

// testBankConfig is an obviously-fake bank fixture: not a real bank code or
// account number.
var testBankConfig = statements.BankConfig{BankCode: "TESTBANK", AccountNumber: "0000000000", AccountName: "NGUYEN VAN A"}

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// newIntegrationDeps wires the real dependency chain router.go uses:
// statements consumes nothing from billing directly — they only share
// tables — but the fixtures need a real billing service to produce invoices
// a period can be closed with.
func newIntegrationDeps(t *testing.T) (*statements.Service, *billing.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	billingSvc := billing.NewService(billing.NewRepository(db, attendanceSvc), txMgr, sessionsSvc, enrollmentsSvc)
	statementsSvc := statements.NewService(statements.NewRepository(db), txMgr, config.StatementsConfig{
		TokenKey:      testTokenKey,
		PublicBaseURL: "https://parent.example.com",
	}, testBankConfig, statements.NewQRBuilder())
	return statementsSvc, billingSvc, db
}

// seededChild is one class+student+enrollment fixture with sessionCount
// held+confirmed sessions (100 000 đồng each) under contactID.
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

// hashOf mirrors this package's own token->hash digest so tests can assert
// against statements.token_hash without depending on unexported helpers.
func hashOf(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

type statementRow struct {
	ID        uuid.UUID
	TeacherID uuid.UUID
	ContactID uuid.UUID
	PeriodID  uuid.UUID
	TokenHash []byte
	TotalDue  int64
	RevokedAt *time.Time
}

func loadStatement(t *testing.T, db *gorm.DB, teacherID, contactID, periodID uuid.UUID) statementRow {
	t.Helper()
	var row statementRow
	require.NoError(t, db.Table("statements").
		Where("teacher_id = ? AND contact_id = ? AND period_id = ?", teacherID, contactID, periodID).
		Take(&row).Error)
	return row
}

// TestGenerateOpenPeriodFailsWithoutWritingAnything proves Generate refuses
// to run against a period still open, and that the refusal writes nothing.
func TestGenerateOpenPeriodFailsWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Open", date("2026-02-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 2)
	require.NoError(t, err)

	_, err = statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var count int64
	require.NoError(t, db.Table("statements").Where("period_id = ?", period.ID).Count(&count).Error)
	require.Zero(t, count, "a rejected generate must leave no row behind")
}

// TestGenerateClosedPeriodWritesOneRowPerContactWithDistinctTokens proves the
// core happy path: two contacts each get one statement, and their tokens
// (and therefore their links) never collide.
func TestGenerateClosedPeriodWritesOneRowPerContactWithDistinctTokens(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contactA := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact A"))
	seedChild(t, db, teacher.ID, contactA.ID, "A1", date("2026-03-01"), 1)
	contactB := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Contact B"))
	seedChild(t, db, teacher.ID, contactB.ID, "B1", date("2026-03-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 3)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Zero(t, result.Refreshed)
	require.Zero(t, result.SkippedRevoked)
	require.Len(t, result.Statements, 2)

	rowA := loadStatement(t, db, teacher.ID, contactA.ID, period.ID)
	rowB := loadStatement(t, db, teacher.ID, contactB.ID, period.ID)
	require.EqualValues(t, 100_000, rowA.TotalDue)
	require.EqualValues(t, 100_000, rowB.TotalDue)
	require.NotEqual(t, rowA.TokenHash, rowB.TokenHash, "two statements must never share a token hash")

	// The URL a teacher-authenticated response carries must embed a token
	// that actually hashes to the row's persisted token_hash — proving
	// ToResponse's on-the-fly derivation round-trips correctly.
	resp := statementsSvc.ToResponse(testutil.ScopeFor(t, db, teacher.ID), result.Statements[0])
	require.NotNil(t, resp.URL)
	token := (*resp.URL)[len("https://parent.example.com/s/"):]
	require.Equal(t, result.Statements[0].TokenHash, hashOf(token))
}

// TestGenerateSkipsContactWithOnlyAVoidedInvoice proves a contact whose only
// invoice this period was voided outright gets no statement —
// TargetContacts only reads non-void invoices, the same rule collections'
// board already relies on for the same table.
func TestGenerateSkipsContactWithOnlyAVoidedInvoice(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Voided", date("2026-04-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 4)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	require.NoError(t, db.Table("invoices").
		Where("teacher_id = ? AND contact_id = ?", teacher.ID, contact.ID).
		Updates(map[string]any{
			"status": billing.InvoiceVoid, "void_reason": "test fixture void", "voided_at": time.Now(),
		}).Error)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Zero(t, result.Created)
	require.Empty(t, result.Statements)

	var count int64
	require.NoError(t, db.Table("statements").Where("period_id = ? AND contact_id = ?", period.ID, contact.ID).Count(&count).Error)
	require.Zero(t, count, "a contact with only a voided invoice must get no statement")
}

// TestGenerateSumsTwoChildrenUnderOneStatement proves a family with two
// children gets exactly one statement, its total the sum across both.
func TestGenerateSumsTwoChildrenUnderOneStatement(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Child1", date("2026-05-01"), 1)
	seedChild(t, db, teacher.ID, contact.ID, "Child2", date("2026-05-02"), 2)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 5)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Len(t, result.Statements, 1)
	require.EqualValues(t, 300_000, result.Statements[0].TotalDue)

	var count int64
	require.NoError(t, db.Table("statements").Where("period_id = ? AND contact_id = ?", period.ID, contact.ID).Count(&count).Error)
	require.EqualValues(t, 1, count, "a two-child family must produce exactly one statement")
}

// TestGenerateTwiceKeepsSameIDAndTokenRefreshesTotalDue proves re-running
// Generate is safe to call repeatedly: the statement's identity and link
// survive, only its total changes.
func TestGenerateTwiceKeepsSameIDAndTokenRefreshesTotalDue(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Refresh", date("2026-06-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 6)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	first, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Len(t, first.Statements, 1)

	// A billing adjustment changes the contact's total_due without touching
	// the statement directly, so the second Generate call has something real
	// to refresh. adjustment_total and total_due move together — the table's
	// own CHECK constraint requires total_due = opening_balance +
	// current_charge + adjustment_total.
	var invoiceRow struct{ ID uuid.UUID }
	require.NoError(t, db.Table("invoices").Select("id").
		Where("teacher_id = ? AND contact_id = ? AND period_id = ?", teacher.ID, contact.ID, period.ID).
		Take(&invoiceRow).Error)
	require.NoError(t, db.Exec(
		`UPDATE invoices SET adjustment_total = adjustment_total + 25000, total_due = total_due + 25000, updated_at = now() WHERE id = ?`,
		invoiceRow.ID,
	).Error)

	second, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Zero(t, second.Created)
	require.Equal(t, 1, second.Refreshed)
	require.Len(t, second.Statements, 1)

	require.Equal(t, first.Statements[0].ID, second.Statements[0].ID, "refresh must keep the same statement id")
	require.Equal(t, first.Statements[0].TokenHash, second.Statements[0].TokenHash, "refresh must never rotate token_hash")
	require.Equal(t, first.Statements[0].TotalDue+25_000, second.Statements[0].TotalDue, "total_due must be refreshed by exactly the adjustment")

	row := loadStatement(t, db, teacher.ID, contact.ID, period.ID)
	require.Equal(t, first.Statements[0].TokenHash, row.TokenHash, "the persisted token_hash must match what generate reported")
}

// TestRevokeThenGenerateLeavesTheStatementRevoked proves a revoked statement
// is never resurrected by a later Generate call for the same period.
func TestRevokeThenGenerateLeavesTheStatementRevoked(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Revoke", date("2026-07-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 7)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	first, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	statementID := first.Statements[0].ID

	require.NoError(t, statementsSvc.Revoke(ctx, testutil.ScopeFor(t, db, teacher.ID), statementID))

	second, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Zero(t, second.Created)
	require.Zero(t, second.Refreshed)
	require.Equal(t, 1, second.SkippedRevoked)
	require.Empty(t, second.Statements)

	row := loadStatement(t, db, teacher.ID, contact.ID, period.ID)
	require.NotNil(t, row.RevokedAt, "a revoked statement must stay revoked across a later generate")
}

// TestTokenHashIsGloballyUnique proves uq_statements_token is real: two
// statements can never share a token_hash, even across different contacts,
// periods, and teachers.
func TestTokenHashIsGloballyUnique(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Unique", date("2026-08-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 8)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	_, err = statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	existing := loadStatement(t, db, teacher.ID, contact.ID, period.ID)
	require.NotEmpty(t, existing.TokenHash)

	otherContact := testutil.Contact(t, db, teacher.ID)
	otherPeriod, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 9)
	require.NoError(t, err)

	err = db.Exec(
		`INSERT INTO statements (id, teacher_id, contact_id, period_id, token_hash, expires_at, total_due, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, now() + interval '90 days', 0, now(), now())`,
		uuid.Must(uuid.NewV7()), teacher.ID, otherContact.ID, otherPeriod.ID, existing.TokenHash,
	).Error
	require.Error(t, err, "inserting a second row with a token hash already in use must violate uq_statements_token")
}

// TestNoTeacherEndpointLeaksAnotherTeachersStatement proves cross-tenant
// reads fail closed: a statement id from teacher A resolves as not found
// under teacher B's identity, so no other teacher's endpoint response can
// ever carry teacher A's parent link.
func TestNoTeacherEndpointLeaksAnotherTeachersStatement(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	_, teacherA := testutil.Teacher(t, db)
	_, teacherB := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacherA.ID)
	seedChild(t, db, teacherA.ID, contact.ID, "Cross", date("2026-10-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacherA.ID), 2026, 10)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacherA.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacherA.ID), period.ID)
	require.NoError(t, err)
	statementID := result.Statements[0].ID

	_, err = statementsSvc.Get(ctx, testutil.ScopeFor(t, db, teacherB.ID), statementID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, _, err = statementsSvc.List(ctx, testutil.ScopeFor(t, db, teacherB.ID), period.ID, pagination.Params{})
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	require.Error(t, statementsSvc.Revoke(ctx, testutil.ScopeFor(t, db, teacherB.ID), statementID))
}
