//go:build integration

package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// classWithBilledContact seeds one explicit class under teacherID with one
// contact whose child attended a single confirmed June-2026 session
// (100 000đ), returning the class and the contact id. Unlike seedChild the
// class is handed back, so class-scoped send tests can grant stints on it and
// address it by id. The caller closes the period once, after seeding every
// class it needs.
func classWithBilledContact(t *testing.T, d *deps, teacherID uuid.UUID, name string) (*classes.Class, uuid.UUID) {
	t.Helper()
	classStart := date("2026-06-01")
	contact := testutil.Contact(t, d.db, teacherID)
	class := testutil.Class(t, d.db, teacherID, testutil.WithClassName(name), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, d.db, teacherID, contact.ID, testutil.WithStudentFullName(name+"-student"))
	enrollment := testutil.Enrollment(t, d.db, teacherID, student.ID, class.ID, classStart)
	sess := testutil.Session(t, d.db, teacherID, class.ID, classStart.AddDate(0, 0, 1),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, d.db, teacherID, sess.ID, student.ID, enrollment.ID)
	return class, contact.ID
}

func closePeriod(t *testing.T, d *deps, teacherID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	sc := testutil.ScopeFor(t, d.db, teacherID)
	period, err := d.billing.EnsurePeriod(ctx, sc, 2026, 6)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, sc, period.ID)
	require.NoError(t, err)
	return period.ID
}

func runClassID(t *testing.T, d *deps, runID uuid.UUID) *uuid.UUID {
	t.Helper()
	var row struct{ ClassID *uuid.UUID }
	require.NoError(t, d.db.Table("notification_runs").Select("class_id").
		Where("id = ?", runID).Take(&row).Error)
	return row.ClassID
}

// A hoc_vu's class-scoped bulk send queues personal DMs carrying CLASS
// statement copies, runs them on the hoc_vu's own Zalo session, and records
// the run against the class dimension.
func TestClassBulkSendDeliversClassCopiesOnTheStaffSession(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()

	_, teacher := testutil.Teacher(t, d.db)
	class, contactID := classWithBilledContact(t, d, teacher.ID, "ClassSendA")
	periodID := closePeriod(t, d, teacher.ID)
	mapContact(t, d.db, contactID, "uid-class-a")

	hocVu, _ := testutil.Teacher(t, d.db)
	testutil.JoinCenter(t, d.db, hocVu.ID, testutil.ScopeFor(t, d.db, teacher.ID).CenterID)
	testutil.StaffAssignment(t, d.db, class, hocVu.ID, "hoc_vu")
	hocVuScope := testutil.ScopeFor(t, d.db, hocVu.ID)

	resp, err := d.notifications.BulkSend(ctx, hocVuScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
		ClassID: &class.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, resp.QueuedCount)
	require.Equal(t, 1, resp.PersonalQueuedCount)
	require.Len(t, resp.Rows, 1)
	require.Equal(t, notifications.ChannelZaloPersonal, resp.Rows[0].Channel)
	require.NotNil(t, resp.RunID)

	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))
	gotClassID := runClassID(t, d, *resp.RunID)
	require.NotNil(t, gotClassID, "a class send's run lives on the class dimension")
	require.Equal(t, class.ID, *gotClassID)

	uids, texts := fake.sent()
	require.Equal(t, []string{"uid-class-a"}, uids)
	require.Equal(t, []string{resp.Rows[0].MessageText}, texts)
	require.Equal(t, []uuid.UUID{hocVu.ID}, fake.sentBy(),
		"the DM goes out on the sending staff's own Zalo session")

	// The queued notification references a CLASS statement copy, never the
	// family statement.
	var stmtClassIDs []*uuid.UUID
	require.NoError(t, d.db.Table("notifications AS n").
		Joins("JOIN statements s ON s.id = n.statement_id").
		Where("n.run_id = ?", *resp.RunID).
		Pluck("s.class_id", &stmtClassIDs).Error)
	require.Len(t, stmtClassIDs, 1)
	require.NotNil(t, stmtClassIDs[0])
	require.Equal(t, class.ID, *stmtClassIDs[0])
}

