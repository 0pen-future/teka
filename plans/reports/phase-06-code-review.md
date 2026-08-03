# Phase 6 Code Review — Frontend Features and Testing

Reviewer: `code-reviewer` subagent · Baseline: `0c5759f` (uncommitted Phase 6 work) · Date: 2026-08-03

## Verdict

DONE_WITH_CONCERNS → all findings resolved (see Resolution). All 6 functional
acceptance criteria met; criterion (e) — no new lint/type/build errors —
initially failed on two prettier issues, fixed same-session.

## Gates (as run by reviewer)

| Gate | Result |
|---|---|
| `npm run lint` | pass |
| `npm run typecheck` | pass |
| `npm run format:check` | fail (2 files) → **fixed** |
| `npm run test` | pass (19 tests / 4 files at review time; 22/5 after fixes) |
| `npm run build` | pass |
| Playwright e2e | 6/6 (run by orchestrator against live stack, re-verified after fixes) |

## Findings and resolution

### High (both fixed)

- **H1** `apps/web/README.md` — new scripts-table rows broke prettier column
  alignment, failing `make lint-web`. Fixed with `prettier --write`.
- **H2** `apps/web/.prettierignore` — missing Playwright artifacts, so every
  `make e2e` run would redden `make lint-web` via `test-results/`. Fixed:
  added `test-results` and `playwright-report`.

### Medium (all fixed)

- **M1** `users-page.tsx` — search input never synced back from the URL, so
  clicking the Users nav link while a search was active restored `?q=` after
  300ms with a redundant request. Fixed with a `lastAppliedQ` ref that mirrors
  external URL changes into the input while ignoring the page's own writes.
- **M2** `docs/frontend-guidelines.md` — claimed route-level `lazy()` code
  splitting that doesn't exist. Reworded: `routes.tsx` keeps pages out of
  cross-feature import chains and leaves room for lazy loading later.
- **M3** `auth-api.ts` — dead `getMe()` export with no consumer or test.
  Removed (YAGNI).
- **M4** `EditUserDialog` partial-PATCH logic untested. Added
  `edit-user-dialog.test.tsx` (3 tests): non-admin name edit sends only
  `{name}`, admin role change includes `role`, no-change submit closes without
  a request.

### Low

- **L2** SortHeader button missing `type="button"` — fixed.
- **L3** Sortable columns missing `aria-sort` — fixed on `TableHead`.
- **L5** `replace: true` on pagination history — kept as deliberate (Back
  leaves the page rather than replaying filter tweaks); documented in a code
  comment.
- **L1** Prev/Next compute target page from possibly-stale `meta.page` during
  `keepPreviousData` fetches (rapid double-click swallows the second click; no
  data corruption) — accepted, not fixed.
- **L4** Boot-restore failure doesn't call `markRefreshDead()`, costing one
  guaranteed-401 refresh round-trip on the next 401 — harmless, accepted.
- **L6** Non-admin visiting `/users` directly sees the error empty-state
  ("admin only") rather than a dedicated forbidden page — acceptable UX,
  server enforces authorization.

## Reviewer-verified non-issues

Single-flight refresh race, auth↔users import cycle (none — `users/index.ts`
exports only schema/type), `useUpdateUser` id stability, query-key hashing,
test isolation (zustand + MSW reset per test), and contract drift (sort
whitelist, `q`/`role` params, seeded e2e credentials all match the Go API).

## Acceptance criteria

| # | Criterion | Result |
|---|---|---|
| 1 | Auth flow + boot session restore | met |
| 2 | Users CRUD, URL-param pagination/search/sort, role gating | met |
| 3 | Duplicate email under the email input (409 CONFLICT via `conflictField`) | met |
| 4 | Vitest offline (MSW `onUnhandledRequest: "error"`), list quartet covered | met |
| 5 | Playwright sequential against seeded stack | met |
| 6 | Health card removed, docs finalized | met |

## Post-fix verification

`npm run lint`, `npm run typecheck`, `npm run format:check` clean;
`npm run test` 22/22 in 5 files; `make e2e` re-run 6/6 against a freshly
seeded stack after all fixes landed.
