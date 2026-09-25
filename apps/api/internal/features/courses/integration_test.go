//go:build integration

package courses_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/courses"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/features/paths"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// fixture is one center with its owner and one role-less member, plus a
// second center's owner as the outsider.
type fixture struct {
	db       *gorm.DB
	svc      *courses.Service
	lib      *library.Service
	owner    authctx.Scope
	member   uuid.UUID
	outsider authctx.Scope
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	tx := database.NewTxManager(db)
	svc := courses.NewService(courses.NewRepository(db), tx)
	lib := library.NewService(library.NewRepository(db), tx)

	_, ownerT := testutil.Teacher(t, db)
	_, memberT := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, memberT.ID, ownerT.CenterID)
	_, outsiderT := testutil.Teacher(t, db)

	return fixture{
		db: db, svc: svc, lib: lib,
		owner:    testutil.ScopeFor(t, db, ownerT.ID),
		member:   memberT.ID,
		outsider: testutil.ScopeFor(t, db, outsiderT.ID),
	}
}

// grant sets one member override on the member's live membership so the
// scope reflects exactly the keys a test hands out.
func (f fixture) grant(t *testing.T, teacherID uuid.UUID, keys ...string) authctx.Scope {
	t.Helper()
	for _, key := range keys {
		err := f.db.Exec(`
			INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
			VALUES (?, ?, ?, TRUE)
			ON CONFLICT (teacher_id, center_id, permission_key) DO UPDATE SET allowed = TRUE`,
			teacherID, f.owner.CenterID, key).Error
		require.NoError(t, err)
	}
	return testutil.ScopeFor(t, f.db, teacherID)
}

func (f fixture) course(t *testing.T, sc authctx.Scope, code string) *courses.CourseResponse {
	t.Helper()
	out, err := f.svc.Create(context.Background(), sc, courses.CourseRequest{Code: code, Name: "Khóa " + code, DefaultUnitPrice: 150000})
	require.NoError(t, err)
	return out
}

