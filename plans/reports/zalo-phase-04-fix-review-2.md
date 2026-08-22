# Zalo Phase 4 — Second review of the F1–F4 fixes

Read-only adversarial review of the uncommitted tree. No production code
modified. Two temporary reproduction tests were written, run, and deleted;
their content is reproduced inline below.

## Verdict

| Item | Result |
|---|---|
| (a) Revocation guarantee — no `Upsert` after `Delete` returns | **Closed** for every attempt that exists when `Unlink` begins. Proven by lock-order analysis and a 1200-iteration randomized hammer under `-race`. One theoretical residual (R-1) that is a fresh consent, not a revocation failure. |
| (b) Deadlock / lock ordering | **Sound.** No path waits on `done` while holding `m.mu`. |
| (c) Leaks (`live` map, `done` channel) | **Sound.** `retire` runs exactly once per goroutine via defer; single `close` site. |
| (d) Unbounded blocking in the request path | **Bounded** by `persistTimeout` (10s) + one long-poll abort, under the 30s `WriteTimeout`. Not a cross-tenant pinning vector. |
| (e) Credential material in errors/logs/responses | **Clean** for everything these changes introduced or touched. |
| (f) Regressions / gates | **Green.** build, vet, full unit tests, zalo `-race -count=5`, full integration run, golangci-lint (only the 4 known pre-existing gosec findings). |

The F1–F4 fixes themselves are correct and complete. One **confirmed** defect
was found adjacent to the same revocation boundary — it predates these fixes,
lives in the session cache rather than the credential store, and has no
reachable consumer today, but it violates the same consent-revocation intent
and will become live the moment anything calls `sessionFor` for sends.

## M-1 (Medium, CONFIRMED, pre-existing) — a health probe racing `Unlink` re-caches a live session for an unlinked teacher

`Service.VerifyAccount` (`apps/api/internal/features/zalo/service.go:203-207`)
evicts, then `sessionFor` reads the row, re-logs-in over the network, and
`cache.Put`s the session (`service.go:242`). Nothing re-checks the row between
the read and the `Put`. Interleaving:

1. Probe: `Evict` → `GetByTeacher` returns the live row → enters `relogin`
   (network, seconds).
2. Teacher: `Unlink` — `Cancel` (no attempts), `Evict` (cache empty), `Delete`
   soft-deletes the row, handler returns 204.
