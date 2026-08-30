package classes

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

const handlerTestSecret = "classes-test-secret-0123456789abcdef"

// fakeScopeResolver resolves every known teacher id to a scope where it owns
// its own center — exactly like the real resolver does for a fixture teacher
// who never joined anyone else's center.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newClassesHTTPTest wires the real routes, auth, and scope middleware over
// the in-memory fake repository.
func newClassesHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(NewService(repo, noopTx{}, noopStaffSeeder{})),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(fakeScopeResolver{}))
	return r, repo
}

// mintToken signs an access token the same way the auth issuer does, without
// importing features/auth.
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

const validCreateBody = `{
	"name": "Toán 8",
	"start_date": "2026-01-05",
	"default_unit_price": 150000,
	"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]
}`

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/classes"},
		{http.MethodGet, "/api/v1/classes"},
		{http.MethodGet, "/api/v1/classes/" + someID},
		{http.MethodPut, "/api/v1/classes/" + someID},
		{http.MethodPost, "/api/v1/classes/" + someID + "/archive"},
		{http.MethodDelete, "/api/v1/classes/" + someID},
		{http.MethodPost, "/api/v1/classes/" + someID + "/schedules"},
		{http.MethodPut, "/api/v1/classes/" + someID + "/schedules/" + someID},
		{http.MethodDelete, "/api/v1/classes/" + someID + "/schedules/" + someID},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	cases := map[string]struct {
		body      string
		wantField string
	}{
		"missing name": {
			`{"start_date":"2026-01-05","default_unit_price":150000,"schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"name",
		},
		"bad start_date": {
			`{"name":"Toán 8","start_date":"05/01/2026","default_unit_price":150000,"schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"start_date",
		},
		"missing price": {
			`{"name":"Toán 8","start_date":"2026-01-05","schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"default_unit_price",
		},
		"negative price": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":-1,"schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"default_unit_price",
		},
		"missing schedules": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000}`,
			"schedules",
		},
		"empty schedules": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000,"schedules":[]}`,
			"schedules",
		},
		"weekday out of range": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000,"schedules":[{"weekday":7,"start_time":"18:00","duration_min":90}]}`,
			"weekday",
		},
		"bad start_time": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000,"schedules":[{"weekday":2,"start_time":"25:99","duration_min":90}]}`,
			"start_time",
		},
	}
	for name, tc := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/classes", tc.body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		found := false
		for field := range env.Error.Fields {
			if field == tc.wantField || strings.Contains(field, tc.wantField) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: want a message for field %q, got %+v", name, tc.wantField, env.Error.Fields)
		}
	}
}

func TestCreateAcceptsFreeClass(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	body := `{"name":"Lớp miễn phí","start_date":"2026-01-05","default_unit_price":0,"schedules":[{"weekday":0,"start_time":"08:00","duration_min":60}]}`
	w, env := do(t, r, http.MethodPost, "/api/v1/classes", body, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("a 0 đồng class is legitimate, got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.DefaultUnitPrice != 0 {
		t.Fatalf("want price 0, got %d", created.DefaultUnitPrice)
	}
	if len(created.Schedules) != 1 || created.Schedules[0].Weekday != 0 {
		t.Fatalf("weekday 0 (Sunday) must survive the round trip, got %+v", created.Schedules)
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201, got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.DefaultUnitPrice != 150000 {
		t.Fatalf("150000 đồng must round-trip exactly, got %d", created.DefaultUnitPrice)
	}
	if created.Status != StatusActive {
		t.Fatalf("new class must be active, got %q", created.Status)
	}
	if len(created.Schedules) != 1 || created.Schedules[0].EffectiveFrom != "2026-01-05" {
		t.Fatalf("schedule effective_from must default to start_date, got %+v", created.Schedules)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/classes/"+created.ID.String(), "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
}

func TestListStatusFilter(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+created.ID.String()+"/archive", "", token); w.Code != http.StatusOK {
		t.Fatalf("archive: got %d %+v", w.Code, env)
	}

	listLen := func(query string) int {
		t.Helper()
		w, env := do(t, r, http.MethodGet, "/api/v1/classes"+query, "", token)
		if w.Code != http.StatusOK {
			t.Fatalf("list %q: got %d %+v", query, w.Code, env)
		}
		var rows []ClassResponse
		if err := json.Unmarshal(env.Data, &rows); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return len(rows)
	}
	if n := listLen(""); n != 0 {
		t.Fatalf("default list must exclude archived classes, got %d", n)
	}
	if n := listLen("?status=archived"); n != 1 {
		t.Fatalf("archived filter must show the class, got %d", n)
	}
	if n := listLen("?status=all"); n != 1 {
		t.Fatalf("all filter must show the class, got %d", n)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/classes?status=bogus", "", token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["status"] == "" {
		t.Fatalf("unknown status must be 422 with a status field message, got %d %+v", w.Code, env)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/classes/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

func TestDeleteWithOpenEnrollmentsConflicts(t *testing.T) {
	r, repo := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	repo.openEnrollments[created.ID] = 2

	w, env = do(t, r, http.MethodDelete, "/api/v1/classes/"+created.ID.String(), "", token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != apperror.CodeConflict {
		t.Fatalf("want 409 CONFLICT, got %d %+v", w.Code, env)
	}
	if !strings.Contains(env.Error.Message, "archive") {
		t.Fatalf("conflict message must suggest archiving, got %q", env.Error.Message)
	}
}

func TestListIsTenantScoped(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	owner := uuid.New()
	stranger := uuid.New()

	if w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, mintToken(t, owner)); w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}

	w, env := do(t, r, http.MethodGet, "/api/v1/classes", "", mintToken(t, stranger))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []ClassResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another teacher's list must be empty, got %+v", rows)
	}
}
