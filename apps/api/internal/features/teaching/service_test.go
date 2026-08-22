package teaching

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// --- fake ClassSource ---

// fakeClassSource mimics classes.Service.Get's scoping: a class outside the
// caller's center — or another teacher's, for a non-owner — resolves as not
// found, never as forbidden.
type fakeClassSource struct {
	rows map[uuid.UUID]*classes.Class
}

func newFakeClassSource() *fakeClassSource {
	return &fakeClassSource{rows: map[uuid.UUID]*classes.Class{}}
}

func (f *fakeClassSource) addClass(teacherID, centerID uuid.UUID) *classes.Class {
	class := &classes.Class{ID: id.New(), TeacherID: teacherID, CenterID: centerID}
	f.rows[class.ID] = class
	return class
}

func (f *fakeClassSource) Get(_ context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	class, ok := f.rows[classID]
	if !ok || class.CenterID != sc.CenterID {
		return nil, classes.ErrNotFound
	}
	if !sc.IsOwner && class.TeacherID != sc.TeacherID {
		return nil, classes.ErrNotFound
	}
	return class, nil
}

// --- fake SessionSource ---

// fakeSessionSource mimics sessions.Service.GetByID's scoping: a session
// outside the caller's center — or another teacher's, for a non-owner —
// resolves as not found, never as forbidden.
type fakeSessionSource struct {
	rows map[uuid.UUID]*sessions.Session
}

func newFakeSessionSource() *fakeSessionSource {
	return &fakeSessionSource{rows: map[uuid.UUID]*sessions.Session{}}
}

func (f *fakeSessionSource) addSession(teacherID, centerID, classID uuid.UUID, date time.Time) *sessions.Session {
	session := &sessions.Session{ID: id.New(), TeacherID: teacherID, CenterID: centerID, ClassID: classID, SessionDate: date}
	f.rows[session.ID] = session
	return session
}

func (f *fakeSessionSource) GetByID(_ context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error) {
	session, ok := f.rows[sessionID]
	if !ok || session.CenterID != sc.CenterID {
		return nil, sessions.ErrNotFound
	}
	if !sc.IsOwner && session.TeacherID != sc.TeacherID {
		return nil, sessions.ErrNotFound
	}
	return session, nil
}

// --- fake RosterSource ---

type fakeRosterSource struct {
	byClass map[uuid.UUID][]uuid.UUID
}

func newFakeRosterSource() *fakeRosterSource {
	return &fakeRosterSource{byClass: map[uuid.UUID][]uuid.UUID{}}
}

func (f *fakeRosterSource) enroll(classID, studentID uuid.UUID) {
	f.byClass[classID] = append(f.byClass[classID], studentID)
}

func (f *fakeRosterSource) ActiveOn(_ context.Context, _ authctx.Scope, classID uuid.UUID, _ time.Time) ([]enrollments.Enrollment, error) {
	var out []enrollments.Enrollment
	for _, studentID := range f.byClass[classID] {
		out = append(out, enrollments.Enrollment{StudentID: studentID})
	}
	return out, nil
}

// passthroughTx satisfies database.TxManager without a database — the unit
// tests assert merge outcomes, not transactionality.
type passthroughTx struct{}

func (passthroughTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// --- fake Repository ---

type fakeRepository struct {
	curricula map[uuid.UUID]*Curriculum
	plans     map[string]*Plan
	names     map[uuid.UUID]string
	notes     map[uuid.UUID]*SessionNote
	marks     map[string]*SessionMark
	// sessions lets the month reads reproduce the class/date join the real
	// repository does in SQL.
	sessions *fakeSessionSource
	// createPlanErr, when set, is returned by CreatePlan — the hook for
	// simulating the concurrent-first-save unique violation.
	createPlanErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		curricula: map[uuid.UUID]*Curriculum{},
		plans:     map[string]*Plan{},
		names:     map[uuid.UUID]string{},
		notes:     map[uuid.UUID]*SessionNote{},
		marks:     map[string]*SessionMark{},
	}
}

func markKey(sessionID, studentID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", sessionID, studentID)
}

func planKey(classID uuid.UUID, index int) string {
	return fmt.Sprintf("%s#%d", classID, index)
}

func (f *fakeRepository) GetCurriculum(_ context.Context, _ authctx.Scope, classID uuid.UUID) (*Curriculum, error) {
	cur, ok := f.curricula[classID]
	if !ok {
		return nil, nil
	}
	copied := *cur
	return &copied, nil
}

