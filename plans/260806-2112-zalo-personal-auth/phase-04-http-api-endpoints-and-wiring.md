---
phase: 4
title: "HTTP API endpoints and wiring"
status: completed
priority: P1
effort: "0.5d"
dependencies: [3]
---

# Phase 4: HTTP API endpoints and wiring

## Overview

Expose the Phase 3 service over HTTP under `/me/zalo`, behind the existing
`requireAuth` middleware, and wire the service into the router/container.
Handlers are thin: bind, resolve `teacherID` from `authctx`, call the service,
map errors through `apperror`. The overriding rule: **no endpoint ever returns
credential material** — not the cookie jar, not the IMEI, not the secret key.

## Requirements

- Functional endpoints (all auth-scoped to the caller's teacher):
  - `GET  /me/zalo` → `{ linked, display_name?, status?, linked_at? }`. `status`
    is read straight from `zalo_accounts`, which the Phase 3 health probe keeps
    fresh — so a session that died since the last use can already read `expired`.
  - `POST /me/zalo/link/start` → request body `{ consent_version }`;
    responds `{ link_id }` (202-style; starts the goroutine). A missing/blank
    `consent_version` is `400` — no link attempt starts without acknowledged
    consent (Phase 2 persists it on success).
  - `GET  /me/zalo/link/status?id=<link_id>` → `{ state, qr_png?, display_name?, error_message? }`.
  - `DELETE /me/zalo` → unlink; `204`.
- Non-functional:
  - `link/status` returns the QR PNG as a base64 data field, safe to drop into
    an `<img src="data:image/png;base64,…">`.
  - A `link_id` belonging to another teacher (or unknown) → `404`, never leaks
    another teacher's attempt.
  - Errors map cleanly: `ErrLinkNotFound`→404, `ErrLinkExpired`→409/expired
    state in body, not linked on `GET`→`{linked:false}` (200, not 404).
  - Response DTOs are hand-built; no struct embeds `protocol.Credentials`.

## Architecture

Follow the notifications feature's shape exactly (`handler.go` + `dto.go` +
`routes.go`, registered in `server/router.go`). `teacherID` comes from
`authctx.TeacherID(c)` — the real signature is `TeacherID(c *gin.Context)
(uuid.UUID, bool)`; handle the `!ok` case as `401` like sibling handlers.
Rate-limiting is out of scope for auth (there is no bulk loop here); a teacher
hammering `link/start` only supersedes their own prior attempt (Phase 3
behavior).

**Wiring** (`server/router.go`, alongside the other `NewService` calls near the
notifications block ~line 153):

```go
zaloRepo := zalo.NewRepository(db)
zaloSvc  := zalo.NewService(zaloRepo, container.ZaloCipher) // cipher built in container
zaloSvc.StartHealthProbe(appCtx)                            // second background component (Phase 3)
zalo.RegisterRoutes(v1, zalo.NewHandler(zaloSvc), requireAuth)
```

The `crypto.Cipher` is constructed once in `app/container.go` from
`cfg.Zalo.CredKey` and threaded through (add a `ZaloCipher *crypto.Cipher` field
to `Container`, or pass `cfg` into the router which already receives it — match
whichever injection style `router.go` currently uses for `cfg`).

**Health-probe lifecycle:** the probe goroutine must start with the server and
stop on shutdown. Start it from an application-scoped context (the one cancelled
on graceful shutdown), not a per-request context. If `router.go` has no
long-lived context to hand out, start the probe in `app/container.go` /
`server` bootstrap where the shutdown context already lives, and only register
routes here. Match the existing lifecycle wiring rather than inventing one.

## Related Code Files

- Create: `apps/api/internal/features/zalo/handler.go`
- Create: `apps/api/internal/features/zalo/dto.go`
- Create: `apps/api/internal/features/zalo/routes.go`
- Create: `apps/api/internal/features/zalo/handler_test.go`
- Create: `apps/api/internal/features/zalo/integration_test.go`
- Modify: `apps/api/internal/server/router.go` (construct + register)
- Modify: `apps/api/internal/app/container.go` (build `crypto.Cipher` from config) — only if the cipher is threaded via the container
- Modify: `apps/api/docs/` API reference if the repo publishes one for routes (check `apps/api/docs/docs.go`; update only if it enumerates endpoints)

## Implementation Steps

1. Write `dto.go`: `LinkStartRequest{ConsentVersion}` (bind + `binding:"required"`),
   `LinkStartResponse{LinkID}`, `LinkStatusResponse{State, QRPNG?, DisplayName?, ErrorMessage?}`,
   `StatusResponse{Linked, DisplayName?, Status?, LinkedAt?}`. No credential fields on any response.
