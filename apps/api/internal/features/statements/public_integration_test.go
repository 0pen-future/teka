//go:build integration

package statements_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/testutil"
)

// neutralCacheControl/qrCacheControl are the two Cache-Control values this
// route ever emits: the route group default (JSON get, and every 404) and
// qrImage's own short-private override.
const (
	neutralCacheControl = "no-store, no-cache, must-revalidate, private"
	qrCacheControl      = "private, max-age=300"
)

// newPublicIntegrationDeps extends newIntegrationDeps (integration_test.go,
// same package) with a real payments service — needed for the payment
// tests below but not for statement generation itself.
func newPublicIntegrationDeps(t *testing.T) (*statements.Service, *billing.Service, *payments.Service, *gorm.DB) {
	t.Helper()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	paymentsSvc := payments.NewService(payments.NewRepository(db), database.NewTxManager(db))
	return statementsSvc, billingSvc, paymentsSvc, db
}

// newPublicRouter mounts only the public, unauthenticated route group —
// nothing else the real router.go wires in — against svc.
func newPublicRouter(svc *statements.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	statements.RegisterPublicRoutes(r, statements.NewPublicHandler(svc))
	return r
}

// tokenOf recomputes one generated row's plaintext token the same way a
// teacher-facing response would have carried it, so a test can drive the
// public route without a real HTTP round trip through the authenticated
// side.
func tokenOf(t *testing.T, svc *statements.Service, row statements.Row) string {
	t.Helper()
	resp := svc.ToResponse(row)
	const prefix = "https://parent.example.com/s/"
	require.True(t, strings.HasPrefix(resp.URL, prefix), "unexpected url shape: %s", resp.URL)
	return resp.URL[len(prefix):]
}

// unknownToken returns a plausible, well-formed token that was never issued
// by any Generate call — the "unknown token" 404 case, as opposed to a
// syntactically garbage one.
func unknownToken() string {
	return base64.RawURLEncoding.EncodeToString(make([]byte, 32))
}

// assertSecurityHeaders checks the three headers every response from this
// route must carry, on both success and the neutral 404.
func assertSecurityHeaders(t *testing.T, h http.Header, wantCacheControl string) {
	t.Helper()
	require.Equal(t, "noindex, nofollow, noarchive", h.Get("X-Robots-Tag"))
	require.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
	require.Equal(t, wantCacheControl, h.Get("Cache-Control"))
}

// publicEnvelope decodes the JSON get route's success envelope.
type publicEnvelope struct {
	Success bool                        `json:"success"`
	Data    *statements.PublicStatement `json:"data"`
}

// getPublic drives one GET against token and decodes its success envelope,
// failing the test on anything but 200.
func getPublic(t *testing.T, router *gin.Engine, token string) *statements.PublicStatement {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var env publicEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.NotNil(t, env.Data)
	return env.Data
}

// TestPublicGetValidTokenReturnsBothChildrenAndFamilyTotal proves the happy
// path end to end over real HTTP: a two-child family's statement carries
// both children, and the family total is their sum.
func TestPublicGetValidTokenReturnsBothChildrenAndFamilyTotal(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Chi Gia Dinh"))
	seedChild(t, db, teacher.ID, contact.ID, "ConMot", date("2026-08-01"), 1)
	seedChild(t, db, teacher.ID, contact.ID, "ConHai", date("2026-08-02"), 2)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 8)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 1)
	token := tokenOf(t, statementsSvc, result.Statements[0])

	router := newPublicRouter(statementsSvc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assertSecurityHeaders(t, rec.Header(), neutralCacheControl)

	var env publicEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.NotNil(t, env.Data)
	require.Equal(t, "Chi Gia Dinh", env.Data.ContactName)
	require.Len(t, env.Data.Children, 2)

	var sum int64
	for _, c := range env.Data.Children {
		sum += c.Subtotal
	}
	require.Equal(t, sum, env.Data.Totals.TotalDue)
	require.EqualValues(t, 300_000, env.Data.Totals.TotalDue)
}

