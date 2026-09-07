# Payments candidacy fix — center-keyed invoice lookups

## Bug

Contacts anchor to the center owner (`contacts.teacher_id`); billing periods
and their invoices anchor to whoever closed the period, which can be any
member of the center (`billing/close.go`'s `sc.AnchorTo(period.TeacherID)`,
`billing/preview.go` stamping `inv.TeacherID = a.TeacherID`). A payment
against a contact resolved its lookup anchor from the contact
(`ResolveContactAnchor`, owner-anchored), then filtered
`CandidateInvoices`/`RecalcInvoicePaid`/`InvoicesByIDs` by
`i.teacher_id = <that anchor's TeacherID>`. When a member closed the period
for a child under an owner-anchored contact, the resulting invoice carried
`teacher_id = member`, so it never matched the owner-anchored filter: `Record`
found zero candidates, `RecalcInvoicePaid` updated zero rows, silently.

## Fix, per method

All three now take the caller's `sc authctx.Scope` (not an `authctx.Anchor`)
and key strictly by `center_id` plus the method's own object id — never a
teacher.

- **`CandidateInvoices(ctx, sc, contactID)`** — `candidateInvoicesQuery`'s
  `WHERE` dropped `(? OR i.teacher_id = ?)`, now
  `WHERE i.center_id = ? AND i.contact_id = ?`. `FOR UPDATE OF i` and the
  `ORDER BY i.id` lock-acquisition order are unchanged.
- **`InvoicesByIDs(ctx, sc, ids)`** — rewritten onto the new
  `invoiceCenterScoped(ctx, sc)` helper (`Table("invoices").Where("center_id
  = ?", sc.CenterID)`) plus `.Where("id IN ?", ids)`. Same
  `Clauses(clause.Locking{Strength: "UPDATE"}).Order("id")` as before.
- **`RecalcInvoicePaid(ctx, sc, invoiceID)`** — `recalcInvoicePaidQuery`'s
  final `WHERE` dropped `(? OR i.teacher_id = ?)`, now
  `WHERE i.id = target.invoice_id AND i.center_id = ?`. The `.Exec(...)` call
  drops the `false, a.TeacherID` params, binding only `sc.CenterID`.

**New helper** `invoiceCenterScoped(ctx, sc authctx.Scope) *gorm.DB` sits
next to `readNarrow` in `repository.go`. It is deliberately not named
`CenterWideFor`/`WriteWide`/an `Anchor`: it reads no permission and binds
`center_id` only, with a doc comment explaining that an Anchor here would lie
about filtering by teacher. `InvoicesByIDs` (a compiled query) chains off it
directly. `CandidateInvoices` and `RecalcInvoicePaid` are raw SQL (a `.Raw()`
LATERAL join and a single `.Exec()` UPDATE) — GORM's `.Raw()`/`.Exec()`
discard any `.Where()` already queued on the same `*gorm.DB` chain, so those
two bind `sc.CenterID` as a literal SQL parameter instead, with a comment
cross-referencing the same invariant.

**Callers updated** to pass `sc` instead of an owner-anchored
`authctx.Anchor`:
- `service.go` `Record`: `CandidateInvoices(txCtx, sc, req.ContactID)`,
  `RecalcInvoicePaid(txCtx, sc, a.InvoiceID)`.
- `reversal.go` `recalcTouched(ctx, sc authctx.Scope, set)` (renamed param
  from `a authctx.Anchor`, doc comment rewritten): `RecalcInvoicePaid(ctx, sc,
  invID)`.
- `reversal.go` `Reallocate`: `InvoicesByIDs(txCtx, sc, lockIDs)`,
  `recalcTouched(txCtx, sc, touched)`.
- `reversal.go` `Reverse`: `InvoicesByIDs(txCtx, sc, affected)`,
  `recalcTouched(txCtx, sc, touched)`.
- `reversal.go` `AutoAllocateRemainder`: `CandidateInvoices(txCtx, sc,
  payment.ContactID)`, `recalcTouched(txCtx, sc, touched)`.

