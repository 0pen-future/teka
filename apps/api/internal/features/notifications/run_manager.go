package notifications

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/zalo"
)

// ErrRunBusy reports that the manager cannot start another run for this
// teacher right now — either one is already sending, or the manager has been
// closed for shutdown.
var ErrRunBusy = errors.New("a notification run is already active")

// Ledger-facing failure texts. Upstream error strings never reach the ledger:
// Zalo writes them, they may quote the request that produced them, and the
// teacher can act on neither — the detail is logged instead.
const (
	// runSendFailureMessage marks one recipient's failed send.
	runSendFailureMessage = "Gửi thất bại"
	// runExpiredFailureMessage marks the rows swept when the Zalo session died
	// mid-run.
	runExpiredFailureMessage = "Phiên Zalo đã hết hạn"
)

// runWriteTimeout bounds each outcome write. Writes ride a context that
// survives the run's cancellation — a shutdown must not lose the verdict of a
// send that already reached Zalo.
const runWriteTimeout = 10 * time.Second

// runBreakerThreshold is how many straight send failures stop a run. Distinct
// recipients failing for distinct reasons is normal; every send failing in a
// row means the account itself is in trouble (rate-limited, soft-blocked), and
// pushing on would burn the whole list. The untried rows stay queued and the
// run becomes interrupted, so the teacher resumes manually once the account
// recovers.
const runBreakerThreshold = 3

// RunItem is one send of a run: the ledger row it reports to, the Zalo user
// to reach, and the rendered text. Text lives only here, in memory — the
// database never stores message bodies, so a run interrupted mid-flight
// re-renders its remaining texts on resume instead of reading them back.
type RunItem struct {
	NotificationID uuid.UUID
	ToUID          string
	Text           string
}

// RunStore is the slice of Repository a run needs to record progress. Tests
// supply a fake; *gormRepository satisfies it.
type RunStore interface {
	MarkOutcome(ctx context.Context, teacherID, id uuid.UUID, status string, providerMsgID, errorMessage *string) error
	FailQueuedInRun(ctx context.Context, teacherID, runID uuid.UUID, reason string) error
	UpdateRunStatus(ctx context.Context, teacherID, runID uuid.UUID, status string) error
}

// DMSender is the consumer-defined slice of the Zalo service a run sends
// through. *zalo.Service satisfies it.
type DMSender interface {
	SendDM(ctx context.Context, teacherID uuid.UUID, toUID, text string) (string, error)
}

type runJob struct {
	teacherID uuid.UUID
	runID     uuid.UUID
	cancel    context.CancelFunc
	done      chan struct{}
}

// RunManager drives paced background sending passes. One run per teacher is
// live at a time, and every send is separated from the next by a randomized
// gap — the pacing exists to keep a teacher's personal account looking like a
// person, so nothing here retries, hurries, or parallelizes.
//
// The manager holds no run state worth keeping: progress lives in the
// notifications rows and the run record, both DB-backed, so a restart loses
// only the in-memory texts — boot reconcile marks such runs interrupted and a
// manual resume re-renders them.
type RunManager struct {
	mu     sync.Mutex
	active map[uuid.UUID]*runJob
	closed bool

	store  RunStore
	sender DMSender
	log    *slog.Logger

	paceMin time.Duration
	paceMax time.Duration
	// sleep waits out one pacing gap, reporting false when the run was
	// cancelled instead. Tests inject a recorder so no test sleeps for real.
	sleep func(ctx context.Context, d time.Duration) bool

	// baseCtx is the manager's lifetime, not any request's: every run derives
	// from it so Close can end them all at shutdown.
	baseCtx    context.Context
	baseCancel context.CancelFunc
}

// NewRunManager builds a manager sending through sender and recording through
// store, pacing sends paceMin to paceMax apart. A nil logger falls back to
// slog.Default.
func NewRunManager(store RunStore, sender DMSender, log *slog.Logger, paceMin, paceMax time.Duration) *RunManager {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &RunManager{
		active:     make(map[uuid.UUID]*runJob),
		store:      store,
		sender:     sender,
		log:        log,
		paceMin:    paceMin,
		paceMax:    paceMax,
		sleep:      sleepUnlessCancelled,
		baseCtx:    ctx,
		baseCancel: cancel,
	}
}

