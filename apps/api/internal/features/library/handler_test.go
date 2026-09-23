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
	reader, editor, assigner uuid.UUID
}

func newHTTPTest(t *testing.T) (*gin.Engine, *httpDeps) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, deps := newTestService()
	d := &httpDeps{testDeps: deps, reader: uuid.New(), editor: uuid.New(), assigner: uuid.New()}
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc),
		middleware.RequireAuth(jwtCfg),
		middleware.ResolveScope(fakeScopeResolver{center: deps.center, ownerID: deps.owner, perms: map[uuid.UUID][]string{
			d.reader:   {authctx.PermLibraryRead},
			d.editor:   {authctx.PermLibraryEdit},
			d.assigner: {authctx.PermPrepAssign},
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
		{http.MethodGet, "/api/v1/library/versions/" + someID},
		{http.MethodGet, "/api/v1/library/versions/" + someID + "/board"},
		{http.MethodPatch, "/api/v1/library/lessons/" + someID + "/prep"},
		{http.MethodPatch, "/api/v1/library/lessons/" + someID + "/assignment"},
		{http.MethodPut, "/api/v1/library/versions/" + someID + "/log-fields"},
		{http.MethodPut, "/api/v1/library/versions/" + someID + "/score-set"},
		{http.MethodPut, "/api/v1/library/lessons/" + someID + "/materials"},
		{http.MethodPut, "/api/v1/library/lessons/" + someID + "/exercises"},
		{http.MethodGet, "/api/v1/library/materials"},
		{http.MethodPost, "/api/v1/library/materials"},
		{http.MethodGet, "/api/v1/library/materials/" + someID},
		{http.MethodPut, "/api/v1/library/materials/" + someID},
		{http.MethodDelete, "/api/v1/library/materials/" + someID},
		{http.MethodGet, "/api/v1/library/exercises"},
		{http.MethodPost, "/api/v1/library/exercises"},
		{http.MethodGet, "/api/v1/library/exercises/" + someID},
		{http.MethodPut, "/api/v1/library/exercises/" + someID},
		{http.MethodDelete, "/api/v1/library/exercises/" + someID},
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

func TestItemsAndAttachmentsOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.owner)
	reader := mintToken(t, d.reader)

	w, env := do(t, r, http.MethodPost, "/api/v1/library/templates", `{"code":"ly-8","name":"Lý 8"}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("create template: %d %+v", w.Code, env)
	}
	tpl := decode[TemplateResponse](t, env)
	_, env = do(t, r, http.MethodGet, "/api/v1/library/templates/"+tpl.ID.String()+"/versions", "", owner)
	vid := decode[[]VersionResponse](t, env)[0].ID.String()
	w, env = do(t, r, http.MethodPost, "/api/v1/library/versions/"+vid+"/lessons", `{"title":"Buổi 1"}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("create lesson: %d %+v", w.Code, env)
	}
	lid := decode[LessonResponse](t, env).ID.String()

	// Materials and exercises: create, list with filter, update, get.
	w, env = do(t, r, http.MethodPost, "/api/v1/library/materials",
		`{"title":"Video Ôm","kind":"video","url":"https://example.com/om","tags":[" điện ","điện",""]}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("create material: %d %+v", w.Code, env)
	}
	mat := decode[MaterialResponse](t, env)
	if len(mat.Tags) != 1 || mat.Tags[0] != "điện" {
		t.Fatalf("tags must be cleaned, got %+v", mat.Tags)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/library/materials", `{"title":"Slide","kind":"doc"}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("create material 2: %d %+v", w.Code, env)
	}
	mat2 := decode[MaterialResponse](t, env)
	w, env = do(t, r, http.MethodGet, "/api/v1/library/materials?q=video&sort=-created_at", "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("list materials: %d %+v", w.Code, env)
	}
	if list := decode[[]MaterialResponse](t, env); len(list) != 1 || list[0].ID != mat.ID {
		t.Fatalf("filtered list %+v", list)
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/library/materials/"+mat.ID.String(),
		`{"title":"Video định luật Ôm","kind":"video","url":"https://example.com/om"}`, owner)
	if w.Code != http.StatusOK || decode[MaterialResponse](t, env).Title != "Video định luật Ôm" {
		t.Fatalf("update material: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/materials/"+mat.ID.String(), "", reader)
	if w.Code != http.StatusOK || decode[MaterialResponse](t, env).Tags == nil {
		t.Fatalf("get material: %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/library/exercises", `{"title":"Bài tập 1","difficulty":3}`, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("create exercise: %d %+v", w.Code, env)
	}
	ex := decode[ExerciseResponse](t, env)
	w, env = do(t, r, http.MethodGet, "/api/v1/library/exercises", "", reader)
	if w.Code != http.StatusOK || len(decode[[]ExerciseResponse](t, env)) != 1 {
		t.Fatalf("list exercises: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/library/exercises/"+ex.ID.String(), `{"title":"Bài tập 1","difficulty":5}`, owner)
	if w.Code != http.StatusOK || *decode[ExerciseResponse](t, env).Difficulty != 5 {
		t.Fatalf("update exercise: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/exercises/"+ex.ID.String(), "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("get exercise: %d %+v", w.Code, env)
	}

	// Attach as bare arrays; lesson detail exposes the links.
	w, env = do(t, r, http.MethodPut, "/api/v1/library/lessons/"+lid+"/materials",
		`[{"material_id":"`+mat2.ID.String()+`","shared_with_students":true},{"material_id":"`+mat.ID.String()+`"}]`, owner)
	if w.Code != http.StatusOK {
		t.Fatalf("set materials: %d %+v", w.Code, env)
	}
	links := decode[[]LessonMaterialResponse](t, env)
	if len(links) != 2 || links[0].ID != mat2.ID || !links[0].SharedWithStudents || links[1].Position != 2 {
		t.Fatalf("material links %+v", links)
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/library/lessons/"+lid+"/exercises",
		`[{"exercise_id":"`+ex.ID.String()+`"}]`, owner)
	if w.Code != http.StatusOK || len(decode[[]LessonExerciseResponse](t, env)) != 1 {
		t.Fatalf("set exercises: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/lessons/"+lid, "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("get lesson: %d %+v", w.Code, env)
	}
	if detail := decode[LessonDetailResponse](t, env); len(detail.Materials) != 2 || len(detail.Exercises) != 1 {
		t.Fatalf("lesson detail %+v", detail)
	}

	// Linked items cannot be deleted; unlink then delete works.
	w, env = do(t, r, http.MethodDelete, "/api/v1/library/materials/"+mat.ID.String(), "", owner)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != CodeMaterialInUse {
		t.Fatalf("delete linked material: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodDelete, "/api/v1/library/exercises/"+ex.ID.String(), "", owner)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != CodeExerciseInUse {
		t.Fatalf("delete linked exercise: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/library/lessons/"+lid+"/exercises", `[]`, owner)
	if w.Code != http.StatusOK || len(decode[[]LessonExerciseResponse](t, env)) != 0 {
		t.Fatalf("clear exercises: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodDelete, "/api/v1/library/exercises/"+ex.ID.String(), "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("delete exercise: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/exercises/"+ex.ID.String(), "", owner)
	if w.Code != http.StatusNotFound {
		t.Fatalf("deleted exercise must be 404: %d %+v", w.Code, env)
	}

	// Log fields and score set on the version, surfaced by the version detail.
	w, env = do(t, r, http.MethodPut, "/api/v1/library/versions/"+vid+"/log-fields",
		`[{"label":"Mức độ hiểu bài","kind":"select","options":["Tốt","Khá"],"required":true},{"label":"Ghi chú","kind":"text","options":["bỏ"]}]`, owner)
	if w.Code != http.StatusOK {
		t.Fatalf("set log fields: %d %+v", w.Code, env)
	}
	fields := decode[[]LogFieldResponse](t, env)
	if len(fields) != 2 || fields[0].Position != 1 || len(fields[0].Options) != 2 || len(fields[1].Options) != 0 {
		t.Fatalf("log fields %+v", fields)
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/library/versions/"+vid+"/score-set",
		`[{"key":"mid","label":"Giữa kỳ","max":10,"weight":0.4},{"key":"final","label":"Cuối kỳ","max":10,"weight":0.6}]`, owner)
	if w.Code != http.StatusOK || len(decode[[]ScoreComponent](t, env)) != 2 {
		t.Fatalf("set score set: %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/versions/"+vid, "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("get version: %d %+v", w.Code, env)
	}
	version := decode[VersionDetailResponse](t, env)
	if len(version.ScoreSet) != 2 || len(version.LogFields) != 2 || len(version.Lessons) != 1 || len(version.Lessons[0].Materials) != 2 {
		t.Fatalf("version detail %+v", version)
	}

	// Readers cannot write anything new.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/library/materials", `{"title":"x","kind":"link"}`},
		{http.MethodPut, "/api/v1/library/lessons/" + lid + "/materials", `[]`},
		{http.MethodPut, "/api/v1/library/versions/" + vid + "/log-fields", `[]`},
		{http.MethodPut, "/api/v1/library/versions/" + vid + "/score-set", `[]`},
		{http.MethodDelete, "/api/v1/library/materials/" + mat2.ID.String(), ""},
	} {
		if w, env := do(t, r, tc.method, tc.path, tc.body, reader); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: reader must get 403, got %d %+v", tc.method, tc.path, w.Code, env)
		}
	}
}

func TestItemValidationOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.owner)
	someID := uuid.NewString()

	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"material without kind", http.MethodPost, "/api/v1/library/materials", `{"title":"x"}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"material bad kind", http.MethodPost, "/api/v1/library/materials", `{"title":"x","kind":"pdf"}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"material script url", http.MethodPost, "/api/v1/library/materials", `{"title":"x","kind":"link","url":"javascript:alert(1)"}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"exercise difficulty out of range", http.MethodPost, "/api/v1/library/exercises", `{"title":"x","difficulty":6}`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"materials body not an array", http.MethodPut, "/api/v1/library/lessons/" + someID + "/materials", `{"material_id":"` + someID + `"}`, http.StatusBadRequest, apperror.CodeBadRequest},
		{"materials element without id", http.MethodPut, "/api/v1/library/lessons/" + someID + "/materials", `[{"shared_with_students":true}]`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"log field bad kind", http.MethodPut, "/api/v1/library/versions/" + someID + "/log-fields", `[{"label":"x","kind":"date"}]`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"select without options", http.MethodPut, "/api/v1/library/versions/" + someID + "/log-fields", `[{"label":"x","kind":"select"}]`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"score component without max", http.MethodPut, "/api/v1/library/versions/" + someID + "/score-set", `[{"key":"a","label":"A"}]`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"score key with spaces", http.MethodPut, "/api/v1/library/versions/" + someID + "/score-set", `[{"key":"giữa kỳ","label":"A","max":10}]`, http.StatusUnprocessableEntity, apperror.CodeValidation},
		{"unknown version detail", http.MethodGet, "/api/v1/library/versions/" + someID, "", http.StatusNotFound, apperror.CodeNotFound},
		{"malformed material id", http.MethodGet, "/api/v1/library/materials/nope", "", http.StatusNotFound, apperror.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, env := do(t, r, tc.method, tc.path, tc.body, owner)
			if w.Code != tc.status || env.Error == nil || env.Error.Code != tc.code {
				t.Fatalf("want %d %s, got %d %+v", tc.status, tc.code, w.Code, env)
			}
		})
	}

	// Element failures name the row so an editor can highlight it.
	_, env := do(t, r, http.MethodPut, "/api/v1/library/versions/"+someID+"/log-fields",
		`[{"label":"ok","kind":"text"},{"label":"","kind":"date"}]`, owner)
	if env.Error == nil || env.Error.Fields["1.label"] == "" || env.Error.Fields["1.kind"] == "" || env.Error.Fields["0.label"] != "" {
		t.Fatalf("want indexed field errors, got %+v", env.Error)
	}
}

func TestPrepOverHTTP(t *testing.T) {
	r, d := newHTTPTest(t)
	owner, reader, editor, assigner := mintToken(t, d.owner), mintToken(t, d.reader), mintToken(t, d.editor), mintToken(t, d.assigner)
	d.repo.members[d.assigner] = "Thầy Minh"

	w, env := do(t, r, http.MethodPost, "/api/v1/library/templates", `{"code":"CT01","name":"Toán 6","lesson_count":2}`, editor)
	if w.Code != http.StatusCreated {
		t.Fatalf("create with lesson_count: got %d %+v", w.Code, env)
	}
	tpl := decode[TemplateResponse](t, env)
	if tpl.Prep == nil || tpl.Prep.LessonCount != 2 {
		t.Fatalf("summary must show the seeded lessons, got %+v", tpl.Prep)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/library/templates", `{"code":"CT02","name":"Toán 7","lesson_count":0}`, editor)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("lesson_count below 1 must be rejected: got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates?has_draft=true", "", reader)
	if w.Code != http.StatusOK || len(decode[[]TemplateResponse](t, env)) != 1 {
		t.Fatalf("has_draft list: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/templates/"+tpl.ID.String()+"/versions", "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("versions: got %d %+v", w.Code, env)
	}
	vid := decode[[]VersionResponse](t, env)[0].ID.String()
	w, env = do(t, r, http.MethodGet, "/api/v1/library/versions/"+vid+"/lessons", "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("lessons: got %d %+v", w.Code, env)
	}
	lessons := decode[[]LessonResponse](t, env)
	lid := lessons[0].ID.String()

	prepBody := `{"prep_status":"doing","checklist":[{"label":"In phiếu","done":true},{"label":"Soạn slide"}]}`
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/prep", prepBody, reader)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reader must not change prep: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/prep", prepBody, editor)
	if w.Code != http.StatusOK {
		t.Fatalf("editor prep: got %d %+v", w.Code, env)
	}
	if got := decode[LessonResponse](t, env); got.PrepStatus != PrepDoing || len(got.Checklist) != 2 {
		t.Fatalf("prep response: %+v", got)
	}
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/prep", `{"prep_status":"blocked"}`, editor)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["prep_status"] == "" {
		t.Fatalf("unknown status must fail validation on prep_status: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/prep", `{"checklist":[{"label":""}]}`, editor)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank checklist label must fail validation: got %d %+v", w.Code, env)
	}

	assignBody := `{"assignee_id":"` + d.assigner.String() + `","due_date":"2026-10-01"}`
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/assignment", assignBody, editor)
	if w.Code != http.StatusForbidden {
		t.Fatalf("library.edit alone must not assign: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/assignment", assignBody, assigner)
	if w.Code != http.StatusOK {
		t.Fatalf("assigner: got %d %+v", w.Code, env)
	}
	if got := decode[LessonResponse](t, env); got.AssigneeID == nil || *got.AssigneeID != d.assigner || got.DueDate == nil || *got.DueDate != "2026-10-01" {
		t.Fatalf("assignment response: %+v", got)
	}
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/assignment", `{"due_date":"01/10/2026"}`, owner)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["due_date"] == "" {
		t.Fatalf("malformed due_date must fail validation on due_date: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPatch, "/api/v1/library/lessons/"+lid+"/assignment", `{"assignee_id":"`+uuid.NewString()+`"}`, owner)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["assignee_id"] == "" {
		t.Fatalf("non-member assignee must be a 422 on assignee_id: got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/library/versions/"+vid+"/board", "", mintToken(t, uuid.New()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("a member without library.read does not read the board: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/versions/"+vid+"/board", "", assigner)
	if w.Code != http.StatusOK {
		t.Fatalf("prep.assign implies library.read, so the assigner reads the board: got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, "/api/v1/library/versions/"+vid+"/board", "", reader)
	if w.Code != http.StatusOK {
		t.Fatalf("reader board: got %d %+v", w.Code, env)
	}
	board := decode[BoardResponse](t, env)
	if len(board.Columns) != 4 || len(board.Columns[1].Lessons) != 1 || board.Columns[1].Lessons[0].AssigneeName == nil || *board.Columns[1].Lessons[0].AssigneeName != "Thầy Minh" {
		t.Fatalf("board must group the doing card with its assignee name, got %+v", board.Columns)
	}
	if board.Columns[2].Lessons == nil {
		t.Fatalf("empty columns must serialise as [] not null")
	}
}
