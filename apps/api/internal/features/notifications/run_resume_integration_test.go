//go:build integration

package notifications_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// interruptibleZalo acks its first send instantly, then parks the second one
// until the run's own context is cancelled — the deterministic way to catch a
// run mid-flight so a shutdown strands its remaining rows.
type interruptibleZalo struct {
	mu      sync.Mutex
	calls   int
	blocked chan struct{} // closed when the second send is parked
}

func (f *interruptibleZalo) VerifyAccount(context.Context, uuid.UUID) error { return nil }

func (f *interruptibleZalo) SendDM(ctx context.Context, _ uuid.UUID, _, _ string) (string, error) {
	f.mu.Lock()
	call := f.calls
	f.calls++
	f.mu.Unlock()
	if call == 0 {
		return "msg-first", nil
	}
	close(f.blocked)
	<-ctx.Done()
	return "", ctx.Err()
}

// interruptedRunFixture drives a real run into the interrupted state: three
// mapped contacts, the first delivered, the second aborted mid-send by
// shutdown, the third never reached. Returns fresh deps (with sender) bound
// to the same database, plus the ids the assertions need.
type interruptedRunFixture struct {
	d         *deps
	sender    *fakeZaloSender
	teacherID uuid.UUID
	periodID  uuid.UUID
	runID     uuid.UUID
	contacts  []uuid.UUID
	rows      map[uuid.UUID]notifications.BulkSendRow // by contact
}

func newInterruptedRun(t *testing.T, sender *fakeZaloSender) interruptedRunFixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	blocking := &interruptibleZalo{blocked: make(chan struct{})}
	d1 := newDepsWithZalo(t, db, blocking)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)

	periodID, contacts := closedPeriodWithContacts(t, d1, teacher.ID, 3)
	mapContact(t, db, contacts[0], "uid-delivered")
	mapContact(t, db, contacts[1], "uid-stranded")
	mapContact(t, db, contacts[2], "uid-unreached")

	resp, err := d1.notifications.BulkSend(ctx, teacher.ID, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)

	select {
	case <-blocking.blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached its second send")
	}
	// Shutdown mid-send: the aborted and unreached rows stay queued, the run
	// record stays running — exactly what a crash leaves behind.
	d1.notifications.Close()
	require.Equal(t, notifications.RunStatusRunning, runStatusOf(t, db, *resp.RunID))

	rows := make(map[uuid.UUID]notifications.BulkSendRow, len(resp.Rows))
	for _, row := range resp.Rows {
		rows[row.ContactID] = row
	}
	return interruptedRunFixture{
		d:         newDepsWithZalo(t, db, sender),
		sender:    sender,
		teacherID: teacher.ID,
		periodID:  periodID,
		runID:     *resp.RunID,
		contacts:  contacts,
		rows:      rows,
	}
}

