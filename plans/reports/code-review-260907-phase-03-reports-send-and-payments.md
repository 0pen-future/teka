# Code review — phase 3 (reports.send implied read keys) + payments invoice-candidacy fix

Reviewed: uncommitted tree on `master`, `/home/cesc/Documents/personal-workspace/teka`.
Scope limited to phase 3 and the payments candidacy slice; phase 1/2/5 changes in the
same dirty tree were read only where they carry a phase-3 conclusion.

## Verdict

The authorization substance is right. Every `ReportsOversight()` survivor is a genuine
send gate, no converted helper backs a write, the implied keys cannot open a route or a
role row, and the payments re-key is a correct fix for a real silent-data bug. What
blocks landing is mechanical: this change set adds a guard test and then fails it in
twelve places, and it leaves a cluster of interface doc comments describing the gate
that was just removed.

Score: **7/10**.

---

## Critical

### C1. `TestRepositoriesWidenWritesThroughWriteWideOnly` fails — 12 violations, all from phase 3

`apps/api/internal/features/scoping_guard_test.go:48`

`go test -count=1 ./internal/features/` is red. The guard is **new in this change set**
(`git show HEAD:apps/api/internal/features/scoping_guard_test.go` declares only
`TestRepositoriesScopeThroughCenterWideOnly`), and every violation it reports was
introduced by the phase-3 conversions:

```
collections/repository.go:60   CenterWideFor inside PeriodExists
collections/repository.go:105  CenterWideFor inside contactBalanceQuery
collections/repository.go:205  CenterWideFor inside childInvoicesByContact
collections/repository.go:264  CenterWideFor inside classCollectionsQuery
collections/repository.go:342,360,388,394,395  CenterWideFor inside PeriodSummary
notifications/repository.go:233  CenterWideFor inside runsPeriodScoped
notifications/repository.go:295  CenterWideFor inside ListByPeriod
notifications/repository.go:589  CenterWideFor inside ZaloMappings
```

The guard's contract is that `CenterWideFor` may appear only inside a function whose
name contains `read`. None of these twelve do.

I verified each flagged function is genuinely a read, so this is **not** a live write
widening. It is still blocking, for two reasons. `make test-api` fails. And the payments
slice report already dismissed this as "pre-existing uncommitted work, outside my file
ownership" while the phase-3 report never ran the package at all — a guard that two
consecutive agents step around is a guard that has already stopped working.

**Fix**: rename to satisfy the convention the guard encodes.
- `runsPeriodScoped` → `runsPeriodRead`, `ListByPeriod` → route the widening through a
  `readNarrow`-style helper, `ZaloMappings` → same.
- In `collections/`, the five inline `if !sc.CenterWideFor(...)` blocks and the four
  `PeriodSummary` bind args should funnel through one `readNarrow(q, sc, col)` helper
  the way `billing/repository.go:349` and `payments/repository.go:208` already do. That
  collapses nine violations to zero and removes the duplicated predicate at the same
  time.

Do not widen the guard's allowlist to make the red go away. The name convention is the
only thing telling reads and writes apart syntactically here.

---

## High

### H1. `docs/api-guidelines.md` documents the contract the tree violates

The rewritten Tenancy section now states that
`apps/api/internal/features/scoping_guard_test.go` pins that "`CenterWideFor` may appear
only inside a read-named function (`readScoped`, `scopedRead`, `readNarrow`,
`GetPeriodRead`, …)". That claim is false against the shipped tree in twelve places.
Resolved by fixing C1; flagged separately because the doc is what a future contributor
will trust.

### H2. Interface doc comments still describe the removed `ReportsOversight` gate

These are the contracts other features read to decide what a call guarantees. They now
name a gate the implementation no longer consults:

| Location | Says | Actually |
|---|---|---|
| `billing/repository.go:110` | `GetPeriodRead`/`ListPeriodsRead` "center-scoped with reports oversight (owner or reports.send holder)" | `billing.view_all` |
| `billing/service.go:110` | `ListPeriods` "center-wide for a reports-oversight caller" | `billing.view_all` |
| `statements/repository.go:128` | `ListByPeriod` "with reports oversight (owner or reports.send holder)" | `statements.view_all` |
| `statements/repository.go:139` | `GetByID` "with reports oversight" | `statements.view_all` |
| `notifications/repository.go:77` | `ListByPeriod` "unless sc holds reports oversight (owner or reports.send)" | `notifications.view_all` |
| `notifications/repository.go:106` | `LatestRunByPeriod` "unless sc holds reports oversight" | `notifications.view_all` |
| `notifications/repository.go:213` | `writeScoped` "the listing reads take the reports-oversight axis in ListByPeriod" | `notifications.view_all` |

