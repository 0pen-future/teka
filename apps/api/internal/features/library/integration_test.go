//go:build integration

package library_test

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
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// fixture is one center with its owner and one role-less member, plus a
// second center's owner as the outsider.
type fixture struct {
	db       *gorm.DB
	svc      *library.Service
	owner    authctx.Scope
	member   uuid.UUID
	outsider authctx.Scope
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	svc := library.NewService(library.NewRepository(db), database.NewTxManager(db))

	_, ownerT := testutil.Teacher(t, db)
	_, memberT := testutil.Teacher(t, db, testutil.WithFullName("Thầy Minh"))
	testutil.JoinCenter(t, db, memberT.ID, ownerT.CenterID)
	_, outsiderT := testutil.Teacher(t, db)

	return fixture{
		db: db, svc: svc,
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

func (f fixture) template(t *testing.T, sc authctx.Scope, code string) *library.TemplateResponse {
	t.Helper()
	out, err := f.svc.CreateTemplate(context.Background(), sc, library.TemplateRequest{Code: code, Name: "Chương trình " + code})
	require.NoError(t, err)
	return out
}

func (f fixture) draft(t *testing.T, sc authctx.Scope, templateID uuid.UUID) library.VersionResponse {
	t.Helper()
	versions, err := f.svc.ListVersions(context.Background(), sc, templateID)
	require.NoError(t, err)
	for _, v := range versions {
		if v.Status == library.StatusDraft {
			return v
		}
	}
	t.Fatal("no draft version")
	return library.VersionResponse{}
}

func (f fixture) lesson(t *testing.T, sc authctx.Scope, versionID uuid.UUID, title string) *library.LessonResponse {
	t.Helper()
	out, err := f.svc.CreateLesson(context.Background(), sc, versionID, library.LessonRequest{Title: title})
	require.NoError(t, err)
	return out
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

func TestPublishedVersionIsLockedAndNewDraftCopiesIt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "TOAN-6")
	v1 := f.draft(t, f.owner, tpl.ID)
	l1 := f.lesson(t, f.owner, v1.ID, "Buổi 1")
	l2 := f.lesson(t, f.owner, v1.ID, "Buổi 2")
	obj := "Cộng trừ phân số"
	dur := 90
	_, err := f.svc.UpdateLesson(ctx, f.owner, l1.ID, library.LessonRequest{Title: "Buổi 1", Objectives: &obj, DurationMin: &dur})
	require.NoError(t, err)

	published, err := f.svc.Publish(ctx, f.owner, v1.ID)
	require.NoError(t, err)
	require.Equal(t, library.StatusPublished, published.Status)
	require.NotNil(t, published.PublishedAt)

	_, err = f.svc.CreateLesson(ctx, f.owner, v1.ID, library.LessonRequest{Title: "Buổi 3"})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	_, err = f.svc.UpdateLesson(ctx, f.owner, l2.ID, library.LessonRequest{Title: "Đổi"})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	requireStatus(t, f.svc.DeleteLesson(ctx, f.owner, l2.ID), http.StatusConflict, library.CodeVersionLocked)
	_, err = f.svc.Publish(ctx, f.owner, v1.ID)
	requireStatus(t, err, http.StatusConflict, library.CodeVersionNotDraft)

	v2, err := f.svc.CreateVersion(ctx, f.owner, tpl.ID, library.CreateVersionRequest{})
	require.NoError(t, err)
	require.Equal(t, 2, v2.VersionNo)
	require.Equal(t, 2, v2.LessonCount)
	copied, err := f.svc.ListLessons(ctx, f.owner, v2.ID)
	require.NoError(t, err)
	require.Len(t, copied, 2)
	require.NotEqual(t, l1.ID, copied[0].ID)
	require.Equal(t, "Buổi 1", copied[0].Title)
	require.Equal(t, &obj, copied[0].Objectives)
	require.Equal(t, &dur, copied[0].DurationMin)
	require.Equal(t, 1, copied[0].Position)
	require.Equal(t, 2, copied[1].Position)

	// A second open draft is refused by the partial unique index too.
	_, err = f.svc.CreateVersion(ctx, f.owner, tpl.ID, library.CreateVersionRequest{})
	requireStatus(t, err, http.StatusConflict, library.CodeDraftExists)

	row, err := f.svc.GetTemplate(ctx, f.owner, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, 1, *row.PublishedVersionNo)
	require.Equal(t, 2, *row.DraftVersionNo)
	require.NotNil(t, row.DraftVersionID)
	require.Equal(t, 2, row.VersionCount)
}

func TestReorderAndDeleteKeepPositionsContiguous(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "ANH-7")
	draft := f.draft(t, f.owner, tpl.ID)
	l1 := f.lesson(t, f.owner, draft.ID, "Buổi 1")
	l2 := f.lesson(t, f.owner, draft.ID, "Buổi 2")
	l3 := f.lesson(t, f.owner, draft.ID, "Buổi 3")

	// Swapping positions collides on (version_id, position) mid-statement
	// unless the constraint is deferred; the reorder must survive it.
	ordered, err := f.svc.ReorderLessons(ctx, f.owner, draft.ID, library.ReorderRequest{LessonIDs: []uuid.UUID{l3.ID, l2.ID, l1.ID}})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{l3.ID, l2.ID, l1.ID}, []uuid.UUID{ordered[0].ID, ordered[1].ID, ordered[2].ID})
	require.Equal(t, []int{1, 2, 3}, []int{ordered[0].Position, ordered[1].Position, ordered[2].Position})

	_, err = f.svc.ReorderLessons(ctx, f.owner, draft.ID, library.ReorderRequest{LessonIDs: []uuid.UUID{l3.ID, l2.ID}})
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)

	require.NoError(t, f.svc.DeleteLesson(ctx, f.owner, l2.ID))
	rest, err := f.svc.ListLessons(ctx, f.owner, draft.ID)
	require.NoError(t, err)
	require.Len(t, rest, 2)
	require.Equal(t, l3.ID, rest[0].ID)
	require.Equal(t, 1, rest[0].Position)
	require.Equal(t, l1.ID, rest[1].ID)
	require.Equal(t, 2, rest[1].Position)

	next := f.lesson(t, f.owner, draft.ID, "Buổi 4")
	require.Equal(t, 3, next.Position)
}

