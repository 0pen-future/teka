package paths

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

const handlerTestSecret = "paths-test-secret-0123456789abcdef"

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
			d.reader: {authctx.PermPathsRead},
			d.editor: {authctx.PermPathsEdit},
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
		{http.MethodGet, "/api/v1/paths"},
		{http.MethodPost, "/api/v1/paths"},
		{http.MethodGet, "/api/v1/paths/" + someID},
		{http.MethodPut, "/api/v1/paths/" + someID},
		{http.MethodDelete, "/api/v1/paths/" + someID},
		{http.MethodPost, "/api/v1/paths/" + someID + "/stages"},
		{http.MethodPut, "/api/v1/paths/" + someID + "/stages/order"},
		{http.MethodPut, "/api/v1/paths/" + someID + "/stages/" + someID},
		{http.MethodDelete, "/api/v1/paths/" + someID + "/stages/" + someID},
		{http.MethodPut, "/api/v1/paths/" + someID + "/stages/" + someID + "/courses"},
	}
	for _, rt := range routes {
		w, _ := do(t, r, rt.method, rt.path, "", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: want 401, got %d", rt.method, rt.path, w.Code)
		}
	}
}

func TestPathLifecycleOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.owner)
	reader := mintToken(t, d.reader)
	editor := mintToken(t, d.editor)

	w, env := do(t, r, http.MethodPost, "/api/v1/paths", `{"code":"lt-6","name":"Lộ trình lớp 6"}`, editor)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := decode[PathResponse](t, env)
	if created.Code != "LT-6" || created.Status != StatusDraft {
		t.Fatalf("unexpected %+v", created)
	}
	if !strings.Contains(w.Body.String(), `"stages":[]`) || !strings.Contains(w.Body.String(), `"stage_count":0`) {
		t.Fatalf("shape: %s", w.Body.String())
	}
	base := "/api/v1/paths/" + created.ID.String()

	w, _ = do(t, r, http.MethodPost, "/api/v1/paths", `{"code":"LT-6","name":"Trùng"}`, owner)
	if w.Code != http.StatusConflict {
		t.Fatalf("clash: want 409, got %d", w.Code)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/paths", `{"code":"X1","name":""}`, owner)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["name"] == "" {
		t.Fatalf("binding: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/paths", `{"code":"X1","name":"x","status":"paused"}`, owner)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status: want 422, got %d", w.Code)
	}
	w, _ = do(t, r, http.MethodPost, "/api/v1/paths", `{"code":"B1","name":"b"}`, reader)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reader create: want 403, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/paths?status=draft&q=lt", "", reader)
	if w.Code != http.StatusOK || len(decode[[]PathResponse](t, env)) != 1 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodGet, "/api/v1/paths?status=paused", "", reader)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status filter: want 422, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodPost, base+"/stages", `{"name":"Nền tảng"}`, editor)
	if w.Code != http.StatusCreated || len(decode[PathResponse](t, env).Stages) != 1 {
		t.Fatalf("create stage: %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodPost, base+"/stages", `{"name":"Nâng cao","goal":"Thi tốt"}`, editor)
	if w.Code != http.StatusCreated {
		t.Fatalf("create second stage: %d %s", w.Code, w.Body.String())
	}
	p := decode[PathResponse](t, env)
	s1, s2 := p.Stages[0].ID.String(), p.Stages[1].ID.String()
	w, _ = do(t, r, http.MethodPost, base+"/stages", `{"name":""}`, editor)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("stage binding: want 422, got %d", w.Code)
	}
	w, _ = do(t, r, http.MethodPost, base+"/stages", `{"name":"x"}`, reader)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reader create stage: want 403, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodPut, base+"/stages/order", `{"stage_ids":["`+s2+`","`+s1+`"]}`, editor)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder: %d %s", w.Code, w.Body.String())
	}
	if p = decode[PathResponse](t, env); p.Stages[0].ID.String() != s2 || p.Stages[0].Position != 1 {
		t.Fatalf("reorder not applied: %+v", p.Stages)
	}
	w, env = do(t, r, http.MethodPut, base+"/stages/order", `{"stage_ids":[]}`, editor)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["stage_ids"] == "" {
		t.Fatalf("empty reorder: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodPut, base+"/stages/order", `{"stage_ids":["`+s1+`"]}`, editor)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("partial reorder: want 422, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodPut, base+"/stages/"+s2, `{"name":"Nâng cao hơn","goal":null}`, editor)
	if w.Code != http.StatusOK || decode[PathResponse](t, env).Stages[0].Name != "Nâng cao hơn" {
		t.Fatalf("update stage: %d %s", w.Code, w.Body.String())
	}

	toan := d.repo.course(d.center, "TOAN-6")
	w, env = do(t, r, http.MethodPut, base+"/stages/"+s2+"/courses", `{"course_ids":["`+toan.String()+`"]}`, editor)
	if w.Code != http.StatusOK {
		t.Fatalf("set courses: %d %s", w.Code, w.Body.String())
	}
	if p = decode[PathResponse](t, env); len(p.Stages[0].Courses) != 1 || p.Stages[0].Courses[0].Code != "TOAN-6" || p.CourseCount != 1 {
		t.Fatalf("courses not applied: %+v", p)
	}
	w, env = do(t, r, http.MethodPut, base+"/stages/"+s2+"/courses", `{}`, editor)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["course_ids"] == "" {
		t.Fatalf("missing course_ids: %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodPut, base+"/stages/"+s2+"/courses", `{"course_ids":["`+uuid.NewString()+`"]}`, editor)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["course_ids"] == "" {
		t.Fatalf("unknown course: %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodPut, base+"/stages/"+s2+"/courses", `{"course_ids":[]}`, editor)
	if w.Code != http.StatusOK || len(decode[PathResponse](t, env).Stages[0].Courses) != 0 {
		t.Fatalf("clear courses: %d %s", w.Code, w.Body.String())
	}

	w, env = do(t, r, http.MethodDelete, base+"/stages/"+s2, "", editor)
	if w.Code != http.StatusOK || len(decode[PathResponse](t, env).Stages) != 1 {
		t.Fatalf("delete stage: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodDelete, base+"/stages/"+s2, "", editor)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete stage twice: want 404, got %d", w.Code)
	}
	w, _ = do(t, r, http.MethodPut, base+"/stages/not-a-uuid", `{"name":"x"}`, editor)
	if w.Code != http.StatusNotFound {
		t.Fatalf("malformed stage id: want 404, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodPut, base, `{"code":"LT-6","name":"Lộ trình lớp 6 mới","status":"active"}`, editor)
	if w.Code != http.StatusOK || decode[PathResponse](t, env).Status != StatusActive {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodDelete, base, "", reader)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reader delete: want 403, got %d", w.Code)
	}
	w, _ = do(t, r, http.MethodDelete, base, "", editor)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	w, _ = do(t, r, http.MethodGet, base, "", reader)
	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: want 404, got %d", w.Code)
	}
	w, _ = do(t, r, http.MethodGet, "/api/v1/paths/not-a-uuid", "", reader)
	if w.Code != http.StatusNotFound {
		t.Fatalf("malformed id: want 404, got %d", w.Code)
	}
}

var _ = apperror.NotFound
