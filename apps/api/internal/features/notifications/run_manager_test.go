package notifications

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/authctx"
)

type outcomeCall struct {
	notificationID uuid.UUID
	status         string
	providerMsgID  *string
	errorMessage   *string
}

// fakeRunStore records every write the manager makes, guarded because the
// manager writes from its own goroutine while the test reads.
type fakeRunStore struct {
	mu       sync.Mutex
	outcomes []outcomeCall
	failed   []string // reasons passed to FailQueuedInRun
	statuses []string // statuses passed to UpdateRunStatus
	// canSend answers the delegated per-item permission probe; nil means
	// always permitted, matching a run whose flag never moves.
	canSend func(call int) (bool, error)
	sendChk int // how many times CanSendReports was asked
}

func (s *fakeRunStore) CanSendReports(_ context.Context, _, _ uuid.UUID) (bool, error) {
	s.mu.Lock()
	call := s.sendChk
	s.sendChk++
	probe := s.canSend
	s.mu.Unlock()
	if probe == nil {
		return true, nil
	}
	return probe(call)
}

func (s *fakeRunStore) MarkOutcome(_ context.Context, _ authctx.Scope, id uuid.UUID, status string, providerMsgID, errorMessage *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = append(s.outcomes, outcomeCall{id, status, providerMsgID, errorMessage})
	return nil
}

func (s *fakeRunStore) FailQueuedInRun(_ context.Context, _ authctx.Scope, _ uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, reason)
	return nil
}

func (s *fakeRunStore) UpdateRunStatus(_ context.Context, _ authctx.Scope, _ uuid.UUID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, status)
	return nil
}

func (s *fakeRunStore) snapshot() (outcomes []outcomeCall, failed, statuses []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]outcomeCall(nil), s.outcomes...),
		append([]string(nil), s.failed...),
		append([]string(nil), s.statuses...)
}

func (s *fakeRunStore) lastStatus() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.statuses) == 0 {
		return ""
	}
	return s.statuses[len(s.statuses)-1]
}

type fakeDM struct {
	mu    sync.Mutex
	calls []string // toUIDs in send order
	send  func(call int, toUID string) (string, error)
}

func (d *fakeDM) SendDM(_ context.Context, _ uuid.UUID, toUID, _ string) (string, error) {
	d.mu.Lock()
	call := len(d.calls)
	d.calls = append(d.calls, toUID)
	d.mu.Unlock()
	return d.send(call, toUID)
}

func (d *fakeDM) sent() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

// recordedSleep replaces real pacing: it logs each requested gap and returns
// immediately, honouring cancellation like the real hook would.
type recordedSleep struct {
	mu   sync.Mutex
	gaps []time.Duration
}

func (r *recordedSleep) hook(ctx context.Context, d time.Duration) bool {
	r.mu.Lock()
	r.gaps = append(r.gaps, d)
	r.mu.Unlock()
	return ctx.Err() == nil
}

func (r *recordedSleep) recorded() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.gaps...)
}

func testItems(n int) []RunItem {
	items := make([]RunItem, n)
	for i := range items {
		items[i] = RunItem{NotificationID: uuid.New(), ToUID: "uid-" + string(rune('a'+i)), Text: "msg"}
	}
	return items
}

func newTestRunManager(t *testing.T, store *fakeRunStore, dm *fakeDM, paceMin, paceMax time.Duration) *RunManager {
	t.Helper()
	m := NewRunManager(store, dm, slog.New(slog.DiscardHandler), paceMin, paceMax)
	t.Cleanup(m.Close)
	return m
}

func waitForTerminalStatus(t *testing.T, store *fakeRunStore) {
	t.Helper()
	require.Eventually(t, func() bool {
		return store.lastStatus() != "" && store.lastStatus() != RunStatusRunning
	}, 5*time.Second, 5*time.Millisecond)
}

func TestRunManagerSendsEveryItemWithAPacedGapBetween(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(_ int, _ string) (string, error) { return "msg-1", nil }}
	m := newTestRunManager(t, store, dm, 3*time.Second, 8*time.Second)
	sleeper := &recordedSleep{}
	m.sleep = sleeper.hook

	items := testItems(3)
	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), items))
	waitForTerminalStatus(t, store)

	outcomes, failed, statuses := store.snapshot()
	require.Empty(t, failed)
	require.Equal(t, []string{RunStatusCompleted}, statuses)
	require.Len(t, outcomes, 3)
	for i, o := range outcomes {
		require.Equal(t, items[i].NotificationID, o.notificationID)
		require.Equal(t, StatusSent, o.status)
		require.NotNil(t, o.providerMsgID)
		require.Equal(t, "msg-1", *o.providerMsgID)
		require.Nil(t, o.errorMessage)
	}

	gaps := sleeper.recorded()
	require.Len(t, gaps, 2, "n items pace n-1 gaps — none before the first, none after the last")
	for _, gap := range gaps {
		require.GreaterOrEqual(t, gap, 3*time.Second)
		require.LessOrEqual(t, gap, 8*time.Second)
	}
}