`notifications/repository.go:180` is the worst of the set. `ZaloMappings` is documented
as "covering live (non-deleted) contacts **sc's own teacher owns** — widened to the whole
center when sc holds reports oversight". The teacher arm was deleted outright in this
change; the real reach is now center plus (`contacts.view_all` OR an active `hoc_vu`
stint). A reader trusting this comment would conclude the opposite of what the query does.

**Fix**: mechanical, one pass over the eight sites. The already-converted comments in
`collections/repository.go:29,50`, `contacts/repository.go:87` and
`statements/repository.go:280` are the right template.

---

## Medium

### M1. `billing.view_all` now widens reads of payments data, bypassing `payments.view_all`

`apps/api/internal/features/collections/repository.go:387-395`

`PeriodSummary`'s `unallocated_credit` figure reads `payments` and `payment_allocations`,
and its three `(? OR teacher_id = ?)` short-circuits are now bound to
`sc.CenterWideFor(PermBillingViewAll)`. At `HEAD` they were bound to
`sc.ReportsOversight()`, so a plain `billing.view_all` holder saw nothing center-wide here.

Net effect: `billing.view_all` is now sufficient to read center-wide payment aggregates,
while `payments/repository.go:208` still gates payments row reads on `payments.view_all`.
The two axes disagree about the same table.

This follows the plan's stated rule ("`view_all` of the resource the ROUTE serves"), and
the leak is an aggregate rather than rows, so it is defensible. It is nonetheless a
widening that neither report calls out and that no test covers — `TestReportsSendImpliedKeysParity`
never touches a collections route. Either accept it explicitly in the plan's decision
record, or bind the two payments-derived fragments to `PermPaymentsViewAll` and leave the
invoice-derived ones on `PermBillingViewAll`.

### M2. `ZaloMappings`'s stint arm is unreachable from every production caller

`apps/api/internal/features/notifications/repository.go:589`

All three call sites (`service.go:252`, `:539`, `:752`) take the `ZaloMappings` branch only
when `classID == nil`, and the family dimension is gated by `ReportsOversight()` at
`service.go:129`, `:499`, `:678`. `reports.send` now implies `contacts.view_all`, so
`sc.CenterWideFor(PermContactsViewAll)` is always true by the time execution arrives. The
class dimension goes to `ZaloMappingsClass` instead.

The implementer names this honestly in the phase-3 report and wrote
`TestZaloMappingsFollowsCenterWideOrStint` against the exported constructor specifically
to reach a branch no route can reach. That is a test that documents dead code rather than
behaviour, and the ambiguous-column fix it found (`contacts.id`) is a fix to a line that
can never execute in production.

Two coherent options, both better than the current state:
- **Simplify** (KISS, and it removes one C1 violation): drop the stint arm, make
  `ZaloMappings` center-keyed like its `ZaloMappingsClass` sibling, and delete the test.
  The send gate upstream is the authorization.
- **Keep as defence in depth**, but say so in the doc comment and in the test name, so the
  next reader does not spend the same twenty minutes proving it is dead.

### M3. `MarkSent` returns 404 for a request containing duplicate ids

`apps/api/internal/features/notifications/repository.go:307-320`

`reachable != int64(len(ids))` compares a `COUNT` of matching rows against the raw slice
length. `{"ids":["A","A"]}` with `A` fully reachable counts 1 against a length of 2 and
returns `NotFound`. `MarkSentRequest` (`dto.go:102`) binds `required,min=1,dive` with no
`unique`, and neither the handler nor the service dedupes.

The failure direction is safe — I confirmed duplicates can only cause a spurious rejection,
never a bypass, since an unreachable id always drops the count below the length. But a
client merging two selection lists gets an unexplained 404 on rows it owns.

**Fix**: dedupe before the count, or compare against the distinct-id count.

```go
uniq := make(map[uuid.UUID]struct{}, len(ids))
for _, id := range ids { uniq[id] = struct{}{} }
if reachable != int64(len(uniq)) { return apperror.NotFound("notification") }
```

---

## Low

### L1. `assertLedgerInvariant` is now weaker than it needs to be

`apps/api/internal/features/payments/integration_test.go:90`

