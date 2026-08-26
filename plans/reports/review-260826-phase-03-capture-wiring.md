# Code Review — Phase 3: Capture Wiring (audit log)

Date: 2026-08-26
Scope: uncommitted Phase 3 changes on `master`, module `teka/apps/api`
Verdict: **DONE_WITH_CONCERNS** — no blocker; 2 Medium items worth deciding before ship.

## Scope

- Files reviewed: `internal/middleware/request_events{,_test}.go`,
  `internal/features/auth/{events.go,service.go,handler.go,events_publish_test.go}`,
  `internal/features/invitations/{events.go,service.go,handler.go,events_publish_test.go}`,
  `internal/features/audit/{subscriber.go,action.go,subscriber_test.go,capture_integration_test.go}`,
  `internal/config/config{,_test}.go`, `internal/app/{container,app}.go`,
  `internal/server/router{,_test}.go`, plus 6 signature-only test files.
- LOC: ~318 added / 96 removed across 19 tracked files + 6 new files.
- Checks re-run: `go build ./...` (clean), `gofmt -l internal/` (clean),
  `go vet -tags integration ./...` (clean), `golangci-lint run ./...` (0 issues),
  `golangci-lint run --build-tags integration ./...` (5 issues, all on lines
  untouched by this diff — verified against `git show HEAD:` — so **no new lint
  findings**; the task note understated the count, it is 3 errcheck in
  `migrations/migrations_test.go:152,255` + QF1002 in
  `features/auth/integration_test.go:286` and
  `features/invitations/integration_test.go:360`).

## Acceptance criteria

| # | Criterion | Verdict |
|---|-----------|---------|
| 1 | one `RequestCompleted` per authenticated mutating `/api/v1` request | Met **except panic-induced 500s** (M1) |
| 2 | auth events, masked phone, idempotent logout, silent internal errors | Met (one nuance, L2) |
| 3 | `MemberJoined` after commit, carries `CenterID` | Met |
| 4 | container lifetime + Close order, no silently dropped row | Met in code; time budget concern (M2) |
| 5 | publishers never import `features/audit` | Met — grep shows only `app/container.go` and audit's own tests import it |
| 6 | publish adds no I/O | Met — `AsyncBus.Publish` is RLock + non-blocking channel send |
| 7 | nil bus safe | Met — nil-guard helper in both services, covered by tests |
| 8 | config defaults + `validateAudit` + subscriber clamps | Met (minor test gap, L5) |

Regression walk of touchpoints (`auth.Login/Logout`, `invitations.Accept`,
`server.NewRouter`, `app.NewContainer/Close`): no business-logic change. Every
error branch, ordering, and return value is preserved; the only edits are the
added `meta` parameter, the extracted `rejected()` closure (same `invalid`
error, same `burnPassword` calls, same latency shape), and moving `Accept`'s
`WithinTx` result into a named `err`. Public contract changes are exactly the
three accepted signature changes. No DB schema change in this phase.

## Findings

### Critical
None.

### High
None.

### Medium

**M1 — a panicking mutation leaves no audit row**
`internal/middleware/request_events.go:66-69`
`RequestEvents` runs its publish *after* `c.Next()` returns normally, with no
`defer`. `middleware.Recovery()` is mounted on the engine (`internal/server/router.go:61-66`),
i.e. **upstream** of the v1 group, so a panic unwinds straight through
`RequestEvents` and the event is never published. The 500s most worth
investigating (a mutation that blew up mid-transaction) are exactly the ones
missing from the trail, and the plan explicitly wants "kể cả 4xx/5xx".
*Fix*: publish from a `defer` that re-panics, e.g.
`defer func() { if r := recover(); r != nil { publish(http.StatusInternalServerError); panic(r) }; publish(c.Writer.Status()) }()`
— a plain `defer` alone would record status 200, because Recovery has not
written the response yet at that point.

