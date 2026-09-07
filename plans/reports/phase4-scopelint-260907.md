# Phase 4 — Repository Scope Linter (`scopelint`)

Plan: `plans/260906-0627-authz-write-scope-root-cause/phase-04-repository-scope-linter.md`
Design counsel: `plans/reports/kongming-phase3-5-checkpoint-260907.md` (Q3–Q4)
Status: **DONE**

## What was built

A `go/analysis` analyzer, `teka/apps/api/tools/scopelint`, that replaces the
three AST-based guard tests in the deleted
`apps/api/internal/features/scoping_guard_test.go` with type-based rules over
`go/types` object identity. It ships two ways:

- `make scopelint` / `go run ./tools/scopelint ./internal/...` — standalone
  CLI via `singlechecker.Main`.
- `go test ./tools/scopelint/...` — self-enforced under plain `go test`
  (and therefore under `test-api-unit`, `test-api`, and CI) via
  `golang.org/x/tools/go/analysis/checker`, so a missing witness fails as a
  test rather than depending on a Makefile hook an agent could step around.

### Rules implemented

- **R1 (scope witness)**: a repository method taking a
  `Scope`/`Anchor`/`OwnerAnchor` parameter that touches a raw `*gorm.DB` root
  (`database.FromContext(ctx, r.db)` or a `*gorm.DB` receiver field) must
  contain, anywhere in its body, either a call to a helper identified by
  *shape* (an unexported method on the same receiver returning `*gorm.DB`
  with ≥1 scope-typed parameter) or a direct selector on its own parameter's
  `CenterID` field. Opt-out: `//scopelint:unscoped <reason>` directly above
  the method; an empty/missing reason is itself flagged.
- **R2 (no reset)**: `.Unscoped()` and `.Session(&gorm.Session{NewDB: true})`
  are forbidden on any `*gorm.DB`-typed expression; `.Table(` is allowed
  (legitimate narrowing).
- **R3 (authority stays in authctx)**: repository-file-scoped bans on
  `Scope.IsOwner` reads, `Scope.Has` calls, `Scope.CenterWideFor` calls
  outside a function whose name contains `read`, and
  `StaffRolesFor`/`StaffRoleCan` calls; plus a module-wide ban (exempt:
  `centers`, `middleware`, `testutil`, `authctx`, `seeds`, and `_test.go`
  files) on hand-built `authctx.Scope{…}`/`OwnerAnchor{…}` composite
  literals, field assignment on a `Scope`-typed value, and
  `MintOwnerAnchor(` calls.

All detection is by `go/types` object identity (function/method/field
identity, named-type identity, selection kind) — no name or regex matching,
except the one spec-approved substring rule ("contains `read`") for
`CenterWideFor`'s read/write axis, which the type system cannot express.

### TDD sequence followed

1. `analyzer_test.go` written first against `analysistest`-driven fixtures
   (RED).
2. `analyzer.go` implemented until GREEN — passed on the very next run after
   fixing an import-path issue (see Deviations), no further design
   iteration needed.
3. `tree_test.go` added to self-enforce over the real module tree via
   `packages.Load` + `checker.Analyze` — GREEN with 0 diagnostics.
4. Negative probe: temporarily appended an unscoped
   `probeUnscopedDelete` method to `billing/repository.go`
   (`database.FromContext(ctx, r.db).Where(...).Delete(...)` with no
   witness) — `go test ./tools/scopelint/...` produced exactly 1
   diagnostic, confirming the analyzer fires on a real, uninstrumented
   violation. Reverted via an exact-suffix removal; `git diff` on the file
   is empty after revert (confirmed clean).
5. Old guard test file deleted only after step 3 was green.
6. Full verification: `go build ./...`, `go vet ./...`,
   `make test-api-unit` (which now runs `scopelint` as a prerequisite),
   `make lint-api` (scopelint + `golangci-lint run`, 0 issues) — all pass.

## Deviations from the literal task text (both verified against live source)

1. **`database.FromContext`'s real import path.** The phase spec and
   delegation text both state
   `teka/apps/api/internal/shared/database`. The function actually lives at
   `teka/apps/api/internal/database` (`apps/api/internal/database/tx.go:35`,
   package `database`, no `shared/` segment). Verified by direct read before
   using it anywhere; the analyzer's `databasePkgPath` constant and the
   testdata stub package both use the corrected path. Grepped the whole repo
   for `internal/shared/database` — no such package exists anywhere, so this
   is a stale reference in the two source documents, not an ambiguity.
