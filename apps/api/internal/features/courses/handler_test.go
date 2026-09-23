package courses

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

const handlerTestSecret = "courses-test-secret-0123456789abcdef"

// fakeScopeResolver resolves the owner id as the owner of the shared center
// and everyone else as a member holding the keys registered for them.
type fakeScopeResolver struct {
	center  uuid.UUID
	ownerID uuid.UUID
	perms   map[uuid.UUID][]string
}

func (f fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{
		TeacherID: teacherID, CenterID: f.center, IsOwner: teacherID == f.ownerID,
		Perms: authctx.BuildPermSet(nil, f.perms[teacherID], nil),
	}, nil
}

type httpDeps struct {
	*testDeps
	reader, editor uuid.UUID
}

func newHTTPTest(t *testing.T) (*gin.Engine, *httpDeps) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, deps := newTestService()
	d := &httpDeps{testDeps: deps, reader: uuid.New(), editor: uuid.New()}
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc),
		middleware.RequireAuth(jwtCfg),
		middleware.ResolveScope(fakeScopeResolver{center: deps.center, ownerID: deps.owner, perms: map[uuid.UUID][]string{
			d.reader: {authctx.PermCoursesRead, authctx.PermPathsRead},
			d.editor: {authctx.PermCoursesEdit},
		}}))
	return r, d
}

func mintToken(t *testing.T, subject uuid.UUID) string {
	t.Helper()
	claims := authctx.AccessClaims{
		Role: authctx.RoleTeacher,
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

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func do(t *testing.T, r *gin.Engine, method, path, body, token string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	if len(w.Body.Bytes()) > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
		}
	}
	return w, env
}

func decode[T any](t *testing.T, env envelope) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode data: %v\n%s", err, string(env.Data))
	}
	return out
}

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newHTTPTest(t)
	someID := uuid.NewString()
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/courses"},
		{http.MethodPost, "/api/v1/courses"},
		{http.MethodGet, "/api/v1/courses/" + someID},
		{http.MethodPut, "/api/v1/courses/" + someID},
		{http.MethodDelete, "/api/v1/courses/" + someID},
		{http.MethodPost, "/api/v1/courses/" + someID + "/archive"},
		{http.MethodPut, "/api/v1/courses/" + someID + "/tuition-packs"},
		{http.MethodGet, "/api/v1/courses/" + someID + "/paths"},
	}
	for _, rt := range routes {
		w, _ := do(t, r, rt.method, rt.path, "", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: want 401, got %d", rt.method, rt.path, w.Code)
		}
	}
}

func TestCourseLifecycleOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.owner)
	reader := mintToken(t, d.reader)
	editor := mintToken(t, d.editor)

	w, env := do(t, r, http.MethodPost, "/api/v1/courses", `{"code":"toan-6","name":"Toán 6","default_unit_price":150000}`, editor)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := decode[CourseResponse](t, env)
	if created.Code != "TOAN-6" || created.Status != StatusDraft {
		t.Fatalf("unexpected %+v", created)
	}
	if !strings.Contains(w.Body.String(), `"tuition_packs":[]`) || !strings.Contains(w.Body.String(), `"default_template":null`) {
		t.Fatalf("shape: %s", w.Body.String())
	}

	w, _ = do(t, r, http.MethodPost, "/api/v1/courses", `{"code":"TOAN-6","name":"Trùng"}`, owner)
	if w.Code != http.StatusConflict {
		t.Fatalf("clash: want 409, got %d", w.Code)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/courses", `{"code":"X1","name":"","default_unit_price":-1}`, owner)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["name"] == "" {
		t.Fatalf("binding: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/courses", `{"code":"X1","name":"x","status":"paused"}`, owner)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status: want 422, got %d", w.Code)
	}

	w, _ = do(t, r, http.MethodPost, "/api/v1/courses", `{"code":"B1","name":"b"}`, reader)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reader create: want 403, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/courses?status=draft&q=to", "", reader)
	if w.Code != http.StatusOK || len(decode[[]CourseResponse](t, env)) != 1 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodGet, "/api/v1/courses?status=paused", "", reader)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status filter: want 422, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodPut, "/api/v1/courses/"+created.ID.String()+"/tuition-packs",
		`[{"name":"Gói 12","sessions":12,"price":1200000},{"name":"Gói 24","sessions":24,"price":2200000}]`, editor)
	if w.Code != http.StatusOK || len(decode[[]TuitionPackResponse](t, env)) != 2 {
		t.Fatalf("packs: %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/courses/"+created.ID.String()+"/tuition-packs",
		`[{"name":"ok","sessions":1},{"name":"","sessions":0}]`, editor)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["1.name"] == "" {
		t.Fatalf("pack element errors: %d %s", w.Code, w.Body.String())
	}

	w, env = do(t, r, http.MethodPut, "/api/v1/courses/"+created.ID.String(), `{"code":"TOAN-6","name":"Toán 6 nâng cao","status":"active"}`, editor)
	if w.Code != http.StatusOK || decode[CourseResponse](t, env).Status != StatusActive {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/courses/"+created.ID.String()+"/archive", "", editor)
	if w.Code != http.StatusOK || decode[CourseResponse](t, env).Status != StatusArchived {
		t.Fatalf("archive: %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/courses/"+created.ID.String()+"/archive", "", editor)
	if w.Code != http.StatusConflict || env.Error.Code != CodeCourseArchived {
		t.Fatalf("second archive: %d %s", w.Code, w.Body.String())
	}

	d.repo.classes[created.ID] = fakeClassCount{running: 1}
	w, env = do(t, r, http.MethodDelete, "/api/v1/courses/"+created.ID.String(), "", editor)
	if w.Code != http.StatusConflict || env.Error.Code != CodeCourseInUse {
		t.Fatalf("delete in use: %d %s", w.Code, w.Body.String())
	}
	delete(d.repo.classes, created.ID)

	w, _ = do(t, r, http.MethodGet, "/api/v1/courses/"+created.ID.String()+"/paths", "", reader)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("paths of a course outside every path: %d %s", w.Code, w.Body.String())
	}
	// courses.edit alone does not open the learning paths.
	w, _ = do(t, r, http.MethodGet, "/api/v1/courses/"+created.ID.String()+"/paths", "", editor)
	if w.Code != http.StatusForbidden {
		t.Fatalf("editor without paths.read: want 403, got %d", w.Code)
	}
	d.repo.paths[created.ID] = []CoursePath{{ID: uuid.New(), Code: "LT-6", Name: "Lộ trình 6", Status: "active",
		StageID: uuid.New(), StageName: "Nền tảng", StagePosition: 1}}
	w, env = do(t, r, http.MethodGet, "/api/v1/courses/"+created.ID.String()+"/paths", "", reader)
	if w.Code != http.StatusOK || len(decode[[]CoursePathResponse](t, env)) != 1 || !strings.Contains(w.Body.String(), `"stage_position":1`) {
		t.Fatalf("paths of a course: %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodDelete, "/api/v1/courses/"+created.ID.String(), "", editor)
	if w.Code != http.StatusConflict || env.Error.Code != CodeCourseInPath {
		t.Fatalf("delete while in a path: %d %s", w.Code, w.Body.String())
	}
	delete(d.repo.paths, created.ID)
	w, _ = do(t, r, http.MethodDelete, "/api/v1/courses/"+created.ID.String(), "", editor)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodGet, "/api/v1/courses/"+created.ID.String(), "", reader)
	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: want 404, got %d", w.Code)
	}
	w, _ = do(t, r, http.MethodGet, "/api/v1/courses/not-a-uuid", "", reader)
	if w.Code != http.StatusNotFound {
		t.Fatalf("malformed id: want 404, got %d", w.Code)
	}
}

var _ = apperror.NotFound
