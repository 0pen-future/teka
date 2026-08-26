package audit

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/invitations"
	"teka/apps/api/internal/middleware"
)

// fakeRepo records every batch it receives; failNext makes the next
// InsertBatch fail once so error handling can be exercised.
type fakeRepo struct {
	mu          sync.Mutex
	batches     [][]Log
	failNext    bool
	sawDeadline bool
}

func (f *fakeRepo) InsertBatch(ctx context.Context, rows []Log) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := ctx.Deadline(); ok {
		f.sawDeadline = true
	}
	if f.failNext {
		f.failNext = false
		return errors.New("db down")
	}
	cp := make([]Log, len(rows))
	copy(cp, rows)
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakeRepo) batchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func (f *fakeRepo) batch(i int) []Log {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.batches[i]
}

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func requestEvent(route, method string) middleware.RequestCompleted {
	return middleware.RequestCompleted{
		OccurredAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		CenterID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ActorID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		ActorRole:  "owner",
		Method:     method,
		Route:      route,
		Path:       route,
		Params:     map[string]string{"id": "abc-123"},
		StatusCode: 200,
		RequestID:  "req-1",
		IP:         "10.0.0.1",
		UserAgent:  "go-test",
	}
}

// TestFlushOnBatchSize proves the subscriber flushes exactly when the batch
// fills, and holds the remainder.
func TestFlushOnBatchSize(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 3, time.Hour)
	defer s.Close()

	for i := 0; i < 4; i++ {
		s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	}

	if got := repo.batchCount(); got != 1 {
		t.Fatalf("batches after 4 events with size 3 = %d, want 1", got)
	}
	if got := len(repo.batch(0)); got != 3 {
		t.Fatalf("first batch size = %d, want 3", got)
	}
}

// TestFlushOnInterval proves events below the batch size still reach the
// repository once the flush interval elapses.
func TestFlushOnInterval(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 100, 20*time.Millisecond)
	defer s.Close()

	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))

	deadline := time.After(2 * time.Second)
	for repo.batchCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("interval flush never happened")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if got := len(repo.batch(0)); got != 2 {
		t.Fatalf("interval batch size = %d, want 2", got)
	}
}

// TestCloseFlushesRemainder proves Close drains the in-memory buffer — the
// bus only guarantees its queues, so shutdown durability lives here.
func TestCloseFlushesRemainder(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 100, time.Hour)

	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	s.Close()

	if got := repo.batchCount(); got != 1 {
		t.Fatalf("batches after Close = %d, want 1", got)
	}
	if got := len(repo.batch(0)); got != 2 {
		t.Fatalf("final batch size = %d, want 2", got)
	}
}

// TestRequestCompletedMapping proves every audit column is filled from the
// request event, including the action map and entity extraction.
func TestRequestCompletedMapping(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 1, time.Hour)
	defer s.Close()

	e := middleware.RequestCompleted{
		OccurredAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		CenterID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ActorID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		ActorRole:  "member",
		Method:     "PUT",
		Route:      "/api/v1/classes/:id",
		Path:       "/api/v1/classes/abc-123",
		Params:     map[string]string{"id": "abc-123"},
		StatusCode: 422,
		RequestID:  "req-9",
		IP:         "10.0.0.9",
		UserAgent:  "go-test",
	}
	s.Handle(context.Background(), e)

	row := repo.batch(0)[0]
	if row.ID == uuid.Nil {
		t.Error("row ID not generated")
	}
	if !row.OccurredAt.Equal(e.OccurredAt) {
		t.Errorf("OccurredAt = %v, want %v", row.OccurredAt, e.OccurredAt)
	}
	if row.CenterID == nil || *row.CenterID != e.CenterID {
		t.Errorf("CenterID = %v, want %v", row.CenterID, e.CenterID)
	}
	if row.ActorUserID == nil || *row.ActorUserID != e.ActorID {
		t.Errorf("ActorUserID = %v, want %v", row.ActorUserID, e.ActorID)
	}
	if row.ActorRole != "member" {
		t.Errorf("ActorRole = %q, want %q", row.ActorRole, "member")
	}
	if row.Action != "class.update" {
		t.Errorf("Action = %q, want %q", row.Action, "class.update")
	}
	if row.EntityType != "class" || row.EntityID != "abc-123" {
		t.Errorf("entity = %q/%q, want class/abc-123", row.EntityType, row.EntityID)
	}
	if row.Method != "PUT" || row.Path != "/api/v1/classes/abc-123" {
		t.Errorf("method/path = %q %q", row.Method, row.Path)
	}
	if row.StatusCode != 422 {
		t.Errorf("StatusCode = %d, want 422", row.StatusCode)
	}
	if row.RequestID != "req-9" || row.IP != "10.0.0.9" || row.UserAgent != "go-test" {
		t.Errorf("request context columns wrong: %+v", row)
	}
}

