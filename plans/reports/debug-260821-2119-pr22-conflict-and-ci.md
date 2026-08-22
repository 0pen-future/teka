# Debug Report — PR #22 (feat/imports: Excel roster import)

**PR:** https://github.com/0pen-future/teka/pull/22 · branch `feature/import-class-student` (1 commit `e2fdcd1`) → `master`
**Status:** OPEN, `mergeable: CONFLICTING`, zero CI checks reported.

## Executive Summary

PR #22 has one real problem chain, plus a pre-existing master problem it will inherit:

1. **Merge conflict** — single file, `apps/web/src/features/roster/pages/students-page.tsx`, 2 hunks. Cause: master's teaching-data feature (`4ad5518`…`f0e5f13`) rewrote the same page header the PR touched.
2. **No CI on the PR** — consequence of (1): `pull_request` workflows run on the merge ref (`refs/pull/22/merge`); GitHub cannot build it while the PR conflicts, so no workflow triggers at all. Not a workflow-config bug (path filters match `apps/api/**` and `apps/web/**`).
3. **Master CI is already red** (independent of PR #22): API lint and Web lint fail on the teaching feature; Web nightly e2e failing 3 days running. After resolving the conflict, PR checks may still fail on lint if the branch inherits these files via merge-from-master.

## Root Cause — the conflict

Branch point is `0c411fb`; master since gained 9 commits (teaching feature). Both sides edited the header of `students-page.tsx`:

- **Master** removed the `useNavigate`-based "⚙ Cài đặt lớp" `HvButton` from the header row, re-rendered it as a `<Link>` in the class-tabs row (`to={/classes/${selectedClassId}/settings}`), dropped `useNavigate` entirely, and added attendance columns (`useSessionsList`, `formatDayMonth`, `useEnrollmentsList`, `currentMonth`).
- **PR** kept the old header and added an owner-only "Nhập từ Excel" `HvButton` (`navigate("/students/import")`) plus `useCenter` import.

**Trap in naive resolution:** taking the PR (HEAD) side of hunk 2 wholesale would (a) duplicate the settings control — master already renders it as a Link in the tabs row — and (b) reference `navigate`, whose declaration and import the auto-merge deleted (master removed them; the PR didn't touch those lines). File would not typecheck.

## Verified Resolution

Applied in a scratch worktree, merge `origin/master` into `e2fdcd1`:

1. **Imports hunk** — union:
   ```ts
   import { useSessionsList } from "@/features/attendance";
   import { useCenter } from "@/features/center";
   import { cn, formatDayMonth, formatPhoneLocal } from "@/lib/utils";
   ```
2. **Header hunk** — take master's side (settings button stays gone from the header; master's Link version survives), keep only the PR's owner-gated import button:
   ```tsx
   {isOwner ? (
     <HvButton variant="ghost" size="sm" onClick={() => { void navigate("/students/import"); }}>
       Nhập từ Excel
     </HvButton>
   ) : null}
   ```
3. **Re-add navigation plumbing** master deleted (its only consumer became a Link; the import button needs it back — `HvButton` renders a plain `<button>`, no `asChild`):
   - `import { Link, useNavigate, useSearchParams } from "react-router";`
   - `const navigate = useNavigate();` at top of `StudentsPage`.

Full diff: scratchpad `pr22-resolution.diff` (224 lines, includes auto-merged context).

## Verification Evidence (fresh, on merged tree)

- `tsc -b --noEmit` (apps/web): clean.
- `vitest run` full web suite: **57 files / 364 tests passed**.
- `go build ./...` (apps/api): clean.
- `go test ./...` full API suite: all pass, incl. `imports`, `teaching`, `server` (router.go auto-merge of both sides' routes is semantically sound).

## Pre-existing master CI failures (will surface on PR checks after resolution)

- **API CI `lint`** (run 32457477136): golangci-lint on teaching feature —
  - `teaching/service_test.go:404,525,530` errcheck: `wantAppError(t, err, …)` return value unchecked
  - `teaching/service.go:248` staticcheck S1016: convert `QueueRow` → `QueueItemResponse` instead of struct literal
- **Web CI `lint`** (run 32457477145): Prettier — 17 files unformatted (e.g. `src/layouts/dashboard-layout.tsx`).
- **Web CI `e2e`**: failing on master pushes + nightly since 2026-08-19 (not investigated further; separate issue).

Note: PR-branch lint scope depends on how CI lints (changed files vs whole tree). golangci-lint and `prettier --check` here run whole-tree, so these master defects will fail PR #22's checks even though its own code is clean. Fix them on master (or fold into the merge commit) before expecting green checks.

## Recommended Sequence

1. `git merge origin/master` on `feature/import-class-student`, resolve per above, push → PR becomes mergeable, CI starts running.
2. Fix master lint defects (4 Go issues + Prettier `--write` on 17 files) — separate small PR to master, or include in the merge commit if the team accepts that scope.
3. Investigate Web e2e nightly failure separately.

## Post-Resolution Update (21:38)

Merge pushed as `98b6bd1` → PR MERGEABLE, CI triggered (confirms conflict was blocking CI). Results:

- **API CI:** test ✅, swagger-drift ✅, lint ❌ — the 4 inherited teaching lint issues, as predicted.
- **Web CI:** lint ❌ (inherited Prettier), **test ❌** — 2 tests in the PR's own `roster-import-page.test.tsx` ("downloads the template…", "surfaces the envelope's message…"), both dying at ~1.1s.

Web test failures diagnosed as **timing flakiness, not logic defects**: both hit the default 1000ms `waitFor`/`findBy` timeout on the slow coverage-instrumented CI runner. Local repro: first `test:coverage` run after `npm ci` failed a *different* test in the same file ("keeps the commit button disabled while the import is in flight", a scripted-`delay()` race); 3 subsequent full coverage runs were 364/364 green. Rerun of the CI job was refused (needs repo admin).

**Hardening suggestion:** in `roster-import-page.test.tsx`, raise the timeout on the two download `waitFor`/`findBy` calls, and replace the in-flight test's fixed `delay()` window with a manually-resolved deferred handler.

## Unresolved Questions

- Web e2e failure cause on master (out of scope here).
- Whether team prefers master-lint fixes as a separate PR or bundled into PR #22's merge commit.
