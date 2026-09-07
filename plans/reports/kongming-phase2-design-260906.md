# Kongming counsel — Phase 2 (row-anchor type) design forks

Date: 2026-09-06. Plan: `plans/260906-0627-authz-write-scope-root-cause/phase-02-row-anchor-type.md`.
Advisory only. Every claim below was checked against the working tree at
`apps/api/internal` (file:line cited); nothing is from memory.

## TL;DR

1. **Q1 — one rule, not N variants.** Anchor-only callers: change the signature
   in place. Mixed callers: the exported `Scope` method becomes a thin wrapper
   that settles the gate with `sc`, then calls an anchored core via `sc.Self()`
   or `sc.AnchorTo(row.TeacherID)`; export the core only when another package
   needs it. Never add an Anchor→Scope bridge. Two of the cross-feature calls
   (enrollments `ActiveOn` from billing, `FindByStudentAndClass` from imports)
   are truthfully *class/center-keyed* queries, not anchored ones — use the
   plan's third category (`centerScoped`) and keep Anchor out of them.
2. **Q2 — NO-GO on anchoring the dashboard.** `targetScope` means "read as
   teacher T would as a plain member" (stint-based `readScoped` in classes,
   sessions, enrollments, attendance). That is Scope semantics; an Anchor would
   silently drop stint-visible rows and cascade into 4 features for a read-only,
   `dashboard.view`-gated view with zero escalation risk. Keep the literal in
   `centers/dashboard.go`, rename it `viewAs`, document it. Step 8 is superseded
   by the Risk clause.
3. **Q3 — allow `authctx.Anchor{}` literals anywhere.** Anchor carries no
   authority, so a literal cannot fake a gate; a constructor buys nothing over
   the struct literal. Prefer `sc.AnchorTo` where an `sc` exists (14 of 22
   sites), literal where there is no caller (public token, background run,
   repo-internal). Make the guard AST-based (the test file already parses AST)
   and add `authctx.OwnerAnchor{` to the ban list.
4. **Q4 — anchored read variant.** Record's response is the read-back of a write
   the caller was authorised to make; give it `AllocationsOf(ctx, a Anchor,
   paymentID)` sharing `allocationRowSelect`, keep `ListAllocations(sc)` for
   Get/List. Reading with `sc` would tie the response to an unrelated
   visibility key and return empty for the D9 member.
5. **Q5 — slicing is sound; import graph verified.** payments and statements
   import no feature package, notifications → statements only, so C and D
   compile independently of B. E must follow B (centers → attendance/classes/
   sessions; imports → enrollments). Lead owns authctx (all five symbols
   including OwnerAnchor/MintOwnerAnchor), the guard, and docs. Guard stays red
   until E lands; subagents must not run it.

## Reframed problem

The decision is not "Anchor everywhere vs Scope everywhere". It is: *which
parameter type makes the reviewer able to tell, at the signature, whether
view_all/stint widening is meant to apply?* Scope = "the caller; widening may
apply". Anchor = "these rows; no widening, ever". The 22 fake literals are
wrong because they use the first type to mean the second. Requirements:
compiler-enforced separation, no `IsOwner: true` minting outside centers, no
owner behaviour change, no route/DTO change, diff under ~40 files and no
handler edits (phase Risk clause). Non-goal: re-modelling caller-dependent
reads.

## Verified facts that drive the answers

- A fake `Scope{TeacherID, CenterID}` has `IsOwner=false, Perms=nil`, so today
  `readScoped(fake)` ≡ `writeScoped(fake)` ≡ own rows of X — *except* where the
  read helper ORs in a class-staff stint:
  `enrollments/repository.go:108-116`, `sessions/repository.go:120-128`,
  `attendance/repository.go:85-95`, `classes/repository.go:107-114`,
  `students/repository.go:101-105`. A pure `anchored()` helper drops that OR
  branch. This is the only place Anchor conversion changes behaviour.
- Dual-use methods (called with caller `sc` AND a fake scope):
  billing `RecalcInvoiceTotals` (`adjustment.go:81` vs `:375`),
  `ComputePeriod` (`service.go:170` vs `close.go:174`),
  `EnsurePeriod` (`handler.go:78` vs `adjustment.go:476/482`);
  sessions `ListUnconfirmedInWindow` (`service.go:267` vs `close.go:85/111`);
  enrollments `ActiveOn` (5 real callers vs `adjustment.go:336`),
  `FindByStudentAndClass` (`enrollments/service.go:269` vs `imports/apply.go:252`);
  classes `Create`/`AddSchedule` (handlers vs imports);
  contacts `Create`, students `Create` (handlers vs imports);
  payments `ListAllocations` (`service.go:133` vs `:120`);
  notifications `MarkOutcome`/`UpdateRunStatus` (`service.go:763/796` vs run manager).
