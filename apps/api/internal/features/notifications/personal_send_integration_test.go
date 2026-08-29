//go:build integration

package notifications_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// fakeZaloSender stands in for *zalo.Service. The zero value verifies every
// account and acks every send with a message id.
type fakeZaloSender struct {
	mu        sync.Mutex
	verifyErr error
	// send decides call outcomes; nil means "msg-ok" for every call. call is
	// zero-based across the fake's lifetime.
	send  func(call int, toUID string) (string, error)
	uids  []string
	texts []string
	// senders records which teacher's Zalo session each DM went out on —
	// delegated-send tests assert attribution with it.
	senders []uuid.UUID
	// friends is what ListFriends answers; friendsErr wins when set.
	friends    []zalo.Friend
	friendsErr error
}

func (f *fakeZaloSender) VerifyAccount(_ context.Context, _ uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verifyErr
}

func (f *fakeZaloSender) SendDM(_ context.Context, teacherID uuid.UUID, toUID, text string) (string, error) {
	f.mu.Lock()
	call := len(f.uids)
	f.uids = append(f.uids, toUID)
	f.texts = append(f.texts, text)
	f.senders = append(f.senders, teacherID)
	send := f.send
	f.mu.Unlock()
	if send == nil {
		return "msg-ok", nil
	}
	return send(call, toUID)
}

func (f *fakeZaloSender) ListFriends(_ context.Context, _ uuid.UUID) ([]zalo.Friend, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.friendsErr != nil {
		return nil, f.friendsErr
	}
	return append([]zalo.Friend(nil), f.friends...), nil
}

func (f *fakeZaloSender) sent() (uids, texts []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.uids...), append([]string(nil), f.texts...)
}

func (f *fakeZaloSender) sentBy() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uuid.UUID(nil), f.senders...)
}

// mapContact gives a contact a Zalo friend mapping, the way the picker
// endpoint would.
func mapContact(t *testing.T, db *gorm.DB, contactID uuid.UUID, uid string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`UPDATE contacts SET zalo_user_id = ?, zalo_name = ? WHERE id = ?`, uid, "friend-"+uid, contactID).Error)
}

func runStatusOf(t *testing.T, db *gorm.DB, runID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, db.Table("notification_runs").Select("status").Where("id = ?", runID).Take(&status).Error)
	return status
}

func waitForRunOutcome(t *testing.T, db *gorm.DB, runID uuid.UUID) string {
	t.Helper()
	require.Eventually(t, func() bool {
		return runStatusOf(t, db, runID) != notifications.RunStatusRunning
	}, 10*time.Second, 10*time.Millisecond)
	return runStatusOf(t, db, runID)
}

// closedPeriodWithContacts seeds n contacts with one child each and a closed
// billing period covering them, returning the contacts in creation order.
func closedPeriodWithContacts(t *testing.T, d *deps, teacherID uuid.UUID, n int) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	contactIDs := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		contact := testutil.Contact(t, d.db, teacherID)
		seedChild(t, d.db, teacherID, contact.ID, "PersonalChild"+string(rune('A'+i)), date("2026-06-01"), 1)
		contactIDs = append(contactIDs, contact.ID)
	}
	sc := testutil.ScopeFor(t, d.db, teacherID)
	period, err := d.billing.EnsurePeriod(ctx, sc, 2026, 6)
	require.NoError(t, err)
	_, err = d.billing.Close(ctx, sc, period.ID)
	require.NoError(t, err)
	return period.ID, contactIDs
}

