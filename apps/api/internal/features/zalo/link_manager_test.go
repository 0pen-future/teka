package zalo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/zalo/protocol"
)

const testConsentVersion = "2026-08-06"

// recordingLinker stands in for the persistence side of a successful attempt so
// the manager can be exercised without a database or a cipher.
type recordingLinker struct {
	mu      sync.Mutex
	calls   int
	teacher uuid.UUID
	consent string
	cred    protocol.Credentials
	name    string
	fail    error
}

func (r *recordingLinker) onLinked(_ context.Context, teacherID uuid.UUID, consentVersion string, sess *protocol.Session, cred *protocol.Credentials) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.teacher = teacherID
	r.consent = consentVersion
	r.cred = *cred
	r.name = sess.DisplayName
	return r.fail
}

func (r *recordingLinker) snapshot() recordingLinker {
	r.mu.Lock()
	defer r.mu.Unlock()
	return recordingLinker{calls: r.calls, teacher: r.teacher, consent: r.consent, cred: r.cred, name: r.name}
}

func newTestManager(t *testing.T, login LoginFunc, linker *recordingLinker, opts LinkOptions) *LinkManager {
	t.Helper()
	m := NewLinkManager(login, linker.onLinked, nil, opts)
	t.Cleanup(m.Close)
	return m
}

// waitState polls until the attempt reaches want. The manager advances state
// only when the fake login does, so this cannot pass by accident.
func waitState(t *testing.T, m *LinkManager, teacherID, linkID uuid.UUID, want LinkState) LinkSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	last := LinkState("")
	for time.Now().Before(deadline) {
		snap, err := m.Status(teacherID, linkID)
		require.NoError(t, err)
		if snap.State == want {
			return snap
		}
		last = snap.State
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %q; last seen %q", want, last)
	return LinkSnapshot{}
}

func TestLinkManagerAdvancesThroughEveryQRMilestone(t *testing.T) {
	t.Parallel()

	step := make(chan struct{})
	login := func(_ context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("qr-png-bytes"))
		<-step
		cb.OnProgress(protocol.QRStateScanned)
		<-step
		cb.OnProgress(protocol.QRStateConfirmed)
		<-step
		sess.DisplayName = "Cô Lan"
		return &protocol.Credentials{IMEI: "imei-1", UserAgent: "ua"}, nil
	}

	linker := &recordingLinker{}
	m := newTestManager(t, login, linker, LinkOptions{})
	teacherID := uuid.New()

	linkID := m.Begin(teacherID, testConsentVersion)
	require.NotEqual(t, uuid.Nil, linkID)

	qr := waitState(t, m, teacherID, linkID, LinkStateQRReady)
	require.Equal(t, []byte("qr-png-bytes"), qr.QRPNG, "the PNG must be readable while the teacher scans")

	step <- struct{}{}
	waitState(t, m, teacherID, linkID, LinkStateScanned)
	step <- struct{}{}
	waitState(t, m, teacherID, linkID, LinkStateConfirmed)
	step <- struct{}{}

	linked := waitState(t, m, teacherID, linkID, LinkStateLinked)
	require.Equal(t, "Cô Lan", linked.DisplayName)
	require.Empty(t, linked.QRPNG, "a finished attempt has no reason to keep serving its QR code")

	got := linker.snapshot()
	require.Equal(t, 1, got.calls, "credentials must be persisted exactly once")
	require.Equal(t, teacherID, got.teacher)
	require.Equal(t, testConsentVersion, got.consent, "the acknowledged consent version must reach persistence")
	require.Equal(t, "imei-1", got.cred.IMEI)
	require.Equal(t, "Cô Lan", got.name)
}