func (f *fakeRepository) UpsertCurriculum(_ context.Context, cur *Curriculum) error {
	if existing, ok := f.curricula[cur.ClassID]; ok {
		existing.Lessons = cur.Lessons
		existing.CurrentIndex = cur.CurrentIndex
		return nil
	}
	copied := *cur
	f.curricula[cur.ClassID] = &copied
	return nil
}

func (f *fakeRepository) ListPlans(_ context.Context, _ authctx.Scope, classID uuid.UUID) ([]Plan, error) {
	var out []Plan
	for _, plan := range f.plans {
		if plan.ClassID == classID {
			out = append(out, *plan)
		}
	}
	return out, nil
}

func (f *fakeRepository) GetPlan(_ context.Context, _ authctx.Scope, classID uuid.UUID, index int) (*Plan, error) {
	plan, ok := f.plans[planKey(classID, index)]
	if !ok {
		return nil, nil
	}
	copied := *plan
	return &copied, nil
}

func (f *fakeRepository) CreatePlan(_ context.Context, plan *Plan) error {
	if f.createPlanErr != nil {
		return f.createPlanErr
	}
	key := planKey(plan.ClassID, plan.LessonIndex)
	if _, ok := f.plans[key]; ok {
		return gorm.ErrDuplicatedKey
	}
	plan.CreatedAt = time.Now()
	copied := *plan
	f.plans[key] = &copied
	return nil
}

func (f *fakeRepository) UpdatePlan(_ context.Context, plan *Plan) error {
	copied := *plan
	f.plans[planKey(plan.ClassID, plan.LessonIndex)] = &copied
	return nil
}

func (f *fakeRepository) ReviewQueue(_ context.Context, sc authctx.Scope) ([]QueueRow, error) {
	var rows []QueueRow
	for _, plan := range f.plans {
		if plan.CenterID == sc.CenterID && plan.Status == StatusPending {
			rows = append(rows, QueueRow{PlanID: plan.ID, ClassID: plan.ClassID, LessonIndex: plan.LessonIndex, SubmittedAt: plan.SubmittedAt})
		}
	}
	return rows, nil
}

func (f *fakeRepository) ListNotesForClassMonth(_ context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]SessionNote, error) {
	var out []SessionNote
	for _, note := range f.notes {
		session, ok := f.sessions.rows[note.SessionID]
		if !ok || note.CenterID != sc.CenterID || session.ClassID != classID {
			continue
		}
		if session.SessionDate.Before(from) || !session.SessionDate.Before(to) {
			continue
		}
		out = append(out, *note)
	}
	return out, nil
}

func (f *fakeRepository) ListMarksForClassMonth(_ context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]SessionMark, error) {
	var out []SessionMark
	for _, mark := range f.marks {
		session, ok := f.sessions.rows[mark.SessionID]
		if !ok || mark.CenterID != sc.CenterID || session.ClassID != classID {
			continue
		}
		if session.SessionDate.Before(from) || !session.SessionDate.Before(to) {
			continue
		}
		out = append(out, *mark)
	}
	return out, nil
}

func (f *fakeRepository) UpsertNote(_ context.Context, note *SessionNote) error {
	copied := *note
	f.notes[note.SessionID] = &copied
	return nil
}

func (f *fakeRepository) DeleteNote(_ context.Context, sc authctx.Scope, sessionID uuid.UUID) error {
	if note, ok := f.notes[sessionID]; ok && note.CenterID == sc.CenterID {
		delete(f.notes, sessionID)
	}
	return nil
}

func (f *fakeRepository) ListMarksBySession(_ context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]SessionMark, error) {
	var out []SessionMark
	for _, mark := range f.marks {
		if mark.SessionID == sessionID && mark.CenterID == sc.CenterID {
			out = append(out, *mark)
		}
	}
	return out, nil
}

func (f *fakeRepository) UpsertMarks(_ context.Context, marks []SessionMark) error {
	for _, mark := range marks {
		copied := mark
		f.marks[markKey(mark.SessionID, mark.StudentID)] = &copied
	}
	return nil
}

func (f *fakeRepository) DeleteMarks(_ context.Context, sc authctx.Scope, sessionID uuid.UUID, studentIDs []uuid.UUID) error {
	for _, studentID := range studentIDs {
		key := markKey(sessionID, studentID)
		if mark, ok := f.marks[key]; ok && mark.CenterID == sc.CenterID {
			delete(f.marks, key)
		}
	}
	return nil
}