func TestTemplateCodeIsUniquePerLiveCenter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "LY-8")
	_, err := f.svc.CreateTemplate(ctx, f.owner, library.TemplateRequest{Code: "ly-8", Name: "Trùng"})
	requireStatus(t, err, http.StatusConflict, library.CodeCodeTaken)

	// Another center may use the same code.
	other := f.template(t, f.outsider, "LY-8")
	require.NotEqual(t, tpl.ID, other.ID)

	// LIKE metacharacters in the search are literal: "_" must not act as a
	// single-character wildcard, and "%" alone matches nothing.
	_, err = f.svc.CreateTemplate(ctx, f.owner, library.TemplateRequest{Code: "LY-9", Name: "Lý 8_9"})
	require.NoError(t, err)
	_, err = f.svc.CreateTemplate(ctx, f.owner, library.TemplateRequest{Code: "LY-9X", Name: "Lý 8x9"})
	require.NoError(t, err)
	rows, total, err := f.svc.ListTemplates(ctx, f.owner, library.ListFilter{Q: "8_9"}, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "LY-9", rows[0].Code)
	_, total, err = f.svc.ListTemplates(ctx, f.owner, library.ListFilter{Q: "%"}, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.Zero(t, total)

	// A soft-deleted template frees its code and vanishes from the list.
	require.NoError(t, f.svc.DeleteTemplate(ctx, f.owner, tpl.ID))
	_, err = f.svc.GetTemplate(ctx, f.owner, tpl.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	rows, total, err = f.svc.ListTemplates(ctx, f.owner, library.ListFilter{Q: "ly-8"}, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
	_, err = f.svc.CreateTemplate(ctx, f.owner, library.TemplateRequest{Code: "LY-8", Name: "Dùng lại"})
	require.NoError(t, err)
}

// A template whose version a class applied cannot be deleted (409), and the
// class-facing read port hands out a published version without library.read
// while refusing drafts and cross-center ids.
func TestTemplateInUseByClassBlocksDeleteAndPublishedPortReads(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "TOAN-6")
	draft := f.draft(t, f.owner, tpl.ID)
	f.lesson(t, f.owner, draft.ID, "Buổi 1")
	f.lesson(t, f.owner, draft.ID, "Buổi 2")

	_, _, err := f.svc.PublishedVersion(ctx, f.owner, draft.ID)
	requireStatus(t, err, http.StatusConflict, library.CodeVersionNotPublished)

	_, err = f.svc.Publish(ctx, f.owner, draft.ID)
	require.NoError(t, err)

	// No library.read on the member scope: the port is gated by the class,
	// not the library.
	member := testutil.ScopeFor(t, f.db, f.member)
	require.False(t, member.Has(authctx.PermLibraryRead))
	version, lessons, err := f.svc.PublishedVersion(ctx, member, draft.ID)
	require.NoError(t, err)
	require.Equal(t, library.StatusPublished, version.Status)
	require.Equal(t, tpl.ID, version.TemplateID)
	require.Len(t, lessons, 2)
	require.Equal(t, "Buổi 1", lessons[0].Title)
	_, _, err = f.svc.PublishedVersion(ctx, f.outsider, draft.ID)
	requireStatus(t, err, http.StatusNotFound, "")

	class := testutil.Class(t, f.db, f.owner.TeacherID)
	require.NoError(t, f.db.Exec(`
		INSERT INTO class_programs (class_id, center_id, template_version_id, applied_by)
		VALUES (?, ?, ?, ?)`, class.ID, f.owner.CenterID, draft.ID, f.owner.TeacherID).Error)
	requireStatus(t, f.svc.DeleteTemplate(ctx, f.owner, tpl.ID), http.StatusConflict, library.CodeTemplateInUse)

	require.NoError(t, f.db.Exec(`DELETE FROM class_programs WHERE class_id = ?`, class.ID).Error)
	require.NoError(t, f.svc.DeleteTemplate(ctx, f.owner, tpl.ID))
}

func TestCrossCenterRowsAreNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "HOA-9")
	draft := f.draft(t, f.owner, tpl.ID)
	lesson := f.lesson(t, f.owner, draft.ID, "Buổi 1")

	_, err := f.svc.GetTemplate(ctx, f.outsider, tpl.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.ListVersions(ctx, f.outsider, tpl.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.CreateVersion(ctx, f.outsider, tpl.ID, library.CreateVersionRequest{})
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.Publish(ctx, f.outsider, draft.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.ListLessons(ctx, f.outsider, draft.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.CreateLesson(ctx, f.outsider, draft.ID, library.LessonRequest{Title: "Lạ"})
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.UpdateLesson(ctx, f.outsider, lesson.ID, library.LessonRequest{Title: "Lạ"})
	requireStatus(t, err, http.StatusNotFound, "")
	requireStatus(t, f.svc.DeleteLesson(ctx, f.outsider, lesson.ID), http.StatusNotFound, "")
	requireStatus(t, f.svc.DeleteTemplate(ctx, f.outsider, tpl.ID), http.StatusNotFound, "")

	// Nothing leaked: the owner still sees everything intact.
	rows, err := f.svc.ListLessons(ctx, f.owner, draft.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestMemberPermissionsFromLiveScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "VAN-6")
	draft := f.draft(t, f.owner, tpl.ID)

	// A member on a system role gets library.read from the role's default
	// keys and nothing more, so they read but cannot write.
	require.NoError(t, f.db.Exec(`
		UPDATE center_members SET role_id = (
			SELECT id FROM center_roles WHERE center_id = ? AND key = 'giao_vien')
		WHERE teacher_id = ? AND center_id = ? AND left_at IS NULL`,
		f.owner.CenterID, f.member, f.owner.CenterID).Error)
	reader := testutil.ScopeFor(t, f.db, f.member)
	require.True(t, reader.Has(authctx.PermLibraryRead))
	_, err := f.svc.GetTemplate(ctx, reader, tpl.ID)
	require.NoError(t, err)
	_, err = f.svc.CreateLesson(ctx, reader, draft.ID, library.LessonRequest{Title: "Buổi 1"})
	requireStatus(t, err, http.StatusForbidden, apperror.CodeForbidden)
	_, err = f.svc.Publish(ctx, reader, draft.ID)
	requireStatus(t, err, http.StatusForbidden, apperror.CodeForbidden)

	// An explicit deny on the member beats the role default.
	require.NoError(t, f.db.Exec(`
		INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		VALUES (?, ?, ?, FALSE)`, f.member, f.owner.CenterID, authctx.PermLibraryRead).Error)
	denied := testutil.ScopeFor(t, f.db, f.member)
	require.False(t, denied.Has(authctx.PermLibraryRead))
	_, err = f.svc.GetTemplate(ctx, denied, tpl.ID)
	requireStatus(t, err, http.StatusForbidden, apperror.CodeForbidden)
	_, err = f.svc.ListLessons(ctx, denied, draft.ID)
	requireStatus(t, err, http.StatusForbidden, apperror.CodeForbidden)
	require.NoError(t, f.db.Exec(`
		DELETE FROM center_member_permissions WHERE teacher_id = ? AND center_id = ? AND permission_key = ?`,
		f.member, f.owner.CenterID, authctx.PermLibraryRead).Error)

	editor := f.grant(t, f.member, authctx.PermLibraryEdit)
	_, err = f.svc.CreateLesson(ctx, editor, draft.ID, library.LessonRequest{Title: "Buổi 1"})
	require.NoError(t, err)
	_, err = f.svc.Publish(ctx, editor, draft.ID)
	requireStatus(t, err, http.StatusForbidden, apperror.CodeForbidden)

	publisher := f.grant(t, f.member, authctx.PermLibraryPublish)
	out, err := f.svc.Publish(ctx, publisher, draft.ID)
	require.NoError(t, err)
	require.Equal(t, library.StatusPublished, out.Status)
}

// lockVersionRow opens a transaction that holds the version's row lock the
// way an in-flight lesson write does, and returns the commit.
func lockVersionRow(t *testing.T, db *gorm.DB, versionID uuid.UUID) func() {
	t.Helper()
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Exec(`SELECT 1 FROM program_template_versions WHERE id = ? FOR UPDATE`, versionID).Error)
	return func() { require.NoError(t, tx.Commit().Error) }
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

func TestLessonWritesSerialiseWithPublish(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "HOA-9")
	v1 := f.draft(t, f.owner, tpl.ID)
	l1 := f.lesson(t, f.owner, v1.ID, "Buổi 1")

	// A lesson write in flight holds the version row: the publish that
	// arrives meanwhile waits for it instead of slipping in between the
	// draft check and the content write.
	release := lockVersionRow(t, f.db, v1.ID)
	published := make(chan error, 1)
	go func() {
		_, err := f.svc.Publish(ctx, f.owner, v1.ID)
		published <- err
	}()
	blocked := make(chan struct{})
	go func() { <-published; close(blocked) }()
	require.False(t, awaits(blocked, 300*time.Millisecond), "publish must wait for the lesson write to commit")
	release()
	require.True(t, awaits(blocked, 5*time.Second), "publish must proceed once the lock is released")

	// After the publish landed, the writes that were queued behind it see a
	// locked version rather than mutating published content.
	_, err := f.svc.UpdateLesson(ctx, f.owner, l1.ID, library.LessonRequest{Title: "Sửa sau khi phát hành"})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)

	// The mirror image: a publish holding the row makes the lesson write wait
	// and then refuse, so the published version is what the publisher saw.
	v2, err := f.svc.CreateVersion(ctx, f.owner, tpl.ID, library.CreateVersionRequest{})
	require.NoError(t, err)
	release = lockVersionRow(t, f.db, v2.ID)
	wrote := make(chan error, 1)
	go func() {
		_, err := f.svc.CreateLesson(ctx, f.owner, v2.ID, library.LessonRequest{Title: "Buổi 2"})
		wrote <- err
	}()
	written := make(chan struct{})
	go func() { <-wrote; close(written) }()
	require.False(t, awaits(written, 300*time.Millisecond), "lesson write must wait for the row lock")
	release()
	require.True(t, awaits(written, 5*time.Second))
}

func TestConcurrentLessonWritesKeepPositionsContiguous(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "SU-7")
	v1 := f.draft(t, f.owner, tpl.ID)
	seed := f.lesson(t, f.owner, v1.ID, "Buổi mở đầu")

	// Several authors append at once while one deletes: every create must
	// succeed with a distinct position, and the list must still be 1..n.
	const writers = 6
	var wg sync.WaitGroup
	errs := make(chan error, writers+1)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.svc.CreateLesson(ctx, f.owner, v1.ID, library.LessonRequest{Title: "Buổi song song"})
			errs <- err
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- f.svc.DeleteLesson(ctx, f.owner, seed.ID)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	rows, err := f.svc.ListLessons(ctx, f.owner, v1.ID)
	require.NoError(t, err)
	require.Len(t, rows, writers)
	for i, row := range rows {
		require.Equal(t, i+1, row.Position, "positions must be contiguous after concurrent writes")
	}
	// And the next append still lands on n+1 rather than a taken slot.
	next, err := f.svc.CreateLesson(ctx, f.owner, v1.ID, library.LessonRequest{Title: "Buổi cuối"})
	require.NoError(t, err)
	require.Equal(t, writers+1, next.Position)
}

