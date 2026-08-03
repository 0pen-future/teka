//go:build integration

package teachers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

func TestMeIsolationAndDeadAccounts(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	db := testutil.StartPostgres(t)
	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	issuer := auth.NewTokenIssuer(jwtCfg)

	r := gin.New()
	svc := teachers.NewService(teachers.NewRepository(db))
	teachers.RegisterRoutes(r.Group("/api/v1"), teachers.NewHandler(svc), middleware.RequireAuth(jwtCfg))

	get := func(token string) (*httptest.ResponseRecorder, string) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w, w.Body.String()
	}

	acctA, _ := testutil.Teacher(t, db, testutil.WithFullName("Teacher A"))
	acctB, _ := testutil.Teacher(t, db, testutil.WithFullName("Teacher B"))

	tokenA, err := issuer.IssueAccess(acctA.ID, authctx.RoleTeacher)
	require.NoError(t, err)
	tokenB, err := issuer.IssueAccess(acctB.ID, authctx.RoleTeacher)
	require.NoError(t, err)

	// Each token sees exactly its own profile — never the other tenant's.
	w, body := get(tokenA)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, acctA.Phone)
	require.Contains(t, body, "Teacher A")
	require.NotContains(t, body, acctB.Phone)
	require.NotContains(t, body, "Teacher B")

	w, body = get(tokenB)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, "Teacher B")
	require.NotContains(t, body, acctA.Phone)

	// Soft-deleting an account kills its still-valid access tokens here.
	require.NoError(t, db.Delete(&teachers.Account{ID: acctB.ID}).Error)
	w, body = get(tokenB)
	require.Equal(t, http.StatusUnauthorized, w.Code, "soft-deleted account token must 401, body: %s", body)

	// Disabling likewise.
	require.NoError(t, db.Model(&teachers.Account{ID: acctA.ID}).Update("status", teachers.StatusDisabled).Error)
	w, body = get(tokenA)
	require.Equal(t, http.StatusUnauthorized, w.Code, "disabled account token must 401, body: %s", body)
}

func TestUpdateMePersistsAgainstRealSQL(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	db := testutil.StartPostgres(t)
	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	issuer := auth.NewTokenIssuer(jwtCfg)

	r := gin.New()
	svc := teachers.NewService(teachers.NewRepository(db))
	teachers.RegisterRoutes(r.Group("/api/v1"), teachers.NewHandler(svc), middleware.RequireAuth(jwtCfg))

	acct, _ := testutil.Teacher(t, db)
	token, err := issuer.IssueAccess(acct.ID, authctx.RoleTeacher)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/me",
		strings.NewReader(`{"full_name":"Đổi Tên","timezone":"Asia/Bangkok"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var env struct {
		Data teachers.TeacherResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "Đổi Tên", env.Data.FullName)
	require.Equal(t, "Asia/Bangkok", env.Data.Timezone)

	var reloaded teachers.Teacher
	require.NoError(t, db.First(&reloaded, "id = ?", acct.ID).Error)
	require.Equal(t, "Đổi Tên", reloaded.FullName)
	require.Equal(t, "Asia/Bangkok", reloaded.Timezone)
}
