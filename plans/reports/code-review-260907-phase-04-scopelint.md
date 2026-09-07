# Code Review: phase 4 scopelint analyzer

Date: 2026-09-07
Reviewer: review-phase4
Spec: `plans/260906-0627-authz-write-scope-root-cause/phase-04-repository-scope-linter.md`
Design counsel: `plans/reports/kongming-phase3-5-checkpoint-260907.md` (lines 111-170)
Implementer report: `plans/reports/phase4-scopelint-260907.md`

**Score: 8/10**

## Scope

Analyzer and tests under `apps/api/tools/scopelint/` (501 lines of analyzer, 2
test files, 6 testdata files), root `Makefile`, `apps/api/go.mod`, deleted
`apps/api/internal/features/scoping_guard_test.go`, `docs/api-guidelines.md`,
`docs/adding-permissions.md`, `apps/api/CLAUDE.md`, two comment-only repository
edits in `grading/repository.go` and `classstaff/repository.go`.

Read-only review. All probes ran against a scratchpad copy of the module; the
working tree was not modified.

## Verification run

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./tools/...` | clean |
| `go test ./tools/scopelint/...` | ok, 3.87s |
| tree test alone, warm | 3.17s |
| `go test -short ./tools/scopelint/...` | runs, not skipped |
| `make scopelint` | 0 diagnostics, 1.5s |
| `golangci-lint run ./tools/...` | 0 issues |
| `grep -rn 'scopelint:unscoped' apps/api/internal` | 0 |
| analyzer package statement coverage | 92.2% |

## (a) Success criteria

All five are met.

Every R1 case named in the criterion has a testdata function in
`apps/api/tools/scopelint/scopelint/testdata/src/teka/apps/api/internal/features/testcase/repository.go`:
`BadNoScope`, `BadTeacherOnly` (flag), `GoodHelper`, `GoodDirectCenterID`,
`GoodNarrowing`, `GoodRawSQL`, `GoodDirective` (pass). R2 has `BadReset` and
`BadUnscoped`, with `GoodTable` proving `.Table(` stays allowed. R3 has
`BadIsOwner`, `BadHas`, `badWrite`, plus `BadScopeLiteral`, `BadScopeAssign`,
`BadMintOwnerAnchor` in `authority.go`, with exempt-path negatives in the
`centers` and `testutil` testdata packages. Two cases beyond the criteria are
welcome: `badRootHelper` (a root helper that never binds `CenterID`) and
`BadBareDirective` (directive with no reason).

The injection probe reproduces exactly. Appending the criterion's method to a
copy of `billing/repository.go` produced one diagnostic and nothing else:

```
internal/features/billing/repository.go:1110:1: ProbeAcceptance builds a query from a raw *gorm.DB root without calling a scoping helper or referencing its scope parameter's CenterID field; scope it, or add //scopelint:unscoped <reason>
```

Tree run is clean with zero directives. Wall time is under the 8s budget. The
guard test is deleted, the Tenancy paragraph exists, and `CLAUDE.md` carries
the one line.

## (b) Parity with the deleted guard

The deleted file made one line-based pass over `*/repository.go`, skipping
`centers`, banning six tokens, and failing hard if the glob matched nothing.

- **`sc.IsOwner` / `scope.IsOwner`** — covered and widened by
  `analyzer.go:383`, which matches any `Scope`-typed expression. A copied scope
  (`s := sc; s.IsOwner`) now flags where the string guard missed it.
- **`.Has(`** — covered by `analyzer.go:396`. `Has` really is a method on
  `Scope` (`internal/shared/authctx/permissions.go:115`), so the object match
  lands. `PermSet.HasKey` escaped both the old guard and the new one; that is
  pre-existing, not a regression.
- **`.CenterWide()`** — moot. No such method exists in `authctx` anymore, only
  `CenterWideFor`. Dropping the ban is correct.
- **`StaffRolesFor` / `StaffRoleCan`** — covered by `analyzer.go:400-403`.
- **Exempt-path list** — covered, and extended from `centers` alone to five
  package bases for the literal rules.
- **Alias imports** — genuinely obsolete, confirmed by probe: a
  `dbx "…/internal/database"` alias resolved and flagged normally.
- **Two checks lost** — see H1 and H2.

The other items in the review brief (Scope literals, `CenterWideFor` read-name,
`MintOwnerAnchor`) were never in `scoping_guard_test.go`. It was the only guard
file at HEAD, so those are new coverage, not ports.

## Critical

None. The change is additive tooling. The tree is green and no repository
behavior changed.

## High

### H1. The analyzer fails open when `repository.go` is renamed or split

`findRepositoryFile` at `apps/api/tools/scopelint/scopelint/analyzer.go:107`
keys the entire R1 and repository-R3 file set on a file whose base name is
literally `repository.go`.

Proven in a copy: with a deliberately unscoped method present, `make scopelint`
reported it; after `mv repository.go queries.go`, the same tool exited 0 with
no output. Every rule for that package vanished silently.

The deleted guard had the matching tripwire and the analyzer does not:

```go
if len(paths) == 0 {
    t.Fatal("no feature repositories found — glob root moved?")
}
```

`TestNoDiagnosticsOnRepositoryTree` at `tree_test.go:21` asserts only that the
diagnostic count is zero, which is trivially satisfied by analyzing nothing.

**Fix:** have the tree test also assert a floor on the analyzed set. Export a
counter or a debug fact from the analyzer (packages whose repository file set
was non-empty) and fail when it drops below the current count, roughly eighteen
feature packages. A cheaper variant with no analyzer change: in `tree_test.go`,
walk `internal/features/*/` and fail when a package declares a method on a
receiver named in a `repository.go`-less file, i.e. a package that has
`*gormRepository` methods but no file named `repository.go`.

### H2. Repository-file rules skip free functions

The decl loop at `analyzer.go:84-91` requires `fn.Recv != nil`, so
package-level functions declared in `repository.go` are never visited. The
deleted guard read the file, not its method set.

Probe inserted into the real `billing/repository.go` in a copy:

```go
func p8FreeInRepoFile(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if sc.IsOwner {
		_ = sc.Has("billing.view_all")
		_ = authctx.StaffRoleCan("assistant", authctx.ClassCapability("x"))
	}
	return database.FromContext(ctx, r0.db).Where("id = ?", id).Delete(&Period{}).Error
}
```

Zero diagnostics. The old guard flagged three of those lines. R1 has the same
hole: a free query builder taking a `Scope` is unchecked.

**Fix:** run `checkRepoAuthority` on every `*ast.FuncDecl` in the file set, and
let `checkWitness` treat a receiverless function with a scope parameter the
same way as a method. The scope-parameter collection in `checkWitness` already
works without a receiver.

## Medium

### M1. The R1 witness is presence-based, so three false witnesses pass

`analyzer.go:341` (`walkForWitness`). All confirmed by probe:

- `_ = sc.CenterID` on its own line satisfies the rule while the query stays
  unscoped.
- A discarded helper result (`_ = r.readScoped(ctx, sc)`) does the same.
- Worst of the three: a narrowing helper that accepts a `Scope` and ignores it
  is a permanent laundering path, because narrowing helpers are exempt from R1
  by shape (no raw root) and still count as a witness for their callers.

Per-chain dataflow is an explicit non-goal, so this is accepted by design, but
the analyzer doc comment at `analyzer.go:29-43` presents the witness as if it
binds the scope.

**Fix:** one sentence in the doc comment saying the witness proves the scope
was mentioned, not that it reached the query, and that row-level security
remains the backstop for that residue.

### M2. The phase-2 carry-over is only half closed

`checkLiteralConstruction` at `analyzer.go:486` tests
`scopeKindOf(pass.TypesInfo.TypeOf(sel.X)) == "Scope"`, and `scopeKindOf`
returns empty for a pointer type. So `s := sc; s.TeacherID = x` flags as
intended, but this does not:

```go
p := &sc
p.TeacherID = uuid.Nil
```

Probed and confirmed silent.

**Fix:** unwrap one pointer level before the `scopeKindOf` call at
`analyzer.go:486`, the same way `recvTypeName` already does at
`analyzer.go:173`.

### M3. The docs tell a reader the wrong command

`docs/adding-permissions.md:105`, a line added in this change set, calls it a
"`go vet`-time scoping guard". It is not wired into `go vet`, and the phase
risk section explicitly rejected the `-vettool` route. Line 169 of the same
file correctly says "at test time", so the file contradicts itself.

### M4. Four legitimate scoping shapes would be flagged today

`walkForWitness` at `analyzer.go:363-368` requires the selector base to be an
`*ast.Ident` bound to a parameter object. Probed and confirmed to report a
missing witness:

- `s := sc` then `s.CenterID`
- `oa.Anchor.CenterID` through the embedded anchor
- `req.sc.CenterID` off a struct field
- `sc2.CenterID` where `sc2` is a nested func literal's own parameter

None occur on today's tree, but the copy-then-read pattern is natural, and the
diagnostic text gives no hint about which forms count.

## Low

- **L1.** Both the analyzer doc comment (`analyzer.go:34-36`) and
  `docs/api-guidelines.md:122-125` say the helper must be "on the same
  receiver", but `isHelperFunc` at `analyzer.go:226` never checks the receiver
  type. A probe with an unexported helper on an unrelated struct satisfied the
  witness. Same-package only, so low risk; align the docs or add the check.
- **L2.** The read-name rule uses `strings.Contains(strings.ToLower(...),
  "read")` at `analyzer.go:379`. A method named `P13AlreadyClosed` passed while
  `P14Write` flagged, because "already" contains "read". Also true of "spread",
  "thread", "ready".
- **L3.** R2 only recognizes a `gorm.Session{NewDB: true}` composite literal in
  the call arguments (`sessionResetsNewDB`, `analyzer.go:438`). Binding it to a
  variable first defeats the check. Zero hits today; the spec's wording was
  stricter than the implementation.
- **L4.** Testdata exercises two of the five exempt package bases.
  `middleware`, `authctx` and `seeds` have no negative case, and the
  sibling-file path in `collectRepoFiles` (`analyzer.go:134`) has no
  analysistest case, only implicit exercise through `sessions/pending.go` on
  the real tree.
- **L5.** A stale reference to `features/scoping_guard_test.go` survives at
  `.claude/agent-memory/kongming/project-teka-invariants.md:12`. Outside
  `docs/` and `apps/api/`, but it is a durable file that now describes a
  deleted test.
- **L6.** `seeds/` and `cmd/` sit outside the `./internal/...` pattern, so the
  module-wide R3 literal rules never see them. The `seeds` name is on the
  exempt list anyway, but a hand-built `Scope` in `cmd/` would go unnoticed.

## Answers to the specific checks

**(c) R1 soundness.** Caught: raw root inside a closure or nested func literal,
`db := r.db` aliasing, a `*gorm.DB` receiver field under any name,
`database.FromContext` through an alias import, a method on a named non-pointer
receiver. Accepted by design under the stated non-goals: an unused
`sc.CenterID`, a discarded helper result. Holes: a narrowing helper that
ignores its scope parameter (M1), and the helper-on-another-receiver-type
mismatch with the documentation (L1).

**(d) R3 assignment and literal rules.** The exempt bases are exactly
`centers`, `middleware`, `testutil`, `authctx`, `seeds`, matched on the last
path element (`isExemptBase`, `analyzer.go:96`). `authctx.Scope{}` with no
elements is correctly not flagged, verified by `GoodScopeZeroValue`. Test files
are skipped at `analyzer.go:74`, before every check, so `_test.go` files under
features are excluded. That matches the deleted guard, which read only
`*/repository.go`. It also means the tree run never sees test files at all,
since `packages.Load` runs without `Tests: true`.

**(e) tree_test.go.** `filepath.Abs("../../..")` resolves to `apps/api`, the
right module root, and the pattern is `teka/apps/api/internal/...`. It fails on
any diagnostic and on any per-action error. It is not gated behind `-short` or
a build tag. It runs under CI because `go list -tags=integration` with the
`make test-api` filter returns `teka/apps/api/tools/scopelint/scopelint`,
confirmed directly. One caveat: the config sets no `BuildFlags`, so the tree
load never applies `-tags=integration` even when the surrounding test run does.
No non-test file under `internal/` carries a build constraint today, so nothing
is missed now.

**(f) Makefile.** The `scopelint` target and the `test-api-unit` and `lint-api`
prerequisites are correct. `make test-api` does not depend on the target but
covers the analyzer through the tree test, which is the design. Coverage drag
is negligible: the analyzer package is at 92.2%, and `tools/scopelint` adds two
uncovered statements in `main.go` against a 60% floor.

**(g) go.mod.** Exactly the `golang.org/x/tools v0.49.0` promotion from
indirect to direct, no version change, and `go.sum` is untouched.

**(h) golangci-lint.** `golangci-lint run ./tools/...` reports 0 issues.

**(i) Naming hygiene.** No plan identifiers, phase numbers, finding codes, or
agent names anywhere in the tool. The `R1`/`R2`/`R3` labels appear in code
comments, but the analyzer's own doc comment defines all three, so a reader
never needs the plan. `.Table(` is allowed and has a passing test. A bare
directive is flagged (`BadBareDirective`).

**(j) Docs.** The Tenancy paragraph in `docs/api-guidelines.md:117-131` is
accurate on the witness rule, the raw-root definition, the read-name
restriction, the directive and its empty-reason failure, except for the "same
receiver" claim in L1. The stale `Scope.CenterWide()` sentence is gone.
`docs/adding-permissions.md` is updated but contradicts itself (M3). No stale
guard-test name remains under `docs/` or `apps/api/`; the comment at
`apps/api/internal/shared/authctx/authctx.go:156` says "the scoping guard"
generically and is now true for the first time.

## Recommended actions

1. Add an analyzed-set floor to the tree test so a rename or split of
   `repository.go` cannot silently disable the linter (H1).
2. Run the R3 authority checks, and ideally R1, on receiverless functions in
   the repository file set (H2).
3. Unwrap pointers in the Scope field-assignment check (M2).
4. Fix the "`go vet`-time" sentence in `docs/adding-permissions.md:105` (M3).
5. State in the analyzer doc comment that the witness is a mention, not a
   binding (M1), and reconcile the "same receiver" wording with the code (L1).
6. Optional hardening: tighten the read-name rule to a word boundary (L2),
   accept nested and copied scope selectors as witnesses (M4), add exempt-path
   testdata for the three untested bases (L4).

## Metrics

- Analyzer package statement coverage: 92.2% (`main.go` 0%, 2 statements)
- Linting issues on `tools/`: 0
- Directives on the tree: 0 (budget 8, expected 0-1)
- Tree-run wall time: 3.17s warm (budget 8s)

## Plan status

Phase 4's five success criteria are all satisfied on evidence. The two
deviations the implementer reported are correct: `database.FromContext` lives
at `teka/apps/api/internal/database`, and the testdata case packages must sit
under `testdata/src/teka/apps/api/internal/features/` for Go's
internal-visibility rule.

I would not block the phase on H1 and H2, but both should land before this is
treated as the standing tenancy guarantee, since H1 removes the guarantee
entirely on an ordinary refactor.

## Unresolved questions

1. Is the narrowing-helper laundering path (M1, third bullet) acceptable
   permanently, or should narrowing helpers be required to reference their
   scope parameter?
2. Should the `*Anchored` call-site allowlist carry-over be closed by a
   name-based rule, or left to human review as kongming suggested?
