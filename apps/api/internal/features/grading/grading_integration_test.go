//go:build integration

package grading_test

import (
	"context"
	"testing"
	"time"

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

// The owner's score-set lifecycle against real Postgres: create with
// components, list resolving names in position order, rename + whole-replace the
// component list, then soft-delete out of the live listing.
func TestScoreSetCRUDRoundTrip(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)

	created, err := svc.CreateSet(ctx, sc, grading.ScoreSetRequest{
		Name: "IELTS", Components: []string{"Listening", "Speaking"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Listening", "Speaking"}, created.Components)

	sets, err := svc.ListSets(ctx, sc)
	require.NoError(t, err)
	require.Len(t, sets, 1)
	require.Equal(t, "IELTS", sets[0].Name)
	require.Equal(t, []string{"Listening", "Speaking"}, sets[0].Components)

	_, err = svc.UpdateSet(ctx, sc, created.ID, grading.ScoreSetRequest{
		Name: "IELTS Academic", Components: []string{"Listening", "Speaking", "Reading"},
	})
	require.NoError(t, err)
	sets, err = svc.ListSets(ctx, sc)
	require.NoError(t, err)
	require.Len(t, sets, 1)
	require.Equal(t, "IELTS Academic", sets[0].Name)
	require.Equal(t, []string{"Listening", "Speaking", "Reading"}, sets[0].Components)

	require.NoError(t, svc.DeleteSet(ctx, sc, created.ID))
	sets, err = svc.ListSets(ctx, sc)
	require.NoError(t, err)
	require.Empty(t, sets, "a soft-deleted set drops out of the live listing")
}

// A duplicate live name in the same center is a 409; the same name is free
// again after the first is soft-deleted.
func TestScoreSetDuplicateName(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)

	first, err := svc.CreateSet(ctx, sc, grading.ScoreSetRequest{Name: "TOEIC", Components: []string{"Reading"}})
	require.NoError(t, err)
	_, err = svc.CreateSet(ctx, sc, grading.ScoreSetRequest{Name: "TOEIC", Components: []string{"Listening"}})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	require.NoError(t, svc.DeleteSet(ctx, sc, first.ID))
	_, err = svc.CreateSet(ctx, sc, grading.ScoreSetRequest{Name: "TOEIC", Components: []string{"Listening"}})
	require.NoError(t, err, "the name frees up once the first set is soft-deleted")
}

// Every score-set surface refuses a plain member with 403 against the real
// membership chain.
func TestScoreSetsAreOwnerOnly(t *testing.T) {
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

	set, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "IELTS", Components: []string{"Listening"}})
	require.NoError(t, err)
	class := testutil.Class(t, db, member.ID)

	forbidden := func(err error) {
		t.Helper()
		require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	}

	_, err = svc.ListSets(ctx, memberScope)
	forbidden(err)
	_, err = svc.CreateSet(ctx, memberScope, grading.ScoreSetRequest{Name: "x", Components: []string{"a"}})
	forbidden(err)
	_, err = svc.UpdateSet(ctx, memberScope, set.ID, grading.ScoreSetRequest{Name: "x", Components: []string{"a"}})
	forbidden(err)
	err = svc.DeleteSet(ctx, memberScope, set.ID)
	forbidden(err)
	_, err = svc.AssignScoreSet(ctx, memberScope, class.ID, set.ID)
	forbidden(err)
	err = svc.ClearScoreSet(ctx, memberScope, class.ID)
	forbidden(err)
}