func (f fixture) material(t *testing.T, sc authctx.Scope, title string) *library.MaterialResponse {
	t.Helper()
	url := "https://example.com/" + title
	out, err := f.svc.CreateMaterial(context.Background(), sc, library.MaterialRequest{Title: title, Kind: library.MaterialKindLink, URL: &url})
	require.NoError(t, err)
	return out
}

func (f fixture) exercise(t *testing.T, sc authctx.Scope, title string) *library.ExerciseResponse {
	t.Helper()
	out, err := f.svc.CreateExercise(context.Background(), sc, library.ExerciseRequest{Title: title})
	require.NoError(t, err)
	return out
}

func materialInputs(ids ...uuid.UUID) []library.LessonMaterialInput {
	out := make([]library.LessonMaterialInput, 0, len(ids))
	for _, id := range ids {
		out = append(out, library.LessonMaterialInput{MaterialID: id})
	}
	return out
}

func TestLessonAttachmentsAreReplacedAndBlockDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "HOA-9")
	v1 := f.draft(t, f.owner, tpl.ID)
	lesson := f.lesson(t, f.owner, v1.ID, "Buổi 1")
	m1 := f.material(t, f.owner, "Slide")
	m2 := f.material(t, f.owner, "Video")
	ex := f.exercise(t, f.owner, "Bài 1")

	links, err := f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, []library.LessonMaterialInput{
		{MaterialID: m2.ID, SharedWithStudents: true}, {MaterialID: m1.ID},
	})
	require.NoError(t, err)
	require.Len(t, links, 2)
	require.Equal(t, m2.ID, links[0].ID)
	require.True(t, links[0].SharedWithStudents)
	require.Equal(t, 2, links[1].Position)

	// Idempotent: the same body again yields the same rows, no duplicate key.
	again, err := f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, []library.LessonMaterialInput{
		{MaterialID: m2.ID, SharedWithStudents: true}, {MaterialID: m1.ID},
	})
	require.NoError(t, err)
	require.Equal(t, links, again)

	// Reorder and drop one; positions are renumbered from 1.
	links, err = f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(m1.ID))
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, m1.ID, links[0].ID)
	require.Equal(t, 1, links[0].Position)
	require.False(t, links[0].SharedWithStudents)

	exLinks, err := f.svc.SetLessonExercises(ctx, f.owner, lesson.ID, []library.LessonExerciseInput{{ExerciseID: ex.ID}})
	require.NoError(t, err)
	require.Len(t, exLinks, 1)

	detail, err := f.svc.GetLesson(ctx, f.owner, lesson.ID)
	require.NoError(t, err)
	require.Len(t, detail.Materials, 1)
	require.Len(t, detail.Exercises, 1)

	// Linked items stay put; a free one goes.
	requireStatus(t, f.svc.DeleteMaterial(ctx, f.owner, m1.ID), http.StatusConflict, library.CodeMaterialInUse)
	requireStatus(t, f.svc.DeleteExercise(ctx, f.owner, ex.ID), http.StatusConflict, library.CodeExerciseInUse)
	require.NoError(t, f.svc.DeleteMaterial(ctx, f.owner, m2.ID))
	_, err = f.svc.GetMaterial(ctx, f.owner, m2.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	rows, total, err := f.svc.ListMaterials(ctx, f.owner, library.ListFilter{}, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, m1.ID, rows[0].ID)

	// A soft-deleted material is no longer attachable.
	_, err = f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(m1.ID, m2.ID))
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)
}