func (f *fakeRepository) TeacherNames(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, teacherID := range ids {
		if name, ok := f.names[teacherID]; ok {
			out[teacherID] = name
		}
	}
	return out, nil
}

// --- harness ---

type testDeps struct {
	repo     *fakeRepository
	classes  *fakeClassSource
	sessions *fakeSessionSource
	roster   *fakeRosterSource
}

func newTestService() (*Service, *testDeps) {
	deps := &testDeps{
		repo:     newFakeRepository(),
		classes:  newFakeClassSource(),
		sessions: newFakeSessionSource(),
		roster:   newFakeRosterSource(),
	}
	deps.repo.sessions = deps.sessions
	return NewService(deps.repo, deps.classes, deps.sessions, deps.roster, passthroughTx{}), deps
}

func ptr[T any](v T) *T { return &v }

func setTo[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: &v} }

func clearIt[T any]() Optional[T] { return Optional[T]{Set: true} }

// ownerSetUp wires one teacher who owns their own center with one class and
// a one-lesson curriculum — the single actor can both edit (class teacher)
// and review (owner), which is exactly the self-approval the product allows.
func ownerSetUp(deps *testDeps) (authctx.Scope, uuid.UUID) {
	teacherID := uuid.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: uuid.New(), IsOwner: true}
	class := deps.classes.addClass(teacherID, sc.CenterID)
	deps.repo.curricula[class.ID] = &Curriculum{
		ID: id.New(), ClassID: class.ID, TeacherID: teacherID, CenterID: sc.CenterID,
		Lessons: StringList{"Bài 1"}, CurrentIndex: 0,
	}
	return sc, class.ID
}

func seedPlan(deps *fakeRepository, sc authctx.Scope, classID uuid.UUID, index int, status string) *Plan {
	plan := &Plan{
		ID: id.New(), ClassID: classID, LessonIndex: index,
		TeacherID: sc.TeacherID, CenterID: sc.CenterID,
		Goal: "goal", Activities: StringList{"a"}, Homework: "hw",
		Status: status, CreatedAt: time.Now(),
	}
	deps.plans[planKey(classID, index)] = plan
	return plan
}

// requireAppError asserts err is an *apperror.AppError carrying the given HTTP
// status and returns it so callers can make further assertions on its fields.
func requireAppError(t *testing.T, err error, status int) *apperror.AppError {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want *apperror.AppError, got %v", err)
	}
	if appErr.Status != status {
		t.Fatalf("want HTTP %d, got %d (%v)", status, appErr.Status, appErr)
	}
	return appErr
}

// wantAppError asserts err is an *apperror.AppError with the given HTTP status
// when the caller does not need the error itself.
func wantAppError(t *testing.T, err error, status int) {
	t.Helper()
	_ = requireAppError(t, err, status)
}

// TestPlanTransitionMatrix exercises every status × action combination
// against the web store's matrix — the whole point of porting the table
// verbatim. Legal moves land on the expected status; every other combination
// is a 409, including all actions on a never-saved plan except save.
func TestPlanTransitionMatrix(t *testing.T) {
	statuses := []string{StatusNone, StatusDraft, StatusPending, StatusApproved, StatusRedo}
	actions := []string{ActionSave, ActionSubmit, ActionApprove, ActionRequestRedo, ActionReopen}
	// The store's matrix, restated independently so a typo in the production
	// table cannot silently agree with itself.
	legal := map[string]map[string]string{
		StatusNone:     {ActionSave: StatusDraft},
		StatusDraft:    {ActionSave: StatusDraft, ActionSubmit: StatusPending},
		StatusPending:  {ActionApprove: StatusApproved, ActionRequestRedo: StatusRedo},
		StatusRedo:     {ActionSave: StatusRedo, ActionSubmit: StatusPending, ActionReopen: StatusPending},
		StatusApproved: {ActionReopen: StatusPending},
	}

	for _, status := range statuses {
		for _, action := range actions {
			t.Run(status+"/"+action, func(t *testing.T) {
				svc, deps := newTestService()
				sc, classID := ownerSetUp(deps)
				ctx := context.Background()
				if status != StatusNone {
					seedPlan(deps.repo, sc, classID, 0, status)
				}

				var resp *PlanResponse
				var err error
				switch action {
				case ActionSave:
					resp, err = svc.SavePlan(ctx, sc, classID, 0, SavePlanRequest{Goal: "g"})
				case ActionSubmit:
					resp, err = svc.SubmitPlan(ctx, sc, classID, 0)
				case ActionApprove:
					resp, err = svc.ApprovePlan(ctx, sc, classID, 0, ReviewRequest{})
				case ActionRequestRedo:
					resp, err = svc.RequestRedo(ctx, sc, classID, 0, ReviewRequest{Comment: "sửa lại"})
				case ActionReopen:
					resp, err = svc.ReopenPlan(ctx, sc, classID, 0)
				}

				want, isLegal := legal[status][action]
				if !isLegal {
					wantAppError(t, err, http.StatusConflict)
					return
				}
				if err != nil {
					t.Fatalf("legal %s from %s failed: %v", action, status, err)
				}
				if resp.Status != want {
					t.Fatalf("%s from %s: want status %q, got %q", action, status, want, resp.Status)
				}
			})
		}
	}
}

