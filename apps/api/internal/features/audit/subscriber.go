package audit

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/invitations"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
)

// BatchInserter is the slice of the repository the subscriber needs.
type BatchInserter interface {
	InsertBatch(ctx context.Context, rows []Log) error
}

// flushTimeout bounds every batch insert, including the final one on Close.
// Without it a saturated database would hang graceful shutdown past the
// orchestrator's grace period; timing out drops the batch, which is the
// same at-most-once outcome as any other failed flush.
const flushTimeout = 5 * time.Second

// Subscriber turns bus events into audit rows, batching writes by size or
// interval. It holds no reference to gin or HTTP — only event structs come
// in. Delivery is at-most-once end to end: a failed flush drops its batch
// with an error log, never retries or blocks.
type Subscriber struct {
	repo      BatchInserter
	log       *slog.Logger
	batchSize int

	mu     sync.Mutex
	buf    []Log
	closed bool

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// maxBatchSize caps a flush at what one postgres statement can bind:
// audit_logs inserts carry 13 parameters per row against the 65535 limit.
const maxBatchSize = 4000

// Client-controlled strings are capped before storage: real user agents fit
// in a fraction of this, and anything longer is an attacker padding rows.
const (
	maxUserAgentLen = 512
	maxPathLen      = 1024
)

// clip truncates s to at most limit bytes, backing off to the previous rune
// boundary so the result stays valid UTF-8 — postgres rejects the whole
// insert batch over one invalid text value.
func clip(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// NewSubscriber starts the interval-flush goroutine. Call Close to stop it
// and flush what remains — the bus only drains its own queues, so rows
// sitting in this buffer at shutdown are the subscriber's responsibility.
// Indefensible tunables are clamped (batch into [1, maxBatchSize], interval
// to a positive value) — config validation rejects them at startup, but a
// zero interval would panic time.NewTicker here.
func NewSubscriber(repo BatchInserter, log *slog.Logger, batchSize int, flushInterval time.Duration) *Subscriber {
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > maxBatchSize {
		batchSize = maxBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	s := &Subscriber{
		repo:      repo,
		log:       log,
		batchSize: batchSize,
		buf:       make([]Log, 0, batchSize),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go s.flushLoop(flushInterval)
	return s
}

func (s *Subscriber) flushLoop(interval time.Duration) {
	defer close(s.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.stop:
			return
		}
	}
}

// Handle implements events.Handler: map the event to a row, buffer it, and
// flush when the batch fills. Unknown events are ignored — the bus is shared
// infrastructure and other subscribers' events are none of our business.
func (s *Subscriber) Handle(_ context.Context, e events.Event) {
	row, ok := s.toRow(e)
	if !ok {
		return
	}
	// Both come verbatim from client-controlled headers/URLs; unauthenticated
	// publishers (failed logins, password resets) mean an attacker chooses
	// their size, so cap them before they occupy the buffer and the table.
	row.UserAgent = clip(row.UserAgent, maxUserAgentLen)
	row.Path = clip(row.Path, maxPathLen)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.log.Warn("audit: event after Close dropped", "action", row.Action)
		return
	}
	s.buf = append(s.buf, row)
	full := len(s.buf) >= s.batchSize
	s.mu.Unlock()
	if full {
		s.flush()
	}
}

// Close stops the interval goroutine and flushes the remaining buffer. Safe
// to call more than once. Anything delivered after Close is dropped with a
// warning, so shutdown must close the bus first: bus.Close(ctx), then
// Subscriber.Close(), then the database pool — in that order the bus has
// drained its queues into this buffer before the final flush runs.
func (s *Subscriber) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.flush()
}

// flush swaps the buffer out under the lock and writes outside it, so a slow
// insert never blocks Handle.
func (s *Subscriber) flush() {
	s.mu.Lock()
	batch := s.buf
	s.buf = make([]Log, 0, s.batchSize)
	s.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	if err := s.repo.InsertBatch(ctx, batch); err != nil {
		s.log.Error("audit: batch insert failed, rows dropped",
			"rows", len(batch), "error", err)
	}
}

func (s *Subscriber) toRow(e events.Event) (Log, bool) {
	switch ev := e.(type) {
	case middleware.RequestCompleted:
		return s.requestRow(ev), true
	case auth.LoginSucceeded:
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			ActorUserID: nilIfZero(ev.UserID),
			Action:      "auth.login",
			IP:          ev.IP,
			UserAgent:   ev.UserAgent,
		}, true
	case auth.LoginFailed:
		return Log{
			ID:         id.New(),
			OccurredAt: ev.OccurredAt,
			Action:     "auth.login_fail",
			IP:         ev.IP,
			UserAgent:  ev.UserAgent,
			Metadata:   Metadata{"phone_masked": ev.PhoneMasked},
		}, true
	case auth.LoggedOut:
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			ActorUserID: nilIfZero(ev.UserID),
			Action:      "auth.logout",
			IP:          ev.IP,
			UserAgent:   ev.UserAgent,
		}, true
	case centers.RolePermissionsChanged:
		// Service events carry the before/after sets the request middleware
		// cannot see; the middleware row for the same request stays the HTTP
		// evidence. Same action name, distinguishable by the Method field.
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			CenterID:    nilIfZero(ev.CenterID),
			ActorUserID: nilIfZero(ev.ActorID),
			Action:      "center.role.permissions_update",
			EntityType:  "center_role",
			EntityID:    ev.RoleID.String(),
			Metadata: Metadata{
				"role_key": ev.RoleKey,
				"before":   strings.Join(ev.Before, ","),
				"after":    strings.Join(ev.After, ","),
			},
		}, true
	case centers.MemberRoleChanged:
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			CenterID:    nilIfZero(ev.CenterID),
			ActorUserID: nilIfZero(ev.ActorID),
			Action:      "center.member.role_update",
			EntityType:  "teacher",
			EntityID:    ev.TeacherID.String(),
			Metadata: Metadata{
				"before": ev.Before,
				"after":  ev.After,
			},
		}, true
	case centers.MemberOverridesChanged:
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			CenterID:    nilIfZero(ev.CenterID),
			ActorUserID: nilIfZero(ev.ActorID),
			Action:      "center.member.overrides_update",
			EntityType:  "teacher",
			EntityID:    ev.TeacherID.String(),
			Metadata: Metadata{
				"before_grants": strings.Join(ev.BeforeGrants, ","),
				"before_denies": strings.Join(ev.BeforeDenies, ","),
				"after_grants":  strings.Join(ev.AfterGrants, ","),
				"after_denies":  strings.Join(ev.AfterDenies, ","),
			},
		}, true
	case enrollments.StudentEnrolled:
		// The service event, not the middleware row, carries which class and
		// student were linked — the request middleware stores no body.
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			CenterID:    nilIfZero(ev.CenterID),
			ActorUserID: nilIfZero(ev.ActorID),
			Action:      "enrollment.create",
			EntityType:  "enrollment",
			EntityID:    ev.EnrollmentID.String(),
			Metadata: Metadata{
				"class_id":   ev.ClassID.String(),
				"student_id": ev.StudentID.String(),
			},
		}, true
	case invitations.MemberJoined:
		// The service event, not the middleware, carries this action: the
		// accept request is anonymous, so only the service knows the center
		// and the account that joined.
		return Log{
			ID:          id.New(),
			OccurredAt:  ev.OccurredAt,
			CenterID:    nilIfZero(ev.CenterID),
			ActorUserID: nilIfZero(ev.UserID),
			Action:      "invitation.accept",
			EntityType:  "invitation",
			EntityID:    ev.InvitationID.String(),
			IP:          ev.IP,
			UserAgent:   ev.UserAgent,
		}, true
	default:
		return Log{}, false
	}
}

func (s *Subscriber) requestRow(ev middleware.RequestCompleted) Log {
	row := Log{
		ID:         id.New(),
		OccurredAt: ev.OccurredAt,
		ActorRole:  ev.ActorRole,
		Method:     ev.Method,
		Path:       ev.Path,
		StatusCode: ev.StatusCode,
		RequestID:  ev.RequestID,
		IP:         ev.IP,
		UserAgent:  ev.UserAgent,
	}
	row.CenterID = nilIfZero(ev.CenterID)
	row.ActorUserID = nilIfZero(ev.ActorID)
	if spec, ok := LookupAction(ev.Method, ev.Route); ok {
		row.Action = spec.Action
		row.EntityType = spec.EntityType
		if spec.IDParam != "" {
			row.EntityID = ev.Params[spec.IDParam]
		}
	} else {
		// Fallback keeps unmapped (new) routes in the trail, just less
		// readable; the route template bounds cardinality better than the
		// raw path.
		route := ev.Route
		if route == "" {
			route = ev.Path
		}
		row.Action = ev.Method + " " + route
	}
	return row
}

// nilIfZero maps uuid.Nil to a SQL NULL: a zero UUID in the table would look
// like a real actor while joining to nothing.
func nilIfZero(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}
