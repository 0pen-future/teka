//go:build integration

package students_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestStaffAssignmentWidensStudentReads pins the read-port contract on
// students: a hoc_vu/tro_giang stint (ended included) reaches the students
// enrolled in the class — through the enrollment join — while the update and
// delete writes keep their own-rows gate.
func TestStaffAssignmentWidensStudentReads(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	_, staff := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, staff.ID, scOwner.CenterID)

	class := testutil.Class(t, db, gv.ID)
	contact := testutil.Contact(t, db, gv.ID)
	student := testutil.Student(t, db, gv.ID, contact.ID)
	testutil.Enrollment(t, db, gv.ID, student.ID, class.ID,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	testutil.StaffAssignment(t, db, class, staff.ID, authctx.StaffRoleTroGiang)

	scStaff := testutil.ScopeFor(t, db, staff.ID)

	got, err := svc.Get(ctx, scStaff, student.ID)
	require.NoError(t, err)
	require.Equal(t, student.ID, got.ID)
	rows, total, err := svc.List(ctx, scStaff, students.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, student.ID, rows[0].ID)

	// Write-freeze: student CRUD is the owner's alone — every member write is
	// an honest 403, stint or no stint.
	_, err = svc.Update(ctx, scStaff, student.ID,
		students.UpdateRequest{FullName: "Bé An (edited)", ContactID: contact.ID})
	require.Equal(t, 403, apperror.From(err).Status)
	require.Equal(t, 403, apperror.From(svc.Delete(ctx, scStaff, student.ID)).Status)

	// An ended stint keeps history readable.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, staff.ID).Error)
	_, err = svc.Get(ctx, scStaff, student.ID)
	require.NoError(t, err, "an ended stint keeps history readable")

	// A soft-deleted class grants nothing, stint or not.
	require.NoError(t, db.Exec(
		"UPDATE classes SET deleted_at = now() WHERE id = ?", class.ID).Error)
	_, err = svc.Get(ctx, scStaff, student.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}
