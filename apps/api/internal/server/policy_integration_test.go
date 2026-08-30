//go:build integration

package server

import (
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

// Wave-1 parity in both directions: a single classes.view_all grant widens
// classes and only classes (students stay narrow), while the legacy
// data.view_center_wide alias still widens every resource through set-build
// expansion — the backfilled legacy holder loses nothing. Students is the
// second probe rather than contacts because contact reads run on the frozen
// ReportsOversight axis (phone privacy), not on a view_all key.
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

	// The legacy alias still widens everything via expansion.
	e.override(t, memberB.ID, owner.CenterID, authctx.PermDataViewCenterWide, true)
	body = e.get(t, "/api/v1/classes", tokB).Body.String()
	require.Contains(t, body, ownerClass.ID.String())
	body = e.get(t, "/api/v1/students", tokB).Body.String()
	require.Contains(t, body, ownerStudent.ID.String(),
		"a legacy center-wide holder must keep full visibility")
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
