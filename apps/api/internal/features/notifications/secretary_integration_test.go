//go:build integration

package notifications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// Failure reasons the service writes on rows it sweeps — asserted literally
// because the teacher-facing ledger is the only place these strings surface.
const (
	revokedReason  = "Quyền gửi báo cáo đã bị thu hồi"
	orphanedReason = "Đợt gửi mới đã thay thế đợt gửi bị bỏ dở"
)

// seedSecretaryCenter builds the delegation cast on the deps' database: an
// owner's center holding teacherX (a plain member with no flag) and a
// secretary member holding can_send_reports. Returns the three teacher ids.
func seedSecretaryCenter(t *testing.T, d *deps) (owner, teacherX, secretary uuid.UUID) {
	t.Helper()
	ownerAcc, _ := testutil.Teacher(t, d.db)
	centerID := testutil.ScopeFor(t, d.db, ownerAcc.ID).CenterID
	x, _ := testutil.Teacher(t, d.db)
	testutil.JoinCenter(t, d.db, x.ID, centerID)
	_, sec := testutil.Secretary(t, d.db, centerID)
	return ownerAcc.ID, x.ID, sec.ID
}

// TestSecretaryDelegatedPersonalSendStampsHerAsSender is the delegated-send
// attribution matrix: a secretary running zalo_personal on ANOTHER teacher's
// period creates the run and every ledger row under HER teacher_id, the DMs
// go out on HER Zalo session, the period's statements stay anchored on the
// period teacher, unmapped contacts still fall back to manual, and the period
// teacher (plain, no flag) can read the delegated rows in their own ledger.
func TestSecretaryDelegatedPersonalSendStampsHerAsSender(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacherX, secretary := seedSecretaryCenter(t, d)

	periodID, contacts := closedPeriodWithContacts(t, d, teacherX, 3)
	mapContact(t, d.db, contacts[0], "uid-delegated-a")
	mapContact(t, d.db, contacts[1], "uid-delegated-b")
	// contacts[2] stays unmapped — must fall back to the manual channel.

	secScope := testutil.ScopeFor(t, d.db, secretary)
	require.True(t, secScope.CanSendReports, "secretary fixture must carry the flag")
	require.False(t, secScope.IsOwner, "the flag holder must be an ordinary member, never the owner")

	resp, err := d.notifications.BulkSend(ctx, secScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err, "a can_send_reports member must be able to run a personal send on another teacher's period")
	require.NotNil(t, resp.RunID)
	require.Equal(t, 2, resp.PersonalQueuedCount)
	require.Equal(t, 1, resp.FallbackManualCount, "the unmapped contact must fall back to manual, delegated or not")
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	// The run and every ledger row are the SECRETARY's — never reassigned to
	// the period teacher whose period, statements, and contacts they concern.
	var runRow struct {
		TeacherID       uuid.UUID
		BillingPeriodID uuid.UUID
	}
	require.NoError(t, d.db.Table("notification_runs").Select("teacher_id, billing_period_id").
		Where("id = ?", *resp.RunID).Take(&runRow).Error)
	require.Equal(t, secretary, runRow.TeacherID, "the run must stamp the secretary as sender")
	require.Equal(t, periodID, runRow.BillingPeriodID)
	for _, row := range resp.Rows {
		var ledgerRow struct{ TeacherID uuid.UUID }
		require.NoError(t, d.db.Table("notifications").Select("teacher_id").
			Where("id = ?", row.NotificationID).Take(&ledgerRow).Error)
		require.Equal(t, secretary, ledgerRow.TeacherID, "every ledger row must stamp the secretary as sender")
	}

	// The DMs went out on the secretary's own Zalo session.
	senders := fake.sentBy()
	require.Len(t, senders, 2)
	for _, s := range senders {
		require.Equal(t, secretary, s, "delegated DMs must use the secretary's session, never the period teacher's")
	}

	// The statements themselves stay the PERIOD teacher's — delegation moves
	// the sending, never the billing identity parents see.
	var stmtTeachers []uuid.UUID
	require.NoError(t, d.db.Table("statements").Distinct("teacher_id").
		Where("period_id = ?", periodID).Pluck("teacher_id", &stmtTeachers).Error)
	require.Equal(t, []uuid.UUID{teacherX}, stmtTeachers)

	// The period's own teacher — plain, no flag — sees the delegated rows and
	// the run in their period's ledger, so they will not double-send by hand.
	xScope := testutil.ScopeFor(t, d.db, teacherX)
	require.False(t, xScope.CanSendReports)
	rows, err := d.notifications.List(ctx, xScope, periodID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 3, "the period teacher must see rows a secretary sent on their period")
	snap, err := d.notifications.RunSnapshot(ctx, xScope, periodID, nil)
	require.NoError(t, err)
	require.NotNil(t, snap.RunID)
	require.Equal(t, *resp.RunID, *snap.RunID, "the period teacher must see the delegated run's progress")

	// The secretary reads the same ledger through oversight.
	rows, err = d.notifications.List(ctx, secScope, periodID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 3)
}

