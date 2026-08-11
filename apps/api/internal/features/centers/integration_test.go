//go:build integration

package centers_test

import (
	"context"
	"encoding/json"
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
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

type env struct {
	db          *gorm.DB
	tx          database.TxManager
	teachersSvc *teachers.Service
	centersSvc  *centers.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), teachersSvc, txMgr)
	teachersSvc.SetCenterProvisioner(centersSvc)
	return &env{db: db, tx: txMgr, teachersSvc: teachersSvc, centersSvc: centersSvc}
}

// scope resolves the caller's scope the way the middleware would, straight
// from the database.
func (e *env) scope(t *testing.T, teacherID uuid.UUID) authctx.Scope {
	t.Helper()
	s, err := e.centersSvc.ResolveScope(context.Background(), teacherID)
	require.NoError(t, err)
	return s
}

// join is the happy-path shorthand: teacher joins the center owned by the
// account with ownerPhone.
func (e *env) join(t *testing.T, teacherID uuid.UUID, ownerPhone string) *centers.JoinResponse {
	t.Helper()
	resp, err := e.centersSvc.Join(context.Background(), e.scope(t, teacherID), centers.JoinRequest{OwnerPhone: ownerPhone})
	require.NoError(t, err)
	return resp
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

func TestCreateTeacherProvisionsPersonalCenter(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	var p *teachers.Profile
	require.NoError(t, e.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		p, err = e.teachersSvc.CreateTeacher(ctx, teachers.CreateRequest{
			Phone: "0901234567", Password: "password-123", FullName: "Cô Lan",
		})
		return err
	}))

	require.NotEqual(t, uuid.Nil, p.Teacher.CenterID, "new teacher must land in a center")

	var center centers.Center
	require.NoError(t, e.db.First(&center, "id = ?", p.Teacher.CenterID).Error)
	require.Equal(t, p.Account.ID, center.OwnerID, "personal center is owned by its teacher")
	require.Equal(t, "Cô Lan", center.Name)

	m := e.liveMembership(t, p.Account.ID)
	require.Equal(t, center.ID, m.CenterID)

	s := e.scope(t, p.Account.ID)
	require.Equal(t, center.ID, s.CenterID)
	require.True(t, s.IsOwner)
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

func TestJoinMovesTeacherAndRetiresPersonalCenter(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, ownerTeacher := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ Trung Tâm"))
	member, memberTeacher := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên B"))
	oldPersonal := memberTeacher.CenterID

	resp := e.join(t, member.ID, owner.Phone)
	require.Equal(t, ownerTeacher.CenterID, resp.CenterID)
	require.False(t, resp.JoinedAt.IsZero())

	var reloaded teachers.Teacher
	require.NoError(t, e.db.First(&reloaded, "id = ?", member.ID).Error)
	require.Equal(t, ownerTeacher.CenterID, reloaded.CenterID)

	// Old membership row closed, new one live.
	var closed centers.Member
	require.NoError(t, e.db.First(&closed, "teacher_id = ? AND center_id = ?", member.ID, oldPersonal).Error)
	require.NotNil(t, closed.LeftAt)
	require.Equal(t, ownerTeacher.CenterID, e.liveMembership(t, member.ID).CenterID)

	// The empty personal center is retired, not deleted.
	var gone int64
	require.NoError(t, e.db.Table("centers").Where("id = ? AND deleted_at IS NOT NULL", oldPersonal).Count(&gone).Error)
	require.EqualValues(t, 1, gone)

	// Membership is effective immediately: the very next scope resolve sees it.
	s := e.scope(t, member.ID)
	require.Equal(t, ownerTeacher.CenterID, s.CenterID)
	require.False(t, s.IsOwner)
}

