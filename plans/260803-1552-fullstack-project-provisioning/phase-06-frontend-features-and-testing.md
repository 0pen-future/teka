---
phase: 6
title: "Phase 6: Frontend Features and Testing"
status: todo
priority: P1
effort: "1.5d"
dependencies: [5]
---

# Phase 6: Frontend Features and Testing

## Overview

Build the `auth` and `users` reference features against the real API contract, establishing the frontend feature-module pattern: query hooks, forms with zod schemas, loading/empty/error states, and per-feature tests. Stand up Vitest + React Testing Library + MSW, and Playwright for end-to-end flows.

## Requirements

- Functional: login/register/logout against the API; users list with pagination/search/sort, user detail, create/edit forms with server-side field errors mapped back to inputs; role-gated UI (admin-only actions hidden).
- Functional: every async view implements the loading (skeleton) / empty / error triad via shared components.
- Non-functional: `make test-web` runs Vitest suites headless with MSW (no real network); `make e2e` runs Playwright against the compose stack.

## Architecture

**Feature module contract** (mirrors backend discipline; in `docs/frontend-guidelines.md`):

```text
features/<name>/
  api/         # typed functions using lib/api client: getUsers(params): Promise<Paginated<User>>
  schemas/     # zod: request/response parsing + form schemas (single source for both)
  types/       # inferred from zod (z.infer) — no hand-duplicated types
  hooks/       # TanStack Query wrappers; query keys via <name>Keys factory
  components/  # feature components (dumb where possible)
  pages/       # route-level components; compose hooks + components
  stores/      # zustand, only if feature owns client state (auth does; users doesn't)
  routes.tsx   # RouteObject[] exported to app/router
  __tests__/   # component + hook tests colocated per feature
```
Cross-feature imports only via a feature's public `index.ts`; `features/*` never import from `app/`.

**Query conventions:** key factories (`usersKeys.list(params)`, `usersKeys.detail(id)`); mutations invalidate the narrowest key; list params (page/search/sort) live in URL search params (`useSearchParams`) so views are shareable/back-button-safe — not in component state.

**Forms:** react-hook-form + zodResolver + shadcn `Form` primitives; on `ApiError` with `fields`, map into `setError(field, {message})`; toast for non-field errors. One shared `useFormWithApiErrors` helper.

**Auth completion:** wire the Phase 5 store to real endpoints — `login`/`register` mutations set user+token; boot-time `GET /auth/me` (via refresh) restores sessions; `logout` clears store + query cache (`queryClient.clear()`).

**Testing stack:**

| Layer | Tool | What |
|---|---|---|
| Component/hook | Vitest + RTL + MSW | pages render states (loading/empty/error/data), forms validate + submit, guards redirect |
| E2E | Playwright | register→login→users CRUD→logout against compose stack; chromium in CI, all browsers locally |

- `src/test/msw/handlers.ts` mirrors the backend envelope exactly (success + error shapes) — MSW is the contract double.
- `renderWithProviders` test util: fresh `QueryClient` (retries off), router memory instance, auth store preset helpers.
- Playwright: `e2e/` with storage-state login fixture to skip repeated UI logins; `data-testid` only where role/text queries are ambiguous.

## Related Code Files

- Create: `apps/web/src/features/auth/{api,schemas,hooks,components,pages,__tests__}/...`, `routes.tsx`
- Create: `apps/web/src/features/users/{api,schemas,types,hooks,components,pages,__tests__}/...`, `routes.tsx`
- Create: `apps/web/src/components/shared/{data-table.tsx,empty-state.tsx,confirm-dialog.tsx}`
- Create: `apps/web/src/test/{setup.ts,utils.tsx,msw/handlers.ts,msw/server.ts}`
- Create: `apps/web/vitest.config.ts`, `apps/web/playwright.config.ts`, `apps/web/e2e/{auth.spec.ts,users.spec.ts}`
- Modify: `apps/web/src/app/router.tsx` (compose feature routes), `src/features/auth/stores/auth-store.ts`
- Modify: root `Makefile` (`test-web`, `e2e`)

## Implementation Steps

1. Set up Vitest (jsdom, globals, setup file with jest-dom + MSW server lifecycle) and `renderWithProviders`.
2. Write auth zod schemas/types; implement auth api + hooks + store wiring; login/register pages with full form handling; tests (validation errors, submit success, 401 message, guard redirect).
3. Write users schemas (`Paginated<T>` generic parse), api, query-key factory, hooks.
4. Build users list page: `data-table` shared component (sorting, pagination controls, URL-synced), search input (debounced), skeleton rows, `empty-state`, error state with retry; detail page; create/edit dialogs with API-error field mapping; delete with `confirm-dialog`.
5. Role-gate admin actions from the session store (`user.role`), matching backend 403 rules.
6. MSW handlers covering the full users/auth surface incl. 422/401/403 cases; component tests for all list states + form error mapping.
7. Playwright config + specs; add `make e2e` that expects the compose stack (Phase 7) and seeded admin.
8. Remove the Phase 5 temporary health card; finalize `docs/frontend-guidelines.md` (module contract, state rules, testing how-to).

## Success Criteria

- [ ] Full manual flow against local API: register, login, users CRUD with pagination/search, role gating, logout
- [ ] Server 422 on duplicate email lands under the email input, not a toast
- [ ] `make test-web` green offline (MSW only); list page tests cover loading/empty/error/data
- [ ] `make e2e` green against `make dev` stack
- [ ] Refreshing the browser mid-session keeps the user logged in (cookie refresh restore)

## Risk Assessment

- **MSW/backend contract drift** — handlers must copy the envelope from `docs/api-guidelines.md`; e2e against the real API is the drift detector.
- **E2E flakiness** — rely on Playwright auto-wait + web-first assertions, seeded deterministic data, per-test unique emails; no sleeps.