func TestAttachmentsRespectCenterAndVersionLock(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "SU-7")
	v1 := f.draft(t, f.owner, tpl.ID)
	lesson := f.lesson(t, f.owner, v1.ID, "Buổi 1")
	mine := f.material(t, f.owner, "Của tôi")
	theirs := f.material(t, f.outsider, "Của họ")
	theirEx := f.exercise(t, f.outsider, "Bài của họ")

	_, err := f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(mine.ID, theirs.ID))
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)
	_, err = f.svc.SetLessonExercises(ctx, f.owner, lesson.ID, []library.LessonExerciseInput{{ExerciseID: theirEx.ID}})
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)
	_, err = f.svc.GetMaterial(ctx, f.owner, theirs.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.SetLessonMaterials(ctx, f.outsider, lesson.ID, materialInputs(theirs.ID))
	requireStatus(t, err, http.StatusNotFound, "")

	_, err = f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(mine.ID))
	require.NoError(t, err)
	fields, err := f.svc.SetLogFields(ctx, f.owner, v1.ID, []library.LogFieldInput{
		{Label: "Hiểu bài", Kind: library.LogFieldSelect, Options: []string{"Tốt", " Khá ", "Tốt"}, Required: true},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Tốt", "Khá"}, []string(fields[0].Options))
	set, err := f.svc.SetScoreSet(ctx, f.owner, v1.ID, []library.ScoreComponentInput{{Key: "final", Label: "Cuối kỳ", Max: 10, Weight: 1}})
	require.NoError(t, err)
	require.Len(t, set, 1)

	_, err = f.svc.Publish(ctx, f.owner, v1.ID)
	require.NoError(t, err)
	_, err = f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(mine.ID))
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	_, err = f.svc.SetLessonExercises(ctx, f.owner, lesson.ID, nil)
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	_, err = f.svc.SetLogFields(ctx, f.owner, v1.ID, nil)
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	_, err = f.svc.SetScoreSet(ctx, f.owner, v1.ID, nil)
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	// Still linked by the published version, so the material cannot go.
	requireStatus(t, f.svc.DeleteMaterial(ctx, f.owner, mine.ID), http.StatusConflict, library.CodeMaterialInUse)

	// The new draft inherits attachments, log fields and score set as its own rows.
	v2, err := f.svc.CreateVersion(ctx, f.owner, tpl.ID, library.CreateVersionRequest{})
	require.NoError(t, err)
	detail, err := f.svc.GetVersion(ctx, f.owner, v2.ID)
	require.NoError(t, err)
	require.Len(t, detail.Lessons, 1)
	require.Len(t, detail.Lessons[0].Materials, 1)
	require.Equal(t, mine.ID, detail.Lessons[0].Materials[0].ID)
	require.Len(t, detail.LogFields, 1)
	require.NotEqual(t, fields[0].ID, detail.LogFields[0].ID)
	require.Equal(t, set, detail.ScoreSet)

	published, err := f.svc.GetVersion(ctx, f.owner, v1.ID)
	require.NoError(t, err)
	require.Len(t, published.LogFields, 1)
	require.Equal(t, set, published.ScoreSet)
	_, err = f.svc.GetVersion(ctx, f.outsider, v1.ID)
	requireStatus(t, err, http.StatusNotFound, "")
}

