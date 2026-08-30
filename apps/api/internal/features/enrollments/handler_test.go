package enrollments

import (
	"context"
	"encoding/json"
	"io"
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

const handlerTestSecret = "enrollments-test-secret-0123456789ab"

// fakeScopeResolver resolves every known teacher id to a scope where it owns
// its own center — matching the fake repository's addClass/addStudent
// convention that a self-owned teacher's center id equals their own id.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newEnrollmentsHTTPTest wires the real routes, auth, and scope middleware
// over the in-memory fake repository.
func newEnrollmentsHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(NewService(repo, nil)),
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

	return w, mustEnvelope(t, w)
}

// mustEnvelope decodes a recorder's body into the response envelope.
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

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newEnrollmentsHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/enrollments"},
		{http.MethodGet, "/api/v1/enrollments"},
		{http.MethodGet, "/api/v1/enrollments/" + someID},
		{http.MethodPost, "/api/v1/enrollments/" + someID + "/end"},
		{http.MethodDelete, "/api/v1/enrollments/" + someID},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	cases := map[string]struct {
		body      string
		wantField string
	}{
		"missing student": {`{"class_id":"` + classID.String() + `"}`, "student_id"},
		"missing class":   {`{"student_id":"` + studentID.String() + `"}`, "class_id"},
		"bad started_on":  {`{"student_id":"` + studentID.String() + `","class_id":"` + classID.String() + `","started_on":"15/01/2026"}`, "started_on"},
		"foreign class":   {`{"student_id":"` + studentID.String() + `","class_id":"` + uuid.NewString() + `"}`, "class_id"},
		"foreign student": {`{"student_id":"` + uuid.NewString() + `","class_id":"` + classID.String() + `"}`, "student_id"},
	}
	for name, tc := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/enrollments", tc.body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		if env.Error.Fields[tc.wantField] == "" {
			t.Fatalf("%s: want a message for field %q, got %+v", name, tc.wantField, env.Error.Fields)
		}
	}

	// A student_id that is not even a uuid fails JSON decoding, which the
	// shared contract reports as 400 BAD_REQUEST, not 422.
	w, env := do(t, r, http.MethodPost, "/api/v1/enrollments",
		`{"student_id":"not-a-uuid","class_id":"`+classID.String()+`"}`, token)
	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("malformed uuid must be 400 BAD_REQUEST, got %d %+v", w.Code, env)
	}
}

func TestCreateIgnoresSuppliedUnitPrice(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	// unit_price in the body is an unknown field: binding drops it, the class
	// price wins. This is the "no request path can set unit_price" criterion.
	w, env := do(t, r, http.MethodPost, "/api/v1/enrollments",
		`{"student_id":"`+studentID.String()+`","class_id":"`+classID.String()+`","started_on":"2026-01-15","unit_price":1}`, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201, got %d %+v", w.Code, env)
	}
	var created EnrollmentResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.UnitPrice != 150_000 {
		t.Fatalf("unit_price must come from the class, got %d", created.UnitPrice)
	}
	if created.StartedOn != "2026-01-15" {
		t.Fatalf("started_on must round-trip verbatim, got %s", created.StartedOn)
	}
	if created.StudentName == "" || created.ClassName == "" {
		t.Fatalf("response must carry display names, got %+v", created)
	}
}

