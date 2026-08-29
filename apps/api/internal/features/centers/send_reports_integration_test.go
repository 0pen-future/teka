//go:build integration

package centers_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/testutil"
)

// TestSendReportsScopeResolution pins how the flag travels into request
// scope: granted → true, revoked → false, and a closed stint resolves false
// even though the center_members row still exists.
func TestSendReportsScopeResolution(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)

	// Default off, for the owner and the member alike.
	require.False(t, e.scope(t, owner.ID).CanSendReports)
	require.True(t, e.scope(t, owner.ID).ReportsOversight(), "the owner has oversight without the flag")
	require.False(t, e.scope(t, member.ID).CanSendReports)
	require.False(t, e.scope(t, member.ID).ReportsOversight())

	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), member.ID, true))
	require.True(t, e.scope(t, member.ID).CanSendReports)
	require.True(t, e.scope(t, member.ID).ReportsOversight())

	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), member.ID, false))
	require.False(t, e.scope(t, member.ID).CanSendReports)

	// A granted member whose stint closes resolves false: the LEFT JOIN only
	// matches the live stint. The teachers.center_id pointer still aims at the
	// old center, which is exactly the state after RemoveMember.
	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), member.ID, true))
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND left_at IS NULL",
		member.ID).Error)
	require.False(t, e.scope(t, member.ID).CanSendReports,
		"a closed stint must not leak the permission into scope")
}

// TestSendReportsDoesNotSurviveRejoin pins the stint-scoped lifecycle:
// grant → close → reopen resolves false, whether the reset came from
// CloseMembership or from OpenMembership's upsert.
func TestSendReportsDoesNotSurviveRejoin(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)

	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), member.ID, true))

	// Close through the repository (the RemoveMember path) — the flag must
	// reset on the closed row itself, not only on a later reopen.
	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), member.ID))
	var closed centers.Member
	require.NoError(t, e.db.First(&closed, "teacher_id = ? AND center_id = ?",
		member.ID, ownerTeacher.CenterID).Error)
	require.False(t, closed.CanSendReports, "CloseMembership must reset the flag")

	// Reopen the same row via the accept flow's upsert; even a stale TRUE
	// planted directly on the closed row must not resurrect.
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET can_send_reports = TRUE WHERE teacher_id = ? AND center_id = ?",
		member.ID, ownerTeacher.CenterID).Error)
	require.NoError(t, e.centersSvc.OpenMembership(ctx, member.ID, ownerTeacher.CenterID))
	var reopened centers.Member
	require.NoError(t, e.db.First(&reopened, "teacher_id = ? AND center_id = ? AND left_at IS NULL",
		member.ID, ownerTeacher.CenterID).Error)
	require.False(t, reopened.CanSendReports, "OpenMembership must reset the flag on reopen")
}

// TestSetSendReportsAuthorizationMatrix pins who can grant to whom: only the
// owner grants, only an active non-owner member is a valid target, and every
// refused target collapses into the same not-found.
func TestSetSendReportsAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	memberB, _ := testutil.Teacher(t, e.db)
	memberC, _ := testutil.Teacher(t, e.db)
	e.join(t, memberB.ID, owner.ID)
	e.join(t, memberC.ID, owner.ID)

	// A plain member cannot grant — not to a peer, not to themselves.
	err := e.centersSvc.SetSendReports(ctx, e.scope(t, memberB.ID), memberC.ID, true)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	err = e.centersSvc.SetSendReports(ctx, e.scope(t, memberB.ID), memberB.ID, true)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	// The owner can never be the target — the flag is member-only.
	err = e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), owner.ID, true)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// A left member and a stranger are the same neutral not-found.
	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), memberC.ID))
	err = e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), memberC.ID, true)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	stranger, _ := testutil.Teacher(t, e.db)
	err = e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), stranger.ID, true)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), uuid.New(), true)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Revoke shares the same guard set.
	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), memberB.ID, true))
	err = e.centersSvc.SetSendReports(ctx, e.scope(t, memberB.ID), memberB.ID, false)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), memberB.ID, false))
}