3. Probe: `relogin` succeeds (Zalo still accepts the session; deleting our row
   revokes nothing on Zalo's side) → `cache.Put(teacherID, sess)`.
   `recordHealthy` → `MarkVerified` fails `ErrAccountNotFound` (the
   `deleted_at IS NULL` filter, `repository.go:132`) and only logs a warning.

Result: the DB is correctly empty — the stated stored-row constraint holds —
but `SessionCache` holds a **usable full-account session** for a teacher who
revoked consent, until process restart. Nothing evicts it: the probe sweeps
`ListLinked()`, which no longer names the teacher.

Reproduced deterministically (test failed exactly as predicted, then deleted):

```go
func TestTempVerifyAccountRacingUnlinkLeavesCachedSession(t *testing.T) {
	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	relogin := func(_ context.Context, sess *protocol.Session, _ protocol.Credentials) error {
		close(entered)
		<-release
		sess.UID = "zalo-uid"
		return nil
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: relogin})

	done := make(chan error, 1)
	go func() { done <- svc.VerifyAccount(context.Background(), teacherID) }()
	<-entered
	require.NoError(t, svc.Unlink(context.Background(), teacherID))
	close(release)
	<-done

	_, err := repo.GetByTeacher(context.Background(), teacherID)
	require.ErrorIs(t, err, ErrAccountNotFound) // passes — the row stays deleted
	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached) // FAILS — a live session survives revocation
}
```

Severity calibration: **latent**. Today `sessionFor` is called only by
`VerifyAccount`, and the probe never selects an unlinked teacher again, so no
code path can read the poisoned entry. But `Service`'s own doc comment names
`sessionFor` as the thing that "hands out live sessions to whatever needs to
act as the teacher" — the first send endpoint makes this exploitable-by-race.
It is not introduced by the F1–F4 fixes and should not block them; it must be
fixed before any consumer of `sessionFor` ships.

Direction for a fix (design is the lead's call): ordering alone cannot close
it — gating `cache.Put` on `MarkVerified` succeeding and moving `Unlink`'s
`Evict` after `Delete` each shrink but do not eliminate the window, because the
probe can pause between its row check and its `Put` while a whole `Unlink`
completes. A robust fix needs a revocation fence in the cache itself, e.g. a
per-teacher generation counter: `Evict` bumps it, `sessionFor` snapshots it
before `GetByTeacher`, and `Put` is a no-op if the generation moved.

## R-1 (Informational, PLAUSIBLE — theoretical only) — a `StartLink` racing `Unlink` can persist after `Delete`

The one surviving `Upsert`-after-`Delete` schedule: `Begin`'s critical section
runs after `Cancel`'s (`link_manager.go:183-193` vs `:227-233`), so the new
record is in neither `Cancel`'s `live` snapshot nor its `active` delete; it is
the current attempt, passes `isCurrent`, and its persist can land after
`Unlink`'s `Delete`. This requires the teacher to open a new QR, scan it, and
approve on the phone inside the `Cancel`→`Delete` window — humanly implausible,
and it is a *fresh consented link*, fully reflected in the UI (`LinkStatus`
reports `linked`). The alternative gate-only design in the previous review has
the identical residual. Accepted limitation; at most worth a sentence on the
`Unlink` doc comment. Not a violation of the revocation guarantee, which is
about attempts existing at unlink time.

## Why (a) is closed for every pre-existing attempt

All the relevant critical sections are totally ordered by `m.mu`: `isCurrent`
(`link_manager.go:341-345`), `Cancel`'s delete-and-snapshot (`:227-233`), and
`retire` (`:349-357`). Case analysis:

- `isCurrent` after `Cancel`'s CS → `active[T]` is deleted → `false` → no
  persist. Covers supersede-then-unlink (F1's original schedule: the
  superseded record additionally fails `isCurrent` the moment `Begin` replaced
  it), sweep eviction (`sweepLocked` removes from `active`, `:375-382`), and
  `Close`.
- `isCurrent` before `Cancel`'s CS → the record was current, persist may start;
  it cannot have `retire`d (retire runs after `onLinked` returns, `:266,307`),
  so it is still in `live[T]` at snapshot time → `Cancel` waits on its `done`,
  which closes only after the `Upsert` completed → `repo.Delete` runs strictly
  after. The check→write window is exactly what the wait covers.
- `Begin` adds to `live` under the same lock before spawning (`:183-195`), so
  no goroutine exists that `live` does not name. Concurrent `Unlink`s and
  `Cancel`+`Close` each take their own snapshot and wait on the same closed
  channels — safe.
- Health probe writes go through `MarkVerified`/`UpdateStatus`, both filtered
  on `deleted_at IS NULL` (`repository.go:129-141`) — cannot revive a deleted
  row. `repo.Upsert`'s only caller in the codebase is `persistLink`
  (`service.go:323`), verified by grep.
- F2: `repo.Delete` now runs on `context.WithoutCancel` with `unlinkTimeout`
  (`service.go:180-186`), so a client disconnect during the wait can no longer
  skip the delete.

Empirical: a temporary hammer (400 iterations × `-count=3` under `-race`;
random supersede, random scan-vs-unlink timing at µs granularity) asserted
after every `Unlink` return that no row and no cached session existed. Zero
failures. (The in-flight-persist `cache.Put` at `service.go:326` is inside
`Cancel`'s wait, so `Unlink`'s `Evict` at `:174` always runs after it —
confirmed by the hammer's cache assertion.)

## (b) Deadlock and lock ordering — sound

- `Cancel` and `Close` release `m.mu` before any `<-rec.done`
  (`link_manager.go:233-238, 255-259`).
- `retire` takes `m.mu`, releases it, then closes `done` (`:349-358`) — never
  the reverse.
- The attempt goroutine takes `m.mu` only in `update`/`isCurrent`/`retire`,
  all short pure sections; `onLinked` (`persistLink`) touches no manager state.
- `Service.Close` → `stopProbe` waits on `probeDone` under no manager lock
  (the probe goroutine takes none), then `links.Close` (`service.go:112-115`).
  Both are idempotent; `Close` racing `Cancel` waits on the same
  already-closing channels.
- Re-entrancy: an `OnLinked` that called `Cancel`/`Unlink` for the same teacher
  would deadlock; the F3 doc note (`link_manager.go:60-66`, "Cancel blocks
  until it returns … should stay as short as a single store") records exactly
  this. Present implementation is safe.

## (c) Leaks — none

- `retire` is the first deferred call in `run` (`:266`), so it executes on
  every exit path; it is the only `close(rec.done)` site and runs once per
  record, so `done` can neither close twice nor stay open for a record that
  ever reached `go m.run` — and every record `Begin` creates does.
- A `live` entry therefore cannot outlive its goroutine, and the per-teacher
  inner map is deleted when empty (`:353-355`). Growth is bounded by running
  goroutines: every superseded record's context is already cancelled by
  `Begin` (`:185-187`), and production `LoginQR` honours cancellation.
- `Begin` after `Close` is benign: the attempt derives from the cancelled
  `baseCtx`, fails immediately as expired, and retires.

## (d) Blocking bound — acceptable

Worst case for `Cancel`: every superseded record was already cancelled at
`Begin`; the only cancellation `Cancel` itself must trigger is the current
attempt's, and cancelled long-polls abort in one round-trip (`http.Client.Do`
returns immediately on context cancellation; the loops check `ctx.Err()`,
`auth.go:402, 430`). Records already inside the persist finish within
`persistTimeout` = 10s, concurrently, so the sequential wait loop's total is
≈ max(remaining persist, poll abort) ≈ 10s, under the 30s `WriteTimeout`
(`internal/server/server.go:24`). A persist is in flight only when *that
teacher's own* scan just succeeded — `teacherID` comes from `authctx`
(`handler.go:31-38`), so no client can pin another tenant's unlink; spamming
DELETE against one's own account burns one's own 10s at worst. Micro-nit, not
required: cancelling all pending records before the first wait would shave the
poll-abort latency out of the sum.

## (e) Credential material — clean

- All six request-construction sites route through `newRequest`
  (`auth.go:232, 288, 313, 351, 460, 476`), which unwraps `*url.Error` and
  keeps only `urlErr.Err` (`protocol/client.go:101-111`); the inner parse
  errors quote at most a single offending character, never the URL, so the
  IMEI/ZCID query can no longer surface (F4 closed). All six `Do` sites still
  route through `doRequest`'s scheme://host/path scrubbing.
- The F1 mechanisms add no new log or error text containing record contents;
  `fail` logs upstream detail (pre-existing, deliberate — clients get only
  `linkFailureMessage`, asserted by `TestLinkManagerReportsAFailedLoginWithoutLeakingItsCause`).
- Responses: `LinkSnapshot`/DTOs carry no credential fields; `unlink` returns a
  bare 204 (`handler.go:135-145`).

## (f) Gates — all green, run against this tree in the project container

| Gate | Result |
|---|---|
| `go build ./... && go vet ./... && go test ./... -count=1` | pass |
| `CGO_ENABLED=1 go test -race -count=5 ./internal/features/zalo/...` | pass |
| `go test -tags=integration ./... -count=1` (full, real Postgres) | pass — the single reported failure was this review's own temporary repro test, present in the mount during the run; the zalo package was re-run clean after its deletion |
| `golangci-lint run ./...` | only the 4 pre-existing gosec findings (G101 teachers, G124/G115 protocol) in unmodified files |

Both temporary test files
(`apps/api/internal/features/zalo/tmp_review_race_test.go`, `…/tmp_review_hammer_test.go`)
are deleted; `git status` for the zalo tree shows only the feature's own
untracked files.

## Recommended actions

1. Accept the F1–F4 fixes — the revocation guarantee holds for every attempt
   existing at unlink time, empirically and by construction.
2. Schedule M-1 (probe/`Unlink` cache race) as its own item, blocking the first
   consumer of `sessionFor`, with a revocation-generation guard in
   `SessionCache` as the suggested shape.
3. Optional: one sentence on `Unlink`'s doc comment acknowledging R-1.

## Unresolved questions

- Should M-1's fix land in this phase (it shares the consent-revocation
  acceptance criterion) or as a Phase-5 precondition? Product call for the
  lead.
- Still open from the earlier review, unaffected here: durable audit trail when
  the probe expires an account.

## Resolution

**M-1 — fixed, not deferred.** The review rated it latent and non-blocking, and
that is a fair read of today's call graph. It was fixed anyway: the whole
feature is still uncommitted, so "pre-existing" here means "from an earlier
phase of this same unshipped work", and a working account session surviving
revocation is the exact failure the consent constraint exists to prevent.
Leaving it for the first `sessionFor` consumer would mean shipping a known
revocation hole and relying on a future change to remember it.

The shape is the one the review suggested. `SessionCache` counts evictions per
teacher; `sessionFor` reads the count before it starts the credential read and
the login, and stores through `PutUnlessEvicted`, which refuses if the count
moved. A restore the teacher's unlink overtook is discarded and the caller gets
`ErrNotLinked` — the session is real and Zalo would still honour it, which is
precisely why it must not be kept or handed back. The count has to live beside
`sessions` rather than inside an entry, because it must outlive the entry it
describes. `TestUnlinkDuringAHealthCheckLeavesNoUsableSession` reproduces the
review's interleaving and fails against the pre-fix code.

Ordering alone was rejected for the reason the review gives: the probe can pause
between its row check and its `Put` while an entire `Unlink` completes, so no
arrangement of the existing calls closes the window.

**R-1 — accepted, documented.** A scan the teacher starts *and finishes* after
`Unlink` has begun re-links the account. That is a fresh consent, visible in the
UI, not a survivor of the revoked one; `Service.Unlink`'s doc comment now says
so.

Gates re-run after the fix: build, vet, full unit suite, zalo `-race -count=5`,
the full integration suite against a real Postgres, and golangci-lint — clean,
with only the four pre-existing gosec findings in unmodified files.
