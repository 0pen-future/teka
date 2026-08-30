//go:build integration

package notifications_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// TestOwnerHasOversightReadAndSendsAsSelfOnMembersPeriod proves owner
// oversight end to end on the notifications feature: an owner can read a
// member's own zalo_personal run and notification ledger without having sent
// anything themselves (oversight read), and can additionally act on that same
// member's closed period — the created row stays anchored on the OWNER as
// sender, never reassigned to the member whose period, statement, and
// contact it concerns (owner sends as self).
func TestOwnerHasOversightReadAndSendsAsSelfOnMembersPeriod(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, d.db)
	member, _ := testutil.Teacher(t, d.db)
	ownerScope := testutil.ScopeFor(t, d.db, owner.ID)
	testutil.JoinCenter(t, d.db, member.ID, ownerScope.CenterID)
	testutil.GrantSendReports(t, d.db, member.ID, true)
	memberScope := testutil.ScopeFor(t, d.db, member.ID)
	require.Equal(t, ownerScope.CenterID, memberScope.CenterID, "member must have joined the owner's center")

	periodID, contacts := closedPeriodWithContacts(t, d, member.ID, 1)
	mapContact(t, d.db, contacts[0], "uid-member-own")

	// The member holds can_send_reports (creating sends requires it for any
	// non-owner) and sends their own zalo_personal run under their own Zalo
	// session.
	memberResp, err := d.notifications.BulkSend(ctx, memberScope, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, memberResp.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *memberResp.RunID))

	// Oversight read: the owner never touched this run, yet can poll it and
	// list its ledger exactly as the member would.
	snap, err := d.notifications.RunSnapshot(ctx, ownerScope, periodID, nil)
	require.NoError(t, err, "an owner must be able to poll a member's run")
	require.NotNil(t, snap.RunID)
	require.Equal(t, *memberResp.RunID, *snap.RunID)
	require.Equal(t, notifications.RunStatusCompleted, snap.Status)
	require.Equal(t, 1, snap.Total)
	require.Equal(t, 1, snap.Sent)

	rows, err := d.notifications.List(ctx, ownerScope, periodID, notifications.ListFilter{})
	require.NoError(t, err, "an owner must be able to list a member's notifications")
	require.Len(t, rows, 1)
	require.Equal(t, contacts[0], rows[0].ContactID)

	// The owner separately sends a reminder for the SAME member's period —
	// this must succeed (owner oversight authorizes acting on a member's
	// closed period), and the created row stamps the OWNER as sender, never
	// reassigned to the member even though the period, statement, and target
	// contact are all the member's own.
	ownerResp, err := d.notifications.BulkSend(ctx, ownerScope, periodID, notifications.BulkSendRequest{Purpose: "reminder"})
	require.NoError(t, err, "an owner must be able to act on a member's closed period")
	require.Len(t, ownerResp.Rows, 1)

	var dbRow struct {
		TeacherID uuid.UUID
		CenterID  uuid.UUID
	}
	require.NoError(t, d.db.Table("notifications").
		Select("teacher_id, center_id").
		Where("id = ?", ownerResp.Rows[0].NotificationID).
		Take(&dbRow).Error)
	require.Equal(t, owner.ID, dbRow.TeacherID,
		"an owner sending for a member's period sends as themselves, never as the member")
	require.Equal(t, ownerScope.CenterID, dbRow.CenterID)

	// Oversight now surfaces both purposes as one ledger, regardless of which
	// of the two teachers sent which row.
	rows, err = d.notifications.List(ctx, ownerScope, periodID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

// TestPeerInSameCenterCannotReadOrActOnAnotherMembersNotifications proves
// center membership grants the owner oversight, not peer-to-peer access: two
// non-owning members in the same center stay fully isolated from each
// other's notification ledger and run state.
func TestPeerInSameCenterCannotReadOrActOnAnotherMembersNotifications(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, d.db)
	memberA, _ := testutil.Teacher(t, d.db)
	memberB, _ := testutil.Teacher(t, d.db)
	ownerCenter := testutil.ScopeFor(t, d.db, owner.ID).CenterID
	testutil.JoinCenter(t, d.db, memberA.ID, ownerCenter)
	testutil.JoinCenter(t, d.db, memberB.ID, ownerCenter)
	testutil.GrantSendReports(t, d.db, memberA.ID, true)
	scopeA := testutil.ScopeFor(t, d.db, memberA.ID)
	scopeB := testutil.ScopeFor(t, d.db, memberB.ID)

	periodID, contacts := closedPeriodWithContacts(t, d, memberA.ID, 1)
	mapContact(t, d.db, contacts[0], "uid-peer-a")

	resp, err := d.notifications.BulkSend(ctx, scopeA, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	// A peer sees an empty ledger and no run for a period that is not their
	// own — silently, never a 403, matching the standard oversight template's
	// non-owner branch.
	rows, err := d.notifications.List(ctx, scopeB, periodID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Empty(t, rows, "a peer must not see another member's notification ledger")

	snap, err := d.notifications.RunSnapshot(ctx, scopeB, periodID, nil)
	require.NoError(t, err)
	require.False(t, snap.Active)
	require.Nil(t, snap.RunID, "a peer must not see another member's run")

	// A peer without can_send_reports cannot create sends at all — the
	// permission gate refuses honestly with a 403 before any period lookup.
	_, err = d.notifications.BulkSend(ctx, scopeB, periodID, notifications.BulkSendRequest{Purpose: "reminder"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"a member without the send-reports permission must get an explicit 403")

	// A peer cannot mark another member's notification sent either.
	require.NoError(t, d.notifications.MarkSent(ctx, scopeB, []uuid.UUID{resp.Rows[0].NotificationID}))
	var status string
	require.NoError(t, d.db.Table("notifications").Select("status").
		Where("id = ?", resp.Rows[0].NotificationID).Take(&status).Error)
	require.Equal(t, notifications.StatusSent, status,
		"the row was already sent by the run; a peer's no-op mark-sent must not touch it")
}

// TestCrossCenterNotificationsAreInvisible proves a teacher in a different
// center gets the same neutral not-found/empty behavior as a missing
// resource on every notifications path — never a 403, and never a peek into
// another center's ledger.
func TestCrossCenterNotificationsAreInvisible(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloSender{}
	d := newDepsWithZalo(t, testutil.StartPostgres(t), fake)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, d.db)
	teacherB, _ := testutil.Teacher(t, d.db)
	scopeA := testutil.ScopeFor(t, d.db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, d.db, teacherB.ID)
	require.NotEqual(t, scopeA.CenterID, scopeB.CenterID, "fixture teachers must start in separate personal centers")

	periodID, contacts := closedPeriodWithContacts(t, d, teacherA.ID, 1)
	mapContact(t, d.db, contacts[0], "uid-cross-center")

	resp, err := d.notifications.BulkSend(ctx, scopeA, periodID, notifications.BulkSendRequest{
		Purpose: "statement",
		Channel: notifications.ChannelZaloPersonal,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.RunID)
	require.Equal(t, notifications.RunStatusCompleted, waitForRunOutcome(t, d.db, *resp.RunID))

	_, err = d.notifications.BulkSend(ctx, scopeB, periodID, notifications.BulkSendRequest{Purpose: "reminder"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"a teacher in another center must not see this period at all")

	rows, err := d.notifications.List(ctx, scopeB, periodID, notifications.ListFilter{})
	require.NoError(t, err)
	require.Empty(t, rows, "another center's ledger must read as empty, not error")

	snap, err := d.notifications.RunSnapshot(ctx, scopeB, periodID, nil)
	require.NoError(t, err)
	require.False(t, snap.Active)
	require.Nil(t, snap.RunID, "another center's run must be invisible")

	_, err = d.notifications.ResumeRun(ctx, scopeB, periodID, nil)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"another center cannot resume a run it cannot even see")

	require.NoError(t, d.notifications.MarkSent(ctx, scopeB, []uuid.UUID{resp.Rows[0].NotificationID}))
	var status string
	require.NoError(t, d.db.Table("notifications").Select("status").
		Where("id = ?", resp.Rows[0].NotificationID).Take(&status).Error)
	require.Equal(t, notifications.StatusSent, status,
		"the row was already sent by the run; a cross-center no-op mark-sent must not touch it")
}