func TestRunManagerRecordsAMissingMessageIDAsSentAllTheSame(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(int, string) (string, error) { return "", nil }}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(1)))
	waitForTerminalStatus(t, store)

	outcomes, _, _ := store.snapshot()
	require.Len(t, outcomes, 1)
	require.Equal(t, StatusSent, outcomes[0].status)
	require.Nil(t, outcomes[0].providerMsgID, "no message id is not a failure — Zalo sometimes acks without one")
}

func TestRunManagerFailsOneRowAndKeepsGoing(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(call int, _ string) (string, error) {
		if call == 1 {
			return "", errors.New("friend request pending")
		}
		return "msg-ok", nil
	}}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	items := testItems(3)
	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), items))
	waitForTerminalStatus(t, store)

	outcomes, failed, statuses := store.snapshot()
	require.Empty(t, failed)
	require.Equal(t, []string{RunStatusCompleted}, statuses, "one bad recipient must not end the run")
	require.Len(t, outcomes, 3)
	require.Equal(t, StatusSent, outcomes[0].status)
	require.Equal(t, StatusFailed, outcomes[1].status)
	require.NotNil(t, outcomes[1].errorMessage)
	require.NotContains(t, *outcomes[1].errorMessage, "friend request pending",
		"upstream error text never reaches the ledger — it is logged, not stored")
	require.Equal(t, StatusSent, outcomes[2].status)
}

func TestRunManagerExpiresTheRunWhenTheSessionDies(t *testing.T) {
	t.Parallel()
	for _, sessionErr := range []error{zalo.ErrLinkExpired, zalo.ErrNotLinked} {
		store := &fakeRunStore{}
		dm := &fakeDM{send: func(call int, _ string) (string, error) {
			if call == 1 {
				return "", sessionErr
			}
			return "msg-ok", nil
		}}
		m := newTestRunManager(t, store, dm, time.Second, time.Second)
		m.sleep = (&recordedSleep{}).hook

		require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(3)))
		waitForTerminalStatus(t, store)

		outcomes, failed, statuses := store.snapshot()
		require.Len(t, outcomes, 1, "a dead session stops the run — no send is attempted after it")
		require.Equal(t, StatusSent, outcomes[0].status)
		require.Len(t, failed, 1, "every remaining queued row is swept failed in one update")
		require.Equal(t, []string{RunStatusExpired}, statuses)
		require.Len(t, dm.sent(), 2)
	}
}

func TestRunManagerRefusesASecondConcurrentRunPerTeacher(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(int, string) (string, error) {
		<-release
		return "msg", nil
	}}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	teacherID := uuid.New()
	require.NoError(t, m.Start(teacherID, uuid.New(), uuid.New(), testItems(1)))
	require.ErrorIs(t, m.Start(teacherID, uuid.New(), uuid.New(), testItems(1)), ErrRunBusy)
	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(1)),
		"another teacher's run is not this teacher's business")
	close(release)
	waitForTerminalStatus(t, store)
}

func TestCloseStopsARunMidGapAndLeavesTheRestQueued(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(int, string) (string, error) { return "msg", nil }}
	// Real sleep hook with an hour-long gap: Close must cut it short.
	m := NewRunManager(store, dm, slog.New(slog.DiscardHandler), time.Hour, time.Hour)

	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(3)))
	require.Eventually(t, func() bool { return len(dm.sent()) == 1 }, 5*time.Second, 5*time.Millisecond)
	m.Close()

	outcomes, failed, statuses := store.snapshot()
	require.Len(t, outcomes, 1, "only the send that finished is recorded")
	require.Equal(t, StatusSent, outcomes[0].status)
	require.Empty(t, failed, "shutdown is not a send failure — rows stay queued for a resume")
	require.Empty(t, statuses, "the run stays running in the DB; boot reconcile marks it interrupted")
	require.Len(t, dm.sent(), 1)

	require.ErrorIs(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(1)), ErrRunBusy,
		"a closed manager starts nothing")
}

func TestRunManagerGapNeverPanicsWhenMinEqualsMax(t *testing.T) {
	t.Parallel()
	m := NewRunManager(&fakeRunStore{}, &fakeDM{}, slog.New(slog.DiscardHandler), 5*time.Second, 5*time.Second)
	defer m.Close()
	for range 10 {
		require.Equal(t, 5*time.Second, m.gap())
	}
	// A misconfigured max below min degrades to the min, never a panic.
	m2 := NewRunManager(&fakeRunStore{}, &fakeDM{}, slog.New(slog.DiscardHandler), 5*time.Second, 2*time.Second)
	defer m2.Close()
	require.Equal(t, 5*time.Second, m2.gap())
}