// TestOnePeriodNeverRunsTwoConcurrentSends proves the period-level run lock:
// while a secretary's run is still sending a period, any other authorized
// sender starting a personal send on that same period is refused with a
// conflict, and the period ends the day with exactly one run.
func TestOnePeriodNeverRunsTwoConcurrentSends(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	parked := make(chan struct{})
	fake := &fakeZaloSender{}
	fake.send = func(call int, _ string) (string, error) {
		if call == 0 {
			close(parked)
			<-release
		}
		return "msg-ok", nil
	}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	owner, teacherX, secretary := seedSecretaryCenter(t, d)

	periodID, contacts := closedPeriodWithContacts(t, d, teacherX, 2)
	mapContact(t, d.db, contacts[0], "uid-lock-a")
	mapContact(t, d.db, contacts[1], "uid-lock-b")
	secScope := testutil.ScopeFor(t, d.db, secretary)

	resp, err := d.notifications.BulkSend(ctx, secScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	select {
	case <-parked:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached its first send")
	}

	// The owner — the other caller who passes the send gate — is refused while
	// the secretary's run occupies the period.
	ownerScope := testutil.ScopeFor(t, d.db, owner)
	_, err = d.notifications.BulkSend(ctx, ownerScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code,
		"a second sender must be refused while the period's run is still sending")

	// So is the secretary herself trying to double-start.
	_, err = d.notifications.BulkSend(ctx, secScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	close(release)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	var runCount int64
	require.NoError(t, d.db.Table("notification_runs").
		Where("billing_period_id = ?", periodID).Count(&runCount).Error)
	require.EqualValues(t, 1, runCount, "the period must end with exactly one run")
}

// TestRevokeMidRunFailsRemainingRowsAndBlocksResume proves revocation is
// enforced between items of an in-flight delegated run: the already-delivered
// row keeps its verdict, every still-queued row fails with the explicit
// revocation reason, the run lands interrupted, and a resume attempt under
// the revoked scope is refused with a 403.
func TestRevokeMidRunFailsRemainingRowsAndBlocksResume(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	parked := make(chan struct{})
	fake := &fakeZaloSender{}
	fake.send = func(call int, _ string) (string, error) {
		if call == 0 {
			close(parked)
			<-release
		}
		return "msg-ok", nil
	}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacherX, secretary := seedSecretaryCenter(t, d)

	periodID, contacts := closedPeriodWithContacts(t, d, teacherX, 3)
	for i, c := range contacts {
		mapContact(t, d.db, c, fmt.Sprintf("uid-revoke-%d", i))
	}
	secScope := testutil.ScopeFor(t, d.db, secretary)

	resp, err := d.notifications.BulkSend(ctx, secScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	select {
	case <-parked:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached its first send")
	}

	// Revoke while the first DM is in flight; the run must notice before the
	// second item and stop.
	testutil.GrantSendReports(t, d.db, secretary, false)
	close(release)
	require.Equal(t, notifications.RunStatusInterrupted, waitForRunOutcome(t, d.db, *resp.RunID))

	var outcomes []struct {
		Status       string
		ErrorMessage *string
	}
	require.NoError(t, d.db.Table("notifications").Select("status, error_message").
		Where("run_id = ?", *resp.RunID).Order("status").Find(&outcomes).Error)
	require.Len(t, outcomes, 3)
	sent, failed := 0, 0
	for _, o := range outcomes {
		switch o.Status {
		case notifications.StatusSent:
			sent++
		case notifications.StatusFailed:
			failed++
			require.NotNil(t, o.ErrorMessage)
			require.Equal(t, revokedReason, *o.ErrorMessage,
				"swept rows must carry the explicit revocation reason")
		default:
			t.Fatalf("unexpected row status %q after revocation", o.Status)
		}
	}
	require.Equal(t, 1, sent, "the row delivered before revocation keeps its verdict")
	require.Equal(t, 2, failed)

	// A resume under the revoked scope is refused honestly — the permission
	// gate reloads the flag per request.
	revokedScope := testutil.ScopeFor(t, d.db, secretary)
	require.False(t, revokedScope.CanSendReports)
	_, err = d.notifications.ResumeRun(ctx, revokedScope, periodID, nil)
	require.Error(t, err)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"a revoked secretary must not resume her interrupted run")
}

// TestNewSendFailsOutAnAbandonedDelegatedRunsQueuedRows proves the orphan
// sweep: when a secretary's run dies mid-flight (process crash) and a
// DIFFERENT authorized sender later sends the same period, the stale run's
// still-queued rows are failed out with an explicit reason instead of
// lingering as ghost queue entries, and the new run proceeds normally.
func TestNewSendFailsOutAnAbandonedDelegatedRunsQueuedRows(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	blocking := &interruptibleZalo{blocked: make(chan struct{})}
	d1 := newDepsWithZalo(t, db, blocking)
	ctx := context.Background()
	_, teacherX, secretary := seedSecretaryCenter(t, d1)

	periodID, contacts := closedPeriodWithContacts(t, d1, teacherX, 3)
	for i, c := range contacts {
		mapContact(t, db, c, fmt.Sprintf("uid-orphan-%d", i))
	}
	secScope := testutil.ScopeFor(t, db, secretary)
	resp, err := d1.notifications.BulkSend(ctx, secScope, periodID, notifications.BulkSendRequest{
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
	// Crash mid-send: one row delivered, two stranded queued.
	d1.notifications.Close()

	fake := &fakeZaloSender{}
	d2 := newDepsWithZalo(t, db, fake)
	require.NoError(t, d2.notifications.ReconcileInterrupted(ctx))
	require.Equal(t, notifications.RunStatusInterrupted, runStatusOf(t, db, *resp.RunID))

	// A second secretary takes over the period. Her send must fail out the
	// abandoned run's queued rows rather than leave them queued forever.
	centerID := secScope.CenterID
	_, sec2 := testutil.Secretary(t, db, centerID)
	sec2Scope := testutil.ScopeFor(t, db, sec2.ID)
	resp2, err := d2.notifications.BulkSend(ctx, sec2Scope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err, "an interrupted run must not block a different sender's new send")
	require.NotNil(t, resp2.RunID)
	require.NotEqual(t, *resp.RunID, *resp2.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, db, *resp2.RunID))

	var staleRows []struct {
		Status       string
		ErrorMessage *string
	}
	require.NoError(t, db.Table("notifications").Select("status, error_message").
		Where("run_id = ?", *resp.RunID).Find(&staleRows).Error)
	require.Len(t, staleRows, 3)
	sent, failed := 0, 0
	for _, o := range staleRows {
		switch o.Status {
		case notifications.StatusSent:
			sent++
		case notifications.StatusFailed:
			failed++
			require.NotNil(t, o.ErrorMessage)
			require.Equal(t, orphanedReason, *o.ErrorMessage,
				"orphaned queued rows must say the new send replaced them")
		default:
			t.Fatalf("stale run still has a %q row after the new send", o.Status)
		}
	}
	require.Equal(t, 1, sent, "the row the dead run delivered keeps its verdict")
	require.Equal(t, 2, failed)
	require.Equal(t, notifications.RunStatusInterrupted, runStatusOf(t, db, *resp.RunID),
		"the stale run record itself stays interrupted")
}

// TestPlainMemberCannotCreateSendsOnAnyChannelButKeepsMarkSent pins the D8
// breaking change: a member without can_send_reports is refused with an
// explicit 403 on every send channel — their OWN period included — and on
// resume, while marking an already-existing row of theirs sent keeps working.
func TestPlainMemberCannotCreateSendsOnAnyChannelButKeepsMarkSent(t *testing.T) {
	t.Parallel()
	d := newDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, d.db)
	centerID := testutil.ScopeFor(t, d.db, owner.ID).CenterID
	member, _ := testutil.Teacher(t, d.db)
	testutil.JoinCenter(t, d.db, member.ID, centerID)
	memberScope := testutil.ScopeFor(t, d.db, member.ID)
	require.False(t, memberScope.CanSendReports)
	require.False(t, memberScope.IsOwner)

	periodID, contacts := closedPeriodWithContacts(t, d, member.ID, 1)

	for _, channel := range []string{
		"", // default channel
		notifications.ChannelZaloManual,
		notifications.ChannelZaloZNS,
		notifications.ChannelSMS,
		notifications.ChannelZaloPersonal,
	} {
		_, err := d.notifications.BulkSend(ctx, memberScope, periodID, notifications.BulkSendRequest{
			Purpose: "statement",
			Channel: channel,
		})
		require.Error(t, err, "channel %q must be refused", channel)
		require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
			"a plain member must get an explicit 403 on channel %q, even on their own period", channel)
	}
	_, err := d.notifications.ResumeRun(ctx, memberScope, periodID, nil)
	require.Error(t, err)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	require.EqualValues(t, 0, notificationCount(t, d.db, periodID),
		"refused sends must write nothing")

	// A row that already exists under the member's name — sent before the
	// permission change, say — can still be marked sent by hand.
	statementID := id.New()
	require.NoError(t, d.db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		statementID, member.ID, centerID, contacts[0], periodID, []byte("hash-mark-sent-regression")).Error)
	repo := notifications.NewRepository(d.db)
	row := &notifications.Notification{
		ID:          id.New(),
		TeacherID:   member.ID,
		CenterID:    centerID,
		StatementID: statementID,
		Channel:     notifications.ChannelZaloManual,
		Purpose:     notifications.PurposeStatements,
		Status:      notifications.StatusQueued,
	}
	require.NoError(t, repo.InsertBatch(ctx, []*notifications.Notification{row}))
	require.NoError(t, d.notifications.MarkSent(ctx, memberScope, []uuid.UUID{row.ID}),
		"marking an existing own row sent must keep working for a plain member")
	var status string
	require.NoError(t, d.db.Table("notifications").Select("status").
		Where("id = ?", row.ID).Take(&status).Error)
	require.Equal(t, notifications.StatusSent, status)
}