func TestJoinRejectsWhenCallerCenterStillHasData(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	caller, callerTeacher := testutil.Teacher(t, e.db)

	contactID := insertContact(t, e.db, caller.ID, callerTeacher.CenterID)
	studentID := insertStudent(t, e.db, caller.ID, callerTeacher.CenterID, contactID)
	classID := insertClass(t, e.db, caller.ID, callerTeacher.CenterID)

	join := func() error {
		_, err := e.centersSvc.Join(ctx, e.scope(t, caller.ID), centers.JoinRequest{OwnerPhone: owner.Phone})
		return err
	}

	// Any of the three root business tables blocks the move.
	require.Equal(t, apperror.CodeConflict, apperror.From(join()).Code)
	require.NoError(t, e.db.Exec("UPDATE classes SET deleted_at = now() WHERE id = ?", classID).Error)
	require.Equal(t, apperror.CodeConflict, apperror.From(join()).Code)
	require.NoError(t, e.db.Exec("UPDATE students SET deleted_at = now() WHERE id = ?", studentID).Error)
	require.Equal(t, apperror.CodeConflict, apperror.From(join()).Code)

	// Soft-deleted rows do not hold the teacher hostage: once everything live
	// is gone the join goes through.
	require.NoError(t, e.db.Exec("UPDATE contacts SET deleted_at = now() WHERE id = ?", contactID).Error)
	require.NoError(t, join())
}

func TestJoinErrorMatrix(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ A"))
	member, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Thành Viên B"))
	outsider, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Người Ngoài C"))
	e.join(t, member.ID, owner.Phone)

	joinAs := func(callerID uuid.UUID, phone string) *apperror.AppError {
		_, err := e.centersSvc.Join(ctx, e.scope(t, callerID), centers.JoinRequest{OwnerPhone: phone})
		return apperror.From(err)
	}

	// Unknown phone → generic 404, indistinguishable from the cases below.
	require.Equal(t, apperror.CodeNotFound, joinAs(outsider.ID, "0999999999").Code)

	// A teacher who is a plain member owns no live center → same 404.
	require.Equal(t, apperror.CodeNotFound, joinAs(outsider.ID, member.Phone).Code)

	// Disabled owner → same 404.
	disabled, _ := testutil.Teacher(t, e.db, testutil.WithStatus(teachers.StatusDisabled))
	require.Equal(t, apperror.CodeNotFound, joinAs(outsider.ID, disabled.Phone).Code)

	// Joining your own center is a semantic error, not a conflict.
	require.Equal(t, apperror.CodeValidation, joinAs(outsider.ID, outsider.Phone).Code)

	// Already in that owner's center → 409.
	require.Equal(t, apperror.CodeConflict, joinAs(member.ID, owner.Phone).Code)

	// A member of another center must leave before joining anywhere else.
	require.Equal(t, apperror.CodeConflict, joinAs(member.ID, outsider.Phone).Code)

	// An owner whose center still has members cannot walk away into another.
	_ = ownerTeacher
	require.Equal(t, apperror.CodeConflict, joinAs(owner.ID, outsider.Phone).Code)

	// Caller-side ineligibility is decided before the phone is even looked
	// up, so an ineligible caller cannot use the 404-vs-409 split to probe
	// which phones own live centers: with a nonsense phone they still get
	// their own 409, not the 404 an eligible caller would see.
	require.Equal(t, apperror.CodeConflict, joinAs(member.ID, "0999999999").Code)
	require.Equal(t, apperror.CodeConflict, joinAs(owner.ID, "0999999999").Code)
}

// TestJoinIgnoresGhostMembers: a member whose account was soft-deleted is
// gone for good — their leftover teachers row must not hold the owner's
// center hostage, and the center can still be retired around it.
func TestJoinIgnoresGhostMembers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	ghost, _ := testutil.Teacher(t, e.db)
	other, otherTeacher := testutil.Teacher(t, e.db)
	e.join(t, ghost.ID, owner.Phone)
	require.NoError(t, e.db.Delete(&teachers.Account{ID: ghost.ID}).Error)

	resp := e.join(t, owner.ID, other.Phone)
	require.Equal(t, otherTeacher.CenterID, resp.CenterID)

	// The old center is retired even though the ghost's teachers row still
	// points at it.
	var retired int64
	require.NoError(t, e.db.Table("centers").
		Where("id = ? AND deleted_at IS NOT NULL", ownerTeacher.CenterID).Count(&retired).Error)
	require.EqualValues(t, 1, retired)
}

