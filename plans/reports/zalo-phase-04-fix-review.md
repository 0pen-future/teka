# Zalo Phase 4 — Review of the C-1 / H-2 / M-4 / M-3 / L-6 fixes

Read-only review. No source modified. `handler.go`, `integration_test.go`, and
`docs/{docs.go,swagger.json,swagger.yaml}` were edited by another agent at
23:26–23:27, mid-review; those two changes are verified in their own sections
below. Findings are stated against the tree as of 23:40.

## Verdict per acceptance criterion

| # | Criterion | Result |
|---|---|---|
| a | No credential material in any error, log, or response | **Holds.** One unreachable latent path (F4). |
| b | `Cancel`'s wait cannot deadlock or block unboundedly | **Holds for deadlock and for the bound.** But the guarantee it was added to provide is incomplete — F1. |
| d | Idempotent DELETE breaks no caller | **Holds.** No Go or TS caller depended on the 404. |
| e | No new lint/type/build/test errors | **Holds.** All gates green. |
| f | No plan ids, phase numbers, or audit labels in code | **Holds.** Grep over `zalo/` and `secrets/` returns nothing. |

## F1 (High, open) — a *superseded* attempt can still resurrect an unlinked account

`Cancel` waits only on the record currently in `active[teacherID]`
(`apps/api/internal/features/zalo/link_manager.go:210-222`). A record that `Begin`
superseded is cancelled and dropped from the map without a wait
(`link_manager.go:177-180`), and its goroutine may already be inside the
uncancellable persist (`link_manager.go:277-282`, bounded by `persistTimeout` =
10s, `:293`).

Failure scenario:

1. Attempt A's scan succeeds; A enters `onLinked` → `persistLink` → `repo.Upsert`
   on `context.WithoutCancel`. The write is slow (pool contention, replica lag).
2. The teacher's UI is still showing "confirmed", so they close and reopen the
   modal. `Begin` starts attempt B: A is cancelled and removed from `active`,
   nothing waits for it.
3. The teacher unlinks. `Cancel` finds B, waits on B's `done` (immediate — B is
   cancelled), then `repo.Delete` soft-deletes the row.
4. A's `Upsert` lands. `repository.go:78` clears `acc.DeletedAt` and the
   `OnConflict{UpdateAll}` clause revives the row with live credentials.

Net: the account is linked again after a successful unlink. Worse than the
original H-2, because `update` no-ops for a superseded record
(`link_manager.go:314-321`), so no state anywhere reflects it — the profile card
says "chưa kết nối" while the row holds usable credentials.

The `Cancel` doc comment (`:205-209`) states this exact invariant — "returning
early would let that write land after the caller removed the account" — so the
supersede path contradicts the comment as written.

Fix: waiting is the wrong lever, because it can only cover records the manager
still tracks. Do what the original review suggested instead — make the write
itself refuse to land when the attempt is no longer current. Pass the link id
into `OnLinked`, and inside `persistLink` re-check `m.active[teacherID].id ==
linkID` under `m.mu` immediately before `Upsert`. That closes the supersede path,
the sweep path, and any future path, and it lets `Cancel` stop blocking at all.

## F2 (Medium, open) — `Unlink`'s delete runs on a request context that the new wait can outlive

`apps/api/internal/features/zalo/service.go:169-181`:

```go
s.links.Cancel(teacherID)   // now blocks up to persistTimeout (10s)
s.cache.Evict(teacherID)
err := s.repo.Delete(ctx, teacherID)   // ctx is c.Request.Context()
```

`repo.Delete` goes through `database.FromContext(ctx, r.db)`
(`repository.go:102-105`), so a cancelled context stops the statement reaching
Postgres. Before this fix `Delete` ran within microseconds of the handler
entering; now it runs after a wait that is bounded only by `persistTimeout`. If
the client disconnects during that wait — teacher navigates away, closes the tab,
mobile network drops, proxy closes the connection — Gin cancels the request
context, `Delete` fails with `context.Canceled`, and the row that the persist just
wrote stays.

Not silent (the caller gets a 500 or an aborted connection), so a teacher who is
watching will retry. But this is consent revocation, and the wait was added
specifically so the delete beats the persist. Fix: run the delete on
`context.WithoutCancel(ctx)` with its own short timeout, matching the reasoning
already applied to the persist it has to beat.

## F3 (Low, latent) — `OnLinked` runs on the goroutine `Cancel` now waits for