func TestBulkSendPersonalSplitsMappedAndUnmappedContacts(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	sc := testutil.ScopeFor(t, d.db, teacher.ID)

	periodID, contacts := closedPeriodWithContacts(t, d, teacher.ID, 3)
	mapContact(t, d.db, contacts[0], "uid-first")
	mapContact(t, d.db, contacts[1], "uid-second")
	// contacts[2] stays unmapped and must fall back to copy-paste.

	resp, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.Equal(t, 3, resp.QueuedCount)
	require.Equal(t, 2, resp.PersonalQueuedCount)
	require.Equal(t, 1, resp.FallbackManualCount)
	require.NotNil(t, resp.RunID)

	rowsByContact := make(map[uuid.UUID]notifications.BulkSendRow, len(resp.Rows))
	for _, row := range resp.Rows {
		rowsByContact[row.ContactID] = row
	}
	require.Equal(t, notifications.ChannelZaloPersonal, rowsByContact[contacts[0]].Channel)
	require.Equal(t, notifications.ChannelZaloPersonal, rowsByContact[contacts[1]].Channel)
	require.Equal(t, notifications.ChannelZaloManual, rowsByContact[contacts[2]].Channel,
		"an unmapped contact falls back to the copy-paste channel, never an error")

	// BulkText is the copy-paste bundle: only the fallback row belongs in it.
	require.Contains(t, resp.BulkText, rowsByContact[contacts[2]].MessageText)
	require.NotContains(t, resp.BulkText, rowsByContact[contacts[0]].MessageText,
		"an auto-sent message has no business in the copy-paste bundle")

	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	uids, texts := fake.sent()
	require.ElementsMatch(t, []string{"uid-first", "uid-second"}, uids)
	require.ElementsMatch(t, []string{rowsByContact[contacts[0]].MessageText, rowsByContact[contacts[1]].MessageText}, texts,
		"the DM must carry exactly the rendered statement message")

	// Both personal rows are sent with the provider's message id; the
	// fallback row waits for the teacher's own mark-sent.
	var statuses []struct {
		Status        string
		ProviderMsgID *string
	}
	require.NoError(t, d.db.Table("notifications").Select("status, provider_msg_id").
		Where("run_id = ?", *resp.RunID).Scan(&statuses).Error)
	require.Len(t, statuses, 2)
	for _, s := range statuses {
		require.Equal(t, notifications.StatusSent, s.Status)
		require.NotNil(t, s.ProviderMsgID)
		require.Equal(t, "msg-ok", *s.ProviderMsgID)
	}
	var fallbackStatus string
	require.NoError(t, d.db.Table("notifications").Select("status").
		Where("id = ?", rowsByContact[contacts[2]].NotificationID).Take(&fallbackStatus).Error)
	require.Equal(t, notifications.StatusQueued, fallbackStatus)
}

func TestBulkSendPersonalWithNoMappedContactStartsNoRun(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	sc := testutil.ScopeFor(t, d.db, teacher.ID)

	periodID, _ := closedPeriodWithContacts(t, d, teacher.ID, 2)

	resp, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.Nil(t, resp.RunID, "nothing to auto-send means no run at all")
	require.Zero(t, resp.PersonalQueuedCount)
	require.Equal(t, 2, resp.FallbackManualCount)
	require.Equal(t, 2, resp.QueuedCount)

	var runCount int64
	require.NoError(t, d.db.Table("notification_runs").Where("teacher_id = ?", teacher.ID).Count(&runCount).Error)
	require.Zero(t, runCount)
	uids, _ := fake.sent()
	require.Empty(t, uids)
}

// A Zalo account is personal: messages go out from the caller's own linked
// session. An owner opening a member's period under zalo_personal would
// therefore DM the member's parents from the owner's account — every mapped
// contact belongs to the member's strict scope, so the whole batch would
// silently fall back to copy-paste. Refuse the combination outright; the
// owner's own periods keep working unchanged.
func TestBulkSendPersonalRefusesAMembersPeriod(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, d.db)
	_, member := testutil.Teacher(t, d.db)
	ownerCenter := testutil.ScopeFor(t, d.db, owner.ID).CenterID
	testutil.JoinCenter(t, d.db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, d.db, owner.ID)

	memberPeriod, memberContacts := closedPeriodWithContacts(t, d, member.ID, 1)
	mapContact(t, d.db, memberContacts[0], "uid-member")

	_, err := d.notifications.BulkSend(ctx, ownerScope, memberPeriod, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	require.Zero(t, notificationCount(t, d.db, memberPeriod),
		"the refused send must write nothing — not even the statement refresh's notifications")
	var runCount int64
	require.NoError(t, d.db.Table("notification_runs").Where("teacher_id = ?", owner.ID).Count(&runCount).Error)
	require.Zero(t, runCount)
	uids, _ := fake.sent()
	require.Empty(t, uids)

	// The owner's own period still sends from their own account.
	ownPeriod, ownContacts := closedPeriodWithContacts(t, d, owner.ID, 1)
	mapContact(t, d.db, ownContacts[0], "uid-own")
	resp, err := d.notifications.BulkSend(ctx, ownerScope, ownPeriod, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	require.Equal(t, 1, resp.PersonalQueuedCount)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))
}

