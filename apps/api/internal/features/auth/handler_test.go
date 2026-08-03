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
	"teka/apps/api/internal/features/users"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
)

// newHandlerHTTPTest wires the real auth routes and middleware over the
// in-memory fakes — full HTTP behavior (cookies, envelopes) without a DB.
func newHandlerHTTPTest(t *testing.T) (*gin.Engine, *fakeUserService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	usersSvc := newFakeUserService()
	jwtCfg := config.JWTConfig{
		Secret:     "handler-test-secret-0123456789abcdef",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	}
	svc := NewService(usersSvc, newFakeTokenRepository(), NewTokenIssuer(jwtCfg), noopTxManager{})
	cfg := &config.Config{Env: config.EnvTest, JWT: jwtCfg}

	r := gin.New()
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc, cfg), middleware.RequireAuth(jwtCfg))
	return r, usersSvc
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
		`{"email":"new@example.com","password":"password-123","name":"New"}`, nil)
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
	if body.User.Email != "new@example.com" {
		t.Fatalf("unexpected user in body: %+v", body.User)
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
		`{"email":"nope","password":"short","name":""}`, nil)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("want 422 VALIDATION_ERROR, got %d %+v", w.Code, env)
	}
	for _, field := range []string{"email", "password", "name"} {
		if env.Error.Fields[field] == "" {
			t.Fatalf("missing per-field message for %q: %+v", field, env.Error.Fields)
		}
	}

	w, env = doJSON(t, r, http.MethodPost, "/api/v1/auth/register", `{"email":`, nil)
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("want 400 BAD_REQUEST, got %d %+v", w.Code, env)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	r, usersSvc := newHandlerHTTPTest(t)
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"email":"a@example.com","password":"wrong-password"}`, nil)
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
		`{"email":"cycle@example.com","password":"password-123","name":"Cycle"}`, nil)
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

func TestMeRequiresAndAcceptsAccessToken(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
		`{"email":"me@example.com","password":"password-123","name":"Me"}`, nil)
	var body TokenResponse
	env := struct {
		Data json.RawMessage `json:"data"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode register body: %v", err)
	}
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}

	w, wireEnv := doJSON(t, r, http.MethodGet, "/api/v1/auth/me", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("me without token: want 401, got %d", w.Code)
	}
	if wireEnv.Success || wireEnv.Error == nil || wireEnv.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("me without token: want UNAUTHORIZED envelope, got %+v", wireEnv)
	}

	w, wireEnv = doJSON(t, r, http.MethodGet, "/api/v1/auth/me", "", func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+body.AccessToken)
	})
	if w.Code != http.StatusOK || !wireEnv.Success {
		t.Fatalf("me with token: want 200, got %d %+v", w.Code, wireEnv)
	}
	if !strings.Contains(string(wireEnv.Data), "me@example.com") {
		t.Fatalf("me must return the profile, got %s", wireEnv.Data)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	r, _ := newHandlerHTTPTest(t)

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/register",
		`{"email":"bye@example.com","password":"password-123","name":"Bye"}`, nil)
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
