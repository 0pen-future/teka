# Zalo Personal Auth — Phase 4 Backend Code Review

Scope: `apps/api/internal/features/zalo/` (incl. `protocol/`), `internal/shared/secrets/`,
`internal/config/config.go`, `internal/server/router.go`, `internal/app/{app,container}.go`,
`migrations/000004_zalo_accounts.*`. Read-only review; no source modified.

## Verdict

| # | Question | Answer |
|---|----------|--------|
| 1 | Credential material in a response, log, or error string? | **YES — logs only.** Plaintext IMEI reaches a log line on the re-login error path. No leak in any HTTP response. |
| 2 | IDOR across the five endpoints? | **NO.** Ownership is enforced in `LinkManager`; unknown / foreign / malformed / missing ids all return an identical `404 NOT_FOUND`. |
| 3 | Goroutine leak, deadlock, or panic-on-double-Close? | **NO panic, NO deadlock, NO unbounded leak.** `Close` is idempotent. Two bounded-lifetime caveats below (H-2, M-3). |
| 4 | Regressions or new lint/type/build errors? | **NO.** `go build`, `go vet` (with and without `-tags=integration`), `go test ./...`, `-race -count=3`, and `golangci-lint v2.7.2` all pass clean. |

Verification run: `go test ./...` all green; `-race -count=3` on `zalo`, `zalo/protocol`,
`server` clean; `golangci-lint run` → `0 issues.`

---

## C-1 (Critical, against the stated contract) — Plaintext IMEI is written to logs

`fetchServerInfo` puts the plaintext IMEI in the **query string**:

- `internal/features/zalo/protocol/auth.go:280` — `"imei": sess.IMEI` inside `siParams`
- `internal/features/zalo/protocol/auth.go:286` — `makeURL(...)` renders it into the URL
- `internal/features/zalo/protocol/auth.go:294-297` — `sess.Client.Do(req)`; on transport
  failure the returned `*url.Error` embeds the full URL, and the raw `err` is returned

That error is wrapped at `auth.go:90-92` (`"zalo_personal: server info: %w"`) and surfaces as the
`relogin` error, which is logged verbatim:

- `internal/features/zalo/service.go:224` — `s.log.Warn("zalo: stored session was rejected", "teacher_id", teacherID, "error", err)`
- `internal/features/zalo/health_probe.go:105` — same error again, per swept account

Reproduced with a `RoundTripper` that always fails, using the exact URL shape `auth.go:286` builds:

```
zalo_personal: server info: Get "https://wpa.chat.zalo.me/api/login/getServerInfo?client_version=1&computer_name=Web&imei=SECRET-IMEI-0000-1111&signkey=abc&type=30": dial tcp 1.2.3.4:443: connect: connection refused
```

Trigger is ordinary, not exotic: DNS failure, refused connection, TLS error, or the 60s client
timeout. The health probe sweeps every linked account every ~15 minutes, so a single network
outage prints **every** linked teacher's IMEI into the application log.

Honest impact calibration: the IMEI alone is not account takeover — the cookie jar is the other
half, and cookies travel in headers, never in a URL, so they do not leak here. But the phase
contract is written as an absolute ("plaintext IMEI/cookies must NEVER appear in any log line"),
and the IMEI is stable for the life of the link. This is a real violation, not an abstract one.

Suggested fix (protocol package, one helper): unwrap `*url.Error` before returning, keeping the
cause and dropping the URL.

```go
resp, err := sess.Client.Do(req)
if err != nil {
    var ue *url.Error
    if errors.As(err, &ue) {
        return nil, fmt.Errorf("zalo_personal: server info request failed: %w", ue.Err)
    }
    return nil, err
}
```

Apply it at `auth.go:238`, `:294`, `:320`, `:357`, `:467`, `:483` (only `:294` carries the IMEI
today, but `qrPost` and friends will carry whatever a later port puts in a URL). `fetchLoginInfo`'s
URL carries `zcid`, which is AES-CBC of `type,imei,ts` under the hardcoded `DefaultZCIDKey`
(`client.go:172-177`) — reversible by anyone holding the source, so worth scrubbing on the same pass.

## H-2 (High) — An in-flight link attempt can silently undo an unlink