// TestMeExposesSendReportsFlag pins the two /centers/me read shapes: the
// owner's roster carries per-member flags, a member sees their own.
func TestMeExposesSendReportsFlag(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), member.ID, true))

	ownerMe, err := e.centersSvc.Me(ctx, e.scope(t, owner.ID))
	require.NoError(t, err)
	me, ok := ownerMe.(*centers.MeResponse)
	require.True(t, ok)
	byID := map[uuid.UUID]centers.MemberResponse{}
	for _, m := range me.Members {
		byID[m.ID] = m
	}
	require.True(t, byID[member.ID].CanSendReports)
	require.False(t, byID[owner.ID].CanSendReports, "the owner never carries the flag")

	memberMe, err := e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	memberResp, ok := memberMe.(*centers.MemberMeResponse)
	require.True(t, ok)
	require.True(t, memberResp.CanSendReports)

	require.NoError(t, e.centersSvc.SetSendReports(ctx, e.scope(t, owner.ID), member.ID, false))
	memberMe, err = e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	require.False(t, memberMe.(*centers.MemberMeResponse).CanSendReports)
}

// TestSendReportsRoutesAndAudit drives the mounted grant/revoke routes with
// real bearer tokens through the real audit middleware: grant then revoke
// must land as two distinguishable audit rows with the owner as actor.
func TestSendReportsRoutesAndAudit(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	e := newEnv(t)

	sub := audit.NewSubscriber(audit.NewRepository(e.db), slog.New(slog.DiscardHandler), 1, time.Hour)
	bus := events.NewSync()
	bus.Subscribe("audit", 0, sub.Handle)

	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	issuer := auth.NewTokenIssuer(jwtCfg)
	r := gin.New()
	r.Use(middleware.RequestID())
	v1 := r.Group("/api/v1")
	v1.Use(middleware.RequestEvents(bus))
	centers.RegisterRoutes(v1, centers.NewHandler(e.centersSvc),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(e.centersSvc))

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	do := func(callerID uuid.UUID, method, path string) *httptest.ResponseRecorder {
		token, err := issuer.IssueAccess(callerID, authctx.RoleTeacher)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	memberPath := "/api/v1/centers/me/members/" + member.ID.String() + "/send-reports"

	// A plain member cannot grant themselves the permission.
	w := do(member.ID, http.MethodPost, memberPath)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// A non-UUID target is the same 404 as an unknown member.
	w = do(owner.ID, http.MethodPost, "/api/v1/centers/me/members/not-a-uuid/send-reports")
	require.Equal(t, http.StatusNotFound, w.Code)

	// The owner cannot be the target.
	w = do(owner.ID, http.MethodPost, "/api/v1/centers/me/members/"+owner.ID.String()+"/send-reports")
	require.Equal(t, http.StatusNotFound, w.Code)

	w = do(owner.ID, http.MethodPost, memberPath)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	require.True(t, e.scope(t, member.ID).CanSendReports)

	w = do(owner.ID, http.MethodDelete, memberPath)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	require.False(t, e.scope(t, member.ID).CanSendReports)

	sub.Close()
	// The middleware records denied attempts too (403/404 above); the two
	// successful mutations must land as distinguishable grant/revoke rows.
	var denied []audit.Log
	require.NoError(t, e.db.
		Find(&denied, "action LIKE 'center.member.send_reports%' AND status_code <> 204").Error)
	require.NotEmpty(t, denied, "denied attempts stay on the trail")
	var rows []audit.Log
	require.NoError(t, e.db.Order("occurred_at").
		Find(&rows, "action LIKE 'center.member.send_reports%' AND status_code = 204").Error)
	require.Len(t, rows, 2, "grant and revoke must each land exactly one row")
	require.Equal(t, "center.member.send_reports_grant", rows[0].Action)
	require.Equal(t, "center.member.send_reports_revoke", rows[1].Action)
	for _, row := range rows {
		require.NotNil(t, row.ActorUserID)
		require.Equal(t, owner.ID, *row.ActorUserID, "the owner is the actor")
		require.Equal(t, "teacher", row.EntityType)
		require.Equal(t, member.ID.String(), row.EntityID)
	}
}