2. Write `handler.go`: four handlers, `authctx` teacher resolution (`!ok → 401`),
   `apperror` mapping; `link/start` binds+validates `consent_version` (blank → 400)
   and passes it to `zaloSvc.StartLink`.
3. Write `routes.go`: `RegisterRoutes(rg, h, requireAuth)` mounting the group.
4. Build the `crypto.Cipher` in the container from `cfg.Zalo.CredKey`; wire the service in `router.go`.
5. `handler_test.go` (table tests with a stubbed service) + `integration_test.go`
   (real DB via `testutil/postgres`, stubbed/faked protocol login) asserting:
   status of an unlinked teacher = `{linked:false}`; another teacher's `link_id` = 404;
   unlink returns 204 and flips status.
6. `go build ./... && go test ./internal/features/zalo/... ./internal/server/...`.

## Success Criteria

- [x] All four endpoints respond behind auth; unauthenticated → 401.
- [x] `POST /me/zalo/link/start` without `consent_version` → 400 and starts no attempt.
- [x] `GET /me/zalo` on an unlinked teacher returns `{linked:false}` with 200.
- [x] `link/status` with another teacher's `id` returns 404.
- [x] No response type can carry credential material — asserted twice: over real
      response bodies (canary IMEI/cookie/secret key never appear) and over the
      response structs by reflection, so a later edit cannot reintroduce it.
- [x] Integration test passes against a real DB.

## Risk Assessment

- **Credential leak through a DTO:** the whole point. Mitigation: hand-built DTOs
  + an explicit test grepping response types; reviewer checkpoint (this phase is
  the security-sensitive surface for red-team).
- **IDOR on `link_id`:** Mitigation: `LinkManager` records are keyed by
  `(teacherID, linkID)`; a mismatched teacher gets 404 (tested).
- **Wiring style drift:** match `router.go`'s existing `cfg` threading rather
  than inventing a new injection path (KISS/consistency).

## Execution Notes

- **The health probe and the link goroutines are owned by `app.Container`.**
  `NewRouter` had no long-lived context and no shutdown hook, so the zalo
  service is the one feature built in `container.go` and passed into
  `NewRouter`; `RunServer` starts the probe on the server's context and
  `Container.Close` stops it. `Service.Close` cancels the probe on its own
  derived context rather than trusting the caller to cancel first, so stopping
  the service cannot deadlock on a context nobody cancelled.
- The cipher is built before the database connection: a credential key that
  cannot make a cipher is a reason not to start, and failing first means there
  is nothing to close on the error path.
- **Blank consent is rejected by the service, not by a binding tag.** One rule
  in one place: `StartLink` already refuses a blank version, so the handler
  binds without `binding:"required"` and maps `ErrConsentVersionRequired` to
  400. An empty body lands on the same 400 through `validation.BindError`.
- A malformed or missing `?id=` is answered 404, exactly like another teacher's
  id. Distinguishing them would tell a caller which ids exist.
- `ErrLinkExpired` is mapped to 409 although no endpoint can currently return
  it. The sending milestone will; leaving it unmapped would turn a known state
  into a 500 the first time it happens.
- `docs/` was regenerated with `make api-docs`; the diff is additive (the four
  new routes and three response schemas), and the generated schemas contain no
  credential field.
- `router_test.go` now builds a real zalo service with a throwaway key instead
  of receiving a nil one — a nil service registered on live routes is a panic
  waiting for the first caller.
- **`DELETE /me/zalo` briefly deviated from this file's `204` spec, then was
  brought back to it.** The delivered code returned `404` when nothing was
  linked (two tests pinned that behavior). A code review
  (`plans/reports/zalo-phase-04-code-review.md`, finding M-4) flagged the
  mismatch against this file's Requirements section; the accepted plan's
  unconditional `204` was treated as authoritative, so `Service.Unlink` was
  changed to return `nil` when there is no row, and the endpoint is now
  idempotent — a double-click or an unlink issued during an attempt that
  never persisted no longer surfaces as an error toast. A third test pinning
  the old `404` (beyond the two the review found) was located and fixed
  afterward in `integration_test.go`, which now asserts back-to-back
  `DELETE /api/v1/me/zalo` calls both return `204`. `docs/` was regenerated a
  second time so the OpenAPI spec matches: `apps/api/docs/swagger.yaml`'s
  `/me/zalo` delete operation now reads "Idempotent: answers 204 whether or
  not an account was linked," with no `404` response documented. Code, tests,
  and the generated spec agree.
