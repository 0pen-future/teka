//go:build integration

package notifications_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// statementFixture is the minimal parent chain a notifications row needs:
// teacher -> contact -> closed-period statement. Periods and statements are
// inserted raw because their own services would drag half the product into
// what is a persistence-layer test.
type statementFixture struct {
	teacherID   uuid.UUID
	centerID    uuid.UUID
	contactID   uuid.UUID
	periodID    uuid.UUID
	statementID uuid.UUID
}

// scope is the fixture's own teacher/center scope, IsOwner false — the run
// lifecycle tests exercise ordinary member behavior; oversight is covered by
// the auth integration tests.
func (f statementFixture) scope() authctx.Scope {
	return authctx.Scope{TeacherID: f.teacherID, CenterID: f.centerID}
}

func seedStatement(t *testing.T, db *gorm.DB) statementFixture {
	t.Helper()
	_, teacher := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	f := statementFixture{
		teacherID:   teacher.ID,
		centerID:    teacher.CenterID,
		contactID:   contact.ID,
		periodID:    id.New(),
		statementID: id.New(),
	}
	require.NoError(t, db.Exec(
		`INSERT INTO billing_periods (id, teacher_id, center_id, year, month, period_start, period_end)
		 VALUES (?, ?, ?, 2026, 8, '2026-08-01', '2026-08-31')`,
		f.periodID, f.teacherID, f.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		f.statementID, f.teacherID, f.centerID, f.contactID, f.periodID, []byte("hash-"+contact.Phone)).Error)
	return f
}

