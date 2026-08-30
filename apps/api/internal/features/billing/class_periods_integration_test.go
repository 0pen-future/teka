//go:build integration

package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// The class-filtered period listing is how class staff discover which billing
// periods carry their class's charges (the send entry point): any stint on the
// class — ended or read-only included — opens the list, no stint gets the
// neutral 404, and the list contains exactly the periods holding invoice
// lines billed to that class's enrollments.
func TestListPeriodsByClassFollowsClassStints(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	center := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	// Class JAN bills in January only; class FEB in February only — so each
	// class maps to exactly one of the member's two periods.
	contactJan := testutil.Contact(t, db, member.ID, testutil.WithContactFullName("JanContact"))
	classJan := testutil.Class(t, db, member.ID, testutil.WithClassName("JanClass"), testutil.WithClassStartDate(date("2026-01-01")))
	studentJan := testutil.Student(t, db, member.ID, contactJan.ID)
	enrollJan := testutil.Enrollment(t, db, member.ID, studentJan.ID, classJan.ID, date("2026-01-01"))
	sessJan := testutil.Session(t, db, member.ID, classJan.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sessJan.ID, studentJan.ID, enrollJan.ID)

	contactFeb := testutil.Contact(t, db, member.ID, testutil.WithContactFullName("FebContact"))
	classFeb := testutil.Class(t, db, member.ID, testutil.WithClassName("FebClass"), testutil.WithClassStartDate(date("2026-02-01")))
	studentFeb := testutil.Student(t, db, member.ID, contactFeb.ID)
	enrollFeb := testutil.Enrollment(t, db, member.ID, studentFeb.ID, classFeb.ID, date("2026-02-01"))
	sessFeb := testutil.Session(t, db, member.ID, classFeb.ID, date("2026-02-03"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sessFeb.ID, studentFeb.ID, enrollFeb.ID)

	periodJan, err := svc.EnsurePeriod(ctx, memberScope, 2026, 1)
	require.NoError(t, err)
	_, err = svc.Close(ctx, memberScope, periodJan.ID)
	require.NoError(t, err)
	periodFeb, err := svc.EnsurePeriod(ctx, memberScope, 2026, 2)
	require.NoError(t, err)
	_, err = svc.Close(ctx, memberScope, periodFeb.ID)
	require.NoError(t, err)

	hocVu, _ := testutil.Teacher(t, db)
	troGiang, _ := testutil.Teacher(t, db)
	outsider, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, hocVu.ID, center)
	testutil.JoinCenter(t, db, troGiang.ID, center)
	testutil.JoinCenter(t, db, outsider.ID, center)
	testutil.StaffAssignment(t, db, classJan, hocVu.ID, "hoc_vu")
	testutil.StaffAssignment(t, db, classJan, troGiang.ID, "tro_giang")

	page := pagination.Params{Page: 1, PerPage: 20}

	// The hoc_vu sees exactly the period carrying their class's lines, with
	// the owning teacher's name attached for display.
	rows, total, err := svc.ListPeriodsClass(ctx, testutil.ScopeFor(t, db, hocVu.ID), classJan.ID, page)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, periodJan.ID, rows[0].ID)
	require.NotEmpty(t, rows[0].TeacherName)

	// The same caller has no stint on the February class: neutral 404.
	_, _, err = svc.ListPeriodsClass(ctx, testutil.ScopeFor(t, db, hocVu.ID), classFeb.ID, page)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Any stint opens the read — tro_giang included.
	_, total, err = svc.ListPeriodsClass(ctx, testutil.ScopeFor(t, db, troGiang.ID), classJan.ID, page)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)

	// A member with no stint gets the neutral 404; the owner passes without
	// any stint at all.
	_, _, err = svc.ListPeriodsClass(ctx, testutil.ScopeFor(t, db, outsider.ID), classJan.ID, page)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	rows, total, err = svc.ListPeriodsClass(ctx, testutil.ScopeFor(t, db, owner.ID), classJan.ID, page)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, periodJan.ID, rows[0].ID)
}

// History follows the person: an ENDED stint keeps the class-period read open
// — the handed-off teacher included — while a class outside the caller's
// center answers the same neutral 404 a nonexistent id does, and a period
// whose class charges were all voided drops out of the list (nothing left to
// send).
func TestListPeriodsByClassEndedStintsForeignClassesAndVoids(t *testing.T) {
	t.Parallel()
	svc, _, _, _, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	_, member := testutil.Teacher(t, db)
	center := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	contact := testutil.Contact(t, db, member.ID)
	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-01-01")))
	student := testutil.Student(t, db, member.ID, contact.ID)
	enroll := testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-01-01"))
	sess := testutil.Session(t, db, member.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sess.ID, student.ID, enroll.ID)

	period, err := svc.EnsurePeriod(ctx, memberScope, 2026, 1)
	require.NoError(t, err)
	_, err = svc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	page := pagination.Params{Page: 1, PerPage: 20}

	// A hoc_vu whose stint has since ended still discovers the period.
	hocVu, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, hocVu.ID, center)
	testutil.StaffAssignment(t, db, class, hocVu.ID, "hoc_vu")
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, hocVu.ID).Error)
	rows, total, err := svc.ListPeriodsClass(ctx, testutil.ScopeFor(t, db, hocVu.ID), class.ID, page)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, period.ID, rows[0].ID)

	// Hand the class to the owner: the previous teacher's giao_vien stint
	// ends, yet the period their lines live under stays discoverable to them.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ? AND ended_at IS NULL",
		class.ID, member.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", owner.ID, class.ID).Error)
	rows, total, err = svc.ListPeriodsClass(ctx, memberScope, class.ID, page)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, period.ID, rows[0].ID)

	// A made-up id and another center's class both answer the neutral 404 —
	// no probing which classes exist elsewhere.
	_, _, err = svc.ListPeriodsClass(ctx, memberScope, id.New(), page)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, foreignOwner := testutil.Teacher(t, db)
	foreignClass := testutil.Class(t, db, foreignOwner.ID)
	_, _, err = svc.ListPeriodsClass(ctx, memberScope, foreignClass.ID, page)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Void every invoice in the period: the class still reads, but the period
	// no longer carries live class charges and drops out of the list.
	require.NoError(t, db.Exec(
		"UPDATE invoices SET status = 'void', voided_at = now(), void_reason = 'khong con hieu luc' WHERE period_id = ?",
		period.ID).Error)
	rows, total, err = svc.ListPeriodsClass(ctx, memberScope, class.ID, page)
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	require.Empty(t, rows)
}