// The redo round-trip's field semantics, ported from the store: the redo
// note survives a save (stays visible until resubmission) and is consumed by
// submit, which also stamps the submitter.
func TestRedoNoteLifecycle(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	note := "thiếu mục tiêu"
	plan := seedPlan(deps.repo, sc, classID, 0, StatusRedo)
	plan.RedoNote = &note

	saved, err := svc.SavePlan(ctx, sc, classID, 0, SavePlanRequest{Goal: "sửa rồi"})
	if err != nil {
		t.Fatalf("save under redo: %v", err)
	}
	if saved.Status != StatusRedo || saved.RedoNote == nil || *saved.RedoNote != note {
		t.Fatalf("save under redo must keep status redo and the owner's note, got %+v", saved)
	}

	submitted, err := svc.SubmitPlan(ctx, sc, classID, 0)
	if err != nil {
		t.Fatalf("submit from redo: %v", err)
	}
	if submitted.RedoNote != nil {
		t.Fatal("submit must consume the redo note")
	}
	if submitted.SubmittedBy == nil || *submitted.SubmittedBy != sc.TeacherID || submitted.SubmittedAt == nil {
		t.Fatalf("submit must stamp submitter and time, got %+v", submitted)
	}
}

// Approve's comment is optional and whole-replaces owner_comment; reopen
// clears both owner notes so the next review starts fresh.
func TestOwnerCommentSemantics(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	seedPlan(deps.repo, sc, classID, 0, StatusPending)

	approved, err := svc.ApprovePlan(ctx, sc, classID, 0, ReviewRequest{Comment: "  tốt lắm  "})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.OwnerComment == nil || *approved.OwnerComment != "tốt lắm" {
		t.Fatalf("approve must store the trimmed comment, got %+v", approved.OwnerComment)
	}

	reopened, err := svc.ReopenPlan(ctx, sc, classID, 0)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.OwnerComment != nil || reopened.RedoNote != nil {
		t.Fatal("reopen must clear both owner notes")
	}

	// Approving again with an empty comment clears the previous one — the
	// panel has a single comment box and empty means "nothing to say".
	if _, err := svc.ApprovePlan(ctx, sc, classID, 0, ReviewRequest{}); err != nil {
		t.Fatalf("re-approve: %v", err)
	}
	stored := deps.repo.plans[planKey(classID, 0)]
	if stored.OwnerComment != nil {
		t.Fatal("empty approve comment must clear owner_comment")
	}
}

// A redo request without an actionable comment is rejected before the state
// machine runs — whitespace-only included.
func TestRequestRedoRequiresComment(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	seedPlan(deps.repo, sc, classID, 0, StatusPending)

	_, err := svc.RequestRedo(ctx, sc, classID, 0, ReviewRequest{Comment: "   "})
	appErr := requireAppError(t, err, http.StatusUnprocessableEntity)
	if _, ok := appErr.Fields["comment"]; !ok {
		t.Fatalf("want a comment field error, got %+v", appErr.Fields)
	}
	if deps.repo.plans[planKey(classID, 0)].Status != StatusPending {
		t.Fatal("a rejected redo request must not move the plan")
	}

	got, err := svc.RequestRedo(ctx, sc, classID, 0, ReviewRequest{Comment: "bổ sung bài tập"})
	if err != nil {
		t.Fatalf("request redo: %v", err)
	}
	if got.Status != StatusRedo || got.RedoNote == nil || *got.RedoNote != "bổ sung bài tập" {
		t.Fatalf("redo must store the comment as redo_note, got %+v", got)
	}
}

