# Teka Web Frontend — Inventory Report (scout → docs-manager)

Scope: `apps/web/` as of 2026-08-19, commit `0c411fb`, branch `master`. Read-only scan. All paths
absolute from repo root `/home/vmo/workspace/testing/teka`.

---

## 1. Stack & Tooling

### 1.1 Dependencies (`apps/web/package.json`)

| Package | Version | Role |
|---|---|---|
| react / react-dom | ^19.2.8 | React 19 (`use()`, `<Context>` as provider) |
| vite / @vitejs/plugin-react | ^8.2.0 / ^6.0.5 | Bundler, dev server |
| typescript | ~6.0.2 | Strict TS |
| tailwindcss + @tailwindcss/vite | ^4.3.3 | Tailwind v4, CSS-first config (`@theme inline`) |
| tw-animate-css | ^1.4.0 | Animate utilities for shadcn |
| radix-ui | ^1.6.7 | Single-package Radix (`import { Dialog as DialogPrimitive } from "radix-ui"`) |
| lucide-react / class-variance-authority | ^1.28.0 / ^0.7.1 | Icons; `cva` variants |
| clsx + tailwind-merge | ^2.1.1 / ^3.6.0 | `cn()` |
| react-router | ^7.18.2 | Data router (`createBrowserRouter`, `route.lazy`) |
| @tanstack/react-query / zustand | ^5.101.4 / ^5.0.14 | Server state / client-session state |
| react-hook-form + @hookform/resolvers | ^7.84.0 / ^5.7.1 | Forms |
| zod / axios / sonner | ^4.4.3 / ^1.19.0 / ^2.0.7 | Schemas; HTTP; toasts |
| @fontsource/baloo-2, @fontsource/nunito | ^5.3.0 | Self-hosted fonts (woff2, vietnamese subset) |

Dev: vitest ^4.1.10 + @vitest/coverage-v8, jsdom ^29, msw ^2.15, @testing-library/{react,jest-dom,user-event},
@playwright/test ^1.62, eslint ^9.39 + typescript-eslint ^8.65 + eslint-plugin-{jsx-a11y,react-hooks,react-refresh},
prettier ^3.9 + eslint-config-prettier, rollup-plugin-visualizer ^7, shadcn ^4.16.

### 1.2 Scripts

`dev` · `build` (`tsc -b && vite build`) · `build:analyze` (`ANALYZE=true` → `stats.html` treemap, gitignored) ·
`lint` · `format` / `format:check` · `typecheck` (`tsc -b --noEmit`) · `test` (vitest run) · `test:coverage` ·
`test:watch` · `e2e` (playwright) · `preview`.
Repo-level wrappers: `make lint-web`, `make test-web`, `make build-web`, `make e2e`, `make web-dev`.

### 1.3 Config facts

- **Path alias**: `@` → `./src`, declared three times — `vite.config.ts:24`, `vitest.config.ts:9`,
  `tsconfig.json`/`tsconfig.app.json` `paths`.
- **TS strictness** (`tsconfig.app.json`): `strict`, `noUncheckedIndexedAccess`, `noUnusedLocals`,
  `noUnusedParameters`, `erasableSyntaxOnly`, `noFallthroughCasesInSwitch`, `verbatimModuleSyntax`,
  `moduleResolution: bundler`, target/lib ES2023, `jsx: react-jsx`, `noEmit`. Project references split app
  (`src`) vs node (vite/vitest/playwright configs, `e2e`).
- **Dev server** (`vite.config.ts:37-59`): fixed `port: 5173`, `strictPort` (the API CORS allowlist references
  it). Two compose-only Node-side knobs, deliberately **not** `VITE_`-prefixed so they never reach the bundle:
  `WEB_API_PROXY_TARGET` (proxies both `/api` and `/public`, **no path rewrite** — the refresh cookie is scoped
  to `/api/v1/auth`) and `WEB_USE_POLLING` (inotify fallback for Docker bind mounts).
- **Prettier**: `printWidth: 100`, double quotes, `trailingComma: "all"`.
- **ESLint** (`eslint.config.js`) flat config: `recommendedTypeChecked` + `stylisticTypeChecked` + react-hooks +
  react-refresh(vite) + **jsx-a11y recommended**, `eslintConfigPrettier` last. Two scoped overrides:
  (1) `src/components/ui/**` — `react-refresh/only-export-components` + `@typescript-eslint/array-type` off
  (keep upstream shadcn shape for regeneration); (2) `src/features/statement/**` +
  `src/layouts/public-layout.tsx` — `no-restricted-imports` **forbids** `@/features/auth*` and
  `@/lib/api/client`, enforcing isolation of the public parent route.
- **shadcn** (`components.json`): style `radix-nova`, baseColor `neutral`, cssVariables true, no prefix, lucide
  icons, css entry `src/styles/globals.css`, aliases `@/components`, `@/components/ui`, `@/lib`.
- **Production image** (`apps/web/Dockerfile`): node:22-alpine (digest-pinned) → `nginxinc/nginx-unprivileged:1.29-alpine`,
  listens 8080. `VITE_API_URL` is a required `--build-arg`; the Dockerfile hard-fails if empty
  (`RUN test -n "${VITE_API_URL}"`) because `env.ts` only validates at browser boot. `nginx.conf`: SPA fallback,
  `/assets/` immutable 1-year cache, **`/api/` and `/public/` return 404** so a proxy mis-route fails loudly
  instead of serving `index.html` as JSON; security headers repeated per location.
- `index.html`: `<html lang="vi">`, `theme-color #f4f8f3` (= `--cream-100`), manifest + svg favicons.

---

## 2. Route Map

Router: `apps/web/src/app/router.tsx` — one `createBrowserRouter` tree; **features export route arrays,
the router file owns layouts + guards**. `HydrateFallback: () => null`.

```
RootLayout                    (skip link + ErrorBoundary + Suspense/Spinner)
├── AuthLayout                (public-only; redirects authed users to state.from ?? "/")
│   ├── /login                            auth        lazy
│   ├── /forgot-password                  auth        lazy
│   ├── /reset-password/:token            auth        lazy
│   └── /invite/:token                    invitation  lazy
├── ProtectedRoute > DashboardLayout   path "/"
│   ├── index  "/"                        dashboard   lazy
│   ├── contacts                          roster      lazy
│   ├── contacts/:id                      roster      lazy
│   ├── students                          roster      lazy
│   ├── students/:id                      roster      lazy
│   ├── classes/:id/settings              roster      lazy
│   ├── sessions                          attendance  lazy
│   │   └── :id/attendance                attendance  lazy  (nested → lg+ two-pane via <Outlet/>)
│   ├── billing                           billing     eager (BillingIndexRedirect)
│   ├── billing/:periodId                 billing     lazy
│   ├── collections/:periodId             collections lazy
│   ├── notifications/:periodId           collections lazy
│   ├── profile                           profile     lazy
│   └── center                            center      lazy
├── PublicLayout              (no header/nav/theme; AppFooter)
│   └── /s/:token                         statement   lazy   ← unauthenticated parent statement
└── *  → NotFound
```

- **Lazy loading**: every feature route uses `lazy: async () => ({ Component: (await import(...)).X })`;
  each page lands in its own chunk. Only `BillingIndexRedirect` is eager (it is a 25-line redirect).
- **Guarding**: single `ProtectedRoute` wrapper around `DashboardLayout` (router.tsx:33-38). It reads
  `useIsAuthenticated()` and `<Navigate to="/login" state={{ from: pathname+search+hash }} replace/>`.
  `AuthLayout` is the inverse guard and also honors `state.from`, so the store-update/navigate race is harmless
  (`apps/web/src/layouts/auth-layout.tsx:10-17`).
- `STATEMENT_PATH_PREFIX = "/s/"` exported from `apps/web/src/features/statement/routes.tsx:9`; the same literal is
  **deliberately duplicated** in `session-restore.tsx:12` because the statement feature may not depend on auth.

