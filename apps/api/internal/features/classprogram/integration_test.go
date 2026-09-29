//go:build integration

package classprogram_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classprogram"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/grading"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// fixture wires the real dependency chain router.go uses: classprogram
// consumes classes (read gate), teaching (the class curriculum), grading (the
// class score components) and library (published versions) through
// consumer-defined interfaces.
type fixture struct {
	db       *gorm.DB
	svc      *classprogram.Service
	classes  *classes.Service
	teaching *teaching.Service
	grading  *grading.Service
	library  *library.Service
	owner    authctx.Scope
	teacher  authctx.Scope
	class    *classes.Class
	outsider authctx.Scope
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	teachingSvc := teaching.NewService(teaching.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)
	gradingSvc := grading.NewService(grading.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)
	librarySvc := library.NewService(library.NewRepository(db), txMgr)
	svc := classprogram.NewService(classprogram.NewRepository(db), classesSvc, teachingSvc, librarySvc, gradingSvc, txMgr)

	_, ownerT := testutil.Teacher(t, db)
	_, teacherT := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, teacherT.ID, ownerT.CenterID)
	_, outsiderT := testutil.Teacher(t, db)

	return fixture{
		db:       db,
		svc:      svc,
		classes:  classesSvc,
		teaching: teachingSvc,
		grading:  gradingSvc,
		library:  librarySvc,
		owner:    testutil.ScopeFor(t, db, ownerT.ID),
		teacher:  testutil.ScopeFor(t, db, teacherT.ID),
		class:    testutil.Class(t, db, teacherT.ID),
		outsider: testutil.ScopeFor(t, db, outsiderT.ID),
	}
}

// publishedVersion builds a template under sc with the given lesson titles
// and publishes its first version.
func (f fixture) publishedVersion(t *testing.T, sc authctx.Scope, code string, titles ...string) library.VersionResponse {
	t.Helper()
	return f.scoredVersion(t, sc, code, nil, titles...)
}

// scoredVersion is publishedVersion with a score set on the version.
func (f fixture) scoredVersion(t *testing.T, sc authctx.Scope, code string, scoreSet []library.ScoreSetGroupInput, titles ...string) library.VersionResponse {
	t.Helper()
	ctx := context.Background()
	tpl, err := f.library.CreateTemplate(ctx, sc, library.TemplateRequest{Code: code, Name: "Chương trình " + code})
	require.NoError(t, err)
	draft := f.draft(t, sc, tpl.ID)
	for _, title := range titles {
		_, err := f.library.CreateLesson(ctx, sc, draft.ID, library.LessonRequest{Title: title})
		require.NoError(t, err)
	}
	if scoreSet != nil {
		_, err := f.library.SetScoreSet(ctx, sc, draft.ID, scoreSet)
		require.NoError(t, err)
	}
	published, err := f.library.Publish(ctx, sc, draft.ID)
	require.NoError(t, err)
	return *published
}

func (f fixture) draft(t *testing.T, sc authctx.Scope, templateID uuid.UUID) library.VersionResponse {
	t.Helper()
	versions, err := f.library.ListVersions(context.Background(), sc, templateID)
	require.NoError(t, err)
	for _, v := range versions {
		if v.Status == library.StatusDraft {
			return v
		}
	}
	t.Fatalf("template %s has no draft", templateID)
	return library.VersionResponse{}
}

func requireStatus(t *testing.T, err error, status int, code string) {
	t.Helper()
	var appErr *apperror.AppError
	require.True(t, errors.As(err, &appErr), "want AppError, got %v", err)
	require.Equal(t, status, appErr.Status, "%s: %s", appErr.Code, appErr.Message)
	if code != "" {
		require.Equal(t, code, appErr.Code)
	}
}