// Two different classes' sends over the same period never contend: the
// second class's run starts and finishes while the first is still mid-flight.
func TestTwoClassRunsOverTheSamePeriodPaceInParallel(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	fake := &fakeZaloSender{}
	fake.send = func(_ int, toUID string) (string, error) {
		if toUID == "uid-par-a" {
			<-release
		}
		return "msg-ok", nil
	}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()

	_, teacher := testutil.Teacher(t, d.db)
	classA, contactA := classWithBilledContact(t, d, teacher.ID, "ParClassA")
	classB, contactB := classWithBilledContact(t, d, teacher.ID, "ParClassB")
	periodID := closePeriod(t, d, teacher.ID)
	mapContact(t, d.db, contactA, "uid-par-a")
	mapContact(t, d.db, contactB, "uid-par-b")

	center := testutil.ScopeFor(t, d.db, teacher.ID).CenterID
	hocVuA, _ := testutil.Teacher(t, d.db)
	hocVuB, _ := testutil.Teacher(t, d.db)
	testutil.JoinCenter(t, d.db, hocVuA.ID, center)
	testutil.JoinCenter(t, d.db, hocVuB.ID, center)
	testutil.StaffAssignment(t, d.db, classA, hocVuA.ID, "hoc_vu")
	testutil.StaffAssignment(t, d.db, classB, hocVuB.ID, "hoc_vu")

	respA, err := d.notifications.BulkSend(ctx, testutil.ScopeFor(t, d.db, hocVuA.ID), periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
		ClassID: &classA.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, respA.RunID)

	// Class A's run is now blocked inside its one DM. Class B's send must
	// still go through — the running-run guard is per class, not per period.
	respB, err := d.notifications.BulkSend(ctx, testutil.ScopeFor(t, d.db, hocVuB.ID), periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
		ClassID: &classB.ID,
	})
	require.NoError(t, err, "a second class's run must not conflict with the first's")
	require.NotNil(t, respB.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *respB.RunID))
	require.Equal(t, notifications.RunStatusRunning, runStatusOf(t, d.db, *respA.RunID),
		"class A's run is still mid-flight while class B completed")

	close(release)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *respA.RunID))

	require.Equal(t, classA.ID, *runClassID(t, d, *respA.RunID))
	require.Equal(t, classB.ID, *runClassID(t, d, *respB.RunID))
}

// Family and class sends over the same period stay possible in both orders,
// but the second dimension's response carries the double-billing-notice
// warning; the first send of each dimension carries none.
func TestOverlapWarningFiresOnTheSecondDimensionOnly(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()

	_, teacher := testutil.Teacher(t, d.db)
	class, contactID := classWithBilledContact(t, d, teacher.ID, "OverlapClass")
	periodID := closePeriod(t, d, teacher.ID)
	mapContact(t, d.db, contactID, "uid-overlap")
	sc := testutil.ScopeFor(t, d.db, teacher.ID)

	hocVu, _ := testutil.Teacher(t, d.db)
	testutil.JoinCenter(t, d.db, hocVu.ID, sc.CenterID)
	testutil.StaffAssignment(t, d.db, class, hocVu.ID, "hoc_vu")
	hocVuScope := testutil.ScopeFor(t, d.db, hocVu.ID)

	famResp, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.Nil(t, famResp.OverlapWarning, "nothing was sent on the class dimension yet")
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *famResp.RunID))

	classResp, err := d.notifications.BulkSend(ctx, hocVuScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
		ClassID: &class.ID,
	})
	require.NoError(t, err, "an earlier family send warns — it never blocks the class send")
	require.NotNil(t, classResp.OverlapWarning)
	require.Contains(t, *classResp.OverlapWarning, "family statements")
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *classResp.RunID))

	famAgain, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, famAgain.OverlapWarning, "the class copies already out warn the family sender back")
	require.Contains(t, *famAgain.OverlapWarning, "class statement copies")
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *famAgain.RunID))
}

// Polling a class run is class data behind the same class-send gate as the
// send itself: the repository's class-run lookup is center-scoped only, so
// without the gate any center member could watch another class's send
// progress. A read-only stint answers an honest 403; no stint, the neutral
// 404; the sender and the oversight owner both read the snapshot.
func TestClassRunSnapshotRequiresTheClassSendGate(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()

	_, teacher := testutil.Teacher(t, d.db)
	class, contactID := classWithBilledContact(t, d, teacher.ID, "ClassSnapshot")
	periodID := closePeriod(t, d, teacher.ID)
	mapContact(t, d.db, contactID, "uid-class-snapshot")
	center := testutil.ScopeFor(t, d.db, teacher.ID).CenterID

	hocVu, _ := testutil.Teacher(t, d.db)
	troGiang, _ := testutil.Teacher(t, d.db)
	outsider, _ := testutil.Teacher(t, d.db)
	testutil.JoinCenter(t, d.db, hocVu.ID, center)
	testutil.JoinCenter(t, d.db, troGiang.ID, center)
	testutil.JoinCenter(t, d.db, outsider.ID, center)
	testutil.StaffAssignment(t, d.db, class, hocVu.ID, "hoc_vu")
	testutil.StaffAssignment(t, d.db, class, troGiang.ID, "tro_giang")
	hocVuScope := testutil.ScopeFor(t, d.db, hocVu.ID)

	resp, err := d.notifications.BulkSend(ctx, hocVuScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
		ClassID: &class.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	snap, err := d.notifications.RunSnapshot(ctx, hocVuScope, periodID, &class.ID)
	require.NoError(t, err)
	require.Equal(t, resp.RunID, snap.RunID)

	snap, err = d.notifications.RunSnapshot(ctx, testutil.ScopeFor(t, d.db, teacher.ID), periodID, &class.ID)
	require.NoError(t, err, "oversight reads any class's run")
	require.Equal(t, resp.RunID, snap.RunID)

	_, err = d.notifications.RunSnapshot(ctx, testutil.ScopeFor(t, d.db, troGiang.ID), periodID, &class.ID)
	require.Equal(t, 403, apperror.From(err).Status)

	_, err = d.notifications.RunSnapshot(ctx, testutil.ScopeFor(t, d.db, outsider.ID), periodID, &class.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}
