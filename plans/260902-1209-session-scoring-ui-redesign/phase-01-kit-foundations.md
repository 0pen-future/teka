---
phase: 1
title: "Kit foundations"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Kit foundations

## Overview

Bổ sung vào `components/hv` sáu primitive mà hai màn chấm điểm/bộ điểm cần và
review tổng (C2, C3, C4, C8) cũng yêu cầu: `HvModal` prop `size`, `HvScoreInput`,
`HvSegmented`, `HvNotice`, `HvConfirmDialog`, `HvStateBlock`. Không sửa màn nào
ở phase này; mọi primitive có test riêng trong `components/hv/__tests__`.

## Requirements

- Functional:
  - `HvModal` nhận `size?: "md" | "lg" | "xl"` (mặc định `md`, giữ nguyên markup hiện tại để test `hv-modal.test.tsx` không đổi).
  - `HvScoreInput`: input điểm 0–10 bước 0.5, `type="text" inputmode="decimal"`, 44px, parse bằng `parseScoreInput` khi blur, hiển thị trạng thái `dirty | saved | invalid`, `aria-invalid` + text lỗi khi không parse được.
  - `HvSegmented`: nhóm nút chọn một (radio group của Radix hoặc `role=tablist` khi dùng làm tab), 44px, biến thể `variant: "segmented" | "tabs"`. Variant `tabs` nhận `idBase` và gắn `id="{idBase}-tab-{value}"` + `aria-controls="{idBase}-panel-{value}"` cho từng tab; caller tự bọc nội dung bằng `role="tabpanel" id="{idBase}-panel-{value}" aria-labelledby="{idBase}-tab-{value}"`.
  - `HvNotice`: khối thông báo tĩnh `tone: info | warning | danger | success`, icon + title tuỳ chọn + nội dung; prop `role?: "note" | "alert" | "status"` ghi đè mặc định theo tone (danger → alert, còn lại → note) để caller giữ được `role="alert"` cho thông điệp 409 màu warning.
  - `HvConfirmDialog`: dựng trên `HvModal`, props `open/onOpenChange/title/description/confirmLabel/cancelLabel/tone (default|danger)/onConfirm/pending`.
  - `HvStateBlock`: `state: "loading" | "empty" | "error"`, `title`, `description?`, `action?` (HvButton), có `role="status"` cho loading, `role="alert"` cho error.
- Non-functional:
  - Chỉ dùng token trong `src/styles/tokens`; không thêm màu/bán kính.
  - Mỗi primitive export từ `components/hv/index.ts`.
  - Không dùng `asChild`; giữ style HvButton hiện có.

## Architecture

```
components/hv/
  hv-modal.tsx        + size map → className của Dialog.Content
  hv-score-input.tsx  controlled: value:string, onCommit(parsed:number|null|"invalid"), state
  hv-segmented.tsx    RadioGroup.Root (variant segmented) | div role=tablist (variant tabs)
  hv-notice.tsx       div role=note|alert theo tone
  hv-confirm-dialog.tsx  HvModal size=md + footer 2 HvButton
  hv-state-block.tsx  HvCard flat + icon + copy + action
```

`HvScoreInput` không tự lưu; nó chỉ chuẩn hoá và báo về cho caller (`useScoreDraft` ở Phase 3).
`parseScoreInput` hiện nằm trong `features/teaching/lib/classbook-stats.ts:243` (test ở
`features/teaching/__tests__/classbook-stats.test.ts`) và được `component-score-grid.tsx` +
`session-detail-panel.tsx` dùng. Kit không được import từ `features`, nên tạo bản mới
`components/hv/score-input-parse.ts` với contract mở rộng (`"invalid"`); Phase 2 đổi hai
caller trong panel sang bản mới; Phase 3 đổi/xoá grid rồi mới xoá hàm cũ và chuyển test còn lại sang `score-input-parse.test.ts`.

**Contract parse chặt hơn hàm cũ**: hàm cũ dùng `parseFloat` nên `"7abc"` → 7 và `"1e1"` → 10. Bản mới trim rồi khớp `^\d{1,2}([.,]\d+)?$`; không khớp → `"invalid"`; chuỗi rỗng/chỉ khoảng trắng → `null`. Đây là thay đổi hành vi có chủ ý (chữ vô nghĩa báo lỗi thay vì bị hiểu là 7).

`HvButton.icon` là `React.ReactNode` (`hv-button.tsx:69`), **không** phải `HvIconName`: mọi caller truyền `icon={<HvIcon name="x" />}`; truyền chuỗi vẫn typecheck xanh nhưng render chữ.

Size map cho `HvModal` (chỉ áp từ `sm:`; dưới `sm` mọi size là bottom sheet):

| size | class |
|------|-------|
| md | `sm:max-w-md` (giữ nguyên) |
| lg | `sm:max-w-[720px]` (`--w-content`) |
| xl | `sm:max-w-[var(--w-page)] sm:h-[90dvh]`; body `flex-1 min-h-0 overflow-auto` |
| xl, dưới `sm` | vẫn là bottom sheet nhưng `max-h-[95dvh]` thay cho `85vh` mặc định (D6) |

