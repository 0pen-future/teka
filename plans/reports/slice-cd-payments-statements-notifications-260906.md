# Slice C+D: authctx.Anchor migration — payments, statements, notifications

Status: DONE_WITH_CONCERNS

## Scope

Replace hand-built `authctx.Scope{...}` literals that describe "whose rows are
being acted on" with `authctx.Anchor` across `internal/features/payments`,
`internal/features/statements`, `internal/features/notifications`, following
the signature-follows-callers rule from the assignment. `authctx.go` and the
AST guard test (`internal/features/scoping_guard_test.go`) were not touched.

## payments

All three assigned sites converted in an earlier part of this session:
- `reversal.go:94,212,301` (`paymentScope` → `sc.AnchorTo(...)`).
- `repository.go:421` (`ResolveContactScope`).
- Added the TDD pin: a member holding only `payments.create` (no
  `payments.view_all`) records a payment for a contact anchored to the owner;
  the `Record` response carries non-empty allocations via the new
  `AllocationsOf` helper.

Verified this session: `gofmt -l`, `go build`, `go vet` clean (no tag). The
package's own integration build (`-tags=integration`) is currently blocked —
see Concerns.

## statements

Repository interface converted to Anchor for every period-scoped write/read
that never owner-bypasses:
`UpsertStatement`, `InvoicesWithLines`, `LiveSessions`, `PeriodInvoiceLines`,
`PeriodClassInvoiceLines`, `Adjustments`, `TouchView`, `ContactTotals`,
`ContactClassTotals`. `TargetContacts`/`TargetContactsClass` kept the
exemplar (Anchor, Scope) shape: rows come from the anchor, `phone_visible` is
judged for the viewer.

Left on Scope, unchanged: `GetPeriodStatus*`, `ClassSendAccess`, `ListByPeriod`,
`ListByPeriodClass`, `GetByID`, `GetByTokenHash`, `Revoke` — these branch on
`sc.ReportsOversight()`/caller identity, per the assignment's explicit
instruction not to touch that axis.

Call sites in `service.go` (`GenerateForSendClass`, `generate`, `RenderPublic`,
`Service.TouchView`, `PeriodFigures`, `PeriodFiguresClass`) and
`public_handler.go` (`touchView`) updated: `RenderPublic`/`touchView` build a
bare `authctx.Anchor{...}` literal (no caller scope exists — public token
path); every other site derives via `sc.AnchorTo(info.TeacherID)`.

Test fixes:
- `service_test.go`'s `fakeRepository` — every affected method retyped to
  Anchor; `TouchView`'s ownership check simplified to `s.TeacherID != a.TeacherID`.
- `phone_privacy_integration_test.go`'s `TestTargetContactsRowsByAnchorPhoneByViewer`
  (the repo-level pin test named in the assignment) — its four `TargetContacts`
  calls updated to pass `memberScope.Self()` / `ownerScope.Self()` as the
  anchor argument; assertions unchanged.

Doc comments updated in `repository.go`, `service.go`, `model.go` to say
"anchor"/"periodAnchor" where they previously said "scope"/"periodScope".

Verified: `gofmt -l`, `go build`, `go vet` clean (no tag). Grepped every other
statements `*_test.go` for direct calls to the nine converted repo methods —
only `phone_privacy_integration_test.go` calls them directly, now fixed.

## notifications

Design per assignment: rename the repo's own-scoped helper and convert the
four RunStore write/read methods plus the run-manager's background-job anchor.

