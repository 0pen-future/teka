---
phase: 4
title: "Responsive overflow"
status: completed
priority: P1
effort: "1d"
dependencies: [1, 3]
---

# Phase 4: Responsive overflow

## Goal

Không route nào tràn ngang ở 375/768 (và 320 cho các route M1). Bảng rộng vẫn cuộn
ngang **bên trong** wrapper của nó, và không tái phát khi thêm bảng mới.

## Root cause (đã chứng minh trong report)

`span.sr-only` (`position:absolute`) nằm trong `th` của bảng `min-w-[760–1000px]`.
Wrapper `overflow-x-auto` không phải positioned, nên containing block của span là
`html`. Span nằm ở left≈889px ngoài vùng cuộn và kéo `html.scrollWidth` lên.
Chỉ 1/19 wrapper có `relative`.

## Files

- Create: `apps/web/src/components/hv/hv-table-scroll.tsx`, export trong `components/hv/index.ts`
- Create: `apps/web/src/components/hv/__tests__/hv-table-scroll.test.tsx`
- Modify (thay wrapper `overflow-x-auto` bọc bảng bằng `HvTableScroll`):
  - `features/roster/components/class-table.tsx`
  - `features/roster/pages/class-invitations-page.tsx`
  - `features/roster/components/class-students-tab.tsx`, `class-sessions-tab.tsx`, `classes-tab.tsx`, `roster-table.tsx`
  - `features/courses/pages/courses-page.tsx`, `learning-paths-page.tsx`, `course-detail-page.tsx`, `features/courses/lib/table-classes.ts`
  - `features/library/components/materials-bank.tsx`, `exercises-bank.tsx`, `lessons-table.tsx`, `score-sets-editor.tsx`, `version-materials-tab.tsx`, `version-exercises-tab.tsx`, `lesson-exercises.tsx`
  - `features/center/components/permission-matrix.tsx`, `class-score-set-table.tsx`
  - `features/teaching/components/sessions-table.tsx`, `review-queue-table.tsx`, `student-records-table.tsx`
  - `features/audit/components/audit-table.tsx`, `features/billing/components/review-table.tsx`
  - `components/shared/data-table.tsx`
- Modify: `apps/web/src/components/hv/hv-segmented.tsx`
- Modify: các trang còn tràn ở 320: classbook, lesson-plans, profile (xác định phần tử thủ phạm bằng spec phase 1)

Không sửa `components/ui/table.tsx` (shadcn, cấm sửa tay). Nếu `data-table.tsx`
dùng `ui/table` thì bọc ở tầng `data-table`.

## Design

### `HvTableScroll`

```tsx
export interface HvTableScrollProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Accessible name for the scroll region so keyboard users can find and scroll it. */
  "aria-label": string;
}

/**
 * Horizontal scroll region for wide tables. `relative` makes this the containing
 * block for absolutely positioned descendants (e.g. `sr-only` header labels), so
 * they are clipped by the scroll box instead of widening the document.
 */
export function HvTableScroll({ className, ...rest }: HvTableScrollProps) {
  return (
    <div
      role="region"
      tabIndex={0}
      className={cn(
        "relative max-w-full overflow-x-auto overscroll-x-contain focus-visible:ring-4",
        className,
      )}
      {...rest}
    />
  );
}
```

- `role="region"` + `tabIndex=0` + `aria-label`: vùng cuộn truy cập được bằng bàn phím (axe `scrollable-region-focusable`).
- **Vì sao dùng primitive chứ không chỉ thêm `relative`:** 18 chỗ cần sửa và lỗi sẽ tái phát ở bảng thứ 20. Primitive mã hoá bất biến; phase 7 thêm quy tắc "bảng rộng luôn bọc `HvTableScroll`" vào guidelines.
- `features/courses/lib/table-classes.ts` đang export chuỗi class wrapper → xoá phần wrapper khỏi đó, dùng component.

### Empty state của `/courses`

Hiện "Không có khóa nào khớp." nằm trong một `td colSpan` giữa bảng 1000px, nên ở 375 nó nằm ngoài viewport.
Render empty state **thay cho** bảng (dùng `HvStateBlock`) khi danh sách rỗng, không đặt trong bảng.
Áp dụng tương tự cho bảng khác có empty-row (grep `colSpan` + chuỗi "Không có").

### `HvSegmented`

Container đang là `inline-flex` với item `whitespace-nowrap`, nên 3 tab của library đẩy nút tới x=491.
Container thêm `max-w-full overflow-x-auto snap-x`, item thêm `snap-start shrink-0`.
Ẩn scrollbar nhưng vẫn cuộn được. Với `block`, giữ `flex-1` khi đủ chỗ.
Không cho xuống dòng: segmented xuống dòng làm vỡ hình "một hàng nút" và phá thứ tự mũi tên của tablist.
Thêm test: khi active item nằm ngoài vùng nhìn thấy thì gọi `scrollIntoView({ inline: "nearest", block: "nearest" })` lúc đổi giá trị.

## Steps

1. Viết `HvTableScroll` + test (render region, có `relative`, forward props).
2. Thay wrapper ở từng file trong danh sách; đặt `aria-label` tiếng Việt theo nội dung bảng ("Bảng danh sách lớp", …).
3. Sửa empty state `/courses` và các bảng tương tự.
4. Sửa `HvSegmented`.
5. Chạy `ux-audit` ở 320/375/768; với các trang còn tràn, dùng snippet sau trong spec debug để tìm thủ phạm rồi sửa tại gốc (thường là `min-w-*` cố định hoặc `whitespace-nowrap` trong flex):
   ```js
   [...document.querySelectorAll("body *")].filter(
     (e) => e.getBoundingClientRect().right > innerWidth + 1,
   );
   ```

## Verification

- `npx playwright test ux-audit -g "overflow"` → 0 ở mọi route/viewport trong DONE contract.
- Test thêm trong `ux-audit`: ở 375, bảng `/classes` có `scrollWidth > clientWidth` bên trong region (vẫn cuộn được), và "Không có khóa nào khớp." nằm trong viewport khi lọc rỗng.
- `grep -rn "overflow-x-auto" src --include='*.tsx' | grep -v hv-table-scroll` chỉ còn các chỗ không phải bảng (board-desktop, score-bar-chart), mỗi chỗ đã kiểm tra.
- `npm run lint && npm run typecheck && npm test`.

## Risks

- `tabIndex=0` thêm một điểm dừng tab mỗi bảng: đúng khuyến nghị WCAG cho vùng cuộn, chấp nhận được.
- Bảng có header sticky có thể đổi hành vi khi wrapper thành `relative`: kiểm tra `review-table` và `permission-matrix` trên desktop.