// The three review actions and the queue are owner-only — a plain member
// gets 403 even on their own class's plan.
func TestReviewActionsAreOwnerOnly(t *testing.T) {
	svc, deps := newTestService()
	ownerScope, _ := ownerSetUp(deps)
	memberID := uuid.New()
	memberScope := authctx.Scope{TeacherID: memberID, CenterID: ownerScope.CenterID, IsOwner: false}
	class := deps.classes.addClass(memberID, ownerScope.CenterID)
	deps.repo.curricula[class.ID] = &Curriculum{ID: id.New(), ClassID: class.ID, TeacherID: memberID, CenterID: ownerScope.CenterID, Lessons: StringList{"Bài 1"}}
	seedPlan(deps.repo, memberScope, class.ID, 0, StatusPending)
	ctx := context.Background()

	if _, err := svc.ApprovePlan(ctx, memberScope, class.ID, 0, ReviewRequest{}); err == nil {
		t.Fatal("member approve must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
	if _, err := svc.RequestRedo(ctx, memberScope, class.ID, 0, ReviewRequest{Comment: "x"}); err == nil {
		t.Fatal("member request-redo must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
	if _, err := svc.ReopenPlan(ctx, memberScope, class.ID, 0); err == nil {
		t.Fatal("member reopen must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
	if _, err := svc.ReviewQueue(ctx, memberScope); err == nil {
		t.Fatal("member review queue must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
}

// The owner reads center-wide but never edits another teacher's content:
// curriculum replace, plan save, and submit are all 403 on a member's class.
func TestContentWritesRequireClassTeacher(t *testing.T) {
	svc, deps := newTestService()
	ownerScope, _ := ownerSetUp(deps)
	memberID := uuid.New()
	memberClass := deps.classes.addClass(memberID, ownerScope.CenterID)
	deps.repo.curricula[memberClass.ID] = &Curriculum{ID: id.New(), ClassID: memberClass.ID, TeacherID: memberID, CenterID: ownerScope.CenterID, Lessons: StringList{"Bài 1"}}
	ctx := context.Background()

	if _, err := svc.PutCurriculum(ctx, ownerScope, memberClass.ID, PutCurriculumRequest{Lessons: []string{"x"}}); err == nil {
		t.Fatal("owner curriculum write on a member's class must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
	if _, err := svc.SavePlan(ctx, ownerScope, memberClass.ID, 0, SavePlanRequest{Goal: "g"}); err == nil {
		t.Fatal("owner plan save on a member's class must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
	if _, err := svc.SubmitPlan(ctx, ownerScope, memberClass.ID, 0); err == nil {
		t.Fatal("owner submit on a member's class must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}

	// Reading stays allowed — that is the owner's oversight.
	if _, err := svc.GetCurriculum(ctx, ownerScope, memberClass.ID); err != nil {
		t.Fatalf("owner read of a member's curriculum must work: %v", err)
	}
}

// A peer (non-owner) cannot even resolve another teacher's class — 404, not
// 403, so the class's existence leaks nothing.
func TestPeerSeesNotFound(t *testing.T) {
	svc, deps := newTestService()
	ownerScope, classID := ownerSetUp(deps)
	peerScope := authctx.Scope{TeacherID: uuid.New(), CenterID: ownerScope.CenterID, IsOwner: false}

	_, err := svc.GetCurriculum(context.Background(), peerScope, classID)
	wantAppError(t, err, http.StatusNotFound)
}

// Plan writes are bounded by the curriculum the class actually has.
func TestLessonIndexValidation(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps) // curriculum has exactly 1 lesson
	ctx := context.Background()

	_, err := svc.SavePlan(ctx, sc, classID, 1, SavePlanRequest{Goal: "g"})
	appErr := requireAppError(t, err, http.StatusUnprocessableEntity)
	if _, ok := appErr.Fields["lesson_index"]; !ok {
		t.Fatalf("want a lesson_index field error, got %+v", appErr.Fields)
	}

	// A class with no curriculum accepts no plan at all.
	bare := deps.classes.addClass(sc.TeacherID, sc.CenterID)
	_, err = svc.SavePlan(ctx, sc, bare.ID, 0, SavePlanRequest{Goal: "g"})
	wantAppError(t, err, http.StatusUnprocessableEntity)
}

// GET returns the empty default for a class that never saved a curriculum;
// PUT clamps the pointer into the new list's range exactly like the store.
func TestCurriculumDefaultAndClamp(t *testing.T) {
	svc, deps := newTestService()
	sc, _ := ownerSetUp(deps)
	ctx := context.Background()
	bare := deps.classes.addClass(sc.TeacherID, sc.CenterID)

	got, err := svc.GetCurriculum(ctx, sc, bare.ID)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if len(got.Lessons) != 0 || got.CurrentIndex != 0 {
		t.Fatalf("want empty default, got %+v", got)
	}

	saved, err := svc.PutCurriculum(ctx, sc, bare.ID, PutCurriculumRequest{Lessons: []string{"a", "b", "c"}, CurrentIndex: 10})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if saved.CurrentIndex != 2 {
		t.Fatalf("pointer must clamp to len-1, got %d", saved.CurrentIndex)
	}

	emptied, err := svc.PutCurriculum(ctx, sc, bare.ID, PutCurriculumRequest{Lessons: []string{}, CurrentIndex: 5})
	if err != nil {
		t.Fatalf("put empty: %v", err)
	}
	if emptied.CurrentIndex != 0 {
		t.Fatalf("empty list must pin the pointer to 0, got %d", emptied.CurrentIndex)
	}
}

// Save trims activities like the web editor: blank lines drop, order holds.
func TestSaveCleansActivities(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)

	got, err := svc.SavePlan(context.Background(), sc, classID, 0, SavePlanRequest{
		Goal:       "g",
		Activities: []string{"  warm-up  ", "", "   ", "game"},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if strings.Join(got.Activities, "|") != "warm-up|game" {
		t.Fatalf("want cleaned activities, got %v", got.Activities)
	}
}

// The marks batch's tri-state merge, walked field by field: an omitted field
// leaves the stored value, null clears it, and a row whose resulting fields
// are both NULL is deleted — the classbook writes scores without touching
// personal notes and the student record does the reverse, so partial writes
// must never clobber the other surface's data.
func TestMarksMergeSemantics(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	session := deps.sessions.addSession(sc.TeacherID, sc.CenterID, classID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	student := uuid.New()
	deps.roster.enroll(classID, student)

	// Set score only.
	got, err := svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: setTo(8.5)}})
	if err != nil {
		t.Fatalf("set score: %v", err)
	}
	if len(got) != 1 || got[0].Score == nil || *got[0].Score != 8.5 || got[0].PersonalNote != nil {
		t.Fatalf("want score 8.5 and no note, got %+v", got)
	}

	// Set note only — the score must survive untouched.
	got, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, PersonalNote: setTo("chăm phát biểu")}})
	if err != nil {
		t.Fatalf("set note: %v", err)
	}
	if len(got) != 1 || got[0].Score == nil || *got[0].Score != 8.5 || got[0].PersonalNote == nil || *got[0].PersonalNote != "chăm phát biểu" {
		t.Fatalf("note write must not clobber the score, got %+v", got)
	}

	// Clear the score — the note must survive.
	got, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: clearIt[float64]()}})
	if err != nil {
		t.Fatalf("clear score: %v", err)
	}
	if len(got) != 1 || got[0].Score != nil || got[0].PersonalNote == nil {
		t.Fatalf("clearing the score must keep the note, got %+v", got)
	}

	// Clear the note too — both NULL, so the row is deleted.
	got, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, PersonalNote: clearIt[string]()}})
	if err != nil {
		t.Fatalf("clear note: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("both-NULL row must be deleted, got %+v", got)
	}
	if len(deps.repo.marks) != 0 {
		t.Fatal("stored row must be gone")
	}

	// Clearing a student who never had a row is a quiet no-op, not an insert.
	got, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: clearIt[float64](), PersonalNote: clearIt[string]()}})
	if err != nil {
		t.Fatalf("clear absent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("clearing an absent row must not create one, got %+v", got)
	}
}

// The batch's shape guards: a student off the session's roster, a duplicate
// student, a score outside the 0–10 scale, an over-long personal note, and an
// oversized batch are each a 422 on "marks", and none of them writes anything.
func TestMarksBatchValidation(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	session := deps.sessions.addSession(sc.TeacherID, sc.CenterID, classID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	enrolled := uuid.New()
	deps.roster.enroll(classID, enrolled)

	stranger := uuid.New()
	_, err := svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: stranger, Score: setTo(7.0)}})
	appErr := requireAppError(t, err, http.StatusUnprocessableEntity)
	if _, ok := appErr.Fields["marks"]; !ok {
		t.Fatalf("want a marks field error, got %+v", appErr.Fields)
	}

	_, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{
		{StudentID: enrolled, Score: setTo(7.0)},
		{StudentID: enrolled, Score: setTo(8.0)},
	})
	wantAppError(t, err, http.StatusUnprocessableEntity)

	_, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: enrolled, Score: setTo(10.5)}})
	wantAppError(t, err, http.StatusUnprocessableEntity)

	_, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{
		{StudentID: enrolled, PersonalNote: setTo(strings.Repeat("chăm ", 250))},
	})
	wantAppError(t, err, http.StatusUnprocessableEntity)

	oversized := make([]MarkEntryRequest, 101)
	for i := range oversized {
		oversized[i] = MarkEntryRequest{StudentID: uuid.New(), Score: setTo(7.0)}
	}
	_, err = svc.PutMarks(ctx, sc, session.ID, oversized)
	wantAppError(t, err, http.StatusUnprocessableEntity)

	if len(deps.repo.marks) != 0 {
		t.Fatal("rejected batches must write nothing")
	}
}