`OnLinked` executes on the attempt goroutine (`link_manager.go:62-63, 279`), and
`Cancel` waits on that goroutine's `done`. Any future `OnLinked` implementation
that calls `Service.Unlink` or `LinkManager.Cancel` for the same teacher
deadlocks permanently — the wait has no timeout and `done` closes only when `run`
returns (`:246`). Unreachable today: `persistLink` touches only cipher, repo, and
cache. Worth one line on the `OnLinked` doc comment now that the hook is inside a
wait.

Otherwise (b) is sound: `Cancel` releases `m.mu` before `<-rec.done` (`:216-220`),
its only caller is `Service.Unlink` which holds no manager lock, `update` takes
`m.mu` for a pure mutation only, and `Close` remains idempotent (`baseCancel` is
idempotent, the map is emptied under the lock, and a record removed by a
concurrent `Cancel` is simply not in `Close`'s list — each waits on its own
`close(rec.done)`, which still happens exactly once). The wait's bound depends on
`LoginFunc` honouring ctx; production `LoginQR` does (every request uses
`http.NewRequestWithContext`, both long-poll loops check `ctx.Err()`, and the
client has a 60s timeout), so the practical bound is `persistTimeout`, i.e. up to
10s of handler block — under the 30s `WriteTimeout` in `server.go:24`.

## F4 (Low, informational) — the one URL path `doRequest` does not cover

`http.NewRequestWithContext` returns `url.Parse`'s `*url.Error`, whose `URL`
field is the full raw URL. `auth.go:232-235` and `:288-291` return that error
unwrapped, and those two URLs carry the plaintext IMEI and the ZCID. Unreachable
in practice — `makeURL` builds through `url.Values.Encode()` and the base URLs are
constants, so the parse cannot fail; if `makeURL` returned `""` the resulting
error carries no URL. Listed only because criterion (a) is stated as absolute.

## Everything else under (a) is clean

- `doRequest` (`protocol/client.go:102-113`) strips both the query string and
  `urlErr.URL`, keeping `urlErr.Err`. All six `Client.Do` sites route through it
  (`auth.go:238, 294, 320, 357, 467, 483`). `TestFetchers_TransportErrorOmitsCredentials`
  asserts the absence of the canary IMEI *and* of any `?`, and that the cause
  survives — the right three assertions.
- Returning `nil, err` from `doRequest` discards no live connection: `Client.Do`
  returns a non-nil response with an error only for a `CheckRedirect` failure, and
  that body is already closed.
- `health_probe.go:104-106` logs the error from `VerifyAccount`, which is
  `ErrLinkExpired` or a repo error — never the raw relogin error. The raw one at
  `service.go:227` is the one `doRequest` now scrubs.
- `secrets.Cipher` errors (`secrets.go:24-81`) and `protocol/crypto.go` errors
  carry neither plaintext nor ciphertext. `openCredentials`
  (`service.go:264-274`) deliberately discards the json error.
- `readJSON` failures (`auth.go:246, 305`) can echo a numeric literal from a
  malformed body but never a credential; unchanged from L-10.
- Zalo-authored error text from the QR long-polls (`auth.go:422, 453`) is logged
  but never returned to a client (`link_manager.go:295-309` substitutes
  `linkFailureMessage`).

## (d) — the idempotent DELETE breaks nothing

- Go: `ErrNotLinked` survives only at `service.go:210` (`sessionFor`) and
  `handler.go:156-157`. `sessionFor` is unexported and reached only by
  `VerifyAccount`, which the health probe calls — that path is untouched and still
  returns `ErrLinkExpired`/`ErrNotLinked` as before. The `linkError` branch for
  `ErrNotLinked` is now unreachable from any wired route; harmless and needed once
  a send endpoint exposes a session.
- Web: `zalo-api.ts:33-36` never inspected the status, and `use-zalo.ts:60-66` /
  `zalo-connect-card.tsx:50-60` only branch on success vs error. No msw handler
  mocks a 404 on `DELETE /me/zalo`. The change strictly removes an error toast the
  teacher could previously trigger by double-clicking.
- Contract: `phase-04-http-api-endpoints-and-wiring.md:31` specifies 204.
  `handler.go:128-134` and the regenerated `docs/swagger.{yaml,json}` +
  `docs.go` now match (a stale `@Failure 404 "nothing linked"` was present when
  this review began and was corrected at 23:26).

## (e) — verification, run against the tree at 23:29–23:40

