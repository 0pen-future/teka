package paths

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// fakeRepo is an in-memory Repository that honours the center boundary the
// way the SQL does: a row of another center reads as not found.
type fakeRepo struct {
	paths  map[uuid.UUID]*LearningPath
	stages map[uuid.UUID]*Stage
	// links holds each stage's course rows in position order.
	links map[uuid.UUID][]StageCourse
	// courses stands in for the courses table the SQL joins and locks.
	courses map[uuid.UUID]fakeCourse
}

type fakeCourse struct {
	centerID   uuid.UUID
	code, name string
	status     string
	deleted    bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		paths:   map[uuid.UUID]*LearningPath{},
		stages:  map[uuid.UUID]*Stage{},
		links:   map[uuid.UUID][]StageCourse{},
		courses: map[uuid.UUID]fakeCourse{},
	}
}

// course registers a live course of the given center and returns its id.
func (f *fakeRepo) course(center uuid.UUID, code string) uuid.UUID {
	id := uuid.New()
	f.courses[id] = fakeCourse{centerID: center, code: code, name: "Khóa " + code, status: "active"}
	return id
}

func (f *fakeRepo) clash(sc authctx.Scope, id uuid.UUID, code string) bool {
	for _, other := range f.paths {
		if other.ID != id && other.CenterID == sc.CenterID && other.DeletedAt == nil && other.Code == code {
			return true
		}
	}
	return false
}

func (f *fakeRepo) live(sc authctx.Scope, id uuid.UUID) (*LearningPath, bool) {
	p, ok := f.paths[id]
	if !ok || p.CenterID != sc.CenterID || p.DeletedAt != nil {
		return nil, false
	}
	return p, true
}

func (f *fakeRepo) Create(_ context.Context, sc authctx.Scope, p *LearningPath) error {
	if f.clash(sc, uuid.Nil, p.Code) {
		return gorm.ErrDuplicatedKey
	}
	p.ID, p.CenterID = uuid.New(), sc.CenterID
	p.CreatedAt, p.UpdatedAt = nowUTC(), nowUTC()
	cp := *p
	f.paths[p.ID] = &cp
	return nil
}

