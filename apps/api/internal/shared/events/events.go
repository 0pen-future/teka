// Package events provides a generic in-process event bus: non-blocking
// publish with per-subscriber fan-out queues. Pure infrastructure — it knows
// nothing about business features; event types live next to their publishers.
package events

import "context"

// Event is anything that can travel on the bus. EventName identifies the
// event type for logging and subscriber-side dispatch.
type Event interface {
	EventName() string
}

// Handler processes one event. Delivery on AsyncBus happens on the
// subscriber's worker goroutine with a background context, since the
// originating request context is already dead by then.
type Handler func(ctx context.Context, e Event)

// Bus decouples publishers from subscribers. Publish must never block the
// caller; delivery is at-most-once (a full subscriber queue drops the event).
type Bus interface {
	Publish(e Event)
	// Subscribe registers a handler behind its own queue of capacity buf.
	// name identifies the subscriber in drop/panic logs.
	Subscribe(name string, buf int, h Handler)
	// Close stops accepting events and drains every subscriber queue,
	// returning ctx's error if draining outlives the context. The guarantee
	// ends at the queue boundary: a subscriber that buffers events
	// internally (e.g. a batcher) must flush in its own shutdown, and a
	// handler that never returns keeps its worker alive until process exit.
	Close(ctx context.Context) error
}