- `repository.go`: `runsOwnScoped(ctx, sc authctx.Scope)` renamed to
  `anchored(ctx, a authctx.Anchor)`, body now binds `a.CenterID, a.TeacherID`.
  `MarkOutcome`, `FailQueuedInRun`, `UpdateRunStatus`, `RunCounts` retyped to
  take `authctx.Anchor`. `QueuedRunRows` also converted (same raw-SQL
  own-scoped shape as `RunCounts`, matching the assignment's "run snapshot
  queries" wording) — its sole caller in `ResumeRun` already resolves `run`
  via `LatestOwnRunByPeriod`, which never returns a run belonging to anyone
  but the caller, so passing `sc.Self()` there is exact, not a widening.
  `HasActiveRun` and `LatestOwnRunByPeriod` were **not** named in the
  assignment and stay `authctx.Scope`-typed (they are literal caller-identity
  checks — "does the caller have/own a run" — not "whose rows"); internally
  they now call `r.anchored(ctx, sc.Self())`. `MarkSent`/`writeScoped` left
  untouched as instructed (another phase's `WriteWide()` axis).
- `service.go`: the four named call sites converted —
  `service.go:222` `staleScope` literal → `s.repo.FailQueuedInRun(ctx, sc.AnchorTo(stale.TeacherID), ...)`;
  `service.go:634` `runScope` literal → `s.repo.RunCounts(ctx, sc.AnchorTo(run.TeacherID), run.ID)`;
  `service.go:736` (`QueuedRunRows`), `:761` (`MarkOutcome`), `:794`
  (`UpdateRunStatus`) inside `ResumeRun` → `sc.Self()` at each, since `run`
  there is already proven the caller's own.
- `run_manager.go`: `RunStore` interface retyped (`MarkOutcome`,
  `FailQueuedInRun`, `UpdateRunStatus` now take `authctx.Anchor`); `runJob.scope()`
  renamed to `runJob.anchor()` returning an `authctx.Anchor{...}` literal
  directly — the background sender goroutine has no caller scope in hand.
  All six internal call sites (`markOutcome`, `revokeRun` ×2, `expireRun` ×2,
  `finishRun`) updated to `job.anchor()`.

Test fixes:
- `run_manager_test.go`'s `fakeRunStore` — `MarkOutcome`/`FailQueuedInRun`/
  `UpdateRunStatus` retyped to `authctx.Anchor`.
- `run_integration_test.go` (repo-direct integration test) — added a
  `statementFixture.anchor()` helper alongside the existing `.scope()`;
  every direct call to `MarkOutcome`, `UpdateRunStatus`, `FailQueuedInRun`,
  `QueuedRunRows`, `RunCounts` switched to `.anchor()`; `HasActiveRun`,
  `LatestRunByPeriod`, `ListByPeriod` calls left on `.scope()` (unaffected
  methods).

Verified: `gofmt -l`, `go build`, `go vet` clean (no tag). Grepped every
notifications `*_test.go` for direct calls to the five converted repo
methods — only `run_integration_test.go` and `run_manager_test.go` call them
directly, both fixed.

## Combined verification (no integration tag)

```
gofmt -l ./internal/features/payments ./internal/features/statements ./internal/features/notifications
  → clean
go build ./internal/features/payments/... ./internal/features/statements/... ./internal/features/notifications/...
  → clean
go vet ./internal/features/payments/... ./internal/features/statements/... ./internal/features/notifications/...
  → clean
```

Sanity grep (must show only the zero-value handler-return exception):
```
grep -n 'authctx.Scope{' internal/features/payments/*.go internal/features/statements/*.go internal/features/notifications/*.go | grep -v _test.go
```
Result:
```
internal/features/payments/handler.go:39:      return authctx.Scope{}, false
internal/features/statements/handler.go:39:    return authctx.Scope{}, false
internal/features/notifications/handler.go:39:    return authctx.Scope{}, false
```
All three are the allowed zero-value literal, confirmed for notifications as
well as the two previously confirmed packages.

## Concerns / Blockers

**The three mandated `-tags=integration` test commands could not be run.**
`internal/features/billing` — a package outside my ownership, actively being
modified (uncommitted) by a different, parallel session as part of the same
larger `authctx.Anchor` migration — currently fails to compile even in its
plain (non-test) form. Since `payments`, `statements`, and `notifications`
integration tests all transitively depend on billing for test setup
(`EnsurePeriod`/`Close`), `go vet -tags=integration` on **any** of the three
fails at the billing compile step, not from anything in this slice. This was
first observed as a statements-only blocker earlier in the session; by the
time notifications was finished, the same billing compile failure (with
shifted line numbers/messages, confirming active edits elsewhere) now also
blocks payments — whose integration suite had been green earlier in this
session, before billing's breakage widened.

This means:
- Non-integration build/vet/gofmt: clean for all three packages, confirmed
  above.
- `go test -tags=integration -p 1 -count=1 ./internal/features/payments/`,
  `.../statements/`, `.../notifications/`: **not run**, blocked by billing.
- Every repo-direct test call site in all three packages has been audited and
  updated to the new Anchor signatures (see per-package sections), so once
  billing compiles again these three commands are expected to pass without
  further changes here — but that expectation is unverified.

No other packages were touched. No wiring changes were needed in `internal/app`
(grepped `container.go` for the ten converted method names — no hits; it only
constructs top-level `Service`s via their unchanged public signatures).

## Files Modified

- `internal/features/payments/{repository,service,reversal,service_test,reversal_test,integration_test}.go`
- `internal/features/statements/{repository,service,public_handler,model,service_test,phone_privacy_integration_test}.go`
- `internal/features/notifications/{repository,service,run_manager,run_manager_test,run_integration_test}.go`

## Status block

Status: DONE_WITH_CONCERNS
Summary: All nine assigned call sites across payments/statements/notifications converted to Anchor per the signature-follows-callers rule; every repo-direct test updated; non-integration build/vet/gofmt clean for all three packages.
Concerns/Blockers: The three `-tags=integration` test runs (payments, statements, notifications) could not execute — `internal/features/billing`, owned by a parallel in-flight session, currently fails to compile, and all three packages' integration suites transitively depend on it for test setup. This is an external blocker, not introduced by this slice; recommend re-running the three integration commands once billing compiles.

> Lead follow-up (2026-09-07): once billing compiled, the lead ran the three integration packages (`payments`, `statements`, `notifications`) with `-tags=integration -p 1 -count=1`; all three `ok`. Blocker cleared.