Moving from a per-teacher to a per-center sum was necessary — a contact's invoices can now
legitimately span teachers. But a center-wide sum cannot detect money moving between two
invoices inside the same center, which is precisely what `Reallocate` does. A per-invoice
assertion (`invoices.paid_amount = signed allocation sum` for every invoice in the center)
is strictly stronger, still true under the new keying, and is what `RecalcInvoicePaid`
actually promises.

### L2. `docs/adding-permissions.md:97` states the deny rule backwards

> "never removable by denying the source key alone — deny the source key itself to remove both"

The first clause should read *denying an implied key alone*. As written the sentence
contradicts itself, and this is the single subtlest behaviour in the whole mechanism.

### L3. Phase 3 Success Criterion 2 is checked off but only partly met

The criterion asks for HTTP-layer parity on contacts, billing **including `ListPeriods`**,
statements, notifications, and **collections**. `TestReportsSendImpliedKeysParity`
(`internal/server/policy_integration_test.go:355`) covers the single-period GET, the
statement list, the notification ledger and the contact, but hits no collections route and
no `GET /billing-periods` list.

Collections is not uncovered — `collections/secretary_integration_test.go:19` exercises a
`reports.send` holder through a real `testutil.ScopeFor` resolve at the service layer. So
the risk is low and I would not block on it. The plan file's checkbox is simply more
confident than the evidence.

### L4. `MatchFriendsScoped` widened the set of principals who may send phones to Zalo

`apps/api/internal/features/zalo/service.go:492` moved from `ReportsOversight()` to
`CenterWideFor(PermContactsViewAll)`. Beyond the intended `reports.send` parity, this also
admits a direct `contacts.view_all` holder, who previously could not match. Matching
transmits phone numbers to a third party.

Consistent with the design premise that a contact row *is* its phone, so a `contacts.view_all`
holder already reads every number. Recording it because third-party egress is a different
question from in-system read reach, and no report names the change.

### L5. Decision-ID references in new comments

`payments/integration_test.go` and `payments/repository.go` add `D8`/`D4`/`D5` references.
`.claude/rules/review-audit-self-decision.md` bans plan/decision labels in code. This is
pre-existing local convention (`git grep D8 HEAD -- apps/api/internal/features/payments`
returns ten hits), so following it here is the consistent choice. Noting it so the rule and
the codebase can be reconciled deliberately rather than by drift.

---

## Verified clean

Each of these was checked against source, not taken from the reports.

**Send gates (check a).** `grep -rn 'ReportsOversight()' internal --include='*.go' | grep -v _test.go`
returns the definition plus six sites. I read each: `statements/service.go:110`
(`AuthorizeClassSend`), `:347` (`ToResponse` populating the public link — the link *is* the
sent artifact), `contacts/repository.go:113` (`scopedMappingWrite`, rewiring where a family's
messages land), `notifications/service.go:129` (`BulkSend`), `:398` (`runGrant`), `:499`
(`SendPreview`), `:678` (`ResumeRun`). All are send-creation. No read remains on the helper.

**No write widened (check b).** Walked every caller of `readScoped`, `scopedRead`,
`runsPeriodScoped`, `centerScoped`, `invoiceCenterScoped`. `runsPeriodScoped` feeds only
`LatestRunByPeriod`; every run mutation (`UpdateRunStatus`, `MarkOutcome`, `FailQueuedInRun`)
takes an `authctx.Anchor`. `notifications.writeScoped` correctly uses `WriteWide()`.
`contacts.scopedMappingWrite` correctly stayed on the send axis and explicitly refuses
`contacts.view_all`. No UPDATE/DELETE/INSERT path reaches a read-widened helper.

**`MarkSent` cannot be bypassed by duplicate ids.** Only the false-rejection direction is
reachable (M3).

**`ZaloMappings` stint arm matches `contacts.scopedRead` exactly.** Both call
`classscope.PhoneVisibleViaContact("contacts.id")` and bind `sc.TeacherID, sc.CenterID` in
that order. The `contacts.id` qualification fix is correct — the fragment's own `s3`/`e3`/`c3`
aliases each carry an `id`, so the bare column was ambiguous.

**Implied keys never persist (check c).** `knownKeysOf` (`centers/service.go:362`) reads the
raw comma-joined DB list and never passes through `BuildPermSet`; the roles projection
(`:415`), the member projection (`:425-426`) and both audit before-images (`:475`, `:572-573`)
all use it. `ListMembers`'s SQL (`centers/repository.go:293-303`) computes only `reports.send`
itself. `TestBuildPermSetImpliesReadKeysForReportsSend` asserts that denying `reports.send`
drops all five keys, and that the four read keys do not imply `reports.send` in reverse.

