//go:build integration

package teaching_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// newIntegrationService wires the real dependency chain router.go uses:
// teaching consumes classes and sessions (class/session resolution = its
// authorization gates) and enrollments (the marks roster check) through
// consumer interfaces.
func newIntegrationService(t *testing.T) (*teaching.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	svc := teaching.NewService(teaching.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)
	return svc, db
}

func date(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func setTo[T any](v T) teaching.Optional[T] { return teaching.Optional[T]{Set: true, Value: &v} }

func clearIt[T any]() teaching.Optional[T] { return teaching.Optional[T]{Set: true} }

// The full review loop against real Postgres: curriculum save, plan save →
// submit → approve, and the final list resolving the submitter's display
// name through the teachers join.
func TestLessonPlanReviewLoop(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	cur, err := svc.PutCurriculum(ctx, sc, class.ID, teaching.PutCurriculumRequest{
		Lessons: []string{"Chào hỏi", "Số đếm"}, CurrentIndex: 1,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Chào hỏi", "Số đếm"}, cur.Lessons)
	require.Equal(t, 1, cur.CurrentIndex)

	saved, err := svc.SavePlan(ctx, sc, class.ID, 1, teaching.SavePlanRequest{
		Goal: "Đếm 1–10", Activities: []string{"flashcards", " "}, Homework: "bài 2",
	})
	require.NoError(t, err)
	require.Equal(t, teaching.StatusDraft, saved.Status)
	require.Equal(t, []string{"flashcards"}, saved.Activities, "blank activity lines must drop")

	submitted, err := svc.SubmitPlan(ctx, sc, class.ID, 1)
	require.NoError(t, err)
	require.Equal(t, teaching.StatusPending, submitted.Status)
	require.NotNil(t, submitted.SubmittedAt)

	approved, err := svc.ApprovePlan(ctx, sc, class.ID, 1, teaching.ReviewRequest{Comment: "duyệt"})
	require.NoError(t, err)
	require.Equal(t, teaching.StatusApproved, approved.Status)
	require.NotNil(t, approved.OwnerComment)
	require.Equal(t, "duyệt", *approved.OwnerComment)

	plans, err := svc.ListPlans(ctx, sc, class.ID)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Equal(t, teaching.StatusApproved, plans[0].Status)
	require.NotNil(t, plans[0].SubmittedByName, "the list must resolve the submitter's display name")
	require.Equal(t, teacher.FullName, *plans[0].SubmittedByName)
}

// A member gets 403 on every owner surface even for their own class's plan;
// the owner reviews the member's plan fine.
func TestMemberGets403OnOwnerActions(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	require.False(t, memberScope.IsOwner)

	class := testutil.Class(t, db, member.ID)
	_, err := svc.PutCurriculum(ctx, memberScope, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)
	_, err = svc.SavePlan(ctx, memberScope, class.ID, 0, teaching.SavePlanRequest{Goal: "g"})
	require.NoError(t, err)
	_, err = svc.SubmitPlan(ctx, memberScope, class.ID, 0)
	require.NoError(t, err)

	_, err = svc.ApprovePlan(ctx, memberScope, class.ID, 0, teaching.ReviewRequest{})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = svc.RequestRedo(ctx, memberScope, class.ID, 0, teaching.ReviewRequest{Comment: "x"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = svc.ReopenPlan(ctx, memberScope, class.ID, 0)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	_, err = svc.ReviewQueue(ctx, memberScope)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	approved, err := svc.ApprovePlan(ctx, ownerScope, class.ID, 0, teaching.ReviewRequest{})
	require.NoError(t, err, "the owner must review a member's plan")
	require.Equal(t, teaching.StatusApproved, approved.Status)
}

// Peer isolation vs owner oversight: a fellow member cannot even resolve
// another member's class (404, existence hidden); the owner reads AND edits
// it center-wide — the write capability gates staff roles, never the owner —
// while the content rows keep the class teacher's anchor.
func TestPeerHiddenOwnerReadsAndEdits(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, memberB := testutil.Teacher(t, db)
	_, memberC := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, memberB.ID, ownerCenter)
	testutil.JoinCenter(t, db, memberC.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	scopeB := testutil.ScopeFor(t, db, memberB.ID)
	scopeC := testutil.ScopeFor(t, db, memberC.ID)

	class := testutil.Class(t, db, memberB.ID)
	_, err := svc.PutCurriculum(ctx, scopeB, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)

	_, err = svc.GetCurriculum(ctx, scopeC, class.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not resolve another member's class")
	_, err = svc.ListPlans(ctx, scopeC, class.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	got, err := svc.GetCurriculum(ctx, ownerScope, class.ID)
	require.NoError(t, err, "the owner must read a member's curriculum")
	require.Equal(t, []string{"Bài 1"}, got.Lessons)

	edited, err := svc.PutCurriculum(ctx, ownerScope, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài sửa"}})
	require.NoError(t, err, "the owner edits a member's content center-wide")
	require.Equal(t, []string{"Bài sửa"}, edited.Lessons)
	var anchor string
	require.NoError(t, db.Raw(
		"SELECT teacher_id::text FROM class_curricula WHERE class_id = ?", class.ID).Scan(&anchor).Error)
	require.Equal(t, memberB.ID.String(), anchor,
		"an owner edit keeps the class teacher's row anchor")
	_, err = svc.SavePlan(ctx, ownerScope, class.ID, 0, teaching.SavePlanRequest{Goal: "g"})
	require.NoError(t, err, "the owner saves a plan on a member's class")
}

// The redo round trip: an empty comment is refused as a validation error;
// with a comment the plan lands in redo and the teacher sees the note in the
// list until resubmission consumes it.
func TestRequestRedoCommentRoundTrip(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	_, err := svc.PutCurriculum(ctx, sc, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)
	_, err = svc.SavePlan(ctx, sc, class.ID, 0, teaching.SavePlanRequest{Goal: "g"})
	require.NoError(t, err)
	_, err = svc.SubmitPlan(ctx, sc, class.ID, 0)
	require.NoError(t, err)

	_, err = svc.RequestRedo(ctx, sc, class.ID, 0, teaching.ReviewRequest{Comment: "  "})
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeValidation, appErr.Code)
	require.NotEmpty(t, appErr.Fields["comment"])

	_, err = svc.RequestRedo(ctx, sc, class.ID, 0, teaching.ReviewRequest{Comment: "thiếu bài tập"})
	require.NoError(t, err)

	plans, err := svc.ListPlans(ctx, sc, class.ID)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Equal(t, teaching.StatusRedo, plans[0].Status)
	require.NotNil(t, plans[0].RedoNote)
	require.Equal(t, "thiếu bài tập", *plans[0].RedoNote)

	_, err = svc.SubmitPlan(ctx, sc, class.ID, 0)
	require.NoError(t, err)
	plans, err = svc.ListPlans(ctx, sc, class.ID)
	require.NoError(t, err)
	require.Nil(t, plans[0].RedoNote, "resubmission must consume the redo note")
}

// The owner queue joins class name, submitter name, and lesson title — and
// the title is NULL-safe: shrinking the curriculum under a pending plan
// leaves the row listed with a nil title instead of dropping or erroring.
func TestReviewQueueJoinsAndNullSafeTitle(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	class := testutil.Class(t, db, member.ID)
	_, err := svc.PutCurriculum(ctx, memberScope, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1", "Bài 2"}})
	require.NoError(t, err)
	_, err = svc.SavePlan(ctx, memberScope, class.ID, 1, teaching.SavePlanRequest{Goal: "g"})
	require.NoError(t, err)
	_, err = svc.SubmitPlan(ctx, memberScope, class.ID, 1)
	require.NoError(t, err)

	queue, err := svc.ReviewQueue(ctx, ownerScope)
	require.NoError(t, err)
	require.Len(t, queue, 1)
	require.Equal(t, class.ID, queue[0].ClassID)
	require.NotEmpty(t, queue[0].ClassName)
	require.Equal(t, 1, queue[0].LessonIndex)
	require.NotNil(t, queue[0].LessonTitle)
	require.Equal(t, "Bài 2", *queue[0].LessonTitle)
	require.NotNil(t, queue[0].TeacherName)
	require.Equal(t, member.FullName, *queue[0].TeacherName)
	require.NotNil(t, queue[0].SubmittedAt)

	// Shrink the curriculum below the pending plan's index: the queue row
	// must survive with a nil title.
	_, err = svc.PutCurriculum(ctx, memberScope, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)
	queue, err = svc.ReviewQueue(ctx, ownerScope)
	require.NoError(t, err)
	require.Len(t, queue, 1)
	require.Nil(t, queue[0].LessonTitle)
}

// A cross-center teacher resolves nothing at all — 404, never 403.
func TestCrossCenterTeachingIsNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacherA := testutil.Teacher(t, db)
	_, teacherB := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	class := testutil.Class(t, db, teacherA.ID)
	_, err := svc.PutCurriculum(ctx, scopeA, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)

	_, err = svc.GetCurriculum(ctx, scopeB, class.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.SavePlan(ctx, scopeB, class.ID, 0, teaching.SavePlanRequest{Goal: "g"})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// The month read's window against real Postgres: exactly the class's sessions
// with session_date in [month start, next month start) — a July 31 and a
// September 1 session must both stay out of August.
func TestMonthMarksWindow(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-07-01")))
	contact := testutil.Contact(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-07-01"))

	edges := map[string]time.Time{
		"before": date("2026-07-31"),
		"first":  date("2026-08-01"),
		"last":   date("2026-08-31"),
		"after":  date("2026-09-01"),
	}
	inAugust := map[uuid.UUID]bool{}
	for name, day := range edges {
		session := testutil.Session(t, db, teacher.ID, class.ID, day)
		_, err := svc.PutNote(ctx, sc, session.ID, teaching.PutNoteRequest{Body: "note " + name})
		require.NoError(t, err)
		_, err = svc.PutMarks(ctx, sc, session.ID, []teaching.MarkEntryRequest{{StudentID: student.ID, Score: setTo(8.0)}})
		require.NoError(t, err)
		if name == "first" || name == "last" {
			inAugust[session.ID] = true
		}
	}

	got, err := svc.GetMonthMarks(ctx, sc, class.ID, "2026-08")
	require.NoError(t, err)
	require.Len(t, got.SessionNotes, 2)
	require.Len(t, got.Marks, 2)
	for _, note := range got.SessionNotes {
		require.True(t, inAugust[note.SessionID], "note outside August leaked into the window")
	}
	for _, mark := range got.Marks {
		require.True(t, inAugust[mark.SessionID], "mark outside August leaked into the window")
	}
}

// The note lifecycle against the real unique row: verbatim body round-trip,
// upsert-in-place on re-save, and delete-on-empty leaving no row behind.
func TestSessionNoteUpsertAndDeleteOnEmpty(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-08-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-08-04"))

	_, err := svc.PutNote(ctx, sc, session.ID, teaching.PutNoteRequest{Body: "Lớp học sôi nổi"})
	require.NoError(t, err)
	saved, err := svc.PutNote(ctx, sc, session.ID, teaching.PutNoteRequest{Body: "Đã sửa nhận xét"})
	require.NoError(t, err)
	require.Equal(t, "Đã sửa nhận xét", saved.Body)

	var count int64
	require.NoError(t, db.Table("session_notes").Where("session_id = ?", session.ID).Count(&count).Error)
	require.EqualValues(t, 1, count, "re-save must upsert the one row, not add another")

	cleared, err := svc.PutNote(ctx, sc, session.ID, teaching.PutNoteRequest{Body: "  "})
	require.NoError(t, err)
	require.Empty(t, cleared.Body)
	require.NoError(t, db.Table("session_notes").Where("session_id = ?", session.ID).Count(&count).Error)
	require.EqualValues(t, 0, count, "an empty body must delete the row")
}

// The marks merge against real rows: partial writes keep the other field,
// clearing both deletes the row, and an off-roster student with no existing
// row is refused (an existing row would stay editable after unenrollment).
func TestSessionMarksMergeAndRoster(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, teacher.ID)
	enrolled := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, enrolled.ID, class.ID, date("2026-08-01"))
	outsider := testutil.Student(t, db, teacher.ID, contact.ID)
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-08-04"))

	_, err := svc.PutMarks(ctx, sc, session.ID, []teaching.MarkEntryRequest{{StudentID: outsider.ID, Score: setTo(5.0)}})
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeValidation, appErr.Code)
	require.NotEmpty(t, appErr.Fields["marks"], "an un-enrolled student must be refused")

	got, err := svc.PutMarks(ctx, sc, session.ID, []teaching.MarkEntryRequest{{StudentID: enrolled.ID, Score: setTo(8.5)}})
	require.NoError(t, err)
	require.Len(t, got, 1)

	got, err = svc.PutMarks(ctx, sc, session.ID, []teaching.MarkEntryRequest{{StudentID: enrolled.ID, PersonalNote: setTo("chăm phát biểu")}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Score)
	require.Equal(t, 8.5, *got[0].Score, "the note write must not clobber the score")
	require.NotNil(t, got[0].PersonalNote)

	got, err = svc.PutMarks(ctx, sc, session.ID, []teaching.MarkEntryRequest{{StudentID: enrolled.ID, Score: clearIt[float64](), PersonalNote: clearIt[string]()}})
	require.NoError(t, err)
	require.Empty(t, got)
	var count int64
	require.NoError(t, db.Table("session_marks").Where("session_id = ?", session.ID).Count(&count).Error)
	require.EqualValues(t, 0, count, "a both-NULL row must be deleted, not stored")
}

// Session write authorization against the real membership chain: the owner
// writes a member's note and marks center-wide (the remarks capability gates
// staff roles, never the owner); a fellow member gets 404; the owner's month
// read of the member's class works.
func TestSessionWritesFollowRemarksCapability(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	_, peer := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	testutil.JoinCenter(t, db, peer.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	peerScope := testutil.ScopeFor(t, db, peer.ID)

	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, member.ID)
	student := testutil.Student(t, db, member.ID, contact.ID)
	testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-08-01"))
	session := testutil.Session(t, db, member.ID, class.ID, date("2026-08-04"))

	_, err := svc.PutNote(ctx, memberScope, session.ID, teaching.PutNoteRequest{Body: "của giáo viên"})
	require.NoError(t, err)

	note, err := svc.PutNote(ctx, ownerScope, session.ID, teaching.PutNoteRequest{Body: "góp ý của chủ trung tâm"})
	require.NoError(t, err, "the owner writes a member's note center-wide")
	require.Equal(t, "góp ý của chủ trung tâm", note.Body)
	var noteAnchor string
	require.NoError(t, db.Raw(
		"SELECT teacher_id::text FROM session_notes WHERE session_id = ?", session.ID).Scan(&noteAnchor).Error)
	require.Equal(t, member.ID.String(), noteAnchor,
		"an owner edit keeps the session teacher's row anchor")
	_, err = svc.PutMarks(ctx, ownerScope, session.ID, []teaching.MarkEntryRequest{{StudentID: student.ID, Score: setTo(5.0)}})
	require.NoError(t, err, "the owner writes a member's marks center-wide")

	_, err = svc.PutNote(ctx, peerScope, session.ID, teaching.PutNoteRequest{Body: "x"})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not even resolve the session")

	got, err := svc.GetMonthMarks(ctx, ownerScope, class.ID, "2026-08")
	require.NoError(t, err, "the owner must read a member's month batch")
	require.Len(t, got.SessionNotes, 1)
}

// Re-saving the curriculum updates the one row per class in place — the
// UNIQUE (class_id) upsert, not a second row.
func TestCurriculumUpsertKeepsOneRowPerClass(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	_, err := svc.PutCurriculum(ctx, sc, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)
	got, err := svc.PutCurriculum(ctx, sc, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1", "Bài 2"}, CurrentIndex: 1})
	require.NoError(t, err)
	require.Equal(t, 1, got.CurrentIndex)

	var count int64
	require.NoError(t, db.Table("class_curricula").Where("class_id = ?", class.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