// versions creates a template in sc's center and returns its published v1
// and the new draft v2 that publishing opens.
func (f fixture) versions(t *testing.T, sc authctx.Scope, code string) (published, draft uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tpl, err := f.lib.CreateTemplate(ctx, sc, library.TemplateRequest{Code: code, Name: "Chương trình " + code})
	require.NoError(t, err)
	list, err := f.lib.ListVersions(ctx, sc, tpl.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	pub, err := f.lib.Publish(ctx, sc, list[0].ID)
	require.NoError(t, err)
	_, err = f.lib.CreateVersion(ctx, sc, tpl.ID, library.CreateVersionRequest{})
	require.NoError(t, err)
	list, err = f.lib.ListVersions(ctx, sc, tpl.ID)
	require.NoError(t, err)
	for _, v := range list {
		if v.Status == library.StatusDraft {
			draft = v.ID
		}
	}
	require.NotEqual(t, uuid.Nil, draft)
	return pub.ID, draft
}

// attach points a fixture class at a course the way the classes service
// will, bypassing it so this package's tests stay independent of it.
func (f fixture) attach(t *testing.T, classID, courseID uuid.UUID) {
	t.Helper()
	require.NoError(t, f.db.Exec(`UPDATE classes SET course_id = ? WHERE id = ?`, courseID, classID).Error)
}

func requireStatus(t *testing.T, err error, status int, code string) {
	t.Helper()
	var appErr *apperror.AppError
	require.True(t, errors.As(err, &appErr), "want AppError %d, got %v", status, err)
	require.Equal(t, status, appErr.Status, "%s: %s", appErr.Code, appErr.Message)
	if code != "" {
		require.Equal(t, code, appErr.Code)
	}
}

func TestCourseCodeUniqueAmongLiveRowsPerCenter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.course(t, f.owner, "TOAN-6")

	_, err := f.svc.Create(ctx, f.owner, courses.CourseRequest{Code: "toan-6", Name: "Trùng"})
	requireStatus(t, err, http.StatusConflict, courses.CodeCodeTaken)

	// Another center may use the same code.
	_, err = f.svc.Create(ctx, f.outsider, courses.CourseRequest{Code: "TOAN-6", Name: "Của trung tâm khác"})
	require.NoError(t, err)

	// Update onto a taken code clashes too, and onto itself does not.
	b := f.course(t, f.owner, "VAN-6")
	_, err = f.svc.Update(ctx, f.owner, b.ID, courses.CourseRequest{Code: "TOAN-6", Name: "Trùng"})
	requireStatus(t, err, http.StatusConflict, courses.CodeCodeTaken)
	out, err := f.svc.Update(ctx, f.owner, b.ID, courses.CourseRequest{Code: "VAN-6", Name: "Văn 6 mới", Status: courses.StatusActive})
	require.NoError(t, err)
	require.Equal(t, "Văn 6 mới", out.Name)
	require.Equal(t, courses.StatusActive, out.Status)

	// A soft-deleted course frees its code.
	require.NoError(t, f.svc.Delete(ctx, f.owner, a.ID))
	_, err = f.svc.Get(ctx, f.owner, a.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.Create(ctx, f.owner, courses.CourseRequest{Code: "TOAN-6", Name: "Lại"})
	require.NoError(t, err)

	rows, total, err := f.svc.List(ctx, f.owner, courses.ListFilter{Q: "toan"}, pagination.Params{PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "TOAN-6", rows[0].Code)
}

func TestDefaultTemplateVersionMustBePublishedInCenter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	published, draft := f.versions(t, f.owner, "TOAN-6")
	foreign, _ := f.versions(t, f.outsider, "TOAN-6")

	for name, id := range map[string]uuid.UUID{"draft": draft, "foreign": foreign, "missing": uuid.New()} {
		_, err := f.svc.Create(ctx, f.owner, courses.CourseRequest{Code: "T-" + name, Name: name, DefaultTemplateVersionID: &id})
		requireStatus(t, err, http.StatusUnprocessableEntity, "")
	}
	out, err := f.svc.Create(ctx, f.owner, courses.CourseRequest{Code: "TOAN-6", Name: "Toán 6", DefaultTemplateVersionID: &published})
	require.NoError(t, err)
	require.NotNil(t, out.DefaultTemplate)
	require.Equal(t, published, out.DefaultTemplate.VersionID)
	require.Equal(t, "TOAN-6", out.DefaultTemplate.Code)
	require.Equal(t, 1, out.DefaultTemplate.VersionNo)

	// Clearing the version on update writes NULL, not the previous value.
	out, err = f.svc.Update(ctx, f.owner, out.ID, courses.CourseRequest{Code: "TOAN-6", Name: "Toán 6"})
	require.NoError(t, err)
	require.Nil(t, out.DefaultTemplateVersionID)
	require.Nil(t, out.DefaultTemplate)
}

// A version that was published when chosen can be archived, or its template
// deleted, afterwards. The course keeps working: resending the stored id is
// not a new choice, the embed reports the version's real status, and a
// deleted template drops the embed while the id stays on the row.
func TestRetiredDefaultTemplateDoesNotBlockEdits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v1, _ := f.versions(t, f.owner, "TOAN-6")
	c, err := f.svc.Create(ctx, f.owner, courses.CourseRequest{Code: "TOAN-6", Name: "Toán 6", Status: courses.StatusActive, DefaultTemplateVersionID: &v1})
	require.NoError(t, err)

	_, err = f.lib.Archive(ctx, f.owner, v1)
	require.NoError(t, err)

	out, err := f.svc.Update(ctx, f.owner, c.ID, courses.CourseRequest{Code: "TOAN-6", Name: "Toán 6 nâng cao", DefaultTemplateVersionID: &v1})
	require.NoError(t, err, "an unrelated edit must not trip over the archived default")
	require.Equal(t, courses.StatusActive, out.Status, "a blank status keeps the stored one")
	require.NotNil(t, out.DefaultTemplate)
	require.Equal(t, library.StatusArchived, out.DefaultTemplate.Status)

	_, err = f.svc.Create(ctx, f.owner, courses.CourseRequest{Code: "TOAN-6B", Name: "Toán 6B", DefaultTemplateVersionID: &v1})
	requireStatus(t, err, http.StatusUnprocessableEntity, "")

	require.NoError(t, f.lib.DeleteTemplate(ctx, f.owner, out.DefaultTemplate.TemplateID))
	got, err := f.svc.Get(ctx, f.owner, c.ID)
	require.NoError(t, err)
	require.Nil(t, got.DefaultTemplate, "a deleted template is not shown")
	require.NotNil(t, got.DefaultTemplateVersionID, "but the stored id is not rewritten behind the user's back")
	_, err = f.svc.Update(ctx, f.owner, c.ID, courses.CourseRequest{Code: "TOAN-6", Name: "Toán 6", DefaultTemplateVersionID: &v1})
	require.NoError(t, err)
}

