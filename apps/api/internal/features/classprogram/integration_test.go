//go:build integration

package classprogram_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classprogram"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// fixture wires the real dependency chain router.go uses: classprogram
// consumes classes (read gate), teaching (the class curriculum) and library
// (published versions) through consumer-defined interfaces.
type fixture struct {
	db       *gorm.DB
	svc      *classprogram.Service
	teaching *teaching.Service
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
	librarySvc := library.NewService(library.NewRepository(db), txMgr)
	svc := classprogram.NewService(classprogram.NewRepository(db), classesSvc, teachingSvc, librarySvc, txMgr)

	_, ownerT := testutil.Teacher(t, db)
	_, teacherT := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, teacherT.ID, ownerT.CenterID)
	_, outsiderT := testutil.Teacher(t, db)

	return fixture{
		db:       db,
		svc:      svc,
		teaching: teachingSvc,
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
	ctx := context.Background()
	tpl, err := f.library.CreateTemplate(ctx, sc, library.TemplateRequest{Code: code, Name: "Chương trình " + code})
	require.NoError(t, err)
	draft := f.draft(t, sc, tpl.ID)
	for _, title := range titles {
		_, err := f.library.CreateLesson(ctx, sc, draft.ID, library.LessonRequest{Title: title})
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
