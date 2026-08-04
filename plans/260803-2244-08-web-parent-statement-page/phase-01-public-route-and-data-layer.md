---
phase: 1
title: "Public Route, Unauthenticated Data Layer, Neutral Error Page"
status: completed
priority: P2
effort: "1d"
dependencies: []
---

# Phase 1: Public Route, Unauthenticated Data Layer, Neutral Error Page

## Overview

Establish the isolation boundary before any statement content exists. The
parent route must be reachable without a session, must never invoke the auth
interceptors, must not be indexed, and must fail neutrally.

Doing this first is deliberate: if the statement view is built on the ordinary
app plumbing, a parent's first bad token will bounce them to `/login`, and the
fix later is a refactor rather than a setting.

## Requirements

- [x] `/s/:token` renders without a session and without contacting
      `/auth/refresh`.
- [x] The route is mounted outside `ProtectedRoute` and outside
      `DashboardLayout` (`apps/web/src/app/router.tsx:24-32`).
- [x] A dedicated axios instance carries no auth interceptors and no
      credentials.
- [x] Any non-200 response — 401, 403, 404, 410, 500, or a network failure —
      renders one identical neutral error page naming no student, no teacher,
      no class, and no reason.
- [x] The page sets `noindex, nofollow` while mounted and removes it on unmount
      so the rest of the app stays indexable.
- [x] The public layout imports nothing from `features/auth`,
      `features/roster`, `features/billing`, or `features/collections`.
- [x] Data is fetched fresh on every mount (`staleTime: 0`, refetch on mount),
      overriding the app-wide 30s `staleTime`
      (`apps/web/src/app/providers.tsx:12`).

## Architecture

**Why a second axios instance.** `apps/web/src/lib/api/client.ts:12` sets
`withCredentials: true` and installs interceptors that, on a 401, attempt a
refresh and then clear the session
(`apps/web/src/lib/api/interceptors.ts`, wired through
`apps/web/src/lib/api/auth-bridge.ts`). On the parent route a 401 is a normal
outcome — a wrong token — and must render an error page, not trigger a refresh
attempt or a redirect. So the public route gets `publicApiClient`: same
`baseURL` from `env.VITE_API_URL` (`apps/web/src/lib/config/env.ts`), same
error normalization via `toApiError` (`apps/web/src/lib/api/errors.ts`), and
nothing else.

The envelope helpers (`apps/web/src/lib/api/envelope.ts:24`) are auth-agnostic
and are reused as-is.

**Route placement.** In `apps/web/src/app/router.tsx`, `/s/:token` becomes a
sibling of the `AuthLayout` and protected subtrees, under `RootLayout` so it
inherits the error boundary and suspense fallback
(`apps/web/src/layouts/root-layout.tsx:8`), but with its own `PublicLayout`
element.

```
RootLayout
├─ AuthLayout        → /login, /register
├─ ProtectedRoute > DashboardLayout → teacher app
├─ PublicLayout      → /s/:token          ← this plan
└─ * → NotFound
```

**Providers caveat.** `Providers` wraps the tree in `SessionRestore`
(`apps/web/src/app/providers.tsx:28`), which calls `/auth/refresh` on boot. On
the public route this is a pointless request that also sets off a 401 for every
parent. Phase 1 must make `SessionRestore` skip the attempt when the current
path starts with `/s/`. Read `window.location.pathname` inside the component's
existing effect; a route-aware check is not available above the router.

**Data flow.**

```
StatementPage (route /s/:token)
  useStatement(token)
     queryKey ["statement", token], staleTime 0, retry false, gcTime 0
     -> publicApiClient.get(`/public/statements/${token}`)
     -> parseData(statementSchema, res.data)
  states: pending  -> StatementSkeleton
          error    -> StatementErrorPage      (identical for every failure)
          success  -> StatementView           (phase 2)
```

`retry: false` matters: retrying a bad token three times is three chances to be
rate-limited and three seconds of a parent staring at a spinner. `gcTime: 0`
keeps a previous parent's data out of memory on a shared device.