func sleepUnlessCancelled(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// RunReservation holds one teacher's run slot between the decision to start a
// run and the commit of that run's rows. Claiming the slot before the
// transaction closes the race two concurrent calls would otherwise have:
// without it, both commit their writes and the loser only finds out at start
// time — after it already mutated a run record the winner is sending.
// Exactly one of Start or Release must eventually be called; Release after
// Start is a no-op, so `defer res.Release()` is the intended usage.
type RunReservation struct {
	m       *RunManager
	ctx     context.Context
	job     *runJob
	settled bool
}

// Reserve claims teacherID's run slot without starting anything. ErrRunBusy
// means a run is already sending for this teacher, or the process is shutting
// down — either way the caller must not create a run.
func (m *RunManager) Reserve(teacherID uuid.UUID) (*RunReservation, error) {
	ctx, cancel := context.WithCancel(m.baseCtx)
	job := &runJob{
		teacherID: teacherID,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	m.mu.Lock()
	if m.closed || m.active[teacherID] != nil {
		m.mu.Unlock()
		cancel()
		return nil, ErrRunBusy
	}
	m.active[teacherID] = job
	m.mu.Unlock()

	return &RunReservation{m: m, ctx: ctx, job: job}, nil
}

// Start consumes the reservation and begins sending in the background. It
// always launches the goroutine, even during shutdown: the goroutine observes
// the cancelled context, exits without writing, and — crucially — closes the
// job's done channel so a concurrent Close is never left waiting.
func (r *RunReservation) Start(runID uuid.UUID, items []RunItem) {
	if r.settled {
		return
	}
	r.settled = true
	r.job.runID = runID
	go r.m.run(r.ctx, r.job, items)
}

// Release frees the slot when the run never materialized — the transaction
// failed, or there was nothing to send. A no-op once Start has run.
func (r *RunReservation) Release() {
	if r.settled {
		return
	}
	r.settled = true
	r.m.release(r.job)
	r.job.cancel()
	close(r.job.done)
}

// Start reserves and immediately starts in one call, for callers whose items
// already exist. ErrRunBusy as in Reserve.
func (m *RunManager) Start(teacherID, runID uuid.UUID, items []RunItem) error {
	res, err := m.Reserve(teacherID)
	if err != nil {
		return err
	}
	res.Start(runID, items)
	return nil
}

// Close stops every live run and waits for each goroutine to return. Runs cut
// short this way write nothing further: their remaining rows stay queued and
// their run records stay running, which the next boot's reconcile turns into
// interrupted. Safe to call more than once.
func (m *RunManager) Close() {
	m.baseCancel()

	m.mu.Lock()
	m.closed = true
	pending := make([]*runJob, 0, len(m.active))
	for _, job := range m.active {
		pending = append(pending, job)
	}
	m.mu.Unlock()

	for _, job := range pending {
		job.cancel()
		<-job.done
	}
}

func (m *RunManager) run(ctx context.Context, job *runJob, items []RunItem) {
	// Announcing the goroutine's exit is the last thing it does, so Close
	// knows a job it waited for can no longer write.
	defer close(job.done)
	defer m.release(job)
	defer job.cancel()

	failureStreak := 0
	for i, item := range items {
		if i > 0 && !m.sleep(ctx, m.gap()) {
			return
		}
		if ctx.Err() != nil {
			return
		}

		msgID, err := m.sender.SendDM(ctx, job.teacherID, item.ToUID, item.Text)
		if err != nil {
			if ctx.Err() != nil {
				// The shutdown aborted this send, not Zalo — the row stays
				// queued for a resume.
				return
			}
			if errors.Is(err, zalo.ErrLinkExpired) || errors.Is(err, zalo.ErrNotLinked) {
				m.expireRun(ctx, job)
				return
			}
			m.log.Warn("notification run: send failed",
				"teacher_id", job.teacherID, "notification_id", item.NotificationID, "error", err)
			m.markOutcome(ctx, job, item.NotificationID, StatusFailed, nil, ptr(runSendFailureMessage))
			failureStreak++
			if failureStreak >= runBreakerThreshold {
				m.log.Warn("notification run: stopping after consecutive send failures",
					"teacher_id", job.teacherID, "run_id", job.runID, "failures", failureStreak)
				m.finishRun(ctx, job, RunStatusInterrupted)
				return
			}
			continue
		}
		failureStreak = 0

		var providerMsgID *string
		if msgID != "" {
			providerMsgID = &msgID
		}
		m.markOutcome(ctx, job, item.NotificationID, StatusSent, providerMsgID, nil)
	}

	m.finishRun(ctx, job, RunStatusCompleted)
}

// gap picks the pause before the next send. A max at or below min degrades to
// a fixed min-length gap rather than a panic — pacing must never be the thing
// that takes the process down.
func (m *RunManager) gap() time.Duration {
	if m.paceMax <= m.paceMin {
		return m.paceMin
	}
	return m.paceMin + rand.N(m.paceMax-m.paceMin) //nolint:gosec // pacing jitter, not a secret
}

// markOutcome records one row's verdict on a context that survives run
// cancellation: a send that already reached Zalo must be recorded even while
// the process is shutting down, or a resume would send it twice.
func (m *RunManager) markOutcome(ctx context.Context, job *runJob, id uuid.UUID, status string, providerMsgID, errorMessage *string) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runWriteTimeout)
	defer cancel()
	if err := m.store.MarkOutcome(writeCtx, job.teacherID, id, status, providerMsgID, errorMessage); err != nil {
		m.log.Error("notification run: recording outcome failed",
			"teacher_id", job.teacherID, "notification_id", id, "error", err)
	}
}

// expireRun is the dead-session ending: every remaining queued row fails with
// one sweep and the run record says expired, so the teacher sees exactly
// which recipients never got their message.
func (m *RunManager) expireRun(ctx context.Context, job *runJob) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runWriteTimeout)
	defer cancel()
	if err := m.store.FailQueuedInRun(writeCtx, job.teacherID, job.runID, runExpiredFailureMessage); err != nil {
		m.log.Error("notification run: sweeping rows after session expiry failed",
			"teacher_id", job.teacherID, "run_id", job.runID, "error", err)
	}
	if err := m.store.UpdateRunStatus(writeCtx, job.teacherID, job.runID, RunStatusExpired); err != nil {
		m.log.Error("notification run: marking run expired failed",
			"teacher_id", job.teacherID, "run_id", job.runID, "error", err)
	}
}

func (m *RunManager) finishRun(ctx context.Context, job *runJob, status string) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runWriteTimeout)
	defer cancel()
	if err := m.store.UpdateRunStatus(writeCtx, job.teacherID, job.runID, status); err != nil {
		m.log.Error("notification run: marking run finished failed",
			"teacher_id", job.teacherID, "run_id", job.runID, "error", err)
	}
}

func (m *RunManager) release(job *runJob) {
	m.mu.Lock()
	if m.active[job.teacherID] == job {
		delete(m.active, job.teacherID)
	}
	m.mu.Unlock()
}

func ptr[T any](v T) *T { return &v }