## Related Code Files

| Action | File | Ghi chú |
|--------|------|---------|
| Modify | `apps/web/src/components/hv/hv-modal.tsx` | thêm `size`, phần body `overflow-auto` cho xl |
| Modify | `apps/web/src/components/hv/index.ts` | export 5 primitive mới + `parseScoreInput` |
| Modify | `apps/web/src/components/hv/hv-icon.tsx` | file là registry re-export `lucide-react` (`iconRegistry` + union `HvIconName`, hiện có home/check/clock/users/file/send/wallet/x/plus): thêm `arrow-up: ArrowUp`, `arrow-down: ArrowDown`, `trash: Trash2`, `table: Table`, `info: Info`, `alert: AlertTriangle`; không vẽ path SVG tay |
| Create | `apps/web/src/components/hv/score-input-parse.ts` | `parseScoreInput(raw): number \| null \| "invalid"` (mở rộng: trả `"invalid"` khi không phải số, thay vì `null`) |
| Create | `apps/web/src/components/hv/hv-score-input.tsx` | |
| Create | `apps/web/src/components/hv/hv-segmented.tsx` | |
| Create | `apps/web/src/components/hv/hv-notice.tsx` | |
| Create | `apps/web/src/components/hv/hv-confirm-dialog.tsx` | |
| Create | `apps/web/src/components/hv/hv-state-block.tsx` | |
| Create | `apps/web/src/components/hv/__tests__/hv-score-input.test.tsx` | |
| Create | `apps/web/src/components/hv/__tests__/hv-segmented.test.tsx` | |
| Create | `apps/web/src/components/hv/__tests__/hv-confirm-dialog.test.tsx` | |
| Create | `apps/web/src/components/hv/__tests__/hv-state-block.test.tsx` | |
| Create | `apps/web/src/components/hv/__tests__/score-input-parse.test.ts` | |
| Modify | `apps/web/src/components/hv/__tests__/hv-modal.test.tsx` | thêm case `size="xl"` |
| Modify | `docs/frontend-guidelines.md` | thêm mục ngắn về hv kit: danh sách primitive và khi nào dùng HvStateBlock/HvNotice/HvConfirmDialog |

## Interfaces

```ts
// hv-modal.tsx
type HvModalProps = { ...existing; size?: "md" | "lg" | "xl" };

// score-input-parse.ts
export type ParsedScore = number | null | "invalid";
export function parseScoreInput(raw: string): ParsedScore;
// "" → null; "7,5" → 7.5; "12" → 10; "7.3" → 7.5; "abc" → "invalid"

// hv-score-input.tsx
type HvScoreInputProps = {
  id?: string;
  value: string;                   // chuỗi thô do caller giữ
  onChange: (raw: string) => void;
  onCommit: (parsed: ParsedScore, raw: string) => void; // gọi khi blur/Enter
  state?: "idle" | "dirty" | "saved" | "invalid";
  disabled?: boolean;
  "aria-label": string;
  onNavigate?: (dir: "up" | "down") => void; // Enter/Shift+Enter, caller quyết định focus
  inputRef?: React.Ref<HTMLInputElement>;
  size?: "sm" | "md"; // 44 | 48
};

// hv-segmented.tsx
type HvSegmentedOption<T extends string> = { value: T; label: React.ReactNode; icon?: HvIconName; disabled?: boolean };
type HvSegmentedProps<T extends string> = {
  value: T; onValueChange: (v: T) => void; options: HvSegmentedOption<T>[];
  variant?: "segmented" | "tabs"; idBase?: string; // bắt buộc khi variant="tabs"
  "aria-label": string; block?: boolean; className?: string;
};

// hv-notice.tsx
type HvNoticeProps = { tone?: "info" | "warning" | "danger" | "success"; role?: "note" | "alert" | "status"; title?: string; children: React.ReactNode; icon?: HvIconName; className?: string };

// hv-confirm-dialog.tsx
type HvConfirmDialogProps = {
  open: boolean; onOpenChange: (o: boolean) => void; title: string; description?: React.ReactNode;
  confirmLabel: string; cancelLabel?: string; tone?: "default" | "danger"; pending?: boolean; onConfirm: () => void;
};

// hv-state-block.tsx
type HvStateBlockProps = { state: "loading" | "empty" | "error"; title: string; description?: string; action?: React.ReactNode; compact?: boolean };
```

## Implementation Steps