**Robots.** Rather than a routing-aware server config, set the meta tag from the
page component with a small `useNoIndex()` effect that appends
`<meta name="robots" content="noindex, nofollow">` to `document.head` and
removes it on unmount. The app is a client-rendered SPA served from one
`index.html` (`apps/web/index.html:1`), so a static tag would apply to
everything. Note for deployment: the statement link is unguessable, so the
practical exposure is a parent pasting it somewhere public; the meta tag is the
cheap correct mitigation.

**Assumed API contract** (reconcile with plan 06):

| Method | Path | Auth | Response |
|---|---|---|---|
| GET | `/public/statements/:token` | none | `200` statement payload; `404` for unknown, revoked, or expired token |

The server hashes the token and looks it up (`statements.token_hash`,
`docs/schema_design.sql:413`), increments `view_count` and stamps
`first_viewed_at` / `last_viewed_at` (`docs/schema_design.sql:417-419`). No
client-side view tracking exists or is needed.

## Design Spec (DS foundation; error + skeleton states)

- `PublicLayout`: `--cream-100` page bg (inherited from the DS body styles),
  centered column `max-w-md`, no shell chrome. The DS tokens/fonts arrive via
  the app-level `globals.css` from plan `260803-2325-web-design-system-foundation`
  — nothing to import per-route.
- `StatementError`: an `HvCard raised` with a soft `--coral-100` icon disc
  (a "!" glyph in `--coral-600`), title `font-display` 700 `--ink-900`, body
  copy 15px `--ink-500`. Warm and neutral — an expired link is routine, not an
  error the parent caused. No retry button (unchanged requirement).
- `StatementSkeleton`: skeleton blocks rounded `--radius-xl` on `--cream-200`,
  mirroring the phase 2 layout (header bar, one child card, dark total block).
- `@/components/hv` imports are allowed here; the isolation guard (step 15)
  bans only `@/features/auth` and `@/lib/api/client`.

## Related Code Files

**Create**

- `apps/web/src/lib/api/public-client.ts` — `publicApiClient`, no interceptors,
  `withCredentials: false`.
- `apps/web/src/layouts/public-layout.tsx` — minimal shell.
- `apps/web/src/lib/hooks/use-no-index.ts` — robots meta effect.
- `apps/web/src/features/statement/api/statement-api.ts` — `getStatement`.
- `apps/web/src/features/statement/schemas/statement-schemas.ts` — minimal
  `statementSchema` for this phase; phase 2 expands it.
- `apps/web/src/features/statement/types/statement-types.ts`
- `apps/web/src/features/statement/hooks/use-statement.ts`
- `apps/web/src/features/statement/components/statement-error.tsx`
- `apps/web/src/features/statement/components/statement-skeleton.tsx`
- `apps/web/src/features/statement/pages/statement-page.tsx`
- `apps/web/src/features/statement/routes.tsx`
- `apps/web/src/features/statement/index.ts`
- `apps/web/src/features/statement/__tests__/statement-page.test.tsx`

**Modify**

- `apps/web/src/app/router.tsx` — add the public branch under `RootLayout`.
- `apps/web/src/features/auth/components/session-restore.tsx` — skip the refresh
  attempt on `/s/` paths.
- `apps/web/src/test/msw/handlers.ts` — a `/public/statements/:token` handler
  returning a fixture for a known token and 404 otherwise.

**Delete**

- None.

## Implementation Steps

1. Create `apps/web/src/lib/api/public-client.ts`:
   `axios.create({ baseURL: env.VITE_API_URL, withCredentials: false, timeout: 10_000 })`.
   Do **not** call `setupInterceptors`. Add a comment stating why the auth
   interceptors are deliberately absent, so a future contributor does not
   "fix" the inconsistency.
2. Create `apps/web/src/layouts/public-layout.tsx`: a centered column
   (`min-h-svh`, `max-w-md mx-auto`, `px-4 py-6`) with `<Outlet />` and an
   `id="main-content"` wrapper matching the skip-link target at
   `apps/web/src/layouts/root-layout.tsx:11`. No header, no nav, no theme
   toggle, no auth imports.
3. Create `apps/web/src/lib/hooks/use-no-index.ts`: appends the robots meta on
   mount, removes it on unmount, and is safe against double invocation under
   React strict mode.
4. Create `statement-schemas.ts` with a minimal shape for this phase —
   `{ contact_name, period: { year, month }, grand_total }` — enough to prove
   the pipeline end to end. Phase 2 extends it with the per-child structure.