func TestApplyCopiesLessonTitlesAndKeepsPlans(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	version := f.publishedVersion(t, f.owner, "TOAN-9", "Bài 1", "Bài 2", "Bài 3")

	// The class already keeps its own hand-written curriculum and a lesson
	// plan on the first entry.
	_, err := f.teaching.PutCurriculum(ctx, f.teacher, f.class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Cũ 1", "Cũ 2"}, CurrentIndex: 1})
	require.NoError(t, err)
	_, err = f.teaching.SavePlan(ctx, f.teacher, f.class.ID, 0, teaching.SavePlanRequest{Goal: "Ôn tập"})
	require.NoError(t, err)

	// Different lesson list → the owner has to confirm the overwrite.
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	requireStatus(t, err, http.StatusConflict, classprogram.CodeCurriculumDiffers)
	var appErr *apperror.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, map[string]string{"current_count": "2", "template_count": "3"}, appErr.Fields)

	got, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID, Confirm: true})
	require.NoError(t, err)
	require.Equal(t, version.ID, got.TemplateVersionID)
	require.Equal(t, version.TemplateID, got.TemplateID)
	require.Equal(t, "Chương trình TOAN-9", got.TemplateName)
	require.Equal(t, 1, got.VersionNo)
	require.Equal(t, 3, got.LessonCount)
	require.Equal(t, f.owner.TeacherID, got.AppliedBy)

	cur, err := f.teaching.GetCurriculum(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Bài 1", "Bài 2", "Bài 3"}, cur.Lessons)
	require.Equal(t, 1, cur.CurrentIndex, "the pointer survives the copy")

	plans, err := f.teaching.ListPlans(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Equal(t, "Ôn tập", plans[0].Goal)

	// Re-applying an identical lesson list needs no confirmation.
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.NoError(t, err)

	// Switching to another version with different titles asks again, and
	// the confirmed switch replaces the single class_programs row.
	other := f.publishedVersion(t, f.owner, "TOAN-9B", "Bài A")
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: other.ID})
	requireStatus(t, err, http.StatusConflict, classprogram.CodeCurriculumDiffers)
	got, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: other.ID, Confirm: true})
	require.NoError(t, err)
	require.Equal(t, other.ID, got.TemplateVersionID)
	var rows int64
	require.NoError(t, f.db.Table("class_programs").Where("class_id = ?", f.class.ID).Count(&rows).Error)
	require.EqualValues(t, 1, rows)

	lessons, err := f.svc.Lessons(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Len(t, lessons, 1)
	require.Equal(t, "Bài A", lessons[0].Title)

	// Removing drops only the link: curriculum and plans stay.
	require.NoError(t, f.svc.Remove(ctx, f.owner, f.class.ID))
	program, err := f.svc.Get(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Nil(t, program)
	lessons, err = f.svc.Lessons(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Empty(t, lessons)
	cur, err = f.teaching.GetCurriculum(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Bài A"}, cur.Lessons)
	plans, err = f.teaching.ListPlans(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Len(t, plans, 1)

	err = f.svc.Remove(ctx, f.owner, f.class.ID)
	requireStatus(t, err, http.StatusNotFound, "")
}

func TestApplyAcceptsOnlyPublishedVersionsOfTheCenter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	tpl, err := f.library.CreateTemplate(ctx, f.owner, library.TemplateRequest{Code: "NHAP", Name: "Nháp"})
	require.NoError(t, err)
	draft := f.draft(t, f.owner, tpl.ID)
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: draft.ID})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionNotPublished)

	theirs := f.publishedVersion(t, f.outsider, "TOAN-9", "Bài của họ")
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: theirs.ID})
	requireStatus(t, err, http.StatusNotFound, "")

	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: uuid.New()})
	requireStatus(t, err, http.StatusNotFound, "")

	// An empty curriculum never asks for confirmation.
	version := f.publishedVersion(t, f.owner, "TOAN-9", "Bài 1")
	got, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.NoError(t, err)
	require.Equal(t, 1, got.LessonCount)
	cur, err := f.teaching.GetCurriculum(ctx, f.owner, f.class.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Bài 1"}, cur.Lessons)
	require.Equal(t, 0, cur.CurrentIndex)
}