// The roster gate only guards row creation: after a student's enrollment
// ends, their existing mark row stays correctable and clearable — a wrong
// score must never become immutable history — while a student with no row
// and no enrollment is still refused.
func TestMarksExistingRowSurvivesUnenrollment(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	session := deps.sessions.addSession(sc.TeacherID, sc.CenterID, classID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	student := uuid.New()
	deps.roster.enroll(classID, student)

	if _, err := svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: setTo(3.0)}}); err != nil {
		t.Fatalf("seed mark: %v", err)
	}

	// The student leaves the class; their row must stay editable.
	deps.roster.byClass[classID] = nil

	got, err := svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: setTo(8.0)}})
	if err != nil {
		t.Fatalf("edit after unenrollment: %v", err)
	}
	if len(got) != 1 || got[0].Score == nil || *got[0].Score != 8.0 {
		t.Fatalf("want corrected score 8.0, got %+v", got)
	}

	got, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: clearIt[float64]()}})
	if err != nil {
		t.Fatalf("clear after unenrollment: %v", err)
	}
	if len(got) != 0 || len(deps.repo.marks) != 0 {
		t.Fatalf("clearing must delete the row, got %+v", got)
	}

	// The row is gone — writing the student again would create a NEW row,
	// which requires roster membership.
	_, err = svc.PutMarks(ctx, sc, session.ID, []MarkEntryRequest{{StudentID: student, Score: setTo(7.0)}})
	wantAppError(t, err, http.StatusUnprocessableEntity)
}

