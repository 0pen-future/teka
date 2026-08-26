package events

import (
	"context"
	"log/slog"
	"sync"
)

// AsyncBus fans events out to per-subscriber buffered queues, each drained by
// its own worker goroutine, so one slow subscriber never delays another or
// the publisher.
type AsyncBus struct {
	log *slog.Logger

	mu     sync.RWMutex
	subs   []*subscriber
	closed bool
	// done is created by the first Close and closed once every worker has
	// exited, so repeated Close calls all wait on the same drain.
	done chan struct{}

	wg sync.WaitGroup
}

var _ Bus = (*AsyncBus)(nil)

type subscriber struct {
	name string
	ch   chan Event
	h    Handler
}

// New returns an empty AsyncBus; wire subscribers with Subscribe before
// publishing.
func New(log *slog.Logger) *AsyncBus {
	return &AsyncBus{log: log}
}

// Subscribe registers h behind its own queue of capacity buf and starts its
// worker goroutine. Subscribing after Close is ignored with a warning.
func (b *AsyncBus) Subscribe(name string, buf int, h Handler) {
	s := &subscriber{name: name, ch: make(chan Event, buf), h: h}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.log.Warn("events: subscribe on closed bus ignored", "subscriber", name)
		return
	}
	b.subs = append(b.subs, s)
	b.wg.Add(1)
	b.mu.Unlock()

	go b.work(s)
}

func (b *AsyncBus) work(s *subscriber) {
	defer b.wg.Done()
	for e := range s.ch {
		b.handle(s, e)
	}
}

// handle isolates one delivery so a panicking handler only loses its own
// event, not the worker. The event name is resolved before the handler runs:
// nothing inside the recover may call back into the event, or a hostile
// EventName would repanic and kill the process.
func (b *AsyncBus) handle(s *subscriber, e Event) {
	name := eventName(e)
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("events: handler panic recovered",
				"subscriber", s.name, "event", name, "panic", r)
		}
	}()
	s.h(context.Background(), e)
}

// Publish enqueues e for every subscriber without ever blocking: a full
// queue drops the event with a warning (at-most-once delivery). A nil event
// is dropped up front.
func (b *AsyncBus) Publish(e Event) {
	if e == nil {
		b.log.Warn("events: nil event dropped")
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		b.log.Warn("events: publish on closed bus dropped", "event", eventName(e))
		return
	}
	for _, s := range b.subs {
		select {
		case s.ch <- e:
		default:
			b.log.Warn("events: subscriber queue full, event dropped",
				"subscriber", s.name, "event", eventName(e))
		}
	}
}

// Close stops accepting events, then waits for every queue to drain. On ctx
// expiry it returns ctx.Err(); workers exit once their in-flight handler
// returns, so a handler that never returns leaks its worker until process
// exit. Repeated calls wait on the same drain rather than reporting done
// early.
func (b *AsyncBus) Close(ctx context.Context) error {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		for _, s := range b.subs {
			close(s.ch)
		}
		b.done = make(chan struct{})
		go func(done chan struct{}) {
			b.wg.Wait()
			close(done)
		}(b.done)
	}
	done := b.done
	b.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// eventName resolves e's log name without trusting the event: a nil
// interface, a typed-nil receiver, or an EventName that panics must never
// take down the caller — this runs inside panic recovery.
func eventName(e Event) (name string) {
	name = "<invalid>"
	if e == nil {
		return "<nil>"
	}
	defer func() { _ = recover() }()
	name = e.EventName()
	return name
}