**M2 — shutdown time budget can exceed the container stop grace**
`internal/app/container.go:136-144`, `internal/server/server.go:14`
Worst case is now `srv.Shutdown` 10s + `Notifications.Close` + `Zalo.Close` +
`DrainTimeout` 5s + `Subscriber.Close`'s `flushTimeout` 5s ≈ 20s+. Neither
`docker-compose.yml` nor any deploy manifest sets `stop_grace_period`, so
Docker's default 10s SIGKILL lands in the middle of the drain and criterion 4's
"no audit row silently dropped" quietly stops holding under load.
*Fix*: set `stop_grace_period: 30s` for the api service (and the equivalent
`terminationGracePeriodSeconds` wherever prod runs), or lower the
`AUDIT_DRAIN_TIMEOUT` default so the total stays inside 10s.

**M3 — unauthenticated, unbounded audit row growth**
`internal/middleware/request_events.go:96-97`, `internal/features/auth/service.go:126-133`
`POST /api/v1/auth/login` has **no rate limiter** (`internal/server/router.go:111-119`
limits only forgot/reset, and those key on `phone`/`token` from the body, which
an attacker rotates freely). Every failed login now writes an `audit_logs` row —
before this phase a failed login wrote nothing. `UserAgent` and `Path` are
copied verbatim into `TEXT` columns with no length cap, and Go's default
`MaxHeaderBytes` is 1 MB, so one anonymous request can persist a ~1 MB row.
Retention/pruning is listed only as a Phase 6 follow-up.
*Fix*: truncate at the publisher (e.g. `UserAgent` to 512 and `Path` to 1024
chars in `RequestEvents` and the auth publishers) — cheap, and it also bounds
the read API's payloads in Phase 4.

### Low

**L1 — nil bus panics at request time, not construction**
`internal/middleware/request_events.go:113` calls `bus.Publish` unguarded while
both services nil-guard. `NewRouter(..., nil)` therefore builds fine and panics
on the first mutation. *Fix*: `if bus == nil { return func(c *gin.Context) { c.Next() } }`
at the top of `RequestEvents`, or panic in `NewRouter` when `bus == nil`.

**L2 — logout of a rotated-but-live session publishes nothing**
`internal/features/auth/service.go:277-287`
`alreadyRevoked := t.Revoked()` is a *per-token* flag, but `RevokeFamily` kills
the whole family. A stale cookie (tab B logs out after tab A refreshed) really
terminates a live session yet emits no `LoggedOut`. Matches the accepted spec
wording, so not a defect against scope — but the honest signal is "did
RevokeFamily change any live row". *Fix (optional)*: have `RevokeFamily` return
rows affected and publish when > 0.

**L3 — skip/allowlist maps are coupled to the mount path**
`internal/middleware/request_events.go:46-60`, `internal/features/audit/action.go:22`
Both key on absolute `/api/v1/...` templates while the prefix is decided in
`router.go`. Changing the group prefix or the auth subgroup silently
re-introduces double-logging (auth) or drops the reset-password rows, and
`request_events_test.go` builds its own router with the same hardcoded strings,
so it would not catch it. *Fix*: a router-level test asserting the real
`NewRouter` engine's registered auth routes are all present in the skip map.

**L4 — 401-rejected mutations are invisible**
An expired/forged access token aborts in `RequireAuth` before a principal
exists, so token-probing against mutating routes leaves no audit trace. This
follows the accepted "skip principal-less requests" decision; recording it as a
known blind spot for the Phase 6 follow-up list, not as a change request.

**L5 — clamp test covers only the lower bound**
`internal/features/audit/subscriber_test.go:285-297` exercises `batchSize=0`
and `interval=0` but never the `maxBatchSize` ceiling. One extra assertion
(`NewSubscriber(repo, log, 99999, time.Hour)` → internal batch 4000) closes it.

**L6 — new env vars undocumented**
`API_AUDIT_BUFFER_SIZE/BATCH_SIZE/FLUSH_INTERVAL/DRAIN_TIMEOUT` exist only in
`config.go`. `docs/api-guidelines.md` documents other tunables in prose; Phase 6
owns docs, so this is a tracking note, not a Phase 3 defect.

## Edge cases checked and found sound

