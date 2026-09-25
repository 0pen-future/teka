//go:build integration

package paths_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/courses"
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
	svc      *paths.Service
	courses  *courses.Service
	owner    authctx.Scope
	member   uuid.UUID
	outsider authctx.Scope
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	tx := database.NewTxManager(db)

	_, ownerT := testutil.Teacher(t, db)
	_, memberT := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, memberT.ID, ownerT.CenterID)
	_, outsiderT := testutil.Teacher(t, db)

	return fixture{
		db:       db,
		svc:      paths.NewService(paths.NewRepository(db), tx),
		courses:  courses.NewService(courses.NewRepository(db), tx),
		owner:    testutil.ScopeFor(t, db, ownerT.ID),
		member:   memberT.ID,
		outsider: testutil.ScopeFor(t, db, outsiderT.ID),
	}
}

// grant sets member overrides on the member's live membership so the scope
// reflects exactly the keys a test hands out.
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

func (f fixture) path(t *testing.T, sc authctx.Scope, code string) *paths.PathResponse {
	t.Helper()
	out, err := f.svc.Create(context.Background(), sc, paths.PathRequest{Code: code, Name: "Lộ trình " + code})
	require.NoError(t, err)
	return out
}

func (f fixture) stage(t *testing.T, sc authctx.Scope, pathID uuid.UUID, name string) *paths.PathResponse {
	t.Helper()
	out, err := f.svc.CreateStage(context.Background(), sc, pathID, paths.StageRequest{Name: name})
	require.NoError(t, err)
	return out
}

func (f fixture) course(t *testing.T, sc authctx.Scope, code string) uuid.UUID {
	t.Helper()
	out, err := f.courses.Create(context.Background(), sc, courses.CourseRequest{Code: code, Name: "Khóa " + code, DefaultUnitPrice: 150000})
	require.NoError(t, err)
	return out.ID
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

func TestPathCodeUniqueAmongLiveRowsPerCenter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := f.path(t, f.owner, "LT-6")

	_, err := f.svc.Create(ctx, f.owner, paths.PathRequest{Code: "lt-6", Name: "Trùng"})
	requireStatus(t, err, http.StatusConflict, paths.CodeCodeTaken)

	// Another center may use the same code.
	_, err = f.svc.Create(ctx, f.outsider, paths.PathRequest{Code: "LT-6", Name: "Của họ"})
	require.NoError(t, err)
	// ... and never sees ours.
	_, err = f.svc.Get(ctx, f.outsider, p.ID)
	requireStatus(t, err, http.StatusNotFound, "")

	require.NoError(t, f.svc.Delete(ctx, f.owner, p.ID))
	_, err = f.svc.Create(ctx, f.owner, paths.PathRequest{Code: "LT-6", Name: "Lại"})
	require.NoError(t, err)

	rows, total, err := f.svc.List(ctx, f.owner, paths.ListFilter{}, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.NotEqual(t, p.ID, rows[0].ID)
}

func TestReorderSwapsPositionsUnderTheDeferredUnique(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := f.path(t, f.owner, "LT-6")
	f.stage(t, f.owner, p.ID, "Một")
	f.stage(t, f.owner, p.ID, "Hai")
	p = f.stage(t, f.owner, p.ID, "Ba")
	require.Len(t, p.Stages, 3)
	a, b, c := p.Stages[0].ID, p.Stages[1].ID, p.Stages[2].ID

	// Swapping the first and last would collide on (path_id, position)
	// mid-way without the deferred unique and the surrounding transaction.
	out, err := f.svc.ReorderStages(ctx, f.owner, p.ID, paths.ReorderRequest{StageIDs: []uuid.UUID{c, b, a}})
	require.NoError(t, err)
	require.Equal(t, []string{"Ba", "Hai", "Một"}, []string{out.Stages[0].Name, out.Stages[1].Name, out.Stages[2].Name})
	for i, st := range out.Stages {
		require.Equal(t, i+1, st.Position)
	}

	_, err = f.svc.ReorderStages(ctx, f.owner, p.ID, paths.ReorderRequest{StageIDs: []uuid.UUID{c, b}})
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)

	// Deleting the middle stage closes the gap.
	out, err = f.svc.DeleteStage(ctx, f.owner, p.ID, b)
	require.NoError(t, err)
	require.Len(t, out.Stages, 2)
	require.Equal(t, 1, out.Stages[0].Position)
	require.Equal(t, 2, out.Stages[1].Position)
	require.Equal(t, "Một", out.Stages[1].Name)
}

func TestStageCoursesStayWithinTheCenter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := f.path(t, f.owner, "LT-6")
	p = f.stage(t, f.owner, p.ID, "Một")
	sid := p.Stages[0].ID
	toan := f.course(t, f.owner, "TOAN-6")
	van := f.course(t, f.owner, "VAN-6")
	theirs := f.course(t, f.outsider, "TOAN-6")

	_, err := f.svc.SetStageCourses(ctx, f.owner, p.ID, sid, paths.StageCoursesRequest{CourseIDs: []uuid.UUID{toan, theirs}})
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)

	out, err := f.svc.SetStageCourses(ctx, f.owner, p.ID, sid, paths.StageCoursesRequest{CourseIDs: []uuid.UUID{van, toan}})
	require.NoError(t, err)
	require.Len(t, out.Stages[0].Courses, 2)
	require.Equal(t, "VAN-6", out.Stages[0].Courses[0].Code)
	require.Equal(t, 1, out.Stages[0].Courses[0].Position)
	require.Equal(t, "TOAN-6", out.Stages[0].Courses[1].Code)
	require.Equal(t, 2, out.Stages[0].Courses[1].Position)
	require.Equal(t, 2, out.CourseCount)

	// Replacing keeps only the new list; [] clears it.
	out, err = f.svc.SetStageCourses(ctx, f.owner, p.ID, sid, paths.StageCoursesRequest{CourseIDs: []uuid.UUID{toan}})
	require.NoError(t, err)
	require.Len(t, out.Stages[0].Courses, 1)
	require.Equal(t, toan, out.Stages[0].Courses[0].ID)
	out, err = f.svc.SetStageCourses(ctx, f.owner, p.ID, sid, paths.StageCoursesRequest{CourseIDs: []uuid.UUID{}})
	require.NoError(t, err)
	require.Empty(t, out.Stages[0].Courses)
	require.Equal(t, 0, out.CourseCount)
}

func TestMemberWithReadOnlyCannotWrite(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p := f.path(t, f.owner, "LT-6")
	p = f.stage(t, f.owner, p.ID, "Một")
	reader := f.grant(t, f.member, authctx.PermPathsRead)

	got, err := f.svc.Get(ctx, reader, p.ID)
	require.NoError(t, err)
	require.Len(t, got.Stages, 1)

	_, err = f.svc.Create(ctx, reader, paths.PathRequest{Code: "LT-7", Name: "x"})
	requireStatus(t, err, http.StatusForbidden, "")
	_, err = f.svc.CreateStage(ctx, reader, p.ID, paths.StageRequest{Name: "Hai"})
	requireStatus(t, err, http.StatusForbidden, "")
	_, err = f.svc.SetStageCourses(ctx, reader, p.ID, p.Stages[0].ID, paths.StageCoursesRequest{CourseIDs: []uuid.UUID{}})
	requireStatus(t, err, http.StatusForbidden, "")

	editor := f.grant(t, f.member, authctx.PermPathsEdit)
	out, err := f.svc.CreateStage(ctx, editor, p.ID, paths.StageRequest{Name: "Hai"})
	require.NoError(t, err)
	require.Len(t, out.Stages, 2)
}
