package events

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testEvent struct{ id int }

func (testEvent) EventName() string { return "test.event" }

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// TestPublishDelivers proves a subscriber receives the exact event published.
func TestPublishDelivers(t *testing.T) {
	bus := New(discardLogger())
	got := make(chan Event, 1)
	bus.Subscribe("s1", 8, func(_ context.Context, e Event) {
		got <- e
	})

	bus.Publish(testEvent{id: 42})

	select {
	case e := <-got:
		if e.(testEvent).id != 42 {
			t.Fatalf("received event id = %d, want 42", e.(testEvent).id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received the event")
	}
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestFanOut proves every subscriber receives its own copy of one publish.
func TestFanOut(t *testing.T) {
	bus := New(discardLogger())
	var wg sync.WaitGroup
	wg.Add(2)
	var c1, c2 atomic.Int32
	bus.Subscribe("s1", 8, func(_ context.Context, _ Event) { c1.Add(1); wg.Done() })
	bus.Subscribe("s2", 8, func(_ context.Context, _ Event) { c2.Add(1); wg.Done() })

	bus.Publish(testEvent{id: 1})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("not all subscribers received the event")
	}
	if c1.Load() != 1 || c2.Load() != 1 {
		t.Fatalf("delivery counts = %d, %d; want 1, 1", c1.Load(), c2.Load())
	}
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestPublishNeverBlocks proves a full subscriber queue drops the event
// instead of blocking the publisher — the at-most-once contract.
func TestPublishNeverBlocks(t *testing.T) {
	bus := New(discardLogger())
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	bus.Subscribe("slow", 1, func(_ context.Context, _ Event) {
		started <- struct{}{}
		<-release
	})

	// First publish occupies the worker; wait until the handler is running so
	// the buffer state is deterministic.
	bus.Publish(testEvent{id: 1})
	<-started
	// Second fills the buf=1 queue.
	bus.Publish(testEvent{id: 2})

	// Third must return immediately (dropped), not block.
	returned := make(chan struct{})
	go func() {
		bus.Publish(testEvent{id: 3})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full subscriber queue")
	}

	close(release)
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestSlowSubscriberDoesNotBlockOthers proves per-subscriber isolation:
// one stuck handler must not delay delivery to a healthy subscriber.
func TestSlowSubscriberDoesNotBlockOthers(t *testing.T) {
	bus := New(discardLogger())
	release := make(chan struct{})
	bus.Subscribe("stuck", 1, func(_ context.Context, _ Event) { <-release })
	fast := make(chan Event, 4)
	bus.Subscribe("fast", 4, func(_ context.Context, e Event) { fast <- e })

	bus.Publish(testEvent{id: 1})

	select {
	case <-fast:
	case <-time.After(2 * time.Second):
		t.Fatal("healthy subscriber was blocked by a stuck one")
	}
	close(release)
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestCloseDrains proves Close only returns after every queued event has been
// handled — required so shutdown never loses buffered audit records.
func TestCloseDrains(t *testing.T) {
	bus := New(discardLogger())
	const n = 50
	var handled atomic.Int32
	bus.Subscribe("s1", n, func(_ context.Context, _ Event) {
		time.Sleep(time.Millisecond)
		handled.Add(1)
	})

	for i := 0; i < n; i++ {
		bus.Publish(testEvent{id: i})
	}
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if handled.Load() != n {
		t.Fatalf("handled %d events after Close, want %d", handled.Load(), n)
	}
}

// TestCloseTimeout proves Close respects ctx cancellation when a handler
// hangs, returning an error instead of waiting forever.
func TestCloseTimeout(t *testing.T) {
	bus := New(discardLogger())
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{}, 1)
	bus.Subscribe("hung", 1, func(_ context.Context, _ Event) {
		started <- struct{}{}
		<-release
	})

	bus.Publish(testEvent{id: 1})
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := bus.Close(ctx); err == nil {
		t.Fatal("Close() = nil, want timeout error with a hung handler")
	}
}

// TestHandlerPanicRecovered proves a panicking handler does not kill the
// worker: the next event is still delivered.
func TestHandlerPanicRecovered(t *testing.T) {
	bus := New(discardLogger())
	got := make(chan Event, 1)
	var first atomic.Bool
	first.Store(true)
	bus.Subscribe("panicky", 8, func(_ context.Context, e Event) {
		if first.Swap(false) {
			panic("boom")
		}
		got <- e
	})

	bus.Publish(testEvent{id: 1})
	bus.Publish(testEvent{id: 2})

	select {
	case e := <-got:
		if e.(testEvent).id != 2 {
			t.Fatalf("received event id = %d, want 2", e.(testEvent).id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker died after handler panic")
	}
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestPublishAfterClose proves publishing to a closed bus is a safe no-op.
func TestPublishAfterClose(t *testing.T) {
	bus := New(discardLogger())
	bus.Subscribe("s1", 1, func(_ context.Context, _ Event) {})
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// Must not panic (send on closed channel would).
	bus.Publish(testEvent{id: 1})
}

// TestConcurrentPublish proves the publish path is race-free under parallel
// producers (run with -race).
func TestConcurrentPublish(t *testing.T) {
	bus := New(discardLogger())
	var handled atomic.Int32
	bus.Subscribe("s1", 1024, func(_ context.Context, _ Event) { handled.Add(1) })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				bus.Publish(testEvent{id: j})
			}
		}()
	}
	wg.Wait()
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if handled.Load() != 800 {
		t.Fatalf("handled %d events, want 800 (buffer large enough for no drops)", handled.Load())
	}
}

// syncBuffer makes a bytes.Buffer safe to share between worker goroutines
// (slog writes) and the test goroutine (assertions).
type syncBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b = append(s.b, p...)
	return len(p), nil
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.b)
}

// panicNameEvent's EventName itself panics — the bus must never let an event
// take down the publisher or the recovery path.
type panicNameEvent struct{}

func (panicNameEvent) EventName() string { panic("bad event") }

// TestDropLogsWarning pins acceptance criterion 1: a dropped event must emit
// a warning carrying both the subscriber name and the event name.
func TestDropLogsWarning(t *testing.T) {
	buf := &syncBuffer{}
	bus := New(slog.New(slog.NewTextHandler(buf, nil)))
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{}, 1)
	bus.Subscribe("slowsub", 1, func(_ context.Context, _ Event) {
		started <- struct{}{}
		<-release
	})

	bus.Publish(testEvent{id: 1})
	<-started
	bus.Publish(testEvent{id: 2}) // fills queue
	bus.Publish(testEvent{id: 3}) // dropped — logged synchronously in Publish

	logged := buf.String()
	for _, want := range []string{"slowsub", "test.event", "WARN"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("drop warning missing %q; log output:\n%s", want, logged)
		}
	}
}

