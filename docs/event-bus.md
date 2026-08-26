# Event bus and audit trail

Owns: the in-process event bus contract, the event catalog, the audit capture
pipeline, and the conventions for extending both.

## Bus

`internal/shared/events` is pure infrastructure: a generic in-process bus with
non-blocking publish and per-subscriber fan-out queues. It knows nothing about
business features; event types live next to their publishers.

- **API** — `Bus.Publish(e)`, `Bus.Subscribe(name, buf, handler)`,
  `Bus.Close(ctx)`. See [`events.go`](../apps/api/internal/shared/events/events.go)
  for the contract and `async_bus.go` for the production implementation
  (`events.NewSync()` exists for tests that need in-line delivery).
- **At-most-once** — `Publish` never blocks the caller: a full subscriber
  queue drops the event with a warning log. A panicking handler loses only its
  own event, not the worker. There is no retry and no persistence upstream of
  the subscriber; losing an event under pressure is the accepted trade-off for
  never delaying a request.
- **Shutdown** — `Close` stops intake and drains every queue within the given
  context. The guarantee ends at the queue boundary: a subscriber that buffers
  internally (like the audit batcher) must flush in its own shutdown.

### Adding a subscriber

Wire it in `internal/app/container.go` next to the existing audit
subscription: construct the subscriber, then
`bus.Subscribe("<name>", bufSize, sub.Handle)`. The handler runs on its own
worker goroutine with a background context (the originating request context is
already dead), so it must be safe without request state and must return —
a handler that never returns keeps its worker alive until process exit. Close
the subscriber's own resources during app shutdown, after the bus drains.

## Event catalog

Field lists live on the structs; link, don't copy.

| Event | Published by | Purpose |
|-------|--------------|---------|
| `http.request_completed` | [`middleware/request_events.go`](../apps/api/internal/middleware/request_events.go) | One event per mutating API request (POST/PUT/PATCH/DELETE), success or failure alike |
| `auth.login_succeeded` / `auth.login_failed` / `auth.logged_out` | [`features/auth/events.go`](../apps/api/internal/features/auth/events.go) | Session lifecycle, published by the auth service itself (the middleware skips `/auth/login`, `/auth/logout`, `/auth/refresh` to avoid double-logging; refresh is deliberately not audited) |
| `invitations.member_joined` | [`features/invitations/events.go`](../apps/api/internal/features/invitations/events.go) | Public invitation accept — carries the center id the middleware cannot resolve for an unauthenticated caller |

## Audit capture pipeline

```
mutating request → RequestEvents middleware (v1 group, after the global stack)
                 → bus queue ("audit", API_AUDIT_BUFFER_SIZE)
                 → audit subscriber: batch by size or interval
                 → one multi-row INSERT into audit_logs
```

The subscriber ([`features/audit/subscriber.go`](../apps/api/internal/features/audit/subscriber.go))
maps events to rows via the action map and flushes when the batch fills or the
interval elapses. Delivery stays at-most-once end to end: a failed flush drops
its batch with an error log. Owners read the trail through
`GET /api/v1/audit-logs` (cursor-paginated, owner-only).

### Action map convention

[`features/audit/action.go`](../apps/api/internal/features/audit/action.go)
maps `METHOD /route/template` to a dot-namespaced action
(`class.create`, `contact.update`, …) plus entity type and id parameter.
**When adding a mutating route, add its entry to the action map** — an
unmapped route still produces a row, but with a fallback action derived from
the raw route instead of a stable name the web UI can filter on. The web
filter groups (`apps/web/src/features/audit/components/audit-filters.tsx`)
key off these prefixes.

### Known blind spots

Accepted trade-offs, not bugs:

- A mutation rejected with **401** leaves no row — capture skips
  principal-less requests except `/auth/forgot-password` and
  `/auth/reset-password`, so unauthenticated probing shows up in server logs
  only. Requests that match **no registered route** (plain 404s) are skipped
  too, keeping attacker-chosen paths out of the table.
- A **logout with a stale or unknown refresh token** publishes no event: an
  unknown token returns before any publish, and a token whose family was
  already revoked still triggers the (idempotent) revoke but skips the
  publish.
- Failed logins are rate-unlimited today; each attempt writes an
  `auth.login_fail` row (the phone is stored masked, never in full). Per-IP
  rate limiting of `POST /auth/login` is backlog.

## Operations

Tunables (all optional, see `AuditConfig` in
[`config.go`](../apps/api/internal/config/config.go)): `API_AUDIT_BUFFER_SIZE`
(default 1024), `API_AUDIT_BATCH_SIZE` (100), `API_AUDIT_FLUSH_INTERVAL` (1s),
`API_AUDIT_DRAIN_TIMEOUT` (5s).

Shutdown needs at least ~20s in the worst case (HTTP drain 10s + bus drain 5s
+ final flush 5s, with other subsystems closing in between). Compose files set
`stop_grace_period: 30s`; any deploy manifest outside this repo must grant a
stop grace of **at least 30s** or the final audit batch can be lost on deploy.
