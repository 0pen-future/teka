---
title: "Restyle Lớp & học sinh + Điểm danh theo prototype design system"
date: 2026-08-05
summary: "Restyle 2 màn hình theo prototype Sổ Lớp với sticky table header, viewport-fit, panel điểm danh bo 28; fix focus ring + robustness sau code review"
---

# Restyle Lớp & học sinh + Điểm danh theo prototype design system

## What happened

Restyle hai màn hình `apps/web` theo prototype claude.ai/design `4a7e6c77` ("Học Vui Mỗi Ngày" design system), 6 file UI, không đổi backend:

- **Lớp & học sinh** (`students-page.tsx`): bảng card bo 20px với header `cream-200` sticky (`th` sticky top-0 trong div `overflow-auto`, card `overflow-hidden` giữ góc bo), thân bảng cuộn trong viewport (`sm:h-[calc(100svh-158px)]` per-breakpoint, offset mirror chrome của DashboardLayout), cột Người liên hệ gộp tên + SĐT, tab pill press-mint, search pill native (bỏ shadcn Input).
- **Điểm danh** (`sessions-page`, `attendance-page`, 3 component): danh sách buổi trong card trắng bo 20, hàng buổi tint màu theo trạng thái đúng prototype, panel bo 28 với header mint-400 + pill đếm Có mặt/Vắng, chip học sinh bo 16 với vòng ✓/✕ 34px bên trái và nhãn trạng thái `aria-hidden` bên phải (giữ accessible name = tên học sinh cho test selectors).

## Notable

- Chrome MCP không truy cập được `localhost:5173` (mở được trang ngoài nhưng error page với mọi địa chỉ local, kể cả 127.0.0.1) → chuyển sang script Playwright của repo để chụp visual check desktop 1440 + mobile 390; xác nhận sticky header và panel render đúng.
- Bẫy focus ring: ring toàn cục là `box-shadow` trong `@layer base`, mọi utility `shadow-*` ở layer utilities đè mất → tab pill phải tự thêm `focus-visible:ring-4` (cùng pattern HvButton). Code review bắt được, đã fix cả 2 màn.
- Fix thêm sau review: `min-h-[240px]` chống card bảng bị bóp khi tab wrap; `max-h-[430px]` list điểm danh chỉ áp `lg+` (mobile giữ 1 trục cuộn); hoist `PanelHeader` dùng chung nhánh live/huỷ; `aria-label` cho ô search.
- Giữ nguyên màu tint prototype dù dưới WCAG AA (ink-400 ~3.0:1) — spec prototype user đã chốt "100% design system", trade-off trình user quyết.

## Verification

`tsc` sạch, eslint sạch, vitest 24 files / 104 tests pass. E2E `roster.spec.ts`/`attendance.spec.ts` fail pre-existing: spec bám route `/classes` đã xoá ở commit 1161903, và DB dev dùng chung có buổi pending thuộc lớp roster rỗng + hết buổi "Sắp diễn ra" chưa huỷ.

## Next steps

- User quyết: giữ màu prototype hay nâng tương phản (chi tiết `plans/260805-1435-design-system-screens/reports/code-review.md`).
- Cập nhật e2e specs theo refactor class-management + seed lại DB dev.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
