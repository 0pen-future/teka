//go:build integration

package classscope_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/classscope"
	"teka/apps/api/internal/testutil"
)

// Runs the fragments against a real schema: an ended stint still reads (R4.1),
// no stint reads nothing, and a soft-deleted class grants nothing even to an
// assignment holder.
func TestFragmentsAgainstDatabase(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)

	_, owner := testutil.Teacher(t, db)
	ownerSc := testutil.ScopeFor(t, db, owner.ID)
	_, member := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, ownerSc.CenterID)
	_, outsider := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, outsider.ID, ownerSc.CenterID)

	class := testutil.Class(t, db, owner.ID)
	contact := testutil.Contact(t, db, owner.ID)
	student := testutil.Student(t, db, owner.ID, contact.ID)
	testutil.Enrollment(t, db, owner.ID, student.ID, class.ID, class.StartDate)

	stintID := testutil.StaffAssignment(t, db, class, member.ID, "tro_giang")

	countClasses := func(teacherID uuid.UUID) int64 {
		frag, _ := classscope.ReadExists("classes.id")
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM classes WHERE `+frag, teacherID, ownerSc.CenterID,
		).Scan(&n).Error)
		return n
	}
	countStudents := func(teacherID uuid.UUID) int64 {
		frag, _ := classscope.ReadExistsViaEnrollment("students.id")
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM students WHERE `+frag, teacherID, ownerSc.CenterID,
		).Scan(&n).Error)
		return n
	}

	require.EqualValues(t, 1, countClasses(member.ID), "active stint reads the class")
	require.EqualValues(t, 1, countStudents(member.ID), "active stint reads enrolled students")
	require.Zero(t, countClasses(outsider.ID), "no stint, no rows")
	require.Zero(t, countStudents(outsider.ID))

	require.NoError(t, db.Exec(
		`UPDATE class_staff SET ended_at = now() WHERE id = ?`, stintID).Error)
	require.EqualValues(t, 1, countClasses(member.ID), "an ended stint keeps history reads")
	require.EqualValues(t, 1, countStudents(member.ID))

	require.NoError(t, db.Exec(
		`UPDATE classes SET deleted_at = now() WHERE id = ?`, class.ID).Error)
	require.Zero(t, countClasses(member.ID), "a soft-deleted class grants nothing")
	require.Zero(t, countStudents(member.ID))
}

// The write fragment grants nothing to ended stints, wrong roles, or
// soft-deleted classes — only an ACTIVE stint whose role is in the bound
// slice writes.
func TestWriteExistsAgainstDatabase(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)

	_, owner := testutil.Teacher(t, db)
	ownerSc := testutil.ScopeFor(t, db, owner.ID)
	_, assistant := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, assistant.ID, ownerSc.CenterID)

	class := testutil.Class(t, db, owner.ID)
	stintID := testutil.StaffAssignment(t, db, class, assistant.ID, "tro_giang")

	countWritable := func(teacherID uuid.UUID, roles []string) int64 {
		frag, _ := classscope.WriteExists("classes.id")
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM classes WHERE `+frag,
			teacherID, ownerSc.CenterID, roles,
		).Scan(&n).Error)
		return n
	}

	attendanceRoles := []string{"giao_vien", "tro_giang"}
	scoreRoles := []string{"giao_vien"}

	require.EqualValues(t, 1, countWritable(assistant.ID, attendanceRoles),
		"active tro_giang stint writes attendance")
	require.Zero(t, countWritable(assistant.ID, scoreRoles),
		"role outside the capability's list writes nothing")

	require.NoError(t, db.Exec(
		`UPDATE class_staff SET ended_at = now() WHERE id = ?`, stintID).Error)
	require.Zero(t, countWritable(assistant.ID, attendanceRoles),
		"an ended stint keeps reads but loses writes")
	require.NoError(t, db.Exec(
		`UPDATE class_staff SET ended_at = NULL WHERE id = ?`, stintID).Error)

	require.NoError(t, db.Exec(
		`UPDATE classes SET deleted_at = now() WHERE id = ?`, class.ID).Error)
	require.Zero(t, countWritable(assistant.ID, attendanceRoles),
		"a soft-deleted class grants no writes")
}
