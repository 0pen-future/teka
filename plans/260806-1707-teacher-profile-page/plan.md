# Hồ sơ giáo viên page + Đăng xuất trong sidebar footer

Status: done — verified 260806 (vitest 128/128, lint 0 errors, typecheck clean;
code review findings H1/M1–M3 resolved)
Source design: claude.ai/design project `4a7e6c77` — `So Lop - Prototype.dc.html`
(screen `isProfile`, sidebar footer lines 47-59). Snapshot cached at scratchpad
`prototype.html`.

## Outcome

- New route `/profile` renders "Hồ sơ giáo viên" matching the prototype layout,
  card order, labels, and design tokens (lg desktop = 1:1 with prototype).
- Sidebar (lg) footer per prototype: below "Kỳ hiện tại" card, separated by
  `border-t line-200` — profile nav entry (36px mint avatar with initial, tên
  giáo viên, caption "Hồ sơ giáo viên"; active = `bg-mint-50`) then nút
  **Đăng xuất** (hover `bg-cream-200` + chữ coral) wired to existing
  `useLogout`.
- Current top-right LogoutButton in main content removed at lg; at < lg the
  header row stays (profile avatar link + Đăng xuất) so mobile/rail keep both
  actions. Icon rail (md) additionally gets avatar disc → `/profile`.

## Constraints (user-decided 260806)

- **No new DB fields / no migration.** Use current API only: `GET/PUT /me`
  (`full_name` + `timezone`).
- Phone input shows `user_accounts.phone` (from auth store) — **readOnly**
  (PUT /me deliberately excludes phone).
- Subject / bank / account / holder have no backing: render empty, editable
  locally, feed live preview, not persisted. Save persists `full_name` only.
- Zalo state = chưa kết nối (prototype default). "Đăng nhập với Zalo" and
  "Tải toàn bộ dữ liệu (.xlsx)" are UI stubs → `hvToast` notice.
- Message-preview period label from `useCurrentPeriod`, not hardcoded T7.

## Non-goals

Zalo OAuth, xlsx export, phone editing, persisting subject/bank, linked-Zalo
state UI, redesign of md/sm breakpoints beyond keeping logout/profile reachable.

## Files

- `apps/web/src/features/profile/` (new, mirrors feature convention):
  - `api/profile-api.ts` — `updateMe` → `PUT /me`, parse `teacherSchema`.
  - `hooks/use-profile.ts` — `useUpdateMe` mutation → writes teacher back to
    auth store.
  - `pages/profile-page.tsx` — page per prototype (2-column flex, HvCard,
    HvButton, Field/Input, react-hook-form + zod cho full_name).
  - `schemas/profile-schemas.ts` — display-name schema (1..100, mirror
    RegisterRequest binding).
  - `routes.tsx`, `index.ts`.
- `apps/web/src/app/router.tsx` — mount `profileRoutes` in protected children.
- `apps/web/src/features/auth/stores/auth-store.ts` — add `setUser(user)`.
- `apps/web/src/layouts/dashboard-layout.tsx` — sidebar footer block, remove
  lg logout header, rail avatar disc.
- Tests: `features/profile/__tests__/profile-page.test.tsx` (render + save →
  PUT /me via msw + store updated), extend dashboard-layout test if present.

## Steps

1. Auth store `setUser`; profile feature (api/hook/schema/routes/page).
2. Page markup per prototype using tokens (`surface-dark`, `text-on-dark`
   verified in `styles/tokens/colors.css:70,81`); summary line "X lớp · Y học
   sinh" via existing roster/dashboard hooks (`useStudentsTotal`, classes
   list).
3. Router mount; sidebar footer + logout relocation in dashboard-layout.
4. Vitest focused run, then `lint` + `typecheck`.

## Acceptance criteria

1. `/profile` matches prototype structure & tokens; empty-backed fields blank.
2. Lưu hồ sơ → `PUT /me`, sidebar name + dashboard greeting update, toast.
3. Sidebar footer shows profile entry + Đăng xuất per prototype position;
   Đăng xuất calls logout mutation and returns to login.
4. Zalo/export stubs toast; no console errors.
5. Focused tests, `npm run lint`, `npm run typecheck` pass in `apps/web`.

## Implementation notes (post-review)

- `index.ts` intentionally not created — only the router imports the feature
  (matches `features/dashboard`, YAGNI).
- Student count via `useStudentsList({ per_page: 1 })` (shares the dashboard
  query key) instead of `useStudentsTotal`.
- Review fixes applied: phone rendered via `formatPhoneLocal`; `nameInitial`
  unit tests added; empty-name test now asserts zero `PUT /me` calls; preview
  omits dangling "—"/"Chuyển khoản:" khi field trống.
- H1 resolved (user-decided 260806): unbacked fields (Môn dạy, bank card) carry
  a small caption "Chưa lưu trên máy chủ — tính năng đang phát triển" so the
  save toast cannot imply they persisted; accepted minimal prototype deviation.

## Risk / rollback

UI-only + one additive store setter; no API/DB change. Rollback = revert the
web commit.