**Implied keys cannot open a route.** No route in `routespec.Specs` is gated on a `view_all`
key (`grep -c view_all internal/server/route_policy_snapshot_test.go` → 0), and no `Has()`
call anywhere tests one. `CenterWideFor` is the only consumer.

**Payments tenancy is sufficient (check d).** `center_id + contact_id` is the right key:
`ResolveContactAnchor` has already proven the contact is in `sc`'s center before
`CandidateInvoices` runs, and `Reallocate`/`Reverse`/`AutoAllocateRemainder` all reach an
invoice only through a payment `LockPayment` already gated under `writeScoped` (own rows
unless owner). A member with `payments.create` **cannot** allocate onto another family's
invoice: `reversal.go:146` (`inv.ContactID != payment.ContactID`) runs before any write and
short-circuits into `Invalid` at `:163`. Every caller of the three re-keyed methods passes
`sc`; no `Anchor` is left at any call site, and the two unit-test fakes match the new
signature. Allocation rows and the payment row still stamp the contact's owner anchor.

**No orphaned hand-built scopes (check e).** `grep -rn 'CanSendReports:'` outside tests
returns `centers/service.go:81`, `testutil/fixtures.go:203` and `seeds/seed.go:354`, all
deriving the flag from `perms.HasKey(PermReportsSend)` so `Perms` is always populated
alongside. The only bare `Scope{CanSendReports: true}` left is
`authctx/phone_visibility_test.go:56`, which exists to assert that such a scope sees nothing.
`MatchFriendsScoped` resolves correctly for all four principals: owner and
`contacts.view_all` holder and `reports.send` holder take the wide arm, `hoc_vu` falls to the
stint lookup, plain member is refused.

**Tooling (check g).**

```
gofmt -l internal                                          clean
go vet -tags=integration ./internal/{features,shared}/... ./internal/server/   clean
make lint-api                                              0 issues
go test ./internal/shared/authctx/                         ok
go test ./internal/features/{payments,notifications,statements,billing,contacts}/  ok
go test ./internal/features/                               FAIL  (C1)
```

`collections` has no unit tests, only integration.

---

## Recommended actions, in order

1. Fix C1 by renaming the twelve sites to the guard's read convention, preferring a shared
   `readNarrow` helper in `collections/repository.go` over nine inline predicates. Re-run
   `go test ./internal/features/`.
2. Correct the eight stale interface doc comments in H2, `notifications/repository.go:180`
   first.
3. Decide M1 explicitly: either bind the two payments-derived fragments in `PeriodSummary`
   to `payments.view_all`, or record the cross-axis widening in the plan's decision list.
4. Dedupe `MarkSent`'s id slice before the reachability count (M3).
5. Resolve M2 — simplify `ZaloMappings` to center-keyed, or document the arm as deliberate
   defence in depth.
6. Fix the reversed sentence at `docs/adding-permissions.md:97` (L2).

## Unresolved questions

- **M1 is a product call.** Should `billing.view_all` carry center-wide sight of unallocated
  payment credit, or should that figure follow `payments.view_all`? The route is a billing
  route but the datum is payments money.
- **M2 needs an owner.** Is the `ZaloMappings` stint arm intended future-proofing for a
  class-dimension caller that does not exist yet, or leftover from before the implication
  landed?
- The phase-3 report flags a separate web display gap in `member-permissions-dialog.tsx`
  (implied keys badged as absent in the permission editor). I did not review web code; it is
  a display-accuracy issue, not enforcement, and still needs a follow-up decision.

```
Status: DONE_WITH_CONCERNS
Summary: The authorization design is sound — every ReportsOversight survivor is a real
send gate, no read conversion widened a write, implied keys cannot open a route or persist
to a role, and the payments center-keying correctly fixes a silent zero-candidate bug. But
the change set adds a scoping guard and then fails it in twelve places, so ./internal/features
is red and make test-api will not pass.
Concerns/Blockers: C1 (failing TestRepositoriesWidenWritesThroughWriteWideOnly, 12 sites)
blocks landing. H2 (eight interface doc comments describing the removed gate, ZaloMappings'
being actively misleading) should land with it. M1 is an undocumented cross-axis read
widening that needs a decision, not necessarily a code change.
```
