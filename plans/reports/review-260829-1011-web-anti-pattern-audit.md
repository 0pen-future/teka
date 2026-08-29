# Anti-Pattern Review — apps/web

Scope: apps/web/src (325 TS/TSX files), reviewed against
`docs/frontend-guidelines.md` + `apps/web/CLAUDE.md` (local conventions are
authority; ak-frontend-development skill's MUI/TanStack Router specifics do not
apply to this stack).

## Findings

### 1. Cross-feature deep imports break module contract (medium)

Guideline: cross-feature imports go **only** through the feature's `index.ts`.

- `apps/web/src/layouts/auth-layout.tsx:3` imports `useIsAuthenticated` from
  `@/features/auth/stores/auth-store` although `@/features/auth` already
  exports it (`dashboard-layout.tsx` does it correctly).
- `apps/web/src/layouts/dashboard-layout.tsx:27` deep-imports
  `@/features/dashboard/hooks/use-dashboard` — root cause: `features/dashboard`
  has **no `index.ts` at all**, so the contract can't be honored.

Fix: change auth-layout import to `@/features/auth`; add
`features/dashboard/index.ts` exporting `usePendingSessions`.

### 2. `use-api-form-errors` bypasses hvToast wrapper (low)

`apps/web/src/lib/forms/use-api-form-errors.ts:27` calls raw
`toast.error("Something went wrong")` from sonner. All 24 other toast call
sites use the design-system `hvToast` (bottom-center ink pill). Also the only
English toast in a Vietnamese UI.

Fix: `hvToast("Đã có lỗi xảy ra", { variant: "danger" })`.

### 3. Duplicated reset-on-open effect + 7 lint suppressions (low, DRY)

Seven dialogs repeat the same `useEffect` "reset form when dialog opens" with
`eslint-disable-next-line react-hooks/exhaustive-deps` (contact-dialog,
class-dialog, student-dialog, record-payment-dialog, cancel-session-dialog,
adjustment-dialog, class-settings-page). Each suppression is individually
justified (form is stable), but the pattern is copy-pasted.

Fix option: shared `useResetOnOpen(form, open, values)` hook in `lib/forms/`
— removes all 7 suppressions and centralizes the invariant.

## Verified clean (non-findings)

- Route-level code splitting: all 12 `features/*/routes.tsx` use `route.lazy`.
- API layer: no raw axios/fetch outside `lib/api/`; UI matches `ApiError.code`,
  never raw axios errors.
- Env: single `import.meta.env` access point (`lib/config/env.ts`, zod-gated).
- State split: only one zustand store (auth, token in-memory by design); no
  server state copied into stores; no `useEffect` data fetching — all server
  state via TanStack Query.
- Loading: skeleton-based quartet (loading/empty/error/data), no
  layout-shifting spinner early-returns.
- TypeScript hygiene: no `any`, no `@ts-ignore`, no `console.log`.
- `components/ui/` shows no hand-edit markers; features use `hv/` wrappers,
  reaching for raw ui only for primitives without hv equivalents
  (input/field/select) — within the "prefer hv" wording.
- Search inputs debounced (300ms) with cleanup and URL-sync guard.

## Note

Skill `ak-frontend-development` describes MUI v7 + TanStack Router +
`useSuspenseQuery`; this app is shadcn/radix + react-router v8 + `useQuery`
with skeleton quartet. Consider updating or scoping that skill so it doesn't
mislead future sessions.

Unresolved questions: none.
