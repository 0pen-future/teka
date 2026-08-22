# Billing package center-tenancy sweep

## Files touched

Billing package (source already center-keyed from prior session; this session's own work was the test layer):
- `apps/api/internal/features/billing/integration_test.go` — full rewrite (~1600 lines): 3 new authorization tests added first, all ~30 existing tests mechanically converted to `authctx.Scope`, `seededDraftFixture`/`seededClosedJanuaryFixture` gained a `scope` field, 3 raw-struct-literal sites fixed, `insertOpenPeriod` re-keyed.
- `apps/api/internal/features/billing/{service,preview,close}_test.go` — unit-test fakes rewritten to `authctx.Scope` (verified compiling and green; done in prior session, re-verified this session).
- `apps/api/internal/features/billing/{model,repository,service,close,preview,adjustment,handler,routes}.go` — already correct from prior session, re-verified unchanged.

Out-of-package mechanical ripple (test files only, billing call-sites only):
- `apps/api/internal/features/payments/integration_test.go`
- `apps/api/internal/features/collections/integration_test.go`
- `apps/api/internal/features/statements/integration_test.go`
- `apps/api/internal/features/statements/public_integration_test.go`
- `apps/api/internal/features/notifications/integration_test.go`
- `apps/api/internal/features/notifications/personal_send_integration_test.go` — not in the original 5-file list; its `closedPeriodWithContacts` helper failed to compile against billing's new signatures (shared by `run_integration_test.go`/`run_resume_integration_test.go`), so fixing it was required just to make the package build. Converted mechanically.

## TDD evidence

3 authorization tests written first: `TestOwnerHasFullOversightOfMembersBillingPeriods`, `TestPeersInSameCenterCannotSeeEachOthersBillingPeriods`, `TestCrossCenterBillingIsNotFound`. Repository-layer center scoping was already implemented in a prior session (not by me this session), so true pre-implementation red wasn't reproducible by writing the tests alone — they passed immediately. Correctness of the enforcement was instead proven by the sabotage check below, which is the honest substitute for red-before-green here.

## Sabotage check

Edited `repository.go`'s `scoped()`:
```go
if !sc.IsOwner {          // before
if true {                 // sabotaged
```
Ran the 3 authorization tests: `TestOwnerHasFullOversightOfMembersBillingPeriods` FAILED (owner could no longer read/close a member's period once the teacher_id filter always applied — proving the test actually exercises the owner-bypass path). The other two tests, which don't depend on owner bypass, still passed as expected. Reverted the edit; `grep -n "if true" repository.go` returns nothing; full suite re-confirmed green.

## Test results

- `go vet -tags integration ./internal/features/billing/...` — clean.
- `go test -tags integration ./internal/features/billing/ -count=1` — all tests pass, including the 3 new authorization tests.
- `gofmt -l internal/ seeds/ && go build ./... && go vet -tags integration ./...` (whole api module) — clean, zero errors.
- 10-package regression gate (billing, attendance, sessions, enrollments, classes, students, contacts, teachers, centers, auth) — all `ok`.

## Out-of-package ripple: what's green vs. blocked

The mechanical conversion (raw `teacherID uuid.UUID` → `testutil.ScopeFor(t, db, teacherID)` at billing call sites) fixed all compile errors across payments/collections/statements/notifications. However `go test` on those 4 packages still fails at runtime with `null value in column "center_id"` on the `payments`, `statements`, and (via one raw SQL `INSERT` in `notifications/run_integration_test.go`) `billing_periods` tables.

This is **not caused by this sweep**. Verified: `payments.Service.Record`/`List` and `statements.Service.Generate` still take a raw `teacherID uuid.UUID`, and `payments/model.go`/`statements/model.go` have no `CenterID` field at all — those packages' own center-tenancy sweep hasn't started. The schema (Phase 1 migration) already added `center_id NOT NULL` to their tables; their service/model layers haven't caught up. This is out of billing's file ownership and requires payments/statements/notifications' own re-key work (a separate task), not further billing changes.

## Raw-SQL site hand-inspected

`TestVoidInvoiceExcludedFromContactBalanceView` queries `v_contact_balance` filtered on `teacher_id = fx.teacher.ID` only. Checked the view definition (`docs/schema_design.sql:658-670`): it already exposes `center_id` as a column (added by the Phase 1 migration) but the test's predicate is scoped to one already-known fixture row (one teacher, one period, one contact) — no cross-tenant ambiguity, so leaving it keyed on `teacher_id` alone is correct; no `center_id` addition needed.

## Shim sites removed (from prior session's work, re-verified this session)

Confirmed via grep that no billing DTO accepts `teacher_id`/`center_id`, and the only place a raw `teacherID uuid.UUID` still appears in `service.go` is the private `teacherLocation` helper, called exclusively with `sc.TeacherID` (create-assigns-self derived scope, not a tenancy check) — consistent with the already-approved derived-scope carve-out.

Status: DONE
Summary: Billing package integration test suite rewritten with 3 new owner/peer/cross-center authorization tests plus mechanical conversion of all ~30 existing tests; full billing suite green; sabotage check confirms the owner-oversight path is actually enforced by the tests. Out-of-package test ripple (5 named files plus 1 additional file needed to keep notifications compiling) mechanically converted; billing call sites are correct everywhere, but payments/collections/statements/notifications packages still fail at runtime due to their own pending center-tenancy sweeps (unrelated to billing, their tables already require center_id but their services don't stamp it yet).
Concerns/Blockers: payments, statements, and notifications packages need their own center-tenancy sweep (model CenterID field + service signature change to authctx.Scope) before their integration suites will pass; notifications/run_integration_test.go and run_resume_integration_test.go also contain a raw SQL INSERT into billing_periods missing center_id, which will need a one-line fix once that package is swept. None of this blocks the billing package itself, which is fully green and self-contained.