// TestSendPreviewBucketsHoldPastAHundredContacts proves the preview computes
// its three buckets from the FULL target set — 105 contacts, past any
// client-side roster page cap — and exposes the server's run-size cap.
func TestSendPreviewBucketsHoldPastAHundredContacts(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacherX, secretary := seedSecretaryCenter(t, d)

	const total, mappedFriends, mappedStrangers = 105, 40, 40
	periodID, contacts := closedPeriodWithContacts(t, d, teacherX, total)
	autoSend := make(map[uuid.UUID]bool, mappedFriends)
	notFriend := make(map[uuid.UUID]bool, mappedStrangers)
	unmapped := make(map[uuid.UUID]bool, total-mappedFriends-mappedStrangers)
	for i, c := range contacts {
		switch {
		case i < mappedFriends:
			uid := fmt.Sprintf("uid-friend-%03d", i)
			mapContact(t, d.db, c, uid)
			fake.friends = append(fake.friends, zalo.Friend{UserID: uid})
			autoSend[c] = true
		case i < mappedFriends+mappedStrangers:
			mapContact(t, d.db, c, fmt.Sprintf("uid-stranger-%03d", i))
			notFriend[c] = true
		default:
			unmapped[c] = true
		}
	}

	secScope := testutil.ScopeFor(t, d.db, secretary)
	preview, err := d.notifications.SendPreview(ctx, secScope, periodID, "statement", nil)
	require.NoError(t, err)
	require.Len(t, preview.AutoSend, mappedFriends)
	require.Len(t, preview.MappedNotFriend, mappedStrangers)
	require.Len(t, preview.Unmapped, total-mappedFriends-mappedStrangers)
	require.Equal(t, 50, preview.MaxRunSize, "the preview must expose the server's run-size cap")
	for _, c := range preview.AutoSend {
		require.True(t, autoSend[c.ContactID], "contact %s does not belong in auto_send", c.ContactID)
	}
	for _, c := range preview.MappedNotFriend {
		require.True(t, notFriend[c.ContactID], "contact %s does not belong in mapped_not_friend", c.ContactID)
	}
	for _, c := range preview.Unmapped {
		require.True(t, unmapped[c.ContactID], "contact %s does not belong in unmapped", c.ContactID)
	}

	// The period's own plain teacher cannot preview — same 403 as sending.
	xScope := testutil.ScopeFor(t, d.db, teacherX)
	_, err = d.notifications.SendPreview(ctx, xScope, periodID, "statement", nil)
	require.Error(t, err)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

// TestSecretaryWithoutLinkedZaloIsRefusedBeforeWriting proves delegation does
// not weaken the personal-session precondition: a secretary whose own Zalo
// account is not linked is refused up front — send and preview alike — with
// nothing written.
func TestSecretaryWithoutLinkedZaloIsRefusedBeforeWriting(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{verifyErr: zalo.ErrNotLinked, friendsErr: zalo.ErrNotLinked}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacherX, secretary := seedSecretaryCenter(t, d)

	periodID, contacts := closedPeriodWithContacts(t, d, teacherX, 1)
	mapContact(t, d.db, contacts[0], "uid-unlinked")
	secScope := testutil.ScopeFor(t, d.db, secretary)

	_, err := d.notifications.BulkSend(ctx, secScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeBadRequest, apperror.From(err).Code,
		"an unlinked secretary must be told to link her own account first")
	require.EqualValues(t, 0, notificationCount(t, d.db, periodID))

	_, err = d.notifications.SendPreview(ctx, secScope, periodID, "statement", nil)
	require.Error(t, err)
	require.Equal(t, apperror.CodeBadRequest, apperror.From(err).Code)
}
