package library

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

// materialRow counts, like materialCountSelect, the live template lessons
// linking the material and the distinct templates among them, across every
// version status.
func (f *fakeRepo) materialRow(sc authctx.Scope, m *Material) MaterialRow {
	row := MaterialRow{Material: *m}
	seen := map[uuid.UUID]bool{}
	for lid, links := range f.lessonMaterials {
		lesson, ok := f.lessons[lid]
		if !ok {
			continue
		}
		v, ok := f.versions[lesson.VersionID]
		if !ok {
			continue
		}
		tpl, ok := f.templates[v.TemplateID]
		if !ok || tpl.DeletedAt != nil {
			continue
		}
		for _, l := range links {
			if l.CenterID != sc.CenterID || l.MaterialID != m.ID {
				continue
			}
			row.LessonCount++
			if !seen[tpl.ID] {
				seen[tpl.ID] = true
				row.TemplateCount++
			}
		}
	}
	return row
}

func (f *fakeRepo) GetMaterial(_ context.Context, sc authctx.Scope, id uuid.UUID) (*MaterialRow, error) {
	m, ok := f.liveMaterial(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	row := f.materialRow(sc, m)
	return &row, nil
}

func (f *fakeRepo) ListMaterials(_ context.Context, sc authctx.Scope, fl ListFilter, _ pagination.Params) ([]MaterialRow, int64, error) {
	var out []MaterialRow
	for _, m := range f.materials {
		if m.CenterID == sc.CenterID && m.DeletedAt == nil &&
			(fl.Q == "" || strings.Contains(strings.ToLower(m.Title), strings.ToLower(fl.Q))) &&
			(fl.Active == nil || m.Active == *fl.Active) {
			out = append(out, f.materialRow(sc, m))
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

func (f *fakeRepo) SetMaterialActive(_ context.Context, sc authctx.Scope, id uuid.UUID, active bool) error {
	cur, ok := f.liveMaterial(sc, id)
	if !ok {
		return ErrNotFound
	}
	cur.Active = active
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

func (f *fakeRepo) LockMaterial(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Material, error) {
	m, ok := f.liveMaterial(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *m
	return &cp, nil
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
	for _, other := range f.exercises {
		if other.CenterID == sc.CenterID && other.DeletedAt == nil && other.Code == e.Code {
			return gorm.ErrDuplicatedKey
		}
	}
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

// exerciseRow mirrors materialRow for exercises.
func (f *fakeRepo) exerciseRow(sc authctx.Scope, e *Exercise) ExerciseRow {
	row := ExerciseRow{Exercise: *e}
	seen := map[uuid.UUID]bool{}
	for lid, links := range f.lessonExercises {
		lesson, ok := f.lessons[lid]
		if !ok {
			continue
		}
		v, ok := f.versions[lesson.VersionID]
		if !ok {
			continue
		}
		tpl, ok := f.templates[v.TemplateID]
		if !ok || tpl.DeletedAt != nil {
			continue
		}
		for _, l := range links {
			if l.CenterID != sc.CenterID || l.ExerciseID != e.ID {
				continue
			}
			row.LessonCount++
			if !seen[tpl.ID] {
				seen[tpl.ID] = true
				row.TemplateCount++
			}
		}
	}
	return row
}

func (f *fakeRepo) GetExercise(_ context.Context, sc authctx.Scope, id uuid.UUID) (*ExerciseRow, error) {
	e, ok := f.liveExercise(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	row := f.exerciseRow(sc, e)
	return &row, nil
}

func (f *fakeRepo) ListExercises(_ context.Context, sc authctx.Scope, fl ListFilter, _ pagination.Params) ([]ExerciseRow, int64, error) {
	var out []ExerciseRow
	for _, e := range f.exercises {
		if e.CenterID != sc.CenterID || e.DeletedAt != nil {
			continue
		}
		if fl.Q != "" {
			needle := strings.ToLower(fl.Q)
			if !strings.Contains(strings.ToLower(e.Title), needle) && !strings.Contains(strings.ToLower(e.Code), needle) {
				continue
			}
		}
		if fl.Active != nil && e.Active != *fl.Active {
			continue
		}
		out = append(out, f.exerciseRow(sc, e))
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
	for _, other := range f.exercises {
		if other.ID != e.ID && other.CenterID == sc.CenterID && other.DeletedAt == nil && other.Code == e.Code {
			return gorm.ErrDuplicatedKey
		}
	}
	cur.Title, cur.Description, cur.Difficulty, cur.Tags = e.Title, e.Description, e.Difficulty, e.Tags
	cur.Code, cur.Skill, cur.Level = e.Code, e.Skill, e.Level
	cur.UpdatedAt = nowUTC()
	return nil
}

func (f *fakeRepo) SetExerciseActive(_ context.Context, sc authctx.Scope, id uuid.UUID, active bool) error {
	cur, ok := f.liveExercise(sc, id)
	if !ok {
		return ErrNotFound
	}
	cur.Active = active
	cur.UpdatedAt = nowUTC()
	return nil
}

// NextExerciseCode mirrors the repository's BT-0001-style generator,
// scanning every row of the center (including soft-deleted) so a code is
// never reissued.
func (f *fakeRepo) NextExerciseCode(_ context.Context, sc authctx.Scope) (string, error) {
	next := 1
	for _, e := range f.exercises {
		if e.CenterID != sc.CenterID || !strings.HasPrefix(e.Code, exerciseCodePrefix) {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(e.Code, exerciseCodePrefix)); err == nil && n >= next {
			next = n + 1
		}
	}
	return fmt.Sprintf("%s%04d", exerciseCodePrefix, next), nil
}

// LockCenterForExerciseCode is a no-op: the in-memory fake runs every call
// on a single goroutine, so there is no concurrent writer to serialise
// against; the real advisory lock is covered by the integration test.
func (f *fakeRepo) LockCenterForExerciseCode(_ context.Context, _ authctx.Scope) error {
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

func (f *fakeRepo) LockExercise(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Exercise, error) {
	e, ok := f.liveExercise(sc, id)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
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

// TestMaterialOtherKindOnlyEditableOnExistingLegacyRows covers the
// asymmetry between creating and updating a material of the retired
// "other" kind: it must never be chosen for a new material, but editing a
// material that already has that kind (seeded before the kind was retired)
// must still work as long as the kind itself is left unchanged.
func TestMaterialOtherKindOnlyEditableOnExistingLegacyRows(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	_, err := svc.CreateMaterial(ctx, sc, MaterialRequest{Title: "Học liệu cũ", Kind: MaterialKindOther})
	appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Fields["kind"] == "" {
		t.Fatalf("creating with kind=other must be rejected, got %+v", appErr.Fields)
	}

	legacyID := uuid.New()
	d.repo.materials[legacyID] = &Material{
		ID: legacyID, CenterID: sc.CenterID, Title: "Học liệu cũ", Kind: MaterialKindOther, Active: true,
		CreatedAt: nowUTC(), UpdatedAt: nowUTC(),
	}

	kept, err := svc.UpdateMaterial(ctx, sc, legacyID, MaterialRequest{Title: "Học liệu cũ (sửa)", Kind: MaterialKindOther})
	if err != nil || kept.Kind != MaterialKindOther || kept.Title != "Học liệu cũ (sửa)" {
		t.Fatalf("updating a legacy other-kind material while keeping its kind must succeed: %v %+v", err, kept)
	}

	otherID := uuid.New()
	d.repo.materials[otherID] = &Material{
		ID: otherID, CenterID: sc.CenterID, Title: "Video mới", Kind: MaterialKindVideo, Active: true,
		CreatedAt: nowUTC(), UpdatedAt: nowUTC(),
	}
	_, err = svc.UpdateMaterial(ctx, sc, otherID, MaterialRequest{Title: "Video mới", Kind: MaterialKindOther})
	appErr = appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Fields["kind"] == "" {
		t.Fatalf("changing a non-other material to kind=other must be rejected, got %+v", appErr.Fields)
	}
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

	bad := []ScoreSetGroupInput{{Key: "Giữa kỳ", Title: "Giữa kỳ",
		Components: []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 10, Weight: 1}}}}
	_, err := svc.SetScoreSet(ctx, sc, draft.ID, bad)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	dupGroup := []ScoreSetGroupInput{{Key: "main", Title: "Giữa kỳ"}, {Key: "main", Title: "Cuối kỳ"}}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, dupGroup)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, []ScoreSetGroupInput{{Key: "main", Title: "  "}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	dupComponent := []ScoreSetGroupInput{{Key: "main", Title: "Chính", Components: []ScoreComponentInput{
		{Key: "gk", Label: "Giữa kỳ", Max: 10}, {Key: "gk", Label: "Cuối kỳ", Max: 10},
	}}}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, dupComponent)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetScoreSet(ctx, sc, draft.ID,
		[]ScoreSetGroupInput{{Key: "main", Title: "Chính", Components: []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 0}}}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	_, err = svc.SetScoreSet(ctx, sc, draft.ID,
		[]ScoreSetGroupInput{{Key: "main", Title: "Chính", Components: []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 10, Weight: -1}}}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	tooManyGroups := make([]ScoreSetGroupInput, 11)
	for i := range tooManyGroups {
		tooManyGroups[i] = ScoreSetGroupInput{Key: "g" + string(rune('a'+i)), Title: "x"}
	}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, tooManyGroups)
	requireAppError(t, err, http.StatusUnprocessableEntity, "")
	tooManyComponents := make([]ScoreComponentInput, 21)
	for i := range tooManyComponents {
		tooManyComponents[i] = ScoreComponentInput{Key: "k" + string(rune('a'+i)), Label: "x", Max: 1}
	}
	_, err = svc.SetScoreSet(ctx, sc, draft.ID, []ScoreSetGroupInput{{Key: "main", Title: "Chính", Components: tooManyComponents}})
	requireAppError(t, err, http.StatusUnprocessableEntity, "")

	d.repo.locks = 0
	set, err := svc.SetScoreSet(ctx, sc, draft.ID, []ScoreSetGroupInput{
		{Key: "main", Title: " Chính ", Components: []ScoreComponentInput{
			{Key: "gk", Label: " Giữa kỳ ", Max: 10, Weight: 0.4},
			{Key: "ck", Label: "Cuối kỳ", Max: 10, Weight: 0.6},
		}},
		{Key: "bonus", Title: "Cộng điểm", Components: []ScoreComponentInput{
			// Same component key as group "main": uniqueness is per group, not global.
			{Key: "gk", Label: "Điểm thưởng giữa kỳ", Max: 2},
		}},
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if d.repo.locks != 1 {
		t.Errorf("score-set writes must take the version lock, got %d", d.repo.locks)
	}
	if len(set) != 2 || set[0].Key != "main" || set[0].Title != "Chính" || len(set[0].Components) != 2 ||
		set[0].Components[0].Key != "gk" || set[0].Components[1].Weight != 0.6 {
		t.Fatalf("unexpected score set %+v", set)
	}
	if set[1].Key != "bonus" || len(set[1].Components) != 1 || set[1].Components[0].Key != "gk" {
		t.Fatalf("component keys must be unique per group, not globally: %+v", set[1])
	}

	detail, err := svc.GetVersion(ctx, sc, draft.ID)
	if err != nil || len(detail.ScoreSet) != 2 || detail.ScoreSet[1].Key != "bonus" {
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
	if _, err := svc.SetScoreSet(ctx, sc, v1.ID, []ScoreSetGroupInput{
		{Key: "main", Title: "Chính", Components: []ScoreComponentInput{{Key: "gk", Label: "Giữa kỳ", Max: 10, Weight: 1}}},
	}); err != nil {
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

func TestExerciseCodeIsAutoGeneratedWhenOmitted(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	first, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài 1"})
	if err != nil || first.Code != "BT-0001" {
		t.Fatalf("first generated code: %v %+v", err, first)
	}
	second, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài 2"})
	if err != nil || second.Code != "BT-0002" {
		t.Fatalf("second generated code must not reuse the first: %v %+v", err, second)
	}
	// A blank code (whitespace only) is treated the same as an omitted one.
	third, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài 3", Code: str("   ")})
	if err != nil || third.Code != "BT-0003" {
		t.Fatalf("blank code must also auto-generate: %v %+v", err, third)
	}
}

// TestExerciseCodeAutoGenerationIgnoresNonNumericCallerCodes covers a
// caller-chosen code that shares the generator's prefix but not its
// BT-<digits> shape: it must not derail the auto-generated sequence, and a
// later caller-chosen numeric code must still push the sequence forward.
func TestExerciseCodeAutoGenerationIgnoresNonNumericCallerCodes(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	chosen, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài chọn tay", Code: str("BT-ABC")})
	if err != nil || chosen.Code != "BT-ABC" {
		t.Fatalf("caller-chosen non-numeric code must be kept as-is: %v %+v", err, chosen)
	}

	generated, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài tự sinh"})
	if err != nil || generated.Code != "BT-0001" {
		t.Fatalf("auto-generation must ignore a non-numeric caller code and start at BT-0001: %v %+v", err, generated)
	}

	numeric, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài số", Code: str("BT-0009")})
	if err != nil || numeric.Code != "BT-0009" {
		t.Fatalf("caller-chosen numeric code: %v %+v", err, numeric)
	}
	next, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài tự sinh kế tiếp"})
	if err != nil || next.Code != "BT-0010" {
		t.Fatalf("auto-generation must resume after the highest numeric code seen so far: %v %+v", err, next)
	}
}

func TestExerciseCodeExplicitIsNormalizedAndUniquePerCenter(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	created, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài 1", Code: str(" bt-01 ")})
	if err != nil || created.Code != "BT-01" {
		t.Fatalf("code must be trimmed and upper-cased: %v %+v", err, created)
	}

	_, err = svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài 2", Code: str("bt-01")})
	requireAppError(t, err, http.StatusConflict, CodeExerciseCodeTaken)

	// A code outside another center does not clash.
	other := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true}
	if _, err := svc.CreateExercise(ctx, other, ExerciseRequest{Title: "Bài khác", Code: str("bt-01")}); err != nil {
		t.Fatalf("a code of another center must not clash: %v", err)
	}

	appErr := appErrorOf(t, mustErr(svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "x", Code: str("BT 01")})), http.StatusUnprocessableEntity)
	if appErr.Fields["code"] == "" {
		t.Fatalf("a code outside letters/digits/dashes must be rejected, got %+v", appErr.Fields)
	}

	// Updating without a code keeps the current one; updating with a taken
	// code (of another exercise) is rejected the same way as create.
	if _, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Bài 2", Code: str("bt-02")}); err != nil {
		t.Fatalf("create second: %v", err)
	}
	kept, err := svc.UpdateExercise(ctx, sc, created.ID, ExerciseRequest{Title: "Bài 1 (mới)"})
	if err != nil || kept.Code != "BT-01" {
		t.Fatalf("update without a code must keep the current one: %v %+v", err, kept)
	}
	_, err = svc.UpdateExercise(ctx, sc, created.ID, ExerciseRequest{Title: "Bài 1", Code: str("bt-02")})
	requireAppError(t, err, http.StatusConflict, CodeExerciseCodeTaken)
}

func TestMaterialAndExerciseStatusToggle(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	editor := d.memberWith(authctx.PermLibraryRead, authctx.PermLibraryEdit)
	reader := d.memberWith(authctx.PermLibraryRead)

	m := mustMaterial(t, svc, editor, "Tài liệu")
	if !m.Active {
		t.Fatalf("a new material must start active, got %+v", m)
	}
	e := mustExercise(t, svc, editor, "Bài tập")
	if !e.Active {
		t.Fatalf("a new exercise must start active, got %+v", e)
	}

	if _, err := svc.SetMaterialStatus(ctx, reader, m.ID, false); err == nil {
		t.Error("status change needs library.edit")
	}
	updated, err := svc.SetMaterialStatus(ctx, editor, m.ID, false)
	if err != nil || updated.Active {
		t.Fatalf("material must turn inactive: %v %+v", err, updated)
	}
	got, err := svc.GetMaterial(ctx, editor, m.ID)
	if err != nil || got.Active {
		t.Fatalf("the toggle must persist: %v %+v", err, got)
	}
	if _, err := svc.SetMaterialStatus(ctx, editor, uuid.New(), false); err == nil {
		t.Error("status change on an unknown material must 404")
	}

	updatedEx, err := svc.SetExerciseStatus(ctx, editor, e.ID, false)
	if err != nil || updatedEx.Active {
		t.Fatalf("exercise must turn inactive: %v %+v", err, updatedEx)
	}
	if _, err := svc.SetExerciseStatus(ctx, reader, e.ID, true); err == nil {
		t.Error("status change needs library.edit")
	}
}

func TestActiveFilterOnListMaterialsAndExercises(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	m1 := mustMaterial(t, svc, sc, "A")
	m2 := mustMaterial(t, svc, sc, "B")
	if _, err := svc.SetMaterialStatus(ctx, sc, m2.ID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	e1 := mustExercise(t, svc, sc, "Bài 1")
	e2 := mustExercise(t, svc, sc, "Bài 2")
	if _, err := svc.SetExerciseStatus(ctx, sc, e2.ID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	active := true
	list, total, err := svc.ListMaterials(ctx, sc, ListFilter{Active: &active}, pagination.Params{})
	if err != nil || total != 1 || list[0].ID != m1.ID {
		t.Fatalf("active=true must keep only the active material: %v total=%d list=%+v", err, total, list)
	}
	inactive := false
	list, total, err = svc.ListMaterials(ctx, sc, ListFilter{Active: &inactive}, pagination.Params{})
	if err != nil || total != 1 || list[0].ID != m2.ID {
		t.Fatalf("active=false must keep only the inactive material: %v total=%d list=%+v", err, total, list)
	}
	_, total, err = svc.ListMaterials(ctx, sc, ListFilter{}, pagination.Params{})
	if err != nil || total != 2 {
		t.Fatalf("no filter must keep both: %v total=%d", err, total)
	}

	exList, total, err := svc.ListExercises(ctx, sc, ListFilter{Active: &active}, pagination.Params{})
	if err != nil || total != 1 || exList[0].ID != e1.ID {
		t.Fatalf("active=true must keep only the active exercise: %v total=%d list=%+v", err, total, exList)
	}
	exList, total, err = svc.ListExercises(ctx, sc, ListFilter{Active: &inactive}, pagination.Params{})
	if err != nil || total != 1 || exList[0].ID != e2.ID {
		t.Fatalf("active=false must keep only the inactive exercise: %v total=%d list=%+v", err, total, exList)
	}
}

func TestExerciseSearchMatchesCode(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()

	created, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Phân số", Code: str("XYZ-9")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateExercise(ctx, sc, ExerciseRequest{Title: "Số học"}); err != nil {
		t.Fatalf("create other: %v", err)
	}

	list, total, err := svc.ListExercises(ctx, sc, ListFilter{Q: "xyz-9"}, pagination.Params{})
	if err != nil || total != 1 || list[0].ID != created.ID {
		t.Fatalf("q must match the code case-insensitively: %v total=%d list=%+v", err, total, list)
	}
	list, total, err = svc.ListExercises(ctx, sc, ListFilter{Q: "phân"}, pagination.Params{})
	if err != nil || total != 1 || list[0].ID != created.ID {
		t.Fatalf("q must still match the title: %v total=%d list=%+v", err, total, list)
	}
}

func TestLessonPickersRejectInactiveMaterialAndExerciseButKeepExistingLinks(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	sc := d.ownerScope()
	tpl := mustTemplate(t, svc, sc, "CT01")
	draft := draftOf(t, svc, sc, tpl.ID)
	lesson := mustLesson(t, svc, sc, draft.ID, "Buổi 1")
	m := mustMaterial(t, svc, sc, "A")
	e := mustExercise(t, svc, sc, "Bài 1")

	// Attach while both are still active.
	if _, err := svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: m.ID}}); err != nil {
		t.Fatalf("attach material: %v", err)
	}
	if _, err := svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: e.ID}}); err != nil {
		t.Fatalf("attach exercise: %v", err)
	}

	if _, err := svc.SetMaterialStatus(ctx, sc, m.ID, false); err != nil {
		t.Fatalf("deactivate material: %v", err)
	}
	if _, err := svc.SetExerciseStatus(ctx, sc, e.ID, false); err != nil {
		t.Fatalf("deactivate exercise: %v", err)
	}

	// Resubmitting the same, now-inactive, already-attached item is fine.
	rows, err := svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: m.ID}})
	if err != nil || len(rows) != 1 || rows[0].ID != m.ID {
		t.Fatalf("an already-attached inactive material must stay attachable: %v %+v", err, rows)
	}
	exRows, err := svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: e.ID}})
	if err != nil || len(exRows) != 1 || exRows[0].ID != e.ID {
		t.Fatalf("an already-attached inactive exercise must stay attachable: %v %+v", err, exRows)
	}

	// A fresh inactive material/exercise cannot be newly attached.
	other := mustMaterial(t, svc, sc, "B")
	if _, err := svc.SetMaterialStatus(ctx, sc, other.ID, false); err != nil {
		t.Fatalf("deactivate other material: %v", err)
	}
	_, err = svc.SetLessonMaterials(ctx, sc, lesson.ID, []LessonMaterialInput{{MaterialID: m.ID}, {MaterialID: other.ID}})
	appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Fields["material_id"] == "" {
		t.Fatalf("rejecting a new inactive material must name the field, got %+v", appErr.Fields)
	}

	otherEx := mustExercise(t, svc, sc, "Bài 2")
	if _, err := svc.SetExerciseStatus(ctx, sc, otherEx.ID, false); err != nil {
		t.Fatalf("deactivate other exercise: %v", err)
	}
	_, err = svc.SetLessonExercises(ctx, sc, lesson.ID, []LessonExerciseInput{{ExerciseID: e.ID}, {ExerciseID: otherEx.ID}})
	appErr = appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Fields["exercise_id"] == "" {
		t.Fatalf("rejecting a new inactive exercise must name the field, got %+v", appErr.Fields)
	}
}
