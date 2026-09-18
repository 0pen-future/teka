---
phase: 2
title: "Kit hv: HvModal stickyFooter, HvSegmented nowrap, HvChip"
status: pending
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 2: Kit hv — `stickyFooter` cho HvModal, `whitespace-nowrap` cho HvSegmented, `HvChip`

## Context Links

- Report mục "Thay đổi kit": `hv-modal.tsx` (footer dính cho md/lg), `hv-segmented.tsx` (`whitespace-nowrap`), `hv-badge.tsx` (giữ, dùng `dot`)
- Quyết định D6 trong [plan.md](./plan.md)
- `apps/web/src/components/hv/hv-modal.tsx` (`sizeClassName`, body `min-h-0 flex-1 overflow-auto` và footer `shrink-0` chỉ khi `size === "xl"`)
- `apps/web/src/components/hv/hv-segmented.tsx` (`itemClassName` chưa có nowrap)
- `apps/web/src/components/hv/hv-badge.tsx` (prop `dot`), `hv-button.tsx` (5 variant, size sm 44px)
- Test kit: `apps/web/src/components/hv/__tests__/{hv-modal,hv-segmented}.test.tsx`
- Token: `apps/web/src/styles/tokens/colors.css` (ink-500 `#5b756c` đạt tương phản; ink-400 không)

## Overview

Ba thay đổi nhỏ, additive, để Phase 3 và 5 không phải viết CSS ad-hoc:

1. `HvModal` prop `stickyFooter?: boolean` — layout cột (`flex-col`, thân cuộn,
   footer `shrink-0`) cho **mọi size** khi bật; `xl` giữ hành vi cũ (tương đương
   luôn bật).
2. `HvSegmented` item `whitespace-nowrap` — sửa kit chung (chặn nhãn dài bị bẻ
   dòng). Lưu ý: theo D11 trang bảng **bỏ** segmented phạm vi, nên sửa này không
   còn là điều kiện của Phase 4/5; giữ vì nhỏ và đúng cho mọi consumer khác.
3. `HvChip` — nút chip bật/tắt (`pressed`), chấm màu tuỳ chọn (`dot` theo
   `HvBadgeVariant`), số đếm tuỳ chọn (`count`), min-height 40px. Dùng cho: chip
   ưu tiên + chip hạn nhanh (Modal A), dải lọc + chip giáo viên (Bảng B).

## Key Insights

- Panel HvModal hiện có `overflow-y-auto max-h-[85vh]` (bottom sheet) — khi
  footer dính, panel phải `overflow-hidden` và thân mới là vùng cuộn, nếu không
  footer cuộn theo.
- Radix Dialog gọi `onOpenChange(false)` khi Escape/click overlay — Phase 3 chặn
  đóng khi dirty ở tầng consumer, kit không cần đổi.
- `HvButton` không có "text-danger"; Phase 3 dùng `variant="ghost"` + class coral
  (D6), không thêm variant.

## Requirements

- `stickyFooter` mặc định `false`; khi `true` hoặc `size === "xl"`: panel
  `flex max-h-[95dvh] flex-col overflow-hidden sm:max-h-[90dvh]`, body
  `min-h-0 flex-1 overflow-auto`, footer `shrink-0` (giữ `max-w` theo size).
- Snapshot/hành vi các modal hiện có (không truyền prop) không đổi.
- `HvChip`: `type="button"`, `aria-pressed` khi dùng như toggle, hoặc
  `role="radio"` + `aria-checked` khi cha truyền `role`; focus ring
  `focus-visible:ring-4 ring-mint-100`; kích cỡ `min-h-10 px-3 text-[13px]`;
  pressed = `bg-ink-900 text-white` (hoặc `bg-mint-50 text-mint-600` cho tone
  nhẹ — chọn một, dùng thống nhất); `dot` render chấm 7px màu theo variant badge.

## Related Code Files

Sửa
- `apps/web/src/components/hv/hv-modal.tsx`, `hv-segmented.tsx`, `index.ts` (export `HvChip`)
- `apps/web/src/components/hv/__tests__/hv-modal.test.tsx`, `hv-segmented.test.tsx`

Tạo mới
- `apps/web/src/components/hv/hv-chip.tsx`, `__tests__/hv-chip.test.tsx`

## Implementation Steps

1. `hv-modal.tsx`: thêm `stickyFooter?: boolean` vào props `HvModal` và
   `HvModalContent`; tính `const scrollsBody = size === "xl" || stickyFooter`;
   tách class layout ra khỏi `sizeClassName.xl` thành `layoutClassName` áp khi
   `scrollsBody`; body/footer dùng `scrollsBody` thay `size === "xl"`. Thêm
   `data-sticky-footer` để test.
2. `hv-modal.test.tsx`: test mới — `stickyFooter` với size md: footer có
   `shrink-0`, body có `overflow-auto`; không truyền prop → không có.
3. `hv-segmented.tsx`: thêm `whitespace-nowrap` vào `itemClassName`; test kiểm
   class có mặt trên item.
4. `hv-chip.tsx`: props `pressed?: boolean`, `dot?: HvBadgeVariant`,
   `count?: number`, `size?: "sm" | "md"`, spread rest lên `<button>`; nếu
   `props.role === "radio"` thì dùng `aria-checked` thay `aria-pressed`. Chấm
   dùng cùng bảng màu với `HvBadge` (tái sử dụng map variant → class).
5. `hv-chip.test.tsx`: render pressed/unpressed, dot có `aria-hidden`, count hiện
   đúng, `role="radio"` chuyển sang `aria-checked`.
6. Export từ `index.ts`; `npm run lint && npm run typecheck && npm run test -- hv-`
   trong `apps/web` (hoặc `make lint-web test-web`).

## Todo

- [ ] `HvModal.stickyFooter` + test
- [ ] `HvSegmented` nowrap + test
- [ ] `HvChip` + test + export
- [ ] `make lint-web test-web` xanh

## Success Criteria

- Modal md với `stickyFooter` và nội dung dài: tiêu đề + footer cố định, thân cuộn (kiểm thủ công ở 375px và 1280px).
- Không test kit hiện có nào đổi kết quả.
- `HvChip` có test bao phủ toggle/radio/dot/count.

## Risk Assessment

- Tách `layoutClassName` khỏi `xl` có thể đổi thứ tự class → chạy toàn bộ
  `hv-modal.test.tsx` và mở nhanh một modal xl hiện có (ví dụ modal dùng
  `size="xl"`, grep `size="xl"` trong `src/features`).

## Security Considerations

Không có (thuần trình diễn).

## Next Steps

Phase 3 dùng `stickyFooter` + `HvChip`; Phase 5 dùng `HvChip`. Không phase nào phụ thuộc segmented nowrap.