`Unlink` cancels the attempt, evicts the session, then deletes the row
(`service.go:169-178`). But the persist step deliberately runs on an **uncancellable** context:

- `link_manager.go:267-269` — `persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)`
- `service.go:310-314` — `persistLink` then does `s.repo.Upsert(...)` followed by `s.cache.Put(...)`

If the teacher's scan completes in the window between `links.Cancel` and `repo.Delete`, the
attempt goroutine writes the row **after** the delete and repopulates the evicted cache. Net
effect: the teacher pressed "unlink", got `204` (or `404`), and the account is linked again with
credentials at rest. `LinkManager.update` is guarded by the active-map check
(`link_manager.go:304-311`), so the *polled state* stays consistent — only the database write is
unguarded.

Window is narrow (one DB round-trip) and requires the scan to land at that instant, so likelihood
is low; consequence is a defeated consent revocation, which is the one thing this feature must not
get wrong. Cheapest fix: have `persistLink` refuse to write when the attempt is no longer the
teacher's current one — e.g. pass the link id through `OnLinked` and re-check ownership under
`LinkManager.mu` immediately before `Upsert`, or make `Unlink` take the same lock the manager
uses so cancel-and-delete is atomic with respect to persist.

## M-3 (Medium) — `Close` does not await every goroutine it cancelled

`LinkManager.Close` collects only records still in `active` (`link_manager.go:221-232`). Records
that were superseded (`:177-180`), cancelled by `Unlink` (`:203-214`), or swept (`:315-322`) are
cancelled but removed from the map, so `Close` returns without waiting for them. They do exit
promptly — no unbounded leak — but shutdown is not the clean barrier the doc comment claims
("shutdown leaves nothing running", `:216-217`).

Conversely, a record that *is* in `active` and has just entered the persist step blocks `Close`
for up to `persistTimeout` (10s, `link_manager.go:283`), because that context ignores
cancellation. `Container.Close` runs `Zalo.Close()` before `database.Close` (`container.go`), so
the pool is still up — correct, but process exit can lag graceful HTTP shutdown by ~10s.

Neither is a defect on its own; the mismatch between the comment and the behavior is what I'd fix,
plus tracking superseded records in a `sync.WaitGroup` if the guarantee is meant to be real.

## M-4 (Medium) — `DELETE /me/zalo` contract mismatch with the brief and the web client

The review brief specifies `DELETE /me/zalo -> 204`. The implementation returns `404 NOT_FOUND`
when nothing is linked (`service.go:174-176` → `handler.go:157-158`), which the phase file does
document (`phase-04...md:133`) and which two tests assert
(`handler_test.go:361-363`, `integration_test.go:407-409`).

The web client does not special-case it — `apps/web/src/features/profile/api/zalo-api.ts:34-36`
just awaits `apiClient.delete("/me/zalo")` — so a double-click, a stale card, or an unlink issued
during an attempt that never persisted surfaces as an error toast. Either make DELETE idempotent
(204 regardless) or handle 404 as success on the client. Flagging for the lead to decide; I have
not changed either side.

## M-5 (Medium) — `Unlink` mutates local state before the durable write

`service.go:170-172` cancels the attempt and evicts the cache, then `:173` deletes. If `Delete`
returns a real DB error, the caller gets a `500` while the attempt is already dead and the cache
already cleared, with the row still present. Retrying is safe, so the blast radius is small, but
the ordering is backwards from the usual "persist first, then drop local state".

## Low

- **L-6** `.env.example:47` ships `API_ZALO_CRED_KEY=change-me-dev-only-zalo-credential-key-32-bytes`
  — 45 bytes, so it *passes* production validation (`config.go` `validateZalo`) if copied.
  `.env.production.example` correctly uses `REPLACE_ME` (too short → fatal). This mirrors the
  existing `API_JWT_SECRET` / `API_STATEMENTS_TOKEN_KEY` convention, so it is a pre-existing
  pattern rather than a Phase 4 regression, but this key protects account-takeover material.
  Consider a sub-32-byte dev placeholder so a copied file fails loudly in production.
- **L-7** `internal/cli/seed.go:29` calls `app.NewContainer`, which now requires a decodable
  credential key. `docker-compose.prod.yml` added `API_ZALO_CRED_KEY` to `migrate` and `api` but
  there is no seed service; anyone seeding a prod-like environment now needs the variable set.