5. Create `statement-api.ts` with `getStatement(token: string)`. It calls
   `publicApiClient.get` against the path `/public/statements/` plus the
   URL-encoded token, then parses through `parseData`
   (`apps/web/src/lib/api/envelope.ts:24`).
6. Create `use-statement.ts`:
   `useQuery({ queryKey: ["statement", token], queryFn, staleTime: 0, gcTime: 0, retry: false, refetchOnMount: "always" })`.
7. Create `StatementError`: a card reading "Không mở được liên kết này. Liên
   kết có thể đã hết hạn. Vui lòng liên hệ thầy/cô để nhận liên kết mới." No
   status code, no token echo, no student or teacher name, and no retry button
   that would invite hammering. One identical component for every failure mode.
8. Create `StatementSkeleton` from the existing `Skeleton` primitive
   (`apps/web/src/components/ui/skeleton.tsx`) roughly matching the final
   layout so the page does not jump.
9. Create `StatementPage`: read `token` via `useParams`, call `useNoIndex()`,
   then branch on the query state to skeleton, error, or a placeholder success
   block showing contact name and grand total (phase 2 replaces this with the
   real view).
10. Create `routes.tsx` exporting `statementRoutes` with `path: "/s/:token"`
    and a lazy import, following `apps/web/src/features/users/routes.tsx:7`.
11. Register the public branch in `apps/web/src/app/router.tsx` as a sibling of
    the auth and protected branches:
    `{ element: <PublicLayout />, children: statementRoutes }`.
12. Modify `session-restore.tsx` to skip the refresh call when
    `window.location.pathname.startsWith("/s/")`, rendering children
    immediately. Keep the existing behavior everywhere else.
13. Add the msw handler: token `valid-token` returns the fixture; anything else
    returns 404 with the API's error envelope shape
    (`apps/web/src/test/msw/handlers.ts:16`).
14. Write `statement-page.test.tsx` asserting: a valid token renders the
    placeholder content; an unknown token renders the neutral error whose text
    contains no student name and no HTTP status; a 500 renders the identical
    error text as the 404 case; the robots meta is present while mounted and
    gone after unmount.
15. Add a static guard test (or an eslint `no-restricted-imports` override
    scoped to `src/features/statement/**` and `src/layouts/public-layout.tsx`)
    forbidding imports from `@/features/auth` and `@/lib/api/client`. This is
    the cheapest durable defense of the isolation boundary.
16. Run `npm --prefix apps/web run typecheck`, `run lint`, and `test`.

## Success Criteria

- [x] Visiting `/s/anything` in a logged-out browser renders without redirecting
      to `/login`.
- [x] The network panel on `/s/:token` shows exactly one request — the statement
      GET — and no `/auth/refresh`.
- [x] A 404, a 500, and a network failure all produce byte-identical error copy.
- [x] The error page contains no student name, teacher name, class name, or
      status code.
- [x] `document.head` carries `noindex, nofollow` on the statement route and not
      on `/login`.
- [x] Reloading the page issues a fresh request rather than serving the cached
      response.
- [x] The lint or test guard fails if `features/statement` imports the
      authenticated client or the auth feature.
- [x] typecheck, lint, and vitest pass.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `SessionRestore` still fires `/auth/refresh` for parents | High if unaddressed | Medium | Step 12 plus the network assertion in the success criteria. |
| A later contributor swaps `publicApiClient` for `apiClient` to reuse interceptors | Medium | High | Comment at the instance plus the lint/test import guard from step 15. |
| Path-prefix check in `SessionRestore` drifts if the route moves | Low | Medium | Keep the `/s/` prefix as a single exported constant used by both the route definition and the check. |
| Token appears in server access logs or a Referer header | Medium | Medium | Out of the web app's control; flag to plan 06 to consider log scrubbing and a `Referrer-Policy: no-referrer` response header. |
| The error page tempts a retry loop on a rate-limited endpoint | Low | Low | `retry: false` and no retry button. |

**Rollback:** additive except for two small edits (router registration and the
`SessionRestore` guard). Reverting the route registration removes the public
surface entirely; the `SessionRestore` guard is inert when no `/s/` route
exists.
