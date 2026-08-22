package attendance

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
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

const handlerTestSecret = "attendance-test-secret-0123456789abcdef"

// fakeScopeResolver resolves every teacher as the sole owner of their own
// center — enough to exercise handler routing without a real centers
// service.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newAttendanceHTTPTest wires the real routes and auth middleware over the
// in-memory fakes.
func newAttendanceHTTPTest(t *testing.T) (*gin.Engine, *testDeps) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, deps := newTestService()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc), middleware.RequireAuth(jwtCfg), middleware.ResolveScope(fakeScopeResolver{}))
	return r, deps
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
	return w, mustEnvelope(t, w)
}

func mustEnvelope(t *testing.T, w *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if len(w.Body.Bytes()) > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
		}
	}
	return env
}

// setUp wires a teacher with a class holding one roster student and one
// planned session, returning the ids to build requests against.
func setUp(deps *testDeps) (teacherID, sessionID, studentID uuid.UUID) {
	teacherID = uuid.New()
	classID := uuid.New()
	e := deps.roster.addEnrollment(classID, uuid.New())
	sessionID = deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)
	return teacherID, sessionID, e.StudentID
}

func TestAttendanceRoutesRequireAuth(t *testing.T) {
	r, _ := newAttendanceHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/sessions/" + someID + "/attendance"},
		{http.MethodPost, "/api/v1/sessions/" + someID + "/attendance"},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestGetAttendanceSheetBeforeConfirm(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	teacherID, sessionID, _ := setUp(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet, "/api/v1/sessions/"+sessionID.String()+"/attendance", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var out Response
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Status != nil {
		t.Fatalf("unconfirmed session must have one roster row with a null status, got %+v", out.Rows)
	}
}

func TestConfirmAttendanceEmptyBodyMarksEveryonePresent(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	teacherID, sessionID, _ := setUp(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+sessionID.String()+"/attendance", `{}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var out Response
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out.Status != sessions.StatusHeld {
		t.Fatalf("confirm must mark the session held, got %s", out.Status)
	}
	if len(out.Rows) != 1 || out.Rows[0].Status == nil || *out.Rows[0].Status != StatusPresent {
		t.Fatalf("empty absent_student_ids must mark everyone present, got %+v", out.Rows)
	}
}

func TestConfirmAttendanceWithAbsentee(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	teacherID, sessionID, studentID := setUp(deps)
	token := mintToken(t, teacherID)

	body := `{"absent_student_ids":["` + studentID.String() + `"],"note":"ốm"}`
	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+sessionID.String()+"/attendance", body, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var out Response
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Status == nil || *out.Rows[0].Status != StatusAbsent {
		t.Fatalf("want student marked absent, got %+v", out.Rows)
	}
	if out.Rows[0].Note == nil || *out.Rows[0].Note != "ốm" {
		t.Fatalf("want note round-tripped, got %+v", out.Rows[0].Note)
	}
}

func TestConfirmAttendanceRejectsUnknownAbsentee(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	teacherID, sessionID, _ := setUp(deps)
	token := mintToken(t, teacherID)

	body := `{"absent_student_ids":["` + uuid.NewString() + `"]}`
	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+sessionID.String()+"/attendance", body, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["absent_student_ids"] == "" {
		t.Fatalf("absent id outside roster must be 422, got %d %+v", w.Code, env)
	}
}

func TestConfirmAttendanceCancelledSessionIs409(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	teacherID := uuid.New()
	classID := uuid.New()
	deps.roster.addEnrollment(classID, uuid.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusCancelled)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+sessionID.String()+"/attendance", `{}`, token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != apperror.CodeConflict {
		t.Fatalf("cancelled session must be 409, got %d %+v", w.Code, env)
	}
}

func TestConfirmAttendanceMissingSessionIs404(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	teacherID, _, _ := setUp(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+uuid.NewString()+"/attendance", `{}`, token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("missing session must be 404, got %d %+v", w.Code, env)
	}
}

func TestCrossTenantAttendanceIs404(t *testing.T) {
	r, deps := newAttendanceHTTPTest(t)
	_, sessionID, _ := setUp(deps)
	stranger := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/sessions/"+sessionID.String()+"/attendance", "", stranger)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodPost, "/api/v1/sessions/"+sessionID.String()+"/attendance", `{}`, stranger)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant confirm must be 404, got %d %+v", w.Code, env)
	}
}