- **L-8** No index on `zalo_accounts.status` and no partial index on `deleted_at IS NULL`;
  `ListLinked` (`repository.go:143-154`) seq-scans. One row per teacher makes this irrelevant now.
- **L-9** `Service.StartHealthProbe` (`service.go:92-108`) can restart the probe after `Close`
  (`:119-130` nils `probeDone`). Unreachable today — `RunServer` starts it once — but `Close` is
  not a terminal state.
- **L-10** `readJSON` parse failures are wrapped with `%w` (`auth.go:246`, `:305`), and Go's JSON
  errors can quote the offending input. Those bodies are Zalo's encrypted blobs or profile info,
  not credentials, so this is informational.

---

## Acceptance criteria — Phase 4

All six success criteria in `phase-04-http-api-endpoints-and-wiring.md:102-109` are met:

| Criterion | Evidence |
|---|---|
| Four endpoints behind auth; unauthenticated → 401 | `routes.go:11-15`; `handler_test.go:171-185` |
| Blank `consent_version` → 400, no attempt started | `service.go:136-141`, `handler.go:86-89`; `handler_test.go:240-259` asserts `{}`, `{"consent_version":""}`, and an empty body all 400 **and** that `links.active` stays empty |
| Unlinked teacher → `{linked:false}` at 200 | `service.go:151-158`; `handler_test.go:189-202` asserts the body is exactly `{"linked":false}` |
| Another teacher's `link_id` → 404 | `link_manager.go:194-198`; `handler_test.go:321-345`, `integration_test.go:372-374` |
| No response can carry credential material — asserted twice | `handler_test.go:368-405` (canary IMEI/cookie/secret-key never appear in real bodies, and no body even names `imei`/`cookie`/`secret`/`credential`) and `handler_test.go:409-429` (reflection over the three response structs) |
| Integration test against a real DB | `integration_test.go:320-410`, `//go:build integration`; `go vet -tags=integration ./...` clean |

Note the scope boundary: criterion 5 covers **responses**, and it holds. C-1 above is a *log*
leak, which the phase file's own overriding rule ("never logged", `model.go:1-5`) also covers.

## IDOR analysis (question 2, detail)

- Teacher identity comes only from `authctx.TeacherID` (`authctx.go:57-63`), which also requires
  `Role == RoleTeacher`; a parent/student token gets 401, not a mis-scoped 200. No endpoint accepts
  a teacher id from path, query, or body (`routes.go:11-15`).
- `LinkManager.Status` (`link_manager.go:189-199`) looks up by `teacherID` first and then requires
  `rec.id == linkID`, so a foreign id and an unknown id are the same `ErrLinkNotFound`.
- A malformed or missing `?id=` is converted to the same `apperror.NotFound("link attempt")`
  before the service is consulted (`handler.go:111-117`) — no parse-error oracle.
- `handler_test.go:334-344` exercises all four shapes (owner's real id from the intruder, a random
  uuid, `not-a-uuid`, and no `id` at all) and requires `404` + `NOT_FOUND` for every one.
- DB reads are keyed on `teacher_id` throughout (`repository.go:88-100`, `:129-141`); the table's
  primary key *is* `teacher_id`, so cross-tenant reads are structurally impossible.

## Lifecycle analysis (question 3, detail)

- `Service.Close` → `stopProbe` + `links.Close` (`service.go:112-115`). `stopProbe` swaps
  `probeStop`/`probeDone` to nil under a mutex (`:119-130`), so a second call is a no-op and a
  concurrent call cannot double-cancel. `LinkManager.Close` calls an idempotent `context.CancelFunc`
  and iterates an already-emptied map. Double `Close` is explicitly tested
  (`health_probe_test.go:192-203`).
- `close(rec.done)` happens exactly once, in `run`'s defer (`link_manager.go:236`).
- No lock is held across a channel receive: `Close` releases `m.mu` before `<-rec.done`
  (`:225-231`); `update` only takes `m.mu` around a pure mutation (`:304-311`).
- `Begin` after `Close` is safe: `baseCtx` is already cancelled, so the attempt fails fast to
  `expired` and `update` no-ops against the cleared map.