// Losing a concurrent first-save race (two clients both read "no row", the
// second insert hits uq_lesson_plans_class_lesson) is the client's ordinary
// stale-view situation — a 409 that triggers its reload path, not a 500.
func TestSavePlanConcurrentCreateConflict(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	deps.repo.createPlanErr = gorm.ErrDuplicatedKey

	_, err := svc.SavePlan(context.Background(), sc, classID, 0, SavePlanRequest{Goal: "g"})
	wantAppError(t, err, http.StatusConflict)
}

// The session note round-trip: save stores the body verbatim, an empty (or
// whitespace-only) body deletes the row instead of storing an empty string,
// and deleting an absent note is idempotent.
func TestNoteEmptyBodyDeletes(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()
	session := deps.sessions.addSession(sc.TeacherID, sc.CenterID, classID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))

	saved, err := svc.PutNote(ctx, sc, session.ID, PutNoteRequest{Body: "Cả lớp học tốt"})
	if err != nil {
		t.Fatalf("save note: %v", err)
	}
	if saved.Body != "Cả lớp học tốt" {
		t.Fatalf("want verbatim body, got %q", saved.Body)
	}
	if len(deps.repo.notes) != 1 {
		t.Fatal("note row must exist")
	}

	cleared, err := svc.PutNote(ctx, sc, session.ID, PutNoteRequest{Body: "   "})
	if err != nil {
		t.Fatalf("clear note: %v", err)
	}
	if cleared.Body != "" || len(deps.repo.notes) != 0 {
		t.Fatal("empty body must delete the row")
	}

	// Idempotent: clearing again is still 200.
	if _, err := svc.PutNote(ctx, sc, session.ID, PutNoteRequest{Body: ""}); err != nil {
		t.Fatalf("clearing an absent note must be a no-op: %v", err)
	}
}

