package library

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// --- fakeRepo: materials, exercises, lesson links, log fields, score set ---

func (f *fakeRepo) CreateMaterial(_ context.Context, sc authctx.Scope, m *Material) error {
	m.ID, m.CenterID = uuid.New(), sc.CenterID
	m.CreatedAt, m.UpdatedAt = nowUTC(), nowUTC()
	cp := *m
	f.materials[m.ID] = &cp
	return nil
}

func (f *fakeRepo) liveMaterial(sc authctx.Scope, id uuid.UUID) (*Material, bool) {
	m, ok := f.materials[id]
	if !ok || m.CenterID != sc.CenterID || m.DeletedAt != nil {
		return nil, false
	}
	return m, true
}

func (f *fakeRepo) GetMaterial(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error) {
	m, ok := f.liveMaterial(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeRepo) ListMaterials(_ context.Context, sc authctx.Scope, fl ListFilter, _ pagination.Params) ([]Material, int64, error) {
	var out []Material
	for _, m := range f.materials {
		if m.CenterID == sc.CenterID && m.DeletedAt == nil &&
			(fl.Q == "" || strings.Contains(strings.ToLower(m.Title), strings.ToLower(fl.Q))) {
			out = append(out, *m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, int64(len(out)), nil
}

func (f *fakeRepo) FindMaterials(_ context.Context, sc authctx.Scope, ids []uuid.UUID) ([]Material, error) {
	var out []Material
	for _, id := range ids {
		if m, ok := f.liveMaterial(sc, id); ok {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (f *fakeRepo) UpdateMaterial(_ context.Context, sc authctx.Scope, m *Material) error {
	cur, ok := f.liveMaterial(sc, m.ID)
	if !ok {
		return ErrNotFound
	}
	cur.Title, cur.Kind, cur.URL, cur.Description, cur.Tags = m.Title, m.Kind, m.URL, m.Description, m.Tags
	cur.UpdatedAt = nowUTC()
	return nil
}

func (f *fakeRepo) SoftDeleteMaterial(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	cur, ok := f.liveMaterial(sc, id)
	if !ok {
		return ErrNotFound
	}
	now := nowUTC()
	cur.DeletedAt = &now
	return nil
}

func (f *fakeRepo) LockMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error) {
	return f.GetMaterial(ctx, sc, id)
}

// usage tallies links by the status of their version, skipping lessons
// whose template is soft-deleted, the way the SQL join does.
func (f *fakeRepo) usage(sc authctx.Scope, lessonID uuid.UUID, u *ItemUsage) {
	l, ok := f.lessons[lessonID]
	if !ok || l.CenterID != sc.CenterID {
		return
	}
	v, ok := f.versions[l.VersionID]
	if !ok {
		return
	}
	if tpl, ok := f.templates[v.TemplateID]; !ok || tpl.DeletedAt != nil {
		return
	}
	if v.Status == StatusDraft {
		u.Draft++
	} else {
		u.Released++
	}
}

func (f *fakeRepo) MaterialUsage(_ context.Context, sc authctx.Scope, id uuid.UUID) (ItemUsage, error) {
	var u ItemUsage
	for lid, links := range f.lessonMaterials {
		for _, l := range links {
			if l.CenterID == sc.CenterID && l.MaterialID == id {
				f.usage(sc, lid, &u)
			}
		}
	}
	return u, nil
}

func (f *fakeRepo) CreateExercise(_ context.Context, sc authctx.Scope, e *Exercise) error {
	e.ID, e.CenterID = uuid.New(), sc.CenterID
	e.CreatedAt, e.UpdatedAt = nowUTC(), nowUTC()
	cp := *e
	f.exercises[e.ID] = &cp
	return nil
}

func (f *fakeRepo) liveExercise(sc authctx.Scope, id uuid.UUID) (*Exercise, bool) {
	e, ok := f.exercises[id]
	if !ok || e.CenterID != sc.CenterID || e.DeletedAt != nil {
		return nil, false
	}
	return e, true
}

func (f *fakeRepo) GetExercise(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error) {
	e, ok := f.liveExercise(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (f *fakeRepo) ListExercises(_ context.Context, sc authctx.Scope, fl ListFilter, _ pagination.Params) ([]Exercise, int64, error) {
	var out []Exercise
	for _, e := range f.exercises {
		if e.CenterID == sc.CenterID && e.DeletedAt == nil &&
			(fl.Q == "" || strings.Contains(strings.ToLower(e.Title), strings.ToLower(fl.Q))) {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, int64(len(out)), nil
}

func (f *fakeRepo) FindExercises(_ context.Context, sc authctx.Scope, ids []uuid.UUID) ([]Exercise, error) {
	var out []Exercise
	for _, id := range ids {
		if e, ok := f.liveExercise(sc, id); ok {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (f *fakeRepo) UpdateExercise(_ context.Context, sc authctx.Scope, e *Exercise) error {
	cur, ok := f.liveExercise(sc, e.ID)
	if !ok {
		return ErrNotFound
	}
	cur.Title, cur.Description, cur.Difficulty, cur.Tags = e.Title, e.Description, e.Difficulty, e.Tags
	cur.UpdatedAt = nowUTC()
	return nil
}

func (f *fakeRepo) SoftDeleteExercise(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	cur, ok := f.liveExercise(sc, id)
	if !ok {
		return ErrNotFound
	}
	now := nowUTC()
	cur.DeletedAt = &now
	return nil
}

func (f *fakeRepo) LockExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error) {
	return f.GetExercise(ctx, sc, id)
}

func (f *fakeRepo) ExerciseUsage(_ context.Context, sc authctx.Scope, id uuid.UUID) (ItemUsage, error) {
	var u ItemUsage
	for lid, links := range f.lessonExercises {
		for _, l := range links {
			if l.CenterID == sc.CenterID && l.ExerciseID == id {
				f.usage(sc, lid, &u)
			}
		}
	}
	return u, nil
}

func (f *fakeRepo) ListLessonMaterials(_ context.Context, sc authctx.Scope, lessonIDs []uuid.UUID) ([]LessonMaterialRow, error) {
	var out []LessonMaterialRow
	for _, lid := range lessonIDs {
		for _, link := range f.lessonMaterials[lid] {
			m := f.materials[link.MaterialID]
			if link.CenterID != sc.CenterID || m == nil {
				continue
			}
			out = append(out, LessonMaterialRow{
				Material: *m, LessonID: lid, SharedWithStudents: link.SharedWithStudents, Position: link.Position,
			})
		}
	}
	return out, nil
}

func (f *fakeRepo) ReplaceLessonMaterials(_ context.Context, sc authctx.Scope, lessonID uuid.UUID, rows []LessonMaterial) error {
	links := make([]LessonMaterial, 0, len(rows))
	for i, r := range rows {
		r.LessonID, r.CenterID, r.Position = lessonID, sc.CenterID, i+1
		links = append(links, r)
	}
	f.lessonMaterials[lessonID] = links
	return nil
}

func (f *fakeRepo) CreateLessonMaterials(_ context.Context, sc authctx.Scope, rows []LessonMaterial) error {
	for _, r := range rows {
		r.CenterID = sc.CenterID
		f.lessonMaterials[r.LessonID] = append(f.lessonMaterials[r.LessonID], r)
	}
	return nil
}

func (f *fakeRepo) ListLessonExercises(_ context.Context, sc authctx.Scope, lessonIDs []uuid.UUID) ([]LessonExerciseRow, error) {
	var out []LessonExerciseRow
	for _, lid := range lessonIDs {
		for _, link := range f.lessonExercises[lid] {
			e := f.exercises[link.ExerciseID]
			if link.CenterID != sc.CenterID || e == nil {
				continue
			}
			out = append(out, LessonExerciseRow{Exercise: *e, LessonID: lid, Position: link.Position})
		}
	}
	return out, nil
}

func (f *fakeRepo) ReplaceLessonExercises(_ context.Context, sc authctx.Scope, lessonID uuid.UUID, rows []LessonExercise) error {
	links := make([]LessonExercise, 0, len(rows))
	for i, r := range rows {
		r.LessonID, r.CenterID, r.Position = lessonID, sc.CenterID, i+1
		links = append(links, r)
	}
	f.lessonExercises[lessonID] = links
	return nil
}

func (f *fakeRepo) CreateLessonExercises(_ context.Context, sc authctx.Scope, rows []LessonExercise) error {
	for _, r := range rows {
		r.CenterID = sc.CenterID
		f.lessonExercises[r.LessonID] = append(f.lessonExercises[r.LessonID], r)
	}
	return nil
}

func (f *fakeRepo) ListLogFields(_ context.Context, sc authctx.Scope, versionID uuid.UUID) ([]LogField, error) {
	var out []LogField
	for _, lf := range f.logFields[versionID] {
		if lf.CenterID == sc.CenterID {
			out = append(out, lf)
		}
	}
	return out, nil
}

func (f *fakeRepo) ReplaceLogFields(_ context.Context, sc authctx.Scope, versionID uuid.UUID, rows []*LogField) error {
	fields := make([]LogField, 0, len(rows))
	for i, r := range rows {
		r.ID, r.VersionID, r.CenterID, r.Position = uuid.New(), versionID, sc.CenterID, i+1
		fields = append(fields, *r)
	}
	f.logFields[versionID] = fields
	return nil
}

func (f *fakeRepo) SetScoreSet(_ context.Context, sc authctx.Scope, versionID uuid.UUID, set ScoreSet) error {
	v, ok := f.versions[versionID]
	if !ok || v.CenterID != sc.CenterID {
		return ErrNotFound
	}
	v.ScoreSet = set
	v.UpdatedAt = nowUTC()
	return nil
}

// --- helpers ---

func mustMaterial(t *testing.T, svc *Service, sc authctx.Scope, title string) *MaterialResponse {
	t.Helper()
	out, err := svc.CreateMaterial(context.Background(), sc, MaterialRequest{Title: title, Kind: MaterialKindLink, URL: str("https://example.com/" + title)})
	if err != nil {
		t.Fatalf("create material %q: %v", title, err)
	}
	return out
}

func mustExercise(t *testing.T, svc *Service, sc authctx.Scope, title string) *ExerciseResponse {
	t.Helper()
	out, err := svc.CreateExercise(context.Background(), sc, ExerciseRequest{Title: title})
	if err != nil {
		t.Fatalf("create exercise %q: %v", title, err)
	}
	return out
}

// --- tests ---

func TestMaterialLifecycleAndPermissions(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	editor := d.memberWith(authctx.PermLibraryRead, authctx.PermLibraryEdit)
	reader := d.memberWith(authctx.PermLibraryRead)

	_, err := svc.CreateMaterial(ctx, reader, MaterialRequest{Title: "Video", Kind: MaterialKindVideo})
	requireAppError(t, err, http.StatusForbidden, "")

	created, err := svc.CreateMaterial(ctx, editor, MaterialRequest{
		Title: "  Video bài 1 ", Kind: MaterialKindVideo, URL: str(" https://youtu.be/x "),
		Description: str("  "), Tags: []string{" Toán ", "", "Toán", "Khối 6"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Title != "Video bài 1" || created.URL == nil || *created.URL != "https://youtu.be/x" || created.Description != nil {
		t.Errorf("text fields must be trimmed and blank stored as null, got %+v", created)
	}
	if got := strings.Join(created.Tags, ","); got != "Toán,Khối 6" {
		t.Errorf("tags must be trimmed, blanks dropped and duplicates collapsed, got %q", got)
	}

	got, err := svc.GetMaterial(ctx, reader, created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("reader must read the material: %v", err)
	}
	if _, err := svc.GetMaterial(ctx, d.memberWith(), created.ID); err == nil {
		t.Error("a member without library.read must not read materials")
	}

	list, total, err := svc.ListMaterials(ctx, reader, ListFilter{Q: "bài"}, pagination.Params{})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list by title fragment: %v total=%d len=%d", err, total, len(list))
	}

	updated, err := svc.UpdateMaterial(ctx, editor, created.ID, MaterialRequest{Title: "Video bài 1 (mới)", Kind: MaterialKindLink})
	if err != nil || updated.Title != "Video bài 1 (mới)" || updated.Kind != MaterialKindLink || len(updated.Tags) != 0 {
		t.Fatalf("update must replace every field: %v %+v", err, updated)
	}
	if _, err := svc.UpdateMaterial(ctx, reader, created.ID, MaterialRequest{Title: "x", Kind: MaterialKindLink}); err == nil {
		t.Error("update needs library.edit")
	}

	if err := svc.DeleteMaterial(ctx, editor, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = svc.GetMaterial(ctx, reader, created.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	err = svc.DeleteMaterial(ctx, editor, created.ID)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestExerciseLifecycle(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	created, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: " Bài 1 ", Difficulty: intp(3), Tags: []string{"phân số"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Title != "Bài 1" || created.Difficulty == nil || *created.Difficulty != 3 {
		t.Errorf("unexpected exercise %+v", created)
	}
	list, total, err := svc.ListExercises(ctx, sc, ListFilter{}, pagination.Params{})
	if err != nil || total != 1 || list[0].ID != created.ID {
		t.Fatalf("list: %v", err)
	}
	updated, err := svc.UpdateExercise(ctx, sc, created.ID, ExerciseRequest{Title: "Bài 1", Description: str("Rút gọn")})
	if err != nil || updated.Difficulty != nil || updated.Description == nil {
		t.Fatalf("update must replace every field: %v %+v", err, updated)
	}
	if err := svc.DeleteExercise(ctx, sc, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = svc.GetExercise(ctx, sc, created.ID)
	requireAppError(t, err, http.StatusNotFound, "")
}

func TestItemsOfAnotherCenterAreNotFound(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	mine := d.ownerScope()
	other := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}

	m := mustMaterial(t, svc, mine, "Tài liệu")
	e := mustExercise(t, svc, mine, "Bài tập")

	_, err := svc.GetMaterial(ctx, other, m.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	_, err = svc.UpdateMaterial(ctx, other, m.ID, MaterialRequest{Title: "x", Kind: MaterialKindLink})
	requireAppError(t, err, http.StatusNotFound, "")
	requireAppError(t, svc.DeleteMaterial(ctx, other, m.ID), http.StatusNotFound, "")
	_, err = svc.GetExercise(ctx, other, e.ID)
	requireAppError(t, err, http.StatusNotFound, "")
	requireAppError(t, svc.DeleteExercise(ctx, other, e.ID), http.StatusNotFound, "")
}

func TestLessonMaterialsAreReplacedWholesale(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	a := mustMaterial(t, svc, sc, "A")
	b := mustMaterial(t, svc, sc, "B")

	d.repo.locks = 0
	rows, err := svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{
		{MaterialID: b.ID, SharedWithStudents: true}, {MaterialID: a.ID},
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if d.repo.locks != 1 {
		t.Errorf("attaching materials must take the version lock, got %d", d.repo.locks)
	}
	if len(rows) != 2 || rows[0].ID != b.ID || rows[0].Position != 1 || !rows[0].SharedWithStudents ||
		rows[1].ID != a.ID || rows[1].Position != 2 || rows[1].SharedWithStudents {
		t.Fatalf("body order is the display order and the share flag is kept, got %+v", rows)
	}

	detail, err := svc.GetLesson(ctx, sc, lesson.ID)
	if err != nil || len(detail.Materials) != 2 || detail.Materials[0].Title != "A" && detail.Materials[0].Title != "B" {
		t.Fatalf("lesson detail must carry its materials: %v %+v", err, detail)
	}

	// A second PUT replaces the list instead of appending to it.
	rows, err = svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: a.ID}})
	if err != nil || len(rows) != 1 || rows[0].ID != a.ID || rows[0].Position != 1 {
		t.Fatalf("second put must replace: %v %+v", err, rows)
	}
	rows, err = svc.SetLessonMaterials(ctx, sc, lesson.ID, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("empty body clears the list: %v %+v", err, rows)
	}
}

func TestLessonMaterialsValidation(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	a := mustMaterial(t, svc, sc, "A")
	foreign := mustMaterial(t, svc, authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}, "Ngoài")

	_, err := svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: uuid.New()}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: foreign.ID}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: a.ID}, {MaterialID: a.ID}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLessonMaterials(ctx, d.memberWith(authctx.PermLibraryRead), lesson.ID, nil)
	requireAppError(t, err, http.StatusForbidden, "")
	_, err = svc.SetLessonMaterials(ctx, sc, uuid.New(), nil)
	requireAppError(t, err, http.StatusNotFound, "")
	tooMany := make([]LessonMaterialInput, maxLessonItems+1)
	for i := range tooMany {
		tooMany[i] = LessonMaterialInput{MaterialID: uuid.New()}
	}
	appErr := appErrorOf(t, mustErr(svc.SetLessonMaterials(ctx, sc, lesson.ID, tooMany)), http.StatusUnprocessableEntity)
	if appErr.Fields["materials"] == "" {
		t.Fatalf("oversized body must name the list, got %+v", appErr.Fields)
	}

	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	_, err = svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: a.ID}})
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)
}

// mustErr drops a call's value so its error can be asserted inline.
func mustErr[T any](_ T, err error) error { return err }

func TestLinkedItemsCannotBeDeleted(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	m := mustMaterial(t, svc, sc, "A")
	e := mustExercise(t, svc, sc, "Bài 1")

	if _, err := svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: m.ID}}); err != nil {
		t.Fatalf("attach material: %v", err)
	}
	rows, err := svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: e.ID}})
	if err != nil || len(rows) != 1 || rows[0].ID != e.ID || rows[0].Position != 1 {
		t.Fatalf("attach exercise: %v %+v", err, rows)
	}

	// Linked from a draft: the author can detach, so the message says so.
	requireAppError(t, svc.DeleteMaterial(ctx, sc, m.ID), http.StatusConflict, CodeMaterialInUse)
	requireAppError(t, svc.DeleteExercise(ctx, sc, e.ID), http.StatusConflict, CodeExerciseInUse)
	if msg := svc.DeleteMaterial(ctx, sc, m.ID).Error(); !strings.Contains(msg, "gỡ khỏi") {
		t.Fatalf("draft link message must ask to detach, got %q", msg)
	}

	// Links of a published version count too and cannot be detached, so
	// the message names the released version instead of the draft.
	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	requireAppError(t, svc.DeleteMaterial(ctx, sc, m.ID), http.StatusConflict, CodeMaterialInUse)
	requireAppError(t, svc.DeleteExercise(ctx, sc, e.ID), http.StatusConflict, CodeExerciseInUse)
	if msg := svc.DeleteMaterial(ctx, sc, m.ID).Error(); !strings.Contains(msg, "đã phát hành") {
		t.Fatalf("released link message must name the released version, got %q", msg)
	}
	if _, err := svc.SetLessonExercises(ctx, sc, lesson.ID, nil); err == nil {
		t.Fatal("published lesson must refuse a link change")
	}

	// Links kept by a deleted template are unreachable, so they stop
	// protecting the item.
	if err := svc.DeleteTemplate(ctx, sc, tpl.ID); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	if err := svc.DeleteMaterial(ctx, sc, m.ID); err != nil {
		t.Fatalf("material linked only from a deleted template must be deletable: %v", err)
	}
	if err := svc.DeleteExercise(ctx, sc, e.ID); err != nil {
		t.Fatalf("exercise linked only from a deleted template must be deletable: %v", err)
	}
}

func TestMaterialURLMustBeHTTP(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,hi", "ftp://files.example.com/a", "example.com/slide", "https://"} {
		_, err := svc.CreateMaterial(ctx, sc, MaterialRequest{Title: "x", Kind: MaterialKindLink, URL: str(bad)})
		appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
		if appErr.Fields["url"] == "" {
			t.Fatalf("%q: want a url field error, got %+v", bad, appErr.Fields)
		}
	}
	created, err := svc.CreateMaterial(ctx, sc, MaterialRequest{Title: "x", Kind: MaterialKindLink, URL: str(" https://example.com/slide ")})
	if err != nil || created.URL == nil || *created.URL != "https://example.com/slide" {
		t.Fatalf("http(s) url must be accepted and trimmed: %v %+v", err, created)
	}
	if _, err := svc.CreateMaterial(ctx, sc, MaterialRequest{Title: "y", Kind: MaterialKindDoc, URL: str("  ")}); err != nil {
		t.Fatalf("blank url means none: %v", err)
	}
	_, err = svc.UpdateMaterial(ctx, sc, created.ID, MaterialRequest{Title: "x", Kind: MaterialKindLink, URL: str("javascript:alert(1)")})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
}

func TestLessonExercisesValidation(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	e := mustExercise(t, svc, sc, "Bài 1")

	_, err := svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: uuid.New()}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: e.ID}, {ExerciseID: e.ID}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	tooMany := make([]LessonExerciseInput, maxLessonItems+1)
	for i := range tooMany {
		tooMany[i] = LessonExerciseInput{ExerciseID: uuid.New()}
	}
	appErr := appErrorOf(t, mustErr(svc.SetLessonExercises(ctx, sc, lesson.ID, tooMany)), http.StatusUnprocessableEntity)
	if appErr.Fields["exercises"] == "" {
		t.Fatalf("oversized body must name the list, got %+v", appErr.Fields)
	}
	rows, err := svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: e.ID}})
	if err != nil || len(rows) != 1 {
		t.Fatalf("set: %v", err)
	}
	detail, err := svc.GetLesson(ctx, sc, lesson.ID)
	if err != nil || len(detail.Exercises) != 1 || detail.Exercises[0].Title != "Bài 1" {
		t.Fatalf("lesson detail must carry its exercises: %v %+v", err, detail)
	}
}

