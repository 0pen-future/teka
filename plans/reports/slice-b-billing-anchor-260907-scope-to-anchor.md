# Slice B report: billing, attendance, sessions, enrollments — Scope→Anchor

Phase 2 (row-anchor type) of `plans/260906-0627-authz-write-scope-root-cause/`,
slice B. Scope: `internal/features/{billing,attendance,sessions,enrollments}`.
`internal/app` needed no edits (confirmed in an earlier segment — no wiring
depends on the changed signatures).

## Status: DONE

## What changed

Repository/service methods in `billing` whose caller always supplies a
row-derived teacher (never the acting caller's own scope) now take
`authctx.Anchor` instead of a hand-built `authctx.Scope{...}` literal, per the
plan's migration rule table. `attendance.TallyByEnrollment` and
`enrollments.ActiveOnClass` (center-keyed, teacher-irrelevant roster read)
were already converted in earlier segments of this same task; this segment's
own work was:

1. Finishing propagation of the (already-decided, already-committed)
   `billing/repository.go` Anchor conversion into the three test-fake files
   that had fallen out of sync with it (discovered via `go vet`, not a design
   decision of mine — see Errors below).
2. Fixing ~20 leftover `authctx.Scope` call sites in
   `internal/features/billing/integration_test.go` and one in
   `secretary_integration_test.go`, all `repo.ListInvoices` /
   `repo.GetInvoiceWithLines` / `repo.GetPeriodByYearMonth` /
   `repo.TallyAttendance` calls that needed `sc.Self()` (verified per call
   site that the scope's own teacher was in fact the target row's teacher —
   true in every case in this file, including the `fx.scope`/`fx.period`
   fixtures from `seedDraftFixture`/`seedClosedJanuaryFixture`, both
   single-teacher fixtures).
3. Writing the two mandatory TDD tests required by the phase's Success
   Criteria and Execution Notes (not present before this segment).
4. Full verification sweep.

## Files modified this segment

- `internal/features/billing/integration_test.go` — ~20 call-site fixes
  (`sc`/`fx.scope` → `.Self()`); appended `newReconciliationDeps` helper and
  `TestAssistantConfirmOnClosedPeriodPostsAdjustmentOnClassTeachersBilling`
  (new TDD test, ~65 lines).
- `internal/features/billing/secretary_integration_test.go` — 1 call-site fix.
- `internal/features/enrollments/staff_read_integration_test.go` — appended
  `TestActiveOnClassIsCenterKeyedAcrossHandoffAndNeverCrossesCenters` (new TDD
  test, ~50 lines).

(Files edited in earlier segments of this same task, already covered in prior
reporting and re-verified clean here: `internal/features/billing/{repository,
adjustment,close,service,preview,service_test,preview_test,close_test}.go`,
`internal/features/enrollments/service_test.go`.)

## TDD tests (mandatory, both new, both passing)

1. `TestAssistantConfirmOnClosedPeriodPostsAdjustmentOnClassTeachersBilling`
   (billing package, integration): a tro_giang assistant with no billing
   permission and no giao_vien stint confirms a session dated inside an
   already-closed period. Asserts `Confirm` returns no warning (the
   reconciliation actually ran, not silently failed) and the resulting
   `invoice_adjustments` row's `teacher_id`/`center_id` belong to the class
   teacher, never the confirming assistant.
2. `TestActiveOnClassIsCenterKeyedAcrossHandoffAndNeverCrossesCenters`
   (enrollments package, integration): after a class handoff (old teacher's
   stint ended, new teacher's stint started), a bystander caller holding no
   stint on the class at all still resolves the full roster via
   `ActiveOnClass` (center-keyed, not stint-keyed); a caller scoped to a
   different center resolves an empty roster for the same class id.

Both use a local dependency-wiring helper local to their own test file
(`newReconciliationDeps` in billing) rather than touching the widely-shared
`newIntegrationDeps`/`newIntegrationService` signatures, since neither of
those existing helpers wires `attendance.SetReconciler`, and threading a 6th
return value through every existing call site was unnecessary churn outside
this task's scope.

## Verification (commands run, in order)

```
cd /home/cesc/Documents/personal-workspace/teka/apps/api
gofmt -l ./internal/features/billing ./internal/features/attendance ./internal/features/sessions ./internal/features/enrollments ./internal/app
go build ./...
go vet ./internal/features/billing/... ./internal/features/attendance/... ./internal/features/sessions/... ./internal/features/enrollments/... ./internal/app/...
go vet -tags=integration ./internal/features/billing/...
go vet -tags=integration ./internal/features/attendance/...
go vet -tags=integration ./internal/features/sessions/...
go vet -tags=integration ./internal/features/enrollments/...
go test -count=1 ./internal/features/billing/... ./internal/features/attendance/... ./internal/features/sessions/... ./internal/features/enrollments/... ./internal/app/...
go test -tags=integration -p 1 -count=1 ./internal/features/billing/
go test -tags=integration -p 1 -count=1 ./internal/features/attendance/
go test -tags=integration -p 1 -count=1 ./internal/features/sessions/
go test -tags=integration -p 1 -count=1 ./internal/features/enrollments/
```

All green:

- `gofmt -l`: no output.
- `go build ./...`: clean.
- `go vet` (no tags, all four packages + `internal/app`): clean.
- `go vet -tags=integration` per package: clean (this surfaced all the
  test-fake/call-site mismatches fixed in this segment).
- Unit tests (`billing`, `attendance`, `sessions`, `enrollments`): `ok`.
- Integration tests, one package at a time with `-p 1 -count=1`: all `ok`
  (billing 10.8s, attendance 8.2s, sessions 8.5s, enrollments 7.0s). No test
  skipped, no expectation weakened.

Final sanity grep (must print nothing outside the standard
`return authctx.Scope{}, false` zero-value error path every `handler.go`
already used before this task — verified, not a migration target: it carries
no fields, matches the plan's explicit allowlist for handler zero values):

```
grep -n 'authctx.Scope{' internal/features/billing/*.go internal/features/attendance/*.go internal/features/sessions/*.go internal/features/enrollments/*.go | grep -v _test.go
```

Output: the four `handler.go` zero-value lines only. No caller-derived
`authctx.Scope{TeacherID: ..., CenterID: ...}` literal remains in any of the
four owned packages outside tests.

No Docker containers were started or stopped by me beyond testcontainers'
own throwaway postgres/ryuk pairs per `go test` run, each torn down
automatically at test completion (confirmed via `docker ps` after the run:
only pre-existing long-running project containers remain, `teka-db`/
`teka-api-1`/`teka-web-1` untouched).

## Errors and fixes (this segment)

- `go vet` (no tags) initially failed on `enrollments` (`*fakeRepository`
  missing `ActiveOnClass`) and on `billing` (`*closeFakeRepository` wrong
  type for `AdjustmentTotals`) — carried over from earlier segments of this
  same task, not introduced here. Root cause: a comprehensive ~20-method
  Scope→Anchor conversion had already landed in `billing/repository.go` in
  an earlier segment but had not been propagated into the three test-fake
  files (`service_test.go`, `preview_test.go`, `close_test.go`). Fixed by
  diffing the real interface against each fake exhaustively.
- `go vet -tags=integration ./internal/features/billing/...` (only run after
  the no-tag vet was clean) surfaced ~21 more mismatches confined to the
  three `//go:build integration` files, invisible to a no-tag vet since they
  live in the external `billing_test` package. Fixed one file/one batch at a
  time, re-running the tagged vet after each batch until clean.
- My first attempt at the mandatory adjustment test set both sessions before
  `Close`, one of them still unconfirmed at close time — `Close` legitimately
  refuses with "period has unconfirmed sessions" for any past-due unconfirmed
  session in the period, not just an empty class. Fixed by moving the second
  session's creation to after `Close` succeeds (also a more realistic
  narrative: the assistant discovers and confirms a session that was entered
  into the system only after the period closed).

## Concerns / open items

None blocking. Two minor observations for the lead, not requiring action from
me:

- The `handler.go` zero-value `authctx.Scope{}, false` returns are identical
  across all four (and, per the earlier grep in this task, likely all other)
  features — this is pre-existing, not something this task introduced, and
  matches the plan's explicit guard allowlist for the auth-extraction failure
  path.
- I did not touch `internal/features/billing/class_periods_integration_test.go`
  — grepped twice (before and after all other fixes) for any
  `repo.`/`authctx.Scope`/`authctx.Anchor` call-site mismatch; none found.

Status: DONE
Summary: All four owned packages (billing, attendance, sessions, enrollments) compile clean with and without the integration tag, both mandatory TDD tests pass, and all four integration test packages pass one at a time with -p 1. Fixed the remaining ~21 Scope→Anchor call-site mismatches in billing's three integration test files plus test-fake drift from an earlier segment's repository.go conversion, then added the tro_giang-closed-period-adjustment and ActiveOnClass-center-keyed-handoff tests the phase's Success Criteria require.
Concerns/Blockers: none.
