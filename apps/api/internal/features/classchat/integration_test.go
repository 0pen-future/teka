//go:build integration

package classchat_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classchat"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// fixture is one center: its owner, the class's primary teacher, and a
// trợ giảng holding an active stint on the class; plus another center's
// owner as the outsider.
type fixture struct {
	db        *gorm.DB
	svc       *classchat.Service
	owner     authctx.Scope
	teacher   authctx.Scope
	assistant authctx.Scope
	class     *classes.Class
	outsider  authctx.Scope
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	staffRepo := classstaff.NewRepository(db)
	classesSvc := classes.NewService(classes.NewRepository(db), database.NewTxManager(db), staffRepo)
	svc := classchat.NewService(classchat.NewRepository(db), classesSvc, staffRepo)

	_, ownerT := testutil.Teacher(t, db)
	_, teacherT := testutil.Teacher(t, db, testutil.WithFullName("Thầy Minh"))
	testutil.JoinCenter(t, db, teacherT.ID, ownerT.CenterID)
	_, assistantT := testutil.Teacher(t, db, testutil.WithFullName("Cô Thu"))
	testutil.JoinCenter(t, db, assistantT.ID, ownerT.CenterID)
	_, outsiderT := testutil.Teacher(t, db)
	class := testutil.Class(t, db, teacherT.ID)
	testutil.StaffAssignment(t, db, class, assistantT.ID, "tro_giang")

	f := fixture{
		db:       db,
		svc:      svc,
		owner:    testutil.ScopeFor(t, db, ownerT.ID),
		class:    class,
		outsider: testutil.ScopeFor(t, db, outsiderT.ID),
	}
	f.teacher = f.grant(t, teacherT.ID, authctx.PermClassMessagesPost)
	f.assistant = f.grant(t, assistantT.ID, authctx.PermClassMessagesPost)
	return f
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

func (f fixture) post(t *testing.T, sc authctx.Scope, body string) *classchat.MessageResponse {
	t.Helper()
	out, err := f.svc.Post(context.Background(), sc, f.class.ID, classchat.PostRequest{Body: body})
	require.NoError(t, err)
	return out
}

func bodiesOf(items []classchat.MessageResponse) []string {
	out := make([]string, 0, len(items))
	for _, m := range items {
		out = append(out, m.Body)
	}
	return out
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

func TestOwnerAndActiveStaffChatWhileAClosedStintIsForbidden(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	first := f.post(t, f.owner, "Chào cả lớp")
	require.Equal(t, f.owner.TeacherID, first.AuthorID)
	require.Equal(t, f.class.ID, first.ClassID)
	second := f.post(t, f.teacher, "  Em chào cô  ")
	require.Equal(t, "Em chào cô", second.Body, "bodies are trimmed")
	require.Equal(t, "Thầy Minh", second.AuthorName)
	f.post(t, f.assistant, "Em đã nhận tài liệu")

	for _, sc := range []authctx.Scope{f.owner, f.teacher, f.assistant} {
		page, err := f.svc.List(ctx, sc, f.class.ID, classchat.ListQuery{})
		require.NoError(t, err)
		require.Equal(t, []string{"Em đã nhận tài liệu", "Em chào cô", "Chào cả lớp"}, bodiesOf(page.Items), "newest first")
		require.Empty(t, page.NextCursor)
	}

	// Blank bodies never land.
	_, err := f.svc.Post(ctx, f.teacher, f.class.ID, classchat.PostRequest{Body: "   "})
	requireStatus(t, err, http.StatusUnprocessableEntity, apperror.CodeValidation)

	// A closed stint keeps the class readable elsewhere, but the chat is for
	// people currently on the class.
	err = f.db.Exec(`UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?`,
		f.class.ID, f.assistant.TeacherID).Error
	require.NoError(t, err)
	_, err = f.svc.List(ctx, f.assistant, f.class.ID, classchat.ListQuery{})
	requireStatus(t, err, http.StatusForbidden, "")
	_, err = f.svc.Post(ctx, f.assistant, f.class.ID, classchat.PostRequest{Body: "Còn ai không"})
	requireStatus(t, err, http.StatusForbidden, "")

	// Without the permission a member on the class still reads but cannot post.
	err = f.db.Exec(`UPDATE center_member_permissions SET allowed = FALSE WHERE teacher_id = ? AND permission_key = ?`,
		f.teacher.TeacherID, authctx.PermClassMessagesPost).Error
	require.NoError(t, err)
	reader := testutil.ScopeFor(t, f.db, f.teacher.TeacherID)
	_, err = f.svc.List(ctx, reader, f.class.ID, classchat.ListQuery{})
	require.NoError(t, err)
	_, err = f.svc.Post(ctx, reader, f.class.ID, classchat.PostRequest{Body: "Xin chào"})
	requireStatus(t, err, http.StatusForbidden, "")

	// Another center never sees the class.
	_, err = f.svc.List(ctx, f.outsider, f.class.ID, classchat.ListQuery{})
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.Post(ctx, f.outsider, f.class.ID, classchat.PostRequest{Body: "Xin chào"})
	requireStatus(t, err, http.StatusNotFound, "")
	_, err = f.svc.List(ctx, f.owner, uuid.New(), classchat.ListQuery{})
	requireStatus(t, err, http.StatusNotFound, "")
}

func TestListPagesNewestFirstWithABeforeCursor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, body := range []string{"1", "2", "3", "4", "5"} {
		f.post(t, f.teacher, body)
	}

	page, err := f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{Limit: 2})
	require.NoError(t, err)
	require.Equal(t, []string{"5", "4"}, bodiesOf(page.Items))
	require.Equal(t, page.Items[1].ID.String(), page.NextCursor)

	cursor := page.Items[1].ID
	page, err = f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{Limit: 2, Before: &cursor})
	require.NoError(t, err)
	require.Equal(t, []string{"3", "2"}, bodiesOf(page.Items))
	require.NotEmpty(t, page.NextCursor)

	cursor = page.Items[1].ID
	page, err = f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{Limit: 2, Before: &cursor})
	require.NoError(t, err)
	require.Equal(t, []string{"1"}, bodiesOf(page.Items))
	require.Empty(t, page.NextCursor, "the last page carries no cursor")

	// A cursor that is not one of the class's messages yields nothing rather
	// than leaking another class's position.
	stray := uuid.New()
	page, err = f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{Limit: 2, Before: &stray})
	require.NoError(t, err)
	require.Empty(t, page.Items)

	// Limits are clamped: 0 falls back to the default, oversized to the cap.
	page, err = f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{Limit: 0})
	require.NoError(t, err)
	require.Len(t, page.Items, 5)
	page, err = f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{Limit: 500})
	require.NoError(t, err)
	require.Len(t, page.Items, 5)

	// The HTTP surface parses the query and rejects a malformed cursor.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { authctx.SetScope(c, f.owner); c.Next() })
	classchat.RegisterRoutes(r.Group("/api/v1"), classchat.NewHandler(f.svc))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/classes/"+f.class.ID.String()+"/messages?limit=2&before="+cursor.String(), nil))
	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data classchat.ListResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, []string{"1"}, bodiesOf(env.Data.Items))

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/classes/"+f.class.ID.String()+"/messages?before=not-a-uuid", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/classes/"+f.class.ID.String()+"/messages",
		strings.NewReader(`{"body":"`+strings.Repeat("a", 2001)+`"}`)))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestDeleteByAuthorOrOwnerOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	msg := f.post(t, f.teacher, "Nhầm lớp")
	ownerMsg := f.post(t, f.owner, "Thông báo")

	err := f.svc.Delete(ctx, f.assistant, f.class.ID, msg.ID)
	requireStatus(t, err, http.StatusForbidden, "")
	err = f.svc.Delete(ctx, f.teacher, f.class.ID, ownerMsg.ID)
	requireStatus(t, err, http.StatusForbidden, "")

	require.NoError(t, f.svc.Delete(ctx, f.teacher, f.class.ID, msg.ID))
	page, err := f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{})
	require.NoError(t, err)
	require.Equal(t, []string{"Thông báo"}, bodiesOf(page.Items))

	// Soft-deleted: the row stays for the record, but is gone for callers.
	var deleted int64
	require.NoError(t, f.db.Table("class_messages").Where("id = ? AND deleted_at IS NOT NULL", msg.ID).Count(&deleted).Error)
	require.EqualValues(t, 1, deleted)
	err = f.svc.Delete(ctx, f.teacher, f.class.ID, msg.ID)
	requireStatus(t, err, http.StatusNotFound, "")

	// The owner retracts anyone's message; a message id under the wrong
	// class is not found.
	other := testutil.Class(t, f.db, f.teacher.TeacherID)
	err = f.svc.Delete(ctx, f.owner, other.ID, ownerMsg.ID)
	requireStatus(t, err, http.StatusNotFound, "")
	require.NoError(t, f.svc.Delete(ctx, f.owner, f.class.ID, ownerMsg.ID))
	page, err = f.svc.List(ctx, f.owner, f.class.ID, classchat.ListQuery{})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}