func TestEnrollTwiceThenEndFlow(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)
	body := `{"student_id":"` + studentID.String() + `","class_id":"` + classID.String() + `","started_on":"2026-01-05"}`

	w, env := do(t, r, http.MethodPost, "/api/v1/enrollments", body, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("first enroll: want 201, got %d %+v", w.Code, env)
	}
	var created EnrollmentResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if w, env = do(t, r, http.MethodPost, "/api/v1/enrollments", body, token); w.Code != http.StatusConflict {
		t.Fatalf("second enroll: want 409, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/enrollments/"+created.ID.String()+"/end",
		`{"ended_on":"2026-01-04"}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["ended_on"] == "" {
		t.Fatalf("ended_on before started_on: want 422, got %d %+v", w.Code, env)
	}

	// Ending with no body defaults ended_on to today.
	w, env = do(t, r, http.MethodPost, "/api/v1/enrollments/"+created.ID.String()+"/end", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("end: want 200, got %d %+v", w.Code, env)
	}
	var ended EnrollmentResponse
	if err := json.Unmarshal(env.Data, &ended); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if ended.EndedOn == nil || *ended.EndedOn != today().Format(dateLayout) {
		t.Fatalf("ended_on must default to today, got %v", ended.EndedOn)
	}

	if w, env = do(t, r, http.MethodPost, "/api/v1/enrollments/"+created.ID.String()+"/end", "", token); w.Code != http.StatusConflict {
		t.Fatalf("double end: want 409, got %d %+v", w.Code, env)
	}
}

// TestEndHonorsBodyWithoutContentLength proves the end handler reads a body
// that arrives without a Content-Length header (as a chunked-encoded request
// does, where ContentLength is -1). Gating the bind on ContentLength would drop
// such a body and silently revert the departure date to today.
func TestEndHonorsBodyWithoutContentLength(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/enrollments",
		`{"student_id":"`+studentID.String()+`","class_id":"`+classID.String()+`","started_on":"2026-01-05"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("enroll: want 201, got %d %+v", w.Code, env)
	}
	var created EnrollmentResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// Build the request so ContentLength stays -1: a body reader whose concrete
	// type httptest.NewRequest does not special-case leaves the length unknown.
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/enrollments/"+created.ID.String()+"/end",
		io.NopCloser(strings.NewReader(`{"ended_on":"2026-03-31"}`)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("end with unsized body: want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var ended EnrollmentResponse
	if err := json.Unmarshal(mustEnvelope(t, rec).Data, &ended); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if ended.EndedOn == nil || *ended.EndedOn != "2026-03-31" {
		t.Fatalf("body ended_on must win over today, got %v", ended.EndedOn)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	r, _ := newEnrollmentsHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/enrollments/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

func TestListRejectsMalformedFilters(t *testing.T) {
	r, _ := newEnrollmentsHTTPTest(t)
	token := mintToken(t, uuid.New())

	for _, param := range []string{"student_id", "class_id"} {
		w, env := do(t, r, http.MethodGet, "/api/v1/enrollments?"+param+"=nope", "", token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields[param] == "" {
			t.Fatalf("malformed %s must be 422 naming the field, got %d %+v", param, w.Code, env)
		}
	}
	w, env := do(t, r, http.MethodGet, "/api/v1/enrollments?active=maybe", "", token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["active"] == "" {
		t.Fatalf("malformed active must be 422, got %d %+v", w.Code, env)
	}
}

func TestListFiltersByActive(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	classID := repo.addClass(teacherID, 150_000)
	open := repo.addStudent(teacherID)
	departed := repo.addStudent(teacherID)

	if w, env := do(t, r, http.MethodPost, "/api/v1/enrollments",
		`{"student_id":"`+open.String()+`","class_id":"`+classID.String()+`","started_on":"2026-01-05"}`, token); w.Code != http.StatusCreated {
		t.Fatalf("enroll open: got %d %+v", w.Code, env)
	}
	w, env := do(t, r, http.MethodPost, "/api/v1/enrollments",
		`{"student_id":"`+departed.String()+`","class_id":"`+classID.String()+`","started_on":"2026-01-05"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("enroll departed: got %d %+v", w.Code, env)
	}
	var departedRow EnrollmentResponse
	if err := json.Unmarshal(env.Data, &departedRow); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if w, env = do(t, r, http.MethodPost, "/api/v1/enrollments/"+departedRow.ID.String()+"/end",
		`{"ended_on":"2026-02-28"}`, token); w.Code != http.StatusOK {
		t.Fatalf("end departed: got %d %+v", w.Code, env)
	}

	count := func(query string) int {
		t.Helper()
		w, env := do(t, r, http.MethodGet, "/api/v1/enrollments"+query, "", token)
		if w.Code != http.StatusOK {
			t.Fatalf("list %q: got %d %+v", query, w.Code, env)
		}
		var rows []EnrollmentResponse
		if err := json.Unmarshal(env.Data, &rows); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return len(rows)
	}
	if got := count(""); got != 2 {
		t.Fatalf("default list must show both, got %d", got)
	}
	if got := count("?active=true"); got != 1 {
		t.Fatalf("active=true must show the open one, got %d", got)
	}
	if got := count("?active=false"); got != 1 {
		t.Fatalf("active=false must show the ended one, got %d", got)
	}
	if got := count("?student_id=" + open.String()); got != 1 {
		t.Fatalf("student filter must narrow, got %d", got)
	}
}

func TestListIsTenantScoped(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	owner := uuid.New()
	classID := repo.addClass(owner, 150_000)
	studentID := repo.addStudent(owner)

	if w, env := do(t, r, http.MethodPost, "/api/v1/enrollments",
		`{"student_id":"`+studentID.String()+`","class_id":"`+classID.String()+`"}`, mintToken(t, owner)); w.Code != http.StatusCreated {
		t.Fatalf("enroll: got %d %+v", w.Code, env)
	}

	w, env := do(t, r, http.MethodGet, "/api/v1/enrollments", "", mintToken(t, uuid.New()))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []EnrollmentResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another teacher's list must be empty, got %+v", rows)
	}
}

// The picker response is names only: each row carries exactly id and
// full_name — no phone, no contact — and a short query yields an empty list
// rather than an error.
func TestEnrollableStudentsResponseCarriesNamesOnly(t *testing.T) {
	r, repo := newEnrollmentsHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)
	classID := repo.addClass(teacherID, 150_000)
	repo.addStudent(teacherID)

	w, env := do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/enrollable-students?q=an", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("data is not a list: %v\nbody: %s", err, w.Body.String())
	}
	if len(rows) != 1 {
		t.Fatalf("want the one matching student, got %d rows", len(rows))
	}
	for key := range rows[0] {
		if key != "id" && key != "full_name" {
			t.Fatalf("picker rows must carry id and full_name only, found %q", key)
		}
	}

	w, env = do(t, r, http.MethodGet,
		"/api/v1/classes/"+classID.String()+"/enrollable-students?q=a", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("short q: want 200 empty, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &rows); err != nil || len(rows) != 0 {
		t.Fatalf("short q must yield an empty list, got %s (err %v)", env.Data, err)
	}
}
