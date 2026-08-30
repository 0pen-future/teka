//go:build integration

package teaching_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestStaffAssignmentWidensTeachingReads pins the read-port contract on
// teaching: a hoc_vu/tro_giang stint (ended included) reads the curriculum,
// the plan list, and the month's notes and marks, while the content writes
// keep their class-teacher / session-teacher gates.
func TestStaffAssignmentWidensTeachingReads(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	_, staff := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, staff.ID, scOwner.CenterID)
	_, peer := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, peer.ID, scOwner.CenterID)

	class := testutil.Class(t, db, gv.ID)
	session := testutil.Session(t, db, gv.ID, class.ID, date("2026-01-06"))
	testutil.StaffAssignment(t, db, class, staff.ID, authctx.StaffRoleTroGiang)

	scGV := testutil.ScopeFor(t, db, gv.ID)
	scStaff := testutil.ScopeFor(t, db, staff.ID)
	scPeer := testutil.ScopeFor(t, db, peer.ID)

	// The giáo viên saves real content so the reads return something.
	_, err := svc.PutCurriculum(ctx, scGV, class.ID, teaching.PutCurriculumRequest{
		Lessons: []string{"Bài 1", "Bài 2"}, CurrentIndex: 1,
	})
	require.NoError(t, err)
	_, err = svc.PutNote(ctx, scGV, session.ID, teaching.PutNoteRequest{Body: "Lớp học tốt"})
	require.NoError(t, err)

	// The assigned member reads curriculum, plans, and the month view.
	cur, err := svc.GetCurriculum(ctx, scStaff, class.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"Bài 1", "Bài 2"}, cur.Lessons)
	_, err = svc.ListPlans(ctx, scStaff, class.ID)
	require.NoError(t, err)
	month, err := svc.GetMonthMarks(ctx, scStaff, class.ID, "2026-01")
	require.NoError(t, err)
	require.Len(t, month.SessionNotes, 1)

	// The unassigned peer keeps 404 — no existence leak.
	_, err = svc.GetCurriculum(ctx, scPeer, class.ID)
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.GetMonthMarks(ctx, scPeer, class.ID, "2026-01")
	require.Equal(t, 404, apperror.From(err).Status)

	// Write-freeze: the stint reaches no content write — the write capability
	// refuses the role, and a caller who can read gets an honest 403.
	_, err = svc.PutCurriculum(ctx, scStaff, class.ID, teaching.PutCurriculumRequest{
		Lessons: []string{"Bài sửa"},
	})
	require.Equal(t, 403, apperror.From(err).Status)
	_, err = svc.PutNote(ctx, scStaff, session.ID, teaching.PutNoteRequest{Body: "sửa trộm"})
	require.Equal(t, 403, apperror.From(err).Status)

	// An ended stint keeps history readable.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, staff.ID).Error)
	_, err = svc.GetCurriculum(ctx, scStaff, class.ID)
	require.NoError(t, err, "an ended stint keeps history readable")

	// A soft-deleted class grants nothing, stint or not.
	require.NoError(t, db.Exec(
		"UPDATE classes SET deleted_at = now() WHERE id = ?", class.ID).Error)
	_, err = svc.GetCurriculum(ctx, scStaff, class.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}

// Content and remarks writes follow the active giao_vien stint. Staff stints
// read (403 on write), an unassigned member gets 404, and a handoff moves the
// write right onto pre-handoff sessions while stripping the old teacher's —
// even though the note rows still carry the old anchor.
func TestContentWritesFollowStintThroughHandoff(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, oldGV := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, oldGV.ID, scOwner.CenterID)
	scOld := testutil.ScopeFor(t, db, oldGV.ID)
	_, newGV := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, newGV.ID, scOwner.CenterID)
	scNew := testutil.ScopeFor(t, db, newGV.ID)
	_, tg := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, tg.ID, scOwner.CenterID)
	scTG := testutil.ScopeFor(t, db, tg.ID)

	class := testutil.Class(t, db, oldGV.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.StaffAssignment(t, db, class, tg.ID, authctx.StaffRoleTroGiang)
	session := testutil.Session(t, db, oldGV.ID, class.ID, date("2026-01-06"))

	_, err := svc.PutCurriculum(ctx, scOld, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài 1"}})
	require.NoError(t, err)
	_, err = svc.PutNote(ctx, scOld, session.ID, teaching.PutNoteRequest{Body: "nhận xét đầu"})
	require.NoError(t, err)

	// The assistant reads but cannot write content or remarks.
	_, err = svc.PutCurriculum(ctx, scTG, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"x"}})
	require.Equal(t, 403, apperror.From(err).Status)
	_, err = svc.PutNote(ctx, scTG, session.ID, teaching.PutNoteRequest{Body: "x"})
	require.Equal(t, 403, apperror.From(err).Status)
	// A member with no stint resolves nothing.
	_, err = svc.PutNote(ctx, scNew, session.ID, teaching.PutNoteRequest{Body: "x"})
	require.Equal(t, 404, apperror.From(err).Status)

	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", newGV.ID, class.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND role_key = 'giao_vien' AND ended_at IS NULL",
		class.ID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO class_staff (class_id, center_id, teacher_id, role_key) VALUES (?, ?, ?, 'giao_vien')",
		class.ID, scOwner.CenterID, newGV.ID).Error)

	// The old GV keeps reads but loses every write — including on the note
	// row still anchored to them.
	_, err = svc.GetCurriculum(ctx, scOld, class.ID)
	require.NoError(t, err, "ended stint keeps history readable")
	_, err = svc.PutCurriculum(ctx, scOld, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"y"}})
	require.Equal(t, 403, apperror.From(err).Status)
	_, err = svc.PutNote(ctx, scOld, session.ID, teaching.PutNoteRequest{Body: "y"})
	require.Equal(t, 403, apperror.From(err).Status,
		"the old anchor on the note row must not readmit the old teacher")

	// The new GV writes the pre-handoff class and session.
	_, err = svc.PutCurriculum(ctx, scNew, class.ID, teaching.PutCurriculumRequest{Lessons: []string{"Bài mới"}})
	require.NoError(t, err)
	_, err = svc.PutNote(ctx, scNew, session.ID, teaching.PutNoteRequest{Body: "nhận xét mới"})
	require.NoError(t, err, "the new GV writes the pre-handoff session's note")
}