func TestReserveHoldsTheTeacherSlotUntilReleasedOrStarted(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(_ int, _ string) (string, error) { return "msg-1", nil }}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	sleeper := &recordedSleep{}
	m.sleep = sleeper.hook
	teacherID := uuid.New()

	res, err := m.Reserve(teacherID, uuid.New())
	require.NoError(t, err)

	_, err = m.Reserve(teacherID, uuid.New())
	require.ErrorIs(t, err, ErrRunBusy, "the slot is taken from Reserve, not from Start")
	require.Empty(t, dm.sent(), "a reservation alone sends nothing")

	res.Release()
	res.Release() // releasing twice must be harmless

	res2, err := m.Reserve(teacherID, uuid.New())
	require.NoError(t, err, "a released slot is free again")
	res2.Start(uuid.New(), testItems(1), false)
	res2.Release() // a no-op after Start: it must not kill the running send
	waitForTerminalStatus(t, store)
	_, _, statuses := store.snapshot()
	require.Equal(t, []string{RunStatusCompleted}, statuses)
	require.Len(t, dm.sent(), 1)
}

func TestRunManagerTripsAfterConsecutiveSendFailures(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	dm := &fakeDM{send: func(int, string) (string, error) {
		return "", errors.New("account temporarily blocked")
	}}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(5)))
	waitForTerminalStatus(t, store)

	outcomes, failed, statuses := store.snapshot()
	require.Len(t, dm.sent(), 3, "the breaker trips on the third straight failure — the rest is never attempted")
	require.Len(t, outcomes, 3)
	for _, o := range outcomes {
		require.Equal(t, StatusFailed, o.status)
	}
	require.Empty(t, failed, "tripping is not a sweep — untried rows stay queued for a resume")
	require.Equal(t, []string{RunStatusInterrupted}, statuses,
		"a tripped run is resumable, exactly like one cut by a crash")
}

func TestRunManagerBreakerResetsOnASuccessfulSend(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{}
	// Two failures, a success, two failures, a success: three-in-a-row never
	// happens, so every item must be attempted.
	dm := &fakeDM{send: func(call int, _ string) (string, error) {
		if call == 2 || call == 5 {
			return "msg-ok", nil
		}
		return "", errors.New("transient send error")
	}}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	items := testItems(6)
	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), items))
	waitForTerminalStatus(t, store)

	outcomes, failed, statuses := store.snapshot()
	require.Len(t, dm.sent(), 6, "a success resets the streak — the run keeps going")
	require.Len(t, outcomes, 6)
	require.Empty(t, failed)
	require.Equal(t, []string{RunStatusCompleted}, statuses)
}

func TestDelegatedRunStopsWhenThePermissionIsRevokedMidRun(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{canSend: func(call int) (bool, error) {
		// Permitted for the first item, revoked before the second — the
		// remaining rows must fail with the revoked reason, not stay queued.
		return call == 0, nil
	}}
	dm := &fakeDM{send: func(int, string) (string, error) { return "msg-1", nil }}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	res, err := m.Reserve(uuid.New(), uuid.New())
	require.NoError(t, err)
	res.Start(uuid.New(), testItems(3), true)
	waitForTerminalStatus(t, store)

	outcomes, failed, statuses := store.snapshot()
	require.Len(t, outcomes, 1, "only the pre-revocation item was sent")
	require.Equal(t, StatusSent, outcomes[0].status)
	require.Len(t, dm.sent(), 1)
	require.Equal(t, []string{runRevokedFailureMessage}, failed)
	require.Equal(t, []string{RunStatusInterrupted}, statuses)
}

func TestOwnRunNeverProbesTheSendPermission(t *testing.T) {
	t.Parallel()
	store := &fakeRunStore{canSend: func(int) (bool, error) {
		t.Error("a non-delegated run must not ask about can_send_reports")
		return false, nil
	}}
	dm := &fakeDM{send: func(int, string) (string, error) { return "msg-1", nil }}
	m := newTestRunManager(t, store, dm, time.Second, time.Second)
	m.sleep = (&recordedSleep{}).hook

	require.NoError(t, m.Start(uuid.New(), uuid.New(), uuid.New(), testItems(2)))
	waitForTerminalStatus(t, store)

	_, _, statuses := store.snapshot()
	require.Equal(t, []string{RunStatusCompleted}, statuses)
}

func TestCloseDoesNotHangOnAPendingReservation(t *testing.T) {
	t.Parallel()
	m := newTestRunManager(t, &fakeRunStore{}, &fakeDM{}, time.Second, time.Second)

	res, err := m.Reserve(uuid.New(), uuid.New())
	require.NoError(t, err)

	closed := make(chan struct{})
	go func() {
		m.Close()
		close(closed)
	}()
	// Close must wait for the reservation to settle, then return promptly.
	res.Release()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close never returned after the pending reservation was released")
	}
}