// AC3 — the per-class snapshot is independent of its source set: assigning
// copies the components with fresh ids, and later editing the source set leaves
// the assigned class untouched.
func TestAssignSnapshotIsIndependentOfSource(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)

	set, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "IELTS", Components: []string{"Listening", "Speaking"}})
	require.NoError(t, err)
	class := testutil.Class(t, db, owner.ID)

	assigned, err := svc.AssignScoreSet(ctx, ownerScope, class.ID, set.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Listening", "Speaking"}, names(assigned.Components))
	for _, comp := range assigned.Components {
		require.NotEqual(t, set.ID, comp.ID, "a snapshot component gets its own id, not the template's")
	}

	// Edit the source set: rename and swap the whole component list.
	_, err = svc.UpdateSet(ctx, ownerScope, set.ID, grading.ScoreSetRequest{
		Name: "IELTS v2", Components: []string{"Reading", "Writing", "Grammar"},
	})
	require.NoError(t, err)

	got, err := svc.GetClassComponents(ctx, ownerScope, class.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Listening", "Speaking"}, names(got.Components),
		"the class snapshot must not follow edits to the source set")
}

// A class that already carries any recorded score refuses re-assign and clear
// with 409 — replacing the components would cascade-delete the grades.
func TestAssignAndClearRefusedWhenClassHasScores(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	set, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "IELTS", Components: []string{"Listening"}})
	require.NoError(t, err)
	other, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "TOEIC", Components: []string{"Reading"}})
	require.NoError(t, err)

	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, member.ID)
	student := testutil.Student(t, db, member.ID, contact.ID)
	testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-08-01"))
	session := testutil.Session(t, db, member.ID, class.ID, date("2026-08-04"))

	assigned, err := svc.AssignScoreSet(ctx, ownerScope, class.ID, set.ID)
	require.NoError(t, err)
	comp := assigned.Components[0].ID

	// The class's teacher records one score — now the class is "scored".
	_, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: comp, Score: fptr(7.5)},
	})
	require.NoError(t, err)

	_, err = svc.AssignScoreSet(ctx, ownerScope, class.ID, other.ID)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code, "re-assign over recorded scores must 409")
	err = svc.ClearScoreSet(ctx, ownerScope, class.ID)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code, "clear over recorded scores must 409")
}

// Assigning a soft-deleted set is a 404 — the set no longer resolves in the
// center.
func TestAssignSoftDeletedSetIsNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)

	set, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "IELTS", Components: []string{"Listening"}})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteSet(ctx, ownerScope, set.ID))
	class := testutil.Class(t, db, owner.ID)

	_, err = svc.AssignScoreSet(ctx, ownerScope, class.ID, set.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
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

	set, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "IELTS", Components: []string{"Listening", "Speaking"}})
	require.NoError(t, err)

	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, member.ID)
	student := testutil.Student(t, db, member.ID, contact.ID)
	testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-08-01"))
	session := testutil.Session(t, db, member.ID, class.ID, date("2026-08-04"))

	assigned, err := svc.AssignScoreSet(ctx, ownerScope, class.ID, set.ID)
	require.NoError(t, err)
	listening := assigned.Components[0].ID
	speaking := assigned.Components[1].ID

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
	otherAssigned, err := svc.AssignScoreSet(ctx, ownerScope, otherClass.ID, set.ID)
	require.NoError(t, err)
	_, err = svc.PutSessionScores(ctx, memberScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: otherAssigned.Components[0].ID, Score: fptr(5)},
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

	set, err := svc.CreateSet(ctx, ownerScope, grading.ScoreSetRequest{Name: "IELTS", Components: []string{"Listening"}})
	require.NoError(t, err)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-08-01")))
	contact := testutil.Contact(t, db, teacher.ID)
	outsider := testutil.Student(t, db, teacher.ID, contact.ID)
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-08-04"))
	assigned, err := svc.AssignScoreSet(ctx, ownerScope, class.ID, set.ID)
	require.NoError(t, err)
	comp := assigned.Components[0].ID

	_, err = svc.PutSessionScores(ctx, ownerScope, session.ID, []grading.ScoreEntryRequest{
		{StudentID: outsider.ID, ComponentID: comp, Score: fptr(5)},
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "a never-enrolled student must be refused a new cell")
}
