---
phase: 3
title: "Session manager and QR link flow"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1, 2]
---

# Phase 3: Session manager and QR link flow

## Overview

The service layer that turns the protocol port + encrypted store into a usable
link flow. Two in-process components plus a service:

- **LinkManager** — orchestrates one QR login attempt as a background goroutine,
  exposing its state (and the QR PNG) for polling. This is the API's **first
  background component**; scope it tightly.
- **SessionCache** — `map[teacherID]*protocol.Session`, lazily populated by
  re-login from stored creds. Assumes the single homelab replica.
- **HealthProbe** — a periodic sweep goroutine (the API's **second** background
  component) that verifies linked sessions and flips `status` to `expired` early,
  so the profile card is honest without waiting for a send. (Validation session 1
  — chose proactive detection over lazy-only.)
- **Service** — `StartLink`, `LinkStatus`, `Status`, `Unlink`, and an internal
  `sessionFor(teacherID)` the send milestone will later reuse.

No HTTP here (Phase 4); no sending (next milestone).

## Requirements

- Functional:
  - `StartLink(teacherID, consentVersion)` → creates a `link_id`, spawns a
    goroutine running `protocol.LoginQR`, returns immediately with the `link_id`.
    The goroutine's `cb.OnQR` stores the PNG into the link record; `cb.OnProgress`
    advances the record through `scanned`/`confirmed`. `consentVersion` is
    carried on the record and written to `zalo_accounts` on success.
  - `LinkStatus(teacherID, linkID)` → `{state, qr_png?, display_name?, error?}`
    where state ∈ `pending|qr_ready|scanned|confirmed|linked|expired|error`.
    `scanned`/`confirmed` come from the ported `LoginQR`'s progress callback
    (Phase 1). On `linked`, credentials + consent are encrypted/persisted and the
    session cached.
  - `Status(teacherID)` → `{linked bool, display_name?, status?}` read from the
    repo (no network).
  - `Unlink(teacherID)` → soft-delete the row and evict the cached session.
  - `sessionFor(teacherID)` → cached live session, or re-login from stored creds
    (updating `last_verified_at`), or `ErrNotLinked` / marks `expired` on failure.
  - **HealthProbe** → on a ticker (default ~15m, configurable, with jitter),
    iterate `linked` accounts, verify each session cheaply (reuse
    `sessionFor` + a lightweight authenticated call), update `last_verified_at`
    on success, flip `status='expired'` + evict the cache on failure. Runs under a
    context cancelled at shutdown; skips work when no accounts are linked.
