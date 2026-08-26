package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
)

// newHandlerHTTPTest wires the real auth routes and middleware over the
// in-memory fakes — full HTTP behavior (cookies, envelopes) without a DB.
// Rate limits are generous (well above what any single test can trigger) so
// they never interfere with a handler test's own assertions.
func newHandlerHTTPTest(t *testing.T) (*gin.Engine, *fakeAccountService, *fakeOwnerResolver, *fakeResetDMSender) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	accounts := newFakeAccountService()
	owners := newFakeOwnerResolver()
	dmSender := &fakeResetDMSender{}
	jwtCfg := config.JWTConfig{
		Secret:     "handler-test-secret-0123456789abcdef",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	}
	svc := NewService(accounts, newFakeTokenRepository(), NewTokenIssuer(jwtCfg), noopTxManager{},
		owners, dmSender, testResetConfig(), "https://app.example.com", nil)
	cfg := &config.Config{Env: config.EnvTest, JWT: jwtCfg}

	r := gin.New()
	h := NewHandler(svc, cfg)
	group := r.Group("/api/v1")
	RegisterRoutes(group, h)
	RegisterPublicRoutes(group, h,
		middleware.RateLimit(middleware.JSONBodyKey("phone"), 1000, time.Minute),
		middleware.RateLimit(middleware.JSONBodyKey("token"), 1000, time.Minute))
	return r, accounts, owners, dmSender
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
	if len(w.Body.Bytes()) > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
		}
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

// TestAuthRegisterRouteIsGone: self-registration is invite-only now (see
// features/invitations); the old route must not linger.
func TestAuthRegisterRouteIsGone(t *testing.T) {
	r, _, _, _ := newHandlerHTTPTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"phone":"0901234567","password":"password-123","full_name":"Cô Lan"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /auth/register must 404, got %d", w.Code)
	}
}

func TestLoginSetsCookieAndEnvelope(t *testing.T) {
	r, accounts, _, _ := newHandlerHTTPTest(t)
	accounts.add(t, "+84901234567", "password-123", teachers.StatusActive)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"0901234567","password":"password-123"}`, nil)
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("want 200 success envelope, got %d %+v", w.Code, env)
	}

	var body TokenResponse
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if body.AccessToken == "" || body.TokenType != "Bearer" || body.ExpiresIn != int64((15*time.Minute).Seconds()) {
		t.Fatalf("unexpected token body: %+v", body)
	}
	if strings.Contains(string(env.Data), "refresh") {
		t.Fatal("refresh token must never appear in the response body")
	}

	c := refreshCookie(t, w)
	if c == nil {
		t.Fatal("login must set the refresh cookie")
	} else if !c.HttpOnly || c.Path != refreshCookiePath || c.Value == "" {
		t.Fatalf("unexpected cookie attributes: %+v", c)
	} else if c.Secure {
		t.Fatal("cookie must not be Secure outside production")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	r, accounts, _, _ := newHandlerHTTPTest(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"+84901234567","password":"wrong-password"}`, nil)
	if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("want 401 UNAUTHORIZED, got %d %+v", w.Code, env)
	}
}

func TestRefreshRequiresCookie(t *testing.T) {
	r, _, _, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", "", nil)
	if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
		t.Fatalf("want 401 UNAUTHORIZED, got %d %+v", w.Code, env)
	}
}

func TestRefreshRotatesCookieOverHTTP(t *testing.T) {
	r, accounts, _, _ := newHandlerHTTPTest(t)
	accounts.add(t, "+84901234567", "password-123", teachers.StatusActive)

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"0901234567","password":"password-123"}`, nil)
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
	r, _, _, _ := newHandlerHTTPTest(t)

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
	r, accounts, _, _ := newHandlerHTTPTest(t)
	accounts.add(t, "+84901234567", "password-123", teachers.StatusActive)

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"0901234567","password":"password-123"}`, nil)
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

// TestForgotPasswordAlwaysReturnsTheIdenticalGenericEnvelope proves the
// anti-enumeration guarantee at the HTTP layer: a member (eligible), an
// owner (excluded), and an unknown phone all answer the exact same 200 body.
func TestForgotPasswordAlwaysReturnsTheIdenticalGenericEnvelope(t *testing.T) {
	r, accounts, owners, _ := newHandlerHTTPTest(t)
	member := accounts.add(t, "+84901234567", "password-123", teachers.StatusActive)
	owners.setMember(member.Account.ID, uuid.New())
	owner := accounts.add(t, "+84902222222", "password-123", teachers.StatusActive)
	owners.setOwner(owner.Account.ID)

	phones := []string{"0901234567", "0902222222", "0909999999"}
	var bodies []string
	for _, phone := range phones {
		w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/forgot-password",
			`{"phone":"`+phone+`"}`, nil)
		if w.Code != http.StatusOK || !env.Success {
			t.Fatalf("phone %q: want 200 success envelope, got %d %+v", phone, w.Code, env)
		}
		bodies = append(bodies, string(env.Data))
	}
	for i, b := range bodies {
		if b != bodies[0] {
			t.Fatalf("phone %q's response body differs from the first: %q vs %q", phones[i], b, bodies[0])
		}
	}
}

func TestForgotPasswordRejectsInvalidPhone(t *testing.T) {
	r, _, _, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/forgot-password", `{"phone":"not-a-phone"}`, nil)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil {
		t.Fatalf("want 422 validation error, got %d %+v", w.Code, env)
	}
}

func TestResetPasswordRejectsUnknownTokenWithGenericBadRequest(t *testing.T) {
	r, _, _, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		`{"token":"does-not-exist","password":"new-password-123"}`, nil)
	if w.Code != http.StatusBadRequest || env.Error == nil {
		t.Fatalf("want 400 generic rejection, got %d %+v", w.Code, env)
	}
}

func TestResetPasswordRejectsShortPassword(t *testing.T) {
	r, _, _, _ := newHandlerHTTPTest(t)

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		`{"token":"whatever","password":"short"}`, nil)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil {
		t.Fatalf("want 422 validation error, got %d %+v", w.Code, env)
	}
}

func TestResetPasswordRoundTripReturnsNoContentAndClearsSession(t *testing.T) {
	r, accounts, owners, dmSender := newHandlerHTTPTest(t)
	p := accounts.add(t, "+84901234567", "old-password", teachers.StatusActive)
	owners.setMember(p.Account.ID, uuid.New())
	dmSender.lookupOK = true
	dmSender.lookupUID = "u1"

	w, env := doJSON(t, r, http.MethodPost, "/api/v1/auth/forgot-password", `{"phone":"0901234567"}`, nil)
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("forgot-password: want 200, got %d %+v", w.Code, env)
	}
	plaintext := extractResetToken(t, dmSender.lastText)

	w, _ = doJSON(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		`{"token":"`+plaintext+`","password":"new-password-123"}`, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("reset-password: want 204, got %d body=%s", w.Code, w.Body.String())
	}

	w, env = doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"0901234567","password":"old-password"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("the old password must be rejected after a reset, got %d %+v", w.Code, env)
	}
	w, env = doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"0901234567","password":"new-password-123"}`, nil)
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("the new password must work after a reset, got %d %+v", w.Code, env)
	}
}
