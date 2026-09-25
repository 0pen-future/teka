package library

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// fakeRepo is an in-memory Repository that honours the center boundary the
// way the SQL does: a row of another center reads as ErrNotFound.
type fakeRepo struct {
	templates map[uuid.UUID]*Template
	versions  map[uuid.UUID]*Version
	lessons   map[uuid.UUID]*Lesson
	materials map[uuid.UUID]*Material
	exercises map[uuid.UUID]*Exercise
	// lessonMaterials and lessonExercises are keyed by lesson id,
	// logFields by version id; each list is kept in position order.
	lessonMaterials map[uuid.UUID][]LessonMaterial
	lessonExercises map[uuid.UUID][]LessonExercise
	logFields       map[uuid.UUID][]LogField
	// groups holds template_exercise_groups rows, keyed by id.
	groups map[uuid.UUID]*ExerciseGroup
	// locks counts LockVersion calls so tests can assert a write took the lock.
	locks int
	// inUse marks templates a class applies (class_programs rows).
	inUse map[uuid.UUID]bool
	// versionClasses stands in for the class_programs join, keyed by
	// version id; empty unless a test populates it.
	versionClasses map[uuid.UUID][]VersionClassRef
	// listVersionsCalls and listVersionsForCalls count invocations so a
	// test can assert a caller batches instead of looping per row.
	listVersionsCalls, listVersionsForCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		templates:       map[uuid.UUID]*Template{},
		versions:        map[uuid.UUID]*Version{},
		lessons:         map[uuid.UUID]*Lesson{},
		materials:       map[uuid.UUID]*Material{},
		exercises:       map[uuid.UUID]*Exercise{},
		lessonMaterials: map[uuid.UUID][]LessonMaterial{},
		lessonExercises: map[uuid.UUID][]LessonExercise{},
		logFields:       map[uuid.UUID][]LogField{},
		groups:          map[uuid.UUID]*ExerciseGroup{},
		inUse:           map[uuid.UUID]bool{},
		versionClasses:  map[uuid.UUID][]VersionClassRef{},
	}
}

func (f *fakeRepo) CreateTemplate(_ context.Context, sc authctx.Scope, t *Template) error {
	for _, other := range f.templates {
		if other.CenterID == sc.CenterID && other.DeletedAt == nil && other.Code == t.Code {
			return gorm.ErrDuplicatedKey
		}
	}
	t.ID, t.CenterID = uuid.New(), sc.CenterID
	t.CreatedAt, t.UpdatedAt = nowUTC(), nowUTC()
	cp := *t
	f.templates[t.ID] = &cp
	return nil
}

func (f *fakeRepo) summary(t *Template) *TemplateRow {
	row := &TemplateRow{Template: *t}
	var latestPublished *Version
	for _, v := range f.versions {
		if v.TemplateID != t.ID {
			continue
		}
		row.VersionCount++
		switch v.Status {
		case StatusPublished:
			if row.PublishedVersionNo == nil || *row.PublishedVersionNo < v.VersionNo {
				n := v.VersionNo
				row.PublishedVersionNo = &n
			}
			if latestPublished == nil || v.VersionNo > latestPublished.VersionNo {
				latestPublished = v
			}
		case StatusDraft:
			n := v.VersionNo
			row.DraftVersionNo = &n
			id := v.ID
			row.DraftVersionID = &id
		}
	}
	// lesson_count mirrors the SQL: the newest published version's lessons
	// when one exists — multiple published versions can coexist without
	// being summed together — else the draft's lessons.
	countVersionID := row.DraftVersionID
	if latestPublished != nil {
		id := latestPublished.ID
		countVersionID = &id
	}
	if countVersionID != nil {
		for _, l := range f.lessons {
			if l.VersionID == *countVersionID {
				row.LessonCount++
			}
		}
	}
	return row
}

func (f *fakeRepo) GetTemplate(_ context.Context, sc authctx.Scope, id uuid.UUID) (*TemplateRow, error) {
	t, ok := f.templates[id]
	if !ok || t.CenterID != sc.CenterID || t.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return f.summary(t), nil
}