2. **Testdata case-package location.** The spec's Architecture section
   suggests `testdata/src/a/…`. Go enforces internal-package import
   visibility purely by import-path prefix: a package outside
   `teka/apps/api/…` cannot legally import
   `teka/apps/api/internal/database` or
   `teka/apps/api/internal/shared/authctx`, even as test-only stubs placed
   at their real paths (which kongming's design requires, for `go/types`
   object-identity matching against the real symbols). `analysistest` failed
   with "use of internal package … not allowed" at the literal `a/` path.
   Fix: moved the case packages to
   `testdata/src/teka/apps/api/internal/features/testcase/{,centers,testutil}`,
   which legitimately sits under the `teka/apps/api` prefix. No other
   change was needed — the analyzer's logic was correct on the first run
   after this relocation.

Both are called out here per `documentation-management.md`'s "verify claims
against source, tests, scripts, artifacts, or live state" — the spec itself
should be corrected in a follow-up if it is reused.

## Real tree result

`TestNoDiagnosticsOnRepositoryTree` found **0 diagnostics** on
`teka/apps/api/internal/...` — better than the 0–1 directive the spec
predicted. **0** `//scopelint:unscoped` directives were added to any real
repository file (file ownership allowed at most one; none was needed).
Wall time: **3.25s** (warm, `go clean -testcache` then single run), well
under the target ceiling.

## Files changed

- `apps/api/tools/scopelint/main.go` (new) — CLI entrypoint.
- `apps/api/tools/scopelint/scopelint/analyzer.go` (new, ~400 lines) —
  analyzer implementation.
- `apps/api/tools/scopelint/scopelint/analyzer_test.go` (new) —
  `analysistest`-driven unit test.
- `apps/api/tools/scopelint/scopelint/tree_test.go` (new) — real-tree
  self-enforcement test.
- `apps/api/tools/scopelint/scopelint/testdata/src/...` (new) — stub
  packages (`gorm.io/gorm`, `teka/apps/api/internal/shared/authctx`,
  `teka/apps/api/internal/database`) and case packages
  (`teka/apps/api/internal/features/testcase{,/centers,/testutil}`)
  covering every R1/R2/R3 positive and negative case, plus the directive
  escape hatch (good and malformed-bare-directive cases).
- `Makefile` — added `scopelint` target; wired as a prerequisite of
  `test-api-unit` and `lint-api`.
- `apps/api/go.mod` — `go mod tidy` promoted `golang.org/x/tools v0.49.0`
  from indirect to direct (no version change). `apps/api/go.sum` diff is
  empty (confirmed via `git diff --stat`).
- `apps/api/internal/features/scoping_guard_test.go` — **deleted**.
- `docs/api-guidelines.md` — Tenancy section: replaced both guard-test
  references with a paragraph on the witness rule, helper-by-shape
  identification, the `read`-name rule for `CenterWideFor`, and the
  directive; and a shorter follow-on pointing the R3-literal ban at the same
  analyzer. Also dropped a now-false claim that `Scope.CenterWide()`
  "survives … has no production callers" — that method no longer exists
  anywhere in `internal/shared/authctx` (verified via grep); removing a
  legacy method the docs still described was out of this phase's scope, so
  the doc is now accurate about the exempt-package list actually
  implemented (`centers`, `middleware`, `testutil`, `authctx`, `seeds`) but
  makes no further behavioral claim.
- `apps/api/CLAUDE.md` — one line added under Verification: run
  `make scopelint` when touching a repository.
- `docs/adding-permissions.md` — updated the §6 tenancy-scoping reference
  from the deleted guard test to `apps/api/tools/scopelint`.

No file outside this list was modified. `git status --short` was checked
against the full pre-existing ~108-file uncommitted set from earlier phases;
none of those files were touched (verified: none of the other modified/
untracked paths in the tree appear in the diff scoped to this phase's file
list).

## Test status