// TestPublicGet404ForEveryInvalidTokenReasonReturnsByteIdenticalBody proves
// the neutral-404 contract: an unknown, malformed, revoked, expired,
// soft-deleted, or already fully-paid token must never be distinguishable
// from one another by response body.
func TestPublicGet404ForEveryInvalidTokenReasonReturnsByteIdenticalBody(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, paymentsSvc, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	router := newPublicRouter(statementsSvc)

	// makeStatement seeds one contact's own period and returns its statement.
	// Every call after the first shares the same teacher across consecutive
	// calendar months, so billing's carried-debt computation (ComputePeriod's
	// PreviousClosedPeriod/CarriedDebtStudents) can roll an earlier month's
	// still-unpaid contact forward into this month's invoice set too —
	// Generate can return more than one statement here. Selecting by
	// contact.ID, rather than assuming index 0, is what keeps each case's
	// token bound to its own contact's own statement.
	makeStatement := func(month int, classStart, name string) (uuid.UUID, string) {
		contact := testutil.Contact(t, db, teacher.ID)
		seedChild(t, db, teacher.ID, contact.ID, name, date(classStart), 1)
		period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, month)
		require.NoError(t, err)
		_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
		require.NoError(t, err)
		result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
		require.NoError(t, err)
		for _, row := range result.Statements {
			if row.ContactID == contact.ID {
				return row.ID, tokenOf(t, statementsSvc, row)
			}
		}
		t.Fatalf("no statement generated for contact %s in period month %d", contact.ID, month)
		return uuid.Nil, ""
	}

	revokedID, revokedToken := makeStatement(1, "2026-01-01", "Revoked")
	require.NoError(t, statementsSvc.Revoke(ctx, testutil.ScopeFor(t, db, teacher.ID), revokedID))

	expiredID, expiredToken := makeStatement(2, "2026-02-01", "Expired")
	require.NoError(t, db.Table("statements").Where("id = ?", expiredID).
		Update("expires_at", time.Now().Add(-24*time.Hour)).Error)

	deletedID, deletedToken := makeStatement(3, "2026-03-01", "Deleted")
	require.NoError(t, db.Exec(`UPDATE statements SET deleted_at = now() WHERE id = ?`, deletedID).Error)

	paidContact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, paidContact.ID, "Paid", date("2026-04-01"), 1)
	paidPeriod, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 4)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), paidPeriod.ID)
	require.NoError(t, err)
	paidResult, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), paidPeriod.ID)
	require.NoError(t, err)
	var paidRow statements.Row
	for _, row := range paidResult.Statements {
		if row.ContactID == paidContact.ID {
			paidRow = row
		}
	}
	require.NotEqual(t, uuid.Nil, paidRow.ID, "expected a statement for the paid contact")
	paidToken := tokenOf(t, statementsSvc, paidRow)
	_, err = paymentsSvc.Record(ctx, testutil.ScopeFor(t, db, teacher.ID), payments.RecordPaymentRequest{
		ContactID: paidContact.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-04-15",
	})
	require.NoError(t, err)

	cases := []struct {
		name  string
		token string
	}{
		{"unknown token", unknownToken()},
		{"malformed token", "not-a-valid-token-format"},
		{"revoked token", revokedToken},
		{"expired token", expiredToken},
		{"soft-deleted token", deletedToken},
		{"fully paid token", paidToken},
	}

	var referenceBody []byte
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/statements/"+tc.token, nil))
			require.Equal(t, http.StatusNotFound, rec.Code)
			assertSecurityHeaders(t, rec.Header(), neutralCacheControl)
			if referenceBody == nil {
				referenceBody = rec.Body.Bytes()
				return
			}
			require.Equal(t, referenceBody, rec.Body.Bytes(),
				"every failure reason must produce a byte-identical 404 body")
		})
	}
}

