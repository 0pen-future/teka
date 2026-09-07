//go:build integration

package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/testutil"
)

// policyEnv is a full production router bound to a real database — the
// authorization matrix runs through the exact chain a request travels:
// RequireAuth, ResolveScope (fresh from the DB), then the route-policy
// enforcer, and finally the feature's own scoped repository.
type policyEnv struct {
	db     *gorm.DB
	router http.Handler
	issuer *auth.TokenIssuer
}

func newPolicyEnv(t *testing.T) *policyEnv {
	t.Helper()
	db := testutil.StartPostgres(t)
	cfg := &config.Config{
		Env:         config.EnvTest,
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost:5173"},
		JWT:         config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour},
		Database:    config.DatabaseConfig{ConnMaxLifetime: time.Minute},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	zaloSvc := newTestZaloService(t)
	statementsSvc := statements.NewService(statements.NewRepository(db), database.NewTxManager(db),
		cfg.Statements, statements.BankConfig{}, statements.NewQRBuilder())
	notificationsSvc := notifications.NewService(notifications.NewRepository(db), database.NewTxManager(db),
		statementsSvc, zaloSvc, log, cfg.Notifications)
	t.Cleanup(notificationsSvc.Close)

	txMgr := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, events.NewSync())
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(cfg.JWT), txMgr,
		centersSvc, zaloSvc, cfg.Onboarding, cfg.Statements.PublicBaseURL, events.NewSync())
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)

	return &policyEnv{
		db:     db,
		router: NewRouter(cfg, log, db, zaloSvc, statementsSvc, notificationsSvc, teachersSvc, centersSvc, authSvc, events.NewSync()),
		issuer: auth.NewTokenIssuer(cfg.JWT),
	}
}

func (e *policyEnv) token(t *testing.T, accountID uuid.UUID) string {
	t.Helper()
	tok, err := e.issuer.IssueAccess(accountID, authctx.RoleTeacher)
	require.NoError(t, err)
	return tok
}

func (e *policyEnv) get(t *testing.T, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// send issues a JSON request with a body — the write-side counterpart of get.
func (e *policyEnv) send(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// giaoVien puts the member on the center's giao_vien system role, which the
// fixtures seed with the operational baseline (DefaultRoleKeys) — the same
// born-with-defaults invariant production centers carry.
func (e *policyEnv) giaoVien(t *testing.T, teacherID, centerID uuid.UUID) uuid.UUID {
	t.Helper()
	var row struct{ ID uuid.UUID }
	require.NoError(t, e.db.Raw(
		"SELECT id FROM center_roles WHERE center_id = ? AND key = 'giao_vien'", centerID).Scan(&row).Error)
	require.NotEqual(t, uuid.Nil, row.ID)
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET role_id = ? WHERE teacher_id = ? AND center_id = ? AND left_at IS NULL",
		row.ID, teacherID, centerID).Error)
	return row.ID
}