func TestOnlyTheOwnerAppliesOrRemovesWhileStaffRead(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	version := f.publishedVersion(t, f.owner, "TOAN-9", "Bài 1", "Bài 2")

	_, assistantT := testutil.Teacher(t, f.db)
	testutil.JoinCenter(t, f.db, assistantT.ID, f.owner.CenterID)
	testutil.StaffAssignment(t, f.db, f.class, assistantT.ID, "tro_giang")
	assistant := testutil.ScopeFor(t, f.db, assistantT.ID)

	for _, sc := range []authctx.Scope{f.teacher, assistant} {
		_, err := f.svc.Apply(ctx, sc, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID, Confirm: true})
		requireStatus(t, err, http.StatusForbidden, "")
		err = f.svc.Remove(ctx, sc, f.class.ID)
		requireStatus(t, err, http.StatusForbidden, "")
	}

	_, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.NoError(t, err)

	for _, sc := range []authctx.Scope{f.teacher, assistant, f.owner} {
		program, err := f.svc.Get(ctx, sc, f.class.ID)
		require.NoError(t, err)
		require.NotNil(t, program)
		require.Equal(t, version.ID, program.TemplateVersionID)
		lessons, err := f.svc.Lessons(ctx, sc, f.class.ID)
		require.NoError(t, err)
		require.Len(t, lessons, 2)
		require.Equal(t, "Bài 1", lessons[0].Title)
		require.Equal(t, 1, lessons[0].Position)
	}

	// Another center never sees the class, let alone its program.
	_, err = f.svc.Get(ctx, f.outsider, f.class.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.Lessons(ctx, f.outsider, f.class.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.Apply(ctx, f.outsider, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	requireStatus(t, err, http.StatusNotFound, "")

	// The template cannot be deleted while a class applies it.
	err = f.library.DeleteTemplate(ctx, f.owner, version.TemplateID)
	requireStatus(t, err, http.StatusConflict, library.CodeTemplateInUse)
	require.NoError(t, f.svc.Remove(ctx, f.owner, f.class.ID))
	require.NoError(t, f.library.DeleteTemplate(ctx, f.owner, version.TemplateID))
}

func TestArchivedVersionStaysReadableButCannotBeApplied(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	version := f.publishedVersion(t, f.owner, "TOAN-6", "Bài 1", "Bài 2")
	_, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.NoError(t, err)

	// Retiring the version in the library must not break the classes that
	// already follow it: a released version is immutable, so its lessons stay
	// readable through the class.
	_, err = f.library.Archive(ctx, f.owner, version.ID)
	require.NoError(t, err)

	got, err := f.svc.Get(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Equal(t, library.StatusArchived, got.VersionStatus)
	lessons, err := f.svc.Lessons(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Len(t, lessons, 2)
	require.Equal(t, "Bài 2", lessons[1].Title)

	// Only a published version can be applied, so re-applying the archived
	// one (or applying it to another class) is still refused.
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID, Confirm: true})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionNotPublished)
}

// awaits reports whether done closes within d.
func awaits(done <-chan struct{}, d time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// TestApplyRereadsCurriculumUnderLockAfterAConcurrentEdit proves the
// CURRICULUM_DIFFERS decision uses the curriculum row as it stands right
// before Apply writes, not a snapshot taken before its transaction opened: a
// curriculum edit in flight when Apply starts must be waited for, and its
// result must feed the comparison.
func TestApplyRereadsCurriculumUnderLockAfterAConcurrentEdit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	version := f.publishedVersion(t, f.owner, "SU-6", "Bài 1", "Bài 2")

	// The class already keeps exactly the template's own lessons, so a read
	// taken before the concurrent edit below lands would see no difference
	// and apply without asking for confirmation.
	_, err := f.teaching.PutCurriculum(ctx, f.teacher, f.class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1", "Bài 2"}})
	require.NoError(t, err)

	tx := f.db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Exec(`SELECT 1 FROM class_curricula WHERE class_id = ? FOR UPDATE`, f.class.ID).Error)

	// applyErr is only read after done closes, so the goroutine's write to it
	// happens-before this test's read: no data race, and no second receive on
	// a single-value channel that would block forever.
	var applyErr error
	done := make(chan struct{})
	go func() {
		_, applyErr = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
		close(done)
	}()
	require.False(t, awaits(done, 300*time.Millisecond), "apply must wait for the concurrent curriculum edit to commit")

	// The concurrent edit swaps in a lesson list the template no longer
	// matches, then commits and releases the row.
	require.NoError(t, tx.Exec(`UPDATE class_curricula SET lessons = '["Bài khác"]' WHERE class_id = ?`, f.class.ID).Error)
	require.NoError(t, tx.Commit().Error)

	require.True(t, awaits(done, 5*time.Second), "apply must proceed once the curriculum edit commits")
	requireStatus(t, applyErr, http.StatusConflict, classprogram.CodeCurriculumDiffers)
}

// TestApplySerialisesWithTemplateDelete proves applying a version and
// deleting its template lock the template row in opposite modes, so whichever
// commits first decides the outcome: the apply sees the delete and gets a
// 404, or the delete sees the class program and gets 409. No class may end up
// applying a deleted template.
func TestApplySerialisesWithTemplateDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	for round := 0; round < 8; round++ {
		version := f.publishedVersion(t, f.owner, "RACE-"+uuid.NewString()[:8], "Bài 1")
		class := testutil.Class(t, f.db, f.owner.TeacherID)

		var wg sync.WaitGroup
		var applyErr, deleteErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, applyErr = f.svc.Apply(ctx, f.owner, class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
		}()
		go func() {
			defer wg.Done()
			deleteErr = f.library.DeleteTemplate(ctx, f.owner, version.TemplateID)
		}()
		wg.Wait()

		switch {
		case applyErr == nil && deleteErr == nil:
			t.Fatalf("round %d: both the apply and the delete went through", round)
		case applyErr == nil:
			requireStatus(t, deleteErr, http.StatusConflict, library.CodeTemplateInUse)
		case deleteErr == nil:
			requireStatus(t, applyErr, http.StatusNotFound, "")
		default:
			t.Fatalf("round %d: both failed: apply=%v delete=%v", round, applyErr, deleteErr)
		}
	}

	var orphans int64
	require.NoError(t, f.db.Raw(`
		SELECT count(*) FROM class_programs p
		JOIN program_template_versions v ON v.id = p.template_version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.deleted_at IS NOT NULL`).Scan(&orphans).Error)
	require.Zero(t, orphans, "no class program may keep applying a deleted template")
}

