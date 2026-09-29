---
phase: 1
title: "UX guard harness"
status: completed
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 1: UX guard harness

## Goal

Mã hoá các check của DONE contract thành test chạy lại được, và chạy nó một lần
để ghi baseline **fail** trước khi sửa. Các phase sau dùng test này làm cổng pass/fail.

## Files

- Create: `apps/web/e2e/ux-audit.spec.ts`
- Create: `apps/web/e2e/helpers/ux-routes.ts` (danh sách route + viewport, dùng chung cho spec)
- Create: `apps/web/src/styles/tokens/__tests__/contrast.test.ts`
- Modify: `apps/web/package.json` (devDependency `@axe-core/playwright`)

## Design

**Vì sao e2e Playwright chứ không unit test cho overflow:** tràn ngang do
`sr-only` absolute thoát khỏi scroll container chỉ xuất hiện khi có layout thật.
jsdom không tính layout nên unit test không bắt được.

**Vì sao contrast unit test riêng:** axe chỉ thấy những gì đang render. Unit test
đọc thẳng `colors.css` và khoá các cặp màu fg/bg của `HvButton` cùng chữ trên
surface, nên một thay đổi token sau này sẽ fail ngay trong `npm test` mà không cần stack.

### `ux-routes.ts`

```ts
export type Viewport = { width: number; height: number };
export const VIEWPORTS = {
  desktop: { width: 1440, height: 900 },
  tablet: { width: 768, height: 1024 },
  phone: { width: 375, height: 812 },
  narrow: { width: 320, height: 720 },
} as const;

export interface UxRoute {
  name: string; // dùng làm tên test và tên ảnh
  path: string | ((page: Page) => Promise<string>); // dynamic: đọc href từ nav
  auth: "owner" | "public";
  narrow?: boolean; // thêm 320×720 (các route liệt kê ở M1)
}
```

- 23 route đã đăng nhập theo `round-1/app-prod/` (home, sessions, classbook,
  records, contacts, classes, classes/recruiting, class-invitations, paths,
  courses, library + 2 tab, billing/:periodId, lesson-plans, students,
  center, center/permissions, center/class-config, audit, tasks, reports, profile).
- `/billing/:periodId` lấy từ `href` của link "Chốt sổ" trong sidebar, không hardcode id.
- Public: `/login`, `/forgot-password`, `/khong-ton-tai` (404). Trang sao kê bị gỡ ở phase 5 nên không audit.
- `narrow: true` cho classes, classbook, lesson-plans, profile (theo bảng M1).

### `ux-audit.spec.ts`

Với mỗi route × viewport, assert:

1. `document.documentElement.scrollWidth - clientWidth === 0`.
2. Đúng 1 phần tử `h1`.
3. `document.title` khác `"Teka"` và kết thúc bằng `" · Teka"`. Thêm một test
   tổng hợp kiểm tra các title đôi một khác nhau.
4. axe với `withRules(["color-contrast", "target-size"])` → 0 violations
   (chỉ chạy ở viewport phone và desktop để giữ thời gian chạy hợp lý).

Screenshot mỗi case vào `test-results/ux-audit/<viewport>-<name>.png` (đã git-ignore)
để phase 7 so với round-1.

### `contrast.test.ts`

- Parse `src/styles/tokens/colors.css` bằng regex `--([a-z0-9-]+):\s*(#[0-9a-f]{6})`.
- Hàm `contrastRatio(fg, bg)` theo công thức WCAG relative luminance, đặt ngay trong file test (không tạo helper production cho thứ chỉ test dùng).
- Bảng cặp phải ≥ 4.5: nền–chữ của từng biến thể `HvButton` (đọc từ đúng token mà phase 3 chọn), `ink-500` trên `cream-100`/`white`/`mint-50`, `mint-600` trên `cream-100`/`white`, `coral-600` trên `coral-100`.
- Viết test theo trạng thái **đích**. Nó sẽ fail cho tới khi phase 3 xong. Đó là
  cố ý (test-first); không `skip`.

## Steps

1. `npm i -D @axe-core/playwright` trong `apps/web`.
2. Viết `ux-routes.ts`, `ux-audit.spec.ts`, `contrast.test.ts` như trên.
3. Dựng stack isolated `teka-e2e` (cổng 55173, fresh seed) theo memory
   `teka-e2e-isolated-stack`; không dùng stack dev của người dùng. Kiểm tra stack
   có sẵn trước khi dựng mới; ghi lại project/cổng; `down -v` khi xong phase 7.
4. Chạy `E2E_BASE_URL=http://localhost:55173 npx playwright test ux-audit` → lưu output vào
   `plans/260929-1133-web-ux-polish/reports/phase-01-baseline.txt`. Kỳ vọng: fail
   đúng các route trong bảng tràn của report, fail contrast, fail title/h1.

## Verification

- `npx vitest run src/styles/tokens` chạy được (đang fail có chủ đích ở các cặp nút).
- Baseline e2e liệt kê fail khớp report (route tràn, h1 billing, title "Teka").
- `npm run lint && npm run typecheck` xanh.

## Risks

- Thời gian chạy: ~27 route × 3–4 viewport. Chỉ chạy axe ở 2 viewport; spec dùng
  một lần login cho mọi route (`storageState`), không login lại mỗi case.
- Spec thuộc project `desktop` của Playwright (tên không đuôi `-mobile`), viewport đặt tay qua `page.setViewportSize`.