**Left untouched, exactly as instructed**: `ResolveContactAnchor`, the
payment insert (`Payment{TeacherID: anchor.TeacherID, ...}`), `AllocationsOf`,
`allocationAnchored`, `DeleteAllocations`, `AllocationsByPayment`,
`MarkReversed`. Every allocation row and the payment row itself still stamp
`paymentAnchor.TeacherID/CenterID` (the contact's owner) — only the invoice
*lookup* queries changed. `paymentAnchor := sc.AnchorTo(payment.TeacherID)` in
`reversal.go` is still used for `AllocationsByPayment`, `DeleteAllocations`,
`MarkReversed`, and every allocation-row stamp; only the invoice-lookup and
recalc calls switched to `sc`.

One incidental fix: `Reallocate`'s validation error for an invoice id that
`InvoicesByIDs` didn't return was `"invoice not found for this teacher"`,
which became inaccurate once the lookup is center-keyed. Changed to `"invoice
not found in this center"`. No test asserted the old string.

## Tests

**New integration test** (RED then GREEN):
`TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod`
in `internal/features/payments/integration_test.go`. Owner and member share
a center; a contact/students are created under the owner; the member closes
their own billing period for those students, producing invoices with
`teacher_id = member`. Covers:
- (a) owner `Record` → 1 allocation, invoice settles.
- (b) a member scope holding only `payments.create` (no `payments.view_all`)
  `Record`s against the same contact → 1 allocation, second invoice settles.
- (c) owner `Reverse`s the first payment → invoice returns to `issued`.
- (d) owner `Reallocate`s the member-recorded payment onto the now-open
  invoice, whose `teacher_id` differs from both the payment's own anchor
  (owner) and the acting caller — exercises the invoice lookup used by
  reallocation directly.
- `assertLedgerInvariant` at the end, scoped to the center.

**RED** (before the fix, run via `go test -tags=integration -p 1 -count=1
-run TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod
-v ./internal/features/payments/`):
```
integration_test.go:1023:
    Error:      	"[]" should have 1 item(s), but has 0
    Test:       	TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod
    Messages:   	the owner's payment must settle the member-closed invoice, not find zero candidates
--- FAIL: TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod (4.07s)
FAIL
```

**GREEN** (after the fix, same command):
```
=== RUN   TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod
--- PASS: TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod (4.00s)
PASS
ok  	teka/apps/api/internal/features/payments	4.269s
```

**`assertLedgerInvariant`** moved from per-teacher to center-level sums
(`invoices.paid_amount` summed by `center_id`, ledger summed by joining
`payment_allocations` to `payments` filtered on `pa.center_id`) — a
per-teacher sum can no longer balance once a contact's invoices legitimately
span more than one `teacher_id`. All 12 existing call sites updated to pass
a center id instead of a teacher id.

**Existing payments tests**: two unit-test fakes updated for the signature
change only (no logic change, since neither used the anchor/scope param):
`fakeRepository.CandidateInvoices`/`RecalcInvoicePaid` in `service_test.go`,
`fakeRepository.InvoicesByIDs` in `reversal_test.go`.

**Cross-contact guard** — `TestReallocateToAnotherContactsInvoiceIsInvalid`
(unit, `reversal_test.go`) and
`TestReallocateToAnotherContactsInvoiceIsRejectedAndWritesNothing`
(integration, `integration_test.go`) both still pass; the guard
(`inv.ContactID != payment.ContactID`) is unaffected by the key change since
it runs after `InvoicesByIDs` returns the row.

## Verification (all from `apps/api`, single-process per the load-warning)

```
gofmt -l internal/features/payments                                            # clean, no output
go build ./...                                                                  # clean
go vet -tags=integration ./internal/features/payments/                         # clean
go test -count=1 ./internal/features/payments/                                 # ok  0.013s
go test -count=1 ./internal/features/                                          # FAIL — see note below
go test -tags=integration -p 1 -count=1 ./internal/features/payments/          # ok  7.809s
```

From repo root: `make lint-api` → `0 issues.`

### Note on `go test -count=1 ./internal/features/`

This fails, but not from anything in `payments/`. The failure is
`TestRepositoriesWidenWritesThroughWriteWideOnly` in
`internal/features/scoping_guard_test.go` flagging `CenterWideFor` calls in
`collections/repository.go` and `notifications/repository.go`. Both files
are pre-existing uncommitted modifications from other in-flight work (not
under `payments/**`, outside this task's file ownership; `git status` shows
them dirty on `master` alongside my changes). Confirmed by stashing just
those two directories and re-running: the `collections/` violations
disappeared (they were introduced by that uncommitted work), while
`notifications/repository.go:212` and three `TestScopeLiteralsOnlyWhereResolved`
violations in `notifications/run_manager.go`/`service.go` persisted — i.e.
those are baked into the committed tree already, unrelated to this task
either way. `payments/repository.go` contributes zero violations to this
guard test in every run. Not fixed, per file-ownership constraint (would
require touching `collections/**` and `notifications/**`).

## Cleanup

Testcontainers (Postgres 16-alpine + ryuk reaper) started and self-terminated
after each integration run; `docker ps` confirms no leftover
`org.testcontainers=true` containers. Production containers (`teka-db`,
`teka-api-1`, `teka-web-1`) were never touched.

## Not done

Nothing in scope was left undone. The `notifications`/`collections` guard
failure above is explicitly out of file ownership and pre-existing.

Status: DONE
Summary: Rekeyed CandidateInvoices/InvoicesByIDs/RecalcInvoicePaid to
center+object-id via a new non-Anchor invoiceCenterScoped helper, dropped the
dead OR-arm SQL trick, updated all callers in service.go/reversal.go to pass
sc; new TDD integration test reproduces and now covers the owner/member
cross-teacher-invoice scenario end to end (Record by owner, Record by a
create-only member, Reverse, Reallocate), assertLedgerInvariant moved to
center-level; full payments unit+integration suites and make lint-api are
green.
Concerns/Blockers: go test ./internal/features/ fails on a pre-existing
scoping-guard violation in collections/ and notifications/ — unrelated to
payments and outside this task's file ownership, verified not caused by
these changes.
