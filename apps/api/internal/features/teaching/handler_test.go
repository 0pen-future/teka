package teaching

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

const handlerTestSecret = "teaching-test-secret-0123456789abcdef"

// fakeScopeResolver resolves a teacher as the owner of their own center
// unless registered as a member of someone else's — the review endpoints
// need a non-owner caller to exercise their 403s.
type fakeScopeResolver struct {
	members map[uuid.UUID]uuid.UUID // teacherID → centerID, resolved as non-owner
}

func (f fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	if centerID, ok := f.members[teacherID]; ok {
		return authctx.Scope{TeacherID: teacherID, CenterID: centerID, IsOwner: false}, nil
	}
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

// newTeachingHTTPTest wires the real routes and auth middleware over the
// in-memory fakes.
func newTeachingHTTPTest(t *testing.T) (*gin.Engine, *testDeps, fakeScopeResolver) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, deps := newTestService()
	resolver := fakeScopeResolver{members: map[uuid.UUID]uuid.UUID{}}
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc), middleware.RequireAuth(jwtCfg), middleware.ResolveScope(resolver))
	return r, deps, resolver
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

// httpSetUp registers one owner-teacher (fakeScopeResolver: center = own id)
// with a class and a two-lesson curriculum.
func httpSetUp(deps *testDeps) (teacherID, classID uuid.UUID) {
	teacherID = uuid.New()
	class := deps.classes.addClass(teacherID, teacherID)
	deps.repo.curricula[class.ID] = &Curriculum{
		ClassID: class.ID, TeacherID: teacherID, CenterID: teacherID,
		Lessons: StringList{"Bài 1", "Bài 2"}, CurrentIndex: 0,
	}
	return teacherID, class.ID
}

func TestTeachingRoutesRequireAuth(t *testing.T) {
	r, _, _ := newTeachingHTTPTest(t)
	someID := uuid.NewString()
	planPath := "/api/v1/classes/" + someID + "/lesson-plans/0"

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/classes/" + someID + "/curriculum"},
		{http.MethodPut, "/api/v1/classes/" + someID + "/curriculum"},
		{http.MethodGet, "/api/v1/classes/" + someID + "/lesson-plans"},
		{http.MethodPut, planPath},
		{http.MethodPost, planPath + "/submit"},
		{http.MethodPost, planPath + "/approve"},
		{http.MethodPost, planPath + "/request-redo"},
		{http.MethodPost, planPath + "/reopen"},
		{http.MethodGet, "/api/v1/teaching/review-queue"},
		{http.MethodGet, "/api/v1/classes/" + someID + "/marks"},
		{http.MethodPut, "/api/v1/sessions/" + someID + "/note"},
		{http.MethodPut, "/api/v1/sessions/" + someID + "/marks"},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestGetCurriculumDefaultOverHTTP(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID := uuid.New()
	class := deps.classes.addClass(teacherID, teacherID) // no curriculum saved
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet, "/api/v1/classes/"+class.ID.String()+"/curriculum", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var out CurriculumResponse
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out.Lessons == nil || len(out.Lessons) != 0 || out.CurrentIndex != 0 {
		t.Fatalf("want the empty default with a non-null lessons array, got %+v", out)
	}
}

func TestMalformedPathParamsAre404(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID, classID := httpSetUp(deps)
	token := mintToken(t, teacherID)

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/classes/not-a-uuid/curriculum", ""},
		{http.MethodPut, "/api/v1/classes/" + classID.String() + "/lesson-plans/abc", `{"goal":"g"}`},
		{http.MethodPut, "/api/v1/classes/" + classID.String() + "/lesson-plans/-1", `{"goal":"g"}`},
	}
	for _, tc := range cases {
		w, env := do(t, r, tc.method, tc.path, tc.body, token)
		if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
			t.Fatalf("%s %s: want 404, got %d %+v", tc.method, tc.path, w.Code, env)
		}
	}
}