### Layout shells

| File | Responsibility |
|---|---|
| `src/layouts/root-layout.tsx` | Skip link (`sr-only focus:not-sr-only` → `#main-content`), `ErrorBoundary`, `Suspense` + centered `Spinner` |
| `src/layouts/auth-layout.tsx` | Centered `max-w-sm` card column, `bg-muted/40`, owns `id="main-content"` |
| `src/layouts/public-layout.tsx` | `mx-auto max-w-md` column + `AppFooter`; imports nothing from auth/roster/billing/collections or `lib/api/client` |
| `src/layouts/dashboard-layout.tsx` (409 L) | 3-way responsive nav shell |

**DashboardLayout responsive model** (main app chrome). Nav entries: Tổng quan `/` · Điểm danh `/sessions`
(pending dot) · Lớp & học sinh `/students` · Phụ huynh `/contacts` · Chốt sổ `/billing/:id` · Gửi thông báo
`/notifications/:id` · Thu tiền `/collections/:id` · Trung tâm `/center`.
- `lg+`: 236px sidebar (white, `border-line-200`) — logo block, all 8 entries, `CurrentPeriodCard` (mint-50,
  "Kỳ hiện tại"), `SidebarFooter` (avatar disc + name → `/profile`, then Đăng xuất).
- `md–lg`: 72px icon rail (`title` + `aria-label`), `ProfileDisc`, `CurrentPeriodDisc` (`T{month}`).
- `<md`: fixed bottom tab bar, 5 slots `min-h-[56px]` = 4 primary tabs + `MoreTab` ("Thêm") opening an `HvModal`
  sheet. `OVERFLOW_LABELS = {Chốt sổ, Gửi thông báo, Phụ huynh, Trung tâm}`;
  `OVERFLOW_PATH_PREFIXES = ["/billing","/notifications","/contacts","/center"]` — static prefixes, because a
  period-scoped `to` is `null` until `useCurrentPeriod` resolves.
- Period-scoped entries render **disabled** while `to === null` (`aria-disabled`, `cursor-not-allowed`,
  `text-ink-300`). `PendingDot` = `size-2 rounded-full bg-coral-400` on the icon.
- Content column widths from DS tokens: `md:max-w-[var(--w-content)]` 720px, `lg:max-w-[var(--w-page)]` 1080px.

---

## 3. Feature-by-Feature

All features follow: `api/` · `schemas/` · `hooks/` · `components/` · `pages/` · `routes.tsx` · `__tests__/`
(+ `lib/`, `types/`, `stores/` where needed) + a public `index.ts`. `index.ts` never exports pages or routes.