- `go build ./...`: pass
- `go vet ./...`: pass
- `go test ./tools/scopelint/...` (`TestAnalyzer` + `TestNoDiagnosticsOnRepositoryTree`): pass
- `make test-api-unit` (scopelint + `go test -short ./...`, all feature
  packages): pass
- `make lint-api` (scopelint + `golangci-lint run`): pass, 0 issues
- `make test-api` / integration tests: **not run**, per constraint

## Review fixes

Applied all items from `plans/reports/code-review-260907-phase-04-scopelint.md`
(score 8/10, no Critical) except the explicitly-skipped L3. TDD followed
throughout: each testdata case was added and confirmed RED
(`go test ./tools/scopelint/... -run TestAnalyzer -v`) before the matching
`analyzer.go` change made it GREEN.

- **H1 (fail-open on rename)**: dropped the `repository.go` filename
  dependency. `findRepositoryReceiver` now scans `pass.Pkg.Scope().Names()`
  (deterministic sorted order) for a named struct with a `*gorm.DB` field;
  `centers` stays exempt from R1. Added a fail-closed floor to
  `tree_test.go`: an exported `atomic.Int64` counter
  (`ResetRepositoryPackages`/`RepositoryPackages`) asserts the real tree
  yields at least 18 repository packages (`minRepositoryPackages`), so a
  future rename/split/emptied-field regression fails loudly instead of
  silently analysing nothing. Doc comment updated to describe type-shape
  identification instead of the filename. Testdata: added `otherRepo` (a
  second `*gorm.DB`-fielded struct, sorts after `gormRepository`) to prove
  detection is unaffected by multiple candidates in one package.
- **H2 (free functions skipped)**: `run()` now iterates every
  `*ast.FuncDecl` in each repository file, not just methods on the
  receiver; `checkRepoAuthority`/`checkWitness` apply identically regardless
  of receiver. Testdata: `queries.go`'s `BadFreeFunction` (reads
  `sc.IsOwner` and touches a raw root with no witness) flags both R1 and R3.
- **M2 (pointer unwrap before Scope-assignment check)**: added `unwrapPtr`,
  applied in `checkLiteralConstruction`'s field-assignment check. Testdata:
  `authority.go`'s `BadScopeAssignPtr` (`p := &sc; p.TeacherID = "x"`) now
  flags.
