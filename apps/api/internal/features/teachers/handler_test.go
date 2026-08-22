package teachers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

const handlerTestSecret = "teachers-test-secret-0123456789abcdef"

// fakeScopeResolver resolves scopes off the in-memory repository the way the
// centers service does off the database: only a live, active account gets one.
type fakeScopeResolver struct {
	repo *fakeRepository
}

func (f *fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	p, ok := f.repo.profiles[teacherID]
	if !ok || p.Account.Status != StatusActive {
		return authctx.Scope{}, apperror.Unauthorized("account is not active")
	}
	return authctx.Scope{TeacherID: teacherID, CenterID: p.Teacher.CenterID, IsOwner: true}, nil
}

// newTeachersHTTPTest wires the real routes, auth, and scope middleware over
// the in-memory fake repository.
func newTeachersHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(NewService(repo)),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(&fakeScopeResolver{repo: repo}))
	return r, repo
}

// mintToken signs an access token the same way the auth issuer does, without
// importing features/auth (which imports this package).
func mintToken(t *testing.T, subject uuid.UUID, role string) string {
	t.Helper()
	claims := authctx.AccessClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(handlerTestSecret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

type meEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func doMe(t *testing.T, r *gin.Engine, method, body, token string) (*httptest.ResponseRecorder, meEnvelope) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/me", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env meEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
	}
	return w, env
}

func TestMeRequiresToken(t *testing.T) {
	r, _ := newTeachersHTTPTest(t)

	w, env := doMe(t, r, http.MethodGet, "", "")
	if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("want 401 UNAUTHORIZED, got %d %+v", w.Code, env)
	}
}

func TestMeReturnsOwnProfile(t *testing.T) {
	r, repo := newTeachersHTTPTest(t)
	p := seedProfile(repo, "+84901234567")

	w, env := doMe(t, r, http.MethodGet, "", mintToken(t, p.Account.ID, authctx.RoleTeacher))
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var body TeacherResponse
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != p.Account.ID || body.Phone != "+84901234567" || body.FullName != "Before" {
		t.Fatalf("unexpected profile: %+v", body)
	}
}

func TestMeRejectsMissingDisabledOrNonTeacherAccounts(t *testing.T) {
	r, repo := newTeachersHTTPTest(t)
	disabled := seedProfile(repo, "+84901234567")
	disabled.Account.Status = StatusDisabled

	cases := map[string]string{
		// A valid token whose account no longer exists (deleted after issue).
		"vanished account": mintToken(t, uuid.New(), authctx.RoleTeacher),
		// A valid token whose account was disabled after issue.
		"disabled account": mintToken(t, disabled.Account.ID, authctx.RoleTeacher),
		// A token carrying a non-teacher role never reaches teacher data.
		"non-teacher role": mintToken(t, disabled.Account.ID, authctx.RoleParent),
	}
	for name, token := range cases {
		w, env := doMe(t, r, http.MethodGet, "", token)
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s: want 401 UNAUTHORIZED, got %d %+v", name, w.Code, env)
		}
	}
}

func TestUpdateMeChangesProfileAndIgnoresOtherFields(t *testing.T) {
	r, repo := newTeachersHTTPTest(t)
	p := seedProfile(repo, "+84901234567")
	token := mintToken(t, p.Account.ID, authctx.RoleTeacher)

	// status/role/phone in the body must be ignored, not persisted.
	w, env := doMe(t, r, http.MethodPut,
		`{"full_name":"After","timezone":"Asia/Bangkok","status":"disabled","role":"admin","phone":"+84999999999"}`,
		token)
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var body TeacherResponse
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.FullName != "After" || body.Timezone != "Asia/Bangkok" {
		t.Fatalf("update not applied: %+v", body)
	}
	stored := repo.profiles[p.Account.ID]
	if stored.Account.Status != StatusActive || stored.Account.Phone != "+84901234567" {
		t.Fatalf("body must not mass-assign account fields: %+v", stored.Account)
	}
}

func TestUpdateMeValidation(t *testing.T) {
	r, repo := newTeachersHTTPTest(t)
	p := seedProfile(repo, "+84901234567")
	token := mintToken(t, p.Account.ID, authctx.RoleTeacher)

	w, env := doMe(t, r, http.MethodPut, `{"full_name":"","timezone":""}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("want 422 VALIDATION_ERROR, got %d %+v", w.Code, env)
	}

	w, env = doMe(t, r, http.MethodPut, `{"full_name":"X","timezone":"Mars/Olympus"}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["timezone"] == "" {
		t.Fatalf("want 422 with timezone field message, got %d %+v", w.Code, env)
	}
}
