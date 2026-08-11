package contacts

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

const handlerTestSecret = "contacts-test-secret-0123456789abcdef"

// fakeScopeResolver resolves every known teacher id to a scope where it owns
// its own center — exactly like the real resolver does for a fixture teacher
// who never joined anyone else's center.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newContactsHTTPTest wires the real routes, auth, and scope middleware over
// the in-memory fake repository.
func newContactsHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(NewService(repo)),
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

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/contacts"},
		{http.MethodGet, "/api/v1/contacts"},
		{http.MethodGet, "/api/v1/contacts/" + someID},
		{http.MethodPut, "/api/v1/contacts/" + someID},
		{http.MethodDelete, "/api/v1/contacts/" + someID},
		{http.MethodPut, "/api/v1/contacts/" + someID + "/zalo-mapping"},
		{http.MethodDelete, "/api/v1/contacts/" + someID + "/zalo-mapping"},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	token := mintToken(t, uuid.New())

	cases := map[string]struct {
		body      string
		wantField string
	}{
		"missing name":  {`{"phone":"0912345678"}`, "full_name"},
		"name too long": {`{"full_name":"` + strings.Repeat("x", 101) + `","phone":"0912345678"}`, "full_name"},
		"missing phone": {`{"full_name":"Chị Hoa"}`, "phone"},
		"bad phone":     {`{"full_name":"Chị Hoa","phone":"12345"}`, "phone"},
	}
	for name, tc := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/contacts", tc.body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		if env.Error.Fields[tc.wantField] == "" {
			t.Fatalf("%s: want a message for field %q, got %+v", name, tc.wantField, env.Error.Fields)
		}
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/contacts",
		`{"full_name":"Chị Hoa","phone":"0912345678"}`, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201, got %d %+v", w.Code, env)
	}
	var created ContactResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.Phone != "+84912345678" {
		t.Fatalf("response must carry the normalised phone, got %q", created.Phone)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/contacts/"+created.ID.String(), "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/contacts/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

func TestListIsTenantScoped(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	owner := uuid.New()
	stranger := uuid.New()

	if w, env := do(t, r, http.MethodPost, "/api/v1/contacts",
		`{"full_name":"Chị Hoa","phone":"0912345678"}`, mintToken(t, owner)); w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}

	w, env := do(t, r, http.MethodGet, "/api/v1/contacts", "", mintToken(t, stranger))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []ContactResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another teacher's list must be empty, got %+v", rows)
	}
}

func TestZaloMappingRoundTrip(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/contacts",
		`{"full_name":"Chị Hoa","phone":"0912345678"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ContactResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	base := "/api/v1/contacts/" + created.ID.String()

	w, env = do(t, r, http.MethodPut, base+"/zalo-mapping",
		`{"zalo_user_id":"8421000123456789","zalo_name":"Hoa Nguyễn"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("map: want 200, got %d %+v", w.Code, env)
	}
	var mapped ContactResponse
	if err := json.Unmarshal(env.Data, &mapped); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if mapped.ZaloUserID != "8421000123456789" || mapped.ZaloName != "Hoa Nguyễn" {
		t.Fatalf("mapping missing from response: %+v", mapped)
	}

	// The regular contact read must carry the mapping too — the list UI shows it.
	w, env = do(t, r, http.MethodGet, base, "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d %+v", w.Code, env)
	}
	var got ContactResponse
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.ZaloUserID != "8421000123456789" || got.ZaloName != "Hoa Nguyễn" {
		t.Fatalf("mapping missing from GET: %+v", got)
	}

	// Unmap is 204 and idempotent, mirroring the unlink endpoint's contract.
	if w, env = do(t, r, http.MethodDelete, base+"/zalo-mapping", "", token); w.Code != http.StatusNoContent {
		t.Fatalf("unmap: want 204, got %d %+v", w.Code, env)
	}
	if w, env = do(t, r, http.MethodDelete, base+"/zalo-mapping", "", token); w.Code != http.StatusNoContent {
		t.Fatalf("second unmap: want 204, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, base, "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("get after unmap: got %d %+v", w.Code, env)
	}
	var cleared ContactResponse
	if err := json.Unmarshal(env.Data, &cleared); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if cleared.ZaloUserID != "" || cleared.ZaloName != "" {
		t.Fatalf("mapping must be gone after unmap: %+v", cleared)
	}
}

func TestZaloMappingValidation(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/contacts",
		`{"full_name":"Chị Hoa","phone":"0912345678"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ContactResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	path := "/api/v1/contacts/" + created.ID.String() + "/zalo-mapping"

	cases := map[string]struct {
		body      string
		wantField string
	}{
		"missing user id":  {`{"zalo_name":"Hoa"}`, "zalo_user_id"},
		"blank user id":    {`{"zalo_user_id":"","zalo_name":"Hoa"}`, "zalo_user_id"},
		"user id too long": {`{"zalo_user_id":"` + strings.Repeat("9", 33) + `","zalo_name":"Hoa"}`, "zalo_user_id"},
		"missing name":     {`{"zalo_user_id":"8421"}`, "zalo_name"},
		"name too long":    {`{"zalo_user_id":"8421","zalo_name":"` + strings.Repeat("x", 101) + `"}`, "zalo_name"},
	}
	for name, tc := range cases {
		w, env := do(t, r, http.MethodPut, path, tc.body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		if env.Error.Fields[tc.wantField] == "" {
			t.Fatalf("%s: want a message for field %q, got %+v", name, tc.wantField, env.Error.Fields)
		}
	}
}

func TestZaloMappingUnknownContactIs404(t *testing.T) {
	r, _ := newContactsHTTPTest(t)
	token := mintToken(t, uuid.New())
	path := "/api/v1/contacts/" + uuid.NewString() + "/zalo-mapping"

	if w, env := do(t, r, http.MethodPut, path,
		`{"zalo_user_id":"8421","zalo_name":"Hoa"}`, token); w.Code != http.StatusNotFound {
		t.Fatalf("map unknown: want 404, got %d %+v", w.Code, env)
	}
	if w, env := do(t, r, http.MethodDelete, path, "", token); w.Code != http.StatusNotFound {
		t.Fatalf("unmap unknown: want 404, got %d %+v", w.Code, env)
	}
}