// TestPublicPostCloseAttendanceCorrectionShowsLiveSessionsAndCarriedAdjustment
// proves the core "why does this not match what I saw last month" flow: a
// billable flip on an already-closed period's session shows up immediately
// in the live session list, and the carried adjustment explains the delta —
// without regenerating the statement.
func TestPublicPostCloseAttendanceCorrectionShowsLiveSessionsAndCarriedAdjustment(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Corrected-student"))
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	session1 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	session2 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-13"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	record1 := testutil.AttendanceRecord(t, db, teacher.ID, session1.ID, student.ID, enrollment.ID)
	testutil.AttendanceRecord(t, db, teacher.ID, session2.ID, student.ID, enrollment.ID)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 200_000, result.Statements[0].TotalDue)
	token := tokenOf(t, statementsSvc, result.Statements[0])

	router := newPublicRouter(statementsSvc)

	before := getPublic(t, router, token)
	require.Len(t, before.Children, 1)
	require.Len(t, before.Children[0].Classes, 1)
	require.Len(t, before.Children[0].Classes[0].Sessions, 2)
	for _, s := range before.Children[0].Classes[0].Sessions {
		require.True(t, s.Counted)
	}
	require.Nil(t, before.Children[0].CarriedAdjustment)

	require.NoError(t, db.Model(&attendance.Record{}).
		Where("id = ?", record1.ID).Update("billable", false).Error)
	_, err = billingSvc.ReconcileSession(ctx, testutil.ScopeFor(t, db, teacher.ID), session1.ID)
	require.NoError(t, err)

	after := getPublic(t, router, token)
	require.Len(t, after.Children[0].Classes[0].Sessions, 2)
	var sawCorrected bool
	for _, s := range after.Children[0].Classes[0].Sessions {
		if s.Date == "2026-01-06" {
			require.False(t, s.Counted, "the corrected session must show as not counted live")
			sawCorrected = true
		}
	}
	require.True(t, sawCorrected, "the corrected session's date must still be present in the live list")

	require.NotNil(t, after.Children[0].CarriedAdjustment, "the correction must surface as a carried adjustment")
	require.EqualValues(t, -100_000, after.Children[0].CarriedAdjustment.Amount)
	require.Contains(t, after.Children[0].CarriedAdjustment.SessionDates, "2026-01-06")
}

// TestPublicViewTrackingCountsOnlyJSONGetNotQRImage proves view tracking
// fires from the JSON route (first_viewed_at set once, view_count
// incremented every open) and never from qr.png alone — a single page load
// fetches both, and must count as one open.
func TestPublicViewTrackingCountsOnlyJSONGetNotQRImage(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Viewed", date("2026-05-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 5)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	row := result.Statements[0]
	token := tokenOf(t, statementsSvc, row)
	router := newPublicRouter(statementsSvc)

	loadView := func() (int, *time.Time) {
		var v struct {
			ViewCount     int
			FirstViewedAt *time.Time
		}
		require.NoError(t, db.Table("statements").Where("id = ?", row.ID).
			Select("view_count, first_viewed_at").Take(&v).Error)
		return v.ViewCount, v.FirstViewedAt
	}

	count, first := loadView()
	require.Zero(t, count)
	require.Nil(t, first)

	imgRec := httptest.NewRecorder()
	router.ServeHTTP(imgRec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token+"/qr.png", nil))
	count, _ = loadView()
	require.Zero(t, count, "qr.png alone must not count as a view")

	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token, nil))
	require.Equal(t, http.StatusOK, firstRec.Code)

	count, first = loadView()
	require.Equal(t, 1, count)
	require.NotNil(t, first)
	firstAt := *first

	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token, nil))
	require.Equal(t, http.StatusOK, secondRec.Code)

	count, first = loadView()
	require.Equal(t, 2, count)
	require.NotNil(t, first)
	require.True(t, first.Equal(firstAt), "first_viewed_at must never move after the first view")
}