func TestApplyRefusesMoreLessonsThanTheCurriculumHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	titles := make([]string, teaching.MaxCurriculumLessons+1)
	for i := range titles {
		titles[i] = "Bài " + strconv.Itoa(i+1)
	}
	version := f.publishedVersion(t, f.owner, "DAI", titles...)
	_, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	requireStatus(t, err, http.StatusUnprocessableEntity, classprogram.CodeTooManyLessons)
	got, err := f.svc.Get(ctx, f.owner, f.class.ID)
	require.NoError(t, err)
	require.Nil(t, got)
}

// scoreGroup is a score set group input whose component keys follow the
// labels' order.
func scoreGroup(key, title string, labels ...string) library.ScoreSetGroupInput {
	components := make([]library.ScoreComponentInput, len(labels))
	for i, label := range labels {
		components[i] = library.ScoreComponentInput{Key: "c" + strconv.Itoa(i+1), Label: label, Max: 10, Weight: 1}
	}
	return library.ScoreSetGroupInput{Key: key, Title: title, Components: components}
}

func (f fixture) componentNames(t *testing.T, classID uuid.UUID) []string {
	t.Helper()
	got, err := f.grading.GetClassComponents(context.Background(), f.teacher, classID)
	require.NoError(t, err)
	out := make([]string, len(got.Components))
	for i, comp := range got.Components {
		out[i] = comp.Name
	}
	return out
}

