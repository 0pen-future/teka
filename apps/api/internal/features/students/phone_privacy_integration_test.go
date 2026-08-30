//go:build integration

package students_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestPhonePrivacyAcrossRoles pins the one phone rule on the students surface:
// the contact's phone reaches the owner, a reports-oversight secretary, and a
// hoc_vu holder with an ACTIVE stint on a class the student is actively
// enrolled in — nobody else, not even the class's giao_vien. The masked form
// is a nil pointer (JSON null), never an empty string.
func TestPhonePrivacyAcrossRoles(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	_, hocVu := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, hocVu.ID, scOwner.CenterID)
	_, troGiang := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, troGiang.ID, scOwner.CenterID)
	// The secretary holds reports oversight but only a tro_giang stint on the
	// class: the stint is what lets them reach the row at all (row access is
	// P2 scoping, not the phone rule), and oversight is what unmasks the phone
	// even though tro_giang alone never would.
	_, secretary := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, secretary.ID, scOwner.CenterID)
	testutil.GrantSendReports(t, db, secretary.ID, true)

	class := testutil.Class(t, db, gv.ID)
	contact := testutil.Contact(t, db, gv.ID, testutil.WithContactPhone("+84911222333"))
	student := testutil.Student(t, db, gv.ID, contact.ID)
	testutil.Enrollment(t, db, gv.ID, student.ID, class.ID,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	hocVuStint := testutil.StaffAssignment(t, db, class, hocVu.ID, authctx.StaffRoleHocVu)
	testutil.StaffAssignment(t, db, class, troGiang.ID, authctx.StaffRoleTroGiang)
	testutil.StaffAssignment(t, db, class, secretary.ID, authctx.StaffRoleTroGiang)

	phoneOf := func(sc authctx.Scope) *string {
		t.Helper()
		row, err := svc.Get(ctx, sc, student.ID)
		require.NoError(t, err)
		listed, _, err := svc.List(ctx, sc, students.ListFilter{ClassID: class.ID}, listParams(t))
		require.NoError(t, err)
		require.Len(t, listed, 1)
		require.Equal(t, row.ContactPhone, listed[0].ContactPhone,
			"list and detail must agree — one rule, every surface")
		return row.ContactPhone
	}

	scGv := testutil.ScopeFor(t, db, gv.ID)
	scHocVu := testutil.ScopeFor(t, db, hocVu.ID)
	scTroGiang := testutil.ScopeFor(t, db, troGiang.ID)
	scSecretary := testutil.ScopeFor(t, db, secretary.ID)

	require.NotNil(t, phoneOf(scOwner), "owner always sees the phone")
	require.Equal(t, "+84911222333", *phoneOf(scOwner))
	require.NotNil(t, phoneOf(scSecretary), "reports oversight sees the phone")
	require.NotNil(t, phoneOf(scHocVu), "active hoc_vu on the class sees the phone")
	require.Nil(t, phoneOf(scGv), "the class's giao_vien creator does not see the phone")
	require.Nil(t, phoneOf(scTroGiang), "tro_giang does not see the phone")

	// Ending the hoc_vu stint keeps the read (R4.1) but drops the phone.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE id = ?", hocVuStint).Error)
	scHocVu = testutil.ScopeFor(t, db, hocVu.ID)
	row, err := svc.Get(ctx, scHocVu, student.ID)
	require.NoError(t, err, "ended stint still reads the student")
	require.Nil(t, row.ContactPhone, "ended stint no longer carries the phone")
}
