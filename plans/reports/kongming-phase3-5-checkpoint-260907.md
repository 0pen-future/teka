# Kongming counsel — Phase 3/5 checkpoint: go/no-go Phase 4, next risk, analyzer design

Date: 2026-09-07. Plan: `plans/260906-0627-authz-write-scope-root-cause/`.
Advisory only. Every load-bearing claim below was checked against the
uncommitted working tree in `apps/api` (file:line), the phase files, the two
reviews, and the previous checkpoint; a syntactic census of repository methods
was run with a throwaway go/ast scanner (numbers quoted are from that run).
Runs on `fable`.

## TL;DR

1. **GO for Phase 4 — on one hard condition: commit the tree first.** The
   previous checkpoint asked for this before Phase 3; it was not done. The
   tree is now 86 modified + 22 untracked files (+4222/−1724) on `master`.
   Phase 4 adds `tools/` and touches every repository file (directives);
   without a commit boundary an analyzer rollout problem becomes inseparable
   from the authz semantics change. Substance of phases 3/5 + H-1 is sound:
   spot-checks found no read path left on `ReportsOversight()`, no write
   widened by a read key, no routespec entry disagreeing with its handler,
   and the guard/routespec/authctx tests are green now.
2. **Next risk before merge is not code, it is a big-bang merge with three
   owner decisions still open** (D9 full version, OQ6 `students.Create`,
   the D6/L4 read widening) and a rollout section that still assumes Phase 1
   deployed alone. Collapse the three into one owner question, commit per
   phase, re-run the prod inventory on deploy day.
3. **Phase 4 as specced cannot land in 1d or under 8 directives.** Census:
   221 scope-taking repository methods, 54 scoping helpers by shape, **78
   scope-taking non-helper methods that build the query from a raw root
   inline**, 103 non-scope methods on raw roots. The spec's R1 ("every
   `*gorm.DB` chain originates from a helper") + R2 ("raw root only in
   helpers") means ~180 violations. Recommend the coordinator's option (b):
   one type-based rule, **"scope witness"** — a scope-taking method that
   touches a raw root must either call a shape-matched helper or reference
   `<scopeParam>.CenterID`. Expected directives on today's tree: 0–1.
4. **Identify helpers by type shape, not by name, allowlist, or marker**:
   unexported method on the repository receiver, returns `*gorm.DB`, has a
   `Scope`/`Anchor`/`OwnerAnchor` parameter. That matches all 54 helpers in
   13 packages (`readScoped`, `scopedRead`, `invoiceCenterScoped`,
   `contactBalanceQuery`, `readNarrow(q, sc, col)`, …) with zero config.
5. **Replace all three AST guards, do not "shrink to smoke"**, and make the
   analyzer self-enforcing under `go test ./...` via a tree-run test using
   `x/tools/go/analysis/checker` (present in v0.49.0). Reason: CI's lint
   job runs `golangci-lint-action` directly and the test job runs
   `make test-api`, which does not depend on `test-api-unit` — a Makefile-only
   hook would never execute in CI.

## Reframed problem

The real decision is not "is the analyzer worth building" (yes — it is the
only durable replacement for a grep guard two agents already stepped around,
review C1) but **what invariant the analyzer can check on this codebase as it
actually is**: 70+ repository methods bind `center_id` inline in raw SQL
(LATERAL joins, `Raw`, `Exec`) and the plan's own rule forbids adding Scope
params or helpers merely to satisfy a linter. The invariant that fits the
code is "a tenant-scoped method visibly binds the tenant" — not "every query
flows through a named helper". Non-goals: converting inline queries, dataflow
over `*gorm.DB` chains, RLS, services (D3 exception lives in a service),
routespec.

## Verified evidence

Phase 2 checkpoint corrections — honoured or deliberately deviated:

