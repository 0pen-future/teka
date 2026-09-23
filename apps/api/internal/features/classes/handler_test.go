package classes

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

const handlerTestSecret = "classes-test-secret-0123456789abcdef"

// fakeScopeResolver resolves every known teacher id to a scope where it owns
// its own center — exactly like the real resolver does for a fixture teacher
// who never joined anyone else's center.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newClassesHTTPTest wires the real routes, auth, and scope middleware over
// the in-memory fake repository.
func newClassesHTTPTest(t *testing.T) (*gin.Engine, *fakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newFakeRepository()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(NewService(repo, noopTx{}, noopStaffSeeder{})),
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

const validCreateBody = `{
	"name": "Toán 8",
	"start_date": "2026-01-05",
	"default_unit_price": 150000,
	"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]
}`

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	someID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/classes"},
		{http.MethodGet, "/api/v1/classes"},
		{http.MethodGet, "/api/v1/classes/stats"},
		{http.MethodGet, "/api/v1/classes/" + someID},
		{http.MethodPut, "/api/v1/classes/" + someID},
		{http.MethodPost, "/api/v1/classes/" + someID + "/archive"},
		{http.MethodDelete, "/api/v1/classes/" + someID},
		{http.MethodPost, "/api/v1/classes/" + someID + "/schedules"},
		{http.MethodPut, "/api/v1/classes/" + someID + "/schedules/" + someID},
		{http.MethodDelete, "/api/v1/classes/" + someID + "/schedules/" + someID},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401 UNAUTHORIZED, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	cases := map[string]struct {
		body      string
		wantField string
	}{
		"missing name": {
			`{"start_date":"2026-01-05","default_unit_price":150000,"schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"name",
		},
		"bad start_date": {
			`{"name":"Toán 8","start_date":"05/01/2026","default_unit_price":150000,"schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"start_date",
		},
		"missing price": {
			`{"name":"Toán 8","start_date":"2026-01-05","schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"default_unit_price",
		},
		"negative price": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":-1,"schedules":[{"weekday":2,"start_time":"18:00","duration_min":90}]}`,
			"default_unit_price",
		},
		"missing schedules": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000}`,
			"schedules",
		},
		"empty schedules": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000,"schedules":[]}`,
			"schedules",
		},
		"weekday out of range": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000,"schedules":[{"weekday":7,"start_time":"18:00","duration_min":90}]}`,
			"weekday",
		},
		"bad start_time": {
			`{"name":"Toán 8","start_date":"2026-01-05","default_unit_price":150000,"schedules":[{"weekday":2,"start_time":"25:99","duration_min":90}]}`,
			"start_time",
		},
	}
	for name, tc := range cases {
		w, env := do(t, r, http.MethodPost, "/api/v1/classes", tc.body, token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
			t.Fatalf("%s: want 422 VALIDATION_ERROR, got %d %+v", name, w.Code, env)
		}
		found := false
		for field := range env.Error.Fields {
			if field == tc.wantField || strings.Contains(field, tc.wantField) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: want a message for field %q, got %+v", name, tc.wantField, env.Error.Fields)
		}
	}
}

func TestCreateAcceptsFreeClass(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	body := `{"name":"Lớp miễn phí","start_date":"2026-01-05","default_unit_price":0,"schedules":[{"weekday":0,"start_time":"08:00","duration_min":60}]}`
	w, env := do(t, r, http.MethodPost, "/api/v1/classes", body, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("a 0 đồng class is legitimate, got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.DefaultUnitPrice != 0 {
		t.Fatalf("want price 0, got %d", created.DefaultUnitPrice)
	}
	if len(created.Schedules) != 1 || created.Schedules[0].Weekday != 0 {
		t.Fatalf("weekday 0 (Sunday) must survive the round trip, got %+v", created.Schedules)
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, token)
	if w.Code != http.StatusCreated || !env.Success {
		t.Fatalf("want 201, got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.DefaultUnitPrice != 150000 {
		t.Fatalf("150000 đồng must round-trip exactly, got %d", created.DefaultUnitPrice)
	}
	if created.Status != StatusActive {
		t.Fatalf("new class must be active, got %q", created.Status)
	}
	if len(created.Schedules) != 1 || created.Schedules[0].EffectiveFrom != "2026-01-05" {
		t.Fatalf("schedule effective_from must default to start_date, got %+v", created.Schedules)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/classes/"+created.ID.String(), "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
}

func TestListStatusFilter(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	teacherID := uuid.New()
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+created.ID.String()+"/archive", "", token); w.Code != http.StatusOK {
		t.Fatalf("archive: got %d %+v", w.Code, env)
	}

	listLen := func(query string) int {
		t.Helper()
		w, env := do(t, r, http.MethodGet, "/api/v1/classes"+query, "", token)
		if w.Code != http.StatusOK {
			t.Fatalf("list %q: got %d %+v", query, w.Code, env)
		}
		var rows []ClassResponse
		if err := json.Unmarshal(env.Data, &rows); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return len(rows)
	}
	if n := listLen(""); n != 0 {
		t.Fatalf("default list must exclude archived classes, got %d", n)
	}
	if n := listLen("?status=archived"); n != 1 {
		t.Fatalf("archived filter must show the class, got %d", n)
	}
	if n := listLen("?status=all"); n != 1 {
		t.Fatalf("all filter must show the class, got %d", n)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/classes?status=bogus", "", token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["status"] == "" {
		t.Fatalf("unknown status must be 422 with a status field message, got %d %+v", w.Code, env)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodGet, "/api/v1/classes/not-a-uuid", "", token)
	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("malformed id must read as 404, got %d %+v", w.Code, env)
	}
}

func TestDeleteWithOpenEnrollmentsConflicts(t *testing.T) {
	r, repo := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	repo.openEnrollments[created.ID] = 2

	w, env = do(t, r, http.MethodDelete, "/api/v1/classes/"+created.ID.String(), "", token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != apperror.CodeConflict {
		t.Fatalf("want 409 CONFLICT, got %d %+v", w.Code, env)
	}
	if !strings.Contains(env.Error.Message, "archive") {
		t.Fatalf("conflict message must suggest archiving, got %q", env.Error.Message)
	}
}

// student_count on list and get reflects the open enrollments per class;
// a class with none reads 0 rather than being absent from the page.
func TestListAndGetCarryStudentCount(t *testing.T) {
	r, repo := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	create := func(body string) ClassResponse {
		t.Helper()
		w, env := do(t, r, http.MethodPost, "/api/v1/classes", body, token)
		if w.Code != http.StatusCreated {
			t.Fatalf("create: got %d %+v", w.Code, env)
		}
		var created ClassResponse
		if err := json.Unmarshal(env.Data, &created); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return created
	}
	withStudents := create(validCreateBody)
	empty := create(strings.Replace(validCreateBody, `"Toán 8"`, `"Văn 9"`, 1))
	repo.openEnrollments[withStudents.ID] = 2

	w, env := do(t, r, http.MethodGet, "/api/v1/classes", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("list: got %d %+v", w.Code, env)
	}
	var rows []ClassResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	counts := map[uuid.UUID]int{}
	for _, row := range rows {
		counts[row.ID] = row.StudentCount
	}
	if counts[withStudents.ID] != 2 || counts[empty.ID] != 0 {
		t.Fatalf("want student_count 2 and 0, got %+v", counts)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/classes/"+withStudents.ID.String(), "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d %+v", w.Code, env)
	}
	var got ClassResponse
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.StudentCount != 2 {
		t.Fatalf("get must carry student_count 2, got %d", got.StudentCount)
	}
}

func TestListIsTenantScoped(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	owner := uuid.New()
	stranger := uuid.New()

	if w, env := do(t, r, http.MethodPost, "/api/v1/classes", validCreateBody, mintToken(t, owner)); w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}

	w, env := do(t, r, http.MethodGet, "/api/v1/classes", "", mintToken(t, stranger))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var rows []ClassResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another teacher's list must be empty, got %+v", rows)
	}
}

// createClass posts body and decodes the created class.
func createClass(t *testing.T, r *gin.Engine, token, body string) ClassResponse {
	t.Helper()
	w, env := do(t, r, http.MethodPost, "/api/v1/classes", body, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %+v", w.Code, env)
	}
	var created ClassResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return created
}

// listIDs fetches the list with query and returns the ids in page order.
func listIDs(t *testing.T, r *gin.Engine, token, query string) []uuid.UUID {
	t.Helper()
	w, env := do(t, r, http.MethodGet, "/api/v1/classes"+query, "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("list %q: got %d %+v", query, w.Code, env)
	}
	var rows []ClassResponse
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

// Filter parameters outside their enums are rejected up front with a field
// message, never silently ignored — a UI chip that sent a typo would
// otherwise show the unfiltered list as if it matched.
func TestListRejectsUnknownFilterValues(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	cases := map[string]string{
		"?weekday=7":    "weekday",
		"?weekday=-1":   "weekday",
		"?weekday=mon":  "weekday",
		"?shift=night":  "shift",
		"?phase=paused": "phase",
		"?status=bogus": "status",
		"?course_id=x":  "course_id",
	}
	for query, field := range cases {
		w, env := do(t, r, http.MethodGet, "/api/v1/classes"+query, "", token)
		if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields[field] == "" {
			t.Fatalf("%s: want 422 with a %s field message, got %d %+v", query, field, w.Code, env)
		}
	}
}

// q matches name or code case-insensitively, weekday/shift look at the
// class's still-effective timetable, tag matches one element exactly, and
// phase is derived from the dates — all combinable.
func TestListFiltersByQueryTimetableTagAndPhase(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	evening := createClass(t, r, token, `{
		"name": "Toán 8", "code": "TOAN8", "tags": ["Toán", "Khối 8"],
		"start_date": "2026-01-05", "default_unit_price": 150000,
		"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]
	}`)
	morning := createClass(t, r, token, `{
		"name": "Văn 9", "code": "VAN9", "tags": ["Văn"],
		"start_date": "2026-10-01", "default_unit_price": 150000,
		"schedules": [{"weekday": 6, "start_time": "08:00", "duration_min": 90}]
	}`)

	want := func(query string, ids ...uuid.UUID) {
		t.Helper()
		got := listIDs(t, r, token, query)
		if len(got) != len(ids) {
			t.Fatalf("%s: want %d rows, got %v", query, len(ids), got)
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Fatalf("%s: want %v, got %v", query, ids, got)
			}
		}
	}
	want("?q=toan8", evening.ID)
	want("?q=v%C4%83n", morning.ID)
	want("?q=zzz")
	want("?weekday=2", evening.ID)
	want("?weekday=2&shift=evening", evening.ID)
	want("?weekday=2&shift=morning")
	want("?shift=morning", morning.ID)
	want("?tag=Kh%E1%BB%91i%208", evening.ID)
	want("?tag=Khối")
	want("?phase=running", evening.ID)
	want("?phase=upcoming", morning.ID)
	want("?phase=upcoming&q=toan")
}

// A code the center already uses is a 409 with its own error code so the
// form can attach the message to the code field; the malformed shape is a
// plain 422.
func TestCreateCodeConflictsAndValidation(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	body := `{"name": "Toán 8", "code": "TOAN8", "start_date": "2026-01-05", "default_unit_price": 150000,
		"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]}`
	created := createClass(t, r, token, body)
	if created.Code != "TOAN8" || created.Phase != PhaseRunning || created.Tags == nil {
		t.Fatalf("response must carry code, phase and a non-null tags list, got %+v", created)
	}

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", body, token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != CodeClassCodeTaken {
		t.Fatalf("want 409 %s, got %d %+v", CodeClassCodeTaken, w.Code, env)
	}

	bad := strings.Replace(body, `"TOAN8"`, `"toán 8"`, 1)
	w, env = do(t, r, http.MethodPost, "/api/v1/classes", bad, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["code"] == "" {
		t.Fatalf("want 422 on code, got %d %+v", w.Code, env)
	}

	tooMany := strings.Replace(body, `"code": "TOAN8",`, `"code": "TOAN8B", "tags": ["1","2","3","4","5","6","7","8","9","10","11"],`, 1)
	w, env = do(t, r, http.MethodPost, "/api/v1/classes", tooMany, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["tags"] == "" {
		t.Fatalf("want 422 on tags, got %d %+v", w.Code, env)
	}
}

// PUT keeps its full-replace contract for the original fields (name is
// still required) while the catalog fields patch: a body naming only
// recruiting leaves code, tags and note untouched.
func TestUpdatePatchesCatalogFieldsOnly(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	created := createClass(t, r, token, `{
		"name": "Toán 8", "code": "TOAN8", "tags": ["Toán"], "note": "Phòng 201",
		"start_date": "2026-01-05", "default_unit_price": 150000,
		"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]
	}`)
	path := "/api/v1/classes/" + created.ID.String()

	w, env := do(t, r, http.MethodPut, path, `{"recruiting": true}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["name"] == "" {
		t.Fatalf("original fields stay required, want 422 on name, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPut, path, `{"name": "Toán 8", "start_date": "2026-01-05", "default_unit_price": 150000, "recruiting": true}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d %+v", w.Code, env)
	}
	var updated ClassResponse
	if err := json.Unmarshal(env.Data, &updated); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !updated.Recruiting || updated.Code != "TOAN8" || len(updated.Tags) != 1 || updated.Note == nil || *updated.Note != "Phòng 201" {
		t.Fatalf("absent catalog fields must be kept, got %+v", updated)
	}
}

// stats is its own route ahead of /:id, counts through the caller's read
// scope, and reflects a recruiting toggle immediately.
func TestStatsRoute(t *testing.T) {
	r, _ := newClassesHTTPTest(t)
	token := mintToken(t, uuid.New())

	stats := func() ClassStatsResponse {
		t.Helper()
		w, env := do(t, r, http.MethodGet, "/api/v1/classes/stats", "", token)
		if w.Code != http.StatusOK {
			t.Fatalf("stats: got %d %+v", w.Code, env)
		}
		var out ClassStatsResponse
		if err := json.Unmarshal(env.Data, &out); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return out
	}
	if got := stats(); got != (ClassStatsResponse{}) {
		t.Fatalf("empty center must count zeros, got %+v", got)
	}

	created := createClass(t, r, token, validCreateBody)
	if got := stats(); got.All != 1 || got.Running != 1 || got.Recruiting != 0 {
		t.Fatalf("one running class: got %+v", got)
	}

	w, env := do(t, r, http.MethodPut, "/api/v1/classes/"+created.ID.String(),
		`{"name": "Toán 8", "start_date": "2026-01-05", "default_unit_price": 150000, "recruiting": true}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d %+v", w.Code, env)
	}
	if got := stats(); got.Recruiting != 1 {
		t.Fatalf("recruiting toggle must count, got %+v", got)
	}

	if w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+created.ID.String()+"/archive", "", token); w.Code != http.StatusOK {
		t.Fatalf("archive: got %d %+v", w.Code, env)
	}
	if got := stats(); got.All != 1 || got.Archived != 1 || got.Running != 0 {
		t.Fatalf("archived class must move buckets, got %+v", got)
	}

	// Another caller's center must not leak into the count.
	other := mintToken(t, uuid.New())
	w, env = do(t, r, http.MethodGet, "/api/v1/classes/stats", "", other)
	var out ClassStatsResponse
	if w.Code != http.StatusOK || json.Unmarshal(env.Data, &out) != nil || out != (ClassStatsResponse{}) {
		t.Fatalf("stats must be tenant scoped, got %d %+v", w.Code, env)
	}
}

// course_id on create binds as a uuid, the response embeds the course as
// {id, code, name} (null without one), and the list filters on it.
func TestCourseAttachmentOverHTTP(t *testing.T) {
	r, repo := newClassesHTTPTest(t)
	teacher := uuid.New()
	token := mintToken(t, teacher)
	course := repo.addCourse(teacher, "TOAN-6", 180_000)

	w, env := do(t, r, http.MethodPost, "/api/v1/classes", `{
		"name": "Toán 6A", "start_date": "2026-01-05", "course_id": "not-a-uuid",
		"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]
	}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["course_id"] == "" {
		t.Fatalf("malformed course_id: want 422 on course_id, got %d %+v", w.Code, env)
	}

	attached := createClass(t, r, token, `{
		"name": "Toán 6A", "start_date": "2026-01-05", "course_id": "`+course.ID.String()+`",
		"schedules": [{"weekday": 2, "start_time": "18:00", "duration_min": 90}]
	}`)
	if attached.Course == nil || attached.Course.ID != course.ID || attached.Course.Code != "TOAN-6" || attached.DefaultUnitPrice != 180_000 {
		t.Fatalf("attached class must embed the course and copy its price, got %+v", attached)
	}
	plain := createClass(t, r, token, `{
		"name": "Văn 6", "start_date": "2026-01-05", "default_unit_price": 100000,
		"schedules": [{"weekday": 3, "start_time": "18:00", "duration_min": 90}]
	}`)
	if plain.Course != nil {
		t.Fatalf("class without course must carry null, got %+v", plain.Course)
	}
	w, _ = do(t, r, http.MethodGet, "/api/v1/classes/"+plain.ID.String(), "", token)
	if !strings.Contains(w.Body.String(), `"course":null`) {
		t.Fatalf("course must serialise as null: %s", w.Body.String())
	}

	ids := listIDs(t, r, token, "?course_id="+course.ID.String())
	if len(ids) != 1 || ids[0] != attached.ID {
		t.Fatalf("course_id filter: want only the attached class, got %v", ids)
	}
	if got := listIDs(t, r, token, ""); len(got) != 2 {
		t.Fatalf("unfiltered list: want 2, got %d", len(got))
	}

	// The patch rule must survive binding: "" detaches, garbage is a 422 on
	// the field, and a blank course_id on create means "no course".
	w, env = do(t, r, http.MethodPut, "/api/v1/classes/"+attached.ID.String(),
		`{"name": "Toán 6A", "start_date": "2026-01-05", "default_unit_price": 180000, "course_id": ""}`, token)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"course":null`) {
		t.Fatalf("empty course_id must detach: got %d %s", w.Code, w.Body.String())
	}
	w, env = do(t, r, http.MethodPut, "/api/v1/classes/"+attached.ID.String(),
		`{"name": "Toán 6A", "start_date": "2026-01-05", "default_unit_price": 180000, "course_id": "x"}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["course_id"] == "" {
		t.Fatalf("malformed course_id on update: want 422 on course_id, got %d %+v", w.Code, env)
	}
	blank := createClass(t, r, token, `{
		"name": "Lý 6", "start_date": "2026-01-05", "default_unit_price": 100000, "course_id": "",
		"schedules": [{"weekday": 4, "start_time": "18:00", "duration_min": 90}]
	}`)
	if blank.Course != nil {
		t.Fatalf("blank course_id on create must mean no course, got %+v", blank.Course)
	}
}