// TestPublicTwoFamiliesDataIsolation proves one family's statement never
// carries a byte of another family's names, even though both belong to the
// same teacher and period.
func TestPublicTwoFamiliesDataIsolation(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contactA := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Chi An"))
	seedChild(t, db, teacher.ID, contactA.ID, "AnCon", date("2026-06-01"), 1)
	contactB := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Chi Binh"))
	seedChild(t, db, teacher.ID, contactB.ID, "BinhCon", date("2026-06-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 6)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 2)

	var rowA statements.Row
	for _, row := range result.Statements {
		if row.ContactID == contactA.ID {
			rowA = row
		}
	}
	require.NotEqual(t, uuid.Nil, rowA.ID)
	tokenA := tokenOf(t, statementsSvc, rowA)

	router := newPublicRouter(statementsSvc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/statements/"+tokenA, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "Chi An")
	require.Contains(t, body, "AnCon-student")
	require.NotContains(t, body, "Chi Binh")
	require.NotContains(t, body, "BinhCon")
}

// TestPublicAdjustmentReasonNeverAppearsInResponseBody proves the teacher's
// free-text adjustment reason is excluded at the projection level: amount
// and kind travel to the public payload, the reason never does.
func TestPublicAdjustmentReasonNeverAppearsInResponseBody(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Adjusted", date("2026-07-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 7)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	var invoiceRow struct{ ID uuid.UUID }
	require.NoError(t, db.Table("invoices").Select("id").
		Where("teacher_id = ? AND contact_id = ? AND period_id = ?", teacher.ID, contact.ID, period.ID).
		Take(&invoiceRow).Error)

	const secretReason = "REASON_MUST_NEVER_LEAK_TO_PARENT"
	_, _, err = billingSvc.AddAdjustment(ctx, testutil.ScopeFor(t, db, teacher.ID), invoiceRow.ID, -15_000, secretReason)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	token := tokenOf(t, statementsSvc, result.Statements[0])

	router := newPublicRouter(statementsSvc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), secretReason)

	var env publicEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.NotNil(t, env.Data)
	require.Len(t, env.Data.Children, 1)
	require.Len(t, env.Data.Children[0].Adjustments, 1)
	require.EqualValues(t, -15_000, env.Data.Children[0].Adjustments[0].Amount)
	require.Equal(t, "manual", env.Data.Children[0].Adjustments[0].Kind)
}

// TestPublicPaymentsByInvoiceMatchesD8UnderpaymentSplit proves the
// payments.by_invoice breakdown reflects D8's real allocation: the
// earlier-starting class's invoice is paid off first.
func TestPublicPaymentsByInvoiceMatchesD8UnderpaymentSplit(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, paymentsSvc, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contact.ID, "Earlier", date("2026-09-01"), 1)
	seedChild(t, db, teacher.ID, contact.ID, "Later", date("2026-09-10"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 9)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 1)
	require.EqualValues(t, 200_000, result.Statements[0].TotalDue)
	token := tokenOf(t, statementsSvc, result.Statements[0])

	_, err = paymentsSvc.Record(ctx, testutil.ScopeFor(t, db, teacher.ID), payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 150_000, Method: payments.MethodCash, ReceivedOn: "2026-09-15",
	})
	require.NoError(t, err)

	router := newPublicRouter(statementsSvc)
	env := getPublic(t, router, token)
	require.EqualValues(t, 150_000, env.Payments.TotalPaid)
	require.Len(t, env.Payments.ByInvoice, 2)

	byStudent := make(map[string]statements.PublicInvoicePayment, 2)
	for _, p := range env.Payments.ByInvoice {
		byStudent[p.StudentName] = p
	}
	earlier := byStudent["Earlier-student"]
	later := byStudent["Later-student"]
	require.EqualValues(t, 100_000, earlier.Paid, "the earlier-starting class settles first under D8")
	require.EqualValues(t, 0, earlier.Outstanding)
	require.EqualValues(t, 50_000, later.Paid, "the later-starting class carries the shortfall")
	require.EqualValues(t, 50_000, later.Outstanding)
}

