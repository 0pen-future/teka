package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
)

// newHandlerHTTPTest wires the real auth routes and middleware over the
// in-memory fakes — full HTTP behavior (cookies, envelopes) without a DB.
func newHandlerHTTPTest(t *testing.T) (*gin.Engine, *fakeAccountService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	accounts := newFakeAccountService()
	jwtCfg := config.JWTConfig{
		Secret:     "handler-test-secret-0123456789abcdef",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	}
	svc := NewService(accounts, newFakeTokenRepository(), NewTokenIssuer(jwtCfg), noopTxManager{})
	cfg := &config.Config{Env: config.EnvTest, JWT: jwtCfg}

	r := gin.New()
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc, cfg))
	return r, accounts
}

type wireEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string, mutate func(*http.Request)) (*httptest.ResponseRecorder, wireEnvelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env wireEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
	}
	return w, env
}

func refreshCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName {
			return c
		}
	}
	return nil
}

func TestRegisterSetsCookieAndEnvelope(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
		`{"phone":"0901234567","password":"password-123","full_name":"Cô Lan"}`, nil)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201 success envelope, got %d %+v", w.Code, env)
	}

	var body TokenResponse
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if body.AccessToken == "" || body.TokenType != "Bearer" || body.ExpiresIn != int64((15*time.Minute).Seconds()) {
		t.Fatalf("unexpected token body: %+v", body)
	}
	if body.Teacher.Phone != "+84901234567" || body.Teacher.FullName != "Cô Lan" {
		t.Fatalf("unexpected teacher in body: %+v", body.Teacher)
	}
	if strings.Contains(string(env.Data), "refresh") {
		t.Fatal("refresh token must never appear in the response body")
	}

	c := refreshCookie(t, w)
	if c == nil {
		t.Fatal("register must set the refresh cookie")
	}
	if !c.HttpOnly || c.Path != refreshCookiePath || c.Value == "" {
		t.Fatalf("unexpected cookie attributes: %+v", c)
	}
	if c.Secure {
		t.Fatal("cookie must not be Secure outside production")
	}
}

func TestRegisterValidationAndMalformedJSON(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
		`{"phone":"12345","password":"short","full_name":""}`, nil)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("want 422 VALIDATION_ERROR, got %d %+v", w.Code, env)
	}
	for _, field := range []string{"phone", "password", "full_name"} {
		if env.Error.Fields[field] == "" {
			t.Fatalf("missing per-field message for %q: %+v", field, env.Error.Fields)
		}
	}

	w, env = doJSON(t, r, http.MethodPost, "/api/v1/auth/register", `{"phone":`, nil)
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("want 400 BAD_REQUEST, got %d %+v", w.Code, env)
	}
}

func TestRegisterRejectsNonVietnamesePhone(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	for _, phone := range []string{"+1555123456", "0123456789", "84901234567", "090123456"} {
		w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
			`{"phone":"`+phone+`","password":"password-123","full_name":"X"}`, nil)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["phone"] == "" {
			t.Fatalf("phone %q: want 422 with phone field message, got %d %+v", phone, w.Code, env)
		}
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	r, accounts := newHandlerHTTPTest(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"+84901234567","password":"wrong-password"}`, nil)
	if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("want 401 UNAUTHORIZED, got %d %+v", w.Code, env)
	}
}

func TestRefreshRequiresCookie(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", "", nil)
	if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("want 401 UNAUTHORIZED, got %d %+v", w.Code, env)
	}
}

func TestRefreshRotatesCookieOverHTTP(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
		`{"phone":"0901234567","password":"password-123","full_name":"Cycle"}`, nil)
	first := refreshCookie(t, w)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", "", func(req *http.Request) {
		req.AddCookie(first)
	})
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200 refresh, got %d %+v", w.Code, env)
	}
	rotated := refreshCookie(t, w)
	if rotated == nil || rotated.Value == first.Value {
		t.Fatal("refresh must rotate the cookie value")
	}
}

func TestAuthMeRouteIsGone(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	// The profile moved to /me on the teachers feature; the old route must
	// not linger as a duplicate.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /auth/me must 404 after the move, got %d", w.Code)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
		`{"phone":"0901234567","password":"password-123","full_name":"Bye"}`, nil)
	issued := refreshCookie(t, w)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/logout", "", func(req *http.Request) {
		req.AddCookie(issued)
	})
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200 logout, got %d %+v", w.Code, env)
	}
	// A Max-Age=0 delete directive parses back as MaxAge -1.
	cleared := refreshCookie(t, w)
	if cleared == nil || cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("logout must clear the refresh cookie (empty value, negative MaxAge), got %+v", cleared)
	}
}
