//go:build integration

package centers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// noopDMSender satisfies auth.ResetDMSender for tests in this package that
// never exercise the forgot-password DM path — every call fails closed with
// ok=false so ForgotPassword's generic no-op branch is taken.
type noopDMSender struct{}

func (noopDMSender) LookupPhone(_ context.Context, _ uuid.UUID, _ string) (string, bool, error) {
	return "", false, nil
}

func (noopDMSender) SendDM(_ context.Context, _ uuid.UUID, _, _ string) (string, error) {
	return "", nil
}

type env struct {
	db          *gorm.DB
	tx          database.TxManager
	teachersSvc *teachers.Service
	centersSvc  *centers.Service
	authSvc     *auth.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	onboardingCfg := config.OnboardingConfig{ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(jwtCfg), txMgr,
		centersSvc, noopDMSender{}, onboardingCfg, "https://app.example.com", nil)
	// centersSvc consumes authSvc to offboard a removed member (disable +
	// revoke refresh tokens); teachersSvc consumes it to invalidate old
	// sessions on reactivate — the same wiring router.go does in production.
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)
	return &env{db: db, tx: txMgr, teachersSvc: teachersSvc, centersSvc: centersSvc, authSvc: authSvc}
}

// scope resolves the caller's scope the way the middleware would, straight
// from the database.
func (e *env) scope(t *testing.T, teacherID uuid.UUID) authctx.Scope {
	t.Helper()
	s, err := e.centersSvc.ResolveScope(context.Background(), teacherID)
	require.NoError(t, err)
	return s
}

// join is the happy-path shorthand: teacher moves into the center owned by
// ownerID, via the same direct-insert path other fixtures use — the real
// production path (an invitation's accept) is exercised in the invitations
// package; centers tests only need a live membership to already exist.
func (e *env) join(t *testing.T, teacherID, ownerID uuid.UUID) {
	t.Helper()
	testutil.JoinCenter(t, e.db, teacherID, e.scope(t, ownerID).CenterID)
}

func (e *env) liveMembership(t *testing.T, teacherID uuid.UUID) centers.Member {
	t.Helper()
	var m centers.Member
	require.NoError(t, e.db.First(&m, "teacher_id = ? AND left_at IS NULL", teacherID).Error)
	return m
}