// Deleting a course and attaching a class to it lock the course row in
// opposite modes, so whichever commits first decides: the attach sees the
// deletion and gets 422, or the delete sees the class and gets 409. No
// live class may end up pointing at a deleted course.
func TestDeleteSerialisesWithClassAttach(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cls := classes.NewService(classes.NewRepository(f.db), database.NewTxManager(f.db), classstaff.NewRepository(f.db))

	for round := 0; round < 8; round++ {
		c := f.course(t, f.owner, "R-"+uuid.NewString()[:6])
		req := classes.CreateClassRequest{
			Name: "Lớp", StartDate: "2026-01-05", CourseID: strPtr(c.ID.String()),
			Schedules: []classes.ScheduleRequest{{Weekday: int16Ptr(1), StartTime: "18:00", DurationMin: 90}},
		}
		var wg sync.WaitGroup
		var createErr, deleteErr error
		wg.Add(2)
		go func() { defer wg.Done(); _, createErr = cls.Create(ctx, f.owner, req) }()
		go func() { defer wg.Done(); deleteErr = f.svc.Delete(ctx, f.owner, c.ID) }()
		wg.Wait()

		switch {
		case createErr == nil && deleteErr == nil:
			t.Fatalf("round %d: both the attach and the delete went through", round)
		case createErr == nil:
			requireStatus(t, deleteErr, http.StatusConflict, courses.CodeCourseInUse)
		case deleteErr == nil:
			requireStatus(t, createErr, http.StatusUnprocessableEntity, "")
		default:
			t.Fatalf("round %d: both failed: create=%v delete=%v", round, createErr, deleteErr)
		}
	}
	var dangling int64
	require.NoError(t, f.db.Raw(`
		SELECT count(*) FROM classes cl JOIN courses co ON co.id = cl.course_id
		WHERE cl.deleted_at IS NULL AND co.deleted_at IS NOT NULL`).Scan(&dangling).Error)
	require.Zero(t, dangling)
}

func strPtr(s string) *string { return &s }
func int16Ptr(v int16) *int16 { return &v }

func TestArchiveKeepsClassesAndDeleteIsGuarded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.course(t, f.owner, "TOAN-6")

	today := time.Now().UTC()
	running := testutil.Class(t, f.db, f.owner.TeacherID, testutil.WithClassStartDate(today.AddDate(0, 0, -7)))
	upcoming := testutil.Class(t, f.db, f.owner.TeacherID, testutil.WithClassStartDate(today.AddDate(0, 0, 7)))
	ended := testutil.Class(t, f.db, f.owner.TeacherID,
		testutil.WithClassStartDate(today.AddDate(0, -2, 0)), testutil.WithClassEndDate(today.AddDate(0, 0, -1)))
	for _, cl := range []uuid.UUID{running.ID, upcoming.ID, ended.ID} {
		f.attach(t, cl, c.ID)
	}

	got, err := f.svc.Get(ctx, f.owner, c.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got.ClassesRunning)
	require.Equal(t, 1, got.ClassesUpcoming)

	archived, err := f.svc.Archive(ctx, f.owner, c.ID)
	require.NoError(t, err)
	require.Equal(t, courses.StatusArchived, archived.Status)
	require.Equal(t, 1, archived.ClassesRunning, "archiving must not detach classes")
	var attached int64
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM classes WHERE course_id = ?`, c.ID).Scan(&attached).Error)
	require.EqualValues(t, 3, attached)
	_, err = f.svc.Archive(ctx, f.owner, c.ID)
	requireStatus(t, err, http.StatusConflict, courses.CodeCourseArchived)

	err = f.svc.Delete(ctx, f.owner, c.ID)
	requireStatus(t, err, http.StatusConflict, courses.CodeCourseInUse)

	// Detaching every live class lifts the guard; the soft delete then
	// leaves the rows alone (no cascade in business logic).
	require.NoError(t, f.db.Exec(`UPDATE classes SET course_id = NULL WHERE course_id = ?`, c.ID).Error)
	require.NoError(t, f.svc.Delete(ctx, f.owner, c.ID))
	var classCount int64
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM classes WHERE deleted_at IS NULL`).Scan(&classCount).Error)
	require.EqualValues(t, 3, classCount)
}

func TestTuitionPacksReplaceWholesaleInOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.course(t, f.owner, "TOAN-6")

	packs, err := f.svc.SetTuitionPacks(ctx, f.owner, c.ID, []courses.TuitionPackInput{
		{Name: "Gói 12 buổi", Sessions: 12, Price: 1_200_000},
		{Name: "Gói 24 buổi", Sessions: 24, Price: 2_200_000},
		{Name: "Gói 36 buổi", Sessions: 36, Price: 3_000_000},
	})
	require.NoError(t, err)
	require.Len(t, packs, 3)
	for i, p := range packs {
		require.Equal(t, i+1, p.Position)
	}

	// Reversing the order reuses the same positions inside one transaction.
	packs, err = f.svc.SetTuitionPacks(ctx, f.owner, c.ID, []courses.TuitionPackInput{
		{Name: "Gói 36 buổi", Sessions: 36, Price: 3_000_000},
		{Name: "Gói 12 buổi", Sessions: 12, Price: 1_200_000},
	})
	require.NoError(t, err)
	require.Len(t, packs, 2)
	require.Equal(t, "Gói 36 buổi", packs[0].Name)
	require.Equal(t, 1, packs[0].Position)

	got, err := f.svc.Get(ctx, f.owner, c.ID)
	require.NoError(t, err)
	require.Len(t, got.TuitionPacks, 2)
	rows, _, err := f.svc.List(ctx, f.owner, courses.ListFilter{}, pagination.Params{PerPage: 20})
	require.NoError(t, err)
	require.Len(t, rows[0].TuitionPacks, 2)

	packs, err = f.svc.SetTuitionPacks(ctx, f.owner, c.ID, nil)
	require.NoError(t, err)
	require.Empty(t, packs)

	// Another center cannot touch the packs.
	_, err = f.svc.SetTuitionPacks(ctx, f.outsider, c.ID, []courses.TuitionPackInput{{Name: "x", Sessions: 1}})
	requireStatus(t, err, http.StatusNotFound, "")
}

