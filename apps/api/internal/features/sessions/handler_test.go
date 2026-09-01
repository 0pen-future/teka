package sessions

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

const handlerTestSecret = "sessions-test-secret-0123456789abcdef"

// fakeScopeResolver resolves every teacher as the sole owner of their own
// center — enough to exercise handler routing without a real centers
// service.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newSessionsHTTPTest wires the real routes and auth middleware over the
// in-memory fakes.
func newSessionsHTTPTest(t *testing.T) (*gin.Engine, *testDeps) {
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

// setUpClass wires a teacher with a class open on 2026-01-01 with a Tuesday
// 18:00 schedule, returning the ids to build requests against.
func setUpClass(deps *testDeps) (teacherID, classID uuid.UUID) {
	teacherID = uuid.New()
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID = deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil) // Tuesday
	return teacherID, classID
}

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newSessionsHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/classes/" + someID + "/sessions?from=2026-01-01&to=2026-01-31"},
		{http.MethodPost, "/api/v1/classes/" + someID + "/sessions"},
		{http.MethodGet, "/api/v1/sessions/pending"},
		{http.MethodGet, "/api/v1/sessions/" + someID},
		{http.MethodDelete, "/api/v1/sessions/" + someID},
		{http.MethodPost, "/api/v1/sessions/" + someID + "/cancel"},
		{http.MethodPost, "/api/v1/sessions/" + someID + "/uncancel"},
		{http.MethodPost, "/api/v1/sessions/" + someID + "/hold"},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestListRangeMissingOrMalformedQuery(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)

	cases := map[string]string{
		"missing from": "/api/v1/classes/" + classID.String() + "/sessions?to=2026-01-31",
		"missing to":   "/api/v1/classes/" + classID.String() + "/sessions?from=2026-01-01",
		"bad from":     "/api/v1/classes/" + classID.String() + "/sessions?from=01-01-2026&to=2026-01-31",
	}
	for name, path := range cases {
		w, env := do(t, r, http.MethodGet, path, "", token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil {
			t.Fatalf("%s: want 422, got %d %+v", name, w.Code, env)
		}
	}
}

func TestListRangeGeneratesAndReturnsSessions(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/sessions?from=2026-01-01&to=2026-01-31", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []SessionResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("want 4 Tuesdays in January 2026, got %d", len(rows))
	}
	if rows[0].ClassName != "Fixture Class" {
		t.Fatalf("response must carry the class name, got %+v", rows[0])
	}

	// Repeating the same range must not duplicate rows.
	w, env = do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/sessions?from=2026-01-01&to=2026-01-31", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("second call: want 200, got %d %+v", w.Code, env)
	}
	var again []SessionResponse
	if err := json.Unmarshal(env.Data, &again); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(again) != len(rows) {
		t.Fatalf("regeneration must be idempotent: got %d, want %d", len(again), len(rows))
	}
}

func TestListRangeRejectsOversizedRange(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/sessions?from=2026-01-01&to=2027-06-01", "", token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("range over 400 days must be 422, got %d %+v", w.Code, env)
	}
}

func TestListRangeUnknownClassIs404(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, _ := setUpClass(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet,
		"/api/v1/classes/"+uuid.NewString()+"/sessions?from=2026-01-01&to=2026-01-31", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("unknown class must be 404, got %d %+v", w.Code, env)
	}
}

// TestSessionResponsesCarryAttendanceSummary drives the JSON contract the
// month-calendar UI renders badges from: a confirmed session carries its
// per-status counts on both the list and the detail endpoint, an unconfirmed
// one carries null.
func TestSessionResponsesCarryAttendanceSummary(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/sessions?from=2026-01-01&to=2026-01-31", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []SessionResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("want 4 generated sessions, got %d", len(rows))
	}

	confirmedAt := time.Now()
	confirmed := deps.repo.rows[rows[0].ID]
	confirmed.Status = StatusHeld
	confirmed.AttendanceConfirmedAt = &confirmedAt
	deps.repo.setAttendanceCounts(rows[0].ID, 2, 1, 1, 0)

	w, env = do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/sessions?from=2026-01-01&to=2026-01-31", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("relist: want 200, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode relist body: %v", err)
	}
	want := AttendanceSummary{Present: 2, Late: 1, Absent: 1, Excused: 0}
	if rows[0].AttendanceSummary == nil || *rows[0].AttendanceSummary != want {
		t.Fatalf("confirmed session in list must carry counts, want %+v got %+v", want, rows[0].AttendanceSummary)
	}
	if rows[1].AttendanceSummary != nil {
		t.Fatalf("unconfirmed session in list must carry null summary, got %+v", *rows[1].AttendanceSummary)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/sessions/"+rows[0].ID.String(), "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("detail: want 200, got %d %+v", w.Code, env)
	}
	var detail SessionResponse
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("decode detail body: %v", err)
	}
	if detail.AttendanceSummary == nil || *detail.AttendanceSummary != want {
		t.Fatalf("detail endpoint must carry the same counts, want %+v got %+v", want, detail.AttendanceSummary)
	}
}