// The month read's contract: a malformed month is a 422 on "month", and a
// valid month returns only that month's rows — with non-nil empty slices when
// there is nothing, so the JSON is [] rather than null.
func TestGetMonthMarksWindowAndValidation(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	ctx := context.Background()

	for _, bad := range []string{"", "2026", "2026-13", "08-2026", "abc"} {
		_, err := svc.GetMonthMarks(ctx, sc, classID, bad)
		appErr := requireAppError(t, err, http.StatusUnprocessableEntity)
		if _, ok := appErr.Fields["month"]; !ok {
			t.Fatalf("month %q: want a month field error, got %+v", bad, appErr.Fields)
		}
	}

	empty, err := svc.GetMonthMarks(ctx, sc, classID, "2026-08")
	if err != nil {
		t.Fatalf("empty month: %v", err)
	}
	if empty.SessionNotes == nil || empty.Marks == nil {
		t.Fatal("empty month must return non-nil slices")
	}

	inMonth := deps.sessions.addSession(sc.TeacherID, sc.CenterID, classID, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))
	nextMonth := deps.sessions.addSession(sc.TeacherID, sc.CenterID, classID, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	student := uuid.New()
	deps.roster.enroll(classID, student)
	for _, session := range []uuid.UUID{inMonth.ID, nextMonth.ID} {
		if _, err := svc.PutNote(ctx, sc, session, PutNoteRequest{Body: "note"}); err != nil {
			t.Fatalf("seed note: %v", err)
		}
		if _, err := svc.PutMarks(ctx, sc, session, []MarkEntryRequest{{StudentID: student, Score: setTo(9.0)}}); err != nil {
			t.Fatalf("seed mark: %v", err)
		}
	}

	got, err := svc.GetMonthMarks(ctx, sc, classID, "2026-08")
	if err != nil {
		t.Fatalf("month read: %v", err)
	}
	if len(got.SessionNotes) != 1 || got.SessionNotes[0].SessionID != inMonth.ID {
		t.Fatalf("want only August's note, got %+v", got.SessionNotes)
	}
	if len(got.Marks) != 1 || got.Marks[0].SessionID != inMonth.ID {
		t.Fatalf("want only August's mark, got %+v", got.Marks)
	}
}

// Note/marks writes are session-teacher only: the owner resolves a member's
// session (their oversight is read) but gets 403 on both writes, while a peer
// cannot even resolve it — 404. The owner's month read of the member's class
// stays allowed.
func TestSessionWritesRequireSessionTeacher(t *testing.T) {
	svc, deps := newTestService()
	ownerScope, _ := ownerSetUp(deps)
	memberID := uuid.New()
	memberClass := deps.classes.addClass(memberID, ownerScope.CenterID)
	session := deps.sessions.addSession(memberID, ownerScope.CenterID, memberClass.ID, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	student := uuid.New()
	deps.roster.enroll(memberClass.ID, student)
	ctx := context.Background()

	if _, err := svc.PutNote(ctx, ownerScope, session.ID, PutNoteRequest{Body: "x"}); err == nil {
		t.Fatal("owner note write on a member's session must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}
	if _, err := svc.PutMarks(ctx, ownerScope, session.ID, []MarkEntryRequest{{StudentID: student, Score: setTo(5.0)}}); err == nil {
		t.Fatal("owner marks write on a member's session must fail")
	} else {
		wantAppError(t, err, http.StatusForbidden)
	}

	peerScope := authctx.Scope{TeacherID: uuid.New(), CenterID: ownerScope.CenterID, IsOwner: false}
	_, err := svc.PutNote(ctx, peerScope, session.ID, PutNoteRequest{Body: "x"})
	wantAppError(t, err, http.StatusNotFound)

	if _, err := svc.GetMonthMarks(ctx, ownerScope, memberClass.ID, "2026-08"); err != nil {
		t.Fatalf("owner month read of a member's class must work: %v", err)
	}
}

// ListPlans resolves submitter display names in one batch.
func TestListPlansResolvesSubmitterNames(t *testing.T) {
	svc, deps := newTestService()
	sc, classID := ownerSetUp(deps)
	deps.repo.names[sc.TeacherID] = "Cô Mai"
	plan := seedPlan(deps.repo, sc, classID, 0, StatusPending)
	plan.SubmittedBy = &sc.TeacherID

	got, err := svc.ListPlans(context.Background(), sc, classID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].SubmittedByName == nil || *got[0].SubmittedByName != "Cô Mai" {
		t.Fatalf("want resolved submitter name, got %+v", got)
	}
}