func TestMemberNeedsCoursesEditToWrite(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.course(t, f.owner, "TOAN-6")

	// A member on a system role gets courses.read from the role's default
	// keys and nothing more, so they read but cannot write; edit is opt-in.
	require.NoError(t, f.db.Exec(`
		UPDATE center_members SET role_id = (
			SELECT id FROM center_roles WHERE center_id = ? AND key = 'giao_vien')
		WHERE teacher_id = ? AND center_id = ? AND left_at IS NULL`,
		f.owner.CenterID, f.member, f.owner.CenterID).Error)
	reader := testutil.ScopeFor(t, f.db, f.member)
	require.True(t, reader.Has(authctx.PermCoursesRead))
	require.False(t, reader.Has(authctx.PermCoursesEdit))
	_, err := f.svc.Get(ctx, reader, c.ID)
	require.NoError(t, err)
	_, err = f.svc.Create(ctx, reader, courses.CourseRequest{Code: "VAN-6", Name: "Văn 6"})
	requireStatus(t, err, http.StatusForbidden, "")
	_, err = f.svc.Update(ctx, reader, c.ID, courses.CourseRequest{Code: "TOAN-6", Name: "x"})
	requireStatus(t, err, http.StatusForbidden, "")
	_, err = f.svc.SetTuitionPacks(ctx, reader, c.ID, nil)
	requireStatus(t, err, http.StatusForbidden, "")
	requireStatus(t, f.svc.Delete(ctx, reader, c.ID), http.StatusForbidden, "")

	editor := f.grant(t, f.member, authctx.PermCoursesEdit)
	_, err = f.svc.Create(ctx, editor, courses.CourseRequest{Code: "VAN-6", Name: "Văn 6"})
	require.NoError(t, err)

	// The outsider's own center never sees this course.
	_, err = f.svc.Get(ctx, f.outsider, c.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	rows, total, err := f.svc.List(ctx, f.outsider, courses.ListFilter{}, pagination.Params{PerPage: 20})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
}

func TestDeleteIsGuardedByLearningPaths(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.course(t, f.owner, "TOAN-6")
	pathSvc := paths.NewService(paths.NewRepository(f.db), database.NewTxManager(f.db))
	p, err := pathSvc.Create(ctx, f.owner, paths.PathRequest{Code: "LT-6", Name: "Lộ trình lớp 6", Status: paths.StatusActive})
	require.NoError(t, err)
	p, err = pathSvc.CreateStage(ctx, f.owner, p.ID, paths.StageRequest{Name: "Nền tảng"})
	require.NoError(t, err)
	p, err = pathSvc.CreateStage(ctx, f.owner, p.ID, paths.StageRequest{Name: "Nâng cao"})
	require.NoError(t, err)
	for _, st := range p.Stages {
		_, err = pathSvc.SetStageCourses(ctx, f.owner, p.ID, st.ID, paths.StageCoursesRequest{CourseIDs: []uuid.UUID{c.ID}})
		require.NoError(t, err)
	}

	got, err := f.svc.ListPaths(ctx, f.owner, c.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "LT-6", got[0].Code)
	require.Equal(t, paths.StatusActive, got[0].Status)
	require.Equal(t, "Nền tảng", got[0].StageName)
	require.Equal(t, 1, got[0].StagePosition)
	require.Equal(t, 2, got[1].StagePosition)
	// The path is invisible from another center.
	_, err = f.svc.ListPaths(ctx, f.outsider, c.ID)
	requireStatus(t, err, http.StatusNotFound, "")

	err = f.svc.Delete(ctx, f.owner, c.ID)
	requireStatus(t, err, http.StatusConflict, courses.CodeCourseInPath)

	// A soft-deleted path no longer holds the course.
	require.NoError(t, pathSvc.Delete(ctx, f.owner, p.ID))
	got, err = f.svc.ListPaths(ctx, f.owner, c.ID)
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, f.svc.Delete(ctx, f.owner, c.ID))
}
