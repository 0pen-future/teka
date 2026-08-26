package invitations

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

const handlerTestSecret = "invitations-test-secret-0123456789abcdef"

// fakeScopeResolver resolves a teacher id to owner-of-their-own-center by
// default, exactly like the real resolver does for a fixture teacher who
// never joined anyone else's center. Registering an id via asMember flips
// that single id to a non-owner scope in an explicit center, so 403 behaviour
// can be exercised over real HTTP without a live centers feature.
type fakeScopeResolver struct {
	members map[uuid.UUID]uuid.UUID // teacherID -> centerID, for a non-owner scope
}

func (f *fakeScopeResolver) asMember(teacherID, centerID uuid.UUID) {
	if f.members == nil {
		f.members = map[uuid.UUID]uuid.UUID{}
	}
	f.members[teacherID] = centerID
}

func (f *fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	if centerID, ok := f.members[teacherID]; ok {
		return authctx.Scope{TeacherID: teacherID, CenterID: centerID, IsOwner: false}, nil
	}
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newInvitationsHTTPTest wires the real routes, auth, and scope middleware
// over the in-memory fake repository and a Zalo sender that always answers
// "skipped" — HTTP tests care about the envelope and status code, not DM
// delivery, which service_test.go already covers in depth.
func newInvitationsHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository, *fakeScopeResolver) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	resolver := &fakeScopeResolver{}
	svc := NewService(repo, &fakeZaloSender{}, newFakeOnboarder(), &fakeOpener{}, noopTxManager{}, testOnboardingConfig(), "https://app.example.com", nil)
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(resolver))
	return r, repo, resolver
}

// noLimit is a rate-limit middleware stand-in for tests that care about the
// public routes' own behavior, not the rate limiter's (middleware/ratelimit_test.go
// already covers that in isolation).
func noLimit(c *gin.Context) { c.Next() }

// newInvitationsPublicHTTPTest wires only the unauthenticated preview/accept
// routes, exposing the fake onboarder and opener so a test can assert on the
// account/membership side effects an HTTP-level Accept call triggers.
func newInvitationsPublicHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository, *fakeOnboarder, *fakeOpener) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	opener := &fakeOpener{}
	svc := NewService(repo, &fakeZaloSender{}, onboarder, opener, noopTxManager{}, testOnboardingConfig(), "https://app.example.com", nil)
	r := gin.New()
	RegisterPublicRoutes(r.Group("/api/v1"), NewHandler(svc), noLimit, noLimit)
	return r, repo, onboarder, opener
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
	r, _, _ := newInvitationsHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/centers/me/invitations"},
		{http.MethodGet, "/api/v1/centers/me/invitations"},
		{http.MethodDelete, "/api/v1/centers/me/invitations/" + someID},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestNonOwnerIsForbidden(t *testing.T) {
	r, _, resolver := newInvitationsHTTPTest(t)
	member := uuid.New()
	resolver.asMember(member, uuid.New())
	token := mintToken(t, member)
	someID := uuid.NewString()

	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/centers/me/invitations", `{"phone":"0901234567"}`},
		{http.MethodGet, "/api/v1/centers/me/invitations", ""},
		{http.MethodDelete, "/api/v1/centers/me/invitations/" + someID, ""},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, route.body, token)
		if w.Code != http.StatusForbidden || env.Error == nil || env.Error.Code != apperror.CodeForbidden {
			t.Fatalf("%s %s: want 403 FORBIDDEN, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	r, _, _ := newInvitationsHTTPTest(t)
	token := mintToken(t, uuid.New())

	cases := map[string]string{
		"missing phone": `{}`,
		"bad phone":     `{"phone":"12345"}`,
	}
	for name, body := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/centers/me/invitations", body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		if env.Error.Fields["phone"] == "" {
			t.Fatalf("%s: want a message for field phone, got %+v", name, env.Error.Fields)
		}
	}
}

func TestCreateReturnsEnvelopeWithLinkAndDMStatus(t *testing.T) {
	r, _, _ := newInvitationsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/centers/me/invitations", `{"phone":"0901234567"}`, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201, got %d %+v", w.Code, env)
	}
	var created CreateResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.Phone != "+84901234567" {
		t.Fatalf("response must carry the normalised phone, got %q", created.Phone)
	}
	if created.Link == "" {
		t.Fatalf("response must always carry a copy-link, got %+v", created)
	}
	if created.DMStatus != DMStatusSkipped {
		t.Fatalf("want dm_status skipped (fake sender never links), got %q", created.DMStatus)
	}
}

func TestListSerializesEmptyAsEmptyArrayNotNull(t *testing.T) {
	r, _, _ := newInvitationsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/centers/me/invitations", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	if string(env.Data) != "[]" {
		t.Fatalf("empty list must serialize as [], got %s", env.Data)
	}
}

