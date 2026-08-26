package events

import "context"

// SyncBus delivers events synchronously inside Publish, in subscription
// order. It exists for tests, where async delivery would force sleeps or
// polling; production wiring uses AsyncBus.
//
// Deliberate divergences from AsyncBus, chosen so tests fail loud:
//   - handler panics propagate to the Publish caller instead of being
//     recovered and logged;
//   - Close is a no-op and later publishes still deliver;
//   - not safe for concurrent use — subscribe and publish from one goroutine.
type SyncBus struct {
	handlers []Handler
}

var _ Bus = (*SyncBus)(nil)

// NewSync returns an empty SyncBus for test wiring.
func NewSync() *SyncBus {
	return &SyncBus{}
}

// Subscribe registers h; buf is ignored since nothing is queued.
func (b *SyncBus) Subscribe(_ string, _ int, h Handler) {
	b.handlers = append(b.handlers, h)
}

// Publish invokes every handler synchronously, in subscription order.
func (b *SyncBus) Publish(e Event) {
	for _, h := range b.handlers {
		h(context.Background(), e)
	}
}

// Close is a no-op; a test bus has nothing to drain.
func (b *SyncBus) Close(_ context.Context) error { return nil }
