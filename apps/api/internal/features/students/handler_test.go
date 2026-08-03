package students

import (
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

const handlerTestSecret = "students-test-secret-0123456789abcdef"

// newStudentsHTTPTest wires the real routes and auth middleware over the
// in-memory fake repository.
func newStudentsHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(NewService(repo, &fakeEnder{}, noopTx{})), middleware.RequireAuth(jwtCfg))
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

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newStudentsHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/students"},
		{http.MethodGet, "/api/v1/students"},
		{http.MethodGet, "/api/v1/students/" + someID},
		{http.MethodPut, "/api/v1/students/" + someID},
		{http.MethodDelete, "/api/v1/students/" + someID},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	r, repo := newStudentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	contactID := repo.addContact(teacherID)

	cases := map[string]struct {
		body      string
		wantField string
	}{
		"missing name":        {`{"contact_id":"` + contactID.String() + `"}`, "full_name"},
		"missing contact":     {`{"full_name":"Bé An"}`, "contact_id"},
		"note too long":       {`{"full_name":"Bé An","contact_id":"` + contactID.String() + `","display_note":"` + strings.Repeat("x", 51) + `"}`, "display_note"},
		"foreign contact":     {`{"full_name":"Bé An","contact_id":"` + uuid.NewString() + `"}`, "contact_id"},
		"name over 100 runes": {`{"full_name":"` + strings.Repeat("x", 101) + `","contact_id":"` + contactID.String() + `"}`, "full_name"},
	}
	for name, tc := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/students", tc.body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		if env.Error.Fields[tc.wantField] == "" {
			t.Fatalf("%s: want a message for field %q, got %+v", name, tc.wantField, env.Error.Fields)
		}
	}

	// A contact_id that is not even a uuid fails JSON decoding, which the
	// shared contract reports as 400 BAD_REQUEST, not 422.
	w, env := do(t, r, http.MethodPost, "/api/v1/students", `{"full_name":"Bé An","contact_id":"not-a-uuid"}`, token)
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("malformed uuid must be 400 BAD_REQUEST, got %d %+v", w.Code, env)
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	r, repo := newStudentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	contactID := repo.addContact(teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/students",
		`{"full_name":"Bé An","contact_id":"`+contactID.String()+`","display_note":"An lớp 9A"}`, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201, got %d %+v", w.Code, env)
	}
	var created StudentResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.ContactName == "" || created.ContactPhone == "" {
		t.Fatalf("response must carry contact details, got %+v", created)
	}
	if created.DisplayNote != "An lớp 9A" {
		t.Fatalf("display note must round-trip, got %q", created.DisplayNote)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/students/"+created.ID.String(), "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	r, _ := newStudentsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/students/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

func TestListRejectsMalformedFilters(t *testing.T) {
	r, _ := newStudentsHTTPTest(t)
	token := mintToken(t, uuid.New())

	for _, param := range []string{"contact_id", "class_id"} {
		w, env := do(t, r, http.MethodGet, "/api/v1/students?"+param+"=nope", "", token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields[param] == "" {
			t.Fatalf("malformed %s must be 422 naming the field, got %d %+v", param, w.Code, env)
		}
	}
}

func TestListIsTenantScoped(t *testing.T) {
	r, repo := newStudentsHTTPTest(t)
	owner := uuid.New()
	contactID := repo.addContact(owner)

	if w, env := do(t, r, http.MethodPost, "/api/v1/students",
		`{"full_name":"Bé An","contact_id":"`+contactID.String()+`"}`, mintToken(t, owner)); w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}

	w, env := do(t, r, http.MethodGet, "/api/v1/students", "", mintToken(t, uuid.New()))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []StudentResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another teacher's list must be empty, got %+v", rows)
	}
}