func (f *fakeRepo) Get(_ context.Context, sc authctx.Scope, id uuid.UUID) (*LearningPath, error) {
	p, ok := f.live(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakeRepo) Lock(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*LearningPath, error) {
	if ctx.Value(fakeTxKey{}) == nil {
		return nil, errors.New("Lock called outside a transaction")
	}
	return f.Get(ctx, sc, id)
}

func (f *fakeRepo) List(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]LearningPath, int64, error) {
	var out []LearningPath
	for _, p := range f.paths {
		if p.CenterID != sc.CenterID || p.DeletedAt != nil {
			continue
		}
		if filter.Status != "" && p.Status != filter.Status {
			continue
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, int64(len(out)), nil
}

func (f *fakeRepo) Update(_ context.Context, sc authctx.Scope, p *LearningPath) error {
	cur, ok := f.live(sc, p.ID)
	if !ok {
		return ErrNotFound
	}
	if f.clash(sc, p.ID, p.Code) {
		return gorm.ErrDuplicatedKey
	}
	cur.Code, cur.Name, cur.Description, cur.Status, cur.UpdatedAt = p.Code, p.Name, p.Description, p.Status, nowUTC()
	return nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	cur, ok := f.live(sc, id)
	if !ok {
		return ErrNotFound
	}
	now := nowUTC()
	cur.DeletedAt = &now
	return nil
}

func (f *fakeRepo) ListStages(_ context.Context, sc authctx.Scope, pathIDs []uuid.UUID) ([]Stage, error) {
	var out []Stage
	for _, pid := range pathIDs {
		for _, st := range f.stages {
			if st.PathID == pid && st.CenterID == sc.CenterID {
				out = append(out, *st)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PathID != out[j].PathID {
			return out[i].PathID.String() < out[j].PathID.String()
		}
		return out[i].Position < out[j].Position
	})
	return out, nil
}

func (f *fakeRepo) GetStage(_ context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) (*Stage, error) {
	st, ok := f.stages[stageID]
	if !ok || st.PathID != pathID || st.CenterID != sc.CenterID {
		return nil, ErrStageNotFound
	}
	cp := *st
	return &cp, nil
}

func (f *fakeRepo) CreateStage(_ context.Context, sc authctx.Scope, st *Stage) error {
	st.ID, st.CenterID = uuid.New(), sc.CenterID
	cp := *st
	f.stages[st.ID] = &cp
	return nil
}

func (f *fakeRepo) UpdateStage(_ context.Context, sc authctx.Scope, st *Stage) error {
	cur, ok := f.stages[st.ID]
	if !ok || cur.PathID != st.PathID || cur.CenterID != sc.CenterID {
		return ErrStageNotFound
	}
	cur.Name, cur.Goal = st.Name, st.Goal
	return nil
}

func (f *fakeRepo) DeleteStage(_ context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) error {
	cur, ok := f.stages[stageID]
	if !ok || cur.PathID != pathID || cur.CenterID != sc.CenterID {
		return ErrStageNotFound
	}
	delete(f.stages, stageID)
	delete(f.links, stageID)
	return nil
}

func (f *fakeRepo) SetStagePositions(_ context.Context, sc authctx.Scope, pathID uuid.UUID, ids []uuid.UUID) error {
	for i, id := range ids {
		cur, ok := f.stages[id]
		if !ok || cur.PathID != pathID || cur.CenterID != sc.CenterID {
			return ErrStageNotFound
		}
		cur.Position = i + 1
	}
	return nil
}

func (f *fakeRepo) ListStageCourses(_ context.Context, sc authctx.Scope, stageIDs []uuid.UUID) ([]StageCourseRow, error) {
	var out []StageCourseRow
	for _, sid := range stageIDs {
		for _, l := range f.links[sid] {
			c, ok := f.courses[l.CourseID]
			if !ok || c.deleted || l.CenterID != sc.CenterID {
				continue
			}
			out = append(out, StageCourseRow{StageID: sid, Position: l.Position, CourseID: l.CourseID, Code: c.code, Name: c.name, Status: c.status})
		}
	}
	return out, nil
}

func (f *fakeRepo) LockCourses(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]uuid.UUID, error) {
	if ctx.Value(fakeTxKey{}) == nil {
		return nil, errors.New("LockCourses called outside a transaction")
	}
	var found []uuid.UUID
	for _, id := range ids {
		if c, ok := f.courses[id]; ok && !c.deleted && c.centerID == sc.CenterID {
			found = append(found, id)
		}
	}
	return found, nil
}

func (f *fakeRepo) ReplaceStageCourses(_ context.Context, sc authctx.Scope, stageID uuid.UUID, courseIDs []uuid.UUID) error {
	rows := make([]StageCourse, 0, len(courseIDs))
	for i, id := range courseIDs {
		rows = append(rows, StageCourse{StageID: stageID, CourseID: id, CenterID: sc.CenterID, Position: i + 1})
	}
	f.links[stageID] = rows
	return nil
}

// fakeTx runs fn inline and counts outermost calls so tests can assert a
// multi-row change was wrapped. Like the real manager, a nested call joins
// the ambient transaction instead of opening another.
type fakeTx struct{ calls int }

type fakeTxKey struct{}

func (f *fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx.Value(fakeTxKey{}) != nil {
		return fn(ctx)
	}
	f.calls++
	return fn(context.WithValue(ctx, fakeTxKey{}, true))
}

type testDeps struct {
	repo   *fakeRepo
	tx     *fakeTx
	center uuid.UUID
	owner  uuid.UUID
}

func newTestService() (*Service, *testDeps) {
	d := &testDeps{repo: newFakeRepo(), tx: &fakeTx{}, center: uuid.New(), owner: uuid.New()}
	return NewService(d.repo, d.tx), d
}

func (d *testDeps) ownerScope() authctx.Scope {
	return authctx.Scope{TeacherID: d.owner, CenterID: d.center, IsOwner: true}
}

// memberWith builds a member of the same center holding exactly keys.
func (d *testDeps) memberWith(keys ...string) authctx.Scope {
	return authctx.Scope{TeacherID: uuid.New(), CenterID: d.center, Perms: authctx.BuildPermSet(nil, keys, nil)}
}

func requireAppError(t *testing.T, err error, status int, code string) {
	t.Helper()
	appErr := appErrorOf(t, err, status)
	if code != "" && appErr.Code != code {
		t.Fatalf("want code %s, got %s", code, appErr.Code)
	}
}

// appErrorOf asserts the status and hands the error back so a test can
// inspect its details.
func appErrorOf(t *testing.T, err error, status int) *apperror.AppError {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want *apperror.AppError with status %d, got %v", status, err)
	}
	if appErr.Status != status {
		t.Fatalf("want status %d, got %d (%s: %s)", status, appErr.Status, appErr.Code, appErr.Message)
	}
	return appErr
}

func str(s string) *string { return &s }

func nowUTC() time.Time { return time.Now().UTC() }

func mustPath(t *testing.T, svc *Service, sc authctx.Scope, code string) *PathResponse {
	t.Helper()
	out, err := svc.Create(context.Background(), sc, PathRequest{Code: code, Name: "Lộ trình " + code})
	if err != nil {
		t.Fatalf("create path: %v", err)
	}
	return out
}

func mustStage(t *testing.T, svc *Service, sc authctx.Scope, pathID uuid.UUID, name string) *PathResponse {
	t.Helper()
	out, err := svc.CreateStage(context.Background(), sc, pathID, StageRequest{Name: name})
	if err != nil {
		t.Fatalf("create stage %s: %v", name, err)
	}
	return out
}

func stageIDs(p *PathResponse) []uuid.UUID {
	ids := make([]uuid.UUID, len(p.Stages))
	for i := range p.Stages {
		ids[i] = p.Stages[i].ID
	}
	return ids
}

func TestCreateNormalisesAndDefaultsToDraft(t *testing.T) {
	svc, d := newTestService()
	out, err := svc.Create(context.Background(), d.ownerScope(), PathRequest{
		Code: " lo-trinh-6 ", Name: "  Lộ trình lớp 6  ", Description: str("  "),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Code != "LO-TRINH-6" || out.Name != "Lộ trình lớp 6" || out.Status != StatusDraft {
		t.Fatalf("unexpected path %+v", out)
	}
	if out.Description != nil {
		t.Fatalf("blank description must be null, got %q", *out.Description)
	}
	if out.Stages == nil || len(out.Stages) != 0 || out.StageCount != 0 || out.CourseCount != 0 {
		t.Fatalf("stages must serialise as [] with zero counts: %+v", out)
	}
}

func TestCreateRejectsBadCodeAndClash(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	_, err := svc.Create(context.Background(), sc, PathRequest{Code: "a", Name: "x"})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["code"] == "" {
		t.Fatalf("want a field error on code, got %+v", appErr.Fields)
	}
	_, err = svc.Create(context.Background(), sc, PathRequest{Code: "LT-1", Name: "   "})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["name"] == "" {
		t.Fatalf("whitespace name must land on name, got %+v", appErr.Fields)
	}
	p := mustPath(t, svc, sc, "LT-1")
	_, err = svc.Create(context.Background(), sc, PathRequest{Code: "lt-1", Name: "Trùng"})
	requireAppError(t, err, http.StatusConflict, CodeCodeTaken)

	// A soft-deleted path frees its code.
	if err := svc.Delete(context.Background(), sc, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Create(context.Background(), sc, PathRequest{Code: "LT-1", Name: "Lại"}); err != nil {
		t.Fatalf("code of a deleted path must be reusable: %v", err)
	}
}

func TestUpdateReplacesFieldsAndKeepsBlankStatus(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	a, err := svc.Create(context.Background(), sc, PathRequest{Code: "A1", Name: "A", Status: StatusActive})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mustPath(t, svc, sc, "B1")
	out, err := svc.Update(context.Background(), sc, a.ID, PathRequest{Code: "a2", Name: "Đổi tên", Description: str(" Mô tả ")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Code != "A2" || out.Name != "Đổi tên" || out.Status != StatusActive || out.Description == nil || *out.Description != "Mô tả" {
		t.Fatalf("unexpected %+v", out)
	}
	out, err = svc.Update(context.Background(), sc, a.ID, PathRequest{Code: "A2", Name: "Đổi tên", Status: StatusArchived})
	if err != nil || out.Status != StatusArchived {
		t.Fatalf("explicit status must apply: %v %+v", err, out)
	}
	_, err = svc.Update(context.Background(), sc, a.ID, PathRequest{Code: "B1", Name: "Trùng"})
	requireAppError(t, err, http.StatusConflict, CodeCodeTaken)
	_, err = svc.Update(context.Background(), sc, uuid.New(), PathRequest{Code: "C1", Name: "Không có"})
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestPermissionsGateReadsAndWrites(t *testing.T) {
	svc, d := newTestService()
	owner := d.ownerScope()
	p := mustPath(t, svc, owner, "LT-1")
	reader := d.memberWith(authctx.PermPathsRead)
	editor := d.memberWith(authctx.PermPathsEdit)
	nobody := d.memberWith()

	if _, err := svc.Get(context.Background(), reader, p.ID); err != nil {
		t.Fatalf("reader get: %v", err)
	}
	_, err := svc.Get(context.Background(), nobody, p.ID)
	requireAppError(t, err, http.StatusForbidden, "")
	_, _, err = svc.List(context.Background(), nobody, ListFilter{}, pagination.Params{})
	requireAppError(t, err, http.StatusForbidden, "")

	_, err = svc.Create(context.Background(), reader, PathRequest{Code: "LT-2", Name: "x"})
	requireAppError(t, err, http.StatusForbidden, "")
	_, err = svc.CreateStage(context.Background(), reader, p.ID, StageRequest{Name: "Giai đoạn 1"})
	requireAppError(t, err, http.StatusForbidden, "")
	_, err = svc.ReorderStages(context.Background(), reader, p.ID, ReorderRequest{StageIDs: []uuid.UUID{uuid.New()}})
	requireAppError(t, err, http.StatusForbidden, "")
	_, err = svc.SetStageCourses(context.Background(), reader, p.ID, uuid.New(), StageCoursesRequest{})
	requireAppError(t, err, http.StatusForbidden, "")
	err = svc.Delete(context.Background(), reader, p.ID)
	requireAppError(t, err, http.StatusForbidden, "")

	// paths.edit implies paths.read through the catalog, so the editor
	// reads back what it wrote.
	out, err := svc.CreateStage(context.Background(), editor, p.ID, StageRequest{Name: "Giai đoạn 1"})
	if err != nil || len(out.Stages) != 1 {
		t.Fatalf("editor create stage: %v %+v", err, out)
	}
}

func TestStageLifecycleKeepsPositionsDense(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	p := mustPath(t, svc, sc, "LT-1")
	mustStage(t, svc, sc, p.ID, "Nền tảng")
	mustStage(t, svc, sc, p.ID, "Nâng cao")
	out := mustStage(t, svc, sc, p.ID, "  Luyện thi  ")
	if len(out.Stages) != 3 || out.StageCount != 3 {
		t.Fatalf("want 3 stages, got %+v", out)
	}
	for i, st := range out.Stages {
		if st.Position != i+1 {
			t.Fatalf("stage %d has position %d", i, st.Position)
		}
		if st.Courses == nil {
			t.Fatalf("courses must serialise as []: %+v", st)
		}
	}
	if out.Stages[2].Name != "Luyện thi" {
		t.Fatalf("stage name must be trimmed: %q", out.Stages[2].Name)
	}
	if d.tx.calls != 3 {
		t.Fatalf("each stage create must run in its own transaction, got %d", d.tx.calls)
	}

	mid := out.Stages[1].ID
	out, err := svc.UpdateStage(context.Background(), sc, p.ID, mid, StageRequest{Name: "Trung cấp", Goal: str(" Nắm vững ")})
	if err != nil {
		t.Fatalf("update stage: %v", err)
	}
	if out.Stages[1].Name != "Trung cấp" || out.Stages[1].Goal == nil || *out.Stages[1].Goal != "Nắm vững" {
		t.Fatalf("stage not updated: %+v", out.Stages[1])
	}
	_, err = svc.UpdateStage(context.Background(), sc, p.ID, mid, StageRequest{Name: "  "})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["name"] == "" {
		t.Fatalf("blank stage name must land on name: %+v", appErr.Fields)
	}

	// Deleting the middle stage renumbers the rest so positions stay 1..n.
	out, err = svc.DeleteStage(context.Background(), sc, p.ID, mid)
	if err != nil {
		t.Fatalf("delete stage: %v", err)
	}
	if len(out.Stages) != 2 || out.Stages[0].Name != "Nền tảng" || out.Stages[0].Position != 1 ||
		out.Stages[1].Name != "Luyện thi" || out.Stages[1].Position != 2 {
		t.Fatalf("positions must be renumbered: %+v", out.Stages)
	}
	_, err = svc.DeleteStage(context.Background(), sc, p.ID, mid)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestStageOfAnotherPathIsNotFound(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	a := mustPath(t, svc, sc, "A1")
	b := mustPath(t, svc, sc, "B1")
	a = mustStage(t, svc, sc, a.ID, "Giai đoạn A")
	sid := a.Stages[0].ID

	_, err := svc.UpdateStage(context.Background(), sc, b.ID, sid, StageRequest{Name: "x"})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.DeleteStage(context.Background(), sc, b.ID, sid)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.SetStageCourses(context.Background(), sc, b.ID, sid, StageCoursesRequest{})
	requireAppError(t, err, http.StatusNotFound, "")
	// A path of another center is invisible, stages included.
	other := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}
	_, err = svc.CreateStage(context.Background(), other, a.ID, StageRequest{Name: "x"})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.Get(context.Background(), other, a.ID)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestReorderStagesRequiresTheExactSet(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	p := mustPath(t, svc, sc, "LT-1")
	mustStage(t, svc, sc, p.ID, "Một")
	mustStage(t, svc, sc, p.ID, "Hai")
	p = mustStage(t, svc, sc, p.ID, "Ba")
	ids := stageIDs(p)

	for name, bad := range map[string][]uuid.UUID{
		"missing":   {ids[0], ids[1]},
		"duplicate": {ids[0], ids[0], ids[1]},
		"foreign":   {ids[0], ids[1], uuid.New()},
	} {
		_, err := svc.ReorderStages(context.Background(), sc, p.ID, ReorderRequest{StageIDs: bad})
		if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["stage_ids"] == "" {
			t.Fatalf("%s: want field error on stage_ids, got %+v", name, appErr.Fields)
		}
	}
	calls := d.tx.calls
	out, err := svc.ReorderStages(context.Background(), sc, p.ID, ReorderRequest{StageIDs: []uuid.UUID{ids[2], ids[0], ids[1]}})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if out.Stages[0].Name != "Ba" || out.Stages[1].Name != "Một" || out.Stages[2].Name != "Hai" {
		t.Fatalf("unexpected order %+v", out.Stages)
	}
	for i, st := range out.Stages {
		if st.Position != i+1 {
			t.Fatalf("stage %s has position %d", st.Name, st.Position)
		}
	}
	if d.tx.calls != calls+1 {
		t.Fatalf("reorder must run in one transaction, got %d new", d.tx.calls-calls)
	}
	_, err = svc.ReorderStages(context.Background(), sc, uuid.New(), ReorderRequest{StageIDs: ids})
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestSetStageCoursesValidatesAndReplaces(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	p := mustPath(t, svc, sc, "LT-1")
	mustStage(t, svc, sc, p.ID, "Một")
	p = mustStage(t, svc, sc, p.ID, "Hai")
	s1, s2 := p.Stages[0].ID, p.Stages[1].ID
	toan := d.repo.course(d.center, "TOAN-6")
	van := d.repo.course(d.center, "VAN-6")
	foreign := d.repo.course(uuid.New(), "TOAN-6")
	deleted := d.repo.course(d.center, "CU")
	fc := d.repo.courses[deleted]
	fc.deleted = true
	d.repo.courses[deleted] = fc

	for name, bad := range map[string][]uuid.UUID{
		"duplicate": {toan, toan},
		"foreign":   {toan, foreign},
		"deleted":   {deleted},
		"missing":   {uuid.New()},
	} {
		_, err := svc.SetStageCourses(context.Background(), sc, p.ID, s1, StageCoursesRequest{CourseIDs: bad})
		if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["course_ids"] == "" {
			t.Fatalf("%s: want field error on course_ids, got %+v", name, appErr.Fields)
		}
	}
	many := make([]uuid.UUID, 0, maxStageCourses+1)
	for i := 0; i <= maxStageCourses; i++ {
		many = append(many, d.repo.course(d.center, "K"+uuid.NewString()[:6]))
	}
	_, err := svc.SetStageCourses(context.Background(), sc, p.ID, s1, StageCoursesRequest{CourseIDs: many})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["course_ids"] == "" {
		t.Fatalf("over the cap: want field error on course_ids, got %+v", appErr.Fields)
	}

	calls := d.tx.calls
	out, err := svc.SetStageCourses(context.Background(), sc, p.ID, s1, StageCoursesRequest{CourseIDs: []uuid.UUID{van, toan}})
	if err != nil {
		t.Fatalf("set courses: %v", err)
	}
	if d.tx.calls != calls+1 {
		t.Fatalf("set courses must run in one transaction, got %d new", d.tx.calls-calls)
	}
	got := out.Stages[0].Courses
	if len(got) != 2 || got[0].ID != van || got[0].Position != 1 || got[0].Code != "VAN-6" || got[1].ID != toan || got[1].Position != 2 {
		t.Fatalf("courses must keep body order with positions 1..n: %+v", got)
	}
	if out.CourseCount != 2 {
		t.Fatalf("course_count: want 2, got %d", out.CourseCount)
	}

	// The same course may sit in two stages; course_count counts it once.
	out, err = svc.SetStageCourses(context.Background(), sc, p.ID, s2, StageCoursesRequest{CourseIDs: []uuid.UUID{toan}})
	if err != nil {
		t.Fatalf("set courses on second stage: %v", err)
	}
	if out.CourseCount != 2 || len(out.Stages[1].Courses) != 1 {
		t.Fatalf("distinct course_count: want 2, got %+v", out)
	}

	// [] clears the stage.
	out, err = svc.SetStageCourses(context.Background(), sc, p.ID, s1, StageCoursesRequest{CourseIDs: []uuid.UUID{}})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(out.Stages[0].Courses) != 0 || out.CourseCount != 1 {
		t.Fatalf("clear must empty the stage: %+v", out)
	}
	// A path of another center is invisible.
	other := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}
	_, err = svc.SetStageCourses(context.Background(), other, p.ID, s1, StageCoursesRequest{CourseIDs: []uuid.UUID{toan}})
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestListFillsStagesAndCountsPerPath(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	a := mustPath(t, svc, sc, "A1")
	mustPath(t, svc, sc, "B1")
	a = mustStage(t, svc, sc, a.ID, "Một")
	toan := d.repo.course(d.center, "TOAN-6")
	if _, err := svc.SetStageCourses(context.Background(), sc, a.ID, a.Stages[0].ID, StageCoursesRequest{CourseIDs: []uuid.UUID{toan}}); err != nil {
		t.Fatalf("set courses: %v", err)
	}
	rows, total, err := svc.List(context.Background(), sc, ListFilter{}, pagination.Params{PerPage: 20})
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("list: %v total=%d rows=%d", err, total, len(rows))
	}
	byCode := map[string]PathResponse{}
	for _, r := range rows {
		byCode[r.Code] = r
	}
	if byCode["A1"].StageCount != 1 || byCode["A1"].CourseCount != 1 || len(byCode["A1"].Stages[0].Courses) != 1 {
		t.Fatalf("A1 must carry its stage and course: %+v", byCode["A1"])
	}
	if byCode["B1"].StageCount != 0 || byCode["B1"].Stages == nil {
		t.Fatalf("B1 must carry an empty stage list: %+v", byCode["B1"])
	}
	rows, _, err = svc.List(context.Background(), sc, ListFilter{Status: StatusActive}, pagination.Params{PerPage: 20})
	if err != nil || len(rows) != 0 {
		t.Fatalf("status filter: %v %d", err, len(rows))
	}
}

func TestDeleteSoftDeletesAndHidesThePath(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	p := mustPath(t, svc, sc, "LT-1")
	p = mustStage(t, svc, sc, p.ID, "Một")
	if err := svc.Delete(context.Background(), sc, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if d.repo.paths[p.ID].DeletedAt == nil {
		t.Fatalf("delete must stamp deleted_at, not drop the row")
	}
	_, err := svc.Get(context.Background(), sc, p.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.CreateStage(context.Background(), sc, p.ID, StageRequest{Name: "Hai"})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.UpdateStage(context.Background(), sc, p.ID, p.Stages[0].ID, StageRequest{Name: "Hai"})
	requireAppError(t, err, http.StatusNotFound, "")
	err = svc.Delete(context.Background(), sc, p.ID)
	requireAppError(t, err, http.StatusNotFound, "")
}
