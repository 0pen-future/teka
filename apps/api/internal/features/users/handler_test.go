package users

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

const handlerTestSecret = "handler-test-secret-0123456789abcdef"

// newHandlerTest wires the real routes, middleware, and service over the
// in-memory fake repository — HTTP behavior without a database.
func newHandlerTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	h := NewHandler(NewService(repo))
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}

	r := gin.New()
	RegisterRoutes(r.Group("/api/v1"), h,
		middleware.RequireAuth(jwtCfg), middleware.RequireRole(authctx.RoleAdmin))
	return r, repo
}

// accessToken mints a token the test router's RequireAuth accepts.
func accessToken(t *testing.T, userID uuid.UUID, role string) string {
	t.Helper()
	now := time.Now()
	claims := authctx.AccessClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(handlerTestSecret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

type wireEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Meta    *struct {
		Page       int   `json:"page"`
		PerPage    int   `json:"per_page"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"total_pages"`
	} `json:"meta"`
	Error *struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func doRequest(t *testing.T, r *gin.Engine, method, path, token, body string) (*httptest.ResponseRecorder, wireEnvelope) {
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

	var env wireEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
	}
	return w, env
}

func TestUsersRequireAuthentication(t *testing.T) {
	r, _ := newHandlerTest(t)

	w, env := doRequest(t, r, http.MethodGet, "/api/v1/users", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	if env.Success || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("want UNAUTHORIZED error envelope, got %+v", env)
	}
}

func TestUsersAdminOnlyRoutes(t *testing.T) {
	r, _ := newHandlerTest(t)
	token := accessToken(t, uuid.New(), RoleUser)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/users", ""},
		{http.MethodPost, "/api/v1/users", `{"email":"a@example.com","password":"password-123","name":"A"}`},
		{http.MethodDelete, "/api/v1/users/" + uuid.NewString(), ""},
	} {
		w, env := doRequest(t, r, tc.method, tc.path, token, tc.body)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: want 403 for role user, got %d", tc.method, tc.path, w.Code)
		}
		if env.Error == nil || env.Error.Code != apperror.CodeForbidden {
			t.Fatalf("%s %s: want FORBIDDEN envelope, got %+v", tc.method, tc.path, env)
		}
	}
}

func TestGetAndUpdateForbidOtherUsers(t *testing.T) {
	r, repo := newHandlerTest(t)
	other := &User{ID: uuid.New(), Email: "other@example.com", Name: "Other", Role: RoleUser}
	repo.users[other.ID] = other
	token := accessToken(t, uuid.New(), RoleUser)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/users/" + other.ID.String(), ""},
		{http.MethodPatch, "/api/v1/users/" + other.ID.String(), `{"name":"Hijacked"}`},
	} {
		w, env := doRequest(t, r, tc.method, tc.path, token, tc.body)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: want 403 for another user's record, got %d", tc.method, tc.path, w.Code)
		}
		if env.Error == nil || env.Error.Code != apperror.CodeForbidden {
			t.Fatalf("%s %s: want FORBIDDEN envelope, got %+v", tc.method, tc.path, env)
		}
	}
	if other.Name != "Other" {
		t.Fatalf("forbidden update must not mutate the record, name = %q", other.Name)
	}
}

func TestCreateValidationErrors(t *testing.T) {
	r, _ := newHandlerTest(t)
	token := accessToken(t, uuid.New(), RoleAdmin)

	w, env := doRequest(t, r, http.MethodPost, "/api/v1/users", token,
		`{"email":"not-an-email","password":"short","name":""}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
	if env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("want VALIDATION_ERROR, got %+v", env)
	}
	for _, field := range []string{"email", "password", "name"} {
		if env.Error.Fields[field] == "" {
			t.Fatalf("missing per-field message for %q: %+v", field, env.Error.Fields)
		}
	}

	// Malformed JSON is a 400, not a validation 422.
	w, env = doRequest(t, r, http.MethodPost, "/api/v1/users", token, `{"email":`)
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("want 400 BAD_REQUEST for malformed JSON, got %d %+v", w.Code, env)
	}
}

func TestCreateAndGetEnvelope(t *testing.T) {
	r, _ := newHandlerTest(t)
	admin := accessToken(t, uuid.New(), RoleAdmin)

	w, env := doRequest(t, r, http.MethodPost, "/api/v1/users", admin,
		`{"email":"new@example.com","password":"password-123","name":"New","role":"user"}`)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201 success envelope, got %d %+v", w.Code, env)
	}
	var created Response
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Email != "new@example.com" || created.ID == uuid.Nil {
		t.Fatalf("unexpected created user: %+v", created)
	}

	w, env = doRequest(t, r, http.MethodGet, "/api/v1/users/"+created.ID.String(), admin, "")
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200 success envelope, got %d %+v", w.Code, env)
	}
}

func TestListEnvelopeMeta(t *testing.T) {
	r, repo := newHandlerTest(t)
	admin := accessToken(t, uuid.New(), RoleAdmin)
	for _, u := range []*User{
		{ID: uuid.New(), Email: "a@example.com", Name: "A", Role: RoleUser},
		{ID: uuid.New(), Email: "b@example.com", Name: "B", Role: RoleUser},
	} {
		repo.users[u.ID] = u
	}

	w, env := doRequest(t, r, http.MethodGet, "/api/v1/users", admin, "")
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200 success, got %d %+v", w.Code, env)
	}
	if env.Meta == nil {
		t.Fatal("list response must carry meta")
	}
	if env.Meta.Page != 1 || env.Meta.PerPage != 20 || env.Meta.Total != 2 || env.Meta.TotalPages != 1 {
		t.Fatalf("unexpected meta: %+v", *env.Meta)
	}
	if string(env.Data) == "null" {
		t.Fatal("list data must serialize as [], never null")
	}
}

func TestGetRejectsInvalidUUID(t *testing.T) {
	r, _ := newHandlerTest(t)
	admin := accessToken(t, uuid.New(), RoleAdmin)

	w, env := doRequest(t, r, http.MethodGet, "/api/v1/users/not-a-uuid", admin, "")
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("want 400 BAD_REQUEST, got %d %+v", w.Code, env)
	}
}