func (e *policyEnv) override(t *testing.T, teacherID, centerID uuid.UUID, key string, allowed bool) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (teacher_id, center_id, permission_key) DO UPDATE SET allowed = EXCLUDED.allowed`,
		teacherID, centerID, key, allowed).Error)
}

// The owner passes both a permission route and an owner-only route; a
// baseline member passes the permission route, is stopped at owner-only
// configuration, and never held a legacy identity key like audit.read.
// Broken authentication never reaches the policy layer at all.
func TestPolicyHTTPOwnerAndBaselineMember(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	ownerAcct, owner := testutil.Teacher(t, e.db)
	memberAcct, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	e.giaoVien(t, member.ID, owner.CenterID)

	ownerTok := e.token(t, ownerAcct.ID)
	memberTok := e.token(t, memberAcct.ID)

	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/classes", ownerTok).Code)
	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/score-sets", ownerTok).Code)
	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/audit-logs", ownerTok).Code)

	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/classes", memberTok).Code)
	require.Equal(t, http.StatusForbidden, e.get(t, "/api/v1/score-sets", memberTok).Code,
		"score-set configuration is owner-only")
	require.Equal(t, http.StatusForbidden, e.get(t, "/api/v1/audit-logs", memberTok).Code,
		"audit.read is a legacy identity key and stays out of the baseline")

	require.Equal(t, http.StatusUnauthorized, e.get(t, "/api/v1/classes", "").Code)
	require.Equal(t, http.StatusUnauthorized, e.get(t, "/api/v1/classes", "garbage").Code)

	expired := auth.NewTokenIssuer(config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: -time.Minute})
	expiredTok, err := expired.IssueAccess(memberAcct.ID, authctx.RoleTeacher)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, e.get(t, "/api/v1/classes", expiredTok).Code)
}

// A deny on classes.list stops the collection route while classes.read keeps
// the item route working — list and read are separate grants, and a deny
// narrows exactly one of them.
func TestPolicyHTTPDenyListKeepsRead(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	ownerAcct, owner := testutil.Teacher(t, e.db)
	memberAcct, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	e.giaoVien(t, member.ID, owner.CenterID)
	class := testutil.Class(t, e.db, member.ID)
	_ = ownerAcct

	tok := e.token(t, memberAcct.ID)
	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/classes", tok).Code)

	e.override(t, member.ID, owner.CenterID, authctx.PermClassesList, false)
	require.Equal(t, http.StatusForbidden, e.get(t, "/api/v1/classes", tok).Code,
		"deny must beat the role grant")
	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/classes/"+class.ID.String(), tok).Code,
		"denying list must not touch read")
}

// Replacing the role's permission set applies on the very next request with
// the same still-valid token — scope is resolved fresh from the database, not
// carried in claims.
func TestPolicyHTTPRoleChangeAppliesImmediately(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	memberAcct, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	roleID := e.giaoVien(t, member.ID, owner.CenterID)

	tok := e.token(t, memberAcct.ID)
	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/classes", tok).Code)

	require.NoError(t, e.db.Exec(
		"DELETE FROM center_role_permissions WHERE role_id = ? AND permission_key = ?",
		roleID, authctx.PermClassesList).Error)
	require.Equal(t, http.StatusForbidden, e.get(t, "/api/v1/classes", tok).Code)
}

// A closed membership stint cuts access on the very next request even though
// the token is still cryptographically valid.
func TestPolicyHTTPRemovedMembershipLosesAccess(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	memberAcct, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	e.giaoVien(t, member.ID, owner.CenterID)

	tok := e.token(t, memberAcct.ID)
	require.Equal(t, http.StatusOK, e.get(t, "/api/v1/classes", tok).Code)

	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND left_at IS NULL",
		member.ID).Error)
	rec := e.get(t, "/api/v1/classes", tok)
	require.GreaterOrEqual(t, rec.Code, 400,
		"a removed member must lose access on the next request, got %d", rec.Code)
}

// Per-resource parity in both directions: a single classes.view_all grant
// widens classes and only classes (students stay narrow), while a stray row
// for the retired data.view_center_wide key — a code rollback re-writing one
// after migration 000020 — widens nothing at all. Students is the second
// probe; contacts.view_all now widens contact reads the same way (see
// TestReportsSendImpliedKeysParity), it is simply not this test's concern.
func TestPolicyHTTPViewAllParity(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	ownerAcct, owner := testutil.Teacher(t, e.db)
	memberAAcct, memberA := testutil.Teacher(t, e.db)
	memberBAcct, memberB := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, memberA.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, memberB.ID, owner.CenterID)
	e.giaoVien(t, memberA.ID, owner.CenterID)
	e.giaoVien(t, memberB.ID, owner.CenterID)
	_ = ownerAcct

	ownerContact := testutil.Contact(t, e.db, owner.ID)
	ownerClass := testutil.Class(t, e.db, owner.ID)
	ownerStudent := testutil.Student(t, e.db, owner.ID, ownerContact.ID)
	aContact := testutil.Contact(t, e.db, memberA.ID)
	aClass := testutil.Class(t, e.db, memberA.ID)
	aStudent := testutil.Student(t, e.db, memberA.ID, aContact.ID)

	tokA := e.token(t, memberAAcct.ID)
	tokB := e.token(t, memberBAcct.ID)

	// Baseline: own rows only, on both resources.
	body := e.get(t, "/api/v1/classes", tokA).Body.String()
	require.Contains(t, body, aClass.ID.String())
	require.NotContains(t, body, ownerClass.ID.String())
	body = e.get(t, "/api/v1/students", tokA).Body.String()
	require.Contains(t, body, aStudent.ID.String())
	require.NotContains(t, body, ownerStudent.ID.String())

	// classes.view_all widens classes — and nothing else.
	e.override(t, memberA.ID, owner.CenterID, authctx.PermClassesViewAll, true)
	body = e.get(t, "/api/v1/classes", tokA).Body.String()
	require.Contains(t, body, ownerClass.ID.String(),
		"classes.view_all must widen the classes list to the whole center")
	body = e.get(t, "/api/v1/students", tokA).Body.String()
	require.NotContains(t, body, ownerStudent.ID.String(),
		"classes.view_all must not leak into students")

	// A planted retired-key row drops out as unknown and widens nothing.
	e.override(t, memberB.ID, owner.CenterID, "data.view_center_wide", true)
	body = e.get(t, "/api/v1/classes", tokB).Body.String()
	require.NotContains(t, body, ownerClass.ID.String(),
		"the retired center-wide key must not widen classes")
	body = e.get(t, "/api/v1/students", tokB).Body.String()
	require.NotContains(t, body, ownerStudent.ID.String(),
		"the retired center-wide key must not widen students")
}

// A <resource>.view_all key widens what a member can SEE and nothing they can
// change: with the key the owner's session and class read fine, yet cancelling
// the session is refused for lack of a class role and editing the class is
// not even found on the write scope. Recording a payment for the owner's
// contact needs no key at all — the route permission admits the collector
// and the payment anchors on the contact's own teacher.
func TestPolicyHTTPViewAllNeverWidensWrites(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	memberAcct, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	e.giaoVien(t, member.ID, owner.CenterID)
	tok := e.token(t, memberAcct.ID)

	ownerContact := testutil.Contact(t, e.db, owner.ID)
	ownerClass := testutil.Class(t, e.db, owner.ID)
	ownerSession := testutil.Session(t, e.db, owner.ID, ownerClass.ID, time.Now().AddDate(0, 0, 7))
	sessionPath := "/api/v1/sessions/" + ownerSession.ID.String()
	classPath := "/api/v1/classes/" + ownerClass.ID.String()

	// Without keys the owner's rows are invisible on read and write alike.
	require.Equal(t, http.StatusNotFound, e.get(t, sessionPath, tok).Code)
	require.Equal(t, http.StatusNotFound, e.send(t, http.MethodPost, sessionPath+"/cancel", tok, `{"reason":"thử"}`).Code)

	e.override(t, member.ID, owner.CenterID, authctx.PermSessionsViewAll, true)
	e.override(t, member.ID, owner.CenterID, authctx.PermClassesViewAll, true)

	require.Equal(t, http.StatusOK, e.get(t, sessionPath, tok).Code,
		"sessions.view_all must open the owner's session for reading")
	require.Equal(t, http.StatusOK, e.get(t, classPath, tok).Code,
		"classes.view_all must open the owner's class for reading")

	rec := e.send(t, http.MethodPost, sessionPath+"/cancel", tok, `{"reason":"thử huỷ"}`)
	require.Equal(t, http.StatusForbidden, rec.Code, "a visibility key must not widen cancelling a session")
	require.Contains(t, rec.Body.String(), "your role on this class",
		"the refusal must come from the class write gate, not the route policy")
	var status string
	require.NoError(t, e.db.Raw("SELECT status FROM class_sessions WHERE id = ?", ownerSession.ID).Scan(&status).Error)
	require.Equal(t, "planned", status, "the owner's session must be untouched")

	rec = e.send(t, http.MethodPut, classPath, tok,
		`{"name":"Lớp Sửa Trộm","start_date":"2026-01-05","default_unit_price":1}`)
	require.Equal(t, http.StatusNotFound, rec.Code, "a visibility key must not widen editing a class")
	var name string
	require.NoError(t, e.db.Raw("SELECT name FROM classes WHERE id = ?", ownerClass.ID).Scan(&name).Error)
	require.Equal(t, ownerClass.Name, name, "the owner's class must be untouched")

	// payments.create alone lets the member collect for the owner's contact;
	// the row belongs to the contact's teacher, not the collector.
	rec = e.send(t, http.MethodPost, "/api/v1/payments", tok,
		`{"contact_id":"`+ownerContact.ID.String()+`","amount":50000,"method":"cash","received_on":"2026-03-01"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var payment struct{ TeacherID uuid.UUID }
	require.NoError(t, e.db.Raw("SELECT teacher_id FROM payments WHERE contact_id = ?", ownerContact.ID).Scan(&payment).Error)
	require.Equal(t, owner.ID, payment.TeacherID, "the payment must anchor on the contact's own teacher")

	// Without payments.create the route policy stops the same request.
	e.override(t, member.ID, owner.CenterID, authctx.PermPaymentsCreate, false)
	rec = e.send(t, http.MethodPost, "/api/v1/payments", tok,
		`{"contact_id":"`+ownerContact.ID.String()+`","amount":50000,"method":"cash","received_on":"2026-03-02"}`)
	require.Equal(t, http.StatusForbidden, rec.Code, "recording a payment is gated by payments.create")
}