| Gate | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go vet -tags=integration ./...` | pass |
| `go test ./...` | pass, all packages |
| `go test -tags=integration -count=1 ./...` | pass, every package, against real Postgres |
| `go test -tags=integration -count=1 -v -run TestZaloHTTPLinkLifecycleAgainstARealDatabase` | `--- PASS (2.93s)` — executed, not skipped |
| `CGO_ENABLED=1 go test -race -count=3 ./internal/features/zalo/... ./internal/server/...` | pass (`zalo` 4.2s, `protocol` 1.0s, `server` 1.1s) |
| `golangci-lint v2.7.2 run ./...` | `0 issues.` |

Note on the integration run: without `-count=1` every package reported
`(cached)`, i.e. it reused the earlier run's results rather than executing. The
row above is the uncached run (`attendance` 13.0s … `zalo` 22.2s … `seeds`
19.2s), so the pass is independently observed.

`integration_test.go:407-409` asserted `404` on the second DELETE when this review
started — it compiles either way, so `go vet -tags=integration` would not have
caught it and only a real integration run would. It was corrected at 23:27 and
now asserts `204`; the lifecycle test passes uncached.

## OpenAPI regeneration — verified

`handler.go:126-134` now carries `@Success 204`, `@Failure 401`, and no 404, with
"Idempotent: answers 204 whether or not an account was linked" in the
description. All three generated artifacts agree and none mentions a 404 for this
operation:

- `docs/swagger.yaml:3354-3374` — `delete:` with only `"204"` and `"401"`
- `docs/swagger.json:4014-4048` — same, description string matches the annotation
  verbatim
- `docs/docs.go:4021-4055` — same

`grep -rn "nothing linked"` over `apps/api/` and `apps/web/` returns nothing.
The docs diff is `800 insertions(+), 0 deletions(-)` across the three files and
every hunk belongs to the four new zalo routes and their schemas — the
regeneration introduced no churn in other features, and the stale 404 never
reached a committed artifact.

## No remaining expectation of a 404 from `DELETE /me/zalo`

Every site checked, current tree:

- Go tests: `integration_test.go:400-409` (both DELETEs → 204),
  `handler_test.go:354-365` (both → 204 with an empty body),
  `handler_test.go:179-183` (unauthenticated → 401),
  `handler_test.go:395` (credential-canary sweep, status not asserted).
- Web: `zalo-connect-card.test.tsx:64-68` mocks the DELETE as `204`; no msw
  handler anywhere returns a 404 for it. The only 404 mock in the profile tests
  is `zalo-link-modal.test.tsx:152`, which is the *link/status* poll — a
  different, still-correct contract.
- Plans/docs: `phase-04-http-api-endpoints-and-wiring.md:31` specifies 204 and
  `:148-161` records the correction; `plan.md:238` matches. The former line 133
  documenting a 404 is gone. The remaining 404 mentions live only in
  `plans/reports/zalo-phase-04-code-review.md`, a stateful record of the earlier
  review — correct to leave as history.

## (f) — clean

`grep -niE 'phase[ -]?0?[0-9]|C-1|H-2|M-3|M-4|L-6|260806'` over
`internal/features/zalo/` and `internal/shared/secrets/` returns nothing.

## L-6 confirmed

`.env.example:52` is `REPLACE_ME`. Every branch of `decodeTokenKey`
(`config.go:200-214`) yields at most 10 bytes, under `minZaloCredKeyLen`, so
`validateZalo` (`:252-259`) is fatal in production. In development it falls back
to a random per-process key with a fingerprint-only warning, which the surrounding
comment documents.

## Recommended actions

1. F1 — gate the write instead of extending the wait: pass the link id through
   `OnLinked` and re-check ownership under `m.mu` before `Upsert`. Blocking for
   the consent-revocation guarantee.
2. F2 — run `repo.Delete` on `context.WithoutCancel(ctx)` with its own timeout.
3. F3 — note on the `OnLinked` doc comment that it runs inside `Cancel`'s wait.
4. F4 — optional: wrap the two `NewRequestWithContext` errors in `auth.go` the
   same way `doRequest` wraps transport errors.

## Unresolved questions

- Should the health probe leave a durable audit trail when it expires an account?
  Still open from the previous review; unaffected by these fixes.

## Resolution

F1, F2, F3, and F4 are fixed; see the Disposition section of
`zalo-phase-04-code-review.md` for what changed and why.

F1's diagnosis was right and its recommended fix was right about the lever but
not about the wait being replaceable. `LinkManager` now tracks every attempt
whose goroutine is still running, so `Cancel` and `Close` wait on superseded and
swept records too, *and* `run` re-checks ownership before calling `OnLinked`. The
gate stops an abandoned attempt from writing at all; the wait covers the instant
between passing that gate and the write landing, which the gate alone cannot.
`TestUnlinkOutlastsASupersededScanThatLandsWhileItRuns` reproduces the exact
revival F1 described and is what proves it closed.
