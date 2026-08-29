//go:build integration

package attendance_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// The send-reports flag grants billing/statement/debt READS only — it never
// opens attendance. A flag holder touching another member's session gets the
// same neutral not-found a plain peer gets, on the read and on the confirm.
func TestSecretaryCannotReadOrConfirmMembersAttendance(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	_, secretary := testutil.Secretary(t, db, ownerCenter)
	secScope := testutil.ScopeFor(t, db, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	contact := testutil.Contact(t, db, member.ID)
	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, member.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, member.ID, contact.ID)
	testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-01-01"))

	_, err := svc.Get(ctx, secScope, session.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not open another member's attendance sheet")

	_, err = svc.Confirm(ctx, secScope, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not let anyone confirm another member's attendance")

	var recordCount int64
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ?", session.ID).Count(&recordCount).Error)
	require.Zero(t, recordCount, "the refused confirm must not have written any records")
}