// lockMaterialRow opens a transaction that holds the material's row the way
// an in-flight write does, runs prepare inside it, and returns the commit.
func lockMaterialRow(t *testing.T, db *gorm.DB, materialID uuid.UUID, strength string, prepare func(tx *gorm.DB)) func() {
	t.Helper()
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Exec(`SELECT 1 FROM library_materials WHERE id = ? FOR `+strength, materialID).Error)
	prepare(tx)
	return func() { require.NoError(t, tx.Commit().Error) }
}

func TestDeleteAndAttachSerialiseOnTheItemRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "HOA-9")
	v1 := f.draft(t, f.owner, tpl.ID)
	lesson := f.lesson(t, f.owner, v1.ID, "Buổi 1")
	m := f.material(t, f.owner, "Slide")

	// An attach in flight has read the material (shared lock) and written
	// the link; the delete that arrives meanwhile waits, then sees the link.
	release := lockMaterialRow(t, f.db, m.ID, "SHARE", func(tx *gorm.DB) {
		require.NoError(t, tx.Exec(`
			INSERT INTO template_lesson_materials (lesson_id, material_id, center_id, shared_with_students, position)
			VALUES (?, ?, ?, FALSE, 1)`, lesson.ID, m.ID, f.owner.CenterID).Error)
	})
	done := make(chan error, 1)
	go func() { done <- f.svc.DeleteMaterial(ctx, f.owner, m.ID) }()
	select {
	case err := <-done:
		t.Fatalf("delete must wait for the attach, returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	requireStatus(t, <-done, http.StatusConflict, library.CodeMaterialInUse)
	detail, err := f.svc.GetLesson(ctx, f.owner, lesson.ID)
	require.NoError(t, err)
	require.Len(t, detail.Materials, 1)

	// The mirror image: a delete in flight holds the row; the attach that
	// arrives meanwhile waits and then finds no live material.
	m2 := f.material(t, f.owner, "Video")
	release = lockMaterialRow(t, f.db, m2.ID, "UPDATE", func(tx *gorm.DB) {
		require.NoError(t, tx.Exec(`UPDATE library_materials SET deleted_at = now() WHERE id = ?`, m2.ID).Error)
	})
	attached := make(chan error, 1)
	go func() {
		_, err := f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(m.ID, m2.ID))
		attached <- err
	}()
	select {
	case err := <-attached:
		t.Fatalf("attach must wait for the delete, returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	requireStatus(t, <-attached, http.StatusUnprocessableEntity, apperror.CodeValidation)
	detail, err = f.svc.GetLesson(ctx, f.owner, lesson.ID)
	require.NoError(t, err)
	require.Len(t, detail.Materials, 1)
	require.Equal(t, m.ID, detail.Materials[0].ID)
}

func TestLinksOfDeletedTemplatesDoNotBlockItemDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "HOA-9")
	v1 := f.draft(t, f.owner, tpl.ID)
	lesson := f.lesson(t, f.owner, v1.ID, "Buổi 1")
	m := f.material(t, f.owner, "Slide")
	ex := f.exercise(t, f.owner, "Bài 1")
	_, err := f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(m.ID))
	require.NoError(t, err)
	_, err = f.svc.SetLessonExercises(ctx, f.owner, lesson.ID, []library.LessonExerciseInput{{ExerciseID: ex.ID}})
	require.NoError(t, err)

	err = f.svc.DeleteMaterial(ctx, f.owner, m.ID)
	requireStatus(t, err, http.StatusConflict, library.CodeMaterialInUse)
	require.Contains(t, err.Error(), "gỡ khỏi")

	_, err = f.svc.Publish(ctx, f.owner, v1.ID)
	require.NoError(t, err)
	err = f.svc.DeleteMaterial(ctx, f.owner, m.ID)
	requireStatus(t, err, http.StatusConflict, library.CodeMaterialInUse)
	require.Contains(t, err.Error(), "đã phát hành")
	err = f.svc.DeleteExercise(ctx, f.owner, ex.ID)
	requireStatus(t, err, http.StatusConflict, library.CodeExerciseInUse)
	require.Contains(t, err.Error(), "đã phát hành")

	// A second, live template keeps its own hold on the item.
	other := f.template(t, f.owner, "HOA-10")
	v2 := f.draft(t, f.owner, other.ID)
	otherLesson := f.lesson(t, f.owner, v2.ID, "Buổi 1")
	_, err = f.svc.SetLessonMaterials(ctx, f.owner, otherLesson.ID, materialInputs(m.ID))
	require.NoError(t, err)

	require.NoError(t, f.svc.DeleteTemplate(ctx, f.owner, tpl.ID))
	err = f.svc.DeleteMaterial(ctx, f.owner, m.ID)
	requireStatus(t, err, http.StatusConflict, library.CodeMaterialInUse)
	require.Contains(t, err.Error(), "gỡ khỏi")
	require.NoError(t, f.svc.DeleteExercise(ctx, f.owner, ex.ID))

	require.NoError(t, f.svc.DeleteTemplate(ctx, f.owner, other.ID))
	require.NoError(t, f.svc.DeleteMaterial(ctx, f.owner, m.ID))
	_, err = f.svc.GetMaterial(ctx, f.owner, m.ID)
	requireStatus(t, err, http.StatusNotFound, "")
}