// TestUnmappedRouteFallsBack proves a route missing from the action map is
// still logged, under "METHOD route" — new routes must never silently escape
// the audit trail.
func TestUnmappedRouteFallsBack(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 1, time.Hour)
	defer s.Close()

	e := requestEvent("/api/v1/brand-new/:id", "POST")
	s.Handle(context.Background(), e)

	row := repo.batch(0)[0]
	if row.Action != "POST /api/v1/brand-new/:id" {
		t.Errorf("fallback action = %q, want %q", row.Action, "POST /api/v1/brand-new/:id")
	}
	if row.EntityType != "" || row.EntityID != "" {
		t.Errorf("unmapped route must not guess entity: %q/%q", row.EntityType, row.EntityID)
	}
}

// TestAuthEventMapping proves the three auth events become rows with the
// right action, actor, and masked metadata — and no center scope.
func TestAuthEventMapping(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 3, time.Hour)
	defer s.Close()

	userID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	at := time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC)
	s.Handle(context.Background(), auth.LoginSucceeded{OccurredAt: at, UserID: userID, IP: "1.2.3.4", UserAgent: "ua"})
	s.Handle(context.Background(), auth.LoginFailed{OccurredAt: at, PhoneMasked: "090***123", IP: "1.2.3.4", UserAgent: "ua"})
	s.Handle(context.Background(), auth.LoggedOut{OccurredAt: at, UserID: userID, IP: "1.2.3.4", UserAgent: "ua"})

	rows := repo.batch(0)
	login, fail, logout := rows[0], rows[1], rows[2]

	if login.Action != "auth.login" || login.ActorUserID == nil || *login.ActorUserID != userID {
		t.Errorf("login row wrong: %+v", login)
	}
	if login.CenterID != nil {
		t.Errorf("auth event must have no center scope, got %v", login.CenterID)
	}
	if fail.Action != "auth.login_fail" || fail.ActorUserID != nil {
		t.Errorf("login-fail row wrong: %+v", fail)
	}
	if fail.Metadata["phone_masked"] != "090***123" {
		t.Errorf("login-fail metadata = %v, want phone_masked=090***123", fail.Metadata)
	}
	if logout.Action != "auth.logout" || logout.ActorUserID == nil || *logout.ActorUserID != userID {
		t.Errorf("logout row wrong: %+v", logout)
	}
	for _, r := range rows {
		if r.IP != "1.2.3.4" || r.UserAgent != "ua" || !r.OccurredAt.Equal(at) {
			t.Errorf("auth row missing request context: %+v", r)
		}
	}
}

// TestMemberJoinedMapping proves a redeemed invitation becomes an
// invitation.accept row scoped to the center, actor = the joining account,
// entity = the invitation — the row the owner actually sees, unlike the
// middleware which skips the anonymous accept request entirely.
func TestMemberJoinedMapping(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 1, time.Hour)
	defer s.Close()

	userID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	centerID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	invID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	s.Handle(context.Background(), invitations.MemberJoined{
		OccurredAt: at, CenterID: centerID, UserID: userID, InvitationID: invID,
		IP: "1.2.3.4", UserAgent: "ua",
	})

	row := repo.batch(0)[0]
	if row.Action != "invitation.accept" || row.EntityType != "invitation" || row.EntityID != invID.String() {
		t.Errorf("row wrong: %+v", row)
	}
	if row.CenterID == nil || *row.CenterID != centerID {
		t.Errorf("center = %v, want %v", row.CenterID, centerID)
	}
	if row.ActorUserID == nil || *row.ActorUserID != userID {
		t.Errorf("actor = %v, want %v", row.ActorUserID, userID)
	}
	if row.IP != "1.2.3.4" || row.UserAgent != "ua" || !row.OccurredAt.Equal(at) {
		t.Errorf("row missing request context: %+v", row)
	}
}

// TestNewSubscriberClampsBadTunables proves indefensible tunables are clamped
// rather than trusted: config validation already rejects them at startup, but
// the subscriber must not panic (time.NewTicker(0)) or thrash if constructed
// directly with garbage.
func TestNewSubscriberClampsBadTunables(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 0, 0)
	s.Handle(context.Background(), auth.LoggedOut{OccurredAt: time.Now(), UserID: uuid.New()})
	s.Close()
	total := 0
	for i := 0; i < repo.batchCount(); i++ {
		total += len(repo.batch(i))
	}
	if total != 1 {
		t.Fatalf("flushed rows = %d, want 1", total)
	}
}

// TestUnknownEventIgnored proves foreign events on the bus are skipped, not
// crashed on or logged as rows.
func TestUnknownEventIgnored(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 1, time.Hour)
	s.Handle(context.Background(), strangeEvent{})
	s.Close()
	if got := repo.batchCount(); got != 0 {
		t.Fatalf("unknown event produced %d batches, want 0", got)
	}
}

