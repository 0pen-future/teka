package zalo

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/zalo/protocol"
)

// LinkState is the stage of a QR link attempt, as a waiting teacher sees it.
type LinkState string

const (
	// LinkStatePending is the moment between starting an attempt and Zalo
	// handing back a QR image.
	LinkStatePending LinkState = "pending"
	// LinkStateQRReady means the QR image is available to display.
	LinkStateQRReady LinkState = "qr_ready"
	// LinkStateScanned means the Zalo app read the image; the account holder
	// still has to approve on their phone.
	LinkStateScanned LinkState = "scanned"
	// LinkStateConfirmed means the approval landed and the session is being
	// finalized.
	LinkStateConfirmed LinkState = "confirmed"
	// LinkStateLinked means credentials were persisted and the session cached.
	LinkStateLinked LinkState = "linked"
	// LinkStateExpired means the attempt ran out of time or was cancelled; the
	// teacher can simply start another one.
	LinkStateExpired LinkState = "expired"
	// LinkStateError means the attempt failed for a reason retrying may not fix.
	LinkStateError LinkState = "error"
)

// Defaults for LinkOptions.
const (
	// defaultAttemptTTL bounds one attempt. A Zalo QR code is valid for about
	// 100 seconds; the extra margin lets the login finish rather than being cut
	// off mid-handshake.
	defaultAttemptTTL = 105 * time.Second
	// defaultRetention keeps a finished record readable after it stops
	// changing, so a poll that arrives late still learns the outcome instead of
	// an unexplained "not found".
	defaultRetention = 2 * time.Minute
)

// linkFailureMessage is what a caller is told about a failed attempt. Upstream
// error text never reaches the client: it is written by Zalo, may quote the
// request that produced it, and the teacher can do nothing with it either way.
// The detail is logged instead.
const linkFailureMessage = "could not complete the Zalo login"

// LoginFunc performs the QR login. protocol.LoginQR is the production value;
// tests inject a fake so no test touches the network.
type LoginFunc func(ctx context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error)

// OnLinked persists a successful attempt. It runs on the attempt's goroutine
// before the record flips to LinkStateLinked, so returning an error fails the
// attempt rather than reporting a link that was never stored. It is called only
// while the attempt is still the teacher's current one, and Cancel blocks until
// it returns — so an unlink cannot interleave with the write. The hook proves
// the credentials with a login before storing them, so anything waiting on
// Cancel may block for up to persistTimeout, not just a database write.
type OnLinked func(ctx context.Context, teacherID uuid.UUID, consentVersion string, sess *protocol.Session, cred *protocol.Credentials) error

// LinkOptions tunes attempt lifetimes. The zero value is the production
// configuration; tests shorten both to keep runtimes down.
type LinkOptions struct {
	// AttemptTTL is how long one attempt may run before it expires.
	AttemptTTL time.Duration
	// Retention is how long a finished record stays readable afterwards.
	Retention time.Duration
}

// LinkSnapshot is the non-secret view of an attempt. It carries no credential
// material — the QR image is a login challenge, not a secret of the teacher's.
type LinkSnapshot struct {
	LinkID      uuid.UUID
	State       LinkState
	QRPNG       []byte
	DisplayName string
	Failure     string
}

type linkRecord struct {
	id             uuid.UUID
	teacherID      uuid.UUID
	consentVersion string

	state       LinkState
	qrPNG       []byte
	displayName string
	failure     string

	cancel    context.CancelFunc
	expiresAt time.Time
	done      chan struct{}
}

func (r *linkRecord) snapshot() LinkSnapshot {
	snap := LinkSnapshot{
		LinkID:      r.id,
		State:       r.state,
		DisplayName: r.displayName,
		Failure:     r.failure,
	}
	if len(r.qrPNG) > 0 {
		snap.QRPNG = append([]byte(nil), r.qrPNG...)
	}
	return snap
}

