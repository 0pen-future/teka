package courses

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// fakeRepo is an in-memory Repository that honours the center boundary the
// way the SQL does: a row of another center reads as ErrNotFound.
type fakeRepo struct {
	courses  map[uuid.UUID]*Course
	packs    map[uuid.UUID][]TuitionPack
	versions map[uuid.UUID]*fakeVersion
	// classes counts live classes per course id, standing in for the rows
	// the SQL counts.
	classes map[uuid.UUID]fakeClassCount
	// paths lists the learning path stages each course sits in, standing in
	// for the rows the SQL joins.
	paths map[uuid.UUID][]CoursePath
}

type fakeVersion struct {
	TemplateVersionRef
	centerID uuid.UUID
	// templateDeleted stands in for program_templates.deleted_at being set.
	templateDeleted bool
}

type fakeClassCount struct{ running, upcoming int }

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		courses:  map[uuid.UUID]*Course{},
		packs:    map[uuid.UUID][]TuitionPack{},
		versions: map[uuid.UUID]*fakeVersion{},
		classes:  map[uuid.UUID]fakeClassCount{},
		paths:    map[uuid.UUID][]CoursePath{},
	}
}

func (f *fakeRepo) clash(sc authctx.Scope, id uuid.UUID, code string) bool {
	for _, other := range f.courses {
		if other.ID != id && other.CenterID == sc.CenterID && other.DeletedAt == nil && other.Code == code {
			return true
		}
	}
	return false
}

func (f *fakeRepo) live(sc authctx.Scope, id uuid.UUID) (*Course, bool) {
	c, ok := f.courses[id]
	if !ok || c.CenterID != sc.CenterID || c.DeletedAt != nil {
		return nil, false
	}
	return c, true
}

func (f *fakeRepo) Create(_ context.Context, sc authctx.Scope, c *Course) error {
	if f.clash(sc, uuid.Nil, c.Code) {
		return gorm.ErrDuplicatedKey
	}
	c.ID, c.CenterID = uuid.New(), sc.CenterID
	c.CreatedAt, c.UpdatedAt = nowUTC(), nowUTC()
	cp := *c
	f.courses[c.ID] = &cp
	return nil
}

func (f *fakeRepo) row(c *Course) CourseRow {
	row := CourseRow{Course: *c}
	n := f.classes[c.ID]
	row.ClassesRunning, row.ClassesUpcoming = n.running, n.upcoming
	if c.DefaultTemplateVersionID != nil {
		if v, ok := f.versions[*c.DefaultTemplateVersionID]; ok && !v.templateDeleted {
			tid, no, code, name, status := v.TemplateID, v.VersionNo, v.TemplateCode, v.TemplateName, v.Status
			row.TemplateID, row.TemplateVersionNo, row.TemplateCode, row.TemplateName = &tid, &no, &code, &name
			row.TemplateVersionStatus = &status
		}
	}
	return row
}

