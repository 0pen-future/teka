---
title: "Phase 1: Shared Event Bus"
status: done
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 1: Shared Event Bus

## Overview

Xây `internal/shared/events`: event bus in-process generic — publish
non-blocking, fan-out per-subscriber (mỗi subscriber queue + worker goroutine
riêng), drain khi Close. Kèm `SyncBus` cho test. Infra thuần, không business
logic, không biết audit tồn tại.

## Requirements

- [x] `Publish` không bao giờ block caller; queue đầy → drop + `slog` warning
      (kèm subscriber name + event name)
- [x] Mỗi subscriber có buffered channel + 1 worker goroutine riêng; slow
      subscriber không chặn subscriber khác và không chặn publisher
- [x] `Close(ctx)` drain hết queue mọi subscriber rồi mới return; ctx timeout
      → return error nhưng vẫn dừng worker
- [x] Handler panic được recover + log, worker sống tiếp
- [x] `SyncBus` deliver đồng bộ trong `Publish` (cho unit/integration test)

## Architecture

```go
// internal/shared/events
type Event interface{ EventName() string }
type Handler func(ctx context.Context, e Event)

type Bus interface {
    Publish(e Event)
    Subscribe(name string, buf int, h Handler)
    Close(ctx context.Context) error
}
```

- `AsyncBus`: `New(log *slog.Logger) *AsyncBus`. `Subscribe` tạo
  `chan Event` (cap=buf) + spawn worker: `for e := range ch { safeHandle(e) }`.
  `Publish` snapshot subscribers (RWMutex) rồi `select { case ch <- e: default: drop+warn }`.
- `Close(ctx)`: đóng mọi channel, đợi workers xong qua `sync.WaitGroup` với
  select trên `ctx.Done()`. Publish sau Close: no-op + warning (guard bằng
  closed flag, không panic vì send vào closed channel — check flag dưới RLock).
- Worker gọi handler với `context.Background()` (request ctx đã chết khi
  deliver async).
- `SyncBus`: giữ slice handlers, `Publish` gọi tuần tự đồng bộ, `Close` no-op.

## Related Code Files

- Create: `apps/api/internal/shared/events/events.go` (Event, Handler, Bus)
- Create: `apps/api/internal/shared/events/async_bus.go`
- Create: `apps/api/internal/shared/events/sync_bus.go`
- Create: `apps/api/internal/shared/events/async_bus_test.go`

## Implementation Steps (TDD)

1. **Test trước** — `async_bus_test.go`:
   - publish → subscriber nhận đúng event (dùng channel/WaitGroup sync trong test)
   - 2 subscribers cùng nhận 1 event (fan-out)
   - subscriber buf=1 + handler block → Publish lần 3 return ngay (không block),
     có drop
   - `Close` drain: publish N events rồi Close → handler đã xử lý đủ N
   - `Close(ctx)` với handler treo → return error khi ctx timeout
   - handler panic → worker vẫn xử lý event kế tiếp
   - Publish sau Close → không panic
2. Chạy test, xác nhận đỏ. Implement `events.go`, `async_bus.go` tới khi xanh.
3. `sync_bus.go` + test ngắn (deliver đồng bộ, thứ tự subscribe).
4. `go vet ./... && go test ./internal/shared/events/`.

## Todo

- [x] Viết async_bus_test.go (đỏ)
- [x] Implement events.go + async_bus.go (xanh)
- [x] Implement sync_bus.go + test
- [x] go vet + go test pass

## Success Criteria

- [x] Toàn bộ test phase này pass, không flaky (chạy `-race -count=3`)
- [x] Không import gì từ `features/*`
- [x] Publish path không alloc goroutine mới per-event (chỉ channel send)

## Risk Assessment

- Race publish/close → giải bằng RWMutex + closed flag; test với `-race`.
- Drop âm thầm khi buffer nhỏ → warning log bắt buộc kèm counter (đơn giản:
  log mỗi lần drop, đủ cho V1).