// TestPublicQRImageServesWithBankConfigAndReturnsNeutral404WithoutIt proves
// the qr.png route end to end: a decodable PNG with the right headers when
// the teacher's bank is configured, and the same neutral 404 as an invalid
// token when it is not — never an empty or placeholder image.
func TestPublicQRImageServesWithBankConfigAndReturnsNeutral404WithoutIt(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("Chi QR"))
	seedChild(t, db, teacher.ID, contact.ID, "QRChild", date("2026-10-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 10)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	token := tokenOf(t, statementsSvc, result.Statements[0])

	router := newPublicRouter(statementsSvc)

	env := getPublic(t, router, token)
	require.NotNil(t, env.QR)
	require.Contains(t, env.QR.ImageURL, "/public/statements/"+token+"/qr.png")

	imgRec := httptest.NewRecorder()
	router.ServeHTTP(imgRec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token+"/qr.png", nil))
	require.Equal(t, http.StatusOK, imgRec.Code)
	assertSecurityHeaders(t, imgRec.Header(), qrCacheControl)
	require.Equal(t, "image/png", imgRec.Header().Get("Content-Type"))
	_, err = png.Decode(bytes.NewReader(imgRec.Body.Bytes()))
	require.NoError(t, err, "qr.png body must decode as a real PNG")

	// Absent bank config: qr is null in the JSON, and the image route
	// returns the same neutral 404 as an invalid token — reusing the same
	// repository/DB so the token stays valid, only the bank config differs.
	noBankSvc := statements.NewService(statements.NewRepository(db), database.NewTxManager(db),
		config.StatementsConfig{TokenKey: testTokenKey, PublicBaseURL: "https://parent.example.com"},
		statements.BankConfig{}, statements.NewQRBuilder())
	noBankRouter := newPublicRouter(noBankSvc)

	noBankEnv := getPublic(t, noBankRouter, token)
	require.Nil(t, noBankEnv.QR)

	noBankImgRec := httptest.NewRecorder()
	noBankRouter.ServeHTTP(noBankImgRec, httptest.NewRequest(http.MethodGet, "/public/statements/"+token+"/qr.png", nil))
	require.Equal(t, http.StatusNotFound, noBankImgRec.Code)
}

// sqlCounter is a minimal gorm logger.Interface that counts every statement
// traced — the same pattern sessions/integration_test.go uses to prove
// ListPending stays a bounded, fixed number of round trips.
type sqlCounter struct {
	mu    sync.Mutex
	count int
}

func (c *sqlCounter) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *sqlCounter) Info(context.Context, string, ...interface{})     {}
func (c *sqlCounter) Warn(context.Context, string, ...interface{})     {}
func (c *sqlCounter) Error(context.Context, string, ...interface{})    {}
func (c *sqlCounter) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

// TestPublicRenderIssuesTheSameQueryCountRegardlessOfFamilySize proves
// RenderPublic never grows a per-child or per-session query: a one-child and
// a three-child family issue the same fixed number of round trips
// (InvoicesWithLines + LiveSessions + Adjustments, nothing more).
func TestPublicRenderIssuesTheSameQueryCountRegardlessOfFamilySize(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, _, db := newPublicIntegrationDeps(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	contactOne := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contactOne.ID, "Solo", date("2026-11-01"), 1)

	contactThree := testutil.Contact(t, db, teacher.ID)
	seedChild(t, db, teacher.ID, contactThree.ID, "Trio1", date("2026-11-01"), 1)
	seedChild(t, db, teacher.ID, contactThree.ID, "Trio2", date("2026-11-02"), 1)
	seedChild(t, db, teacher.ID, contactThree.ID, "Trio3", date("2026-11-03"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, testutil.ScopeFor(t, db, teacher.ID), 2026, 11)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, testutil.ScopeFor(t, db, teacher.ID), period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 2)

	var stmtOne, stmtThree *statements.Statement
	for i := range result.Statements {
		row := result.Statements[i]
		s := row.Statement
		if row.ContactID == contactOne.ID {
			stmtOne = &s
		} else {
			stmtThree = &s
		}
	}
	require.NotNil(t, stmtOne)
	require.NotNil(t, stmtThree)

	cfg := config.StatementsConfig{TokenKey: testTokenKey, PublicBaseURL: "https://parent.example.com"}

	counterOne := &sqlCounter{}
	svcOne := statements.NewService(statements.NewRepository(db.Session(&gorm.Session{Logger: counterOne})),
		database.NewTxManager(db), cfg, testBankConfig, statements.NewQRBuilder())
	_, _, err = svcOne.RenderPublic(ctx, stmtOne)
	require.NoError(t, err)

	counterThree := &sqlCounter{}
	svcThree := statements.NewService(statements.NewRepository(db.Session(&gorm.Session{Logger: counterThree})),
		database.NewTxManager(db), cfg, testBankConfig, statements.NewQRBuilder())
	_, _, err = svcThree.RenderPublic(ctx, stmtThree)
	require.NoError(t, err)

	require.Equal(t, counterOne.count, counterThree.count,
		"RenderPublic must issue the same fixed number of queries regardless of family size")
	require.Equal(t, 3, counterOne.count,
		"RenderPublic must be exactly InvoicesWithLines + LiveSessions + Adjustments, never a per-child query")
}