// seedSecondPeriodStatement gives the SAME teacher a second period with its
// own contact and statement, for tests that must tell run scoping apart from
// teacher scoping.
func seedSecondPeriodStatement(t *testing.T, db *gorm.DB, f statementFixture) statementFixture {
	t.Helper()
	contact := testutil.Contact(t, db, f.teacherID)
	second := statementFixture{
		teacherID:   f.teacherID,
		centerID:    f.centerID,
		contactID:   contact.ID,
		periodID:    id.New(),
		statementID: id.New(),
	}
	require.NoError(t, db.Exec(
		`INSERT INTO billing_periods (id, teacher_id, center_id, year, month, period_start, period_end)
		 VALUES (?, ?, ?, 2026, 9, '2026-09-01', '2026-09-30')`,
		second.periodID, second.teacherID, second.centerID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO statements (id, teacher_id, center_id, contact_id, period_id, token_hash, expires_at, total_due)
		 VALUES (?, ?, ?, ?, ?, ?, now() + interval '7 days', 100000)`,
		second.statementID, second.teacherID, second.centerID, second.contactID, second.periodID, []byte("hash-"+contact.Phone)).Error)
	return second
}

// seedRun creates a running notification_runs record for the fixture's period.
func seedRun(t *testing.T, repo notifications.Repository, f statementFixture) *notifications.Run {
	t.Helper()
	run := &notifications.Run{
		ID:              id.New(),
		TeacherID:       f.teacherID,
		CenterID:        f.centerID,
		BillingPeriodID: f.periodID,
		Purpose:         notifications.PurposeStatements,
		Status:          notifications.RunStatusRunning,
	}
	require.NoError(t, repo.CreateRun(context.Background(), run))
	return run
}

// seedRunRow inserts one queued zalo_personal notification attached to the run.
func seedRunRow(t *testing.T, repo notifications.Repository, f statementFixture, runID uuid.UUID) uuid.UUID {
	t.Helper()
	n := &notifications.Notification{
		ID:          id.New(),
		TeacherID:   f.teacherID,
		CenterID:    f.centerID,
		StatementID: f.statementID,
		Channel:     notifications.ChannelZaloPersonal,
		Purpose:     notifications.PurposeStatements,
		Status:      notifications.StatusQueued,
		RunID:       &runID,
	}
	require.NoError(t, repo.InsertBatch(context.Background(), []*notifications.Notification{n}))
	return n.ID
}

func strPtr(s string) *string { return &s }

func TestRunLifecycleIsDBBacked(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()
	f := seedStatement(t, db)
	sc := f.scope()

	// No run yet: nothing active, nothing to snapshot.
	active, err := repo.HasActiveRun(ctx, sc)
	require.NoError(t, err)
	require.False(t, active)
	_, err = repo.LatestRunByPeriod(ctx, sc, f.periodID)
	require.ErrorIs(t, err, notifications.ErrRunNotFound)

	run := seedRun(t, repo, f)
	rowSent := seedRunRow(t, repo, f, run.ID)
	rowFailed := seedRunRow(t, repo, f, run.ID)
	seedRunRow(t, repo, f, run.ID)

	active, err = repo.HasActiveRun(ctx, sc)
	require.NoError(t, err)
	require.True(t, active)
	// Another teacher's run must not read as this teacher's.
	other := seedStatement(t, db)
	active, err = repo.HasActiveRun(ctx, other.scope())
	require.NoError(t, err)
	require.False(t, active)

	require.NoError(t, repo.MarkOutcome(ctx, sc, rowSent, notifications.StatusSent, strPtr("msg-1"), nil))
	require.NoError(t, repo.MarkOutcome(ctx, sc, rowFailed, notifications.StatusFailed, nil, strPtr("friend refused")))

	counts, err := repo.RunCounts(ctx, sc, run.ID)
	require.NoError(t, err)
	require.Equal(t, notifications.RunCounts{Total: 3, Sent: 1, Failed: 1}, counts)

	require.NoError(t, repo.UpdateRunStatus(ctx, sc, run.ID, notifications.RunStatusCompleted))
	active, err = repo.HasActiveRun(ctx, sc)
	require.NoError(t, err)
	require.False(t, active, "a completed run is no longer active")

	got, err := repo.LatestRunByPeriod(ctx, sc, f.periodID)
	require.NoError(t, err)
	require.Equal(t, run.ID, got.ID)
	require.Equal(t, notifications.RunStatusCompleted, got.Status)
	require.NotNil(t, got.FinishedAt, "a terminal status must stamp finished_at")

	// Reopening the run (manual resume) clears the finish stamp.
	require.NoError(t, repo.UpdateRunStatus(ctx, sc, run.ID, notifications.RunStatusRunning))
	got, err = repo.LatestRunByPeriod(ctx, sc, f.periodID)
	require.NoError(t, err)
	require.Nil(t, got.FinishedAt)

	// The other teacher can neither read nor move this run.
	_, err = repo.LatestRunByPeriod(ctx, other.scope(), f.periodID)
	require.ErrorIs(t, err, notifications.ErrRunNotFound)
}

func TestMarkOutcomeOnlyMovesQueuedRowsOfTheirOwnTeacher(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()
	f := seedStatement(t, db)
	run := seedRun(t, repo, f)
	rowID := seedRunRow(t, repo, f, run.ID)

	// Another teacher marking this row must change nothing.
	other := seedStatement(t, db)
	require.NoError(t, repo.MarkOutcome(ctx, other.scope(), rowID, notifications.StatusSent, strPtr("stolen"), nil))
	var status string
	require.NoError(t, db.Table("notifications").Select("status").Where("id = ?", rowID).Take(&status).Error)
	require.Equal(t, notifications.StatusQueued, status)

	require.NoError(t, repo.MarkOutcome(ctx, f.scope(), rowID, notifications.StatusSent, strPtr("msg-9"), nil))
	var row struct {
		Status        string
		ProviderMsgID *string
		ErrorMessage  *string
		SentAt        *string
	}
	require.NoError(t, db.Table("notifications").
		Select("status, provider_msg_id, error_message, sent_at::text AS sent_at").
		Where("id = ?", rowID).Take(&row).Error)
	require.Equal(t, notifications.StatusSent, row.Status)
	require.NotNil(t, row.ProviderMsgID)
	require.Equal(t, "msg-9", *row.ProviderMsgID)
	require.NotNil(t, row.SentAt, "a sent outcome must stamp sent_at")

	// A row already sent is final: a late failed outcome must not rewrite it.
	require.NoError(t, repo.MarkOutcome(ctx, f.scope(), rowID, notifications.StatusFailed, nil, strPtr("late error")))
	require.NoError(t, db.Table("notifications").Select("status").Where("id = ?", rowID).Take(&status).Error)
	require.Equal(t, notifications.StatusSent, status)
}

func TestFailQueuedInRunSparesFinishedRowsAndOtherRuns(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()
	f := seedStatement(t, db)
	run := seedRun(t, repo, f)
	sentRow := seedRunRow(t, repo, f, run.ID)
	queuedRow := seedRunRow(t, repo, f, run.ID)
	require.NoError(t, repo.MarkOutcome(ctx, f.scope(), sentRow, notifications.StatusSent, nil, nil))

	// A second run of the SAME teacher keeps its queued rows — the sweep is
	// scoped by run, not by teacher. The first run steps aside (only one may
	// be running per teacher) but its queued rows stay swept-able.
	require.NoError(t, repo.UpdateRunStatus(ctx, f.scope(), run.ID, notifications.RunStatusInterrupted))
	otherPeriod := seedSecondPeriodStatement(t, db, f)
	otherRun := seedRun(t, repo, otherPeriod)
	otherRow := seedRunRow(t, repo, otherPeriod, otherRun.ID)

	require.NoError(t, repo.FailQueuedInRun(ctx, f.scope(), run.ID, "phiên Zalo hết hạn"))

	var got struct {
		Status       string
		ErrorMessage *string
	}
	require.NoError(t, db.Table("notifications").Select("status, error_message").Where("id = ?", queuedRow).Take(&got).Error)
	require.Equal(t, notifications.StatusFailed, got.Status)
	require.NotNil(t, got.ErrorMessage)
	require.Equal(t, "phiên Zalo hết hạn", *got.ErrorMessage)

	var status string
	require.NoError(t, db.Table("notifications").Select("status").Where("id = ?", sentRow).Take(&status).Error)
	require.Equal(t, notifications.StatusSent, status, "a row already sent must survive the sweep")
	require.NoError(t, db.Table("notifications").Select("status").Where("id = ?", otherRow).Take(&status).Error)
	require.Equal(t, notifications.StatusQueued, status, "another run's rows are not this run's failures")

	// The ledger read must surface the failure reason — it is the only place
	// a teacher can learn why a row was not delivered.
	listed, err := repo.ListByPeriod(ctx, f.scope(), f.periodID, notifications.ListFilter{Status: notifications.StatusFailed})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.NotNil(t, listed[0].ErrorMessage)
	require.Equal(t, "phiên Zalo hết hạn", *listed[0].ErrorMessage)
	// The row must also carry which run failed it, so a client can pin a
	// run's failures to that run alone instead of the whole ledger.
	require.NotNil(t, listed[0].RunID)
	require.Equal(t, run.ID, *listed[0].RunID)
}

func TestQueuedRunRowsReturnsOnlyTheRunsQueuedRows(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()
	f := seedStatement(t, db)
	run := seedRun(t, repo, f)
	queuedRow := seedRunRow(t, repo, f, run.ID)
	sentRow := seedRunRow(t, repo, f, run.ID)
	require.NoError(t, repo.MarkOutcome(ctx, f.scope(), sentRow, notifications.StatusSent, nil, nil))

	rows, err := repo.QueuedRunRows(ctx, f.scope(), run.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, queuedRow, rows[0].NotificationID)
	require.Equal(t, f.statementID, rows[0].StatementID)
	require.Equal(t, f.contactID, rows[0].ContactID)
}

func TestZaloMappingsReturnsLiveMappedContactsOnly(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()

	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	mapped := testutil.Contact(t, db, teacher.ID)
	unmapped := testutil.Contact(t, db, teacher.ID)
	deleted := testutil.Contact(t, db, teacher.ID)
	require.NoError(t, db.Exec(`UPDATE contacts SET zalo_user_id = 'uid-1', zalo_name = 'Chị Hà' WHERE id = ?`, mapped.ID).Error)
	require.NoError(t, db.Exec(`UPDATE contacts SET zalo_user_id = 'uid-2', zalo_name = 'Cũ', deleted_at = now() WHERE id = ?`, deleted.ID).Error)

	// The same friend id under another teacher must never bleed in.
	_, otherTeacher := testutil.Teacher(t, db)
	otherContact := testutil.Contact(t, db, otherTeacher.ID)
	require.NoError(t, db.Exec(`UPDATE contacts SET zalo_user_id = 'uid-1', zalo_name = 'Khác' WHERE id = ?`, otherContact.ID).Error)

	got, err := repo.ZaloMappings(ctx, sc, []uuid.UUID{mapped.ID, unmapped.ID, deleted.ID, otherContact.ID})
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]string{mapped.ID: "uid-1"}, got)
}

// The database allows one running run per teacher (partial unique index);
// the repository turns that violation into ErrRunActive so callers can answer
// with a conflict instead of a server error. This is the cross-process guard —
// the in-memory reservation cannot see another API instance.
func TestRunWritesSurfaceTheActiveRunConflict(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()
	f := seedStatement(t, db)
	first := seedRun(t, repo, f)

	second := &notifications.Run{
		ID:              id.New(),
		TeacherID:       f.teacherID,
		CenterID:        f.centerID,
		BillingPeriodID: f.periodID,
		Purpose:         notifications.PurposeStatements,
		Status:          notifications.RunStatusRunning,
	}
	require.ErrorIs(t, repo.CreateRun(ctx, second), notifications.ErrRunActive)

	// Reopening an interrupted run while another run is live must refuse the
	// same way — that is resume racing a fresh bulk send across processes.
	require.NoError(t, repo.UpdateRunStatus(ctx, f.scope(), first.ID, notifications.RunStatusInterrupted))
	otherPeriod := seedSecondPeriodStatement(t, db, f)
	seedRun(t, repo, otherPeriod)
	require.ErrorIs(t, repo.UpdateRunStatus(ctx, f.scope(), first.ID, notifications.RunStatusRunning),
		notifications.ErrRunActive)
}

func TestMarkInterruptedReconcilesEveryRunningRun(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := notifications.NewRepository(db)
	ctx := context.Background()

	running := seedStatement(t, db)
	runningRun := seedRun(t, repo, running)
	seedRunRow(t, repo, running, runningRun.ID)

	finished := seedStatement(t, db)
	finishedRun := seedRun(t, repo, finished)
	require.NoError(t, repo.UpdateRunStatus(ctx, finished.scope(), finishedRun.ID, notifications.RunStatusCompleted))

	n, err := repo.MarkInterrupted(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	got, err := repo.LatestRunByPeriod(ctx, running.scope(), running.periodID)
	require.NoError(t, err)
	require.Equal(t, notifications.RunStatusInterrupted, got.Status)
	// Its rows stay queued: interruption is the process dying, not the sends failing.
	var status string
	require.NoError(t, db.Table("notifications").Select("status").Where("run_id = ?", runningRun.ID).Take(&status).Error)
	require.Equal(t, notifications.StatusQueued, status)

	got, err = repo.LatestRunByPeriod(ctx, finished.scope(), finished.periodID)
	require.NoError(t, err)
	require.Equal(t, notifications.RunStatusCompleted, got.Status, "a finished run is not the reconciler's business")
}