- Anchor-only (every caller passes a row-derived teacher): all statements
  methods listed at `statements/repository.go:91-189` except
  `GetPeriodStatus*`; `attendance.TallyByEnrollment` (only billing calls it:
  `billing/repository.go:557,1025`); classes `FindActiveByName`,
  `ScheduleExists` (only imports); contacts `FindIDByPhone`, students
  `FindIDByName` (only imports); notifications `RunCounts`, `FailQueuedInRun`;
  the ~18 billing period/invoice methods in the inventory.
- notifications already has the anchored helper under another name:
  `runsOwnScoped` (`notifications/repository.go:242-245`) — center+teacher, no
  widen. statements `TargetContacts(ctx, periodScope, viewer, …)`
  (`repository.go:91`) already models the (Anchor, Scope) pair — it is the
  exemplar signature for the docs.
- Dashboard cascade (`centers/dashboard.go:203,216,219,258`): `classes.List/Get`
  → `readScoped` (stint), `sessions.ListRangeReadOnly` → readScoped,
  `attendance.Get` → `sessions.GetReadableByID` (stint) → `buildResponse` →
  `enrollments.ActiveOn` (stint) + `StudentNames` (`readNarrowNames`, stint) +
  `ListBySession` (stint). Anchoring = 4 packages, ~8 read methods, and the
  owner would stop seeing classes T staffs but does not own.
- Guard bypass the plan's regexes miss: a multi-line literal
  (`authctx.Scope{` at end of line, fields on the next lines — exactly how
  `centers/service.go:74` is written) and `authctx.OwnerAnchor{Anchor: …}`.
  `.TeacherID =` on a Scope copy cannot be grep-guarded because
  `statements/repository.go:472-473` legitimately assigns `stmt.TeacherID`.
- Test isolation: one testcontainers Postgres per package
  (`testutil/postgres.go:19-24`), so parallel slices do not share a DB, but
  each package run costs a container start under load 30-50.

## What to do

### Q1 — the rule and its application

Rule: **signature follows the callers.**

| Callers | Action | Dup cost |
|---|---|---|
| all pass a row-derived teacher | change param to `Anchor`, route through `anchored(ctx, a)` | none |
| mixed, and the method is a WRITE (gate already settled by a `*ForWrite`/`Lock*` read) | keep one method on `Anchor`; the `sc` caller passes `sc.AnchorTo(row.TeacherID)` | none |
| mixed, and the method is a caller-dependent READ or an entry point with its own gate | exported `X(ctx, sc, …)` = gate/resolve with `sc` + call unexported core `x(ctx, a, …)`; export `XAnchored` only if another package needs it | one-line wrapper |
| all callers are real callers and semantics depend on rights | keep `Scope` | none |

Applied:

- **billing.** `ComputePeriod(ctx, repo, sc, periodID)` keeps `GetPeriod(sc)`
  then calls `computePeriod(ctx, repo, sc.AnchorTo(period.TeacherID), period)`;
  Close calls `computePeriod` with the locked period (drops a redundant
  GetPeriod). `DraftPeriod(ctx, repo, a Anchor, …)`. `RecalcInvoiceTotals(ctx,
  a, id)`; `adjustment.go:81` passes `sc.AnchorTo(inv.TeacherID)` (inv came
  from `GetInvoiceForWrite`). `EnsurePeriod(ctx, sc, y, m)` →
  `s.ensurePeriod(ctx, sc.Self(), y, m)`; `adjustment.go:476/482` call the
  core. `SessionMeta(ctx, sc, id) (…, Anchor, error)` via `centerScoped` (D5).
  `LiveBillableCounts` builds `authctx.Anchor{period.TeacherID, period.CenterID}`.
  The remaining period/invoice methods flip to `Anchor` + `anchored`/
  `invoiceAnchored` helpers (replace the `(? OR teacher_id = ?)` SQL trick with
  a plain `teacher_id = ?` bind).
- **attendance.** `TallyByEnrollment(ctx, a Anchor, from, to)`; `readNarrowTally`
  becomes `anchoredTally(q, a)` = the enrollments join on `a.TeacherID`, no
  view_all branch. `attendance/service.go:215` consumes the Anchor SessionMeta
  now returns (via billing.ReconcileSession — signature of ReconcileSession
  itself stays `(ctx, sc, sessionID)`).