| Item | State in tree |
|---|---|
| `statements/service.go:347` (`resp.URL`) stays on send axis | honoured (`:347` still `ReportsOversight()`) |
| `contacts/repository.go` `scopedMappingWrite` stays send axis | honoured (`:113`) |
| `ZaloMappings` re-keyed center + `contacts.view_all` OR stint | honoured (`notifications/repository.go:239-240`, `contactsReadWide`) |
| Implied keys inside `BuildPermSet` after deny | honoured (`authctx/permissions.go:92-99`); 3 resolvers (`centers/service.go:73`, `testutil/fixtures.go:195`, `seeds/seed.go:346`) all go through it; `EffectiveKeys` reads `Perms` (`permissions.go:127-135`) |
| `MarkSent` count reachable → 404, distinct ids | honoured (`notifications/repository.go:307-320`) |
| Parity 3 principals at HTTP | `TestReportsSendImpliedKeysParity` (`server/policy_integration_test.go`) |
| H-1: center-keyed candidacy, `sc` + `invoiceCenterScoped` | honoured (`payments/repository.go:186-204`); `CandidateInvoices`/`RecalcInvoicePaid` are raw SQL binding `sc.CenterID` |
| `students.Create` `IsOwner` gate | **not applied**; lead escalated to OQ6 because tests (`students/integration_test.go:165,:282`) assert the opposite of docs. Legitimate deviation — owner call, must be answered before merge |
| Commit the tree before Phase 3 | **not done** |

Spot-checks (Q1):