### auth (956 L)
Pages `LoginPage`, `ForgotPasswordPage`, `ResetPasswordPage` (self-registration removed — invite-only).
API `POST /auth/{login,forgot-password,reset-password,refresh,logout}`, `GET /me`.
Schemas: `vnPhonePattern = /^(0|\+84)(3|5|7|8|9)\d{8}$/` mirroring the Go `validation.go`; `normalizePhone()`
(`0…`→`+84…`) applied as a zod `.transform`; `loginSchema`, `forgotPasswordInput/ResponseSchema`,
`resetPasswordFormSchema` (8..72 + `confirm_password` refine, client-only), `teacherSchema`, `sessionSchema`
(`{access_token, token_type, expires_in, teacher}` — key is `teacher`, **not** `user`).
Store `stores/auth-store.ts` (the app's only Zustand store) registers itself via `connectAuthBridge` at module load.
Hooks `useLogin`, `useForgotPassword`, `useResetPassword`, `useLogout` (onSettled → `clearSession()` +
`queryClient.clear()`). Components `ProtectedRoute`, `SessionRestore`.

### dashboard (756 L, no `index.ts`)
Page `DashboardPage` (50 L) — time-of-day greeting ("Chào buổi sáng/trưa/tối"), stats, class cards, pending alert.
API `GET /sessions/pending` (`{total, items[]}` keyed `session_id`, not `id`), `GET /billing-periods/:id/preview`.
Hooks `usePendingSessions`, `useStudentsTotal`, `useClassStudentCounts` (`useQueries` fan-out over classes),
`useClassPeriodSessions`, `usePeriodPreview`. Components `dashboard-stats`, `class-overview-cards`,
`pending-attendance-alert`.

### roster (6126 L — largest)
Pages `ContactsPage`, `ContactDetailPage`, `StudentsPage` (404 L, consolidated "Lớp & học sinh"),
`StudentDetailPage`, `ClassSettingsPage`.
API (4 files): students `GET/POST /students`, `GET/PUT/DELETE /students/:id` (delete = anonymize); contacts
`GET/POST /contacts`, `GET/PUT/DELETE /contacts/:id`, `PUT|DELETE /contacts/:id/zalo-mapping`; classes
`GET/POST /classes`, `GET/PUT /classes/:id`, `POST /classes/:id/schedules`,
`PUT|DELETE /classes/:id/schedules/:scheduleId`; enrollments `GET/POST /enrollments`, `GET /enrollments/:id`,
`POST /enrollments/:id/end`, `DELETE /enrollments/:id` (no UI consumer).
Schemas: contact/student/schedule/class/enrollment + inputs (`classCreateInputSchema` requires ≥1 schedule;
`classDialogInputSchema`, `classSettingsInputSchema`, `scheduleSlotInputSchema`; `endEnrollmentInputSchema` is a
factory). `roster-keys.ts` holds all four key factories in one module so cross-entity invalidation needs no
hook-file cycles; `useClassSearch` is a client-side filter.
lib: `roster-format.ts` (`parseMoney`, `formatWeekday` full/short, `formatScheduleSummary` →
`"T2 · T4 — 18:00, T6 — 20:00"`), `schedule-diff.ts` (`activeSchedules`, `deriveScheduleSlots`).
13 components incl. class/contact/student dialogs, `contact-picker`, `weekday-chips`, `schedule-slots-editor`,
`money-input`, `enroll-/end-enrollment-`/`anonymize-student-dialog`, `zalo-auto-map-dialog`, `zalo-friend-picker`.

### attendance (1788 L)
Pages `SessionsPage` (list + `<Outlet/>` two-pane at lg+), `AttendancePage`.
API `GET /classes/:id/sessions`, `GET /sessions/:id`, `GET|POST /sessions/:id/attendance`,
`POST /sessions/:id/cancel`, `POST /billing-periods` (period-for-date).
Schemas `sessionSchema`, `attendanceRowSchema`, `attendanceResponseSchema`, `confirmAttendanceInputSchema`,
`cancelSessionInputSchema`.
Hooks `useSessionsList`, `useSession`, `useSessionRoster`, `usePeriodForDate` (staleTime 5 min),
`useSaveAttendance`, `useCancelSession` — save invalidates roster + detail + lists + dashboard pending +
billing current period. Components `attendance-row` (tap toggle, `aria-pressed`), `confirm-attendance-bar`,
`cancel-session-dialog`, `closed-period-warning`, `session-list-item`.

### billing (1668 L)
Page `BillingReviewPage` ("Chốt sổ").
API `POST /billing-periods` (idempotent ensure = "current period"), `GET /billing-periods`,
`GET /billing-periods/:id`, `POST /billing-periods/:id/draft`, `GET /billing-periods/:id/preview`,
`GET /sessions/pending`, `POST /billing-periods/:id/close`, `POST /invoices/:id/adjustments`.
Schemas `periodSchema`, `invoiceLineSchema`, `reviewRow/Totals/reviewSchema`, `blockingSessionSchema`,
`unconfirmedSessionSchema`, `closeWarningsSchema`, `closeResponseSchema`, `adjustmentInput/ResponseSchema`.
Hooks `useCurrentPeriod` (staleTime 5 min), `usePeriod`, `usePeriods`, `useReview` (staleTime 30 s),
`useBlockingSessions`, `useCreateAdjustment`, `useClosePeriod`.
Components `review-table` / `review-card-list` (responsive pair), `adjustment-dialog`, `close-period-dialog`,
`blocking-sessions-panel`, `period-switcher`, `billing-index-redirect`.

### collections (3540 L)
Pages `CollectionsPage` ("Thu tiền"), `NotificationsPage` (384 L, "Gửi thông báo").
API — collections: `GET /billing-periods/:id`, `GET /billing-periods/:id/collections` (`view=contact|class`),
`…/collections/summary`, `POST /payments`, `PUT /payments/:id/allocations`; notifications:
`POST /billing-periods/:id/notifications/bulk-send`, `GET /billing-periods/:id/notifications`,
`POST /notifications/mark-sent`, `GET /billing-periods/:id/notifications/run`, + resume.
Schemas `paymentStatusSchema` (`unpaid|partial|paid`), `paymentMethodSchema` (`cash|transfer|other`),
`allocatedBySchema` (`auto|manual`), contact/class collection rows, `collectionsSummarySchema`,
`recordPaymentInputSchema`, `reallocateInputSchema`, notification purpose/channel/status enums,
`bulkSendInput/Row/ResponseSchema`, `runSnapshotSchema`, `notificationRowSchema`.
types/: `COLLECTIONS_VIEWS = ["contact","class"]` (contact = PRD default); `ListContactCollectionsParams`;
`ListClassCollectionsParams` (`class_id` required, 422 without).
Hooks `usePeriod`, `useContactCollectionsList`/`useClassCollectionsList` (both `keepPreviousData`),
`useCollectionsSummary`, `useRecordPayment`, `useReallocatePayment`; `useNotificationsList`,
`useBulkSendNotifications`, `useNotificationRun` (**polls** via `refetchInterval` while a run is in progress),
`useResumeNotificationRun`, `useMarkNotificationsSent`. lib `money-format.ts#parseMoney`.
Components `allocation-editor`, `class-collection-group`, `collections-view-toggle`, `contact-collection-row`,
`message-card`, `money-field`, `record-payment-dialog`, `run-progress-banner`.

### statement (897 L) — public, isolated
Page `StatementPage` (28 L) at `/s/:token`. API `publicApiClient.get("/public/statements/:token")` (root-mounted,
outside `/api/v1`). Hook `useStatement` — `staleTime 0`, `gcTime 0`, `retry false` (stale money is worse than a
spinner; a shared phone must not keep the previous parent's statement in memory).
Schemas: nested tree `statementSessionSchema` → Class → Child → `statementSchema`, plus
`statementTotalsSchema`, `statementPaymentsSchema`, `statementQrSchema` (nullable).
Components `statement-view`, `child-section`, `class-block`, `session-date-list`, `grand-total`, `payment-qr`,
`copy-field`, `statement-error` (neutral, leaks nothing), `statement-skeleton`.
lib `format-chip-date.ts` (string-slice `dd/MM`, no `Date`). Calls `useNoIndex()` → `robots: noindex, nofollow`.

### profile (1638 L)
Page `ProfilePage` (213 L) — teacher profile + Zalo connection.
API `PUT /me`; zalo `GET /me/zalo`, `GET /me/zalo/friends`, `POST /me/zalo/friends/{match,request}`,
`POST /me/zalo/link/start`, `GET /me/zalo/link/status?id=`, `DELETE /me/zalo`.
Schemas `profileFormSchema`; `zaloStatusSchema`, `zaloFriendSchema`, `zaloFriendMatchSchema`,
`zaloLinkStartSchema`, `zaloLinkStateSchema`, `zaloLinkStatusSchema`, `isTerminalLinkState()`; `ZALO_CONSENT`
(version `"2026-08-personal-v1"` + 3 consent points + checkbox label, colocated so what was read and what is
recorded cannot drift). Constants `ZALO_POLL_INTERVAL_MS 1500`, `ZALO_MAX_POLL_ERRORS 3`,
`ZALO_FRIENDS_STALE_MS 60_000`, `ZALO_MATCH_MAX_PHONES`, `ZALO_MATCH_REQUEST_SIZE`.
Hooks `useUpdateMe`, `useZaloStatus`, `useZaloFriends(enabled)`, `useMatchZaloFriends`,
`useSendZaloFriendRequest`, `useStartZaloLink`, `useZaloLinkStatus` (polls until terminal, `gcTime 0`),
`useUnlinkZalo`. Components `zalo-connect-card`, `zalo-link-modal`.

### center (754 L)
Page `CenterPage` — `GET /centers/me` is **role-shaped**: `centerMeSchema` is a `z.union` of
`centerMeOwnerSchema | centerMeMemberSchema`. API `GET|PATCH /centers/me`,
`DELETE /centers/me/members/:teacherId`. Hooks `useCenter`, `useRenameCenter`, `useRemoveMember` (key `centerKeys.me`).
Components `member-list`, `rename-center-dialog`, `remove-member-dialog`.

### invitation (1066 L)
Page `AcceptInvitePage` at `/invite/:token` (public, under AuthLayout).
API `POST|GET /centers/me/invitations`, `DELETE /centers/me/invitations/:id`, `POST /invitations/preview`,
`POST /invitations/accept`. Schemas `invitationSchema`, `createInviteInputSchema`, `createInviteResponseSchema`
(incl. `link`, `dm_status`), `invitePreviewSchema` (`center_name`, `phone_masked`), `acceptInviteFormSchema`.
Hooks `useInvites`, `useCreateInvite`, `useRevokeInvite`, `useInvitePreview` (`staleTime 0, gcTime 0, retry false`
— mirrors the public statement preview), `useAcceptInvite`. Components `invite-section` (exported for the center
page), `invite-list`, `copy-link-dialog`, `invite-error`. lib `format-expiry.ts` → `toLocaleDateString("vi-VN")`.
Anti-enumeration: every rejection collapses to one generic error.

---

## 4. State Architecture

### TanStack Query
Single client in `apps/web/src/app/providers.tsx:9-22`:
```
staleTime: 30_000
retry: (failureCount, error) =>
  error instanceof ApiError && error.status !== null && error.status < 500 ? false : failureCount < 1
```
i.e. **no retry on 4xx, at most 1 retry otherwise**.

Per-query overrides observed: `staleTime 5 min` (`useCurrentPeriod`, `usePeriodForDate`), `30 s` (`useReview`),
`60 s` (`useZaloFriends`), `staleTime 0 + gcTime 0 + retry false` (`useStatement`, `useInvitePreview`,
`useZaloLinkStatus`), `keepPreviousData` on every paginated roster/collections list, `enabled: Boolean(param)`
on every id-scoped query, `refetchInterval` on `useNotificationRun` and `useZaloLinkStatus`.

**Query key convention** — hierarchical factory objects, `all → lists() → list(params)` /
`details() → detail(id)`. Roots: `["roster", "contacts"|"students"|"classes"|"enrollments"]`
(`features/roster/hooks/roster-keys.ts`, all four in one module) · `["attendance","sessions"]` + `roster`,
`period-for-date` (`use-sessions.ts:13`) · `["billing"]` → period/review/blocking-sessions (`use-billing.ts:14`) ·
`["collections"]` → period/list/summary (`use-collections.ts:23`) · `notificationsKeys` (list/run) ·
`dashboardKeys` · `["zalo"]` → status/friends/link (`use-zalo.ts:31`) · `centerKeys.me` · `invitationKeys` ·
`statementKeys`.
Cross-feature invalidation duplicates a literal key with a "keep in sync" comment when the target feature has no
barrel export (see `features/attendance/hooks/use-sessions.ts:26-30` for `dashboardKeys.pendingSessions()`).

### Zustand vs Query vs Context
| Concern | Home |
|---|---|
| Any server data | TanStack Query only — never copied into a store |
| Session (`accessToken`, `user`) | Zustand `features/auth/stores/auth-store.ts` |
| Theme | React context (`components/shared/theme-context.ts` + `theme-provider.tsx`), `localStorage` key `teka-theme` |
| UI-local (dialog open, filters) | `useState` / URL search params (`?view=`, `?status=`) |

Only one Zustand store exists in the whole app.

### Token storage policy
- **Access token: memory only** (`auth-store.ts:9` comment: "never localStorage (XSS surface)").
- **Refresh token: httpOnly cookie**, sent because `apiClient` sets `withCredentials: true`; cookie is scoped to
  `/api/v1/auth` server-side (hence the dev proxy must not rewrite the path).
- On a full reload the access token is gone → `SessionRestore` performs exactly **one** silent
  `POST /auth/refresh` before rendering, showing a centered spinner meanwhile.

### Axios interceptor chain (`src/lib/api/`)
`client.ts` — `axios.create({ baseURL: env.VITE_API_URL, withCredentials: true, timeout: 10_000 })`, then
`setupInterceptors`.

Request interceptor: attach `Authorization: Bearer <token>` from `getAuthBridge()?.getAccessToken()`
unless the config already carries one.

Response error interceptor (`interceptors.ts:55-84`), on `401` with a config that is **not** `_retried` and
**not** a `/auth/*` call: (1) if the store already holds a *newer* token than the request carried (a refresh
settled mid-flight) → retry immediately with it, no new refresh; (2) else if the gate is open
(`!isRefreshDead()`) → `refreshAccessToken()`, **single-flight** via a shared `refreshInFlight` promise (`??=`)
because refresh-token rotation would revoke the whole family on replay — success → `setAccessToken` +
`markRefreshAlive` + retry; failure → `markRefreshDead()` + `clearSession()` → `ProtectedRoute` bounces to
`/login`; (3) anything falling through → `throw toApiError(error)`.

**auth-bridge.ts** is the inversion point: `lib/` never imports feature code; the auth store calls
`connectAuthBridge({getAccessToken,setAccessToken,clearSession})` at module load. It also owns the
module-scoped `refreshDead` gate (reset by `setSession` on a fresh login, and by tests).

**errors.ts** — `class ApiError extends Error { code, status: number|null, fields?: Record<string,string> }`.
`toApiError()`: axios error **with** response → `{code, message, fields}` from the API's
`{success:false, error:{…}}` envelope, defensively (`extractErrorBody` tolerates HTML 502s / empty bodies →
falls back to `UNKNOWN_ERROR` / `"Something went wrong"`); **without** response →
`new ApiError(NETWORK_ERROR, "Cannot reach the server", null)`. `NETWORK_ERROR` is an exported constant.

**envelope.ts** — `parseData(schema, body)`, `parseArray(...)` (endpoints with no `meta`), `parseList(...)` →
`{items, meta}`; `metaSchema = {page, per_page, total, total_pages}`. Every response is zod-parsed so contract
drift fails loudly instead of surfacing as `undefined` deep in a component.

**public-client.ts** — separate axios instance for `/s/:token`: `withCredentials: false`, **no auth
interceptors**, only `toApiError` normalization; base URL = `env.VITE_API_URL.replace(/\/api\/v1\/?$/, "")`.
Carries an explicit "do not 'fix' this by calling setupInterceptors" warning.

**config/env.ts** — zod-validated `import.meta.env`; `VITE_API_URL` must be an absolute URL **or** a
root-relative path (`/api/v1`, but not protocol-relative `//host`). Throws at module load; `main.tsx` catches it
and renders a readable `<pre>` fallback instead of a white screen.

---

## 5. DESIGN SYSTEM — "Học Vui Mỗi Ngày" (hướng "Dịu Mát")

### 5.0 Governing principles

- **The design bundle is the source of truth.** From `adr.md:834` ("Web Design System Foundation"):
  when a phase spec's prose and `_ds_bundle.js` (the design project's original recipe) disagree,
  **the bundle wins** — rule "100% design system". Reconciled deviations are recorded in the ADR so a later
  audit does not reverse them.
- **Token files are copied verbatim** from the design project into `src/styles/tokens/*.css`; they are the
  single source of every value. The Tailwind `@theme inline` bridge in `globals.css` references those custom
  properties and **never hex**.
- **Ownership rule** (`docs/frontend-guidelines.md` §Module layout + `eslint.config.js`):
  - `components/ui/` — generated shadcn primitives. Regenerate, don't hand-tune. Lint exceptions scoped here.
  - `components/hv/` — the "Học Vui Mỗi Ngày" kit; app-facing visual vocabulary; built on tokens (and Radix
    where behavior is needed).
  - `components/shared/` — app-owned generic building blocks (theme, error boundary, page header, data table…).
  - Anything used by 2+ features is promoted into `components/shared` or `lib`.

### 5.1 Color tokens (`src/styles/tokens/colors.css`) — actual values

| Ramp | Tokens |
|---|---|
| Cream (page neutrals, warm green-tinted) | `--cream-50 #fbfdfb` · `--cream-100 #f4f8f3` *(app background)* · `--cream-200 #eaf1ea` · `--cream-300 #dfe9df` |
| Ink (warm green-gray text) | `--ink-900 #1c3a31` *(headings)* · `--ink-700 #27433b` *(body)* · `--ink-500 #5b756c` *(muted)* · `--ink-400 #86998f` *(placeholder/caption)* · `--ink-300 #aab9b1` *(disabled)* · `--white #ffffff` |
| Mint — PRIMARY / môn Toán / success | `50 #e9f7f1` · `100 #d3efe3` · `200 #a9e0cb` · `300 #7fd2b4` · `400 #5cc9a7` *(primary)* · `500 #3fa888` *(pressed/shadow)* · `600 #2e8d6e` *(ink on light)* · `700 #1f6b53` |
| Sky — SECONDARY / Tiếng Việt / info | `50 #eaf5fb` · `100 #d6ecf7` · `200 #a9d8ee` · `300 #7fc8e8` *(secondary)* · `400 #4fa9d4` · `500 #2d7aa0` *(ink)* · `600 #226383` |
| Sun — REWARD / warning | `100 #fff4d6` · `200 #ffe6a3` · `300 #ffd86b` · `400 #ffc83d` *(stars/streak)* · `500 #f0a500` · `600 #b87d00` *(ink on sun)* |
| Coral — lives/tim / error | `100 #ffe7e1` · `300 #ff9b8a` · `400 #ff7a66` · `500 #e85a44` · `600 #b23a28` |
| Lines / borders | `--line-100 #eef4ee` · `--line-200 #e2ece2` *(default border)* · `--line-300 #cfe0d4` *(strong)* |

**Semantic aliases (prefer these in components)** — text: `--text-strong/body/muted/soft/disabled`,
`--text-on-primary #fff`, `--text-on-dark #eafff7`. Surfaces: `--surface-page` (cream-100),
`--surface-card`/`--surface-raised` (white), `--surface-sunken` (cream-200), `--surface-primary-soft` (mint-50),
`--surface-info-soft` (sky-50), `--surface-reward-soft` (sun-100), `--surface-danger-soft` (coral-100),
`--surface-dark #16514c` (celebration/reward screens). Borders: `--border-subtle` (line-200),
`--border-strong` (line-300), `--border-focus` (mint-400). Brand: `--brand-primary/-dk/-ink/-soft`,
`--brand-secondary/-dk/-ink`. States: `--success/-dk/-soft/-ink`, `--info/-soft/-ink`, `--warning/-soft/-ink`,
`--danger/-dk/-soft/-ink`. Subject coding: `--subject-math(-soft)`, `--subject-viet(-soft)`.
Gamification: `--reward-star` (sun-400), `--reward-streak` (coral-400), `--reward-badge` (sky-300).

### 5.2 Typography (`tokens/typography.css`)

- **Families**: `--font-display: "Baloo 2", system-ui, sans-serif` (display/headings) ·
  `--font-body: "Nunito", system-ui, sans-serif`. Both chosen for full Vietnamese diacritic coverage.
  **Self-hosted via `@fontsource`** — `globals.css:8-14` imports Baloo 2 600/700/800 and Nunito 400/600/700/800;
  zero third-party font requests (ADR deviation from the DS's Google Fonts CDN).
- **Weights**: `--fw-regular 400` · `medium 500` · `semibold 600` · `bold 700` · `extra 800` · `black 900`.
- **Scale (px)**: `--text-2xs 12` · `xs 13` · `sm 15` · **`md 17` (base body — larger than web default, kid app)** ·
  `lg 20` · `xl 24` · `2xl 30` · `3xl 38` · `4xl 48` · `5xl 60`.
- **Line heights**: `tight 1.1` · `snug 1.25` · `normal 1.45` · `relaxed 1.6`.
- **Tracking**: `tight -0.01em` · `normal 0` · `wide 0.04em` (buttons/labels).
- **Semantic roles**: display = Baloo/bold/tight; heading = Baloo/semibold/snug; body = Nunito/**semibold**/normal
  ("kids read better at 600"); label = Baloo/semibold/wide.
- Convenience classes `.t-display`, `.t-heading`, `.t-body`, `.t-muted` exist but components mostly use vars.
- **Base layer** (`globals.css:205-246`): `body` = `--surface-page` bg, `--text-body`, Nunito 600 @ 17px,
  `line-height 1.45`, antialiased, `optimizeLegibility`. `h1–h4` = Baloo, `--fw-bold`, snug, `--text-strong`,
  `margin: 0`. `::selection` = mint-200 on ink-900.

### 5.3 Spacing (`tokens/spacing.css`) — base 4px

`--space-0 0` · `1 4` · `2 8` · `3 12` · `4 16` · `5 20` · `6 24` · `7 32` · `8 40` · `9 48` · `10 64` · `11 80`.
Semantic: `--gap-inline` (8) · `--gap-stack` (16) · `--gap-section` (32) · `--pad-screen` (20) · `--pad-card` (20).
Touch: `--touch-min 48px` (absolute floor) · `--touch-kid 64px` (preferred for primary actions).
Containers: `--w-phone 390px` · `--w-content 720px` (dashboard content column) · `--w-page 1080px` (full width).

### 5.4 Radii, shadows, motion (`tokens/effects.css`)

- **Radii**: `--radius-xs 8` · `sm 12` · `md 16` · `lg 20` · `xl 24` · `2xl 32` · `--radius-pill 999px` ·
  `--radius-blob 46% 46% 48% 48% / 52% 52% 46% 46%` (mascot/organic).
  Separately, shadcn's scale is derived in `globals.css:125-131` from `--radius: 20px`:
  `sm ×0.6 = 12, md ×0.8 = 16, lg ×1 = 20, xl ×1.4 = 28, 2xl ×1.8 = 36, 3xl ×2.2, 4xl ×2.6`.
  **Name collision worth documenting**: sm/md/lg agree between the two scales, but `xl` (24 vs 28) and `2xl`
  (32 vs 36) do not. Code uses both notations — `rounded-[var(--radius-xl)]` ×9 (DS value) vs bare
  `rounded-xl` ×5 (Tailwind value, incl. `ui/card` and `ui/dialog`); `rounded-[var(--radius-md)]` ×16,
  `-lg` ×12, `-pill` ×15, bare `rounded-lg` ×13, `rounded-md` ×10. The convention to state:
  **prefer `rounded-[var(--radius-*)]` for hv/app surfaces**, leave bare utilities to generated `ui/`.
- **Soft ambient shadows** (Tailwind: `shadow-soft-*`):
  `--shadow-xs 0 1px 2px rgba(28,58,49,.06)` · `--shadow-sm 0 2px 6px rgba(28,58,49,.08)` ·
  `--shadow-md 0 8px 20px -8px rgba(28,58,49,.18)` · `--shadow-lg 0 18px 40px -18px rgba(28,58,49,.28)` ·
  `--shadow-xl 0 28px 60px -24px rgba(28,58,49,.32)`.
- **Chunky press shadows** (solid bottom edge, Tailwind: `shadow-press-*`):
  `--press-mint 0 5px 0 var(--mint-500)` · `--press-sky 0 5px 0 var(--sky-400)` ·
  `--press-sun 0 5px 0 var(--sun-500)` · `--press-coral 0 5px 0 var(--coral-500)` ·
  `--press-line 0 4px 0 var(--line-300)` · **`--press-depth: 5px`**.
- **Focus rings**: `--ring 0 0 0 4px var(--mint-200)` · `--ring-danger 0 0 0 4px var(--coral-300)`.
- **Motion**: `--ease-soft cubic-bezier(.34,1.56,.64,1)` (overshoot) · `--ease-out cubic-bezier(.22,1,.36,1)` ·
  `--ease-in-out cubic-bezier(.65,0,.35,1)`; `--dur-fast 140ms` · `--dur-base 220ms` · `--dur-slow 360ms` ·
  `--dur-celebrate 600ms`.
- **Keyframes** (`components/hv/hv-animations.css`, the only bespoke CSS in the kit):
  `popIn` (opacity 0→1, scale .96→1), `toastIn` (translateY 12px→0), `slideUp` (translateY 100%→0).
- **Reduced motion**: global kill switch in `@layer base` — `prefers-reduced-motion: reduce` forces
  animation/transition duration to `0.001ms !important` (`globals.css:237-245`).

### 5.5 The focus-visible ring model (an explicit ADR decision)

`--ring` collides between the two systems: the DS defines it as a full box-shadow, shadcn expects a *color*.
Resolution (`globals.css:105-107, 155-156`; `adr.md`):
- `@theme inline { --color-ring: var(--mint-200) }` → shadcn `ring-*` utilities get the mint color.
- `--ring` is **not** redeclared in `:root`, so it stays the DS 4px box-shadow, driving the global
  `:focus-visible { outline: none; box-shadow: var(--ring) }` in `@layer base`.
- `hv-*` components additionally use `focus-visible:ring-4`, which composes with the press shadow.

### 5.6 shadcn semantic bridge (`globals.css:137-171`)

Every shadcn primitive inherits the palette without per-usage overrides:
`--background` cream-100 · `--foreground` ink-700 · `--card`/`--popover` white (fg ink-700) ·
`--primary` mint-400 / fg white · `--secondary` sky-300 / fg ink-900 · `--muted` cream-200 / fg ink-400 ·
`--accent` mint-50 / fg mint-600 · `--destructive` coral-400 · `--border`/`--input` line-200 · `--radius` 20px ·
charts 1-5 = mint-400, sky-300, sun-400, coral-400, mint-600 · sidebar tokens = cream-50 / ink-700 / mint-400 /
mint-50 / line-200 / mint-200.

### 5.7 Dark mode status — **defined but unused**

`.dark { … }` exists (`globals.css:175-201`, mapping onto `--surface-dark #16514c` and `--text-on-dark`) purely so
shadcn's `dark:` variants do not error. The header comment states: "The design system defines no dark mode; this
selector is left present but unused." Evidence: `ModeToggle` (`components/shared/mode-toggle.tsx`) has **zero
usages** anywhere in the app; only **2** `dark:` occurrences across all features/layouts/hv/shared. `ThemeProvider`
still runs (defaults to `system`, persists to `localStorage["teka-theme"]`, toggles the `light`/`dark` class on
`<html>`), so the plumbing is live but no UI exposes it. `docs/frontend-guidelines.md` still claims "both color
schemes" — **stale**.

### 5.8 The `hv-*` component kit (`src/components/hv/`, 1001 L, 16 files)

| Component | Purpose | Variants | Sizes | Notes |
|---|---|---|---|---|
| `HvButton` | Primary action button | `primary` (mint-400/white/`shadow-press-mint`), `secondary` (sky-300/white), `reward` (sun-400/sun-600), `danger` (coral-400/white), `ghost` (white/mint-600 + 2px inset line-200 border) | `sm` 44px min-h, `radius-md`, text-sm · `md` 56px, px-6, text-md · `lg` 64px, px-8, text-lg | `block` prop (full width); `icon`/`iconRight` slots sized `1.15em`; `forwardRef`; `type="button"` default |
| `HvCard` | Surface | `raised` (border line-100 + `shadow-soft-md`), `flat` (border line-200), `sunken` (no border, cream-200) | padding `sm` 12 / `md` `--pad-card` 20 / `lg` 24 | `interactive` → hover `-translate-y-0.5` + `shadow-soft-lg`, active back to `shadow-soft-sm`; radius `--radius-xl` (24px); `forwardRef` |
| `HvBadge` | Topic/state chip | `math`, `viet`, `success`, `info`, `warning`, `danger`, `neutral` | `sm` (9/4px, 12px text) · `md` (11/6px, 13px) | `solid` prop only overrides `math`→mint-400/white and `viet`→sky-300/white; `dot` renders a 7px `bg-current` dot; pill radius, Nunito bold |
| `StatusPill` | Collections payment status | `paid` mint-50/mint-600 · `partial` sun-100/sun-600 · `unpaid` coral-100/coral-600 | one | Labels from `status-pill-labels.ts`: Đã đóng / Đóng thiếu / Chưa đóng — reusable by screens *and* tests |
| `StatPill` | Gamification stat | `star` (sun), `heart` (coral), `streak` (coral+Flame), `time` (sky+Clock) | `md` / `lg` | **Currently unused in app code** (kit completeness) |
| `ProgressBar` | Progress track | color `mint`, `viet`, `reward`, `missing` (coral-400) | `sm` 10px · `md` 14px · `lg` 20px | value clamped 0–100; `role="progressbar"` + `aria-valuenow/min/max`; track cream-200; fill has `inset 0 -3px 0 rgba(0,0,0,.08)`; width transition `--dur-slow`/`--ease-out`; optional `label` + `showValue` head row. `missing` is a Teka addition beyond the bundle (ADR-recorded) |
| `HvModal` | Dialog | — | — | See 5.9 |
| `hvToast` / `useHvToast` | Toast | `default` (ink-900 pill), `success` (mint-600), `danger` (coral-500) | — | Wraps sonner: `position: "bottom-center"`, `duration: 2600`, pill radius, Baloo, `animate-[toastIn]`. Reuses the single `<Toaster/>` in `providers.tsx` — never mount a second |
| `HvIcon` + 8 named wrappers | Icon defaults | `home,check,users,file,send,wallet,x,plus` | — | Lucide with DS defaults `strokeWidth 2`, `size 20`. Named exports `HvHomeIcon`…`HvPlusIcon` |

**Press-depth interaction (the kit's signature)**: every pressable surface carries a *solid* bottom shadow and
drops by exactly `--press-depth` (5px) on `:active`, with the shadow removed — the button visually "lands".
`hv-button.tsx:13-19`: `transition-[transform,box-shadow,filter] duration-[var(--dur-fast)] ease-[var(--ease-out)]`,
`hover:brightness-[1.04]`, `active:translate-y-[var(--press-depth)]`, per-variant `active:shadow-none`.
Disabled/`aria-disabled`: `translate-y-0`, `cursor-not-allowed`, `bg-line-200`, `text-ink-300`, no shadow,
`brightness-100`.
**ADR note**: `ghost` deliberately hardcodes `0 var(--press-depth) 0 var(--line-300)` instead of using
`--press-line` (which is 4px) — a 4px shadow under a 5px translate would leave a 1px gap.

**Kit adoption** (occurrences in `src/features` + `src/layouts`): HvButton 240 · HvCard 108 · HvModal 61 ·
hvToast 48 · HvBadge 40 · StatusPill 4 · ProgressBar 2 · StatPill 0.

### 5.9 `HvModal` — responsive bottom-sheet ↔ centered panel

`src/components/hv/hv-modal.tsx`. Built **directly on `DialogPrimitive`** (radix), not on
`@/components/ui/dialog`'s `DialogContent`, because that primitive hardcodes centered positioning
(`top-1/2 left-1/2 -translate-*`) which cannot be overridden per breakpoint (ADR-recorded).

- **`<sm`**: `fixed inset-x-0 bottom-0 top-auto`, full width, `max-h-[85vh]`, `overflow-y-auto`,
  `rounded-t-[var(--radius-xl)] rounded-b-none`, animation `slideUp` `--dur-base` `--ease-soft`.
- **`sm+`**: `sm:left-1/2 sm:top-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2 sm:max-w-md`,
  `sm:rounded-[var(--radius-xl)]`, animation `popIn` `--dur-base` `--ease-soft`.
- **Overlay**: `bg-[rgba(28,58,49,0.4)]` — the DS scrim (ink-900 @ 40%), replacing shadcn's default; fades in/out
  via `data-open`/`data-closed` `animate-in`/`animate-out`.
- **Close button**: 48×48 (`h-12 w-12`) circular hit target top-right, `text-ink-400`, hover cream-200,
  `focus-visible:ring-4`, `<span className="sr-only">Đóng</span>`.
- **A11y**: always renders a `DialogPrimitive.Title` — a visible `HvModalTitle` (Baloo, `--text-lg`, ink-900,
  `pr-10` for the close button) or an `sr-only` "Hộp thoại" fallback, so the dialog always has an accessible name.
  `aria-describedby` is wired to a `useId()`-generated id only when `description` is passed, otherwise explicitly
  `undefined` to silence Radix's Description warning.
- **Props**: `open`, `onOpenChange`, `title?`, `description?`, `children`, `footer?` (right-aligned action row,
  gap `--space-2`), `className?`. Radix focus trap / esc / portal behavior is retained.

### 5.10 `components/ui/` — generated shadcn primitives (12 files, 1171 L)

`button` (variants default/outline/secondary/ghost/destructive/link; sizes default/xs/sm/lg/icon/icon-xs/icon-sm/icon-lg),
`card`, `dialog`, `dropdown-menu`, `field` (`Field`, `FieldSet`, `FieldLegend`, `FieldGroup`, `FieldLabel`,
`FieldError`, …), `input`, `label`, `select`, `separator`, `skeleton`, `sonner`, `table`.

Actual import counts across `src`: input 22, **field 16**, button 6, dialog 3, sonner 2, skeleton 2, select 2,
table 1, separator 1, label 1, dropdown-menu 1, card 1. → In practice: forms use `ui/field` + `ui/input`;
everything else visual comes from `hv/`.

### 5.11 `components/shared/` (11 files, 523 L)

`app-footer` (public layout only — brand blurb, Sản phẩm column rendered as plain text because those pages don't
exist yet [a dead `href="#"` would lie to AT], Hỗ trợ column with real Zalo/email links, data-minimalism promise
line), `confirm-dialog` (Huỷ / Xác nhận, `destructive` + `pending` props, blocks close while pending),
`data-table` (generic columns/rows, skeleton loading rows, `aria-sort` on sortable headers, pagination footer,
`empty` slot), `empty-state`, `error-boundary` (class component, `getDerivedStateFromError`, "Try again" reset),
`mode-toggle` (**unused**), `not-found`, `page-header`, `spinner` (`<output aria-label="Loading">` + spinning
Loader2), `theme-context` + `theme-provider`.

Note: `data-table`, `empty-state`, `page-header`, `not-found`, `error-boundary` still carry **English** copy
("No results.", "Page X of Y", "Something went wrong", "Back to dashboard") — inconsistent with the
Vietnamese feature UI.

---

## 6. Forms & Validation

Canonical pattern (reference implementation `src/features/auth/pages/login-page.tsx`):

```tsx
const form = useForm<LoginInput>({ resolver: zodResolver(loginSchema), defaultValues: {...} });
const mutation = useLogin();
const handleApiError = useApiFormErrors(form);                       // optional { conflictField }
const onSubmit = form.handleSubmit((v) => mutation.mutate(v, { onSuccess, onError: handleApiError }));
// <form onSubmit={(e) => void onSubmit(e)} noValidate> → <FieldGroup> → per input:
//   <Field data-invalid={…}> <FieldLabel htmlFor="phone"> <Input id="phone" aria-invalid={…}
//     {...form.register("phone")} /> <FieldError errors={[errors.phone]} /> </Field>
// plus a form-level <FieldError errors={[errors.root]} />, submit = <HvButton size="lg" block>
```

- 16 files use `zodResolver`; 12 use `useApiFormErrors`.
- Schemas live in the feature's `schemas/` folder, colocated with the wire types they mirror; each is documented
  with the Go DTO it corresponds to.
- **`lib/forms/use-api-form-errors.ts`** returns an `onError` handler that:
  1. non-`ApiError` → `toast.error("Something went wrong")` *(English — inconsistent)*;
  2. `error.fields` non-empty → `form.setError(field, {type:"server", message})` for every key that exists in
     `form.getValues()`; **unknown keys are not dropped** — they are folded into the form-level `root` error as
     `"<field>: <message>"`;
  3. `error.code === "CONFLICT"` + configured `conflictField` → set on that input (the API reports duplicates as
     CONFLICT with a plain message, not a field-level VALIDATION_ERROR);
  4. otherwise → `form.setError("root", …)`.
- Client validation is kept in **lockstep with the server**: the phone regex mirrors
  `apps/api/internal/shared/validation/validation.go`; password bounds (8..72) mirror `ResetPasswordRequest`.
- Client-only fields (e.g. `confirm_password`) are marked as such and never sent.
- Money inputs parse via feature-local `parseMoney` (strips non-digits) — `roster/lib/roster-format.ts` and
  `collections/lib/money-format.ts` (intentionally duplicated rather than shared; documented in both files).

---

## 7. Accessibility Baseline (what is actually enforced)

| Mechanism | Status |
|---|---|
| `eslint-plugin-jsx-a11y` recommended flat config | Enforced on all `**/*.{ts,tsx}` (`eslint.config.js`), runs in `make lint-web` |
| Skip link | `root-layout.tsx:11-16` → `#main-content`; every layout that renders a page shell owns `id="main-content"` (auth, dashboard `<main>`, public, not-found) |
| Radix primitives | Dialog, DropdownMenu, Select, Label, Separator, Slot — focus trap, esc, portal, roving focus come free |
| Global focus ring | `:focus-visible { outline: none; box-shadow: var(--ring) }` = 4px mint-200; hv components add `focus-visible:ring-4` |
| Reduced motion | Global `prefers-reduced-motion` override in `@layer base` |
| `<nav aria-label="Main">` | On all three nav renderings in `dashboard-layout.tsx` |
| Icon-only controls | `aria-label` + `title` (rail items, period disc, profile disc, ModeToggle) |
| Attribute counts across `src/**/*.tsx` | `role=` 29 · `aria-label` 29 · `aria-hidden` 22 · `aria-pressed` 14 · `sr-only` 11 · `aria-disabled` 9 · `aria-describedby` 4 · `aria-live` 3 · `aria-sort` 1 |
| Touch targets | DS floor `--touch-min 48px`; HvButton md/lg = 56/64px; bottom tabs `min-h-[56px]`; modal close 48×48 |
| Form a11y | `FieldLabel htmlFor` + `Input id`, `aria-invalid`, `data-invalid` on the Field wrapper, `FieldError` |
| Disabled affordances | Non-interactive nav entries use `aria-disabled="true"` on a `<span>` rather than a dead link |
| `lang="vi"` | Set on `<html>` in `index.html` |
| Decorative images | `alt=""` + `aria-hidden="true"` (sidebar logo) |

Not enforced: no automated axe/a11y test run, no contrast audit in CI, dark mode not offered.

---

## 8. Testing

### 8.1 Unit / component — Vitest + Testing Library + MSW (44 test files)

- `vitest.config.ts`: `include: ["src/**/*.test.{ts,tsx}"]` (Playwright owns `e2e/`), `environment: "jsdom"`,
  `setupFiles: ["./src/test/setup.ts"]`, `env.VITE_API_URL = "http://localhost:8080/api/v1"` — needed only so
  `env.ts` boot validation passes; **nothing listens on that port**, MSW intercepts everything.
- `src/test/setup.ts`: `server.listen({ onUnhandledRequest: "error" })` — **any unmocked request fails the test**;
  `afterEach` → `resetHandlers()` + RTL `cleanup()`; `afterAll` → `server.close()`; `beforeEach` resets the
  module-scoped auth state (`useAuthStore.setState({user:null,accessToken:null})`, `markRefreshAlive()`) and
  clears `localStorage` → order-independent tests. jsdom shims Radix/ThemeProvider need: `window.matchMedia`,
  `ResizeObserver` stub, `scrollIntoView`, `hasPointerCapture`/`setPointerCapture`/`releasePointerCapture`.
- `src/test/msw/handlers.ts` (470 L) — happy-path fixtures + envelope builders mirroring the Go API exactly:
  `ok(data, meta?)`, `fail(code, message, fields?)`, `listMeta(total,page,perPage)`; factories `makeTeacher`,
  `makeSession`, `makePendingSession`, `makeClass`, `makeClassSession`, `makePreview`, `makeCollectionsSummary`,
  `makePeriod`; exported `primaryTeacher` ("Cô Lan") / `secondaryTeacher` ("Thầy Minh"); 4 public-statement
  fixtures keyed by token (`valid-token`, `two-child-token`, `cancelled-session-token`, `no-qr-token`) — any
  other token 404s; default `/auth/refresh` returns 401 (a fresh visitor has no session). Tests override per
  case with `server.use(...)`; feature-local handler modules exist too (`features/*/__tests__/*-handlers.ts`).
- `src/test/utils.tsx`: `renderWithProviders(ui, {route, path, extraRoutes})` — **fresh QueryClient per test**
  with `retry: false` on queries *and* mutations (no cache bleed, no retries hiding errors), `ThemeProvider`,
  `createMemoryRouter`, `<Toaster/>`; returns `{...rtlResult, router, queryClient}`. `signInAs(teacher)` →
  `setSession(teacher, "test-access-token")`; re-exports `testPrimaryTeacher` / `testSecondaryTeacher`.
- **Convention** (`docs/frontend-guidelines.md` §Testing): list pages must cover the
  **loading / empty / error / data quartet**. *That doc still names `signInAs(testAdmin | testUser)`, which no
  longer exist.*
- `components/hv/__tests__/`: `hv-button`, `hv-modal`, `progress-bar`, `status-pill` — the design kit is
  unit-tested independently of features.

### 8.2 E2E — Playwright (8 specs, 767 L)

`playwright.config.ts`: `testDir: "./e2e"`, `fullyParallel: false`, **`workers: 1`**, `timeout: 30_000`,
`baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173"`, `trace: "retain-on-failure"`.
**Single worker because the specs mutate a shared seeded database** (users table, billing periods, attendance);
specs also depend on ordering (`collections.spec.ts` pays Chị Hoa in full, so `statement.spec.ts` deliberately
targets Chị Mai whose links still resolve). Requires a running stack + `make seed`.

| Spec | Covers |
|---|---|
| `auth.spec.ts` (130 L) | unauth redirect, server-message on rejected creds, login→reload-persists→logout, client-side phone gate, **exactly one refresh per reload + cookie rotation**, revoked family → exactly one failed refresh, `/s/:token` never pings auth, pending-attendance alert |
| `attendance.spec.ts` (95 L) | mark absentees + one-tap confirm clears dashboard alert; cancel-with-reason bills nobody |
| `billing.spec.ts` (76 L) | close blocked by pending sessions → confirm each → close → locked pill survives reload |
| `collections.spec.ts` (86 L) | two-child family paid in full, persists across reload, reads paid in by-class view |
| `roster.spec.ts` (124 L) | contact → 2 students → class with slot → enroll both → end one |
| `invite-accept.spec.ts` (66 L) | owner invites, invitee accepts in a fresh context and logs in; unknown token → generic error |
| `forgot-password.spec.ts` (56 L) | identical generic confirmation for registered/unregistered (no enumeration), client gate, expired link |
| `statement.spec.ts` (134 L) | mobile viewport 375×667; valid vs invalid token; QR-or-fallback; **error page leaks no digits**; two-child names; **no horizontal overflow at 320px** |

Guidance in `docs/frontend-guidelines.md`: prefer role-based locators; use `exact: true` on cell lookups so row
`aria-label`s don't collide in strict mode.

---

## 9. i18n / Locale Reality

- **No i18n library. All user-facing copy is hardcoded Vietnamese** in JSX. 103 of 155 `.tsx` files under
  `features/`+`components/`+`layouts/` contain Vietnamese diacritics. `<html lang="vi">`.
- **Residual English** (would need cleanup for a "UI is Vietnamese" claim): all of `components/shared/`
  (`data-table` "No results." / "Page X of Y" / "Previous"/"Next", `empty-state`, `not-found` "Page not found" /
  "Back to dashboard", `error-boundary` "Something went wrong"), `spinner` `aria-label="Loading"`,
  `root-layout` "Skip to content", `use-api-form-errors` fallback toast, `ApiError` default messages
  ("Something went wrong", "Cannot reach the server"), `main.tsx` boot failure text.
- **Money** — `lib/utils/format.ts#formatMoney`: `Intl.NumberFormat("vi-VN", {style:"currency", currency:"VND",
  maximumFractionDigits: 0})` → `1500000` → `"1.500.000 ₫"`. Money is a đồng-denominated BIGINT server-side, so it
  always renders as a grouped integer, never a decimal. Reverse parsing via `parseMoney` (strip non-digits).
- **Dates** — `formatSessionDate("2026-07-15")` → `"Th 4, 15/07"`. Deliberately **avoids `Intl.DateTimeFormat`**:
  the vi-VN short-weekday form varies across ICU/CLDR builds ("Th 4" vs "Thứ 4"), so labels are a fixed array
  `["CN","Th 2",…,"Th 7"]` indexed by `getUTCDay`. Parsed/formatted in **UTC** so the calendar day never shifts
  (`session_date` is a DATE column, not an instant). Unparseable input passes through unchanged.
  `statement/lib/format-chip-date.ts` goes further — pure string slicing to `dd/MM`, no `Date` at all.
  `invitation/lib/format-expiry.ts` does use `toLocaleDateString("vi-VN")` → `"19/08/2026"`.
  `roster/lib/roster-format.ts#formatWeekday` — full (`"Thứ 2"`) and short (`"T2"`) label sets, 0 = Chủ nhật.
- **Phone** — `normalizePhone` (`0…` → `+84…`) on the way to the API; `formatPhoneLocal` (`+84…` → `0…`) for
  display. Regex `^(0|\+84)(3|5|7|8|9)\d{8}$`.
- **Names** — `nameInitial(fullName)` takes the **last** whitespace-separated token's first character, because
  Vietnamese names put the given name last ("Nguyễn Thị Lan" → "L"). Used for avatar discs.
- **Timezone** — `teacherSchema.timezone` exists (fixture `"Asia/Ho_Chi_Minh"`); no client-side tz conversion.

---

## 10. Statistics

### Per feature (`.ts` + `.tsx`, tests included)

| Feature | Total L | Test L | Prod L | Files | Pages |
|---|---:|---:|---:|---:|---:|
| roster | 6126 | 1942 | 4184 | 45 | 5 |
| collections | 3540 | 1358 | 2182 | 24 | 2 |
| attendance | 1788 | 611 | 1177 | 15 | 2 |
| billing | 1668 | 450 | 1218 | 16 | 1 |
| profile | 1638 | 615 | 1023 | 17 | 1 |
| invitation | 1066 | 428 | 638 | 16 | 1 |
| auth | 956 | 338 | 618 | 16 | 3 |
| statement | 897 | 265 | 632 | 19 | 1 |
| dashboard | 756 | 226 | 530 | 9 | 1 |
| center | 754 | 320 | 434 | 12 | 1 |
| **features total** | **19189** | **6553** | **12636** | **189** | **18** |

### Other groups (lines / files)

`components/ui` (shadcn) 1171/12 · `components/hv` (design kit, incl. 4 tests + css) 1001/16 ·
`components/shared` 523/11 · `layouts` (incl. 1 test) 616/5 · `lib` 559/14 · `test` 582/4 ·
`styles` (globals + 4 token files) 538/5 · `app` 125/3 · `e2e` (Playwright) 767/8.
**`src` grand total (ts/tsx/css): 24 304 lines.**

Largest single files: `students-page.tsx` 404 · `dashboard-layout.tsx` 409 · `notifications-page.tsx` 384 ·
`attendance-page.tsx` 293 · `class-settings-page.tsx` 272 · `sessions-page.tsx` 264. Unit test files: 44.

---

## Cross-check against existing docs

`docs/frontend-guidelines.md` (103 L) is accurate on module layout, the state split, the API/error contract, the
form pattern, and the testing strategy. Three points are **stale**:
1. "both color schemes (class-based dark mode via the theme provider)" — the theme plumbing exists but no UI
   exposes it and the DS defines no dark mode (see 5.7).
2. `signInAs(testAdmin | testUser)` — actual exports are `testPrimaryTeacher` / `testSecondaryTeacher`.
3. It predates `hv/` — it describes only `ui/` and `shared/`, with no mention of the design-system kit,
   `styles/tokens/*`, or the "bundle is source of truth" rule. It also lists layouts as "root, auth, dashboard",
   omitting `public-layout` and the statement-isolation lint rule.

`adr.md:834` ("Web Design System Foundation") is the authoritative record of DS deviations and must not be
reversed by later audits: ring-vs-focus-shadow split, HvCard `shadow-soft-md`/`-lg`, ProgressBar easing/track +
the added `missing` color, ghost press = 5px via `--press-depth`, HvModal built on `DialogPrimitive`,
self-hosted fonts, dark mode present-but-unused.

---

## Unresolved questions

1. Should the residual English strings in `components/shared/`, `lib/api/errors.ts`, `lib/forms/`, and
   `root-layout.tsx` be translated to Vietnamese, or is English intentional for developer-facing fallbacks?
2. Is `ModeToggle` + `ThemeProvider` intended to stay dead code, or should the theme toggle be removed (and the
   `.dark` block kept only as a shadcn no-op)?
3. `StatPill` has zero consumers and `ProgressBar` only two — should the docs present the whole kit as the
   design vocabulary, or mark the gamification pieces as "available, not yet used"?
4. `docs/frontend-guidelines.md` overlaps heavily with the doc set this report feeds. Should the new frontend
   architecture / design-guidelines docs supersede it, or should it be trimmed to a pointer?
5. Radius naming collision (§5.4): should the DS `--radius-xl`/`--radius-2xl` be renamed, or should the guideline
   simply mandate `rounded-[var(--radius-*)]` in app code? Docs currently state neither.