// The full happy loop over real HTTP: save → submit → approve, statuses and
// review fields visible in each response.
func TestLessonPlanReviewLoopOverHTTP(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID, classID := httpSetUp(deps)
	token := mintToken(t, teacherID)
	planPath := "/api/v1/classes/" + classID.String() + "/lesson-plans/0"

	w, env := do(t, r, http.MethodPut, planPath, `{"goal":"Ngữ pháp thì quá khứ","activities":["warm-up",""],"homework":"bài 3"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("save: want 200, got %d %+v", w.Code, env)
	}
	var plan PlanResponse
	if err := json.Unmarshal(env.Data, &plan); err != nil {
		t.Fatalf("decode save: %v", err)
	}
	if plan.Status != StatusDraft || len(plan.Activities) != 1 {
		t.Fatalf("save must create a draft with cleaned activities, got %+v", plan)
	}

	w, env = do(t, r, http.MethodPost, planPath+"/submit", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: want 200, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &plan); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	if plan.Status != StatusPending || plan.SubmittedBy == nil {
		t.Fatalf("submit must mark pending and stamp the submitter, got %+v", plan)
	}

	w, env = do(t, r, http.MethodPost, planPath+"/approve", `{"comment":"tốt"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: want 200, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &plan); err != nil {
		t.Fatalf("decode approve: %v", err)
	}
	if plan.Status != StatusApproved || plan.OwnerComment == nil || *plan.OwnerComment != "tốt" {
		t.Fatalf("approve must mark approved with the comment, got %+v", plan)
	}

	// A second submit from approved has no legal transition.
	w, env = do(t, r, http.MethodPost, planPath+"/submit", "", token)
	if w.Code != http.StatusConflict || env.Error == nil || env.Error.Code != apperror.CodeConflict {
		t.Fatalf("submit from approved must be 409, got %d %+v", w.Code, env)
	}
}

func TestRequestRedoWithoutCommentIs422OverHTTP(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID, classID := httpSetUp(deps)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	seedPlan(deps.repo, sc, classID, 0, StatusPending)
	token := mintToken(t, teacherID)

	path := "/api/v1/classes/" + classID.String() + "/lesson-plans/0/request-redo"
	w, env := do(t, r, http.MethodPost, path, `{"comment":""}`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["comment"] == "" {
		t.Fatalf("redo without comment must be 422 with a comment field error, got %d %+v", w.Code, env)
	}
}

// A member (non-owner) hitting the owner surfaces gets 403 from the review
// queue and from a review action on a pending plan in their own class.
func TestMemberGets403OnOwnerSurfaces(t *testing.T) {
	r, deps, resolver := newTeachingHTTPTest(t)
	ownerID, _ := httpSetUp(deps)
	memberID := uuid.New()
	resolver.members[memberID] = ownerID // member of the owner's center
	memberClass := deps.classes.addClass(memberID, ownerID)
	deps.repo.curricula[memberClass.ID] = &Curriculum{ClassID: memberClass.ID, TeacherID: memberID, CenterID: ownerID, Lessons: StringList{"Bài 1"}}
	memberScope := authctx.Scope{TeacherID: memberID, CenterID: ownerID, IsOwner: false}
	seedPlan(deps.repo, memberScope, memberClass.ID, 0, StatusPending)
	token := mintToken(t, memberID)

	w, env := do(t, r, http.MethodGet, "/api/v1/teaching/review-queue", "", token)
	if w.Code != http.StatusForbidden || env.Error == nil || env.Error.Code != apperror.CodeForbidden {
		t.Fatalf("member review queue must be 403, got %d %+v", w.Code, env)
	}

	path := "/api/v1/classes/" + memberClass.ID.String() + "/lesson-plans/0/approve"
	w, env = do(t, r, http.MethodPost, path, `{}`, token)
	if w.Code != http.StatusForbidden || env.Error == nil || env.Error.Code != apperror.CodeForbidden {
		t.Fatalf("member approve must be 403, got %d %+v", w.Code, env)
	}
}

// The tri-state marks contract can only be proven over real JSON — an omitted
// key, an explicit null, and a value must each bind differently through
// Optional's UnmarshalJSON.
func TestMarksTriStateOverHTTP(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID, classID := httpSetUp(deps)
	session := deps.sessions.addSession(teacherID, teacherID, classID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	student := uuid.New()
	deps.roster.enroll(classID, student)
	token := mintToken(t, teacherID)
	marksPath := "/api/v1/sessions/" + session.ID.String() + "/marks"

	// Value sets.
	w, env := do(t, r, http.MethodPut, marksPath, `[{"student_id":"`+student.String()+`","score":8.5}]`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("set score: want 200, got %d %+v", w.Code, env)
	}
	var marks []MarkResponse
	if err := json.Unmarshal(env.Data, &marks); err != nil {
		t.Fatalf("decode marks: %v", err)
	}
	if len(marks) != 1 || marks[0].Score == nil || *marks[0].Score != 8.5 {
		t.Fatalf("want score 8.5, got %+v", marks)
	}

	// Omitted key leaves the other field untouched.
	w, env = do(t, r, http.MethodPut, marksPath, `[{"student_id":"`+student.String()+`","personal_note":"chăm"}]`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("set note: want 200, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &marks); err != nil {
		t.Fatalf("decode marks: %v", err)
	}
	if len(marks) != 1 || marks[0].Score == nil || *marks[0].Score != 8.5 || marks[0].PersonalNote == nil {
		t.Fatalf("omitted score key must leave the score, got %+v", marks)
	}

	// Explicit nulls clear both — the row disappears.
	w, env = do(t, r, http.MethodPut, marksPath, `[{"student_id":"`+student.String()+`","score":null,"personal_note":null}]`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("clear both: want 200, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &marks); err != nil {
		t.Fatalf("decode marks: %v", err)
	}
	if len(marks) != 0 {
		t.Fatalf("both-null row must be gone, got %+v", marks)
	}

	// Off-roster student is a 422 on marks.
	w, env = do(t, r, http.MethodPut, marksPath, `[{"student_id":"`+uuid.NewString()+`","score":5}]`, token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["marks"] == "" {
		t.Fatalf("off-roster student must be 422 on marks, got %d %+v", w.Code, env)
	}
}

// The note round-trip and the month batch read over HTTP, including the
// missing/malformed month query being a 422.
func TestNoteAndMonthMarksOverHTTP(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID, classID := httpSetUp(deps)
	session := deps.sessions.addSession(teacherID, teacherID, classID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	token := mintToken(t, teacherID)
	notePath := "/api/v1/sessions/" + session.ID.String() + "/note"
	marksPath := "/api/v1/classes/" + classID.String() + "/marks"

	w, env := do(t, r, http.MethodPut, notePath, `{"body":"Cả lớp sôi nổi"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("save note: want 200, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, marksPath, "", token)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["month"] == "" {
		t.Fatalf("missing month must be 422 on month, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodGet, marksPath+"?month=2026-08", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("month read: want 200, got %d %+v", w.Code, env)
	}
	var out MonthMarksResponse
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode month: %v", err)
	}
	if len(out.SessionNotes) != 1 || out.SessionNotes[0].SessionID != session.ID || out.SessionNotes[0].Body != "Cả lớp sôi nổi" {
		t.Fatalf("want the saved note in the month read, got %+v", out)
	}
	if out.Marks == nil || len(out.Marks) != 0 {
		t.Fatalf("want a non-null empty marks array, got %+v", out.Marks)
	}

	// Empty body deletes the note.
	w, env = do(t, r, http.MethodPut, notePath, `{"body":""}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("clear note: want 200, got %d %+v", w.Code, env)
	}
	w, env = do(t, r, http.MethodGet, marksPath+"?month=2026-08", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("re-read: want 200, got %d %+v", w.Code, env)
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode re-read: %v", err)
	}
	if len(out.SessionNotes) != 0 {
		t.Fatalf("cleared note must vanish from the month read, got %+v", out.SessionNotes)
	}
}

func TestReviewQueueOverHTTP(t *testing.T) {
	r, deps, _ := newTeachingHTTPTest(t)
	teacherID, classID := httpSetUp(deps)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	plan := seedPlan(deps.repo, sc, classID, 0, StatusPending)
	now := time.Now()
	plan.SubmittedBy = &teacherID
	plan.SubmittedAt = &now
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodGet, "/api/v1/teaching/review-queue", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %+v", w.Code, env)
	}
	var out []QueueItemResponse
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out) != 1 || out[0].ClassID != classID || out[0].LessonIndex != 0 {
		t.Fatalf("want the pending plan in the queue, got %+v", out)
	}
}