- Context cancellation reaches the network — every protocol call uses
  `http.NewRequestWithContext` (`auth.go:232,288,313,351,460,476`) and the client has a 60s
  timeout (`client.go:47-56`) — so the probe cannot hang shutdown indefinitely.
- Race detector clean over three runs of the `zalo` and `server` packages.

## Repo-pattern conformance

Layering matches `notifications` exactly (`handler.go` + `dto.go` + `routes.go` + `service.go` +
`repository.go`, mounted from `registerFeatures`). Errors go through `apperror` and the shared
`response.Envelope`; `apperror.From` wraps unknown errors as `Internal` with a fixed message
(`apperror.go:81-96`), so the `default:` branch of `linkError` (`handler.go:161-162`) cannot leak
an internal error string to a client. `zalo` being built in `container.go` rather than
`router.go` is a justified deviation (it owns goroutines the router has no lifecycle for) and is
documented at the `NewRouter` signature (`router.go:36-39`) and on the `Container.Zalo` field.

Grep for plan ids, phase numbers, and audit labels in `internal/features/zalo/` and
`internal/shared/secrets/` returns nothing — repo rule satisfied. `golangci.yml` gains two
narrowly-scoped exclusions (`G401|G501` and `revive:exported`) confined to
`internal/features/zalo/protocol/`, justified by the reverse-engineered wire format; not blanket
suppression.

## Recommended actions

1. Fix C-1 — strip `*url.Error` before returning transport failures in `protocol/auth.go`. Blocking.
2. Fix H-2 — re-check attempt ownership inside `persistLink` before `Upsert`, so `Unlink` wins.
3. Decide M-4 — idempotent `204` on unlink, or handle `404` in `zalo-api.ts`. Pick one and align both sides.
4. M-3 / M-5 — either make `Close`'s guarantee real or soften the comment; move `Unlink`'s local
   state drop after the successful delete.
5. L-6 — shorten the `.env.example` placeholder so it cannot pass production validation.

## Unresolved questions

- Is the `404`-on-nothing-linked the intended `DELETE` contract, or was the brief's `204`
  authoritative? Two tests currently pin the `404`, so this needs a decision, not a silent change.
- Should the health probe leave a durable audit trail when it expires an account? Today the only
  record is a log line and the flipped `status` column.

---

## Disposition

Applied, each with a test written first and confirmed to fail against the
pre-fix code.

**C-1 — fixed.** Rather than patching the six call sites individually, the
request itself now goes through one helper, `doRequest` in
`protocol/client.go`. It unwraps `*url.Error` and rebuilds the message from the
scheme, host, and path only, so the query string — which carried the plaintext
IMEI and the ZCID beside it — never reaches the error text, while the
underlying cause is still wrapped with `%w` for both `errors.Is` and the log.
Every `sess.Client.Do` in `auth.go` routes through it; the only remaining raw
call is the one inside the helper. `TestFetchers_TransportErrorOmitsCredentials`
drives `fetchServerInfo` and `fetchLoginInfo` through a failing transport and
asserts a canary IMEI is absent, no `?` survives, and the cause does. Reverted
against the old code it reproduces the review's leak verbatim, including the
`zcid` in `getLoginInfo`.

**H-2 — fixed, and then fixed properly.** The first attempt made `Cancel` wait
on `<-rec.done` so `Unlink`'s `repo.Delete` could not race an in-flight persist,
and rejected the review's suggested ownership re-check as non-atomic. That
rejection was wrong in a way the wait made worse rather than better, and the
follow-up review (`zalo-phase-04-fix-review.md`, F1) demonstrated it: `Cancel`
waited only on the record still in `active`, so an attempt the teacher had
superseded by reopening the modal was cancelled, dropped, and never waited for.
Its persist could still land after the delete, and because `update` no-ops for a
superseded record, nothing anywhere would show it — the profile card would read
"chưa kết nối" over a row holding usable credentials.