// TestRetireCenterRefusesWhileTeachersRemain pins the row-level guard that
// keeps the join race from bricking accounts: a center with any live teacher
// still inside can never be retired, not even by its owner.
func TestRetireCenterRefusesWhileTeachersRemain(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	repo := centers.NewRepository(e.db)
	err := repo.SoftDeleteCenter(ctx, ownerTeacher.CenterID, owner.ID)
	require.ErrorIs(t, err, centers.ErrNotFound,
		"a center whose owner still lives in it must refuse retirement")
}

func TestMeShowsCenterAndMembers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ Trung Tâm"))
	member, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên B"))
	e.join(t, member.ID, owner.Phone)

	me, err := e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	require.Equal(t, ownerTeacher.CenterID, me.Center.ID)
	require.False(t, me.Center.IsOwner)
	require.Len(t, me.Members, 2)

	byID := map[uuid.UUID]centers.MemberResponse{}
	for _, m := range me.Members {
		byID[m.ID] = m
	}
	require.True(t, byID[owner.ID].IsOwner)
	require.False(t, byID[member.ID].IsOwner)
	require.Equal(t, owner.Phone, byID[owner.ID].Phone)
	require.Equal(t, "Chủ Trung Tâm", byID[owner.ID].FullName)

	ownerMe, err := e.centersSvc.Me(ctx, e.scope(t, owner.ID))
	require.NoError(t, err)
	require.True(t, ownerMe.Center.IsOwner)

	// A disabled member is a temporary lock, still part of the roster; a
	// soft-deleted account is gone for good and must disappear from it.
	disabledMember, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên Bị Khoá"))
	deletedMember, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên Đã Xoá"))
	e.join(t, disabledMember.ID, owner.Phone)
	e.join(t, deletedMember.ID, owner.Phone)
	require.NoError(t, e.db.Model(&teachers.Account{ID: disabledMember.ID}).Update("status", teachers.StatusDisabled).Error)
	require.NoError(t, e.db.Delete(&teachers.Account{ID: deletedMember.ID}).Error)

	me, err = e.centersSvc.Me(ctx, e.scope(t, owner.ID))
	require.NoError(t, err)
	require.Len(t, me.Members, 3)
	ids := map[uuid.UUID]bool{}
	for _, m := range me.Members {
		ids[m.ID] = true
	}
	require.True(t, ids[disabledMember.ID], "disabled member must stay on the roster")
	require.False(t, ids[deletedMember.ID], "soft-deleted account must leave the roster")
}

func TestRenameIsOwnerOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.Phone)

	err := e.centersSvc.Rename(ctx, e.scope(t, member.ID), centers.RenameRequest{Name: "Trung Tâm Bình Minh"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	require.NoError(t, e.centersSvc.Rename(ctx, e.scope(t, owner.ID), centers.RenameRequest{Name: "Trung Tâm Bình Minh"}))
	var reloaded centers.Center
	require.NoError(t, e.db.First(&reloaded, "id = ?", ownerTeacher.CenterID).Error)
	require.Equal(t, "Trung Tâm Bình Minh", reloaded.Name)
}

func TestRemoveMemberByOwnerDataStaysBehind(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên B"))
	e.join(t, member.ID, owner.Phone)
	classID := insertClass(t, e.db, member.ID, ownerTeacher.CenterID)

	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), member.ID))

	// The removed teacher owns a fresh personal center and is its live member.
	s := e.scope(t, member.ID)
	require.NotEqual(t, ownerTeacher.CenterID, s.CenterID)
	require.True(t, s.IsOwner)
	var personal centers.Center
	require.NoError(t, e.db.First(&personal, "id = ?", s.CenterID).Error)
	require.Equal(t, member.ID, personal.OwnerID)
	require.Equal(t, "Giáo Viên B", personal.Name)
	require.Equal(t, s.CenterID, e.liveMembership(t, member.ID).CenterID)

	// Their teaching history stays in the old center under their name.
	var stayed int64
	require.NoError(t, e.db.Table("classes").
		Where("id = ? AND center_id = ? AND teacher_id = ?", classID, ownerTeacher.CenterID, member.ID).
		Count(&stayed).Error)
	require.EqualValues(t, 1, stayed)
}

