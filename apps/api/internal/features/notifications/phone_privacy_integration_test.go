//go:build integration

package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// The one phone rule on the notification ledger, plus the send gate: bulk
// sending is reports-oversight work (owner or reports.send holder) — a
// plain class teacher may read their own period's ledger but never trigger a
// send. Ledger rows carry the contact's phone only to owner/oversight or a
// caller holding an ACTIVE hoc_vu stint over one of the contact's actively
// enrolled students.
func TestLedgerPhoneAndSendGateFollowTheOnePhoneRule(t *testing.T) {
	t.Parallel()
	d := newDeps(t)
	db := d.db
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	hocVu, _ := testutil.Teacher(t, db)
	troGiang, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	center := ownerScope.CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	testutil.JoinCenter(t, db, memberB.ID, center)
	testutil.JoinCenter(t, db, hocVu.ID, center)
	testutil.JoinCenter(t, db, troGiang.ID, center)
	_, secretary := testutil.Secretary(t, db, center)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactPhone("+84905556666"))
	classStart := date("2026-03-01")
	class := testutil.Class(t, db, member.ID, testutil.WithClassName("LedgerPrivacyA"), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, member.ID, contact.ID, testutil.WithStudentFullName("Ledger-privacy-student"))
	enrollment := testutil.Enrollment(t, db, member.ID, student.ID, class.ID, classStart)
	sess := testutil.Session(t, db, member.ID, class.ID, classStart.AddDate(0, 0, 1),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sess.ID, student.ID, enrollment.ID)
	testutil.StaffAssignment(t, db, class, hocVu.ID, "hoc_vu")
	testutil.StaffAssignment(t, db, class, troGiang.ID, "tro_giang")

	period, err := d.billing.EnsurePeriod(ctx, memberScope, 2026, 3)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	// The send gate: the period's own teacher is not oversight, so even their
	// own period refuses a bulk send.
	_, err = d.notifications.BulkSend(ctx, memberScope, period.ID, notifications.BulkSendRequest{Purpose: "statement"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"bulk sending reports is oversight work, not the class teacher's")

	// Oversight sends; the manual channel queues without touching Zalo.
	sendResp, err := d.notifications.BulkSend(ctx, secScope, period.ID, notifications.BulkSendRequest{Purpose: "statement"})
	require.NoError(t, err)
	require.Equal(t, 1, sendResp.QueuedCount)

	// Owner and secretary read the ledger with the phone.
	ownerRows, err := d.notifications.List(ctx, ownerScope, period.ID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, ownerRows, 1)
	require.NotNil(t, ownerRows[0].Phone, "the owner sees every phone")
	require.Equal(t, "+84905556666", *ownerRows[0].Phone)

	secRows, err := d.notifications.List(ctx, secScope, period.ID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, secRows, 1)
	require.NotNil(t, secRows[0].Phone, "reports oversight sees every phone")

	// The period's own teacher sees their ledger rows — masked.
	memberRows, err := d.notifications.List(ctx, memberScope, period.ID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, memberRows, 1)
	require.Nil(t, memberRows[0].Phone, "a class teacher without hoc_vu must not see the contact's phone")

	// Out-of-reach members get an empty ledger, not an error: the listing is
	// center-scoped and simply matches none of their rows.
	hocVuRows, err := d.notifications.List(ctx, testutil.ScopeFor(t, db, hocVu.ID), period.ID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Empty(t, hocVuRows, "a hoc_vu stint alone opens no ledger rows on another teacher's period")
	troGiangRows, err := d.notifications.List(ctx, testutil.ScopeFor(t, db, troGiang.ID), period.ID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Empty(t, troGiangRows)

	// The row grant is the ONE rule: an active hoc_vu stint on another
	// teacher's class with this contact's student actively enrolled unlocks
	// the phone on the period teacher's own ledger read.
	classB := testutil.Class(t, db, memberB.ID, testutil.WithClassName("LedgerPrivacyB"), testutil.WithClassStartDate(classStart))
	testutil.Enrollment(t, db, memberB.ID, student.ID, classB.ID, classStart)
	testutil.StaffAssignment(t, db, classB, member.ID, "hoc_vu")

	memberRows, err = d.notifications.List(ctx, memberScope, period.ID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, memberRows, 1)
	require.NotNil(t, memberRows[0].Phone, "an active hoc_vu stint over the contact's student unlocks the phone")
	require.Equal(t, "+84905556666", *memberRows[0].Phone)
}