Neither mechanism is sufficient alone, so the fix uses both. `LinkManager` now
keeps a `live` set of every attempt whose goroutine is still running, not just
the one it currently names; `Cancel` cancels and waits on all of them, and
`Close` does the same across teachers. Independently, `run` re-checks
`isCurrent` before calling `OnLinked`, so a scan for an abandoned attempt is not
stored at all — that also stops a superseded attempt from overwriting the
credentials of the one that replaced it. The ownership gate covers everything
the manager has let go of; the wait covers the window between that gate and the
write, which is exactly the non-atomicity the first disposition described. That
window is now the only place the gate is not decisive, and no record can be in
it without being in `live`.

`TestUnlinkOutlastsAScanThatLandsWhileItRuns` holds a login at the moment of
approval, releases it 50ms into an unlink, and asserts with `require.Never` that
the row does not come back.
`TestUnlinkOutlastsASupersededScanThatLandsWhileItRuns` does the same after a
second `StartLink` has superseded the scanning attempt; it fails against the
wait-only code with "Condition satisfied" — the revival the follow-up review
predicted — and passes with the gate in place.

**F2 — fixed.** `repo.Delete` ran on the request context, which the new wait can
outlive: a teacher who closed the tab during it would have left the just-written
row in place. The delete now runs on `context.WithoutCancel` with its own
`unlinkTimeout`.

**F3 — fixed.** The `OnLinked` doc comment now states that it is called only
while the attempt is current and that `Cancel` blocks until it returns, so a
future implementation does not put slow work behind an unlink.

**F4 — fixed.** `http.NewRequestWithContext` quotes the whole URL in its error,
including the query string that carries the IMEI. All six construction sites in
`auth.go` now go through a `newRequest` helper that reports the method and the
parse cause only. The path is unreachable today — the URLs are built in-package
— which is the reason to close it in the helper rather than trust every future
call site to notice.

**M-4 — decided as 204.** The accepted phase-04 plan specifies `204` for
`DELETE /me/zalo` and documents no not-found case, so the `404` was an
implementation deviation that the tests had been written to match, not a
recorded decision being reversed. `Service.Unlink` now returns nil when there is
no row, and the two tests were updated. This also removes the false error toast
a double-click produced, so no client change was needed; `ErrNotLinked` is
untouched on the `sessionFor` path.

**M-3 — fixed.** The comment was first corrected to admit that `Close` waits
only for tracked attempts, on the grounds that a second registry for records the
manager had let go of "buys nothing at shutdown". The F1 fix needed that
registry anyway for `Cancel`, so `Close` now uses it too and the original
guarantee — nothing still running when shutdown returns — is real.

**M-5 — not changed, deliberately.** `Unlink` evicts the cached session before
the durable delete. That order fails closed: if the delete fails, the row
survives but the cached session is gone, costing a re-login and nothing more.
Reversing it would leave a live cached session behind a failed delete.

**L-6 — fixed.** The `.env.example` placeholder is now `REPLACE_ME`, too short
to satisfy production validation, with a comment saying why. The identical
pattern on `API_JWT_SECRET` and `API_STATEMENTS_TOKEN_KEY` is left alone: those
belong to other features and changing them here would widen this change beyond
its scope.

**L-7, L-8, L-9, L-10 — deferred, informational.** No seed service exists to
break yet; `ListLinked` scans one row per teacher; `StartHealthProbe` after
`Close` is unreachable from `RunServer`; the bodies `readJSON` may quote are
Zalo's encrypted blobs, not credentials.

Verification after the fixes: `go build`, `go vet` with and without
`-tags=integration`, the full `go test ./...`, the integration-tagged suite
against a real Postgres, `go test -race -count=3` on the zalo packages, and
`golangci-lint` — all clean. The four `gosec` findings the latest linter reports
(`G101`, `G115`, `G124`) are in files this work never touched and predate it;
CI's pinned version does not raise them.

### Found while verifying the fixes

The review counted two tests pinning the `404` on unlink. There were three: the
Postgres-backed `TestZaloHTTPLinkLifecycleAgainstARealDatabase` also asserted
it. It surfaced only once the integration-tagged suite was actually run, which
needs the Docker socket mounted into the toolchain container — without that the
tests fail while provisioning Postgres, which reads as an environment error
rather than a real result. The whole integration suite now passes against a real
database, every feature package included, and the OpenAPI spec was regenerated
so the published contract no longer advertises the removed `404`.
