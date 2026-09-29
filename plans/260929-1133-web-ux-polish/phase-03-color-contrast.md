---
phase: 3
title: "Color tokens and button contrast"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 3: Color tokens and button contrast

## Goal

Mọi chữ mang thông tin và mọi nút đạt WCAG AA (≥ 4.5:1, chữ lớn ≥ 3:1) mà vẫn giữ
nền thương hiệu mint/sky/coral/sun. Quyết định đã chốt: **chữ `ink-900` trên nền pastel**.

## Files

Không sửa màu trong `features/statement/`: phase 5 xoá cả feature.


- Modify: `apps/web/src/styles/tokens/colors.css`: `--mint-600` → `#24775c`; thêm token `--text-on-brand: var(--ink-900)` và đổi `--text-on-primary` trỏ vào đó.
- Modify: `apps/web/src/components/hv/hv-button.tsx`: đổi màu chữ các biến thể, export `hvButtonVariants`.
- Modify: `apps/web/src/components/hv/index.ts`: export `hvButtonVariants`.
- Modify: `apps/web/src/components/hv/hv-segmented.tsx`: chữ ở trạng thái active.
- Modify (link-button tự dựng → `hvButtonVariants`):
  - `features/billing/components/blocking-sessions-panel.tsx`
  - `features/billing/pages/billing-review-page.tsx`
  - `features/dashboard/components/pending-attendance-alert.tsx`
  - `features/attendance/pages/sessions-page.tsx`
  - `features/roster/pages/students-page.tsx`
  - `features/teaching/components/next-plan-card.tsx`
  - `features/teaching/components/plan-review-panel.tsx`
- Modify (`bg-mint-400 text-white` và tương tự, tìm lại bằng grep ở bước 4):
  `attendance-page.tsx` (header), `period-switcher.tsx`,
  `collections-view-toggle.tsx`, `collections-page.tsx`, `notifications-page.tsx`,
  `weekday-chips.tsx`, `attendance-status-meta.tsx`, `quick-done-button.tsx`,
  `learning-paths-page.tsx`, `hv-badge.tsx`, `dashboard-layout.tsx` (avatar), cùng các chỗ `bg-sky-300`/`bg-coral-400` + `text-white`.
- Modify: các chỗ `text-ink-400` mang **thông tin** (không phải placeholder/disabled) → `text-ink-500`.
- Test: `src/components/hv/__tests__/hv-button.test.tsx` (cập nhật class assertions nếu có), `src/styles/tokens/__tests__/contrast.test.ts` (từ phase 1).

## Design

### Biến thể `HvButton`

| Variant | Nền | Chữ mới | Ratio |
|---|---|---|---|
| primary | `mint-400` | `ink-900` | 6.10 |
| secondary | `sky-300` | `ink-900` | 6.67 |
| danger | `coral-400` | `ink-900` | 4.84 |
| reward | `sun-400` | `ink-900` | 7.99 |
| ghost | `white` | `mint-600` (mới `#24775c`) | 5.43 |
| disabled | `line-200` | `ink-300` | Được miễn theo WCAG 1.4.3 (control bị vô hiệu); giữ nguyên |

Dùng class `text-[var(--text-on-brand)]` (hoặc utility `text-ink-900`) để quy tắc
nằm ở một token: nếu sau này đổi hướng sang "nền đậm + chữ trắng", chỉ sửa token và bảng nền.

### Loại bỏ bản sao tay (DRY)

Có 7 file tự chép chuỗi class của `HvButton` vì `<button>` không lồng được trong
`<Link>`. Export `hvButtonVariants` (cva đã có sẵn) rồi dùng:

```tsx
<Link to={...} className={hvButtonVariants({ variant: "danger", size: "sm" })}>Điểm danh</Link>
```

Như vậy màu nút chỉ có một nguồn sự thật. Đây chính là loại lỗi report tìm ra:
sửa `HvButton` mà quên bản sao. Không thêm prop `asChild` vào `HvButton`: làm vậy
kéo theo Slot và đổi API công khai, trong khi export cva đã đủ.

### `--mint-600`

Đổi hex `#2e8d6e` → `#24775c`: 5.06 trên cream-100, 4.72 trên cream-200, 4.92 trên mint-50, 5.43 trên trắng.
Việc này sửa 142 chỗ `text-mint-600` cùng các alias `--brand-primary-ink`, `--success-ink`, `--accent-foreground` mà không đụng từng file.
Kiểm tra `hv-chip.tsx` (`bg-mint-600` cho môn Toán) và dark mode (`--accent: var(--mint-600)`) vẫn đọc được.

### `ink-400` → `ink-500`

Không đổi hex `--ink-400`: nó vẫn đúng cho placeholder và caption trang trí. Rà
325 chỗ `text-ink-400` và đổi sang `text-ink-500` những chỗ là **nội dung**: nhãn
trạng thái ("Kỳ đang mở"), đếm học sinh, subtitle login, tiêu đề
nhóm sidebar, meta dòng bảng. Giữ `ink-400` cho icon trang trí, placeholder và
separator. Tiêu chí đo: axe `color-contrast` = 0 trên các trang trong DONE contract.
Đổi `--muted-foreground` (shadcn) sang `ink-500` để các component `ui` được sửa mà không phải chỉnh tay `components/ui`.

## Steps

1. Sửa token (`colors.css`, `globals.css` cho `--muted-foreground`).
2. Sửa `hv-button.tsx`, export `hvButtonVariants`, cập nhật test hv.
3. Thay 7 bản sao link-button bằng `hvButtonVariants(...)`; giữ các class layout riêng (vd. `shrink-0`) qua `cn(...)`.
4. `grep -rnE "bg-(mint-400|sky-300|coral-400)[^/]" src | grep text-white` → sửa từng chỗ sang `text-ink-900`. Riêng header attendance: `opacity-90` trên chữ phụ phải được đo lại (ink-900 ở 90% trên mint-400 vẫn > 5:1).
5. Rà `text-ink-400` theo tiêu chí ở trên.
6. Chụp login, dashboard, sessions, billing ở 375/1440 và so với `round-1` bằng mắt/vision: không lệch thương hiệu.

## Verification

- `npx vitest run src/styles/tokens src/components/hv` xanh (contrast test phase 1 chuyển sang pass).
- `grep -rnE "shadow-press-(mint|sky|coral|sun)" src --include='*.tsx' | grep -v hv-button` → không còn chuỗi class nút chép tay.
- `npx playwright test ux-audit -g "color-contrast"` → 0 vi phạm trên login, dashboard, 404, billing (404 sẽ pass hẳn sau phase 5).
- `npm run lint && npm run typecheck && npm test`.

## Risks

- Cảm giác thương hiệu tối hơn: chỉ chữ đổi, nền giữ nguyên; ảnh so sánh ở bước 6 là cổng duyệt.
- Test snapshot/class trong feature có thể assert `text-white`: sửa assertion theo hành vi mới, không nới lỏng.