// LinkManager runs QR login attempts in the background and exposes their state
// for polling. One attempt per teacher is live at a time: starting a new one
// cancels the previous, which keeps the number of open long-polls against Zalo
// bounded by the number of teachers actually linking.
//
// Records live in memory only. A restart drops in-flight attempts, which is
// correct — the goroutine driving them is gone too, and the teacher just scans
// again.
type LinkManager struct {
	mu     sync.Mutex
	active map[uuid.UUID]*linkRecord
	// live holds every attempt whose goroutine is still running, including ones
	// active no longer names. Superseding an attempt does not stop it, and only
	// a goroutine that has actually returned is known not to write.
	live map[uuid.UUID]map[*linkRecord]struct{}

	login     LoginFunc
	onLinked  OnLinked
	log       *slog.Logger
	ttl       time.Duration
	retention time.Duration

	// baseCtx is the manager's lifetime, not any request's: every attempt
	// derives from it so Close can end them all at shutdown.
	baseCtx    context.Context
	baseCancel context.CancelFunc
}

// NewLinkManager builds a manager. A nil logger falls back to slog.Default.
func NewLinkManager(login LoginFunc, onLinked OnLinked, log *slog.Logger, opts LinkOptions) *LinkManager {
	if log == nil {
		log = slog.Default()
	}
	if opts.AttemptTTL <= 0 {
		opts.AttemptTTL = defaultAttemptTTL
	}
	if opts.Retention <= 0 {
		opts.Retention = defaultRetention
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &LinkManager{
		active:     make(map[uuid.UUID]*linkRecord),
		live:       make(map[uuid.UUID]map[*linkRecord]struct{}),
		login:      login,
		onLinked:   onLinked,
		log:        log,
		ttl:        opts.AttemptTTL,
		retention:  opts.Retention,
		baseCtx:    ctx,
		baseCancel: cancel,
	}
}

// Begin starts an attempt for the teacher and returns its link id immediately;
// the login itself runs in the background. Any attempt the teacher already had
// running is cancelled and its link id stops resolving.
func (m *LinkManager) Begin(teacherID uuid.UUID, consentVersion string) uuid.UUID {
	ctx, cancel := context.WithTimeout(m.baseCtx, m.ttl)
	rec := &linkRecord{
		id:             uuid.New(),
		teacherID:      teacherID,
		consentVersion: consentVersion,
		state:          LinkStatePending,
		cancel:         cancel,
		expiresAt:      time.Now().Add(m.ttl),
		done:           make(chan struct{}),
	}

	m.mu.Lock()
	m.sweepLocked(time.Now())
	if prev, ok := m.active[teacherID]; ok {
		prev.cancel()
	}
	m.active[teacherID] = rec
	if m.live[teacherID] == nil {
		m.live[teacherID] = make(map[*linkRecord]struct{})
	}
	m.live[teacherID][rec] = struct{}{}
	m.mu.Unlock()

	go m.run(ctx, rec)
	return rec.id
}

// Status returns the attempt's current state. The link id must be the one
// issued to this teacher: another teacher presenting it learns nothing.
func (m *LinkManager) Status(teacherID, linkID uuid.UUID) (LinkSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(time.Now())

	rec, ok := m.active[teacherID]
	if !ok || rec.id != linkID {
		return LinkSnapshot{}, ErrLinkNotFound
	}
	return rec.snapshot(), nil
}

// Cancel ends every attempt the teacher has running — the current one and any
// the teacher superseded by reopening the QR modal — and does not return until
// all of them have stopped. Unlinking calls it so a QR code left open on another
// tab cannot re-link the account behind the teacher's back.
//
// Both halves of that are load-bearing. A scan that has already succeeded is
// persisted on a context that survives cancellation — deliberately, so a slow
// database cannot lose a link the teacher completed — so returning while such a
// goroutine is still running would let its write land after the caller removed
// the account. Waiting only on the current attempt is not enough either: a
// superseded one is no longer named anywhere but is still running, and the
// stored row it would resurrect is a live credential nothing in the UI reports.
// The wait is bounded by each attempt's own deadline and persistTimeout.
func (m *LinkManager) Cancel(teacherID uuid.UUID) {
	m.mu.Lock()
	delete(m.active, teacherID)
	pending := make([]*linkRecord, 0, len(m.live[teacherID]))
	for rec := range m.live[teacherID] {
		pending = append(pending, rec)
	}
	m.mu.Unlock()

	for _, rec := range pending {
		rec.cancel()
		<-rec.done
	}
}

// Close cancels every attempt still running, whether or not the manager still
// names it, and waits for each one's goroutine to return. It is safe to call
// more than once.
func (m *LinkManager) Close() {
	m.baseCancel()

	m.mu.Lock()
	pending := make([]*linkRecord, 0, len(m.live))
	for _, recs := range m.live {
		for rec := range recs {
			pending = append(pending, rec)
		}
	}
	m.active = make(map[uuid.UUID]*linkRecord)
	m.mu.Unlock()

	for _, rec := range pending {
		rec.cancel()
		<-rec.done
	}
}

func (m *LinkManager) run(ctx context.Context, rec *linkRecord) {
	// Announcing the goroutine's exit is the last thing it does, so anyone who
	// saw it in live and waited for done knows it can no longer write.
	defer m.retire(rec)
	defer rec.cancel()

	sess := protocol.NewSession()
	cb := protocol.QRCallbacks{
		OnQR: func(png []byte) {
			m.update(rec, func(r *linkRecord) {
				r.state = LinkStateQRReady
				r.qrPNG = png
			})
		},
		OnProgress: func(state protocol.QRState) {
			m.update(rec, func(r *linkRecord) {
				switch state {
				case protocol.QRStateScanned:
					r.state = LinkStateScanned
				case protocol.QRStateConfirmed:
					r.state = LinkStateConfirmed
				}
			})
		},
	}

	cred, err := m.login(ctx, sess, cb)
	if err != nil {
		m.fail(rec, ctx.Err() != nil, "zalo qr login failed", err)
		return
	}

	// A scan that arrives for an attempt the teacher has moved on from is not
	// stored. The teacher may have unlinked in the meantime, and the write would
	// bring the account back with working credentials while every screen still
	// says it is gone — a superseded record reports no state of its own.
	if !m.isCurrent(rec) {
		return
	}

	// The attempt's deadline is about how long a teacher may take to scan; it
	// must not abort the write that makes their scan count.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	if err := m.onLinked(persistCtx, rec.teacherID, rec.consentVersion, sess, cred); err != nil {
		m.fail(rec, false, "persisting the zalo link failed", err)
		return
	}

	m.update(rec, func(r *linkRecord) {
		r.state = LinkStateLinked
		r.displayName = sess.DisplayName
		// The challenge is spent; there is no reason to keep serving it.
		r.qrPNG = nil
	})
}

