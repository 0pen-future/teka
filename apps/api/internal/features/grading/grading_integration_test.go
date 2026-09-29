//go:build integration

package grading_test

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
	"teka/apps/api/internal/features/grading"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// newIntegrationService wires the real dependency chain router.go uses: grading
// resolves classes and sessions (resolution = its read/authz gate) and consults
// enrollments for the score roster check, all through consumer interfaces.
func newIntegrationService(t *testing.T) (*grading.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	svc := grading.NewService(grading.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)
	return svc, db
}

func date(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func fptr(v float64) *float64 { return &v }

func names(comps []grading.ClassComponentResponse) []string {
	out := make([]string, len(comps))
	for i, c := range comps {
		out[i] = c.Name
	}
	return out
}

// applyComponents gives a class a one-group snapshot through the template
// copy path and returns the snapshot's components, position order.
func applyComponents(t *testing.T, svc *grading.Service, sc authctx.Scope, classID uuid.UUID, labels ...string) []grading.ClassComponentResponse {
	t.Helper()
	ctx := context.Background()
	outcome, err := svc.SyncTemplateComponents(ctx, sc, classID, []grading.TemplateScoreGroup{{Title: "Bộ điểm", Labels: labels}})
	require.NoError(t, err)
	require.Equal(t, grading.SnapshotReplaced, outcome)
	got, err := svc.GetClassComponents(ctx, sc, classID)
	require.NoError(t, err)
	return got.Components
}

// The template copy replaces a class's snapshot only while the class has no
// score: a repeat is a no-op, a recorded score freezes the snapshot and the
// grade (replacing would cascade-delete it), and an empty template keeps
// whatever the class has.
func TestSyncTemplateComponentsReplacesUntilScored(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, member.ID)
	student := testutil.Student(t, db, member.ID, contact.ID)
	testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-08-01"))
	session := testutil.Session(t, db, member.ID, class.ID, date("2026-08-04"))

	ielts := []grading.TemplateScoreGroup{
		{Title: "Giữa kỳ", Labels: []string{"Nghe", "Nói"}},
		{Title: "Cuối kỳ", Labels: []string{"Viết"}},
	}
	outcome, err := svc.SyncTemplateComponents(ctx, ownerScope, class.ID, ielts)
	require.NoError(t, err)
	require.Equal(t, grading.SnapshotReplaced, outcome)
	got, err := svc.GetClassComponents(ctx, ownerScope, class.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Giữa kỳ · Nghe", "Giữa kỳ · Nói", "Cuối kỳ · Viết"}, names(got.Components))
	for i, comp := range got.Components {
		require.EqualValues(t, i, comp.Position)
	}

	outcome, err = svc.SyncTemplateComponents(ctx, ownerScope, class.ID, ielts)
	require.NoError(t, err)
	require.Equal(t, grading.SnapshotUnchanged, outcome)
	again, err := svc.GetClassComponents(ctx, ownerScope, class.ID)
	require.NoError(t, err)
	require.Equal(t, got.Components, again.Components, "an identical template keeps the same component ids")

	// The class's teacher records one score — now the class is "scored".
	_, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: got.Components[0].ID, Score: fptr(7.5)},
	})
	require.NoError(t, err)

	outcome, err = svc.SyncTemplateComponents(ctx, ownerScope, class.ID, []grading.TemplateScoreGroup{
		{Title: "TOEIC", Labels: []string{"Reading"}},
	})
	require.NoError(t, err)
	require.Equal(t, grading.SnapshotKeptScored, outcome)
	kept, err := svc.GetClassComponents(ctx, ownerScope, class.ID)
	require.NoError(t, err)
	require.Equal(t, got.Components, kept.Components, "a scored class keeps its snapshot")
	grid, err := svc.GetSessionScores(ctx, ownerScope, session.ID)
	require.NoError(t, err)
	require.Len(t, grid.Scores, 1, "the recorded grade survives")
	require.Equal(t, 7.5, grid.Scores[0].Score)

	empty := testutil.Class(t, db, member.ID)
	applyComponents(t, svc, ownerScope, empty.ID, "Chuyên cần")
	outcome, err = svc.SyncTemplateComponents(ctx, ownerScope, empty.ID, []grading.TemplateScoreGroup{{Title: "Trống"}})
	require.NoError(t, err)
	require.Equal(t, grading.SnapshotKeptEmptyTemplate, outcome)
	unchanged, err := svc.GetClassComponents(ctx, ownerScope, empty.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Chuyên cần"}, names(unchanged.Components), "an empty template keeps the class's snapshot")

	_, err = svc.SyncTemplateComponents(ctx, memberScope, class.ID, ielts)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code, "only the owner changes a class's components")
}

// The score write path against real rows: the session's teacher writes, edits,
// and clears (null) cells; the owner may also write (deliberate divergence from
// teaching.PutMarks); a peer member cannot even resolve the session (404); a
// component from another class and an out-of-range score are both 422.
func TestSessionScoreWriteAuthorizationAndValidation(t *testing.T) {
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

	components := applyComponents(t, svc, ownerScope, class.ID, "Listening", "Speaking")
	listening := components[0].ID
	speaking := components[1].ID

	// The session's teacher writes a cell.
	got, err := svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: listening, Score: fptr(6.5)},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 6.5, got[0].Score)

	// The teacher edits the same cell in place (upsert, not a second row).
	got, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: listening, Score: fptr(7.0)},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 7.0, got[0].Score)

	// The owner writes on the teacher's class — the divergence from teaching.
	got, err = svc.PutSessionScores(ctx, ownerScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: speaking, Score: fptr(8.0)},
	})
	require.NoError(t, err, "the owner must be allowed to record component scores")
	require.Len(t, got, 2)

	// The teacher clears a cell with null — the row is deleted.
	got, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: listening, Score: nil},
	})
	require.NoError(t, err)
	require.Len(t, got, 1, "the null cell must be deleted, leaving only the speaking score")
	var count int64
	require.NoError(t, db.Table("student_scores").Where("session_id = ? AND component_id = ?", session.ID, listening).Count(&count).Error)
	require.EqualValues(t, 0, count)

	// A peer member cannot resolve the session at all — 404, existence hidden.
	_, err = svc.GetSessionScores(ctx, peerScope, session.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.PutSessionScores(ctx, peerScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: speaking, Score: fptr(5)},
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// A component from a different class is refused as a validation error.
	otherClass := testutil.Class(t, db, member.ID)
	otherComponents := applyComponents(t, svc, ownerScope, otherClass.ID, "Listening", "Speaking")
	_, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: otherComponents[0].ID, Score: fptr(5)},
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "a component of another class must be rejected")

	// An out-of-range score is a validation error end-to-end.
	_, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: speaking, Score: fptr(10.5)},
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
}

// A brand-new cell for a student who was never on the session roster is
// refused; but a student already carrying a recorded score stays
// correctable/clearable after their enrollment ends.
func TestSessionScoreRosterGate(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, teacher.ID)

	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, teacher.ID)
	outsider := testutil.Student(t, db, teacher.ID, contact.ID)
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-08-04"))
	comp := applyComponents(t, svc, ownerScope, class.ID, "Listening")[0].ID

	_, err := svc.PutSessionScores(ctx, ownerScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: outsider.ID, ComponentID: comp, Score: fptr(5)},
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "a never-enrolled student must be refused a new cell")
}
