# Code Review — Phase 2: Audit Schema And Feature Core

Plan: `plans/260826-1228-audit-log/` | Reviewer: code-reviewer subagent | Status: DONE_WITH_CONCERNS → all code findings fixed

## Verdict

All 7 acceptance criteria pass. Route-action map verified complete against all 57
mutating `/api/v1` registrations. Dependency direction verified: nothing imports
`features/audit`; subscriber holds no gin/HTTP reference. Build/vet/lint/race all
clean. Findings concentrated in shutdown lifecycle — invisible to Phase 2 tests,
would detonate at Phase 3 wiring.

## Findings and dispositions

| ID | Severity | Finding | Disposition |
|----|----------|---------|-------------|
| H1 | High | `flush()` used uncancellable `context.Background()`; saturated DB could hang graceful shutdown past orchestrator grace period | **Fixed**: `flushTimeout = 5s` bounds every insert incl. final Close flush; timeout drops batch = approved at-most-once. Test `TestFlushContextHasDeadline` |
| M1 | Medium | No closed state: events after `Close()` buffered and silently lost; wrong Phase-3 shutdown order loses bus-drained events with no signal | **Fixed**: `closed` flag under mutex; `Handle` drops+warns post-Close; `Close` doc states mandatory order `bus.Close(ctx)` → `Subscriber.Close()` → DB pool. Tests `TestCloseTwiceAndHandleAfterClose`, `TestConcurrentHandleAndClose` |
| M2 | Medium | `NewSubscriber` accepts panic/degenerate values (`flushInterval<=0` panics ticker; `batchSize=0` → N+1 inserts; `batchSize>4369` exceeds pg 65535-param limit → audit blackout) | **Carried to Phase 3**: add `validateAudit` beside existing `validateX` in config.go + defensive clamp in constructor |
| M3 | Medium | `POST /auth/forgot-password`, `/auth/reset-password`, `/auth/refresh` audited by neither action map nor service events — password reset unauditable | **User decision** (asked at P2→P3 gate) |
| M4 | Medium | `invitation.accept` is public-route: rows land with NULL center/actor → permanently invisible under Phase-4 JOIN visibility rule. Approved limitation covered login-fail only | **User decision** (asked at P2→P3 gate) |
| L1 | Low | `Metadata.Scan` lacked NULL case (breaks future outer-join projections); unmarshal-into-populated-map merges keys | **Fixed**: `case nil` + reset `*m = nil` first, matching `teaching.StringList` pattern |
| L2 | Low | Auth event rows wrote zero-UUID instead of NULL on `uuid.Nil` actor | **Fixed**: `nilIfZero` helper applied uniformly (also simplified `requestRow`) |
| L3 | Low | `idx_audit_logs_actor` (actor, occurred_at DESC) unlikely to serve any center-scoped Phase-4 query — write amplification | **Deferred to Phase 4**: confirm against real query plan, then keep/fix/drop. Index was plan-specified |
| L4 | Low | No retention/partitioning story for unbounded table | **Deferred to Phase 6 docs**: operational note |
| L5 | Low | "Safe to call more than once" untested; no concurrent Handle+Close race test | **Fixed**: covered by the two new tests above |

## Post-fix verification

- `golangci-lint run ./...` — 0 issues (also fixed pre-review revive stutter: `audit.AuditLog` → `audit.Log`)
- `go vet ./...` + `go vet -tags integration` — clean
- `go test -race -count=3 ./internal/features/audit/` — ok (12 tests)
- `go test ./...` — no FAIL across 29 packages
- Integration (docker): `TestInsertBatchRoundTrip`, `TestInsertBatchEmpty`, `TestMigrationRoundTrip` — pass

## Carry-forward into Phase 3 spec

1. Shutdown order invariant: `bus.Close(ctx)` → `subscriber.Close()` → `database.Close()` (documented on `Subscriber.Close`).
2. `validateAudit` config validation + constructor clamp (M2).
3. Publisher must use `c.Request.URL.Path`, never `RequestURI` (query strings can carry phone numbers).
4. M3/M4 outcomes per user decision at gate.

## Unresolved questions

1. M4 — accept `invitation.accept` invisible to owners in V1, or resolve center at publish?
2. M3 — password reset: service event vs middleware-captured route (NULL actor)?
3. L3 — does any Phase-4 query actually plan onto `idx_audit_logs_actor`?