func TestLinkManagerExpiresAnAttemptThatOutlivesItsDeadline(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	login := func(ctx context.Context, _ *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	linker := &recordingLinker{}
	m := newTestManager(t, login, linker, LinkOptions{AttemptTTL: 50 * time.Millisecond, Retention: time.Minute})
	teacherID := uuid.New()

	linkID := m.Begin(teacherID, testConsentVersion)
	<-started

	waitState(t, m, teacherID, linkID, LinkStateExpired)
	require.Zero(t, linker.snapshot().calls, "an expired attempt must persist nothing")
}

func TestLinkManagerSweepsAFinishedAttemptOnceItsRetentionPasses(t *testing.T) {
	t.Parallel()

	login := func(ctx context.Context, _ *protocol.Session, _ protocol.QRCallbacks) (*protocol.Credentials, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	m := newTestManager(t, login, &recordingLinker{}, LinkOptions{
		AttemptTTL: 20 * time.Millisecond,
		Retention:  100 * time.Millisecond,
	})
	teacherID := uuid.New()

	linkID := m.Begin(teacherID, testConsentVersion)
	waitState(t, m, teacherID, linkID, LinkStateExpired)

	require.Eventually(t, func() bool {
		_, err := m.Status(teacherID, linkID)
		return errors.Is(err, ErrLinkNotFound)
	}, 2*time.Second, 5*time.Millisecond, "a long-finished record must not be held forever")
}

func TestLinkManagerReportsAFailedLoginWithoutLeakingItsCause(t *testing.T) {
	t.Parallel()

	login := func(_ context.Context, _ *protocol.Session, _ protocol.QRCallbacks) (*protocol.Credentials, error) {
		return nil, errors.New("zalo returned imei=secret-value cookie=zpsid-secret")
	}

	m := newTestManager(t, login, &recordingLinker{}, LinkOptions{})
	teacherID := uuid.New()

	linkID := m.Begin(teacherID, testConsentVersion)
	snap := waitState(t, m, teacherID, linkID, LinkStateError)
	require.NotEmpty(t, snap.Failure, "the client needs something to show")
	require.NotContains(t, snap.Failure, "secret-value", "an upstream error must never reach the client verbatim")
	require.NotContains(t, snap.Failure, "zpsid")
}

func TestLinkManagerReportsAFailedPersistAsAnError(t *testing.T) {
	t.Parallel()

	login := func(_ context.Context, _ *protocol.Session, _ protocol.QRCallbacks) (*protocol.Credentials, error) {
		return &protocol.Credentials{IMEI: "imei", UserAgent: "ua"}, nil
	}
	linker := &recordingLinker{fail: errors.New("database is down")}

	m := newTestManager(t, login, linker, LinkOptions{})
	teacherID := uuid.New()

	linkID := m.Begin(teacherID, testConsentVersion)
	snap := waitState(t, m, teacherID, linkID, LinkStateError)
	require.NotContains(t, snap.Failure, "database is down")
}

// Restarting the flow from the profile page must not leave the previous QR
// long-poll running against Zalo.
func TestLinkManagerSupersedesTheTeachersPreviousAttempt(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	cancelled := 0
	login := func(ctx context.Context, _ *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		<-ctx.Done()
		mu.Lock()
		cancelled++
		mu.Unlock()
		return nil, ctx.Err()
	}

	m := newTestManager(t, login, &recordingLinker{}, LinkOptions{AttemptTTL: 5 * time.Second, Retention: time.Minute})
	teacherID := uuid.New()

	first := m.Begin(teacherID, testConsentVersion)
	waitState(t, m, teacherID, first, LinkStateQRReady)

	second := m.Begin(teacherID, testConsentVersion)
	require.NotEqual(t, first, second)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return cancelled == 1
	}, 2*time.Second, 5*time.Millisecond, "the superseded attempt must be cancelled")

	_, err := m.Status(teacherID, first)
	require.ErrorIs(t, err, ErrLinkNotFound, "the superseded link id is no longer readable")
	waitState(t, m, teacherID, second, LinkStateQRReady)
}

// A link id is only meaningful for the teacher who started it; another teacher
// presenting it must learn nothing.
func TestLinkManagerRefusesAnotherTeachersLinkID(t *testing.T) {
	t.Parallel()

	login := func(ctx context.Context, _ *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		<-ctx.Done()
		return nil, ctx.Err()
	}

	m := newTestManager(t, login, &recordingLinker{}, LinkOptions{AttemptTTL: 5 * time.Second, Retention: time.Minute})
	owner := uuid.New()
	linkID := m.Begin(owner, testConsentVersion)
	waitState(t, m, owner, linkID, LinkStateQRReady)

	_, err := m.Status(uuid.New(), linkID)
	require.ErrorIs(t, err, ErrLinkNotFound)

	_, err = m.Status(owner, uuid.New())
	require.ErrorIs(t, err, ErrLinkNotFound)
}

func TestLinkManagerCancelStopsTheAttempt(t *testing.T) {
	t.Parallel()

	login := func(ctx context.Context, _ *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		<-ctx.Done()
		return nil, ctx.Err()
	}

	m := newTestManager(t, login, &recordingLinker{}, LinkOptions{AttemptTTL: 5 * time.Second, Retention: time.Minute})
	teacherID := uuid.New()
	linkID := m.Begin(teacherID, testConsentVersion)
	waitState(t, m, teacherID, linkID, LinkStateQRReady)

	m.Cancel(teacherID)

	_, err := m.Status(teacherID, linkID)
	require.ErrorIs(t, err, ErrLinkNotFound)
}

// Shutdown must not leave a QR long-poll running.
func TestLinkManagerCloseEndsEveryInFlightGoroutine(t *testing.T) {
	t.Parallel()

	login := func(ctx context.Context, _ *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		<-ctx.Done()
		return nil, ctx.Err()
	}

	m := NewLinkManager(login, (&recordingLinker{}).onLinked, nil, LinkOptions{
		AttemptTTL: time.Hour,
		Retention:  time.Hour,
	})

	teachers := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	done := make([]<-chan struct{}, 0, len(teachers))
	for _, teacherID := range teachers {
		linkID := m.Begin(teacherID, testConsentVersion)
		waitState(t, m, teacherID, linkID, LinkStateQRReady)
		m.mu.Lock()
		done = append(done, m.active[teacherID].done)
		m.mu.Unlock()
	}

	m.Close()

	for i, ch := range done {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("attempt %d leaked its goroutine past Close", i)
		}
	}
}