func TestBulkSendPersonalRejectsAnUnhealthySessionBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		verifyErr error
		wantCode  string
	}{
		{"not linked", zalo.ErrNotLinked, apperror.CodeBadRequest},
		{"expired", zalo.ErrLinkExpired, apperror.CodeConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newDepsWithZalo(t, testutil.StartPostgres(t), &fakeZaloSender{verifyErr: tc.verifyErr})
			ctx := context.Background()
			_, teacher := testutil.Teacher(t, d.db)
			sc := testutil.ScopeFor(t, d.db, teacher.ID)
			periodID, contacts := closedPeriodWithContacts(t, d, teacher.ID, 1)
			mapContact(t, d.db, contacts[0], "uid-any")

			_, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
				Purpose: "statement",
				Channel: notifications.ChannelZaloPersonal,
			})
			require.Error(t, err)
			require.Equal(t, tc.wantCode, apperror.From(err).Code)
			require.Zero(t, notificationCount(t, d.db, periodID),
				"a bad channel choice never leaves partial state behind")
		})
	}
}

func TestBulkSendPersonalOverTheRunSizeCapWritesNothing(t *testing.T) {
	t.Parallel()
	d := newDepsWithZaloAndRunCap(t, testutil.StartPostgres(t), &fakeZaloSender{}, 1)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	sc := testutil.ScopeFor(t, d.db, teacher.ID)
	periodID, contacts := closedPeriodWithContacts(t, d, teacher.ID, 2)
	mapContact(t, d.db, contacts[0], "uid-one")
	mapContact(t, d.db, contacts[1], "uid-two")

	_, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeBadRequest, apperror.From(err).Code)
	require.Zero(t, notificationCount(t, d.db, periodID))
	var runCount int64
	require.NoError(t, d.db.Table("notification_runs").Where("teacher_id = ?", teacher.ID).Count(&runCount).Error)
	require.Zero(t, runCount)
}

func TestBulkSendPersonalRefusesWhileARunIsStillSending(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	fake := &fakeZaloSender{send: func(int, string) (string, error) {
		<-release
		return "msg-ok", nil
	}}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	sc := testutil.ScopeFor(t, d.db, teacher.ID)

	periodID, contacts := closedPeriodWithContacts(t, d, teacher.ID, 1)
	mapContact(t, d.db, contacts[0], "uid-slow")

	first, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, first.RunID)

	countBefore := notificationCount(t, d.db, periodID)
	_, err = d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.Error(t, err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	require.Equal(t, countBefore, notificationCount(t, d.db, periodID),
		"the refused second call must write nothing")

	close(release)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *first.RunID))

	// Once the run has finished, a new personal send is allowed again.
	third, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "reminder",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, third.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *third.RunID))
}

func TestBulkSendPersonalExpiringMidRunFailsTheRemainingRows(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{send: func(call int, _ string) (string, error) {
		if call == 0 {
			return "msg-ok", nil
		}
		return "", zalo.ErrLinkExpired
	}}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, d.db)
	sc := testutil.ScopeFor(t, d.db, teacher.ID)

	periodID, contacts := closedPeriodWithContacts(t, d, teacher.ID, 3)
	for i, c := range contacts {
		mapContact(t, d.db, c, "uid-"+string(rune('a'+i)))
	}

	resp, err := d.notifications.BulkSend(ctx, sc, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	require.Equal(t, notifications.RunStatusExpired, waitForRunOutcome(t, d.db, *resp.RunID))

	var rows []struct {
		Status       string
		ErrorMessage *string
	}
	require.NoError(t, d.db.Table("notifications").Select("status, error_message").
		Where("run_id = ?", *resp.RunID).Order("created_at").Scan(&rows).Error)
	require.Len(t, rows, 3)
	sent, failed := 0, 0
	for _, r := range rows {
		switch r.Status {
		case notifications.StatusSent:
			sent++
		case notifications.StatusFailed:
			failed++
			require.NotNil(t, r.ErrorMessage)
			require.Equal(t, "Phiên Zalo đã hết hạn", *r.ErrorMessage)
		}
	}
	require.Equal(t, 1, sent, "the send that succeeded before the session died keeps its verdict")
	require.Equal(t, 2, failed, "every row the dead session stranded is failed, not left queued")
}