func TestMemberLeavesOnTheirOwn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.Phone)

	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, member.ID), member.ID))
	s := e.scope(t, member.ID)
	require.NotEqual(t, ownerTeacher.CenterID, s.CenterID)
	require.True(t, s.IsOwner)
}

func TestRemoveMemberAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	memberB, _ := testutil.Teacher(t, e.db)
	memberC, _ := testutil.Teacher(t, e.db)
	e.join(t, memberB.ID, owner.Phone)
	e.join(t, memberC.ID, owner.Phone)

	// A plain member cannot remove anyone but themselves.
	err := e.centersSvc.RemoveMember(ctx, e.scope(t, memberB.ID), memberC.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	// The owner cannot leave while the center still has members…
	err = e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), owner.ID)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	// …and cannot leave their own center even once it is empty.
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
// old center before the kick and the new personal center right after —
// without ever reissuing a token.
func TestKickIsEffectiveOnTheNextRequest(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.Phone)

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

	after, code := scopeVia(member.ID)
	require.Equal(t, http.StatusOK, code)
	require.NotEqual(t, ownerTeacher.CenterID, after.CenterID, "kick must bite on the very next request")
	require.True(t, after.IsOwner)

	// And a dead account stops at the middleware with a 401.
	require.NoError(t, e.db.Model(&teachers.Account{ID: member.ID}).Update("status", teachers.StatusDisabled).Error)
	_, code = scopeVia(member.ID)
	require.Equal(t, http.StatusUnauthorized, code)
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

	owner, ownerTeacher := testutil.Teacher(t, e.db, testutil.WithFullName("Chủ Trung Tâm"))
	member, _ := testutil.Teacher(t, e.db)
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

	// Join accepts the local phone form (0…) thanks to the vnphone binding +
	// normalization, and answers 201 with the new center id.
	localPhone := "0" + strings.TrimPrefix(owner.Phone, "+84")
	w := do(tokenFor(member.ID), http.MethodPost, "/api/v1/centers/join",
		`{"owner_phone":"`+localPhone+`"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var joined struct {
		Data centers.JoinResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &joined))
	require.Equal(t, ownerTeacher.CenterID, joined.Data.CenterID)

	// A malformed phone never reaches the service.
	w = do(tokenFor(owner.ID), http.MethodPost, "/api/v1/centers/join", `{"owner_phone":"12345"}`)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// GET /centers/me shows the shared roster to the new member.
	w = do(tokenFor(member.ID), http.MethodGet, "/api/v1/centers/me", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "Chủ Trung Tâm")

	// Rename is owner-only over HTTP too.
	w = do(tokenFor(member.ID), http.MethodPatch, "/api/v1/centers/me", `{"name":"Đổi Tên"}`)
	require.Equal(t, http.StatusForbidden, w.Code)

	// A non-UUID path param is a 404, same shape as an unknown member.
	w = do(tokenFor(owner.ID), http.MethodDelete, "/api/v1/centers/me/members/not-a-uuid", "")
	require.Equal(t, http.StatusNotFound, w.Code)

	// The owner removes the member: 204, no body.
	w = do(tokenFor(owner.ID), http.MethodDelete, "/api/v1/centers/me/members/"+member.ID.String(), "")
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// No token → 401 before any handler runs.
	w = do("", http.MethodGet, "/api/v1/centers/me", "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
}
