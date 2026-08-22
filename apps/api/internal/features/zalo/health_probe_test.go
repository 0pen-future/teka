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

type verifySpy struct {
	mu   sync.Mutex
	seen []uuid.UUID
	fail map[uuid.UUID]error
}

func (v *verifySpy) verify(_ context.Context, teacherID uuid.UUID) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = append(v.seen, teacherID)
	return v.fail[teacherID]
}

func (v *verifySpy) visited() []uuid.UUID {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]uuid.UUID(nil), v.seen...)
}

func staticAccounts(ids ...uuid.UUID) func(context.Context) ([]uuid.UUID, error) {
	return func(context.Context) ([]uuid.UUID, error) { return ids, nil }
}

func TestHealthProbeVerifiesEveryLinkedAccount(t *testing.T) {
	t.Parallel()

	first, second := uuid.New(), uuid.New()
	spy := &verifySpy{}
	probe := NewHealthProbe(staticAccounts(first, second), spy.verify, nil, ProbeOptions{})

	probe.Sweep(context.Background())

	require.ElementsMatch(t, []uuid.UUID{first, second}, spy.visited())
}

// Pinging Zalo when nothing is linked is pure risk with no benefit.
func TestHealthProbeSkipsTheSweepWhenNothingIsLinked(t *testing.T) {
	t.Parallel()

	spy := &verifySpy{}
	probe := NewHealthProbe(staticAccounts(), spy.verify, nil, ProbeOptions{})

	probe.Sweep(context.Background())

	require.Empty(t, spy.visited())
}

func TestHealthProbeKeepsSweepingAfterOneAccountFails(t *testing.T) {
	t.Parallel()

	broken, healthy := uuid.New(), uuid.New()
	spy := &verifySpy{fail: map[uuid.UUID]error{broken: errors.New("session rejected")}}
	probe := NewHealthProbe(staticAccounts(broken, healthy), spy.verify, nil, ProbeOptions{})

	probe.Sweep(context.Background())

	require.ElementsMatch(t, []uuid.UUID{broken, healthy}, spy.visited(),
		"one dead session must not hide the state of the others")
}

func TestHealthProbeStopsMidSweepWhenShutdownStarts(t *testing.T) {
	t.Parallel()

	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	ctx, cancel := context.WithCancel(context.Background())
	spy := &verifySpy{}
	verify := func(c context.Context, teacherID uuid.UUID) error {
		cancel()
		return spy.verify(c, teacherID)
	}
	probe := NewHealthProbe(staticAccounts(ids...), verify, nil, ProbeOptions{})

	probe.Sweep(ctx)

	require.Len(t, spy.visited(), 1, "shutdown must not wait for the whole roster")
}

func TestHealthProbeToleratesAFailedListing(t *testing.T) {
	t.Parallel()

	spy := &verifySpy{}
	accounts := func(context.Context) ([]uuid.UUID, error) { return nil, errors.New("database is down") }
	probe := NewHealthProbe(accounts, spy.verify, nil, ProbeOptions{})

	require.NotPanics(t, func() { probe.Sweep(context.Background()) })
	require.Empty(t, spy.visited())
}

func TestHealthProbeRunSweepsOnTheIntervalAndStopsOnShutdown(t *testing.T) {
	t.Parallel()

	spy := &verifySpy{}
	probe := NewHealthProbe(staticAccounts(uuid.New()), spy.verify, nil, ProbeOptions{
		Interval: 5 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		probe.Run(ctx)
		close(stopped)
	}()

	require.Eventually(t, func() bool { return len(spy.visited()) >= 2 }, 2*time.Second, 5*time.Millisecond)

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the probe leaked its goroutine past shutdown")
	}
}

// The behaviour the probe exists for: a linked account Zalo no longer accepts
// is reported as expired before the teacher tries to use it.
func TestHealthProbeExpiresAnAccountZaloRejectsAndLeavesHealthyOnesAlone(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	broken, healthy := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{broken, healthy} {
		repo.accounts[id] = &Account{
			TeacherID:            id,
			EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
			Status:               StatusLinked,
			ConsentVersion:       testConsentVersion,
		}
	}

	relogin := func(_ context.Context, _ *protocol.Session, cred protocol.Credentials) error {
		if cred.IMEI == "broken" {
			return errors.New("session rejected")
		}
		return nil
	}
	repo.accounts[broken].EncryptedCredentials = sealCredentials(t, protocol.Credentials{IMEI: "broken", UserAgent: "ua"})

	svc := newTestService(t, repo, ServiceOptions{Relogin: relogin})
	svc.cache.Put(broken, protocol.NewSession())
	svc.cache.Put(healthy, protocol.NewSession())

	probe := NewHealthProbe(svc.LinkedTeachers, svc.VerifyAccount, nil, ProbeOptions{})
	probe.Sweep(context.Background())

	require.Equal(t, StatusExpired, repo.stored(t, broken).Status)
	_, cached := svc.cache.Get(broken)
	require.False(t, cached, "a dead session must be dropped, not served to the next caller")

	require.Equal(t, StatusLinked, repo.stored(t, healthy).Status)
	require.NotNil(t, repo.stored(t, healthy).LastVerifiedAt, "a passing check records when it passed")
}

// The service owns the probe's goroutine so that shutting the service down
// stops it, whether or not the context it was started with is cancelled first.
func TestServiceStartsAndStopsTheHealthProbe(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}

	spy := &reloginSpy{}
	svc := NewService(repo, testCipher(t), ServiceOptions{Relogin: spy.relogin})
	svc.StartHealthProbe(context.Background(), ProbeOptions{Interval: 5 * time.Millisecond})
	svc.StartHealthProbe(context.Background(), ProbeOptions{Interval: 5 * time.Millisecond})

	require.Eventually(t, func() bool {
		calls, _ := spy.snapshot()
		return calls >= 1
	}, 2*time.Second, 5*time.Millisecond, "the probe never swept")

	stopped := make(chan struct{})
	go func() {
		svc.Close()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not stop the health probe")
	}

	svc.Close()
}
