# statements package: teacher-tenancy to center-tenancy re-key

## Files modified

- `apps/api/internal/features/statements/model.go` — added `CenterID uuid.UUID` to `Statement`.
- `apps/api/internal/features/statements/repository.go` — every `Repository` method except `GetByTokenHash` now takes `sc authctx.Scope`; `scoped()`/`withContact()` filter `center_id` always, `teacher_id` only when `!sc.IsOwner`; `GetPeriodStatus` returns `PeriodInfo{Status, TeacherID}`; every raw-SQL JOIN switched from `teacher_id` to `center_id` matching; `UpsertStatement` stamps `TeacherID`/`CenterID` from `sc`.
- `apps/api/internal/features/statements/service.go` — `Generate` authorizes via `GetPeriodStatus(ctx, sc, periodID)`, derives `periodScope := authctx.Scope{TeacherID: info.TeacherID, CenterID: sc.CenterID}`, uses it for `TargetContacts`/`ContactTotals`/`UpsertStatement`; `PeriodFigures` gained the same authorize-then-derive step (this authorization step did not exist before — a real gap, not just a signature change); `RenderPublic`/`TouchView` derive scope from the loaded `Statement` row's own `TeacherID`/`CenterID`, never the caller's.
- `apps/api/internal/features/statements/handler.go` — replaced `authctx.TeacherID(c)` with `h.scope(c)` returning `authctx.Scope` via `authctx.ScopeFrom(c)`.
- `apps/api/internal/features/statements/public_handler.go` — `touchView` derives `authctx.Scope{TeacherID: stmt.TeacherID, CenterID: stmt.CenterID}` from the resolved statement row.
- `apps/api/internal/features/statements/routes.go` — `RegisterRoutes` takes `resolveScope gin.HandlerFunc`, applied to both authenticated route groups; `RegisterPublicRoutes` unchanged.
- `apps/api/internal/server/router.go:163` — `statements.RegisterRoutes(v1, statements.NewHandler(statementsSvc), requireAuth, resolveScope)`.
- `apps/api/internal/features/notifications/service.go` — `StatementsSource.Generate`/`PeriodFigures` take `sc authctx.Scope`; the 4 call sites shimmed with `authctx.Scope{TeacherID: teacherID}` (temporary, zero `CenterID`), first site carries the required comment. Nothing else in this file touched; its own test suite stays pre-existing red (not run, per instruction).
- `apps/api/internal/features/statements/integration_test.go`, `public_integration_test.go` — mechanically updated every `svc.Generate/List/Get/Revoke` call site to pass `testutil.ScopeFor(t, db, teacherX.ID)`.
- `apps/api/internal/features/statements/auth_integration_test.go` — new file, the 3 mandated auth tests.
- `apps/api/internal/features/statements/service_test.go` — a pre-existing unit-test file discovered mid-sweep (not previously flagged): its `fakeRepository` implemented the old `Repository` interface. Converted every method to `sc authctx.Scope` (matching, with owner bypass, the real `scoped()` semantics), added `setPeriod()` to carry `PeriodInfo`, and rebuilt every test's scope construction as two-field `authctx.Scope{TeacherID: ..., CenterID: ...}` literals (never single-field, per the leftover-reference rule).

## Tasks completed

- Owner oversight, peer isolation, and cross-center + public-token-regression tests written first, observed RED against the old package (pre-compaction), then implementation done to match.
- Statement model carries `CenterID`; `Generate` anchors created rows on the period's own owning teacher, not the caller.
- Public path derives scope from the resolved statement row on every downstream read (`InvoicesWithLines`, `LiveSessions`, `Adjustments`, `TouchView`).
- `zero authctx.TeacherID` references and zero single-field `authctx.Scope{TeacherID: x}` literals in `statements` — confirmed via grep (only two-field/three-field literals remain, all legitimate derivations).

## Tests status

- `go test -tags integration ./internal/features/statements/ -count=1` — **PASS**, all 20 tests including the 3 new auth tests and the public-token regression.
- `gofmt -l internal/ seeds/` — clean, no output.
- `go build -buildvcs=false ./...` — clean.
- `go vet -tags integration -buildvcs=false ./...` — clean.
- `notifications` package suite intentionally **not run** (pre-existing red, raw SQL fixtures missing `center_id`, unrelated to this sweep).

## Sabotage check

Inverted `scoped()`'s owner bypass in `repository.go` (`if !sc.IsOwner` → `if true`), re-ran `TestOwnerGeneratesAndReadsMembersStatementsAnchoredOnMember`:

```
auth_integration_test.go:50:
    Error Trace: /src/apps/api/internal/features/statements/auth_integration_test.go:50
    Error:       Not equal:
                 expected: int(1)
                 actual  : int64(0)
    Test:        TestOwnerGeneratesAndReadsMembersStatementsAnchoredOnMember
```

The owner's `List` call returned 0 statements instead of 1 once the always-apply `teacher_id` filter excluded the member-owned row — proving the test actually detects the real vulnerability. Reverted; full suite green again afterward.

## Concerns

- One genuine bug found and fixed in my own newly-written `auth_integration_test.go`: `pagination.Params{}` zero-value has `PerPage: 0`, which GORM's `Limit(0)` turns into zero rows regardless of matching data — unrelated to center-tenancy, a plain pagination-fixture mistake. Fixed to `pagination.Params{Page: 1, PerPage: 20}`, matching the established pattern in `payments/integration_test.go`.
- `service_test.go`'s `fakeRepository` does not model center mismatch (it ignores `CenterID` on lookups, matching on `TeacherID`/`IsOwner` only) — acceptable because these are unit tests of `Service`'s own logic (period authorization, upsert idempotency, revoke), while center-scoping itself is proven by the real-Postgres integration tests in `auth_integration_test.go`.
- `notifications/service.go`'s shim scopes carry `CenterID: uuid.Nil`. This is pre-authorized as temporary scope until notifications itself is re-keyed to center tenancy; it compiles and the 4 call sites are the only allowed literals outside `statements`.

Status: DONE
Summary: Statements package re-keyed to center-tenancy with owner oversight; all 3 mandated auth tests plus the full pre-existing suite pass, sabotage check confirmed the tests detect the real vulnerability, and gofmt/build/vet are clean. Only the pre-authorized file set was touched, including one previously-undiscovered unit test file (`service_test.go`) whose fake repository needed the same interface conversion.
Concerns/Blockers: None blocking. One incidental test-fixture bug (zero-value `pagination.Params{}`) was found and fixed in my own new test file. `notifications` package tests remain pre-existing red as instructed, untouched.