// TestPanicLogsError pins criterion 4's observable half: a recovered handler
// panic must be logged with subscriber name, event name, and panic value.
func TestPanicLogsError(t *testing.T) {
	buf := &syncBuffer{}
	bus := New(slog.New(slog.NewTextHandler(buf, nil)))
	done := make(chan struct{}, 1)
	bus.Subscribe("panicky", 8, func(_ context.Context, _ Event) {
		defer func() { done <- struct{}{} }()
		panic("boom")
	})

	bus.Publish(testEvent{id: 1})
	<-done
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	logged := buf.String()
	for _, want := range []string{"panicky", "test.event", "boom", "ERROR"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("panic log missing %q; log output:\n%s", want, logged)
		}
	}
}

// TestHostileEventsDoNotPanic proves a nil event or an event whose EventName
// panics cannot crash the publisher or the recovery path.
func TestHostileEventsDoNotPanic(t *testing.T) {
	bus := New(discardLogger())
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	bus.Subscribe("s1", 1, func(_ context.Context, _ Event) {
		started <- struct{}{}
		<-release
	})

	bus.Publish(nil) // must be a safe no-op

	bus.Publish(panicNameEvent{}) // occupies worker
	<-started
	bus.Publish(panicNameEvent{}) // fills queue
	bus.Publish(panicNameEvent{}) // dropped — name resolution must not panic

	close(release)
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestSubscribeAfterClose proves a late Subscribe is ignored without breaking
// Close's drain accounting.
func TestSubscribeAfterClose(t *testing.T) {
	bus := New(discardLogger())
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	delivered := make(chan Event, 1)
	bus.Subscribe("late", 1, func(_ context.Context, e Event) { delivered <- e })
	bus.Publish(testEvent{id: 1})
	select {
	case <-delivered:
		t.Fatal("subscriber registered after Close received an event")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestSecondCloseWaitsForDrain proves Close is not a lying no-op on repeat:
// a second Close must still wait for workers instead of returning nil while
// they run — shutdown code may retry Close and trust its nil.
func TestSecondCloseWaitsForDrain(t *testing.T) {
	bus := New(discardLogger())
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var handled atomic.Int32
	bus.Subscribe("slow", 1, func(_ context.Context, _ Event) {
		started <- struct{}{}
		<-release
		handled.Add(1)
	})

	bus.Publish(testEvent{id: 1})
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := bus.Close(ctx); err == nil {
		t.Fatal("first Close() = nil, want timeout error")
	}

	close(release)
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if handled.Load() != 1 {
		t.Fatalf("second Close returned before drain: handled = %d, want 1", handled.Load())
	}
}
