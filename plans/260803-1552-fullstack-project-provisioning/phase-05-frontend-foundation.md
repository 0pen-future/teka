---
phase: 5
title: "Phase 5: Frontend Foundation"
status: todo
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 5: Frontend Foundation

## Overview

Scaffold the React app: Vite + TypeScript (strict), Tailwind CSS + shadcn/ui, ESLint (flat config) + Prettier, routing with layouts and protected routes, API infrastructure (axios + interceptors + normalized errors), TanStack Query, Zustand auth store skeleton, and zod-validated environment config. Ends with a themed app shell where routing, auth guard redirects, and a health-check API call work.

## Requirements

- Functional: `npm run dev` serves the app; public routes render in `auth-layout`, protected routes redirect to `/login` when unauthenticated.
- Functional: API client reads `VITE_API_URL`, attaches bearer token, auto-refreshes on 401 (single-flight), normalizes errors to one `ApiError` shape.
- Non-functional: TS `strict: true` (+ `noUncheckedIndexedAccess`); `npm run lint` and `npm run format:check` clean; path alias `@/` everywhere.

## Architecture

**Bootstrap:** `src/app/main.tsx` mounts `<Providers><RouterProvider/></Providers>`. `providers.tsx` composes `QueryClientProvider` (defaults: `staleTime 30s`, `retry 1`, no retry on 4xx), theme provider, and shadcn `Toaster`. `router.tsx` uses **React Router v7** `createBrowserRouter`; feature route arrays are imported and composed here — features export routes, `app/` owns the tree.

**Routing/layout separation:**

```text
/            → root-layout (error boundary + suspense shell)
  /login,/register → auth-layout (public-only: redirects authed users to /)
  /                → dashboard-layout (ProtectedRoute wrapper)
    /users, /users/:id, /settings …
```
`ProtectedRoute` reads the Zustand auth store; unauthenticated → `<Navigate to="/login" state={{from}}/>`; after login, return to `from`.

**API infrastructure (`src/lib/api/`):**
- `client.ts`: axios instance, `baseURL` from validated env, `withCredentials: true` (refresh cookie).
- `interceptors.ts`: request → attach access token from auth store; response 401 → single-flight `POST /auth/refresh`, queue concurrent 401s, retry once, on refresh failure clear store + redirect `/login`.
- `errors.ts`: normalize the backend envelope to `class ApiError { code; message; status; fields? }`; network/timeout → `code: "NETWORK_ERROR"`. Components never touch raw axios errors.

**State split (documented rule in `docs/frontend-guidelines.md`):** server data lives in TanStack Query only; Zustand holds session (user, access token) + pure UI state (sidebar). No server data duplicated into stores; forms hold their own state via react-hook-form.

**Env config (`src/lib/config/env.ts`):** zod-parse `import.meta.env` (`VITE_API_URL` url required); fail fast at boot with readable error. `.env.development` committed with localhost defaults; secrets never in `VITE_*` (they are public — documented).

**Styling:** Tailwind v4 (`@import "tailwindcss"` in `globals.css`) + shadcn/ui init (`components.json`, neutral base, CSS variables for theming); `cn()` in `lib/utils`. Dark mode via class strategy. Responsive: mobile-first; dashboard sidebar collapses under `md`.

**Accessibility baseline:** shadcn/Radix primitives only for interactive components (focus management for free); `eslint-plugin-jsx-a11y` in flat config; skip-link in root layout; icon buttons require `aria-label`.

**Lint/format:** ESLint 9 flat config — `typescript-eslint` (type-checked), `react-hooks`, `jsx-a11y`, `react-refresh`; Prettier separate (`eslint-config-prettier` last). ESLint (not TSLint — deprecated).

## Related Code Files

- Create: `apps/web/` scaffold (`index.html`, `vite.config.ts`, `tsconfig.json`, `package.json`)
- Create: `apps/web/eslint.config.js`, `.prettierrc`, `components.json`
- Create: `apps/web/src/app/{main.tsx,app.tsx,providers.tsx,router.tsx}`
- Create: `apps/web/src/layouts/{root-layout,auth-layout,dashboard-layout}.tsx`
- Create: `apps/web/src/lib/api/{client.ts,interceptors.ts,errors.ts}`
- Create: `apps/web/src/lib/config/env.ts`, `apps/web/src/lib/utils/cn.ts`
- Create: `apps/web/src/features/auth/stores/auth-store.ts` (skeleton: state + actions, wired fully in Phase 6)
- Create: `apps/web/src/components/shared/{error-boundary,spinner,page-header}.tsx`
- Create: `apps/web/src/styles/globals.css`
- Modify: root `Makefile` (`web-dev`, `lint-web`, `build-web`)

## Implementation Steps

1. `npm create vite@latest` (react-ts) into `apps/web`; enable strict TS options; configure `@/` alias in `tsconfig` + `vite.config.ts`.
2. Install & configure Tailwind v4 and shadcn/ui; add starter components: `button`, `input`, `form`, `card`, `table`, `dialog`, `dropdown-menu`, `sonner`, `skeleton`.
3. Replace template ESLint with full flat config; add Prettier; wire `lint`, `format`, `format:check` scripts; confirm lefthook globs from Phase 1 now activate.
4. Implement env validation, then API client/interceptors/errors (config first — client depends on it).
5. Implement providers, router, layouts, `ProtectedRoute`, error boundary with friendly fallback + reset.
6. Add a temporary dashboard home card calling `/healthz` through the client to prove the API path (replaced in Phase 6).
7. Wire Makefile targets.

## Success Criteria

- [ ] `make web-dev` → app shell renders, dark/light toggle works, no console errors
- [ ] Visiting `/users` unauthenticated redirects to `/login` and back after (stub) login
- [ ] `npm run lint`, `format:check`, `tsc --noEmit`, `npm run build` all clean
- [ ] Killing the API turns the health card into the normalized error state (`NETWORK_ERROR`)

## Risk Assessment

- **shadcn/Tailwind v4 setup drift** — CLI generators change; follow current shadcn docs during implementation rather than pinned copy-paste, commit `components.json`.
- **Token-in-JS tradeoff** — access token in memory (store), refresh in httpOnly cookie; XSS surface documented in `docs/frontend-guidelines.md`; no tokens in localStorage.