func TestApplyCopiesTemplateScoreSetIntoClassComponents(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	version := f.scoredVersion(t, f.owner, "IELTS-5", []library.ScoreSetGroupInput{
		scoreGroup("giua_ky", "Giữa kỳ", "Nghe", "Nói"),
		scoreGroup("cuoi_ky", "Cuối kỳ", "Viết"),
	}, "Bài 1")

	_, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Giữa kỳ · Nghe", "Giữa kỳ · Nói", "Cuối kỳ · Viết"}, f.componentNames(t, f.class.ID))

	// Another class applying a single-group version gets the bare labels.
	single := f.scoredVersion(t, f.owner, "IELTS-6", []library.ScoreSetGroupInput{
		scoreGroup("tong_ket", "Tổng kết", "Nghe", "Đọc"),
	}, "Bài 1")
	other := testutil.Class(t, f.db, f.teacher.TeacherID)
	_, err = f.svc.Apply(ctx, f.owner, other.ID, classprogram.ApplyRequest{TemplateVersionID: single.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Nghe", "Đọc"}, f.componentNames(t, other.ID))
}

func TestReapplyKeepsComponentsOnceTheClassHasScores(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := f.scoredVersion(t, f.owner, "TOEIC-1", []library.ScoreSetGroupInput{
		scoreGroup("chinh", "Chính", "Nghe", "Đọc"),
	}, "Bài 1")
	second := f.scoredVersion(t, f.owner, "TOEIC-2", []library.ScoreSetGroupInput{
		scoreGroup("chinh", "Chính", "Nói", "Viết"),
	}, "Bài 1")

	_, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: first.ID})
	require.NoError(t, err)
	// Before any score the class follows whichever version is applied.
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: second.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Nói", "Viết"}, f.componentNames(t, f.class.ID))

	contact := testutil.Contact(t, f.db, f.teacher.TeacherID)
	student := testutil.Student(t, f.db, f.teacher.TeacherID, contact.ID)
	testutil.Enrollment(t, f.db, f.teacher.TeacherID, student.ID, f.class.ID, date("2026-01-01"))
	session := testutil.Session(t, f.db, f.teacher.TeacherID, f.class.ID, date("2026-01-06"))
	components, err := f.grading.GetClassComponents(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	score := 8.0
	_, err = f.grading.PutSessionScores(ctx, f.teacher, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: components.Components[0].ID, Score: &score},
	})
	require.NoError(t, err)

	got, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: first.ID})
	require.NoError(t, err, "a scored class still switches its program")
	require.Equal(t, first.ID, got.TemplateVersionID)
	after, err := f.grading.GetClassComponents(ctx, f.teacher, f.class.ID)
	require.NoError(t, err)
	require.Equal(t, components.Components, after.Components, "a scored class keeps its components")
	grid, err := f.grading.GetSessionScores(ctx, f.teacher, session.ID)
	require.NoError(t, err)
	require.Len(t, grid.Scores, 1, "the recorded grade survives the re-apply")
}

func TestRemoveKeepsClassComponents(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	version := f.scoredVersion(t, f.owner, "VAN-9", []library.ScoreSetGroupInput{
		scoreGroup("chinh", "Chính", "Miệng", "15 phút"),
	}, "Bài 1")

	_, err := f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.NoError(t, err)
	require.NoError(t, f.svc.Remove(ctx, f.owner, f.class.ID))
	require.Equal(t, []string{"Miệng", "15 phút"}, f.componentNames(t, f.class.ID))

	// Applying a version without a score set keeps them as well.
	plain := f.publishedVersion(t, f.owner, "VAN-9B", "Bài 1")
	_, err = f.svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: plain.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Miệng", "15 phút"}, f.componentNames(t, f.class.ID))
}

// failingScoreStore writes the snapshot through the real grading service and
// then fails, as a late error inside Apply's transaction would.
type failingScoreStore struct {
	inner *grading.Service
}

var errSnapshotFailed = errors.New("snapshot write failed")

func (s failingScoreStore) SyncTemplateComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID, groups []grading.TemplateScoreGroup) (grading.SnapshotOutcome, error) {
	if _, err := s.inner.SyncTemplateComponents(ctx, sc, classID, groups); err != nil {
		return "", err
	}
	return "", errSnapshotFailed
}

func TestApplyRollsBackWhenTheScoreCopyFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	version := f.scoredVersion(t, f.owner, "LY-8", []library.ScoreSetGroupInput{
		scoreGroup("chinh", "Chính", "Thực hành"),
	}, "Bài 1", "Bài 2")
	svc := classprogram.NewService(classprogram.NewRepository(f.db), f.classes, f.teaching, f.library,
		failingScoreStore{inner: f.grading}, database.NewTxManager(f.db))

	_, err := svc.Apply(ctx, f.owner, f.class.ID, classprogram.ApplyRequest{TemplateVersionID: version.ID})
	require.ErrorIs(t, err, errSnapshotFailed)

	program, err := f.svc.Get(ctx, f.owner, f.class.ID)
	require.NoError(t, err)
	require.Nil(t, program, "the class program link rolls back")
	cur, err := f.teaching.GetCurriculum(ctx, f.owner, f.class.ID)
	require.NoError(t, err)
	require.Empty(t, cur.Lessons, "the curriculum copy rolls back")
	require.Empty(t, f.componentNames(t, f.class.ID), "the score components roll back")
}

func date(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}