- **gin context pooling**: `Params` is deep-copied into a fresh map
  (`request_events.go:107-112`) rather than aliasing `c.Params`, whose backing
  array gin recycles — the async worker cannot read another request's params.
  Every other event field is an immutable string/int.
- **Publish-after-commit**: `Accept` assigns `joined` inside the closure and
  publishes only after `WithinTx` returns nil; all three success branches set
  `UserID`, and `commitFailingTxManager` locks the rollback case in a test.
- **Query strings**: `URL.Path` is used, and the test asserts
  `?phone=0901234567` never reaches the event.
- **404 / unmatched routes**: empty `FullPath()` short-circuits before publish.
- **Container error paths**: nothing after `events.New(log)` can fail, so no
  orphaned subscriber goroutine on a failed `NewContainer`.
- **Repeat Close**: `AsyncBus.Close` shares one drain channel and
  `Subscriber.Close` is `sync.Once`-guarded; CLI `defer c.Close()` paths are safe.
- **Post-drain deliveries**: events arriving after `closed=true` are dropped
  with a warn; an insert racing `database.Close` returns an error and logs, no panic.
- **Phone masking**: `LoginRequest.Phone` is `binding:"vnphone"`, so `maskPhone`
  never sees an unbounded string; short inputs mask entirely.

## Repo-pattern conformance

Follows `validateX` config pattern, feature-package layout, event-next-to-
publisher rule, and per-package test style (plain `testing` in auth unit tests,
`require` in invitations unit tests matching that file's existing convention,
`require` + `testutil.StartPostgres` in integration tests). No new abstraction,
no parallel utility, no lint suppression, no scope drift — the six "signature
update only" files are exactly that.

## Recommended actions

1. Decide M1 (panic → no row): fix with the recover/re-panic defer, or record
   it as an accepted limitation in the phase doc.
2. Decide M2: set a stop grace period ≥ 25s, or shrink `AUDIT_DRAIN_TIMEOUT`.
3. Apply M3 truncation of `UserAgent`/`Path` at the publishers.
4. Optional: L1 nil-bus guard, L5 ceiling assertion.
5. Carry L4 and L6 into the Phase 6 follow-up list.

## Unresolved questions

- Is there a production deploy manifest outside this repo whose grace period
  should be checked for M2?
- Does the product want failed-authorization (401/403) mutations in the trail
  later, or is L4 permanently out of scope?

---

## Fixes applied after review (260826, session)

- **M1 — fixed.** `middleware/request_events.go`: publish chuyển vào `defer`
  với `recover()` — handler panic vẫn publish event status 500 rồi re-panic
  cho `Recovery` trả response. Test: `TestPanickingMutationStillPublishes`.
- **M3 (storage) — fixed.** `audit/subscriber.go`: `clip()` cắt
  `UserAgent` (512B) và `Path` (1024B) tại `Handle`, lùi về ranh giới rune
  để giữ UTF-8 hợp lệ. Test: `TestHandleClipsClientControlledStrings`.
  Phần rate-limit login là quyết định scope riêng — đưa lên user ở gate P3→P4.
- **L1 — fixed.** `RequestEvents(nil)` trả pass-through handler thay vì panic.
  Test: `TestNilBusDisablesCapture`.
- **L5 — fixed.** Thêm `TestNewSubscriberClampsOversizedBatch` phủ clamp trên
  (`maxBatchSize`).
- **M2 — chờ user.** `stop_grace_period` thuộc manifest deploy (ngoài repo);
  trình bày ở gate P3→P4.
- **L2, L4** — đúng spec đã chốt (at-most-once, skip principal-less): giữ
  nguyên, ghi nhận là blind spot cho docs Phase 6. **L3** — rủi ro refactor
  tương lai, test hiện tại khóa hành vi; không đổi. **L6** — docs thuộc
  Phase 6.

Verification sau fix: `go build`, `go vet`, `gofmt`, `golangci-lint` (0 issues),
`go test ./... -race` và `go test -tags integration ./internal/features/audit/
-race` đều xanh.