1. Tạo `score-input-parse.ts` theo logic hàm trong `classbook-stats.ts` (thay dấu phẩy, clamp 0–10, làm tròn 0.5) nhưng khớp regex thay `parseFloat`; thêm nhánh `"invalid"`; chưa xoá hàm cũ; viết test bảng giá trị (`""`→null, `"  "`→null, `"7"`→7, `"7,5"`→7.5, `"7.3"`→7.5, `"-1"`→invalid, `"12"`→10, `"abc"`→invalid, `" 8 "`→8, `"7abc"`→invalid, `"7,5,5"`→invalid, `"1e1"`→invalid).
2. Thêm `size` vào `HvModal`; xl dùng `flex flex-col` với header/footer cố định, body cuộn. Bổ sung test: `size="xl"` có class `sm:h-[90dvh]`, `size` mặc định không đổi snapshot class.
3. `HvScoreInput`: `type="text" inputmode="decimal"`, `min-h-11 w-full text-center text-[length:var(--text-md)] font-semibold tabular-nums`; state map: dirty → `bg-sun-100 border-sun-400`, saved → `border-mint-400`, invalid → `border-coral-400 aria-invalid`; hiển thị text lỗi "Điểm 0–10, bước 0,5" qua `aria-describedby` khi invalid. Enter/Shift+Enter gọi `onNavigate`; Enter cũng commit.
4. `HvSegmented`: variant `segmented` bằng `RadioGroup.Root/Item` từ `radix-ui`; variant `tabs` bằng `role="tablist"`/`role="tab"` `aria-selected` với điều hướng mũi tên trái/phải. Cả hai chung style: container `rounded-[var(--radius-md)] bg-cream-100 p-1`, item `min-h-11 rounded-[calc(var(--radius-md)-4px)]`, active `bg-white shadow-sm text-ink-900`.
5. `HvNotice`: `rounded-[var(--radius-md)] border p-[var(--space-3)] text-[length:var(--text-sm)]`, tone map dùng sky/sun/coral/mint 50/200/700. Mặc định `tone=danger` → `role="alert"`, còn lại `role="note"`; prop `role` ghi đè.
6. `HvConfirmDialog`: bọc `HvModal size="md"`, footer `HvButton variant="ghost"` (huỷ, theo quy ước nút huỷ hiện có) + `HvButton variant={tone==="danger"?"danger":"primary"}` (xác nhận, `disabled={pending}`), focus mặc định vào nút huỷ.
7. `HvStateBlock`: loading dùng `Skeleton` từ `components/ui/skeleton` (3 dòng) hoặc spinner nhỏ, `role="status" aria-live="polite"`; empty dùng HvIcon + copy; error `role="alert"` + action "Thử lại".
8. Thêm sáu icon vào `hv-icon.tsx` (Phase 2 dùng arrow/trash, Phase 4 dùng table, HvNotice dùng info/alert). Export tất cả từ `index.ts`; chạy `npm run lint && npm run typecheck && npx vitest run src/components/hv`.

## Test scenarios

| Component | Case | Assertion |
|-----------|------|-----------|
| parseScoreInput | 12 giá trị bảng | kết quả đúng như bước 1 |
| HvModal | size xl | Content có `sm:h-[90dvh]`; md không có |
| HvScoreInput | gõ "7,5" rồi blur | `onCommit(7.5, "7,5")` |
| HvScoreInput | gõ "abc" rồi blur | `onCommit("invalid")`; input `aria-invalid="true"`; text lỗi hiện |
| HvScoreInput | Enter / Shift+Enter | `onNavigate("down")` / `("up")`; Enter cũng commit |
| HvScoreInput | state dirty | có `data-state="dirty"` |
| HvSegmented segmented | click option 2; ArrowRight | `onValueChange` đúng; role radio |
| HvSegmented tabs | role tab, `aria-selected`, `id`/`aria-controls` theo `idBase` | đúng item active |
| HvNotice | `tone="warning" role="alert"` | phần tử có `role="alert"` |
| HvConfirmDialog | click confirm / cancel / Escape | `onConfirm` gọi 1 lần; `onOpenChange(false)` |
| HvConfirmDialog | pending | nút confirm disabled |
| HvStateBlock | 3 state | role đúng; action render khi error |

## Success Criteria

- [x] `hv-modal.test.tsx` cũ vẫn xanh không sửa assertion.
- [x] 5 primitive mới có test và export từ `components/hv/index.ts`.
- [x] Không file nào ngoài `components/hv` bị sửa ở phase này (trừ docs).
- [x] `components/hv/score-input-parse.ts` tồn tại với contract `number | null | "invalid"`; hàm cũ trong `classbook-stats.ts` chưa bị đụng (Phase 3 xoá).

## Risk Assessment

- **Radix RadioGroup trong jsdom**: đã dùng ở `dropdown-menu`, test setup có `ResizeObserver` shim; rủi ro thấp.
- **`parseScoreInput` đổi contract** (`null` → `"invalid"` khi không phải số): caller cũ (grid, ô điểm chung trong panel) xử lý `null` là "xoá ô"; Phase 2 (panel) và Phase 3 (grid → hook) phải xử lý nhánh `"invalid"` (giữ ô, báo lỗi, không đưa vào payload) ngay khi đổi import, nếu không "abc" sẽ bị hiểu là xoá điểm.
- **xl modal trên màn thấp (< 700px cao)**: `90dvh` vẫn hợp lệ; footer cố định nên nút Lưu luôn thấy.
