package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/events"
	"teka/apps/api/pkg/kanban"
)

// ColumnDeleted is published on the bus after Service.DeleteColumn's
// underlying transaction commits. The generic request-log audit path already
// records this DELETE, but move_to travels as a query parameter it never
// inspects, so this event adds a second, richer row carrying move_to and
// MovedCount — same action name as the request row, distinguished by the
// Method field, mirroring centers.RolePermissionsChanged. See
// audit/subscriber.go's case for it.
type ColumnDeleted struct {
	OccurredAt time.Time
	CenterID   uuid.UUID
	ActorID    uuid.UUID
	ColumnID   uuid.UUID
	MoveTo     *uuid.UUID
	MovedCount int
}

// EventName implements events.Event.
func (ColumnDeleted) EventName() string { return "tasks.column_deleted" }

// deleteColumnCtxKey carries the caller identity a DeleteColumn call needs
// its eventSink to see. kanban.EventSink.Publish only receives (ctx, event)
// — no tenant/actor — because the core is oblivious to what a host calls its
// callers; ctx is the one channel available to thread that identity down to
// the moment DeleteColumn's own sink.Publish call fires, which happens
// inside the same call stack this ctx was passed into. See
// pkg/kanban/service.go's DeleteColumn: it forwards the same ctx unchanged.
type deleteColumnCtxKey struct{}

type deleteColumnCaller struct {
	centerID uuid.UUID
	actorID  uuid.UUID
	// moved receives ColumnDeleted.MovedCount, when non-nil, so
	// Service.DeleteColumn can report it in its HTTP response — the core
	// use-case itself only reports success or failure, never a count.
	moved *int
}

// withDeleteColumnCaller attaches the caller identity a subsequent
// DeleteColumn call's EventSink.Publish needs, and, when moved is non-nil, a
// slot to write back how many tasks the delete moved.
func withDeleteColumnCaller(ctx context.Context, centerID, actorID uuid.UUID, moved *int) context.Context {
	return context.WithValue(ctx, deleteColumnCtxKey{}, deleteColumnCaller{centerID: centerID, actorID: actorID, moved: moved})
}

// eventSink implements kanban.EventSink. It only knows how to translate the
// single v1 core event, kanban.ColumnDeleted; anything else is ignored
// rather than panicking, since a future core event type must not crash an
// adapter written before it existed.
type eventSink struct {
	bus events.Bus
}

// newEventSink builds the kanban.EventSink that republishes onto bus.
func newEventSink(bus events.Bus) *eventSink {
	return &eventSink{bus: bus}
}

var _ kanban.EventSink = (*eventSink)(nil)

// Publish implements kanban.EventSink.
func (s *eventSink) Publish(ctx context.Context, event any) {
	cd, ok := event.(kanban.ColumnDeleted)
	if !ok {
		return
	}
	caller, _ := ctx.Value(deleteColumnCtxKey{}).(deleteColumnCaller)
	if caller.moved != nil {
		*caller.moved = cd.MovedCount
	}
	var moveTo *uuid.UUID
	if cd.MoveTo != nil {
		v := uuid.UUID(*cd.MoveTo)
		moveTo = &v
	}
	s.publish(ColumnDeleted{
		OccurredAt: time.Now(),
		CenterID:   caller.centerID,
		ActorID:    caller.actorID,
		ColumnID:   uuid.UUID(cd.ColumnID),
		MoveTo:     moveTo,
		MovedCount: cd.MovedCount,
	})
}

// publish emits e when a bus is wired; a nil bus (e.g. a Service constructed
// without one in a test) makes every publish a no-op, matching
// centers/service.go's publish helper.
func (s *eventSink) publish(e events.Event) {
	if s.bus != nil {
		s.bus.Publish(e)
	}
}