func TestAttachToLessonDeletedMeanwhileIsNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl := f.template(t, f.owner, "HOA-9")
	v1 := f.draft(t, f.owner, tpl.ID)
	lesson := f.lesson(t, f.owner, v1.ID, "Buổi 1")
	m := f.material(t, f.owner, "Slide")

	// A lesson delete in flight holds the version row; the attach that
	// resolved the lesson before the lock must not insert a dangling link.
	tx := f.db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Exec(`SELECT 1 FROM program_template_versions WHERE id = ? FOR UPDATE`, v1.ID).Error)
	require.NoError(t, tx.Exec(`DELETE FROM template_lessons WHERE id = ?`, lesson.ID).Error)
	done := make(chan error, 1)
	go func() {
		_, err := f.svc.SetLessonMaterials(ctx, f.owner, lesson.ID, materialInputs(m.ID))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("attach must wait for the version lock, returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit().Error)
	requireStatus(t, <-done, http.StatusNotFound, "")
	require.NoError(t, f.svc.DeleteMaterial(ctx, f.owner, m.ID))
}

func TestPrepBoardAndAssignment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	page := pagination.Params{Page: 1, PerPage: 20}

	tpl, err := f.svc.CreateTemplate(ctx, f.owner, library.TemplateRequest{Code: "CB-3", Name: "Chuẩn bị 3 buổi", LessonCount: intp(3)})
	require.NoError(t, err)
	require.NotNil(t, tpl.Prep)
	require.Equal(t, 3, tpl.Prep.LessonCount)
	require.Equal(t, 0, tpl.Prep.DoneCount)
	require.Empty(t, tpl.Prep.Assignees)
	draft := f.draft(t, f.owner, tpl.ID)
	require.NotNil(t, tpl.DraftVersionID)
	require.Equal(t, draft.ID, *tpl.DraftVersionID)
	lessons, err := f.svc.ListLessons(ctx, f.owner, draft.ID)
	require.NoError(t, err)
	require.Len(t, lessons, 3)
	for i, l := range lessons {
		require.Equal(t, i+1, l.Position)
		require.Equal(t, "Buổi "+strconv.Itoa(i+1), l.Title)
		require.Equal(t, library.PrepTodo, l.PrepStatus)
		require.NotNil(t, l.Checklist)
		require.Empty(t, l.Checklist)
	}

	// A published-only template drops out of the has_draft list.
	other := f.template(t, f.owner, "CB-0")
	_, err = f.svc.Publish(ctx, f.owner, f.draft(t, f.owner, other.ID).ID)
	require.NoError(t, err)
	rows, total, err := f.svc.ListTemplates(ctx, f.owner, library.ListFilter{HasDraft: true}, page)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, tpl.ID, rows[0].ID)
	rows, total, err = f.svc.ListTemplates(ctx, f.owner, library.ListFilter{}, page)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	for _, row := range rows {
		if row.ID == other.ID {
			require.Nil(t, row.Prep, "no draft, no prep summary")
		}
	}

	editor := f.grant(t, f.member, authctx.PermLibraryEdit)
	checklist := []library.ChecklistItem{{Label: "In phiếu bài tập", Done: true}, {Label: "Soạn slide"}}
	got, err := f.svc.UpdateLessonPrep(ctx, editor, lessons[0].ID, library.PrepRequest{PrepStatus: strp(library.PrepDoing), Checklist: &checklist})
	require.NoError(t, err)
	require.Equal(t, library.PrepDoing, got.PrepStatus)
	require.Equal(t, checklist, got.Checklist)
	_, err = f.svc.UpdateLessonPrep(ctx, editor, lessons[1].ID, library.PrepRequest{PrepStatus: strp(library.PrepDone)})
	require.NoError(t, err)

	// Assignment needs prep.assign on top of library.edit, and only a live
	// member of the center can be assigned.
	_, err = f.svc.UpdateLessonAssignment(ctx, editor, lessons[0].ID, library.AssignmentRequest{AssigneeID: &f.member})
	requireStatus(t, err, http.StatusForbidden, "")
	assigner := f.grant(t, f.member, authctx.PermPrepAssign)
	got, err = f.svc.UpdateLessonAssignment(ctx, assigner, lessons[0].ID, library.AssignmentRequest{AssigneeID: &f.member, DueDate: strp("2026-10-01")})
	require.NoError(t, err)
	require.Equal(t, &f.member, got.AssigneeID)
	require.Equal(t, "2026-10-01", *got.DueDate)
	_, err = f.svc.UpdateLessonAssignment(ctx, f.owner, lessons[0].ID, library.AssignmentRequest{AssigneeID: &f.outsider.TeacherID})
	var appErr *apperror.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.NotEmpty(t, appErr.Fields["assignee_id"])

	// A member who has since left the center (soft-leave: left_at stamped,
	// row kept) must also be rejected — IsLiveMember's left_at IS NULL check,
	// not the assignee_id FK, is what keeps a departed member unassignable.
	_, leftT := testutil.Teacher(t, f.db, testutil.WithFullName("Cô Lan"))
	testutil.JoinCenter(t, f.db, leftT.ID, f.owner.CenterID)
	require.NoError(t, f.db.Exec(
		`UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND center_id = ? AND left_at IS NULL`,
		leftT.ID, f.owner.CenterID).Error)
	_, err = f.svc.UpdateLessonAssignment(ctx, assigner, lessons[0].ID, library.AssignmentRequest{AssigneeID: &leftT.ID})
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.NotEmpty(t, appErr.Fields["assignee_id"])

	board, err := f.svc.GetBoard(ctx, f.grant(t, f.member, authctx.PermLibraryRead), draft.ID)
	require.NoError(t, err)
	require.Equal(t, tpl.ID, board.Template.ID)
	require.Equal(t, draft.ID, board.Version.ID)
	require.Len(t, board.Columns, 4)
	require.Equal(t, library.PrepStatuses, []string{board.Columns[0].Status, board.Columns[1].Status, board.Columns[2].Status, board.Columns[3].Status})
	require.Len(t, board.Columns[0].Lessons, 1)
	require.Len(t, board.Columns[1].Lessons, 1)
	require.NotNil(t, board.Columns[2].Lessons)
	require.Empty(t, board.Columns[2].Lessons)
	require.Len(t, board.Columns[3].Lessons, 1)
	card := board.Columns[1].Lessons[0]
	require.Equal(t, lessons[0].ID, card.ID)
	require.Equal(t, "Thầy Minh", *card.AssigneeName)
	require.Equal(t, "2026-10-01", *card.DueDate)
	require.Equal(t, 1, card.ChecklistDone)
	require.Equal(t, 2, card.ChecklistTotal)

	summary, err := f.svc.GetTemplate(ctx, f.owner, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, &library.PrepSummaryResponse{LessonCount: 3, DoneCount: 1, Assignees: []string{"Thầy Minh"}}, summary.Prep)

	// Locked versions reject preparation changes; a new draft starts over.
	_, err = f.svc.Publish(ctx, f.owner, draft.ID)
	require.NoError(t, err)
	_, err = f.svc.UpdateLessonPrep(ctx, f.owner, lessons[0].ID, library.PrepRequest{PrepStatus: strp(library.PrepDone)})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	_, err = f.svc.UpdateLessonAssignment(ctx, f.owner, lessons[0].ID, library.AssignmentRequest{})
	requireStatus(t, err, http.StatusConflict, library.CodeVersionLocked)
	board, err = f.svc.GetBoard(ctx, f.owner, draft.ID)
	require.NoError(t, err, "the board of a locked version stays readable")
	require.Equal(t, library.StatusPublished, board.Version.Status)

	v2, err := f.svc.CreateVersion(ctx, f.owner, tpl.ID, library.CreateVersionRequest{})
	require.NoError(t, err)
	copied, err := f.svc.ListLessons(ctx, f.owner, v2.ID)
	require.NoError(t, err)
	require.Len(t, copied, 3)
	for _, l := range copied {
		require.Equal(t, library.PrepTodo, l.PrepStatus)
		require.Nil(t, l.AssigneeID)
		require.Nil(t, l.DueDate)
		require.Empty(t, l.Checklist)
	}
	summary, err = f.svc.GetTemplate(ctx, f.owner, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, &library.PrepSummaryResponse{LessonCount: 3, DoneCount: 0, Assignees: []string{}}, summary.Prep)
}

func intp(n int) *int       { return &n }
func strp(s string) *string { return &s }