func (f *fakeRepo) Get(_ context.Context, sc authctx.Scope, id uuid.UUID) (*CourseRow, error) {
	c, ok := f.live(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	row := f.row(c)
	return &row, nil
}

func (f *fakeRepo) Lock(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Course, error) {
	if ctx.Value(fakeTxKey{}) == nil {
		return nil, errors.New("Lock called outside a transaction")
	}
	c, ok := f.live(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (f *fakeRepo) List(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]CourseRow, int64, error) {
	var out []CourseRow
	for _, c := range f.courses {
		if c.CenterID != sc.CenterID || c.DeletedAt != nil {
			continue
		}
		if filter.Status != "" && c.Status != filter.Status {
			continue
		}
		out = append(out, f.row(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, int64(len(out)), nil
}

func (f *fakeRepo) Update(_ context.Context, sc authctx.Scope, c *Course) error {
	cur, ok := f.live(sc, c.ID)
	if !ok {
		return ErrNotFound
	}
	if f.clash(sc, c.ID, c.Code) {
		return gorm.ErrDuplicatedKey
	}
	id, center, created := cur.ID, cur.CenterID, cur.CreatedAt
	*cur = *c
	cur.ID, cur.CenterID, cur.CreatedAt, cur.UpdatedAt = id, center, created, nowUTC()
	return nil
}

func (f *fakeRepo) SetStatus(_ context.Context, sc authctx.Scope, id uuid.UUID, status string) error {
	cur, ok := f.live(sc, id)
	if !ok {
		return ErrNotFound
	}
	cur.Status = status
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

func (f *fakeRepo) LiveClassCount(_ context.Context, _ authctx.Scope, id uuid.UUID) (int64, error) {
	n := f.classes[id]
	return int64(n.running + n.upcoming), nil
}

func (f *fakeRepo) PathCount(_ context.Context, _ authctx.Scope, id uuid.UUID) (int64, error) {
	return int64(len(f.paths[id])), nil
}

func (f *fakeRepo) ListPaths(_ context.Context, _ authctx.Scope, id uuid.UUID) ([]CoursePath, error) {
	return f.paths[id], nil
}

func (f *fakeRepo) FindTemplateVersion(_ context.Context, sc authctx.Scope, versionID uuid.UUID) (*TemplateVersionRef, error) {
	v, ok := f.versions[versionID]
	if !ok || v.centerID != sc.CenterID || v.templateDeleted {
		return nil, ErrNotFound
	}
	ref := v.TemplateVersionRef
	return &ref, nil
}

func (f *fakeRepo) ListPacks(_ context.Context, sc authctx.Scope, courseIDs []uuid.UUID) ([]TuitionPack, error) {
	var out []TuitionPack
	for _, id := range courseIDs {
		if _, ok := f.live(sc, id); !ok {
			continue
		}
		out = append(out, f.packs[id]...)
	}
	return out, nil
}

func (f *fakeRepo) ReplacePacks(_ context.Context, sc authctx.Scope, courseID uuid.UUID, rows []*TuitionPack) error {
	if _, ok := f.live(sc, courseID); !ok {
		return ErrNotFound
	}
	out := make([]TuitionPack, 0, len(rows))
	for i, r := range rows {
		out = append(out, TuitionPack{ID: uuid.New(), CourseID: courseID, CenterID: sc.CenterID,
			Name: r.Name, Sessions: r.Sessions, Price: r.Price, Position: i + 1})
	}
	f.packs[courseID] = out
	return nil
}

// version registers a template version of the given center and status.
func (f *fakeRepo) version(center uuid.UUID, status string) uuid.UUID {
	id := uuid.New()
	f.versions[id] = &fakeVersion{centerID: center, TemplateVersionRef: TemplateVersionRef{
		ID: id, TemplateID: uuid.New(), VersionNo: 1, Status: status, TemplateCode: "TOAN-6", TemplateName: "Toán 6",
	}}
	return id
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

func nowUTC() time.Time { return time.Now().UTC() }

func mustCourse(t *testing.T, svc *Service, sc authctx.Scope, code string) *CourseResponse {
	t.Helper()
	out, err := svc.Create(context.Background(), sc, CourseRequest{Code: code, Name: "Toán 6 cơ bản", DefaultUnitPrice: 150000})
	if err != nil {
		t.Fatalf("create course: %v", err)
	}
	return out
}

func TestCreateNormalisesAndDefaultsToDraft(t *testing.T) {
	svc, d := newTestService()
	out, err := svc.Create(context.Background(), d.ownerScope(), CourseRequest{
		Code: " toan-6 ", Name: "  Toán 6  ", Subject: str("  "), Level: str(" Lớp 6 "), DefaultUnitPrice: 120000,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Code != "TOAN-6" || out.Name != "Toán 6" || out.Status != StatusDraft {
		t.Fatalf("unexpected course %+v", out)
	}
	if out.Subject != nil || out.Level == nil || *out.Level != "Lớp 6" {
		t.Fatalf("blank text must be null and level trimmed: %+v", out)
	}
	if out.TuitionPacks == nil || len(out.TuitionPacks) != 0 {
		t.Fatalf("tuition_packs must serialise as [] not null: %#v", out.TuitionPacks)
	}
	if out.DefaultTemplate != nil {
		t.Fatalf("no default template expected, got %+v", out.DefaultTemplate)
	}
}

func TestCreateRejectsBadCodeAndClash(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	_, err := svc.Create(context.Background(), sc, CourseRequest{Code: "a", Name: "x"})
	appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Fields["code"] == "" {
		t.Fatalf("want a field error on code, got %+v", appErr.Fields)
	}
	mustCourse(t, svc, sc, "TOAN-6")
	_, err = svc.Create(context.Background(), sc, CourseRequest{Code: "toan-6", Name: "Trùng"})
	requireAppError(t, err, http.StatusConflict, CodeCodeTaken)
}

func TestDefaultTemplateMustBePublishedInCenter(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	draft := d.repo.version(d.center, library.StatusDraft)
	foreign := d.repo.version(uuid.New(), library.StatusPublished)
	published := d.repo.version(d.center, library.StatusPublished)

	for name, id := range map[string]uuid.UUID{"draft": draft, "foreign": foreign, "missing": uuid.New()} {
		_, err := svc.Create(context.Background(), sc, CourseRequest{Code: "T-" + name, Name: name, DefaultTemplateVersionID: &id})
		appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
		if appErr.Fields["default_template_version_id"] == "" {
			t.Fatalf("%s: want field error, got %+v", name, appErr.Fields)
		}
	}
	out, err := svc.Create(context.Background(), sc, CourseRequest{Code: "TOAN-6", Name: "ok", DefaultTemplateVersionID: &published})
	if err != nil {
		t.Fatalf("create with published version: %v", err)
	}
	if out.DefaultTemplate == nil || out.DefaultTemplate.VersionID != published || out.DefaultTemplate.Code != "TOAN-6" {
		t.Fatalf("default_template not embedded: %+v", out.DefaultTemplate)
	}
}

func TestUpdateReplacesFieldsAndKeepsCodeUnique(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	a := mustCourse(t, svc, sc, "A1")
	mustCourse(t, svc, sc, "B1")
	sessions := 24
	out, err := svc.Update(context.Background(), sc, a.ID, CourseRequest{Code: "a2", Name: "Đổi tên", Status: StatusActive, TotalSessions: &sessions})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Code != "A2" || out.Name != "Đổi tên" || out.Status != StatusActive || out.TotalSessions == nil || *out.TotalSessions != 24 {
		t.Fatalf("unexpected %+v", out)
	}
	_, err = svc.Update(context.Background(), sc, a.ID, CourseRequest{Code: "B1", Name: "Trùng"})
	requireAppError(t, err, http.StatusConflict, CodeCodeTaken)
	_, err = svc.Update(context.Background(), sc, uuid.New(), CourseRequest{Code: "C1", Name: "Không có"})
	requireAppError(t, err, http.StatusNotFound, "")
}

// A default version that was published when chosen may be archived, or its
// template deleted, later. Resending that same id is not a new choice, so
// an unrelated edit must still go through; picking a retired version anew
// is refused. Response-side, the archived status is reported and a deleted
// template drops the embed while the id stays stored.
func TestUpdateKeepsUnchangedTemplateAndStatus(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	v1 := d.repo.version(d.center, library.StatusPublished)
	c, err := svc.Create(context.Background(), sc, CourseRequest{Code: "TOAN-6", Name: "Toán 6", Status: StatusActive, DefaultTemplateVersionID: &v1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	d.repo.versions[v1].Status = library.StatusArchived

	out, err := svc.Update(context.Background(), sc, c.ID, CourseRequest{Code: "TOAN-6", Name: "Toán 6 nâng cao", DefaultTemplateVersionID: &v1})
	if err != nil {
		t.Fatalf("rename with the stored version must pass: %v", err)
	}
	if out.Name != "Toán 6 nâng cao" || out.Status != StatusActive {
		t.Fatalf("a blank status must keep the stored one: %+v", out)
	}
	if out.DefaultTemplate == nil || out.DefaultTemplate.Status != library.StatusArchived {
		t.Fatalf("the embed must carry the version's real status: %+v", out.DefaultTemplate)
	}

	v2 := d.repo.version(d.center, library.StatusArchived)
	_, err = svc.Update(context.Background(), sc, c.ID, CourseRequest{Code: "TOAN-6", Name: "Toán 6", DefaultTemplateVersionID: &v2})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["default_template_version_id"] == "" {
		t.Fatalf("choosing an archived version anew is refused: %+v", appErr.Fields)
	}

	d.repo.versions[v1].templateDeleted = true
	out, err = svc.Update(context.Background(), sc, c.ID, CourseRequest{Code: "TOAN-6", Name: "Toán 6", DefaultTemplateVersionID: &v1})
	if err != nil {
		t.Fatalf("a deleted template must not block edits: %v", err)
	}
	if out.DefaultTemplate != nil || out.DefaultTemplateVersionID == nil {
		t.Fatalf("deleted template: embed dropped, id kept: %+v", out)
	}
}

func TestBlankNamesAreRejected(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	_, err := svc.Create(context.Background(), sc, CourseRequest{Code: "TOAN-6", Name: "   "})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["name"] == "" {
		t.Fatalf("whitespace name must land on name: %+v", appErr.Fields)
	}
	c := mustCourse(t, svc, sc, "A1")
	_, err = svc.SetTuitionPacks(context.Background(), sc, c.ID, []TuitionPackInput{
		{Name: "Gói 12", Sessions: 12, Price: 1},
		{Name: " ", Sessions: 24, Price: 1},
	})
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["1.name"] == "" {
		t.Fatalf("whitespace pack name must land on its row: %+v", appErr.Fields)
	}
}

func TestDeleteLocksTheCourseWithinTx(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	c := mustCourse(t, svc, sc, "A1")
	if err := svc.Delete(context.Background(), sc, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if d.tx.calls != 1 {
		t.Fatalf("delete must lock, count and stamp in one transaction, got %d", d.tx.calls)
	}
}

func TestArchiveIsIdempotentlyRefused(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	c := mustCourse(t, svc, sc, "A1")
	d.repo.classes[c.ID] = fakeClassCount{running: 2}
	out, err := svc.Archive(context.Background(), sc, c.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if out.Status != StatusArchived || out.ClassesRunning != 2 {
		t.Fatalf("archive must leave classes attached: %+v", out)
	}
	_, err = svc.Archive(context.Background(), sc, c.ID)
	requireAppError(t, err, http.StatusConflict, CodeCourseArchived)
}

func TestDeleteRefusedWhileClassesAttached(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	c := mustCourse(t, svc, sc, "A1")
	d.repo.classes[c.ID] = fakeClassCount{upcoming: 1}
	err := svc.Delete(context.Background(), sc, c.ID)
	requireAppError(t, err, http.StatusConflict, CodeCourseInUse)

	delete(d.repo.classes, c.ID)
	if err := svc.Delete(context.Background(), sc, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = svc.Get(context.Background(), sc, c.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	// The code is free again after the soft delete.
	mustCourse(t, svc, sc, "A1")
}

func TestSetTuitionPacksReplacesInOrderWithinTx(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	c := mustCourse(t, svc, sc, "A1")
	out, err := svc.SetTuitionPacks(context.Background(), sc, c.ID, []TuitionPackInput{
		{Name: " Gói 12 buổi ", Sessions: 12, Price: 1200000},
		{Name: "Gói 24 buổi", Sessions: 24, Price: 2200000},
	})
	if err != nil {
		t.Fatalf("set packs: %v", err)
	}
	if len(out) != 2 || out[0].Position != 1 || out[0].Name != "Gói 12 buổi" || out[1].Position != 2 {
		t.Fatalf("unexpected packs %+v", out)
	}
	if d.tx.calls != 1 {
		t.Fatalf("replace must run in one transaction, got %d", d.tx.calls)
	}
	got, err := svc.Get(context.Background(), sc, c.ID)
	if err != nil || len(got.TuitionPacks) != 2 {
		t.Fatalf("packs must ride along with the course: %v %+v", err, got)
	}
	out, err = svc.SetTuitionPacks(context.Background(), sc, c.ID, []TuitionPackInput{})
	if err != nil || len(out) != 0 {
		t.Fatalf("[] must clear: %v %+v", err, out)
	}
	too := make([]TuitionPackInput, maxTuitionPacks+1)
	for i := range too {
		too[i] = TuitionPackInput{Name: "g", Sessions: 1}
	}
	_, err = svc.SetTuitionPacks(context.Background(), sc, c.ID, too)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetTuitionPacks(context.Background(), sc, uuid.New(), nil)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestListAttachesPacksAndFiltersStatus(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	a := mustCourse(t, svc, sc, "A1")
	b := mustCourse(t, svc, sc, "B1")
	if _, err := svc.Archive(context.Background(), sc, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetTuitionPacks(context.Background(), sc, a.ID, []TuitionPackInput{{Name: "g", Sessions: 8, Price: 1}}); err != nil {
		t.Fatal(err)
	}
	rows, total, err := svc.List(context.Background(), sc, ListFilter{}, pagination.Params{})
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("list: %v total=%d rows=%d", err, total, len(rows))
	}
	for _, r := range rows {
		if r.ID == a.ID && len(r.TuitionPacks) != 1 {
			t.Fatalf("course A must carry its pack: %+v", r)
		}
		if r.ID == b.ID && len(r.TuitionPacks) != 0 {
			t.Fatalf("course B must have no packs: %+v", r)
		}
	}
	rows, total, err = svc.List(context.Background(), sc, ListFilter{Status: StatusArchived}, pagination.Params{})
	if err != nil || total != 1 || rows[0].ID != b.ID {
		t.Fatalf("status filter: %v total=%d", err, total)
	}
}

func TestPermissionsAndCenterBoundary(t *testing.T) {
	svc, d := newTestService()
	owner := d.ownerScope()
	c := mustCourse(t, svc, owner, "A1")

	reader := d.memberWith(authctx.PermCoursesRead)
	if _, err := svc.Get(context.Background(), reader, c.ID); err != nil {
		t.Fatalf("reader get: %v", err)
	}
	_, err := svc.Create(context.Background(), reader, CourseRequest{Code: "B1", Name: "b"})
	requireAppError(t, err, http.StatusForbidden, "")
	_, err = svc.Archive(context.Background(), reader, c.ID)
	requireAppError(t, err, http.StatusForbidden, "")
	requireAppError(t, svc.Delete(context.Background(), reader, c.ID), http.StatusForbidden, "")
	_, err = svc.SetTuitionPacks(context.Background(), reader, c.ID, nil)
	requireAppError(t, err, http.StatusForbidden, "")

	editor := d.memberWith(authctx.PermCoursesEdit)
	if _, err := svc.Get(context.Background(), editor, c.ID); err != nil {
		t.Fatalf("edit implies read: %v", err)
	}
	nobody := d.memberWith()
	_, err = svc.Get(context.Background(), nobody, c.ID)
	requireAppError(t, err, http.StatusForbidden, "")

	outsider := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}
	_, err = svc.Get(context.Background(), outsider, c.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.Update(context.Background(), outsider, c.ID, CourseRequest{Code: "A1", Name: "x"})
	requireAppError(t, err, http.StatusNotFound, "")
	// The outsider may reuse the code in its own center.
	if _, err := svc.Create(context.Background(), outsider, CourseRequest{Code: "A1", Name: "x"}); err != nil {
		t.Fatalf("codes are per center: %v", err)
	}
}

func TestDeleteIsRefusedWhileACourseSitsInAPath(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	c := mustCourse(t, svc, sc, "A1")
	d.repo.paths[c.ID] = []CoursePath{{ID: uuid.New(), Code: "LT-6", Name: "Lộ trình 6", Status: "active",
		StageID: uuid.New(), StageName: "Nền tảng", StagePosition: 1}}
	err := svc.Delete(context.Background(), sc, c.ID)
	requireAppError(t, err, http.StatusConflict, CodeCourseInPath)
	// Archiving stays open: the path keeps showing the course as archived.
	if _, err := svc.Archive(context.Background(), sc, c.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	delete(d.repo.paths, c.ID)
	if err := svc.Delete(context.Background(), sc, c.ID); err != nil {
		t.Fatalf("delete once detached: %v", err)
	}
}

func TestListPathsNeedsPathsReadAndALiveCourse(t *testing.T) {
	svc, d := newTestService()
	sc := d.ownerScope()
	c := mustCourse(t, svc, sc, "A1")
	stage := uuid.New()
	d.repo.paths[c.ID] = []CoursePath{{ID: uuid.New(), Code: "LT-6", Name: "Lộ trình 6", Status: "draft",
		StageID: stage, StageName: "Nền tảng", StagePosition: 2}}

	out, err := svc.ListPaths(context.Background(), sc, c.ID)
	if err != nil || len(out) != 1 || out[0].StageID != stage || out[0].StagePosition != 2 || out[0].Code != "LT-6" {
		t.Fatalf("list paths: %v %+v", err, out)
	}
	_, err = svc.ListPaths(context.Background(), d.memberWith(authctx.PermCoursesRead), c.ID)
	requireAppError(t, err, http.StatusForbidden, "")
	if _, err := svc.ListPaths(context.Background(), d.memberWith(authctx.PermCoursesRead, authctx.PermPathsRead), c.ID); err != nil {
		t.Fatalf("courses.read + paths.read must list: %v", err)
	}
	_, err = svc.ListPaths(context.Background(), d.memberWith(authctx.PermPathsRead), c.ID)
	requireAppError(t, err, http.StatusForbidden, "")
	_, err = svc.ListPaths(context.Background(), sc, uuid.New())
	requireAppError(t, err, http.StatusNotFound, "")
	other := mustCourse(t, svc, sc, "B1")
	if out, err := svc.ListPaths(context.Background(), sc, other.ID); err != nil || out == nil || len(out) != 0 {
		t.Fatalf("a course outside every path must list []: %v %+v", err, out)
	}
}