type strangeEvent struct{}

func (strangeEvent) EventName() string { return "something.else" }

// TestCloseTwiceAndHandleAfterClose proves the documented Close contract:
// idempotent, and anything delivered after Close is dropped rather than
// buffered where nothing will ever flush it.
func TestCloseTwiceAndHandleAfterClose(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 100, time.Hour)

	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	s.Close()
	s.Close()

	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	s.Close()

	if got := repo.batchCount(); got != 1 {
		t.Fatalf("batches = %d, want 1 (post-Close event must be dropped, not buffered)", got)
	}
	if got := len(repo.batch(0)); got != 1 {
		t.Fatalf("final batch size = %d, want 1", got)
	}
}

// TestConcurrentHandleAndClose runs handlers against Close under the race
// detector: rows may be flushed or dropped, but nothing may race or wedge.
func TestConcurrentHandleAndClose(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 4, time.Millisecond)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
			}
		}()
	}
	s.Close()
	wg.Wait()
	s.Close()

	total := 0
	repo.mu.Lock()
	for _, b := range repo.batches {
		total += len(b)
	}
	repo.mu.Unlock()
	if total > 200 {
		t.Fatalf("flushed %d rows from 200 handled — duplicates leaked", total)
	}
}

// TestFlushContextHasDeadline proves every insert carries a deadline — an
// unresponsive database must time out and drop the batch, never hang a
// graceful shutdown.
func TestFlushContextHasDeadline(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 1, time.Hour)
	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST"))
	s.Close()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if !repo.sawDeadline {
		t.Fatal("InsertBatch received a context without a deadline")
	}
}

// TestInsertFailureDropsBatchAndContinues proves a failed flush drops that
// batch (at-most-once) without wedging the subscriber.
func TestInsertFailureDropsBatchAndContinues(t *testing.T) {
	repo := &fakeRepo{failNext: true}
	s := NewSubscriber(repo, testLogger(), 1, time.Hour)
	defer s.Close()

	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST")) // fails, dropped
	s.Handle(context.Background(), requestEvent("/api/v1/classes", "POST")) // must still flush

	if got := repo.batchCount(); got != 1 {
		t.Fatalf("batches after one failure = %d, want 1", got)
	}
	if got := len(repo.batch(0)); got != 1 {
		t.Fatalf("surviving batch size = %d, want 1 (failed batch must not be retried)", got)
	}
}

// TestHandleClipsClientControlledStrings proves oversized client-controlled
// header/URL values are capped before storage, cutting on a rune boundary so
// the row stays valid UTF-8 (postgres rejects a whole batch over one bad
// text value).
func TestHandleClipsClientControlledStrings(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), 100, time.Hour)
	longUA := strings.Repeat("é", maxUserAgentLen) // 2 bytes per rune: crosses the cap mid-rune
	s.Handle(context.Background(), middleware.RequestCompleted{
		OccurredAt: time.Now(),
		ActorID:    uuid.New(),
		Method:     "POST",
		Route:      "/api/v1/classes",
		Path:       "/api/v1/classes/" + strings.Repeat("x", 5000),
		StatusCode: 201,
		UserAgent:  longUA,
	})
	s.Close()

	if repo.batchCount() != 1 {
		t.Fatalf("batches = %d, want 1", repo.batchCount())
	}
	row := repo.batch(0)[0]
	if len(row.UserAgent) > maxUserAgentLen || !utf8.ValidString(row.UserAgent) {
		t.Errorf("user agent len = %d valid = %v", len(row.UserAgent), utf8.ValidString(row.UserAgent))
	}
	if len(row.Path) != maxPathLen {
		t.Errorf("path len = %d, want %d", len(row.Path), maxPathLen)
	}
}

// TestNewSubscriberClampsOversizedBatch proves the upper clamp: a batch size
// beyond what one insert statement can bind flushes at maxBatchSize instead.
func TestNewSubscriberClampsOversizedBatch(t *testing.T) {
	repo := &fakeRepo{}
	s := NewSubscriber(repo, testLogger(), maxBatchSize+1000, time.Hour)
	for i := 0; i < maxBatchSize; i++ {
		s.Handle(context.Background(), auth.LoggedOut{OccurredAt: time.Now(), UserID: uuid.New()})
	}
	if got := repo.batchCount(); got != 1 {
		t.Fatalf("batches before Close = %d, want 1 (buffer must flush at the clamp)", got)
	}
	if got := len(repo.batch(0)); got != maxBatchSize {
		t.Fatalf("batch size = %d, want %d", got, maxBatchSize)
	}
	s.Close()
}