- Non-functional:
  - A link attempt self-expires (~100s, matching `LoginQR`'s own timeout) and
    is swept from the map; a bounded number of concurrent attempts per teacher
    (a new `StartLink` supersedes the previous one).
  - The goroutine respects a context tied to the attempt; `Unlink`/shutdown
    cancels it. No goroutine leak on server stop.
  - QR PNG is returned base64 in the status payload, never written to disk.

## Architecture

```
StartLink ──► LinkManager.begin(teacherID, consentVersion)
                 linkID = id.New()
                 ctx, cancel = context.WithTimeout(bg, 105s)
                 rec = &linkRecord{state: pending, consentVersion}
                 go run(ctx, rec):
                     sess = protocol.NewSession()
                     cb = protocol.QRCallbacks{
                         OnQR:       func(png){ rec.set(qr_ready, png) },
                         OnProgress: func(s){ rec.advance(s) },  // scanned, confirmed
                     }
                     cred, err = protocol.LoginQR(ctx, sess, cb)
                     on success:
                         enc = crypto.Seal(json(cred))
                         repo.Upsert(teacherID, enc, sess.UID, displayName, rec.consentVersion)
                         cache.put(teacherID, sess)
                         rec.set(linked, displayName)
                     on err/timeout: rec.set(expired|error)
```

**State granularity vs. `LoginQR`:** goclaw's `LoginQR` only calls back with the
PNG — scan/confirm are internal long-polls. Phase 1 therefore threads a progress
callback (`cb.OnProgress`) into the ported `LoginQR` at the `waiting-scan` /
`waiting-confirm` boundaries so LinkManager can advance the record through
`scanned` → `confirmed` → `linked`. (Validation session 1 chose this over the
coarser `qr_ready → linked` v1 — the "đã quét, chờ xác nhận" step is real signal,
not a client-side guess.) The callback is optional at the protocol layer: a nil
`OnProgress` degrades to PNG-only without breaking the port.

**SessionCache concurrency:** guard the map with a `sync.RWMutex`. Values are
`*protocol.Session` (holds an `*http.Client` with a cookie jar). Eviction on
unlink and on re-login failure.

**Re-login + verification triggers:** two paths, complementary.
- *On demand:* `sessionFor` re-logins from stored creds on a cache miss. The
  first post-restart action needing Zalo pays the re-login latency once.
- *Proactive (HealthProbe):* a ticker-driven sweep verifies `linked` accounts on
  an interval and flips `status='expired'` on failure, so `GET /me/zalo` (repo
  read) reflects a dead session without waiting for a send. Keep the interval
  conservative (~15m) with jitter — frequent programmatic pings risk Zalo
  flagging the session as bot-like, so this trades tight freshness for safety.

## Related Code Files

- Create: `apps/api/internal/features/zalo/service.go`
- Create: `apps/api/internal/features/zalo/link_manager.go`
- Create: `apps/api/internal/features/zalo/session_cache.go`
- Create: `apps/api/internal/features/zalo/health_probe.go` (ticker sweep, expired detection)
- Create: `apps/api/internal/features/zalo/errors.go` (`ErrNotLinked`, `ErrLinkExpired`, `ErrLinkNotFound`)
- Create: `apps/api/internal/features/zalo/service_test.go`
- Create: `apps/api/internal/features/zalo/link_manager_test.go`
- Create: `apps/api/internal/features/zalo/health_probe_test.go`

## Implementation Steps

1. Define `errors.go` and the service struct (`repo`, `cipher *crypto.Cipher`,
   `cache *SessionCache`, `links *LinkManager`, `probe *HealthProbe`).
2. Write `SessionCache` (RWMutex map, `get`/`put`/`evict`).
3. Write `LinkManager`: `begin(teacherID, consentVersion)` spawns the goroutine
   with a `QRCallbacks` carrying `OnQR`+`OnProgress`; `record(linkID)` reads
   state; a background sweep (or lazy TTL check on read) drops expired records.
   Use `context.WithTimeout`; store `cancel` on the record.
4. Write the service methods. `sessionFor` = cache hit → else decrypt+`LoginWithCredentials`
   → cache → on failure `repo.UpdateStatus(expired)` and return `ErrLinkExpired`.
5. Write `HealthProbe`: a `Start(ctx)` that runs a jittered ticker calling a
   `verify(teacherID)` (which delegates to `sessionFor` + a cheap authenticated
   call); on failure flip status + evict. Injectable interval + clock so the test
   drives ticks deterministically without real sleeps.
6. Tests: `link_manager_test.go` with a **fake login func** injected (do not hit
   the network) — assert transitions pending→qr_ready→scanned→confirmed→linked
   (driving `OnProgress` from the fake) and →expired on timeout, and that
   `Upsert` (with the consent version) + `cache.put` happen exactly on success.
   `service_test.go` for `sessionFor` cache/reload/expire branches with a stub
   repo+cipher. `health_probe_test.go`: a linked account whose fake verify fails
   is flipped to `expired` and evicted on the next tick; a passing one is not.
7. `go test ./internal/features/zalo/...`.

## Success Criteria

- [x] `StartLink` returns without blocking; the goroutine drives state through
      `qr_ready → scanned → confirmed` as the (fake) `OnQR`/`OnProgress` fire.
- [x] On fake-success, exactly one `Upsert` (carrying encrypted bytes **and** the
      consent version) and one `cache.put` occur; `LinkStatus` reports `linked` +
      display name.
- [x] On timeout, the record becomes `expired` and is swept; no goroutine leak
      (assert via a done-channel in the test).
- [x] `sessionFor` re-logins from stored creds on a cache miss and marks
      `expired` when re-login fails.
- [x] `HealthProbe` flips a linked account whose verify fails to `expired` and
      evicts it on the next (test-driven) tick; a healthy account is untouched.
- [x] `Unlink` evicts the cache and soft-deletes the row.

## Execution Notes

- **HealthProbe verifies by re-login.** The plan assumed "`sessionFor` + a cheap
  authenticated call". Phase 1 ported the auth subset only — there is no
  authenticated call to make — so `VerifyAccount` evicts the cached session and
  forces `LoginWithCredentials`. That is the honest check (a cached session
  proves only that a login once worked), and it is why the interval stays long.
- The repository gained `MarkVerified` and `ListLinked`. Phase 2 could not know
  it needed them: `last_verified_at` had no writer and the probe needs the
  linked roster. Both are covered by integration tests against real SQL.
- `LinkManager` owns a base context cancelled by `Close()` rather than accepting
  a shutdown context per attempt. `Close()` also waits for each goroutine to
  return, so "no goroutine leak on stop" is enforced rather than assumed.
- Finished records are swept lazily on every read and start instead of by a
  background sweeper — a third goroutine bought nothing here. A record stays
  readable for `Retention` (2m) past its deadline so a late poll still learns
  the outcome instead of an unexplained "not found".
- `sessionFor` restores `status='linked'` when a previously expired account logs
  in again. A network blip is indistinguishable from a dead session at the
  moment it happens, so expiry has to be reversible by evidence.
- `LinkStatus` never carries upstream error text: Zalo writes it, it can quote
  the request that produced it, and a teacher can do nothing with it. The detail
  is logged; the client gets one fixed message.
- The persist that follows a successful scan runs on `context.WithoutCancel` +
  its own 10s timeout. The attempt deadline bounds how long a teacher may take
  to scan; it must not abort the write that makes their scan count.
- **Because that write survives cancellation, unlinking has to outrank it, and
  two mechanisms are needed rather than one.** `LinkManager` tracks every
  attempt whose goroutine is still running — not only the one it currently names
  — so `Cancel` and `Close` wait for superseded and swept attempts too; and
  `run` re-checks that the attempt is still the teacher's current one before
  persisting, so an abandoned scan is never stored. The gate alone leaves the
  instant between check and write; the wait alone misses attempts the manager
  has already let go of, which is how a superseded scan could resurrect a
  soft-deleted row (`repository.go` clears `deleted_at` on conflict) with live
  credentials and no state anywhere reporting it. `Service.Unlink`'s delete runs
  detached from the request context for the same reason: a client that
  disconnects during that wait must still end up unlinked. Both paths are
  covered by tests that fail against the single-mechanism versions.
- **Revocation reaches the session cache as well as the row.** Restoring a
  session costs a network login, and deleting Teka's row revokes nothing on
  Zalo's side, so a health check that began before an unlink still succeeds
  after it and would cache a working account session nothing would ever drop —
  the probe only sweeps accounts still linked. `SessionCache` therefore counts
  evictions per teacher; `sessionFor` reads the count before it starts and
  stores through `PutUnlessEvicted`, which refuses when the count moved, and
  returns `ErrNotLinked` instead. Reordering the existing calls cannot close
  this: the probe can stall between its row check and its store while a whole
  unlink completes.
- Probe jitter defaults to a third of the interval and is drawn from
  `crypto/rand`, which sidesteps the insecure-randomness lint entirely at a cost
  that is irrelevant a few times an hour.

## Risk Assessment

- **Goroutine leak / unbounded attempts:** Mitigation: per-teacher single active
  attempt (supersede + cancel prior), hard timeout, context cancellation on
  shutdown wired through the container's lifecycle.
- **Testing a network protocol without the network:** Mitigation: inject the
  login function (`type loginFunc func(ctx, *Session, cb) (*Credentials, error)`)
  so `LinkManager` is unit-testable; the real `protocol.LoginQR` is the
  production value.
- **Single-replica assumption:** documented; a second replica would not see
  another's cached session or in-flight link, and each would run its own health
  probe. Out of scope; note in code comment.
- **Health-probe detection risk:** frequent authenticated pings can make Zalo
  flag the session as automated. Mitigation: conservative default interval
  (~15m) + jitter, verify with the cheapest authenticated call available, and
  make the interval configurable so it can be widened without a code change.
- **Two background components now:** LinkManager + HealthProbe both hold
  goroutines. Mitigation: both take a shutdown-cancelled context wired through
  the container lifecycle; assert no leak in tests (done-channel / injected
  clock), and keep the probe a single sweep loop, not a goroutine-per-account.