func (f *fakeRepo) ListTemplates(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]TemplateRow, int64, error) {
	var out []TemplateRow
	for _, t := range f.templates {
		if t.CenterID != sc.CenterID || t.DeletedAt != nil {
			continue
		}
		row := f.summary(t)
		if filter.HasDraft && row.DraftVersionNo == nil {
			continue
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, int64(len(out)), nil
}

func (f *fakeRepo) UpdateTemplate(_ context.Context, sc authctx.Scope, t *Template) error {
	cur, ok := f.templates[t.ID]
	if !ok || cur.CenterID != sc.CenterID || cur.DeletedAt != nil {
		return ErrNotFound
	}
	for _, other := range f.templates {
		if other.ID != t.ID && other.CenterID == sc.CenterID && other.DeletedAt == nil && other.Code == t.Code {
			return gorm.ErrDuplicatedKey
		}
	}
	cur.Code, cur.Name, cur.Subject, cur.Level, cur.Description = t.Code, t.Name, t.Subject, t.Level, t.Description
	cur.UpdatedAt = nowUTC()
	return nil
}

func (f *fakeRepo) TemplateInUse(_ context.Context, sc authctx.Scope, id uuid.UUID) (bool, error) {
	return f.inUse[id] && f.templates[id] != nil && f.templates[id].CenterID == sc.CenterID, nil
}

func (f *fakeRepo) LockTemplate(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Template, error) {
	t, ok := f.templates[id]
	if !ok || t.CenterID != sc.CenterID || t.DeletedAt != nil {
		return nil, ErrNotFound
	}
	f.locks++
	cp := *t
	return &cp, nil
}

// LockTemplateForVersion mirrors the SQL join: a version whose template is
// gone or of another center reads as missing.
func (f *fakeRepo) LockTemplateForVersion(_ context.Context, sc authctx.Scope, versionID uuid.UUID) error {
	v, ok := f.versions[versionID]
	if !ok || v.CenterID != sc.CenterID {
		return ErrNotFound
	}
	tpl, ok := f.templates[v.TemplateID]
	if !ok || tpl.CenterID != sc.CenterID || tpl.DeletedAt != nil {
		return ErrNotFound
	}
	f.locks++
	return nil
}

func (f *fakeRepo) SoftDeleteTemplate(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	cur, ok := f.templates[id]
	if !ok || cur.CenterID != sc.CenterID || cur.DeletedAt != nil {
		return ErrNotFound
	}
	now := nowUTC()
	cur.DeletedAt = &now
	return nil
}

func (f *fakeRepo) CreateVersion(_ context.Context, sc authctx.Scope, v *Version) error {
	if v.Status == StatusDraft {
		for _, other := range f.versions {
			if other.TemplateID == v.TemplateID && other.Status == StatusDraft {
				return gorm.ErrDuplicatedKey
			}
		}
	}
	v.ID, v.CenterID = uuid.New(), sc.CenterID
	v.CreatedAt, v.UpdatedAt = nowUTC(), nowUTC()
	cp := *v
	f.versions[v.ID] = &cp
	return nil
}

func (f *fakeRepo) versionRow(v *Version) *VersionRow {
	row := &VersionRow{Version: *v}
	for _, l := range f.lessons {
		if l.VersionID == v.ID {
			row.LessonCount++
		}
	}
	return row
}

func (f *fakeRepo) GetVersion(_ context.Context, sc authctx.Scope, id uuid.UUID) (*VersionRow, error) {
	v, ok := f.versions[id]
	if !ok || v.CenterID != sc.CenterID {
		return nil, ErrNotFound
	}
	return f.versionRow(v), nil
}

// LockVersion mirrors the SQL join: a version whose template is gone
// reads as missing on write paths.
func (f *fakeRepo) LockVersion(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Version, error) {
	v, ok := f.versions[id]
	if !ok || v.CenterID != sc.CenterID {
		return nil, ErrNotFound
	}
	if tpl, ok := f.templates[v.TemplateID]; !ok || tpl.DeletedAt != nil {
		return nil, ErrNotFound
	}
	f.locks++
	cp := *v
	return &cp, nil
}

func (f *fakeRepo) ListVersions(_ context.Context, sc authctx.Scope, templateID uuid.UUID) ([]VersionRow, error) {
	f.listVersionsCalls++
	var out []VersionRow
	for _, v := range f.versions {
		if v.CenterID == sc.CenterID && v.TemplateID == templateID {
			out = append(out, *f.versionRow(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VersionNo > out[j].VersionNo })
	return out, nil
}

func (f *fakeRepo) ListVersionsFor(_ context.Context, sc authctx.Scope, templateIDs []uuid.UUID) ([]VersionRow, error) {
	f.listVersionsForCalls++
	want := map[uuid.UUID]bool{}
	for _, id := range templateIDs {
		want[id] = true
	}
	var out []VersionRow
	for _, v := range f.versions {
		if v.CenterID == sc.CenterID && want[v.TemplateID] {
			out = append(out, *f.versionRow(v))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TemplateID != out[j].TemplateID {
			return out[i].TemplateID.String() < out[j].TemplateID.String()
		}
		return out[i].VersionNo > out[j].VersionNo
	})
	return out, nil
}

func (f *fakeRepo) LatestReleasedVersion(_ context.Context, sc authctx.Scope, templateID uuid.UUID) (*Version, error) {
	var best *Version
	for _, v := range f.versions {
		if v.CenterID == sc.CenterID && v.TemplateID == templateID && v.Status != StatusDraft &&
			(best == nil || v.VersionNo > best.VersionNo) {
			best = v
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	cp := *best
	return &cp, nil
}

func (f *fakeRepo) NextVersionNo(_ context.Context, sc authctx.Scope, templateID uuid.UUID) (int, error) {
	next := 1
	for _, v := range f.versions {
		if v.CenterID == sc.CenterID && v.TemplateID == templateID && v.VersionNo >= next {
			next = v.VersionNo + 1
		}
	}
	return next, nil
}

func (f *fakeRepo) SetVersionStatus(_ context.Context, sc authctx.Scope, id uuid.UUID, from, to string, publishedAt *time.Time) error {
	v, ok := f.versions[id]
	if !ok || v.CenterID != sc.CenterID || v.Status != from {
		return ErrNotFound
	}
	v.Status = to
	if publishedAt != nil {
		v.PublishedAt = publishedAt
	}
	v.UpdatedAt = nowUTC()
	return nil
}

// lessonRow builds the LessonRow the SQL layer's subselects compute, from
// the in-memory link maps.
func (f *fakeRepo) lessonRow(l *Lesson) LessonRow {
	return LessonRow{
		Lesson:        *l,
		MaterialCount: len(f.lessonMaterials[l.ID]),
		ExerciseCount: len(f.lessonExercises[l.ID]),
	}
}

func (f *fakeRepo) ListLessons(_ context.Context, sc authctx.Scope, versionID uuid.UUID) ([]LessonRow, error) {
	var out []LessonRow
	for _, l := range f.lessons {
		if l.CenterID == sc.CenterID && l.VersionID == versionID {
			out = append(out, f.lessonRow(l))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

func (f *fakeRepo) GetLesson(_ context.Context, sc authctx.Scope, id uuid.UUID) (*LessonRow, error) {
	l, ok := f.lessons[id]
	if !ok || l.CenterID != sc.CenterID {
		return nil, ErrNotFound
	}
	row := f.lessonRow(l)
	return &row, nil
}

func (f *fakeRepo) NextPosition(_ context.Context, sc authctx.Scope, versionID uuid.UUID) (int, error) {
	next := 1
	for _, l := range f.lessons {
		if l.CenterID == sc.CenterID && l.VersionID == versionID && l.Position >= next {
			next = l.Position + 1
		}
	}
	return next, nil
}

func (f *fakeRepo) CreateLessons(_ context.Context, sc authctx.Scope, rows []*Lesson) error {
	for _, l := range rows {
		l.ID, l.CenterID = uuid.New(), sc.CenterID
		l.CreatedAt, l.UpdatedAt = nowUTC(), nowUTC()
		cp := *l
		f.lessons[l.ID] = &cp
	}
	return nil
}

func (f *fakeRepo) UpdateLesson(_ context.Context, sc authctx.Scope, l *Lesson) error {
	cur, ok := f.lessons[l.ID]
	if !ok || cur.CenterID != sc.CenterID {
		return ErrNotFound
	}
	cur.Title, cur.Mode, cur.Unit = l.Title, l.Mode, l.Unit
	cur.Objectives, cur.DurationMin, cur.HomeworkNote = l.Objectives, l.DurationMin, l.HomeworkNote
	cur.UpdatedAt = nowUTC()
	return nil
}

func (f *fakeRepo) DeleteLesson(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	cur, ok := f.lessons[id]
	if !ok || cur.CenterID != sc.CenterID {
		return ErrNotFound
	}
	delete(f.lessons, id)
	return nil
}

// ClearLessons deletes every lesson of the version, the same cascade the
// SQL foreign keys give the join tables.
func (f *fakeRepo) ClearLessons(_ context.Context, sc authctx.Scope, versionID uuid.UUID) error {
	for id, l := range f.lessons {
		if l.CenterID == sc.CenterID && l.VersionID == versionID {
			delete(f.lessons, id)
			delete(f.lessonMaterials, id)
			delete(f.lessonExercises, id)
		}
	}
	return nil
}

func (f *fakeRepo) SetPositions(_ context.Context, sc authctx.Scope, versionID uuid.UUID, ids []uuid.UUID) error {
	for i, id := range ids {
		l, ok := f.lessons[id]
		if !ok || l.CenterID != sc.CenterID || l.VersionID != versionID {
			return ErrNotFound
		}
		l.Position = i + 1
	}
	return nil
}

// ListVersionClasses reads from versionClasses, empty unless a test
// populated it; the real per-version 50-row cap is a SQL LIMIT, so it is
// covered by the integration test instead.
func (f *fakeRepo) ListVersionClasses(_ context.Context, _ authctx.Scope, versionID uuid.UUID) ([]VersionClassRef, error) {
	return f.versionClasses[versionID], nil
}

func (f *fakeRepo) ListVersionClassesFor(_ context.Context, _ authctx.Scope, versionIDs []uuid.UUID) ([]VersionClassLink, error) {
	var out []VersionClassLink
	for _, id := range versionIDs {
		for _, c := range f.versionClasses[id] {
			out = append(out, VersionClassLink{VersionID: id, ID: c.ID, Name: c.Name})
		}
	}
	return out, nil
}

func (f *fakeRepo) exerciseGroupRow(g *ExerciseGroup) ExerciseGroupRow {
	count := 0
	for _, links := range f.lessonExercises {
		for _, l := range links {
			if l.GroupID != nil && *l.GroupID == g.ID {
				count++
			}
		}
	}
	return ExerciseGroupRow{ExerciseGroup: *g, ExerciseCount: count}
}

func (f *fakeRepo) ListExerciseGroups(_ context.Context, sc authctx.Scope, versionID uuid.UUID) ([]ExerciseGroupRow, error) {
	var out []ExerciseGroupRow
	for _, g := range f.groups {
		if g.CenterID == sc.CenterID && g.VersionID == versionID {
			out = append(out, f.exerciseGroupRow(g))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

func (f *fakeRepo) NextGroupPosition(_ context.Context, sc authctx.Scope, versionID uuid.UUID) (int, error) {
	next := 1
	for _, g := range f.groups {
		if g.CenterID == sc.CenterID && g.VersionID == versionID && g.Position >= next {
			next = g.Position + 1
		}
	}
	return next, nil
}

func (f *fakeRepo) CreateExerciseGroup(_ context.Context, sc authctx.Scope, g *ExerciseGroup) error {
	g.ID, g.CenterID = uuid.New(), sc.CenterID
	g.CreatedAt, g.UpdatedAt = nowUTC(), nowUTC()
	cp := *g
	f.groups[g.ID] = &cp
	return nil
}

func (f *fakeRepo) CreateExerciseGroups(_ context.Context, sc authctx.Scope, rows []*ExerciseGroup) error {
	for _, g := range rows {
		g.ID, g.CenterID = uuid.New(), sc.CenterID
		g.CreatedAt, g.UpdatedAt = nowUTC(), nowUTC()
		cp := *g
		f.groups[g.ID] = &cp
	}
	return nil
}

// DeleteExerciseGroup mirrors the real foreign key's ON DELETE SET NULL:
// every lesson-exercise link pointing at the group loses its group_id
// instead of being deleted.
func (f *fakeRepo) DeleteExerciseGroup(_ context.Context, sc authctx.Scope, versionID, id uuid.UUID) error {
	g, ok := f.groups[id]
	if !ok || g.CenterID != sc.CenterID || g.VersionID != versionID {
		return ErrNotFound
	}
	delete(f.groups, id)
	for lessonID, links := range f.lessonExercises {
		for i := range links {
			if links[i].GroupID != nil && *links[i].GroupID == id {
				f.lessonExercises[lessonID][i].GroupID = nil
			}
		}
	}
	return nil
}

func (f *fakeRepo) FilterExistingGroupIDs(_ context.Context, sc authctx.Scope, versionID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	var found []uuid.UUID
	for _, id := range ids {
		g, ok := f.groups[id]
		if ok && g.CenterID == sc.CenterID && g.VersionID == versionID {
			found = append(found, id)
		}
	}
	return found, nil
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

// appErrorOf asserts the status and hands the error back so a test can inspect
// its details.
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

func mustTemplate(t *testing.T, svc *Service, sc authctx.Scope, code string) *TemplateResponse {
	t.Helper()
	out, err := svc.CreateTemplate(context.Background(), sc, TemplateRequest{Code: code, Name: "Toán 6 cơ bản"})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	return out
}

func draftOf(t *testing.T, svc *Service, sc authctx.Scope, templateID uuid.UUID) VersionResponse {
	t.Helper()
	versions, err := svc.ListVersions(context.Background(), sc, templateID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	for _, v := range versions {
		if v.Status == StatusDraft {
			return v
		}
	}
	t.Fatal("template has no draft")
	return VersionResponse{}
}

func mustLesson(t *testing.T, svc *Service, sc authctx.Scope, versionID uuid.UUID, title string) *LessonResponse {
	t.Helper()
	out, err := svc.CreateLesson(context.Background(), sc, versionID, LessonRequest{Title: title})
	if err != nil {
		t.Fatalf("create lesson %q: %v", title, err)
	}
	return out
}

func TestCreateTemplateOpensFirstDraft(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()

	out, err := svc.CreateTemplate(ctx, d.memberWith(authctx.PermLibraryEdit),
		TemplateRequest{Code: "ct-01", Name: "Toán 6", Subject: str("Toán")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Code != "CT-01" {
		t.Errorf("code must be upper-cased, got %q", out.Code)
	}
	if out.DraftVersionNo == nil || *out.DraftVersionNo != 1 || out.VersionCount != 1 || out.DraftVersionID == nil {
		t.Errorf("a new template must open draft v1, got %+v", out)
	}
	if out.PublishedVersionNo != nil {
		t.Errorf("nothing is published yet, got %+v", out)
	}
	if d.tx.calls != 1 {
		t.Errorf("template and its first draft must be created in one transaction, got %d tx calls", d.tx.calls)
	}
}

func TestCreateTemplateValidation(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	_, err := svc.CreateTemplate(ctx, sc, TemplateRequest{Code: "mã lớp!", Name: "Toán 6"})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")

	mustTemplate(t, svc, sc, "CT01")
	_, err = svc.CreateTemplate(ctx, sc, TemplateRequest{Code: "ct01", Name: "Trùng mã"})
	requireAppError(t, err, http.StatusConflict, CodeCodeTaken)

	_, err = svc.CreateTemplate(ctx, d.memberWith(authctx.PermLibraryRead), TemplateRequest{Code: "CT02", Name: "Không có quyền"})
	requireAppError(t, err, http.StatusForbidden, "")
}

func TestReadsRequireLibraryRead(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	tpl := mustTemplate(t, svc, d.ownerScope(), "CT01")

	reader := d.memberWith(authctx.PermLibraryRead)
	if _, err := svc.GetTemplate(ctx, reader, tpl.ID); err != nil {
		t.Fatalf("a library.read holder must read the template: %v", err)
	}
	rows, total, err := svc.ListTemplates(ctx, reader, ListFilter{}, pagination.Params{})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("list: %v total=%d rows=%d", err, total, len(rows))
	}

	stranger := d.memberWith()
	_, err = svc.GetTemplate(ctx, stranger, tpl.ID)
	requireAppError(t, err, http.StatusForbidden, "")
	_, _, err = svc.ListTemplates(ctx, stranger, ListFilter{}, pagination.Params{})
	requireAppError(t, err, http.StatusForbidden, "")

	// The write keys imply the read key, so an editor with no explicit read
	// grant still reads.
	editor := d.memberWith(authctx.PermLibraryEdit)
	if _, err := svc.GetTemplate(ctx, editor, tpl.ID); err != nil {
		t.Fatalf("library.edit must imply library.read: %v", err)
	}
}

func TestTemplateOfAnotherCenterIsNotFound(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	tpl := mustTemplate(t, svc, d.ownerScope(), "CT01")
	draft := draftOf(t, svc, d.ownerScope(), tpl.ID)
	lesson := mustLesson(t, svc, d.ownerScope(), draft.ID, "Buổi 1")

	other := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}
	_, err := svc.GetTemplate(ctx, other, tpl.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.UpdateTemplate(ctx, other, tpl.ID, TemplateRequest{Code: "CT01", Name: "Đổi"})
	requireAppError(t, err, http.StatusNotFound, "")
	requireAppError(t, svc.DeleteTemplate(ctx, other, tpl.ID), http.StatusNotFound, "")
	_, err = svc.CreateVersion(ctx, other, tpl.ID, CreateVersionRequest{})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.Publish(ctx, other, draft.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.CreateLesson(ctx, other, draft.ID, LessonRequest{Title: "Lạ"})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.UpdateLesson(ctx, other, lesson.ID, LessonRequest{Title: "Lạ"})
	requireAppError(t, err, http.StatusNotFound, "")
	requireAppError(t, svc.DeleteLesson(ctx, other, lesson.ID), http.StatusNotFound, "")
	_, err = svc.ListLessons(ctx, other, draft.ID)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestUpdateAndDeleteTemplate(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	a := mustTemplate(t, svc, sc, "CT01")
	mustTemplate(t, svc, sc, "CT02")

	out, err := svc.UpdateTemplate(ctx, sc, a.ID, TemplateRequest{Code: "ct01", Name: "Toán 6 nâng cao", Level: str("Lớp 6")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Name != "Toán 6 nâng cao" || out.Level == nil || *out.Level != "Lớp 6" || out.Code != "CT01" {
		t.Errorf("update must replace the own fields, got %+v", out)
	}
	_, err = svc.UpdateTemplate(ctx, sc, a.ID, TemplateRequest{Code: "CT02", Name: "Trùng"})
	requireAppError(t, err, http.StatusConflict, CodeCodeTaken)

	_, err = svc.UpdateTemplate(ctx, d.memberWith(authctx.PermLibraryRead), a.ID, TemplateRequest{Code: "CT01", Name: "x"})
	requireAppError(t, err, http.StatusForbidden, "")

	if err := svc.DeleteTemplate(ctx, sc, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = svc.GetTemplate(ctx, sc, a.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	rows, _, err := svc.ListTemplates(ctx, sc, ListFilter{}, pagination.Params{})
	if err != nil || len(rows) != 1 || rows[0].Code != "CT02" {
		t.Fatalf("a soft-deleted template must leave the list: %v %+v", err, rows)
	}
	// The code is free again after the soft delete.
	if _, err := svc.CreateTemplate(ctx, sc, TemplateRequest{Code: "CT01", Name: "Dùng lại mã"}); err != nil {
		t.Fatalf("reusing a soft-deleted code: %v", err)
	}
}

func TestPublishLocksLessons(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	l1 := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	l2 := mustLesson(t, svc, sc, draft.ID, "Buổi 2")
	if l1.Position != 1 || l2.Position != 2 {
		t.Fatalf("create must append positions 1,2 — got %d,%d", l1.Position, l2.Position)
	}

	_, err := svc.Publish(ctx, d.memberWith(authctx.PermLibraryEdit), draft.ID)
	requireAppError(t, err, http.StatusForbidden, "")

	published, err := svc.Publish(ctx, d.memberWith(authctx.PermLibraryPublish), draft.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.Status != StatusPublished || published.PublishedAt == nil || published.LessonCount != 2 {
		t.Errorf("publish must stamp status and time, got %+v", published)
	}
	_, err = svc.Publish(ctx, sc, draft.ID)
	requireAppError(t, err, http.StatusConflict, CodeVersionNotDraft)

	_, err = svc.CreateLesson(ctx, sc, draft.ID, LessonRequest{Title: "Buổi 3"})
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)
	_, err = svc.UpdateLesson(ctx, sc, l1.ID, LessonRequest{Title: "Đổi tên"})
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)
	requireAppError(t, svc.DeleteLesson(ctx, sc, l1.ID), http.StatusConflict, CodeVersionLocked)
	_, err = svc.ReorderLessons(ctx, sc, draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{l2.ID, l1.ID}})
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)

	// Reads keep working on a locked version.
	got, err := svc.ListLessons(ctx, d.memberWith(authctx.PermLibraryRead), draft.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("list lessons of a published version: %v (%d)", err, len(got))
	}
	tplRow, err := svc.GetTemplate(ctx, sc, tpl.ID)
	if err != nil || tplRow.PublishedVersionNo == nil || *tplRow.PublishedVersionNo != 1 || tplRow.DraftVersionNo != nil {
		t.Fatalf("template summary must reflect the publish: %v %+v", err, tplRow)
	}
}

func TestNewVersionCopiesLatestPublishedLessons(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)

	_, err := svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	requireAppError(t, err, http.StatusConflict, CodeDraftExists)

	l1 := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	if _, err := svc.UpdateLesson(ctx, sc, l1.ID, LessonRequest{Title: "Buổi 1", Objectives: str("Mục tiêu"), DurationMin: intp(90), HomeworkNote: str("BTVN")}); err != nil {
		t.Fatalf("update lesson: %v", err)
	}
	mustLesson(t, svc, sc, draft.ID, "Buổi 2")
	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	d.tx.calls = 0
	v2, err := svc.CreateVersion(ctx, d.memberWith(authctx.PermLibraryEdit), tpl.ID, CreateVersionRequest{Changelog: str("Thêm buổi ôn")})
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if v2.VersionNo != 2 || v2.Status != StatusDraft || v2.LessonCount != 2 || v2.Changelog == nil {
		t.Errorf("v2 must be a draft carrying the 2 published lessons, got %+v", v2)
	}
	if d.tx.calls != 1 {
		t.Errorf("version and copied lessons must be written in one transaction, got %d", d.tx.calls)
	}
	copied, err := svc.ListLessons(ctx, sc, v2.ID)
	if err != nil || len(copied) != 2 {
		t.Fatalf("copied lessons: %v (%d)", err, len(copied))
	}
	if copied[0].ID == l1.ID || copied[0].Title != "Buổi 1" || copied[0].Position != 1 ||
		copied[0].Objectives == nil || *copied[0].Objectives != "Mục tiêu" ||
		copied[0].DurationMin == nil || *copied[0].DurationMin != 90 {
		t.Errorf("copy must be a new row with the same content, got %+v", copied[0])
	}
	// The copy is independent: editing v2 leaves v1 untouched.
	if _, err := svc.UpdateLesson(ctx, sc, copied[0].ID, LessonRequest{Title: "Buổi 1 (sửa)"}); err != nil {
		t.Fatalf("edit copied lesson: %v", err)
	}
	orig, _ := svc.GetLesson(ctx, sc, l1.ID)
	if orig.Title != "Buổi 1" {
		t.Errorf("editing the draft copy must not touch the published lesson, got %q", orig.Title)
	}

	// A third version is refused while v2 is still a draft; once v2 is
	// published, v3 copies v2 (the latest published), not v1.
	_, err = svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	requireAppError(t, err, http.StatusConflict, CodeDraftExists)
	if _, err := svc.Publish(ctx, sc, v2.ID); err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	v3, err := svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	if err != nil {
		t.Fatalf("create v3: %v", err)
	}
	v3Lessons, _ := svc.ListLessons(ctx, sc, v3.ID)
	if v3.VersionNo != 3 || len(v3Lessons) != 2 || v3Lessons[0].Title != "Buổi 1 (sửa)" {
		t.Errorf("v3 must copy the latest published version, got %+v / %+v", v3, v3Lessons)
	}
	tplRow, _ := svc.GetTemplate(ctx, sc, tpl.ID)
	if tplRow.PublishedVersionNo == nil || *tplRow.PublishedVersionNo != 2 || tplRow.DraftVersionNo == nil || *tplRow.DraftVersionNo != 3 || tplRow.VersionCount != 3 {
		t.Errorf("summary must show published v2 and draft v3, got %+v", tplRow)
	}
}

// TestListVersionsIncludesClassesForEveryVersion covers the ListVersions
// endpoint filling each row's class chips from one batched call, the same
// way GetVersion already does for a single version.
func TestListVersionsIncludesClassesForEveryVersion(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	v2, err := svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}

	classA, classB := uuid.New(), uuid.New()
	d.repo.versionClasses[draft.ID] = []VersionClassRef{{ID: classA, Name: "Lớp A"}}
	d.repo.versionClasses[v2.ID] = []VersionClassRef{{ID: classB, Name: "Lớp B"}}

	versions, err := svc.ListVersions(ctx, sc, tpl.ID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("want 2 versions, got %d", len(versions))
	}
	byID := map[uuid.UUID]VersionResponse{}
	for _, v := range versions {
		byID[v.ID] = v
	}
	if v1Resp := byID[draft.ID]; len(v1Resp.Classes) != 1 || v1Resp.Classes[0].ID != classA {
		t.Errorf("v1 must carry its own class, got %+v", v1Resp.Classes)
	}
	if v2Resp := byID[v2.ID]; len(v2Resp.Classes) != 1 || v2Resp.Classes[0].ID != classB {
		t.Errorf("v2 must carry its own class, not v1's, got %+v", v2Resp.Classes)
	}
}

func TestArchiveKeepsLessons(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	mustLesson(t, svc, sc, draft.ID, "Buổi 1")

	_, err := svc.Archive(ctx, sc, draft.ID)
	requireAppError(t, err, http.StatusConflict, CodeVersionNotPublished)
	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	_, err = svc.Archive(ctx, d.memberWith(authctx.PermLibraryRead), draft.ID)
	requireAppError(t, err, http.StatusForbidden, "")

	archived, err := svc.Archive(ctx, d.memberWith(authctx.PermLibraryEdit), draft.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.Status != StatusArchived || archived.LessonCount != 1 {
		t.Errorf("archive must keep the lessons, got %+v", archived)
	}
	_, err = svc.Archive(ctx, sc, draft.ID)
	requireAppError(t, err, http.StatusConflict, CodeVersionNotPublished)
	_, err = svc.CreateLesson(ctx, sc, draft.ID, LessonRequest{Title: "Buổi 2"})
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)
	tplRow, _ := svc.GetTemplate(ctx, sc, tpl.ID)
	if tplRow.PublishedVersionNo != nil {
		t.Errorf("an archived version is no longer the published one, got %+v", tplRow)
	}
}

// TestTemplateLessonCountCountsOnlyTheNewestPublishedVersion covers the
// case where two published versions coexist (publishing never auto-archives
// an older one): the template's lesson_count must reflect only the newest
// published version's lessons, not every published version's lessons summed.
func TestTemplateLessonCountCountsOnlyTheNewestPublishedVersion(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	mustLesson(t, svc, sc, draft.ID, "Buổi 2")
	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	v2, err := svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	v2Lessons, err := svc.ListLessons(ctx, sc, v2.ID)
	if err != nil || len(v2Lessons) != 2 {
		t.Fatalf("v2 copied lessons: %v (%d)", err, len(v2Lessons))
	}
	// Drop one of v2's copied lessons so v1 (2 lessons) and v2 (1 lesson)
	// differ: summing both published versions (3) is distinguishable from
	// counting only the newest (1).
	if err := svc.DeleteLesson(ctx, sc, v2Lessons[0].ID); err != nil {
		t.Fatalf("delete lesson: %v", err)
	}
	if _, err := svc.Publish(ctx, sc, v2.ID); err != nil {
		t.Fatalf("publish v2: %v", err)
	}

	tplRow, err := svc.GetTemplate(ctx, sc, tpl.ID)
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	if tplRow.PublishedVersionNo == nil || *tplRow.PublishedVersionNo != 2 {
		t.Fatalf("v1 and v2 must both stay published, newest is v2: %+v", tplRow)
	}
	if tplRow.LessonCount != 1 {
		t.Errorf("lesson_count must count only the newest published version's lessons (1), got %d", tplRow.LessonCount)
	}
}

// TestDuplicateLessonTruncatesATitleAtTheColumnLimit covers a source title
// already at the 200-rune column limit: appending the "(bản sao)" suffix
// verbatim would overflow VARCHAR(200) and 500 the request, so the
// duplicate's title must be truncated to still fit.
func TestDuplicateLessonTruncatesATitleAtTheColumnLimit(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)

	longTitle := strings.Repeat("a", lessonTitleMaxLen)
	src := mustLesson(t, svc, sc, draft.ID, longTitle)

	dup, err := svc.DuplicateLesson(ctx, sc, src.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	runes := []rune(dup.Title)
	if len(runes) > lessonTitleMaxLen {
		t.Fatalf("duplicated title must not exceed %d runes, got %d: %q", lessonTitleMaxLen, len(runes), dup.Title)
	}
	if !strings.HasSuffix(dup.Title, duplicateTitleSuffix) {
		t.Errorf("duplicated title must still carry the suffix, got %q", dup.Title)
	}
}

// TestLessonUnitIsTrimmedAndBlankBecomesNull covers CreateLesson and
// UpdateLesson storing a padded unit trimmed, and a whitespace-only or
// omitted one as NULL rather than an empty string.
func TestLessonUnitIsTrimmedAndBlankBecomesNull(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)

	created, err := svc.CreateLesson(ctx, sc, draft.ID, LessonRequest{Title: "Buổi 1", Unit: str("  Unit 1  ")})
	if err != nil || created.Unit == nil || *created.Unit != "Unit 1" {
		t.Fatalf("create must trim unit: %v %+v", err, created)
	}

	blank, err := svc.CreateLesson(ctx, sc, draft.ID, LessonRequest{Title: "Buổi 2", Unit: str("   ")})
	if err != nil || blank.Unit != nil {
		t.Fatalf("create with a whitespace-only unit must store null, got %v %v", err, blank.Unit)
	}

	updated, err := svc.UpdateLesson(ctx, sc, created.ID, LessonRequest{Title: "Buổi 1", Unit: str("  Unit 2 ")})
	if err != nil || updated.Unit == nil || *updated.Unit != "Unit 2" {
		t.Fatalf("update must trim unit: %v %+v", err, updated)
	}

	cleared, err := svc.UpdateLesson(ctx, sc, created.ID, LessonRequest{Title: "Buổi 1"})
	if err != nil || cleared.Unit != nil {
		t.Fatalf("update with an omitted unit must store null, got %v %v", err, cleared.Unit)
	}
}

func TestLessonDeleteRenumbersAndReorderValidates(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	l1 := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	l2 := mustLesson(t, svc, sc, draft.ID, "Buổi 2")
	l3 := mustLesson(t, svc, sc, draft.ID, "Buổi 3")

	_, err := svc.ReorderLessons(ctx, sc, draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{l3.ID, l1.ID}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.ReorderLessons(ctx, sc, draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{l3.ID, l1.ID, l1.ID}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.ReorderLessons(ctx, sc, draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{l3.ID, l1.ID, uuid.New()}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")

	d.tx.calls = 0
	ordered, err := svc.ReorderLessons(ctx, d.memberWith(authctx.PermLibraryEdit), draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{l3.ID, l1.ID, l2.ID}})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if d.tx.calls != 1 {
		t.Errorf("reorder must run in one transaction, got %d", d.tx.calls)
	}
	if ordered[0].ID != l3.ID || ordered[0].Position != 1 || ordered[1].ID != l1.ID || ordered[2].ID != l2.ID || ordered[2].Position != 3 {
		t.Errorf("reorder must renumber in the given order, got %+v", ordered)
	}

	d.tx.calls = 0
	if err := svc.DeleteLesson(ctx, sc, l1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if d.tx.calls != 1 {
		t.Errorf("delete plus renumber must run in one transaction, got %d", d.tx.calls)
	}
	rest, _ := svc.ListLessons(ctx, sc, draft.ID)
	if len(rest) != 2 || rest[0].ID != l3.ID || rest[0].Position != 1 || rest[1].ID != l2.ID || rest[1].Position != 2 {
		t.Errorf("delete must close the gap, got %+v", rest)
	}
	_, err = svc.GetLesson(ctx, sc, l1.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.CreateLesson(ctx, d.memberWith(authctx.PermLibraryRead), draft.ID, LessonRequest{Title: "Không quyền"})
	requireAppError(t, err, http.StatusForbidden, "")
}

func intp(n int) *int { return &n }

func TestWritesToDeletedTemplateAreNotFound(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	if err := svc.DeleteTemplate(ctx, sc, tpl.ID); err != nil {
		t.Fatalf("delete template: %v", err)
	}

	// Reads keep working for anything still bound to the version …
	if _, err := svc.GetLesson(ctx, sc, lesson.ID); err != nil {
		t.Fatalf("a deleted template's lesson must stay readable: %v", err)
	}
	// … but every write path answers 404 once the template is gone.
	_, err := svc.CreateLesson(ctx, sc, draft.ID, LessonRequest{Title: "Buổi 2"})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.UpdateLesson(ctx, sc, lesson.ID, LessonRequest{Title: "Đổi"})
	requireAppError(t, err, http.StatusNotFound, "")
	requireAppError(t, svc.DeleteLesson(ctx, sc, lesson.ID), http.StatusNotFound, "")
	_, err = svc.ReorderLessons(ctx, sc, draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{lesson.ID}})
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.Publish(ctx, sc, draft.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.Archive(ctx, sc, draft.ID)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestLessonWritesAndTransitionsTakeTheVersionLock(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	d.repo.locks, d.tx.calls = 0, 0

	if _, err := svc.UpdateLesson(ctx, sc, lesson.ID, LessonRequest{Title: "Buổi 1b"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.ReorderLessons(ctx, sc, draft.ID, ReorderRequest{LessonIDs: []uuid.UUID{lesson.ID}}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if err := svc.DeleteLesson(ctx, sc, lesson.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := svc.Archive(ctx, sc, draft.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if d.repo.locks != 5 {
		t.Errorf("each write must lock the version row once, got %d locks", d.repo.locks)
	}
	if d.tx.calls < 5 {
		t.Errorf("each write must run inside a transaction, got %d", d.tx.calls)
	}
}

func TestNewDraftCopiesArchivedLessonsWhenNothingIsPublished(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	v1 := draftOf(t, svc, sc, tpl.ID)
	mustLesson(t, svc, sc, v1.ID, "Buổi 1")
	if _, err := svc.Publish(ctx, sc, v1.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := svc.Archive(ctx, sc, v1.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// Archiving the only released version must not throw its content away:
	// the next draft still starts from it.
	v2, err := svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	if v2.VersionNo != 2 || v2.LessonCount != 1 {
		t.Errorf("draft v2 must copy the archived lesson, got %+v", v2)
	}
}

func TestOptionalTemplateTextIsTrimmedAndBlankBecomesNull(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	subject, level, desc := "  Toán  ", "   ", ""
	out, err := svc.CreateTemplate(context.Background(), sc, TemplateRequest{
		Code: "CT01", Name: "Toán 6", Subject: &subject, Level: &level, Description: &desc,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Subject == nil || *out.Subject != "Toán" {
		t.Errorf("subject must be trimmed, got %v", out.Subject)
	}
	if out.Level != nil || out.Description != nil {
		t.Errorf("blank optional text must be stored as NULL, got level=%v description=%v", out.Level, out.Description)
	}
}

func TestCreateTemplateWithLessonCountSeedsEmptyLessons(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	d.tx.calls = 0

	tpl, err := svc.CreateTemplate(ctx, sc, TemplateRequest{Code: "CT08", Name: "Toán 8", LessonCount: intp(8)})
	if err != nil {
		t.Fatalf("create with lesson_count: %v", err)
	}
	if d.tx.calls != 1 {
		t.Errorf("template, first draft and seeded lessons must share one transaction, got %d", d.tx.calls)
	}
	if tpl.LessonCount != 8 {
		t.Fatalf("summary must report the seeded draft, got %+v", tpl)
	}
	draft := draftOf(t, svc, sc, tpl.ID)
	lessons, err := svc.ListLessons(ctx, sc, draft.ID)
	if err != nil || len(lessons) != 8 {
		t.Fatalf("want 8 seeded lessons, got %d (%v)", len(lessons), err)
	}
	for i, l := range lessons {
		want := "Buổi " + strconv.Itoa(i+1)
		if l.Position != i+1 || l.Title != want {
			t.Errorf("lesson %d: want %q at position %d, got %+v", i, want, i+1, l)
		}
	}

	plain := mustTemplate(t, svc, sc, "CT09")
	if plain.LessonCount != 0 {
		t.Errorf("a template without lesson_count still has an empty draft, got %+v", plain)
	}
	if got, err := svc.ListLessons(ctx, sc, draftOf(t, svc, sc, plain.ID).ID); err != nil || len(got) != 0 {
		t.Errorf("no lesson_count must seed nothing, got %d (%v)", len(got), err)
	}
}

func TestListTemplatesHasDraftFilter(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	withDraft := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, withDraft.ID)
	mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	mustLesson(t, svc, sc, draft.ID, "Buổi 2")
	published := mustTemplate(t, svc, sc, "CT02")
	if _, err := svc.Publish(ctx, sc, draftOf(t, svc, sc, published.ID).ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	d.repo.listVersionsCalls, d.repo.listVersionsForCalls = 0, 0
	all, total, err := svc.ListTemplates(ctx, sc, ListFilter{}, pagination.Params{Page: 1, PerPage: 20})
	if err != nil || total != 2 || len(all) != 2 {
		t.Fatalf("unfiltered list: %v total=%d len=%d", err, total, len(all))
	}
	if d.repo.listVersionsForCalls != 1 || d.repo.listVersionsCalls != 0 {
		t.Errorf("ListTemplates must batch versions in one ListVersionsFor call instead of looping ListVersions per template, got ListVersionsFor=%d ListVersions=%d",
			d.repo.listVersionsForCalls, d.repo.listVersionsCalls)
	}

	drafts, total, err := svc.ListTemplates(ctx, sc, ListFilter{HasDraft: true}, pagination.Params{Page: 1, PerPage: 20})
	if err != nil || total != 1 || len(drafts) != 1 || drafts[0].ID != withDraft.ID {
		t.Fatalf("has_draft must keep only templates with an open draft: %v total=%d rows=%+v", err, total, drafts)
	}
	if drafts[0].LessonCount != 2 {
		t.Errorf("draft summary must count its lessons, got %+v", drafts[0])
	}
}