// persistTimeout bounds the work that follows a successful scan: the login
// that proves the credentials, then the database write that stores them.
const persistTimeout = 10 * time.Second

func (m *LinkManager) fail(rec *linkRecord, timedOut bool, msg string, err error) {
	if timedOut {
		m.update(rec, func(r *linkRecord) {
			r.state = LinkStateExpired
			r.qrPNG = nil
		})
		return
	}
	m.log.Warn(msg, "teacher_id", rec.teacherID, "error", err)
	m.update(rec, func(r *linkRecord) {
		r.state = LinkStateError
		r.failure = linkFailureMessage
		r.qrPNG = nil
	})
}

// isCurrent reports whether the record is still the teacher's attempt. Cancel
// removes it, and so does starting a replacement.
func (m *LinkManager) isCurrent(rec *linkRecord) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[rec.teacherID] == rec
}

// retire records that the attempt's goroutine has finished and can no longer
// touch anything. Whoever is waiting on done learns it from the closed channel.
func (m *LinkManager) retire(rec *linkRecord) {
	m.mu.Lock()
	if recs := m.live[rec.teacherID]; recs != nil {
		delete(recs, rec)
		if len(recs) == 0 {
			delete(m.live, rec.teacherID)
		}
	}
	m.mu.Unlock()
	close(rec.done)
}

// update applies a change to a record, but only while that record is still the
// teacher's current attempt. A superseded or cancelled attempt whose goroutine
// has not noticed yet therefore cannot write over its replacement.
func (m *LinkManager) update(rec *linkRecord, mutate func(*linkRecord)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[rec.teacherID] != rec {
		return
	}
	mutate(rec)
}

// sweepLocked drops records that stopped being useful. Doing it on every read
// and every start keeps the map bounded without a third background goroutine.
func (m *LinkManager) sweepLocked(now time.Time) {
	for teacherID, rec := range m.active {
		if now.After(rec.expiresAt.Add(m.retention)) {
			rec.cancel()
			delete(m.active, teacherID)
		}
	}
}