- **sessions.** `PendingSource.UnconfirmedInWindowAnchored(ctx, a Anchor, …)`;
  repo `ListPendingAnchored` shares the private query builder in
  `pending.go:92` with `ListPending` (parameterise `base` on the scoped query).
  Today's `readScopedFeed(fake)` is already pure own-rows, so no behaviour delta.
- **enrollments.** `ownScope` → `sc.Self()` for `ClassDefaultPrice`
  (`repository.go:278` uses `scoped`, own-rows for members — semantics kept).
  Billing's `EnrollmentSource.ActiveOn`: today `readScoped(fake)` =
  `teacher_id = X OR stint(X)`. Recommended: replace with a **class-keyed**
  `ActiveOnClass(ctx, sc, classID, on)` under a `centerScoped(ctx, sc)` helper
  (plan category 3), passing the real caller `sc` — the roster of a class does
  not depend on who asks, and the caller already passed attendance's write
  gate on that session. In the only branch that uses it (`adjustment.go:331-343`)
  the result is filtered to `e.ID == sessionEnrollmentID`, so the superset is
  harmless and strictly more correct after a handoff. Fallback if the owner
  wants zero semantic movement: `ActiveOnAnchored(ctx, a, …)` and document the
  dropped stint branch.
- **payments.** `ResolveContactAnchor(ctx, sc, contactID) (Anchor, bool, error)`
  returning `sc.AnchorTo(rows[0].TeacherID)`. Reversal paths:
  `sc.AnchorTo(payment.TeacherID)` (LockPayment already center-filtered).
  `recalcTouched(ctx, a, …)`. `CandidateInvoices`, `InvoicesByIDs`,
  `AllocationsByPayment`, `DeleteAllocations`, `MarkReversed`,
  `RecalcInvoicePaid` → `Anchor`. Open question 3 (candidacy by contact+center):
  keep `i.teacher_id = a.TeacherID` — it holds by construction while contacts
  anchor to the owner; record that as the decision and do not bundle a
  candidacy re-key into this phase.
- **statements.** All period-scoped methods → `Anchor`; `TargetContacts(ctx, a
  Anchor, viewer Scope, …)`. `RenderPublic` and `touchView` build
  `authctx.Anchor{stmt.TeacherID, stmt.CenterID}`; `TouchView(ctx, a, id)`.
  `UpsertStatement` stamps from `a` (`repository.go:472-473`).
- **notifications.** Rename `runsOwnScoped` → `anchored(ctx, a)`; `RunStore`
  methods take `Anchor`; `runJob.anchor()` literal; `service.go:222` →
  `sc.AnchorTo(stale.TeacherID)`; `:635` → `sc.AnchorTo(run.TeacherID)`;
  `:763/:796` → `sc.Self()`.
- **imports / classes / contacts / students / enrollments (E).**
  `anchorFor` → returns `Anchor` (literal is fine; `owner.CenterID` comes from
  the OwnerAnchor). classes: `FindActiveByName(ctx, a, …)`, `ScheduleExists(ctx,
  a, …)` (import-only → in place); `Create(ctx, sc, req)` =
  `CreateAnchored(ctx, sc.Self(), req)` (Create already stamps `sc.TeacherID`,
  `service.go:61`); `AddSchedule(sc)` and `AddScheduleAnchored(a)` share a
  private `addSchedule(class, req)` core after their own `GetByID`.
  contacts: `Create(sc)` keeps the `IsOwner` gate and calls a private
  `create(ctx, teacherID, centerID, req)`; `CreateAnchored(ctx, oa OwnerAnchor,
  req)` calls the same core — no centers import needed in contacts.
  `FindIDByPhone(ctx, oa OwnerAnchor, phone)` via `centerScoped`. students:
  same shape; `CreateAnchored` does a center-keyed contact check.
  enrollments: `CreateAnchored(ctx, actor Scope, a Anchor, req)` with
  `StudentEnrolled.ActorID = actor.TeacherID`; `Create(sc)` =
  `CreateAnchored(ctx, sc, sc.Self(), req)`. `FindByStudentAndClass`: check
  whether its own caller (`service.go:269`) means the center-wide natural key;
  if yes, make it `centerScoped` and let imports pass the importer's real `sc`
  (one fewer anchored entry point); otherwise `…Anchored`.
  `MemberDirectory` gains `ResolveOwnerAnchor(ctx, sc) (OwnerAnchor, error)`
  implemented in centers; delete the `IsOwner: true` literal.