func TestCreateListRevokeRoundTrip(t *testing.T) {
	r, _, _ := newInvitationsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/centers/me/invitations", `{"phone":"0901234567"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created CreateResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/centers/me/invitations", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("list: got %d %+v", w.Code, env)
	}
	var rows []InvitationResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != StatusPending {
		t.Fatalf("want one pending row, got %+v", rows)
	}

	path := "/api/v1/centers/me/invitations/" + created.ID.String()
	if w, env = do(t, r, http.MethodDelete, path, "", token); w.Code != http.StatusNoContent {
		t.Fatalf("revoke: want 204, got %d %+v", w.Code, env)
	}
	// Idempotent: revoking again still succeeds.
	if w, env = do(t, r, http.MethodDelete, path, "", token); w.Code != http.StatusNoContent {
		t.Fatalf("second revoke: want 204, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/centers/me/invitations", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("list after revoke: got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != StatusRevoked {
		t.Fatalf("want one revoked row, got %+v", rows)
	}
}

func TestRevokeUnknownIDIs404(t *testing.T) {
	r, _, _ := newInvitationsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodDelete, "/api/v1/centers/me/invitations/"+uuid.NewString(), "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("want 404 NOT_FOUND, got %d %+v", w.Code, env)
	}
}

func TestRevokeMalformedIDIs404(t *testing.T) {
	r, _, _ := newInvitationsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodDelete, "/api/v1/centers/me/invitations/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

func TestPreviewAndAcceptRoutesDoNotRequireAuth(t *testing.T) {
	r, repo, _, _ := newInvitationsPublicHTTPTest(t)
	plaintext, _ := mintInvitation(t, repo, uuid.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	w, env := do(t, r, http.MethodPost, "/api/v1/invitations/preview", `{"token":"`+plaintext+`"}`, "")
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("preview without auth: want 200, got %d %+v", w.Code, env)
	}
}

func TestPreviewValidationRequiresToken(t *testing.T) {
	r, _, _, _ := newInvitationsPublicHTTPTest(t)

	w, env := do(t, r, http.MethodPost, "/api/v1/invitations/preview", `{}`, "")
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("want 422 VALIDATION_ERROR, got %d %+v", w.Code, env)
	}
}

func TestPreviewUnknownTokenIs404OverHTTP(t *testing.T) {
	r, _, _, _ := newInvitationsPublicHTTPTest(t)

	w, env := do(t, r, http.MethodPost, "/api/v1/invitations/preview", `{"token":"does-not-exist"}`, "")
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("want 404 NOT_FOUND, got %d %+v", w.Code, env)
	}
}

func TestAcceptValidationRequiresFullNameAndPassword(t *testing.T) {
	r, repo, _, _ := newInvitationsPublicHTTPTest(t)
	plaintext, _ := mintInvitation(t, repo, uuid.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	cases := map[string]string{
		"missing full_name":  `{"token":"` + plaintext + `","password":"password1"}`,
		"missing password":   `{"token":"` + plaintext + `","full_name":"Nguyễn Văn A"}`,
		"password too short": `{"token":"` + plaintext + `","full_name":"Nguyễn Văn A","password":"short"}`,
	}
	for name, body := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/invitations/accept", body, "")
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
	}
}

func TestAcceptHappyPathReturnsNoContentAndCreatesAccount(t *testing.T) {
	r, repo, onboarder, opener := newInvitationsPublicHTTPTest(t)
	plaintext, invID := mintInvitation(t, repo, uuid.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	body := `{"token":"` + plaintext + `","full_name":"Nguyễn Văn A","password":"matkhau123"}`
	w, env := do(t, r, http.MethodPost, "/api/v1/invitations/accept", body, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d %+v", w.Code, env)
	}
	if len(onboarder.created) != 1 {
		t.Fatalf("want exactly one account created, got %+v", onboarder.created)
	}
	if len(opener.opened) != 1 {
		t.Fatalf("want exactly one membership opened, got %+v", opener.opened)
	}
	if repo.rows[invID].Status != StatusAccepted {
		t.Fatalf("want invitation marked accepted, got %q", repo.rows[invID].Status)
	}
}

func TestAcceptRejectionOverHTTPIsGenericBadRequest(t *testing.T) {
	r, _, _, _ := newInvitationsPublicHTTPTest(t)

	body := `{"token":"does-not-exist","full_name":"Nguyễn Văn A","password":"matkhau123"}`
	w, env := do(t, r, http.MethodPost, "/api/v1/invitations/accept", body, "")
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("want 400 BAD_REQUEST, got %d %+v", w.Code, env)
	}
}

func TestTokenNeverAppearsInThePathOrQuery(t *testing.T) {
	r, _, _, _ := newInvitationsPublicHTTPTest(t)
	// The only way to reach preview/accept is a POST with the token in the
	// JSON body — there is no path segment or query parameter an access log
	// could capture it from. This unmatched GET route falls through to gin's
	// own plain-text 404, not the JSON envelope, so it is asserted directly
	// rather than through the do() envelope helper.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/preview/anything", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("a path-based token route must not exist, got %d", w.Code)
	}
}