- `ReportsOversight()` non-test hits: definition + 7 sites (`statements/service.go:110,347`, `contacts/repository.go:113`, `notifications/service.go:129,398,499,678`). Read each: all create or gate a send. No read left on it.
- `CenterWideFor(` in repository files: every site sits in a read-named function or in `readWide`/`contactsReadWide`/`readNarrow*` predicates; `TestRepositoriesWidenWritesThroughWriteWideOnly` green (ran `go test -short -run 'TestRepositories|TestScopeLiterals' ./internal/features/`). Service-level sites (`sessions/service.go:211` = D3, `enrollments/service.go:134`, `classstaff/service.go:49`, `billing/service.go:132`) are reads.
- `WriteWide()` sites (15) all in `writeScoped`/`scoped` write helpers or `periodStatus` write arm (`statements/repository.go:319`).
- routespec vs handlers: `POST /api/v1/payments` → `payments.create` + `payment.create` audit (`routespec.go:318`); `POST /api/v1/notifications/mark-sent` → `notifications.mark_sent` (`:347`, handler `notifications/handler.go:226`); `POST /api/v1/statements/:id/revoke` → `statements.revoke` (`:332`); `PUT|DELETE /api/v1/contacts/:id/zalo-mapping` → `contacts.link_zalo` (`:238-240`); `POST /api/v1/enrollments` = `SourceService`, `GET` = `none` (`:256-258`) — consistent with `TestSamePathSameAuditSource` (mutating only) and `TestSkipSourceNeverSharesPathWithAuditedRoute`, and identical to HEAD's path-only skip. The reviewer independently diffed the three HEAD tables against the manifest (0 drift), so any Kind/Key-vs-handler disagreement would be pre-existing, not Phase 5's.
- `go build ./...` OK; `go vet` on `routespec`, `authctx` OK; `go test -short` on `routespec`, `authctx`, `features` guards OK.
- Zero hits for `.Session(`, `NewDB`, `.Unscoped()` in `internal/features` non-test files. `.Table(` is used legitimately (`payments.invoiceCenterScoped`, `collections.PeriodExists`, `billing.SessionMeta`).
- `*Anchored` cross-package callers: `imports/apply.go:143,174,202,243,261,285` (through imports' own interfaces `imports/service.go:52-78`) **and** `billing/close.go:86,112` → `sessions.ListUnconfirmedInWindowAnchored` through billing's `PendingLister` interface (`close.go:32`). The carry-over premise "only classes/enrollments entry points + imports" is already false for the tree.
- Scope copy-mutation: no `x := sc; x.TeacherID =` in non-test code. The one `.TeacherID =` hit is `statements/repository.go:474` `stmt.TeacherID = a.TeacherID` (a Statement, not a Scope) — a string guard would false-positive here; a typed rule will not.
- Census (scope-taking, non-helper, raw root): 78 methods; **only 2 lack a `<param>.CenterID` reference**: `billing.GetInvoiceWithLines` (second query binds row-derived `inv.CenterID` after an `invoiceAnchored` chain, `repository.go` "defense-in-depth belt" comment) and `billing.SessionMeta` (narrows through `r.centerScoped(q, sc, "class_sessions.center_id")`, a narrowing helper by shape). Only `collections.PeriodSummary` has more than one raw root (3) among scope-taking non-helper methods; every other one has exactly one.
- `x/tools v0.49.0` is already required indirectly (`go mod why`: `docs → swaggo/swag → x/tools/go/loader`); `go.sum` has both `h1:` lines. `go/analysis/checker` exists at v0.49.0 (`Analyze(analyzers, pkgs, opts)`, needs `packages.LoadAllSyntax`). `analysistest.Run` at v0.49.0 supports a `go.mod` in testdata (module mode) or `testdata/src` GOPATH layout.
- CI (`.github/workflows/api-ci.yml`): lint job = `golangci-lint-action@v9` directly; test job = `make test-api`; `test-api` (Makefile:61) does not depend on `test-api-unit` (Makefile:68). `.golangci.yml` `default: none` + 8 linters; no custom-analyzer plugin.

## Q1 — Go/no-go for Phase 4

**GO**, conditional on:

1. Branch `authz-write-scope` off master; commit now. Try `git add -p` per phase (1: repos + guard; 2: Anchor + imports; H-1: `payments/`; 3: authctx implied + reads + docs; 5: `routespec` + server/audit/middleware). If hunks do not separate cleanly, one commit per file-group is still better than none. No push (needs user approval per repo convention).
2. Record two accepted-but-undocumented widenings in `plan.md` D6 so the merge is honest: `zalo.MatchFriendsScoped` now admits a direct `contacts.view_all` holder (review L4, phone egress); collections' `unallocated_credit` under `billing.view_all` (already OQ7 — fine).
3. Optional 3-line hardening while in `authctx`: a unit test that no value in `impliedKeys` is itself a key of `impliedKeys` — `BuildPermSet` iterates a map, so a chained implication would be order-dependent. Not blocking.

No hole found that blocks Phase 4. Nothing in Phase 4 depends on OQ6 or D9.

## Q2 — Next risk to watch before merge

**A big-bang authz merge with open owner decisions and no commit boundaries.**
Rollout §1 of `plan.md` still says Phase 1 ships alone as a hotfix, then
inventory, then owner confirms D9; in reality phases 1–3, H-1 and 5 are one
uncommitted tree. Concretely:

- Ask the owner one message with three questions: (a) D9 full form — any member with `payments.create` records payments for any contact; only the owner can reverse/reallocate; a member without `payments.view_all` cannot read back the payment they recorded; (b) OQ6 — `students.Create` owner-only (docs) or member-allowed (tests); (c) D6 — `reports.send`-only members now read billing periods list, statements list, notifications list of every teacher, and `contacts.view_all` holders can run Zalo friend matching. Do not merge before (a) and (b) are answered; (c) is informational.
- Re-run `plans/reports/inventory-260906-prod-view-all-write-exposure.md` queries on deploy day; grants may have changed since 09-06.
- Post-deploy signals: invoices with `teacher_id <> owner` moving to `paid`/`partially_paid` after owner records a payment (H-1 proof); 404 rate on `POST /notifications/mark-sent` (was silent no-op, now 404 for rows the sender does not own); size/shape of `permissions` in the center-context payload for `reports.send` members (web `member-permissions-dialog.tsx` shows implied keys as absent — display only, already a recorded follow-up).

## Q3 — Phase 4 analyzer design

### Coordinator's question: which of (a)–(d)

**(b), with (a) folded in.** One recommendation, one rule set:

- **R1 "scope witness"** (replaces spec R1 + R2): in the analysed file set (same set as `repositoryPaths` today: `features/*/repository.go` plus siblings declaring `*gormRepository` methods, `centers/` exempt), every method with a parameter of type `authctx.Scope`, `authctx.Anchor` or `authctx.OwnerAnchor` that contains a raw root — a call to `database.FromContext` or a read of a receiver field of type `*gorm.DB`, both by `types.Object` — must contain at least one witness: (i) a call to a shape-matched helper (below), or (ii) a selector `<scopeParam>.CenterID`. Methods without a scope parameter are out of scope, by design: they insert or update a struct the service built, or look up by a token/id the service already gated (auth, invitations, teachers, zalo, centers, `Create(ctx, *Model)`), and the plan forbids adding a Scope param just to please the linter. Say this in the doc comment; RLS is the backstop for that class (D7).
- **R2 "no reset"** (spec R2b minus `.Table(`): `.Session(`, `gorm.Session{NewDB: true}`, `.Unscoped()` on a `*gorm.DB` anywhere in non-test `internal/features` files. Zero hits today; cheap. `.Table(` is legitimate table selection, not a reset — drop it.
- **R3 "authority stays in authctx"** (port of the three guards, type-based): in repository files, ban reading field `Scope.IsOwner`, calling `PermSet.Has`, `Scope.CenterWide`, `StaffRolesFor`, `StaffRoleCan` (by object); `Scope.CenterWideFor` only inside a function whose lowercase name contains `read` (keep this one name rule — it is the read/write axis, which the type system cannot express, and every current helper already satisfies it). Outside `centers/`, `middleware/`, `testutil/`, `authctx/`, `seeds/`: composite literal of `authctx.Scope` with elements, any assignment to a field of a `Scope`-typed value (this is the `s := sc; s.TeacherID = x` carry-over — the LHS type check is what makes it precise), any `OwnerAnchor` literal, and `MintOwnerAnchor` calls outside `centers/`. Alias-import ban becomes unnecessary (objects, not names) — drop it.
- **Directive** `//scopelint:unscoped <reason>` on the line above a method exempts it from R1 only. Budget ≤8 stays; expected use on today's tree: 0 (if a helper call anywhere in the method counts as witness, `GetInvoiceWithLines` passes via `invoiceAnchored`; `SessionMeta` passes via `centerScoped`) or 1.

Replacement acceptance criteria for the phase file:

- [ ] `analysistest` per rule: R1 flags (raw root, no helper, no `CenterID`), (raw root + `sc.TeacherID` only); passes (helper root), (raw `Where("center_id = ?", sc.CenterID)`), (narrowing helper `centerScoped(q, sc, col)`), (`Raw(sql, a.CenterID)`), (directive'd method). R2 flags `.Session(&gorm.Session{NewDB: true})`, `.Unscoped()`. R3 flags `sc.IsOwner`, `.Has(`, `CenterWideFor` in a non-read-named method, `authctx.Scope{TeacherID: x}`, `s := sc; s.TeacherID = x`, `MintOwnerAnchor` outside centers; and does not flag the same in an exempt path.
- [ ] Tree run: 0 diagnostics over `./internal/...` with ≤1 directive (named in the PR); injecting `func (r *gormRepository) X(ctx context.Context, sc authctx.Scope, id uuid.UUID) error { return database.FromContext(ctx, r.db).Where("id = ?", id).Delete(&Period{}).Error }` into `billing/repository.go` yields exactly one diagnostic.
- [ ] Runs under plain `go test ./...` (tree-run test in `tools/scopelint`) — therefore under `make test-api-unit`, `make test-api` and CI with no workflow change; `make scopelint` gives standalone output. Wall time < 8s warm (5s was the spec's number; a go/packages load of ~40 packages is the floor — measure, do not `-short`-skip).
- [ ] `apps/api/internal/features/scoping_guard_test.go` deleted; `docs/api-guidelines.md` Tenancy explains the witness rule and the directive in one paragraph; `apps/api/CLAUDE.md` one line.
- [ ] Explicit non-goals in the phase file: non-scope methods, per-chain dataflow, services, raw-SQL string inspection.

### (a) Identifying helpers across packages

**Type shape, no config.** A helper is an unexported method on the repository receiver type that returns `*gorm.DB` and has at least one `Scope`/`Anchor`/`OwnerAnchor` parameter. Two shapes fall out naturally: root helpers (`readScoped(ctx, sc)`) and narrowing helpers (`readNarrow(q, sc, col)`, `billing.centerScoped(q, sc, col)`, `students.readNarrowContacts`, `statements.withContact`). Census: 54 methods match across attendance, billing, classes, collections (`contactBalanceQuery`, `classCollectionsQuery`), contacts (`scoped`, `scopedRead`, `scopedMappingWrite`, `withStudentCount`), enrollments, notifications, payments (`invoiceCenterScoped`, `allocationAnchored`), sessions, statements, students — every name the spec's allowlist would have missed is covered. The two shape matches without a scope param (`payments.allocationsQuery(ctx, centerID, paymentID)`, `teachers.scoped(ctx, teacherID)`) are excluded by the param requirement, correctly.

Why not the others: a per-package name allowlist in code is the thing that drifts (the spec's list already omits eight of today's names); a naming convention regex is what the spec itself forbids and names are inconsistent (`readScoped` vs `scopedRead`); a marker comment is a second directive with no compiler backing and can be pasted onto anything. The shape is the marker.

Helpers themselves get one extra check under R1: a root helper must reference `<param>.CenterID` (all 54 do except the 6 narrowing helpers, which take a `*gorm.DB` and are exempt by shape).

### (b) The three guard tests

Replace, not shrink. All three port into R3 type-based (and become alias-proof and false-positive-proof — see `statements/repository.go:474`). Delete `scoping_guard_test.go` in the same PR as the tree-run test; a "smoke test that the Makefile mentions scopelint" is theatre and will be the next thing an agent steps around. If the lead insists on Makefile-only wiring, keep the file untouched until CI proves the Makefile path, then delete it — never a half-version.

### (c) Carry-overs and the directive budget

- Scope field assignment → **in the analyzer**, ~15 lines, type-based, real (it is the one bypass the AST guard admits it cannot see).
- `*Anchored` call-site allowlist → **cannot be type-based**: every cross-package caller goes through the caller's own interface (`imports/service.go:52-78`, `billing/close.go:32`), so the callee `types.Object` is a local interface method, not `classes.Service.CreateAnchored`. Any rule here is name-based. Also the premise is already false (billing → sessions `ListUnconfirmedInWindowAnchored`). Options: a 5-line name rule on selectors `CreateAnchored`/`AddScheduleAnchored` exactly (the three entry points) allowed from same package or `features/imports`, doc-commented as name-based; or leave it to review. Include only if it costs nothing in the same pass; do not generalise to "any exported method with an `Anchor` param".
- Budget ≤8 is realistic only under R1 "witness"; under the spec's R2 it is ~180. Expected: 0–1.

### (d) Wiring pitfalls

1. **CI never runs a Makefile-only hook.** Lint job = `golangci-lint-action` (not `make lint-api`); test job = `make test-api`, which does not call `test-api-unit`. Either add `test-api: scopelint` as a prerequisite or, preferred, make the analyzer self-enforcing as a Go test: `tools/scopelint/scopelint/tree_test.go` loads `teka/apps/api/internal/...` with `packages.Load(&packages.Config{Mode: packages.LoadAllSyntax})` and runs `checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)`, failing on any diagnostic. Keep `make scopelint` (`go run ./tools/scopelint ./internal/...`) for readable output and hook it into `lint-api` for humans. Reason to prefer the test: agents in this repo demonstrably run `go test` on single packages and skip make targets (review C1 history); a rule that lives under `go test ./...` survives that.
2. **go.mod**: importing `go/analysis` makes `x/tools` direct; `go mod tidy` drops `// indirect` on one line, no version bump (v0.49.0 already resolved), `go.sum` already carries both hashes; tidy may add `x/mod`/`x/sync` sums if not present — commit go.mod/go.sum together (setup-go cache keys on go.sum).
3. **analysistest needs stub packages at the real import paths** because the analyzer matches `types.Object` by full path: `testdata/src/gorm.io/gorm` (type `DB` with `Where/Table/Session/Unscoped/Find` stubs, `type Session struct{ NewDB bool }`), `testdata/src/teka/apps/api/internal/shared/authctx` (`Scope`, `Anchor`, `OwnerAnchor`, `PermSet`, `MintOwnerAnchor`), `testdata/src/teka/apps/api/internal/shared/database` (`FromContext`), plus `testdata/src/a` for cases. GOPATH layout, no `go.mod` in testdata, nothing to fetch. Do not point testdata at the real `authctx` (module resolution inside analysistest is the classic time sink).
4. **Coverage**: `-coverpkg=./...` now counts `tools/scopelint`; `analyzer.go` is covered by its own tests, `main.go` (3 lines) is not — noise against 77.5%.
5. golangci-lint will lint `tools/` (revive wants a doc comment on exported `Analyzer`; gosec is quiet on this kind of code).
6. Skip `go vet -vettool` and any golangci plugin build; the phase file's risk note is right.
7. If the tree-run test exceeds the 5s target, accept it (it is a package load, not a test suite); do not gate it behind `-short`.

## Q4 — What the lead must NOT do in Phase 4

- Convert the ~70 inline-binding methods to helpers "so the linter passes" — regression surface in grading/teaching/classstaff that phases 1–3 never touched; the plan's own rule.
- Add Scope params to non-scope methods (`attendance.UpsertMany`, `auth.*`, `invitations.*`) to make them "checkable".
- Build per-chain dataflow (`go/ssa`, def-use over `*gorm.DB` locals): one multi-root method exists (`collections.PeriodSummary`); not worth it now — note as a hardening follow-up.
- Extend the analyzer to services (the D3 exception at `sessions/service.go:211` would need a directive and the rule would drift into authorization logic).
- Rename helpers to a uniform convention across packages — churn on files just reviewed; the shape rule makes it unnecessary.
- Inspect raw SQL strings for `center_id` — string matching, forbidden by the spec; RLS backstop.
- Resolve OQ6 or D9 inside the analyzer or its testdata.
- Delete the guard tests before the tree-run test is green in CI.
- Touch `routespec` or route policy.
- Start Phase 4 on the uncommitted tree.

## Alternatives & trade-offs

- **Spec as written (per-chain origin from allowlisted helpers)**: strongest guarantee, but requires converting 78 methods and adding ~100 directives or exemptions; multiple days, high regression risk, and the name allowlist still drifts. Rejected.
- **Option (d), report-only mode**: an analyzer that does not fail the build is a dashboard nobody reads; the C1 episode shows even failing guards get stepped around. Rejected unless (b) turns out to produce > 8 real violations — evidence says 0–1.
- **Option (c), raise the budget**: only masks that R2 is the wrong invariant for this codebase. Rejected.
- **Makefile-only wiring** vs tree-run test: simpler to write, but never executes in CI as configured and is bypassed by direct `go test`. If chosen, `test-api: scopelint` is mandatory.

## Work checklist

1. Branch + commit (per phase where separable). Owner message with D9/OQ6/D6 questions.
2. `tools/scopelint/scopelint/analyzer.go`: R1 witness (helper-by-shape + `CenterID` selector), R2 reset ban, R3 authority rules + Scope field assignment; directive parsing on the method's doc comment.
3. `analysistest` with stub packages at real import paths; cases per the replacement criteria.
4. `tree_test.go` (`packages.Load` + `checker.Analyze`), `main.go` (`singlechecker.Main`), `make scopelint`, hook into `lint-api`; `go mod tidy`.
5. Run on the tree; name the 0–1 directives with one-sentence reasons; inject the negative probe into billing and confirm one diagnostic.
6. Delete `scoping_guard_test.go`; docs one paragraph; `apps/api/CLAUDE.md` one line; update the phase file's Requirements/Success Criteria to the replacement set above.
7. `go test -short ./tools/... ./internal/features/`, `make lint-api`; tester for full suite.

## Success metrics

- Analyzer green on the tree with ≤1 directive; the billing probe fails `go test ./tools/...` and `make test-api-unit` alike.
- CI green with no workflow change; wall time of `tools/scopelint` tests < 8s.
- `scoping_guard_test.go` gone; `grep -rn 'scopelint:unscoped' apps/api/internal` ≤ 1.
- After merge: owner's three answers recorded in `plan.md`; inventory re-run dated deploy day.

## Assumptions

- The lead can commit locally on a branch without user approval (only pushes to master need it) — **high**; if not, ask the user for a branch commit before Phase 4 rather than proceeding.
- `packages.LoadAllSyntax` over `./internal/...` completes in single-digit seconds on the CI runner with warm module cache — **medium**; if it is 15s+, keep it (still cheaper than one integration package) and drop the 5s criterion.
- Non-scope repository methods are all struct-carrying writes or service-gated id/token lookups — **medium-high** (census by package: audit, auth, centers, invitations, teachers, zalo, plus `Create`/`Upsert*`/`Insert*` elsewhere); a cross-tenant id lookup without any scope param would be visible in the interface and is the RLS plan's target.
- Owner will answer D9/OQ6 before the PR — **medium**; if not, merge is blocked by plan rule (user decisions are not silently undone), not by Phase 4.