// A reports.send holder and a holder of exactly the four keys it implies
// (billing/statements/notifications/contacts view_all, granted directly, no
// reports.send) see the identical center-wide read surface — contacts with
// phone, the billing period, statements, and the notification ledger — while
// a baseline member sees none of it. The two diverge only on send-only
// surfaces: bulk-sending, previewing, resuming a run, the statement's public
// link, and mapping a contact's Zalo friend all stay reports.send-only, and
// marking another teacher's notification sent stays refused for both, since
// neither the source permission nor its implied keys ever widen a write.
func TestReportsSendImpliedKeysParity(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	ownerAcct, owner := testutil.Teacher(t, e.db)
	memberAAcct, memberA := testutil.Teacher(t, e.db)
	memberBAcct, memberB := testutil.Teacher(t, e.db)
	memberCAcct, memberC := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, memberA.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, memberB.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, memberC.ID, owner.CenterID)
	e.giaoVien(t, memberA.ID, owner.CenterID)
	e.giaoVien(t, memberB.ID, owner.CenterID)
	e.giaoVien(t, memberC.ID, owner.CenterID)

	e.override(t, memberA.ID, owner.CenterID, authctx.PermReportsSend, true)
	e.override(t, memberB.ID, owner.CenterID, authctx.PermBillingViewAll, true)
	e.override(t, memberB.ID, owner.CenterID, authctx.PermStatementsViewAll, true)
	e.override(t, memberB.ID, owner.CenterID, authctx.PermNotificationsViewAll, true)
	e.override(t, memberB.ID, owner.CenterID, authctx.PermContactsViewAll, true)

	tokOwner := e.token(t, ownerAcct.ID)
	tokA := e.token(t, memberAAcct.ID)
	tokB := e.token(t, memberBAcct.ID)
	tokC := e.token(t, memberCAcct.ID)

	// Fixture data anchored on the owner: a contact with a billable, attended
	// child, closed into an invoice, then a generated statement and a queued
	// notification.
	ownerContact := testutil.Contact(t, e.db, owner.ID)
	classStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	class := testutil.Class(t, e.db, owner.ID, testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, e.db, owner.ID, ownerContact.ID)
	enrollment := testutil.Enrollment(t, e.db, owner.ID, student.ID, class.ID, classStart)
	session := testutil.Session(t, e.db, owner.ID, class.ID, classStart.AddDate(0, 0, 1),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, e.db, owner.ID, session.ID, student.ID, enrollment.ID)

	rec := e.send(t, http.MethodPost, "/api/v1/billing-periods", tokOwner, `{"year":2026,"month":3}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var period struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &period))
	periodID := period.Data.ID
	periodPath := "/api/v1/billing-periods/" + periodID.String()

	require.Equal(t, http.StatusOK, e.send(t, http.MethodPost, periodPath+"/close", tokOwner, "").Code)
	require.Equal(t, http.StatusOK, e.send(t, http.MethodPost, periodPath+"/statements/generate", tokOwner, "").Code)
	rec = e.send(t, http.MethodPost, periodPath+"/notifications/bulk", tokOwner, `{"purpose":"statement","channel":"zalo_manual"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Read parity: A and B see the whole center on every converted resource;
	// C, holding neither reports.send nor an implied key, sees none of it.
	contactPath := "/api/v1/contacts/" + ownerContact.ID.String()
	for _, tok := range []string{tokA, tokB} {
		body := e.get(t, contactPath, tok).Body.String()
		require.Contains(t, body, ownerContact.Phone, "contacts.view_all (direct or implied) must widen contact reads")
	}
	require.Equal(t, http.StatusNotFound, e.get(t, contactPath, tokC).Code,
		"a baseline member must not see another teacher's contact")

	for _, tok := range []string{tokA, tokB} {
		require.Equal(t, http.StatusOK, e.get(t, periodPath, tok).Code,
			"billing.view_all (direct or implied) must widen the period read")
	}
	require.Equal(t, http.StatusNotFound, e.get(t, periodPath, tokC).Code)

	stmtBodyA := e.get(t, periodPath+"/statements", tokA).Body.String()
	stmtBodyB := e.get(t, periodPath+"/statements", tokB).Body.String()
	require.Contains(t, stmtBodyA, `"total_due"`, "statements.view_all (direct or implied) must widen the statement list")
	require.Contains(t, stmtBodyB, `"total_due"`, "statements.view_all (direct or implied) must widen the statement list")
	require.Equal(t, http.StatusNotFound, e.get(t, periodPath+"/statements", tokC).Code)

	notifBodyA := e.get(t, periodPath+"/notifications", tokA).Body.String()
	notifBodyB := e.get(t, periodPath+"/notifications", tokB).Body.String()
	notifBodyC := e.get(t, periodPath+"/notifications", tokC).Body.String()
	require.Contains(t, notifBodyA, ownerContact.ID.String(), "notifications.view_all (direct or implied) must widen the notification ledger")
	require.Contains(t, notifBodyB, ownerContact.ID.String(), "notifications.view_all (direct or implied) must widen the notification ledger")
	require.NotContains(t, notifBodyC, ownerContact.ID.String(), "a baseline member must not see another teacher's notification ledger")

	// Send-only surfaces stay reports.send-only: B's implied keys read but
	// never send.
	require.Equal(t, http.StatusForbidden,
		e.send(t, http.MethodPost, periodPath+"/notifications/bulk", tokB, `{"purpose":"statement","channel":"zalo_manual"}`).Code,
		"the implied keys must not widen sending")
	require.Equal(t, http.StatusOK,
		e.send(t, http.MethodPost, periodPath+"/notifications/bulk", tokA, `{"purpose":"statement","channel":"zalo_manual"}`).Code,
		"reports.send itself still sends")

	require.Equal(t, http.StatusForbidden, e.get(t, periodPath+"/notifications/preview", tokB).Code,
		"the implied notifications.view_all must not widen the send preview")

	// resumeRun checks send-oversight before it ever looks for a run: B is
	// refused before the lookup, A passes the gate and then gets "no run" —
	// distinct failures that both refuse the actual resume.
	recA := e.send(t, http.MethodPost, periodPath+"/notifications/run/resume", tokA, "")
	require.Equal(t, http.StatusNotFound, recA.Code, "reports.send passes the oversight gate onto a real (absent) run lookup")
	recB := e.send(t, http.MethodPost, periodPath+"/notifications/run/resume", tokB, "")
	require.Equal(t, http.StatusForbidden, recB.Code, "the implied keys must not pass the send-oversight gate")

	// The statement's public link is a send-only field: populated for A,
	// absent for B, on the very same generated statement.
	require.Contains(t, stmtBodyA, `"url":"`, "reports.send must expose the statement's public link")
	require.NotContains(t, stmtBodyB, `"url":"`, "the implied statements.view_all must not expose the send-only public link")

	// Mapping a contact to a Zalo friend is send-only: A can, B (view_all
	// only) and C (baseline) cannot.
	mapBody := `{"zalo_user_id":"zalo-friend-1","zalo_name":"Bạn Zalo"}`
	require.Equal(t, http.StatusOK, e.send(t, http.MethodPut, contactPath+"/zalo-mapping", tokA, mapBody).Code)
	require.Equal(t, http.StatusNotFound,
		e.send(t, http.MethodPut, contactPath+"/zalo-mapping", tokB, mapBody).Code,
		"contacts.view_all must not widen the zalo-mapping write")
	require.Equal(t, http.StatusNotFound,
		e.send(t, http.MethodPut, contactPath+"/zalo-mapping", tokC, mapBody).Code)

	// Marking another teacher's notification sent stays refused for both A
	// and B: reports.send only gates creating what a family receives, and the
	// implied view_all keys are read-only by construction.
	var notifIDs struct {
		Data []struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(notifBodyA), &notifIDs))
	require.NotEmpty(t, notifIDs.Data, "the owner's bulk send must have queued at least one notification")
	markSentBody := `{"ids":["` + notifIDs.Data[0].ID.String() + `"]}`
	require.Equal(t, http.StatusNotFound, e.send(t, http.MethodPost, "/api/v1/notifications/mark-sent", tokA, markSentBody).Code,
		"reports.send must not widen marking another teacher's notification sent")
	require.Equal(t, http.StatusNotFound, e.send(t, http.MethodPost, "/api/v1/notifications/mark-sent", tokB, markSentBody).Code,
		"the implied notifications.view_all must not widen marking another teacher's notification sent")
}

// An unauthenticated probe of a policy-guarded route must never leak whether
// the object exists: the envelope is the standard error shape with no data.
func TestPolicyHTTPUnauthenticatedEnvelope(t *testing.T) {
	t.Parallel()
	e := newPolicyEnv(t)
	rec := e.get(t, "/api/v1/classes/"+uuid.New().String(), "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.True(t, strings.Contains(rec.Body.String(), "UNAUTHORIZED") || strings.Contains(rec.Body.String(), "unauthorized"),
		"unexpected envelope: %s", rec.Body.String())
}