Naming: use the `Anchored` suffix the plan already uses, not `For`. It greps
cleanly and a later linter can assert "`*Anchored` takes `Anchor`".

### Q2 — dashboard: NO-GO, record this

Keep `targetScope` as a Scope literal inside `centers/dashboard.go` (already in
the allowlist), rename to `viewAs(sc, teacherID)`, and write the doc comment as:
"a rights-less Scope impersonating T's own read view; it can never widen
anything because IsOwner=false and Perms=nil; it is Scope, not Anchor, because
the consumed reads are stint-based." Reasons to record in the plan: (1)
semantics are caller-view, not row-anchor — Anchor would change what the owner
sees; (2) read-only and gated by `dashboard.view` + `WasEverMember`, so no
escalation surface; (3) cascade cost is ~8 read methods across classes,
sessions, enrollments, attendance, which trips the Risk clause on its own; (4)
centers is the designated scope-resolution home, so the guard already tolerates
it. Step 8's dashboard half is superseded; its enrollments half (`sc.Self()`)
stands.

### Q3 — Anchor literals and the guard

Allow `authctx.Anchor{…}` literals everywhere. Do not add `NewAnchor`. Prefer
`sc.AnchorTo(x)` when an `sc` is in hand (it makes "CenterID is always the
caller's" structural); use the literal only where there is no caller:
`RenderPublic`, `touchView`, `runJob.anchor`, `LiveBillableCounts`,
`SessionMeta`'s return.

What keeps the invariant meaningful: Scope construction is monopolised (centers,
middleware, testutil, tests); Anchor has no capability fields; no
Anchor→Scope function exists; repositories route Anchor only through
`anchored()`. Guard changes:

- Use `go/ast` (the file already parses AST in
  `TestRepositoriesWidenWritesThroughWriteWideOnly`): resolve the import name
  bound to `…/shared/authctx` per file, flag `CompositeLit` of `<name>.Scope`
  with `len(Elts) > 0`, and `CompositeLit` of `<name>.OwnerAnchor` at all. This
  closes the multi-line-literal and alias bypasses in one move.
- Flag `AssignStmt` whose LHS is a `SelectorExpr` with `Sel` in {`IsOwner`,
  `Perms`, `CanSendReports`} outside the allowlist.
- Keep the `MintOwnerAnchor(` check.
- Residual gap to log for phase 4 (types needed): `s := sc; s.TeacherID = other`
  keeps the caller's rights on another teacher's rows. grep cannot separate it
  from `stmt.TeacherID = …`. Note it in the guard's doc comment.

### Q4 — Record's allocation read-back

Add `AllocationsOf(ctx, a Anchor, paymentID) ([]AllocationRow, error)` reusing
`allocationRowSelect`; Record calls it with the contact anchor. Keep
`ListAllocations(ctx, sc, …)` for Get/List. This does not touch open question 1
(member cannot later Get the payment); it only keeps Record's response
truthful. If the owner later restricts D9, the fix is at the route gate, not here.

### Q5 — slicing

Keep the four slices with these adjustments:

- **Lead first:** `authctx` gets all of `Anchor`, `AnchorTo`, `Self`,
  `OwnerAnchor`, `MintOwnerAnchor` plus unit tests, and the guard test is
  landed in its final form (red). Nothing else compiles-breaks on this step.
- **B** (billing, attendance, sessions, enrollments): owns `attendance/service.go:215`
  and the billing consumer interfaces. B must not touch classes even though
  sessions imports it. B's enrollments work = `Self()`, `ActiveOnClass`/
  `centerScoped`. Leave `CreateAnchored`/`FindByStudentAndClass` to E.
- **C** (payments) and **D** (statements, notifications): fully independent of B
  (verified with `go list`). Both are small; under load 30-50 consider running
  them as one sequential agent to halve container churn — three concurrent
  testcontainers plus B's four packages is where timeouts will come from, not
  CPU in the tests themselves.
- **E** after B: imports, classes, contacts, students, centers (`viewAs`,
  `ResolveOwnerAnchor`). E is the only slice that touches enrollments after B,
  and the only one that touches centers.
- **Lead at the end:** guard green, `make test-api`, `make lint-api`,
  `docs/api-guidelines.md` Tenancy "Scope vs Anchor" (use
  `TargetContacts(ctx, a Anchor, viewer Scope, …)` as the example).
- Tell every subagent: run only `go test -tags=integration -p 1 ./internal/features/<pkg>/...`
  for its own packages; never `./...`, never the features-level guard, never
  `make lint-api`.

## What to avoid

- Any `func (a Anchor) Scope()` / `asScope()` helper, even package-private. Its
  output is rights-less so it cannot escalate, but it lets a reviewer no longer
  tell at a signature whether widening applies — the exact confusion this phase
  exists to remove.
- Anchoring caller-dependent reads (dashboard, `ActiveOn` for the 5 real
  callers, `ListAllocations` for Get/List).
- Duplicating logic instead of wrapping: every `XAnchored` must share the
  query/core with `X`; if the bodies diverge, the split is wrong.
- Re-keying payment candidacy (open question 3) inside this phase.
- Letting subagents "fix" the red guard by widening the allowlist.

## Alternatives considered

- **Bridge type with no widen (`authctx.Actor` interface implemented by both):**
  smaller diff, but collapses the two meanings again at every call site; rejected.
- **Anchor the dashboard anyway for uniformity:** costs ~8 read variants and
  changes owner visibility (drops staffed-but-not-owned classes); no security gain.
- **`ActiveOnAnchored` instead of `ActiveOnClass`:** same size, zero semantic
  movement, but silently loses post-handoff enrollment rows in the
  `!hasSessionLine` branch; acceptable fallback if the owner prefers no
  behaviour change in phase 2.

## Work checklist

1. Lead: authctx symbols + tests; AST guard (red, 22 hits + OwnerAnchor check).
2. B: billing `anchored`/`invoiceAnchored`/`centerScoped`, `computePeriod`,
   `ensurePeriod`, `SessionMeta`→Anchor; attendance `TallyByEnrollment`;
   sessions `ListPendingAnchored`/`UnconfirmedInWindowAnchored`; enrollments
   `Self()` + `ActiveOnClass`; tests: assistant confirm → adjustment, no owner
   expectation changes.
3. C: `ResolveContactAnchor`, reversal anchors, `AllocationsOf`; test: member
   with `payments.create` only gets non-empty allocations in Record's response.
4. D: statements Anchor methods + public render literals; notifications
   `anchored`, `RunStore` on Anchor, `Self()` at 763/796.
5. E: `ResolveOwnerAnchor`, `CreateAnchored` ×3, classes anchored lookups,
   `viewAs`, delete `IsOwner: true`; tests: import by member anchors
   contacts/students to owner, audit actor = importer.
6. Lead: guard green, full test/lint, docs.

## Success metrics

- Guard passes; outside centers/middleware/testutil/tests there are zero Scope
  literals with fields, zero `OwnerAnchor{`, zero `MintOwnerAnchor(`.
- No handler file in the diff; non-test diff ≤ ~38 files.
- Existing owner-path tests unchanged; the three new tests (assistant
  adjustment, import actor, payments.create Record response) green.
- Every `*Anchored` method body is a wrapper or shares a core with its Scope
  sibling (review check).

## Assumptions

- `FindByStudentAndClass`'s own caller means the center-wide natural key
  (medium). If not, use `…Anchored`.
- Owner accepts the `ActiveOnClass` superset semantics for reconciliation
  (medium). Evidence that flips it: owner explicitly wants no behaviour change
  in phase 2 → use `ActiveOnAnchored`.
- Contacts continue to anchor to the owner, so payment candidacy on
  `a.TeacherID` stays correct (high; `contacts/service.go:38-44`).
- Test containers per package are the bottleneck under load, not test CPU
  (high; `testutil/postgres.go:19-24`).
- Guard allowlist unchanged: `features/centers/`, `middleware/`, `testutil/`,
  `*_test.go` (high).

Status: DONE_WITH_CONCERNS
Summary: Adopt "signature follows the callers" (in-place Anchor for anchor-only
methods, thin wrappers for mixed ones, class/center-keyed queries where the
teacher is irrelevant); keep the dashboard on Scope; allow Anchor literals;
anchored read-back for Record; keep the four slices with C and D possibly
merged under load.
Concerns: (1) pure `anchored()` drops the stint OR-branch that
`enrollments.readScoped` gives today's fake scope — decide `ActiveOnClass` vs
`ActiveOnAnchored` before B starts; (2) the plan's regex guard misses
multi-line literals and `OwnerAnchor{}` — switch to AST; (3) `.TeacherID =` on
a Scope copy stays unguarded until phase 4.