- **M4 witness broadening + M1 doc sentence**: the CenterID witness now
  accepts a selector on any expression (one pointer level unwrapped) typed
  `Scope`/`Anchor`/`OwnerAnchor` — a copy, an embedded field, not just the
  function's own parameter. Testdata: `GoodCopiedScope` (`s := sc;
  s.CenterID`) and `GoodEmbeddedAnchor` (`oa.Anchor.CenterID`) added as
  negatives (no diagnostic). Added the "a witness proves the scope was
  mentioned … not that every query chain applied it; RLS is the backstop"
  sentence to `analyzer.go`'s doc comment and mirrored it verbatim in
  `docs/api-guidelines.md`'s Tenancy section, which was also corrected there
  to describe the broadened selector rule instead of the old
  "own parameter only" claim.
- **L1 (helper must be on the same receiver)**: `isHelperFunc` gained an
  `expectRecv *types.TypeName` parameter; a shape-matching method on an
  unrelated struct no longer counts. Testdata: `otherRepo.unrelatedScoped`
  + `BadUnrelatedHelper` (calls it, still flags).
- **L2 (exact "read" token, not substring)**: replaced
  `strings.Contains(lower, "read")` with `hasReadToken`/`splitCamelTokens`
  (camelCase- and `_`-boundary tokenization, case-insensitive exact-token
  match). Testdata: `alreadyClosed` (contains "read" as a substring of
  "already", not as a token) now flags a bare `CenterWideFor` call.
  Confirmed zero new real-tree diagnostics: manually verified every real
  `CenterWideFor` caller (`readWide`, `readScoped`, `readNarrowContacts`,
  `scopedRead`, `GetPeriodStatusRead`, `readScopedFeed`, `readNarrow`,
  `readScopedSchedules`, `contactsReadWide`, `readNarrowNames`) still
  contains an exact "read" token, confirmed empirically by
  `TestNoDiagnosticsOnRepositoryTree` staying at 0 diagnostics after the
  change.
- **M3 (stale "go vet" claim)**: `docs/adding-permissions.md:105` changed
  from "a `go vet`-time scoping guard" to "a test-time scoping guard
  (`go test ./tools/...`, also `make scopelint`)". Grepped `docs/` and
  `apps/api/CLAUDE.md` for other "go vet" claims about scopelint — none
  found; the only other hit was the line just fixed.
- **L4 (exempt-path + sibling-file negative testdata)**: added
  `testcase/middleware/middleware.go`, `testcase/authctx/authctx.go` (real
  stub imported under an explicit alias to avoid same-name-package
  confusion), `testcase/seeds/seeds.go` — each builds a Scope literal by
  hand and must NOT flag, proving the R3 module-wide exempt-base list.
  Added `queries.go` as a second non-test file in the `testcase` package
  declaring only a method on `gormRepository` (`BadSiblingFile`), no type
  of its own — proves the repository file set is not limited to the file
  declaring the receiver type.
- **L5 (stale memory reference)**: fixed
  `.claude/agent-memory/kongming/project-teka-invariants.md:12` to name the
  `scopelint` analyzer instead of the deleted
  `features/scoping_guard_test.go` (gitignored file, one-line edit, no
  `git status` footprint).
- **L6 (widen tree test to `cmd/`)**: `tree_test.go`'s `packages.Load` now
  loads both `teka/apps/api/internal/...` and `teka/apps/api/cmd/...`;
  `tools/` stays excluded. Confirmed `cmd/` has zero
  `authctx.Scope{`/`OwnerAnchor{`/`MintOwnerAnchor` usage before the
  change, so it added zero new diagnostics — confirmed empirically by the
  post-change clean run.
- **L3 (skipped, per instruction)**: variable-bound `gorm.Session` literal
  ban was not implemented — zero real hits and the spec targets the
  literal-argument form only. Added one sentence to `analyzer.go`'s R2 doc
  paragraph noting the literal-only scope explicitly, so the boundary is
  documented rather than silently assumed.

### Verification after fixes

- `gofmt -l ./tools`: clean (also opportunistically fixed a pre-existing
  gofmt misalignment in `testdata/src/gorm.io/gorm/gorm.go`, a file from
  this phase's earlier session, untouched by the review items).
- `go build ./...`: pass (3.0s wall).
- `go vet ./tools/...`: pass (0.26s wall).
- `go test -count=1 ./tools/...`: pass — `TestAnalyzer` (1.00s),
  `TestNoDiagnosticsOnRepositoryTree` (3.27s, 0 diagnostics, repository
  packages found ≥ 18 floor satisfied).
- `make scopelint` (repo root): pass, `0 issues.`
- `make lint-api` (repo root): pass, 0 issues.
- Negative probe re-verification: appended `ProbeAcceptance` (a
  `database.FromContext` call with no witness) to
  `billing/repository.go`; `go run ./tools/scopelint
  ./internal/...` produced exactly 1 diagnostic
  (`ProbeAcceptance builds a query from a raw *gorm.DB root`, matching the
  original review's finding). Reverted by restoring the pre-probe backup;
  `diff -q` against the backup now reports no difference — the file is
  byte-identical to its pre-probe state (an initial suffix-string-based
  revert left the file one trailing newline short; fixed by restoring from
  the backup directly rather than patching the revert script further).

### Unresolved from the review (not addressed, per team-lead's instruction scope)

The review's two open questions — the narrowing-helper laundering path and
whether a `*Anchored`-suffix call-site allowlist would be simpler than the
current shape rule — were not part of the 11-item fix list and remain open
for a future decision.

## Concerns / follow-ups (not in this phase's file ownership)

- Two repository files carry stale comments naming the deleted test file:
  `apps/api/internal/features/grading/repository.go:20` and
  `apps/api/internal/features/classstaff/repository.go:17` both say
  "enforced by scoping_guard_test". Left untouched — file ownership for
  this phase permits `repository.go` edits only to add a
  `//scopelint:unscoped` directive, and neither file needed one. Worth a
  one-line comment fix in a follow-up commit.
- The phase spec and delegation instructions both cite
  `teka/apps/api/internal/shared/database` for `FromContext`; the real path
  is `teka/apps/api/internal/database`. Flagging so the plan source itself
  can be corrected if reused.
