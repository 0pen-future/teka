# sweep-payments: payments package center-tenancy re-key

Task: re-key `apps/api/internal/features/payments` from teacher-tenancy to center-tenancy with owner oversight, mirroring `billing`/`attendance`. TDD, mechanical out-of-package ripple pre-authorized for 2 files only.

## Files touched

| File | Purpose |
|---|---|
| `payments/model.go` | Added `CenterID uuid.UUID` to `Payment` and `PaymentAllocation` with house invariant comments. |
| `payments/repository.go` | Added `scoped()`/`allocationScoped()` helpers; converted every method and raw SQL predicate (candidateInvoicesQuery, recalcInvoicePaidQuery, ListPayments' EXISTS subquery, InvoicesByIDs, ListAllocations, ListAllocationsForPayments) to center + conditional-teacher discipline; added `ResolveContactScope`. |
| `payments/service.go` | `Record/Get/List` take `authctx.Scope`; `Record` resolves the contact's own owning scope via `ResolveContactScope` and stamps the payment/allocations on that, not the caller's scope, with the owner-records-member's-payment invariant comment. |
| `payments/reversal.go` | `Reallocate/Reverse/AutoAllocateRemainder/recalcTouched` take `authctx.Scope`; each derives a `paymentScope`/local scope from the locked payment row for every internal follow-up call, with the final read using the caller's own `sc`. |
| `payments/handler.go` | `teacherID()` → `scope()` using `authctx.ScopeFrom(c)`; all 6 handlers pass `sc` downstream. |
| `payments/routes.go` | `RegisterRoutes` gained `resolveScope gin.HandlerFunc` param, applied to the route group. |
| `server/router.go` | payments wiring passes `resolveScope` (line 156). |
| `payments/service_test.go` | Rewritten: fake repository + all 8 unit tests converted to `authctx.Scope` via a `scopeFor(teacherID)` helper (random CenterID per call, mirrors billing's unit-test convention — CenterID/IsOwner tenancy is not modeled at unit level). |
| `payments/reversal_test.go` | Rewritten: fake repository extension methods + all 12 unit tests converted the same way. |
| `payments/integration_test.go` | Converted all 27 existing `paymentsSvc.*` call sites from raw `teacher.ID`/`stranger.ID` to `testutil.ScopeFor(t, db, ...)`; hoisted scope resolution out of the two goroutine-based concurrent tests (testutil.ScopeFor calls t.Fatalf, unsafe from a spawned goroutine); added 3 new authorization tests; added `pagination` import. |
| `collections/integration_test.go` | Pre-authorized mechanical fix: 2 call sites, `teacher.ID` → `testutil.ScopeFor(t, db, teacher.ID)`. |
| `statements/public_integration_test.go` | Pre-authorized mechanical fix: 2 call sites, same substitution. |
| `notifications/integration_test.go` | Team-lead-approved mechanical fix (post-report, one call site at line 227): `teacher.ID` → `testutil.ScopeFor(t, d.db, teacher.ID)`, compile-only, rest of the suite left unswept. |

## Anchor decision: Record's owner-records-member's-payment twist

Evidence: `ResolveContactScope` (`repository.go:381-400`) queries `contacts` filtered by `center_id = sc.CenterID` (and `teacher_id = sc.TeacherID` when `!sc.IsOwner`), then returns `authctx.Scope{TeacherID: rows[0].TeacherID, CenterID: sc.CenterID}` — the contact's own owning teacher, not the caller. `CenterID: sc.CenterID` is not a caller-authority shim: the contact row was already proven to belong to that center by the `WHERE center_id = sc.CenterID` filter, so it's the same center value the loaded row was matched against, not an unverified pass-through.

`Record` (`service.go`) calls this, gets 404 if `!ok`, then stamps `Payment{TeacherID: ownerScope.TeacherID, CenterID: ownerScope.CenterID, ...}` and every `PaymentAllocation` the same way. Verified end-to-end by `TestOwnerHasFullOversightOfMembersPayments`: direct DB read after an owner records a payment for a member's contact shows `teacher_id = member.ID`, not the owner's; the owner's own scoped `Get` read-back agrees.

## Derived-scope discipline for internal follow-ups

`Reallocate`, `Reverse`, `AutoAllocateRemainder` each lock the payment row first, then derive `paymentScope := authctx.Scope{TeacherID: payment.TeacherID, CenterID: payment.CenterID}` and thread it through every downstream call inside the transaction (`AllocationsByPayment`, `InvoicesByIDs`, `DeleteAllocations`, new allocation rows, `recalcTouched`). The final post-transaction read uses the caller's raw `sc`, not `paymentScope` — mirroring billing's `periodScope` split between internal recompute and the authorized final read.

## Test results

- `go vet -tags integration ./internal/features/payments/...`: clean.
- `go test -tags integration ./internal/features/payments/ -count=1`: `ok teka/apps/api/internal/features/payments 6.3s` (full suite, including the 3 new tests, verified individually green with `-v -run`).
- `gofmt -l internal/features/payments/`: no output (clean).
- 11-package regression gate (payments, billing, attendance, sessions, enrollments, classes, students, contacts, teachers, centers, auth), all with `-tags integration -count=1`: all `ok`.

## Sabotage check

Inverted `scoped()`'s `if !sc.IsOwner` to `if true` in `repository.go` (always applying the teacher_id filter, defeating owner oversight on the `payments` table). Re-ran `TestOwnerHasFullOversightOfMembersPayments`:

```
Error Trace: /src/apps/api/internal/features/payments/integration_test.go:723
Error:       Received unexpected error: NOT_FOUND: payment not found
Test:        TestOwnerHasFullOversightOfMembersPayments
Messages:    owner must be able to read a member's payment
```

Failed exactly as expected — the owner's scoped `Get` on the member's payment now 404s. Reverted the sabotage, re-ran the full payments suite: green again (`ok teka/apps/api/internal/features/payments 6.3s`).

## Zero-shims verification

- `grep -rn "authctx.TeacherID" apps/api/internal/features/payments/`: empty.
- `grep -rn "authctx.Scope{" payments/*.go` (non-test): every construction sources `CenterID` from a loaded row (`payment.CenterID`, `original.CenterID` in reversal.go) or from a row already filtered by that same center (`ResolveContactScope`'s `sc.CenterID`, proven by the query's own `WHERE center_id = sc.CenterID`). No bare caller-authority literal.
- Hand-inspected every raw-SQL predicate site in `repository.go`: `candidateInvoicesQuery` (`i.center_id = ? AND (? OR i.teacher_id = ?)`), `recalcInvoicePaidQuery` (same owner short-circuit pattern), `ListPayments`' period-filter EXISTS subquery (`pa.center_id = ?` + conditional `pa.teacher_id = ?`, joined `i.center_id = pa.center_id`), `InvoicesByIDs` (`center_id = ? AND id IN ?` + conditional teacher_id), `ListAllocations`/`ListAllocationsForPayments` (`pa.center_id = ?` + conditional `pa.teacher_id = ?`, joined `i.center_id = pa.center_id`). All confirmed correct.

## Deviation from strict TDD ordering (disclosed)

The payments implementation (model/repository/service/reversal/handler/routes/router) was already complete from a prior session segment before this segment began; the 3 new authorization tests were written and run only after the implementation existed, so true pre-implementation "red" was not captured for them. The sabotage check above is offered as equivalent evidence that these tests genuinely exercise the center-oversight boundary they were written to prove — the assertion at `integration_test.go:723` fails precisely and specifically when that boundary is broken.

## Notifications breakage — reported, approved, fixed

`go vet -tags integration ./...` at the module root surfaced one call site outside the originally pre-authorized ripple: `apps/api/internal/features/notifications/integration_test.go:227` called `d.payments.Record(ctx, teacher.ID, payments.RecordPaymentRequest{...})` with a raw `teacher.ID`, which no longer compiled against the new `authctx.Scope`-based signature. Per the assignment's explicit rule, this was reported to main and left untouched pending approval. Main approved the identical mechanical fix scoped to that one line. Applied: `teacher.ID` → `testutil.ScopeFor(t, d.db, teacher.ID)` (using the deps struct's own `db` field, matching the surrounding helpers in that file; `testutil` was already imported). Nothing else in notifications was touched — that suite remains otherwise unswept, compile-only fix.

After this fix, re-ran the full module-wide gate:

```
gofmt -l internal/ seeds/ && go build -buildvcs=false ./... && go vet -tags integration -buildvcs=false ./...
```

No output — clean. Re-ran the payments suite once more as a final sanity check: `ok teka/apps/api/internal/features/payments 6.4s`.

Status: DONE
Summary: Payments package fully re-keyed to center-tenancy with owner oversight; payments suite green including 3 new authorization tests; sabotage check passed with the expected failing assertion; 11-package regression gate green; zero shims confirmed; module-wide gofmt/build/vet clean after the approved one-line notifications fix.
Concerns/Blockers: none.
