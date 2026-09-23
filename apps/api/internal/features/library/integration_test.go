//go:build integration

package library_test

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
	_, memberT := testutil.Teacher(t, db)
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