func TestCreateAdHocValidationAndConflict(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)

	// Missing session_date is 422.
	w, env := do(t, r, http.MethodPost, "/api/v1/classes/"+classID.String()+"/sessions", `{}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["session_date"] == "" {
		t.Fatalf("missing session_date must be 422, got %d %+v", w.Code, env)
	}

	// Malformed start_time is 422.
	w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+classID.String()+"/sessions",
		`{"session_date":"2026-01-15","start_time":"9am"}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["start_time"] == "" {
		t.Fatalf("malformed start_time must be 422, got %d %+v", w.Code, env)
	}

	// A free date succeeds.
	w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+classID.String()+"/sessions",
		`{"session_date":"2026-01-15","start_time":"10:00"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("ad-hoc on a free date: want 201, got %d %+v", w.Code, env)
	}

	// The same date again conflicts.
	w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+classID.String()+"/sessions",
		`{"session_date":"2026-01-15"}`, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("ad-hoc on an occupied date must be 409, got %d %+v", w.Code, env)
	}
}

// generateOne generates a single-session range and returns its id.
func generateOne(t *testing.T, r *gin.Engine, classID uuid.UUID, token string) SessionResponse {
	t.Helper()
	w, env := do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/sessions?from=2026-01-01&to=2026-01-31", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("generate: want 200, got %d %+v", w.Code, env)
	}
	var rows []SessionResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one generated session")
	}
	return rows[0]
}

func TestCancelRequiresReason(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)
	row := generateOne(t, r, classID, token)

	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+row.ID.String()+"/cancel", `{}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["reason"] == "" {
		t.Fatalf("missing reason must be 422, got %d %+v", w.Code, env)
	}
}

func TestCancelUncancelHoldFlow(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)
	row := generateOne(t, r, classID, token)

	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+row.ID.String()+"/cancel",
		`{"reason":"nghỉ lễ"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: want 200, got %d %+v", w.Code, env)
	}
	var cancelled SessionResponse
	if err := json.Unmarshal(env.Data, &cancelled); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if cancelled.Status != StatusCancelled || cancelled.CancelReason == nil || *cancelled.CancelReason != "nghỉ lễ" {
		t.Fatalf("cancel must persist status and reason, got %+v", cancelled)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/sessions/"+row.ID.String()+"/uncancel", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("uncancel: want 200, got %d %+v", w.Code, env)
	}
	var uncancelled SessionResponse
	if err := json.Unmarshal(env.Data, &uncancelled); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if uncancelled.Status != StatusPlanned {
		t.Fatalf("uncancel must return to planned, got %s", uncancelled.Status)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/sessions/"+row.ID.String()+"/hold", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("hold: want 200, got %d %+v", w.Code, env)
	}
	var held SessionResponse
	if err := json.Unmarshal(env.Data, &held); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if held.Status != StatusHeld {
		t.Fatalf("hold must set status held, got %s", held.Status)
	}
}

func TestCancelAndDeleteRefuseWhenAttendanceConfirmed(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)
	row := generateOne(t, r, classID, token)

	confirmedAt := time.Now()
	deps.repo.rows[row.ID].AttendanceConfirmedAt = &confirmedAt

	w, env := do(t, r, http.MethodPost, "/api/v1/sessions/"+row.ID.String()+"/cancel",
		`{"reason":"nghỉ lễ"}`, token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != apperror.CodeConflict {
		t.Fatalf("cancel with confirmed attendance must be 409, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodDelete, "/api/v1/sessions/"+row.ID.String(), "", token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != apperror.CodeConflict {
		t.Fatalf("delete with confirmed attendance must be 409, got %d %+v", w.Code, env)
	}
}

func TestDeleteThenGetIs404(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)
	row := generateOne(t, r, classID, token)

	w, env := do(t, r, http.MethodDelete, "/api/v1/sessions/"+row.ID.String(), "", token)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/sessions/"+row.ID.String(), "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("get after delete must be 404, got %d %+v", w.Code, env)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, _ := setUpClass(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet, "/api/v1/sessions/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

// TestPendingRouteIsNotShadowedByGetByID guards route registration order:
// GET /sessions/pending must resolve to the pending feed, not be swallowed by
// GET /sessions/:id (which would otherwise try to uuid.Parse("pending") and
// answer 404, exactly like TestGetRejectsMalformedID does for other garbage).
func TestPendingRouteIsNotShadowedByGetByID(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, _ := setUpClass(deps)
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet, "/api/v1/sessions/pending", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /sessions/pending must resolve to the feed (want 200, got %d %+v)", w.Code, env)
	}
	var out PendingResponse
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}

func TestCrossTenantGetIs404(t *testing.T) {
	r, deps := newSessionsHTTPTest(t)
	teacherID, classID := setUpClass(deps)
	token := mintToken(t, teacherID)
	row := generateOne(t, r, classID, token)

	stranger := uuid.New()
	deps.teachers.addTeacher(stranger, "Asia/Ho_Chi_Minh")
	strangerToken := mintToken(t, stranger)

	w, env := do(t, r, http.MethodGet, "/api/v1/sessions/"+row.ID.String(), "", strangerToken)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %d %+v", w.Code, env)
	}
}
