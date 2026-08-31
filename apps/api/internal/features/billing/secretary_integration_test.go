//go:build integration

package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// A reports.send-holding member reads billing center-wide — any member's period,
// with the owning teacher's identity attached so the client can group by
// teacher — but the flag never relaxes a single billing WRITE: close, void,
// and adjustment on another member's data still answer the same neutral 404
// a stranger gets.
func TestSecretaryReadsCenterWideButCannotWriteBilling(t *testing.T) {
	t.Parallel()
	svc, repo, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	_, secretary := testutil.Secretary(t, db, ownerCenter)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	contact := testutil.Contact(t, db, member.ID)
	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, member.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-01-01"))
	sess := testutil.Session(t, db, member.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sess.ID, student.ID, enrollment.ID)

	period, err := svc.EnsurePeriod(ctx, memberScope, 2026, 1)
	require.NoError(t, err)

	// Center-wide read, with the owning teacher's identity for grouping.
	got, err := svc.GetPeriod(ctx, secScope, period.ID)
	require.NoError(t, err, "a send-reports holder must read any member's period in the center")
	require.Equal(t, period.ID, got.ID)
	require.Equal(t, member.ID, got.TeacherID)
	require.NotEmpty(t, got.TeacherName, "the read must carry the owning teacher's name")

	listed, _, err := svc.ListPeriods(ctx, secScope, pagination.Params{Page: 1, PerPage: 50})
	require.NoError(t, err)
	found := false
	for _, p := range listed {
		if p.ID == period.ID {
			found = true
			require.Equal(t, member.ID, p.TeacherID)
			require.NotEmpty(t, p.TeacherName)
		}
	}
	require.True(t, found, "the center-wide list must include the member's period")

	// Writes stay owner/self territory: the flag grants none of them.
	_, err = svc.Close(ctx, secScope, period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"a send-reports holder must not close another member's period")

	// Close it as the member so an invoice exists to aim void/adjustment at.
	closeResp, err := svc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, closeResp.IssuedCount)
	invoices, err := repo.ListInvoices(ctx, memberScope, period.ID)
	require.NoError(t, err)
	require.Len(t, invoices, 1)

	_, err = svc.VoidInvoice(ctx, secScope, invoices[0].ID, "secretary should not be able to do this")
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"a send-reports holder must not void another member's invoice")

	_, _, err = svc.AddAdjustment(ctx, secScope, invoices[0].ID, -50000, "secretary should not be able to do this")
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"a send-reports holder must not adjust another member's invoice")
}