func TestLogFieldsAreValidatedAndNumbered(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)

	_, err := svc.SetLogFields(ctx, sc, draft.ID, []LogFieldInput{{Label: "Mức độ", Kind: LogFieldSelect}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLogFields(ctx, sc, draft.ID, []LogFieldInput{{Label: "Mức độ", Kind: LogFieldSelect, Options: []string{" ", ""}}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLogFields(ctx, sc, draft.ID, []LogFieldInput{{Label: "   ", Kind: LogFieldText}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	tooMany := make([]LogFieldInput, 31)
	for i := range tooMany {
		tooMany[i] = LogFieldInput{Label: "Trường", Kind: LogFieldText}
	}
	_, err = svc.SetLogFields(ctx, sc, draft.ID, tooMany)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetLogFields(ctx, d.memberWith(authctx.PermLibraryRead), draft.ID, nil)
	requireAppError(t, err, http.StatusForbidden, "")

	d.repo.locks = 0
	fields, err := svc.SetLogFields(ctx, sc, draft.ID, []LogFieldInput{
		{Label: " Ghi chú ", Kind: LogFieldText, Options: []string{"bị bỏ"}, Required: true},
		{Label: "Mức độ", Kind: LogFieldSelect, Options: []string{" Tốt ", "Khá", "Tốt"}},
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if d.repo.locks != 1 {
		t.Errorf("log-field writes must take the version lock, got %d", d.repo.locks)
	}
	if len(fields) != 2 || fields[0].Position != 1 || fields[0].Label != "Ghi chú" || len(fields[0].Options) != 0 || !fields[0].Required {
		t.Fatalf("text field must drop options and keep its flags, got %+v", fields)
	}
	if fields[1].Position != 2 || strings.Join(fields[1].Options, ",") != "Tốt,Khá" {
		t.Fatalf("select options must be trimmed and deduplicated in order, got %+v", fields[1])
	}

	fields, err = svc.SetLogFields(ctx, sc, draft.ID, nil)
	if err != nil || len(fields) != 0 {
		t.Fatalf("empty body clears the fields: %v %+v", err, fields)
	}

	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	_, err = svc.SetLogFields(ctx, sc, draft.ID, []LogFieldInput{{Label: "x", Kind: LogFieldText}})
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)
}

func TestScoreSetIsValidatedAndStoredOnTheVersion(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)

	bad := []ScoreComponentInput{{Key: "Giữa kỳ", Label: "Giữa kỳ", Max: 10, Weight: 1}}
	_, err := svc.SetScoreSet(ctx, sc, draft.ID, bad)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	dup := []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 10}, {Key: "gk", Label: "Cuối kỳ", Max: 10}}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, dup)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 0}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 10, Weight: -1}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	tooMany := make([]ScoreComponentInput, 21)
	for i := range tooMany {
		tooMany[i] = ScoreComponentInput{Key: "k" + string(rune('a'+i)), Label: "x", Max: 1}
	}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, tooMany)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")

	d.repo.locks = 0
	set, err := svc.SetScoreSet(ctx, sc, draft.ID, []ScoreComponentInput{
		{Key: "gk", Label: " Giữa kỳ ", Max: 10, Weight: 0.4},
		{Key: "ck", Label: "Cuối kỳ", Max: 10, Weight: 0.6},
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if d.repo.locks != 1 {
		t.Errorf("score-set writes must take the version lock, got %d", d.repo.locks)
	}
	if len(set) != 2 || set[0].Key != "gk" || set[0].Label != "Giữa kỳ" || set[1].Weight != 0.6 {
		t.Fatalf("unexpected score set %+v", set)
	}

	detail, err := svc.GetVersion(ctx, sc, draft.ID)
	if err != nil || len(detail.ScoreSet) != 2 || detail.ScoreSet[1].Key != "ck" {
		t.Fatalf("version detail must carry the score set: %v %+v", err, detail)
	}
	if _, err := svc.GetVersion(ctx, d.memberWith(), draft.ID); err == nil {
		t.Error("version detail needs library.read")
	}

	if _, err := svc.Publish(ctx, sc, draft.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, nil)
	requireAppError(t, err, http.StatusConflict, CodeVersionLocked)
}

func TestNewDraftCopiesAttachmentsLogFieldsAndScoreSet(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	v1 := draftOf(t, svc, sc, tpl.ID)
	l1 := mustLesson(t, svc, sc, v1.ID, "Buổi 1")
	l2 := mustLesson(t, svc, sc, v1.ID, "Buổi 2")
	m := mustMaterial(t, svc, sc, "Tài liệu")
	e := mustExercise(t, svc, sc, "Bài tập")
	if _, err := svc.SetLessonMaterials(ctx, sc, l1.ID, []LessonMaterialInput{{MaterialID: m.ID, SharedWithStudents: true}}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := svc.SetLessonExercises(ctx, sc, l2.ID, []LessonExerciseInput{{ExerciseID: e.ID}}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := svc.SetLogFields(ctx, sc, v1.ID, []LogFieldInput{{Label: "Ghi chú", Kind: LogFieldText}}); err != nil {
		t.Fatalf("log fields: %v", err)
	}
	if _, err := svc.SetScoreSet(ctx, sc, v1.ID, []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 10, Weight: 1}}); err != nil {
		t.Fatalf("score set: %v", err)
	}
	if _, err := svc.Publish(ctx, sc, v1.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	v2, err := svc.CreateVersion(ctx, sc, tpl.ID, CreateVersionRequest{})
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	detail, err := svc.GetVersion(ctx, sc, v2.ID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Lessons) != 2 || len(detail.LogFields) != 1 || len(detail.ScoreSet) != 1 {
		t.Fatalf("v2 must copy lessons, log fields and score set, got %+v", detail)
	}
	if len(detail.Lessons[0].Materials) != 1 || !detail.Lessons[0].Materials[0].SharedWithStudents || detail.Lessons[0].Materials[0].ID != m.ID {
		t.Errorf("lesson 1 materials must be copied with the share flag, got %+v", detail.Lessons[0].Materials)
	}
	if len(detail.Lessons[1].Exercises) != 1 || detail.Lessons[1].Exercises[0].ID != e.ID {
		t.Errorf("lesson 2 exercises must be copied, got %+v", detail.Lessons[1].Exercises)
	}
	if detail.Lessons[0].ID == l1.ID {
		t.Error("copied lessons must be new rows")
	}
	// Copied log fields belong to v2, so v1's list is untouched.
	old, err := svc.GetVersion(ctx, sc, v1.ID)
	if err != nil || len(old.LogFields) != 1 || old.LogFields[0].ID == detail.LogFields[0].ID {
		t.Fatalf("v1 log fields must stay as they were: %v %+v", err, old)
	}
}
