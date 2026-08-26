package events

import (
	"context"
	"testing"
)

// TestSyncBusDeliversInSubscribeOrder proves SyncBus delivers synchronously
// inside Publish, in subscription order — what tests rely on for determinism.
func TestSyncBusDeliversInSubscribeOrder(t *testing.T) {
	bus := NewSync()
	var order []string
	bus.Subscribe("first", 0, func(_ context.Context, e Event) {
		if e.EventName() != "test.event" {
			t.Errorf("event name = %q, want %q", e.EventName(), "test.event")
		}
		order = append(order, "first")
	})
	bus.Subscribe("second", 0, func(_ context.Context, _ Event) {
		order = append(order, "second")
	})

	bus.Publish(testEvent{id: 1})

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("delivery order = %v, want [first second]", order)
	}
	if err := bus.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