func TestRunSnapshotReportsTheLatestRunOrNoneAtAll(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	periodID, contacts := closedPeriodWithContacts(t, d, teacher.ID, 2)

	snap, err := d.notifications.RunSnapshot(ctx, teacher.ID, periodID)
	require.NoError(t, err)
	require.False(t, snap.Active)
	require.Nil(t, snap.RunID, "a period that never ran reports nothing, not a 404")

	mapContact(t, d.db, contacts[0], "uid-one")
	mapContact(t, d.db, contacts[1], "uid-two")
	resp, err := d.notifications.BulkSend(ctx, teacher.ID, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	snap, err = d.notifications.RunSnapshot(ctx, teacher.ID, periodID)
	require.NoError(t, err)
	require.False(t, snap.Active, "a finished run is history, not activity")
	require.NotNil(t, snap.RunID)
	require.Equal(t, *resp.RunID, *snap.RunID)
	require.Equal(t, notifications.RunStatusCompleted, snap.Status)
	require.Equal(t, notifications.PurposeStatements, snap.Purpose)
	require.Equal(t, 2, snap.Total)
	require.Equal(t, 2, snap.Sent)
	require.Zero(t, snap.Failed)
}

func TestReconcileThenResumeSendsOnlyTheStrandedRows(t *testing.T) {
	t.Parallel()
	sender := &fakeZaloSender{}
	f := newInterruptedRun(t, sender)
	ctx := context.Background()

	// The mapping for the never-reached contact disappears before the resume
	// — its row must fail with a reason, not error the whole resume.
	require.NoError(t, f.d.db.Exec(
		`UPDATE contacts SET zalo_user_id = NULL, zalo_name = NULL WHERE id = ?`, f.contacts[2]).Error)

	require.NoError(t, f.d.notifications.ReconcileInterrupted(ctx))
	snap, err := f.d.notifications.RunSnapshot(ctx, f.teacherID, f.periodID)
	require.NoError(t, err)
	require.Equal(t, notifications.RunStatusInterrupted, snap.Status)
	require.Equal(t, 3, snap.Total)
	require.Equal(t, 1, snap.Sent)
	require.Zero(t, snap.Failed)

	resumed, err := f.d.notifications.ResumeRun(ctx, f.teacherID, f.periodID)
	require.NoError(t, err)
	require.NotNil(t, resumed.RunID)
	require.Equal(t, f.runID, *resumed.RunID, "a resume continues the same run, never a new one")
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, f.d.db, f.runID))

	uids, texts := f.sender.sent()
	require.Equal(t, []string{"uid-stranded"}, uids,
		"only the stranded, still-mapped row is re-sent — never the delivered one")
	require.Equal(t, []string{f.rows[f.contacts[1]].MessageText}, texts,
		"the resumed message is re-rendered from live data, which is unchanged here")

	var rows []struct {
		ID           uuid.UUID
		Status       string
		ErrorMessage *string
	}
	require.NoError(t, f.d.db.Table("notifications").Select("id, status, error_message").
		Where("run_id = ?", f.runID).Scan(&rows).Error)
	require.Len(t, rows, 3)
	byID := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		byID[r.ID] = r.Status
		if r.ID == f.rows[f.contacts[2]].NotificationID {
			require.NotNil(t, r.ErrorMessage)
			require.Equal(t, "Chưa gán bạn Zalo", *r.ErrorMessage)
		}
	}
	require.Equal(t, notifications.StatusSent, byID[f.rows[f.contacts[0]].NotificationID])
	require.Equal(t, notifications.StatusSent, byID[f.rows[f.contacts[1]].NotificationID])
	require.Equal(t, notifications.StatusFailed, byID[f.rows[f.contacts[2]].NotificationID],
		"an unmapped row cannot be auto-sent")

	// The finished run cannot be resumed again.
	_, err = f.d.notifications.ResumeRun(ctx, f.teacherID, f.periodID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
}

func TestResumeRefusesWhenThereIsNothingToResume(t *testing.T) {
	t.Parallel()
	d := newDepsWithZalo(t, testutil.StartPostgres(t), &fakeZaloSender{})
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	periodID, _ := closedPeriodWithContacts(t, d, teacher.ID, 1)

	_, err := d.notifications.ResumeRun(ctx, teacher.ID, periodID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "no run ever existed for this period")
}

func TestResumeRefusesWhenTheSessionIsDead(t *testing.T) {
	t.Parallel()
	sender := &fakeZaloSender{verifyErr: zalo.ErrLinkExpired}
	f := newInterruptedRun(t, sender)
	ctx := context.Background()
	require.NoError(t, f.d.notifications.ReconcileInterrupted(ctx))

	_, err := f.d.notifications.ResumeRun(ctx, f.teacherID, f.periodID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	require.Equal(t, notifications.RunStatusInterrupted, runStatusOf(t, f.d.db, f.runID),
		"a refused resume leaves the run interrupted and resumable")
	uids, _ := sender.sent()
	require.Empty(t, uids)
}