// insertContact writes a contacts row with an explicit center_id; the
// contacts model gains its CenterID field in the re-key sweep, so raw SQL is
// the honest way to plant center-scoped data here.
func insertContact(t *testing.T, db *gorm.DB, teacherID, centerID uuid.UUID) uuid.UUID {
	t.Helper()
	rowID := id.New()
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, teacher_id, center_id, full_name, phone) VALUES (?, ?, ?, 'Chị Hoa', ?)`,
		rowID, teacherID, centerID, "+84900000001").Error)
	return rowID
}

func insertStudent(t *testing.T, db *gorm.DB, teacherID, centerID, contactID uuid.UUID) uuid.UUID {
	t.Helper()
	rowID := id.New()
	require.NoError(t, db.Exec(
		`INSERT INTO students (id, teacher_id, center_id, contact_id, full_name) VALUES (?, ?, ?, ?, 'Bé An')`,
		rowID, teacherID, centerID, contactID).Error)
	return rowID
}

func insertClass(t *testing.T, db *gorm.DB, teacherID, centerID uuid.UUID) uuid.UUID {
	t.Helper()
	rowID := id.New()
	require.NoError(t, db.Exec(
		`INSERT INTO classes (id, teacher_id, center_id, name, start_date, default_unit_price)
		 VALUES (?, ?, ?, 'Lớp Toán 9', '2026-01-05', 100000)`,
		rowID, teacherID, centerID).Error)
	return rowID
}

func TestResolveScopeRejectsDeadAccountsAndDeadCenters(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	acct, teacher := testutil.Teacher(t, e.db)
	e.scope(t, acct.ID) // sanity: resolves while alive

	require.NoError(t, e.db.Model(&teachers.Account{ID: acct.ID}).Update("status", teachers.StatusDisabled).Error)
	_, err := e.centersSvc.ResolveScope(ctx, acct.ID)
	require.Equal(t, apperror.CodeUnauthorized, apperror.From(err).Code, "disabled account must not resolve a scope")

	require.NoError(t, e.db.Model(&teachers.Account{ID: acct.ID}).Update("status", teachers.StatusActive).Error)
	require.NoError(t, e.db.Delete(&teachers.Account{ID: acct.ID}).Error)
	_, err = e.centersSvc.ResolveScope(ctx, acct.ID)
	require.Equal(t, apperror.CodeUnauthorized, apperror.From(err).Code, "soft-deleted account must not resolve a scope")

	// A soft-deleted center is fenced at scope resolution, not just in reads.
	acct2, teacher2 := testutil.Teacher(t, e.db)
	require.NoError(t, e.db.Exec("UPDATE centers SET deleted_at = now() WHERE id = ?", teacher2.CenterID).Error)
	_, err = e.centersSvc.ResolveScope(ctx, acct2.ID)
	require.Equal(t, apperror.CodeUnauthorized, apperror.From(err).Code, "soft-deleted center must not resolve a scope")
	_ = teacher
}

func TestMeShowsCenterAndMembers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ Trung Tâm"))
	member, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên B"))
	e.join(t, member.ID, owner.ID)

	// A plain member sees only the center's name — the roster is owner-only
	// data.
	memberMe, err := e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	memberResp, ok := memberMe.(*centers.MemberMeResponse)
	require.True(t, ok, "a non-owner must get the member shape")
	require.Equal(t, "Chủ Trung Tâm", memberResp.CenterName)

	ownerMe, err := e.centersSvc.Me(ctx, e.scope(t, owner.ID))
	require.NoError(t, err)
	me, ok := ownerMe.(*centers.MeResponse)
	require.True(t, ok, "an owner must get the full roster shape")
	require.Equal(t, ownerTeacher.CenterID, me.Center.ID)
	require.True(t, me.Center.IsOwner)
	require.Len(t, me.Members, 2)

	byID := map[uuid.UUID]centers.MemberResponse{}
	for _, m := range me.Members {
		byID[m.ID] = m
	}
	require.True(t, byID[owner.ID].IsOwner)
	require.False(t, byID[member.ID].IsOwner)
	require.Equal(t, owner.Phone, byID[owner.ID].Phone)
	require.Equal(t, "Chủ Trung Tâm", byID[owner.ID].FullName)

	// A disabled account only ever exists because RemoveMember offboarded it,
	// so it must drop off the roster exactly like a soft-deleted account —
	// same as ListMembers' active-only filter.
	disabledMember, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên Bị Khoá"))
	deletedMember, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên Đã Xoá"))
	e.join(t, disabledMember.ID, owner.ID)
	e.join(t, deletedMember.ID, owner.ID)
	require.NoError(t, e.db.Model(&teachers.Account{ID: disabledMember.ID}).Update("status", teachers.StatusDisabled).Error)
	require.NoError(t, e.db.Delete(&teachers.Account{ID: deletedMember.ID}).Error)

	ownerMe, err = e.centersSvc.Me(ctx, e.scope(t, owner.ID))
	require.NoError(t, err)
	me, ok = ownerMe.(*centers.MeResponse)
	require.True(t, ok)
	require.Len(t, me.Members, 2)
	ids := map[uuid.UUID]bool{}
	for _, m := range me.Members {
		ids[m.ID] = true
	}
	require.False(t, ids[disabledMember.ID], "disabled (removed) member must leave the roster")
	require.False(t, ids[deletedMember.ID], "soft-deleted account must leave the roster")
}

func TestRenameIsOwnerOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)

	err := e.centersSvc.Rename(ctx, e.scope(t, member.ID), centers.RenameRequest{Name: "Trung Tâm Bình Minh"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	require.NoError(t, e.centersSvc.Rename(ctx, e.scope(t, owner.ID), centers.RenameRequest{Name: "Trung Tâm Bình Minh"}))
	var reloaded centers.Center
	require.NoError(t, e.db.First(&reloaded, "id = ?", ownerTeacher.CenterID).Error)
	require.Equal(t, "Trung Tâm Bình Minh", reloaded.Name)
}

// TestRemoveMemberByOwnerDataStaysBehind pins the offboarding contract: no
// new center is provisioned for the removed member, their membership closes,
// their account is disabled and every refresh token they held stops working,
// teachers.center_id is left pointing at the center they were just removed
// from, and their teaching history stays reachable there under their name.
func TestRemoveMemberByOwnerDataStaysBehind(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên B"))
	e.join(t, member.ID, owner.ID)
	classID := insertClass(t, e.db, member.ID, ownerTeacher.CenterID)

	sess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: testutil.DefaultPassword}, auth.ClientMeta{})
	require.NoError(t, err)

	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), member.ID))

	// The membership stint closed; no new center is provisioned.
	var closed centers.Member
	require.NoError(t, e.db.First(&closed, "teacher_id = ? AND center_id = ?", member.ID, ownerTeacher.CenterID).Error)
	require.NotNil(t, closed.LeftAt)

	var centerCount int64
	require.NoError(t, e.db.Table("centers").
		Where("owner_id = ? AND deleted_at IS NULL", member.ID).Count(&centerCount).Error)
	require.Zero(t, centerCount, "removal must not provision a new center")

	// teachers.center_id is left pointing at the center they were removed
	// from — only a later reactivate moves it.
	var reloaded teachers.Teacher
	require.NoError(t, e.db.First(&reloaded, "id = ?", member.ID).Error)
	require.Equal(t, ownerTeacher.CenterID, reloaded.CenterID)

	// The account is disabled and can no longer resolve a scope.
	var acct teachers.Account
	require.NoError(t, e.db.First(&acct, "id = ?", member.ID).Error)
	require.Equal(t, teachers.StatusDisabled, acct.Status)
	_, err = e.centersSvc.ResolveScope(ctx, member.ID)
	require.Equal(t, apperror.CodeUnauthorized, apperror.From(err).Code)

	// Their refresh token from before the removal no longer works.
	_, err = e.authSvc.Refresh(ctx, sess.RefreshToken)
	require.Error(t, err)

	// Their teaching history stays in the old center under their name.
	var stayed int64
	require.NoError(t, e.db.Table("classes").
		Where("id = ? AND center_id = ? AND teacher_id = ?", classID, ownerTeacher.CenterID, member.ID).
		Count(&stayed).Error)
	require.EqualValues(t, 1, stayed)
}

// Class-staff stints belong to a membership stint: removal soft-closes every
// active one (history reads survive as ended rows), and reopening a membership
// never resurrects them — even a stint a non-standard close path left active.
func TestMembershipCloseAndReopenEndStaffStints(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	classID := insertClass(t, e.db, member.ID, ownerTeacher.CenterID)

	activeStints := func() int64 {
		var n int64
		require.NoError(t, e.db.Raw(
			`SELECT count(*) FROM class_staff
			 WHERE teacher_id = ? AND center_id = ? AND ended_at IS NULL`,
			member.ID, ownerTeacher.CenterID).Scan(&n).Error)
		return n
	}
	addStint := func(roleKey string) {
		require.NoError(t, e.db.Exec(
			`INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
			 VALUES (?, ?, ?, ?)`,
			classID, ownerTeacher.CenterID, member.ID, roleKey).Error)
	}

	addStint("giao_vien")
	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), member.ID))
	require.Zero(t, activeStints(), "removal soft-closes every active stint")

	var total int64
	require.NoError(t, e.db.Raw(
		`SELECT count(*) FROM class_staff WHERE teacher_id = ?`, member.ID).Scan(&total).Error)
	require.EqualValues(t, 1, total, "the stint is closed, not deleted")

	// A stint left active by a non-standard close path dies on reopen too.
	addStint("hoc_vu")
	_, err := centers.NewRepository(e.db).OpenMembership(ctx, member.ID, ownerTeacher.CenterID)
	require.NoError(t, err)
	require.Zero(t, activeStints(), "reopening a membership resurrects no stint")
}

func TestRemoveMemberAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	memberB, _ := testutil.Teacher(t, e.db)
	memberC, _ := testutil.Teacher(t, e.db)
	e.join(t, memberB.ID, owner.ID)
	e.join(t, memberC.ID, owner.ID)

	// A plain member cannot remove anyone — not another member, and not even
	// themselves; offboarding is owner-only.
	err := e.centersSvc.RemoveMember(ctx, e.scope(t, memberB.ID), memberC.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	err = e.centersSvc.RemoveMember(ctx, e.scope(t, memberB.ID), memberB.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	// The owner cannot remove themselves while the center still has other
	// members…
	err = e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), owner.ID)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	// …and cannot remove themselves even once it is empty.
	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), memberB.ID))
	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), memberC.ID))
	err = e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), owner.ID)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	// Someone who was never a member is a 404, not a 403 — same shape as any
	// other cross-tenant probe.
	stranger, _ := testutil.Teacher(t, e.db)
	err = e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), stranger.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), uuid.New())
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// TestKickIsEffectiveOnTheNextRequest proves the no-JWT-caching decision end
// to end: the same principal, run through the real middleware twice, sees the
// old center before the kick and 401s right after — offboarding disables the
// account outright, so the kicked member never lands in a scope of their own.
func TestKickIsEffectiveOnTheNextRequest(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)

	resolve := middleware.ResolveScope(e.centersSvc)
	scopeVia := func(teacherID uuid.UUID) (authctx.Scope, int) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		authctx.Set(c, authctx.Principal{UserID: teacherID, Role: authctx.RoleTeacher})
		resolve(c)
		s, _ := authctx.ScopeFrom(c)
		return s, w.Code
	}

	before, code := scopeVia(member.ID)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, ownerTeacher.CenterID, before.CenterID)

	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), member.ID))

	_, code = scopeVia(member.ID)
	require.Equal(t, http.StatusUnauthorized, code, "kick must bite on the very next request")
}

// TestCentersRoutesEndToEnd drives the mounted routes with real bearer
// tokens: status codes, request binding, and param parsing are contracts of
// the HTTP layer that service-level tests cannot pin.
func TestCentersRoutesEndToEnd(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	e := newEnv(t)

	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	issuer := auth.NewTokenIssuer(jwtCfg)
	r := gin.New()
	centers.RegisterRoutes(r.Group("/api/v1"), centers.NewHandler(e.centersSvc),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(e.centersSvc))

	owner, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ Trung Tâm"))
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	tokenFor := func(id uuid.UUID) string {
		token, err := issuer.IssueAccess(id, authctx.RoleTeacher)
		require.NoError(t, err)
		return token
	}
	do := func(token, method, path, body string) *httptest.ResponseRecorder {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// GET /centers/me shows the full roster to the owner…
	w := do(tokenFor(owner.ID), http.MethodGet, "/api/v1/centers/me", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Chủ Trung Tâm")
	require.Contains(t, w.Body.String(), "members")

	// …and only the center's name to a plain member — no roster leaks.
	w = do(tokenFor(member.ID), http.MethodGet, "/api/v1/centers/me", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Chủ Trung Tâm")
	require.NotContains(t, w.Body.String(), "members")

	// Rename is owner-only over HTTP too.
	w = do(tokenFor(member.ID), http.MethodPatch, "/api/v1/centers/me", `{"name":"Đổi Tên"}`)
	require.Equal(t, http.StatusForbidden, w.Code)

	// A non-UUID path param is a 404, same shape as an unknown member.
	w = do(tokenFor(owner.ID), http.MethodDelete, "/api/v1/centers/me/members/not-a-uuid", "")
	require.Equal(t, http.StatusNotFound, w.Code)

	// A plain member cannot remove anyone.
	w = do(tokenFor(member.ID), http.MethodDelete, "/api/v1/centers/me/members/"+owner.ID.String(), "")
	require.Equal(t, http.StatusForbidden, w.Code)

	// The owner removes the member: 204, no body.
	w = do(tokenFor(owner.ID), http.MethodDelete, "/api/v1/centers/me/members/"+member.ID.String(), "")
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// The removed member's account is disabled and their very next request
	// 401s — offboarding bites immediately, not just at the next login.
	var acct teachers.Account
	require.NoError(t, e.db.First(&acct, "id = ?", member.ID).Error)
	require.Equal(t, teachers.StatusDisabled, acct.Status)
	w = do(tokenFor(member.ID), http.MethodGet, "/api/v1/centers/me", "")
	require.Equal(t, http.StatusUnauthorized, w.Code)

	// No token → 401 before any handler runs.
	w = do("", http.MethodGet, "/api/v1/centers/me", "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Dashboard ---

// newDashboard mirrors router wiring: the dashboard consumes classes,
// sessions, and attendance through read-only consumer interfaces.
func newDashboard(e *env) *centers.Dashboard {
	classesSvc := classes.NewService(classes.NewRepository(e.db), e.tx, classstaff.NewRepository(e.db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(e.db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(e.db), classesSvc, e.teachersSvc, enrollmentsSvc)
	attendanceSvc := attendance.NewService(attendance.NewRepository(e.db), enrollmentsSvc, sessionsSvc, e.tx)
	return centers.NewDashboard(centers.NewRepository(e.db), classesSvc, sessionsSvc, attendanceSvc)
}

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

// dashboardScenario is one center with an owner, a member, and an outsider in
// their own center, plus enough March 2026 activity to hand-compute every
// dashboard metric. Soft-deleted and cross-center rows exist specifically so
// aggregates that forget a filter produce wrong numbers.
type dashboardScenario struct {
	owner, member, outsider *teachers.Teacher
	centerID                uuid.UUID
	memberClass             *classes.Class // "Toán 6", active
	memberArchived          *classes.Class // "Văn 6", archived, no activity
	ownerClass              *classes.Class // "Lý 8", active
	outsiderClass           *classes.Class
	ses1, ses2, ses3, ses4  *sessions.Session // held, held, planned, cancelled
	ownerSession            *sessions.Session
	outsiderSession         *sessions.Session
}

func buildDashboardScenario(t *testing.T, e *env) *dashboardScenario {
	t.Helper()
	db := e.db
	_, owner := testutil.Teacher(t, db, testutil.WithFullName("Chủ Trung Tâm"))
	_, member := testutil.Teacher(t, db, testutil.WithFullName("Giáo Viên A"))
	_, outsider := testutil.Teacher(t, db, testutil.WithFullName("Người Ngoài"))
	centerID := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, centerID)

	// Member roster: s1 enrolled all of March, s2 leaves 03-15, s3 joins
	// 03-10. A soft-deleted enrollment and class exist only as leak bait.
	mc := testutil.Contact(t, db, member.ID)
	s1 := testutil.Student(t, db, member.ID, mc.ID, testutil.WithStudentFullName("An"))
	s2 := testutil.Student(t, db, member.ID, mc.ID, testutil.WithStudentFullName("Bình"))
	s3 := testutil.Student(t, db, member.ID, mc.ID, testutil.WithStudentFullName("Chi"))
	memberClass := testutil.Class(t, db, member.ID,
		testutil.WithClassName("Toán 6"), testutil.WithClassStartDate(day("2026-01-01")))
	memberArchived := testutil.Class(t, db, member.ID,
		testutil.WithClassName("Văn 6"), testutil.WithClassStatus(classes.StatusArchived),
		testutil.WithClassStartDate(day("2026-01-01")))
	deletedClass := testutil.Class(t, db, member.ID,
		testutil.WithClassName("Sử 6"), testutil.WithClassStartDate(day("2026-01-01")))
	require.NoError(t, db.Delete(deletedClass).Error)

	e1 := testutil.Enrollment(t, db, member.ID, s1.ID, memberClass.ID, day("2026-01-05"))
	e2 := testutil.Enrollment(t, db, member.ID, s2.ID, memberClass.ID, day("2026-02-01"))
	require.NoError(t, db.Model(e2).Update("ended_on", day("2026-03-15")).Error)
	eDel := testutil.Enrollment(t, db, member.ID, s3.ID, memberClass.ID, day("2026-01-01"))
	require.NoError(t, db.Delete(eDel).Error)
	e3 := testutil.Enrollment(t, db, member.ID, s3.ID, memberClass.ID, day("2026-03-10"))

	confirmed := testutil.WithSessionAttendanceConfirmed(time.Now())
	ses1 := testutil.Session(t, db, member.ID, memberClass.ID, day("2026-03-03"), confirmed)
	ses2 := testutil.Session(t, db, member.ID, memberClass.ID, day("2026-03-10"), confirmed)
	ses3 := testutil.Session(t, db, member.ID, memberClass.ID, day("2026-03-17"))
	ses4 := testutil.Session(t, db, member.ID, memberClass.ID, day("2026-03-24"),
		testutil.WithSessionStatus(sessions.StatusCancelled), testutil.WithSessionCancelReason("nghỉ lễ"))
	sesFeb := testutil.Session(t, db, member.ID, memberClass.ID, day("2026-02-10"), confirmed)
	sesDel := testutil.Session(t, db, member.ID, memberClass.ID, day("2026-03-31"), confirmed)

	testutil.AttendanceRecord(t, db, member.ID, ses1.ID, s1.ID, e1.ID)
	testutil.AttendanceRecord(t, db, member.ID, ses1.ID, s2.ID, e2.ID)
	arDel := testutil.AttendanceRecord(t, db, member.ID, ses1.ID, s3.ID, e3.ID)
	require.NoError(t, db.Delete(arDel).Error)
	testutil.AttendanceRecord(t, db, member.ID, ses2.ID, s1.ID, e1.ID)
	testutil.AttendanceRecord(t, db, member.ID, ses2.ID, s2.ID, e2.ID,
		testutil.WithAttendanceStatus(attendance.StatusAbsent))
	testutil.AttendanceRecord(t, db, member.ID, sesFeb.ID, s1.ID, e1.ID)
	// The record survives its session's soft delete; joins that skip the
	// session's deleted_at filter would double-count it.
	testutil.AttendanceRecord(t, db, member.ID, sesDel.ID, s1.ID, e1.ID)
	require.NoError(t, db.Delete(sesDel).Error)

	// Member's March close: one issued invoice (line 400k on e1, adjustment
	// -50k sourced at ses2, adjustment -10k with no source session), and one
	// void invoice whose line must never count. The sourced adjustment counts
	// in the class-level books but is a double-count trap at session level:
	// only the reconciler writes sourced adjustments, and their effect is
	// already reflected in the live attendance records.
	now := time.Now()
	period := &billing.Period{ID: id.New(), TeacherID: member.ID, CenterID: centerID,
		Year: 2026, Month: 3, PeriodStart: day("2026-03-01"), PeriodEnd: day("2026-03-31"),
		Status: billing.PeriodClosed, ClosedAt: &now}
	require.NoError(t, db.Create(period).Error)
	inv := &billing.Invoice{ID: id.New(), TeacherID: member.ID, CenterID: centerID,
		PeriodID: period.ID, StudentID: s1.ID, ContactID: mc.ID,
		StudentName: "An", ContactName: mc.FullName,
		CurrentCharge: 400_000, AdjustmentTotal: -60_000, TotalDue: 340_000, Status: billing.InvoiceIssued}
	require.NoError(t, db.Create(inv).Error)
	require.NoError(t, db.Create(&billing.InvoiceLine{ID: id.New(), TeacherID: member.ID,
		CenterID: centerID, InvoiceID: inv.ID, EnrollmentID: e1.ID, ClassName: "Toán 6",
		BillableCount: 4, UnitPrice: 100_000, Amount: 400_000}).Error)
	require.NoError(t, db.Create(&billing.InvoiceAdjustment{ID: id.New(), TeacherID: member.ID,
		CenterID: centerID, InvoiceID: inv.ID, Amount: -50_000, Reason: "giảm trừ",
		SourceSessionID: &ses2.ID}).Error)
	require.NoError(t, db.Create(&billing.InvoiceAdjustment{ID: id.New(), TeacherID: member.ID,
		CenterID: centerID, InvoiceID: inv.ID, Amount: -10_000, Reason: "chiết khấu chung"}).Error)
	voidReason := "nhập nhầm"
	voided := &billing.Invoice{ID: id.New(), TeacherID: member.ID, CenterID: centerID,
		PeriodID: period.ID, StudentID: s2.ID, ContactID: mc.ID,
		StudentName: "Bình", ContactName: mc.FullName,
		CurrentCharge: 1_000_000, TotalDue: 1_000_000,
		Status: billing.InvoiceVoid, VoidReason: &voidReason, VoidedAt: &now}
	require.NoError(t, db.Create(voided).Error)
	require.NoError(t, db.Create(&billing.InvoiceLine{ID: id.New(), TeacherID: member.ID,
		CenterID: centerID, InvoiceID: voided.ID, EnrollmentID: e2.ID, ClassName: "Toán 6",
		BillableCount: 10, UnitPrice: 100_000, Amount: 1_000_000}).Error)

	// Owner teaches too: one held March session, no closed period.
	oc := testutil.Contact(t, db, owner.ID)
	os1 := testutil.Student(t, db, owner.ID, oc.ID, testutil.WithStudentFullName("Dương"))
	ownerClass := testutil.Class(t, db, owner.ID,
		testutil.WithClassName("Lý 8"), testutil.WithClassStartDate(day("2026-01-01")))
	oe1 := testutil.Enrollment(t, db, owner.ID, os1.ID, ownerClass.ID, day("2026-01-01"))
	ownerSession := testutil.Session(t, db, owner.ID, ownerClass.ID, day("2026-03-05"), confirmed)
	testutil.AttendanceRecord(t, db, owner.ID, ownerSession.ID, os1.ID, oe1.ID)

	// A stranger's center with the same shape of March activity: none of it
	// may surface through center O's dashboard.
	xc := testutil.Contact(t, db, outsider.ID)
	xs := testutil.Student(t, db, outsider.ID, xc.ID)
	outsiderClass := testutil.Class(t, db, outsider.ID,
		testutil.WithClassName("Hoá 9"), testutil.WithClassStartDate(day("2026-01-01")))
	xe := testutil.Enrollment(t, db, outsider.ID, xs.ID, outsiderClass.ID, day("2026-01-01"))
	outsiderSession := testutil.Session(t, db, outsider.ID, outsiderClass.ID, day("2026-03-03"), confirmed)
	testutil.AttendanceRecord(t, db, outsider.ID, outsiderSession.ID, xs.ID, xe.ID)

	return &dashboardScenario{
		owner: owner, member: member, outsider: outsider, centerID: centerID,
		memberClass: memberClass, memberArchived: memberArchived,
		ownerClass: ownerClass, outsiderClass: outsiderClass,
		ses1: ses1, ses2: ses2, ses3: ses3, ses4: ses4,
		ownerSession: ownerSession, outsiderSession: outsiderSession,
	}
}

func TestDashboardTeachersRosterAndCounts(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dash := newDashboard(e)
	ctx := context.Background()
	sn := buildDashboardScenario(t, e)
	ownerScope := e.scope(t, sn.owner.ID)

	rows, err := dash.Teachers(ctx, ownerScope)
	require.NoError(t, err)
	require.Len(t, rows, 2, "roster is the center's current members only")
	require.Equal(t, sn.owner.ID, rows[0].Teacher.ID, "owner sorts first")
	require.True(t, rows[0].Teacher.IsOwner)
	require.Equal(t, 1, rows[0].ActiveClasses)
	require.Equal(t, 1, rows[0].ActiveStudents)
	require.Equal(t, sn.member.ID, rows[1].Teacher.ID)
	require.Equal(t, 1, rows[1].ActiveClasses, "archived and soft-deleted classes are not active")
	require.Equal(t, 2, rows[1].ActiveStudents, "an ended enrollment leaves the active count")

	_, err = dash.Teachers(ctx, e.scope(t, sn.member.ID))
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

func TestDashboardOverviewMatchesHandComputedNumbers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dash := newDashboard(e)
	ctx := context.Background()
	sn := buildDashboardScenario(t, e)
	ownerScope := e.scope(t, sn.owner.ID)

	groups, err := dash.Overview(ctx, ownerScope, "2026-03")
	require.NoError(t, err)

	byClass := map[uuid.UUID]centers.OverviewClassResponse{}
	for _, g := range groups {
		for _, c := range g.Classes {
			byClass[c.ClassID] = c
		}
	}
	require.Len(t, byClass, 3, "live classes of the center only — no soft-deleted, no other centers")

	m := byClass[sn.memberClass.ID]
	require.Equal(t, 2, m.SessionsHeld, "February and soft-deleted sessions do not count")
	require.NotNil(t, m.AvgAttendance)
	require.InDelta(t, 2.0, *m.AvgAttendance, 1e-9)
	require.NotNil(t, m.PresentRate)
	require.InDelta(t, 0.75, *m.PresentRate, 1e-9, "3 of 4 live records are present")
	require.NotNil(t, m.RetentionRate)
	require.InDelta(t, 0.5, *m.RetentionRate, 1e-9, "2 active on 03-01, 1 still active on 03-31")
	require.EqualValues(t, 400_000, m.EstimatedRevenue)
	require.NotNil(t, m.InvoicedRevenue, "member's March period is closed")
	require.EqualValues(t, 350_000, *m.InvoicedRevenue,
		"line 400k + session-sourced adjustment -50k; unattributed adjustment and void invoice excluded")

	a := byClass[sn.memberArchived.ID]
	require.Equal(t, 0, a.SessionsHeld)
	require.Nil(t, a.AvgAttendance)
	require.Nil(t, a.PresentRate)
	require.Nil(t, a.RetentionRate, "no enrollment active on 03-01 → null, not zero")
	require.EqualValues(t, 0, a.EstimatedRevenue)
	require.NotNil(t, a.InvoicedRevenue)
	require.EqualValues(t, 0, *a.InvoicedRevenue)

	o := byClass[sn.ownerClass.ID]
	require.Equal(t, 1, o.SessionsHeld)
	require.NotNil(t, o.RetentionRate)
	require.InDelta(t, 1.0, *o.RetentionRate, 1e-9)
	require.EqualValues(t, 100_000, o.EstimatedRevenue)
	require.Nil(t, o.InvoicedRevenue, "owner has no closed March period → null")

	_, err = dash.Overview(ctx, ownerScope, "2026-13")
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	_, err = dash.Overview(ctx, e.scope(t, sn.member.ID), "2026-03")
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

func TestDashboardDrillDownAuthz(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dash := newDashboard(e)
	ctx := context.Background()
	sn := buildDashboardScenario(t, e)
	ownerScope := e.scope(t, sn.owner.ID)
	memberScope := e.scope(t, sn.member.ID)
	page := pagination.Params{Page: 1, PerPage: 100}
	activeOnly := classes.ListFilter{Status: classes.StatusActive}

	// A plain member is refused everywhere.
	_, _, err := dash.TeacherClasses(ctx, memberScope, sn.member.ID, activeOnly, page)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = dash.ClassSessions(ctx, memberScope, sn.member.ID, sn.memberClass.ID, day("2026-03-01"), day("2026-03-31"))
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = dash.Session(ctx, memberScope, sn.ses1.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	// Another center's ids are refused with the same generic 403.
	_, _, err = dash.TeacherClasses(ctx, ownerScope, sn.outsider.ID, activeOnly, page)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = dash.ClassSessions(ctx, ownerScope, sn.member.ID, sn.outsiderClass.ID, day("2026-03-01"), day("2026-03-31"))
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = dash.Session(ctx, ownerScope, sn.outsiderSession.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	// A class that exists in the center but under a different teacher than
	// the path claims is refused, not silently served.
	_, err = dash.ClassSessions(ctx, ownerScope, sn.member.ID, sn.ownerClass.ID, day("2026-03-01"), day("2026-03-31"))
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

func TestDashboardClassSessionsReadsWithoutWriting(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dash := newDashboard(e)
	ctx := context.Background()
	sn := buildDashboardScenario(t, e)
	ownerScope := e.scope(t, sn.owner.ID)

	var before int64
	require.NoError(t, e.db.Table("class_sessions").Count(&before).Error)

	rows, err := dash.ClassSessions(ctx, ownerScope, sn.member.ID, sn.memberClass.ID,
		day("2026-03-01"), day("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 4, "only rows that already exist — nothing generated, soft-deleted excluded")

	require.Equal(t, sn.ses1.ID, rows[0].SessionID)
	require.Equal(t, 2, rows[0].AttendanceTotal)
	require.Equal(t, 2, rows[0].PresentCount, "the soft-deleted record does not count")
	require.EqualValues(t, 200_000, rows[0].EstimatedRevenue)
	require.Equal(t, sn.ses2.ID, rows[1].SessionID)
	require.Equal(t, 2, rows[1].AttendanceTotal)
	require.Equal(t, 1, rows[1].PresentCount)
	require.EqualValues(t, 200_000, rows[1].EstimatedRevenue, "billable absences still bill")
	require.Equal(t, sn.ses3.ID, rows[2].SessionID)
	require.Equal(t, sessions.StatusPlanned, rows[2].Status)
	require.Zero(t, rows[2].AttendanceTotal)
	require.EqualValues(t, 0, rows[2].EstimatedRevenue)
	require.Equal(t, sn.ses4.ID, rows[3].SessionID)
	require.Equal(t, sessions.StatusCancelled, rows[3].Status)

	var after int64
	require.NoError(t, e.db.Table("class_sessions").Count(&after).Error)
	require.Equal(t, before, after, "an owner's dashboard GET must never insert sessions")

	_, err = dash.ClassSessions(ctx, ownerScope, sn.member.ID, sn.memberClass.ID,
		day("2026-03-31"), day("2026-03-01"))
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
}

func TestDashboardSessionDetail(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dash := newDashboard(e)
	ctx := context.Background()
	sn := buildDashboardScenario(t, e)
	ownerScope := e.scope(t, sn.owner.ID)

	detail, err := dash.Session(ctx, ownerScope, sn.ses2.ID)
	require.NoError(t, err)
	require.Equal(t, sn.ses2.ID, detail.Session.ID)
	require.Equal(t, "Toán 6", detail.Session.ClassName)
	require.Equal(t, sn.member.ID, detail.Session.TeacherID)
	require.Len(t, detail.Attendance, 3, "roster active on 03-10: An, Bình, and Chi who joined that day")
	byName := map[string]centers.SessionAttendanceRow{}
	for _, r := range detail.Attendance {
		byName[r.StudentName] = r
	}
	require.NotNil(t, byName["An"].Status)
	require.Equal(t, attendance.StatusPresent, *byName["An"].Status)
	require.NotNil(t, byName["Bình"].Status)
	require.Equal(t, attendance.StatusAbsent, *byName["Bình"].Status)
	require.Nil(t, byName["Chi"].Status, "on the roster but never recorded → null status")
	require.EqualValues(t, 200_000, detail.EstimatedRevenue)
	require.NotNil(t, detail.InvoicedRevenue)
	require.EqualValues(t, 100_000, *detail.InvoicedRevenue,
		"e1's line-backed share only; e2 has no non-void line, and the "+
			"adjustment sourced at this session must not be re-added — its "+
			"effect already lives in the records (reconciler contract)")

	ownerDetail, err := dash.Session(ctx, ownerScope, sn.ownerSession.ID)
	require.NoError(t, err)
	require.Nil(t, ownerDetail.InvoicedRevenue, "session's month has no closed period → null")
	require.EqualValues(t, 100_000, ownerDetail.EstimatedRevenue)
}

func TestDashboardKeepsARemovedTeachersData(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dash := newDashboard(e)
	ctx := context.Background()
	sn := buildDashboardScenario(t, e)
	ownerScope := e.scope(t, sn.owner.ID)
	page := pagination.Params{Page: 1, PerPage: 100}

	// Drill-down works on a live member first; removal must not be the only
	// path exercised.
	liveList, _, err := dash.TeacherClasses(ctx, ownerScope, sn.member.ID,
		classes.ListFilter{Status: classes.StatusActive}, page)
	require.NoError(t, err)
	require.Len(t, liveList, 1)

	require.NoError(t, e.centersSvc.RemoveMember(ctx, ownerScope, sn.member.ID))

	rows, err := dash.Teachers(ctx, ownerScope)
	require.NoError(t, err)
	require.Len(t, rows, 1, "removed teacher leaves the roster")

	// Their left-behind data stays reachable: the rows are anchored on the
	// center, not the membership.
	classList, _, err := dash.TeacherClasses(ctx, ownerScope, sn.member.ID,
		classes.ListFilter{Status: classes.StatusActive}, page)
	require.NoError(t, err)
	require.Len(t, classList, 1)
	require.Equal(t, sn.memberClass.ID, classList[0].ID)

	archivedList, _, err := dash.TeacherClasses(ctx, ownerScope, sn.member.ID,
		classes.ListFilter{Status: classes.StatusArchived}, page)
	require.NoError(t, err)
	require.Len(t, archivedList, 1)
	require.Equal(t, sn.memberArchived.ID, archivedList[0].ID)

	sessionRows, err := dash.ClassSessions(ctx, ownerScope, sn.member.ID, sn.memberClass.ID,
		day("2026-03-01"), day("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, sessionRows, 4)

	// The removed teacher's session detail — roster sheet included — stays
	// readable, and reading it writes nothing.
	var before int64
	require.NoError(t, e.db.Table("class_sessions").Count(&before).Error)
	detail, err := dash.Session(ctx, ownerScope, sn.ses1.ID)
	require.NoError(t, err)
	require.Equal(t, sn.member.ID, detail.Session.TeacherID)
	require.NotEmpty(t, detail.Attendance)
	var after int64
	require.NoError(t, e.db.Table("class_sessions").Count(&after).Error)
	require.Equal(t, before, after, "session detail is a pure read")

	groups, err := dash.Overview(ctx, ownerScope, "2026-03")
	require.NoError(t, err)
	found := false
	for _, g := range groups {
		for _, c := range g.Classes {
			if c.ClassID == sn.memberClass.ID {
				found = true
			}
		}
	}
	require.True(t, found, "overview still reports the removed teacher's classes")
}

// TestDashboardRoutesEndToEnd drives the mounted dashboard routes with real
// bearer tokens. Status codes, path-param parsing, and the
// authorization-before-validation order are HTTP-layer contracts that
// service-level tests cannot pin; mounting also proves the route tree has no
// gin wildcard conflicts.
func TestDashboardRoutesEndToEnd(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	e := newEnv(t)

	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	issuer := auth.NewTokenIssuer(jwtCfg)
	r := gin.New()
	centers.RegisterDashboardRoutes(r.Group("/api/v1"), centers.NewDashboardHandler(newDashboard(e)),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(e.centersSvc))

	owner, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ Trung Tâm"))
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	tokenFor := func(id uuid.UUID) string {
		token, err := issuer.IssueAccess(id, authctx.RoleTeacher)
		require.NoError(t, err)
		return token
	}
	do := func(token, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// No token → 401 before any handler runs.
	w := do("", "/api/v1/centers/dashboard/teachers")
	require.Equal(t, http.StatusUnauthorized, w.Code)

	// The owner reads the roster: themselves plus the member.
	w = do(tokenFor(owner.ID), "/api/v1/centers/dashboard/teachers")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Chủ Trung Tâm")

	// A member is 403 on every dashboard route — including ones with invalid
	// query parameters, which must not leak a 422 past the authorization gate.
	memberPaths := []string{
		"/centers/dashboard/teachers",
		"/centers/dashboard/overview?month=2026-13",
		"/centers/dashboard/teachers/" + member.ID.String() + "/classes?status=bogus",
		"/centers/dashboard/teachers/" + member.ID.String() + "/classes/" + uuid.New().String() + "/sessions?from=garbage",
		"/centers/dashboard/sessions/" + uuid.New().String(),
	}
	for _, p := range memberPaths {
		w = do(tokenFor(member.ID), "/api/v1"+p)
		require.Equal(t, http.StatusForbidden, w.Code, p+" → "+w.Body.String())
	}

	// A non-UUID path param gets the same uniform 403 as any inaccessible id.
	w = do(tokenFor(owner.ID), "/api/v1/centers/dashboard/teachers/not-a-uuid/classes")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = do(tokenFor(owner.ID), "/api/v1/centers/dashboard/sessions/not-a-uuid")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// For the owner, validation still bites: bad status and a missing range
	// are 422 once authorization has passed.
	w = do(tokenFor(owner.ID), "/api/v1/centers/dashboard/teachers/"+member.ID.String()+"/classes?status=bogus")
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	w = do(tokenFor(owner.ID), "/api/v1/centers/dashboard/teachers/"+member.ID.String()+"/classes/"+uuid.New().String()+"/sessions")
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}
