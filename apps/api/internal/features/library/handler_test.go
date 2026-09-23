package library

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

const handlerTestSecret = "library-test-secret-0123456789abcdef"

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
			d.reader: {authctx.PermLibraryRead},
			d.editor: {authctx.PermLibraryEdit},
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
		{http.MethodGet, "/api/v1/library/templates"},
		{http.MethodPost, "/api/v1/library/templates"},
		{http.MethodGet, "/api/v1/library/templates/" + someID},
		{http.MethodPut, "/api/v1/library/templates/" + someID},
		{http.MethodDelete, "/api/v1/library/templates/" + someID},
		{http.MethodGet, "/api/v1/library/templates/" + someID + "/versions"},
		{http.MethodPost, "/api/v1/library/templates/" + someID + "/versions"},
		{http.MethodPost, "/api/v1/library/versions/" + someID + "/publish"},
		{http.MethodPost, "/api/v1/library/versions/" + someID + "/archive"},
		{http.MethodGet, "/api/v1/library/versions/" + someID + "/lessons"},
		{http.MethodPost, "/api/v1/library/versions/" + someID + "/lessons"},
		{http.MethodPut, "/api/v1/library/versions/" + someID + "/lessons/order"},
		{http.MethodGet, "/api/v1/library/lessons/" + someID},
		{http.MethodPut, "/api/v1/library/lessons/" + someID},
		{http.MethodDelete, "/api/v1/library/lessons/" + someID},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestTemplateLifecycleOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.owner)

	w, env := do(t, r, http.MethodPost, "/api/v1/library/templates",
		`{"code":"toan-6","name":"Toán 6","subject":"Toán","level":"Lớp 6"}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d %+v", w.Code, env)
	}
	tpl := decode[TemplateResponse](t, env)
	if tpl.Code != "TOAN-6" || tpl.DraftVersionNo == nil || *tpl.DraftVersionNo != 1 {
		t.Fatalf("unexpected template %+v", tpl)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates?q=toan&sort=-created_at", "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d %+v", w.Code, env)
	}
	if list := decode[[]TemplateResponse](t, env); len(list) != 1 || list[0].ID != tpl.ID {
		t.Fatalf("list must contain the template, got %+v", list)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates/"+tpl.ID.String()+"/versions", "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("versions: want 200, got %d %+v", w.Code, env)
	}
	versions := decode[[]VersionResponse](t, env)
	if len(versions) != 1 || versions[0].Status != StatusDraft {
		t.Fatalf("want one draft, got %+v", versions)
	}
	vid := versions[0].ID.String()

	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/lessons", `{"title":"Buổi 1","duration_min":90}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("lesson 1: want 201, got %d %+v", w.Code, env)
	}
	l1 := decode[LessonResponse](t, env)
	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/lessons", `{"title":"Buổi 2"}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("lesson 2: want 201, got %d %+v", w.Code, env)
	}
	l2 := decode[LessonResponse](t, env)

	w, env = do(t, r, http.MethodPut, "/api/v1/library/versions/"+vid+"/lessons/order",
		`{"lesson_ids":["`+l2.ID.String()+`","`+l1.ID.String()+`"]}`, owner)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder: want 200, got %d %+v", w.Code, env)
	}
	if ordered := decode[[]LessonResponse](t, env); ordered[0].ID != l2.ID || ordered[0].Position != 1 {
		t.Fatalf("reorder result %+v", ordered)
	}

	w, env = do(t, r, http.MethodPut, "/api/v1/library/lessons/"+l1.ID.String(), `{"title":"Buổi 1 (sửa)","homework_note":"Làm bài 1-5"}`, owner)
	if w.Code != http.StatusOK || decode[LessonResponse](t, env).Title != "Buổi 1 (sửa)" {
		t.Fatalf("update lesson: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/lessons/"+l1.ID.String(), "", owner)
	if w.Code != http.StatusOK || decode[LessonResponse](t, env).Position != 2 {
		t.Fatalf("get lesson: got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/publish", "", owner)
	if w.Code != http.StatusOK || decode[VersionResponse](t, env).Status != StatusPublished {
		t.Fatalf("publish: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/lessons", `{"title":"Buổi 3"}`, owner)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != CodeVersionLocked {
		t.Fatalf("lesson on published version: want 409 VERSION_LOCKED, got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodDelete, "/api/v1/library/lessons/"+l2.ID.String(), "", owner)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete lesson on published version: want 409, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/library/templates/"+tpl.ID.String()+"/versions", `{"changelog":"Bổ sung"}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("v2: want 201, got %d %+v", w.Code, env)
	}
	v2 := decode[VersionResponse](t, env)
	if v2.VersionNo != 2 || v2.LessonCount != 2 {
		t.Fatalf("v2 must copy the two lessons, got %+v", v2)
	}
	// A body-less POST is fine too once v2 is out of the way.
	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+v2.ID.String()+"/publish", "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("publish v2: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/library/templates/"+tpl.ID.String()+"/versions", "", owner)
	if w.Code != http.StatusCreated || decode[VersionResponse](t, env).VersionNo != 3 {
		t.Fatalf("v3 without body: got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/archive", "", owner)
	if w.Code != http.StatusOK || decode[VersionResponse](t, env).Status != StatusArchived {
		t.Fatalf("archive v1: got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPut, "/api/v1/library/templates/"+tpl.ID.String(), `{"code":"TOAN-6","name":"Toán 6 mới"}`, owner)
	if w.Code != http.StatusOK || decode[TemplateResponse](t, env).Name != "Toán 6 mới" {
		t.Fatalf("update template: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodDelete, "/api/v1/library/templates/"+tpl.ID.String(), "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("delete template: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates/"+tpl.ID.String(), "", owner)
	if w.Code != http.StatusNotFound {
		t.Fatalf("deleted template must be 404, got %d %+v", w.Code, env)
	}
}

func TestValidationAndPathErrors(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.owner)

	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"empty template body", http.MethodPost, "/api/v1/library/templates", `{}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"bad code shape", http.MethodPost, "/api/v1/library/templates", `{"code":"mã lớp","name":"x"}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"malformed json", http.MethodPost, "/api/v1/library/templates", `{"code":`, http.StatusBadRequest, apperror.CodeBadRequest},
		{"malformed template id", http.MethodGet, "/api/v1/library/templates/not-a-uuid", "", http.StatusNotFound, apperror.CodeNotFound},
		{"unknown template", http.MethodGet, "/api/v1/library/templates/" + uuid.NewString(), "", http.StatusNotFound, apperror.CodeNotFound},
		{"unknown version", http.MethodPost, "/api/v1/library/versions/" + uuid.NewString() + "/publish", "", http.StatusNotFound, apperror.CodeNotFound},
		{"malformed lesson id", http.MethodDelete, "/api/v1/library/lessons/nope", "", http.StatusNotFound, apperror.CodeNotFound},
		{"lesson without title", http.MethodPost, "/api/v1/library/versions/" + uuid.NewString() + "/lessons", `{"duration_min":30}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"lesson duration out of range", http.MethodPut, "/api/v1/library/lessons/" + uuid.NewString(), `{"title":"x","duration_min":0}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"reorder empty", http.MethodPut, "/api/v1/library/versions/" + uuid.NewString() + "/lessons/order", `{"lesson_ids":[]}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, env := do(t, r, tc.method, tc.path, tc.body, owner)
			if w.Code != tc.status || env.Error == nil || env.Error.Code != tc.code {
				t.Fatalf("want %d %s, got %d %+v", tc.status, tc.code, w.Code, env)
			}
		})
	}
}

func TestPermissionsOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner, reader, editor := mintToken(t, d.owner), mintToken(t, d.reader), mintToken(t, d.editor)

	w, env := do(t, r, http.MethodPost, "/api/v1/library/templates", `{"code":"CT01","name":"Toán 6"}`, editor)
	if w.Code != http.StatusCreated {
		t.Fatalf("editor create: got %d %+v", w.Code, env)
	}
	tpl := decode[TemplateResponse](t, env)
	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates/"+tpl.ID.String()+"/versions", "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("reader versions: got %d %+v", w.Code, env)
	}
	vid := decode[[]VersionResponse](t, env)[0].ID.String()

	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/lessons", `{"title":"Buổi 1"}`, reader)
	if w.Code != http.StatusForbidden || env.Error == nil || env.Error.Code != apperror.CodeForbidden {
		t.Fatalf("reader must not add lessons: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/publish", "", editor)
	if w.Code != http.StatusForbidden {
		t.Fatalf("editor must not publish: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/publish", "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("owner publish: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/versions/"+vid+"/lessons", "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("reader lists lessons: got %d %+v", w.Code, env)
	}
	stranger := mintToken(t, uuid.New())
	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates", "", stranger)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member without library.read: got %d %+v", w.Code, env)
	}
}
