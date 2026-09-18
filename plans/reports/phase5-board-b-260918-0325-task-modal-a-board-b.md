# Phase 5 — Bảng · Phương án B: lọc nhanh, Xong một cú nhấp, cột tô màu/thu gọn, URL state

Plan: `plans/260917-2142-task-modal-a-board-b/phase-05-board-b-dieu-phoi-filters-quick-done.md`

## Tóm tắt

Đã triển khai đầy đủ 13 bước của phase: lib kanban generic theo cột (D7,
tương thích ngược, test lib không đổi), `AppColumn.color` xuyên suốt tầng UI,
tint + chấm màu cột + `ColumnColorPicker` trong Cấu hình cột, `useBoardUrlState`
(`filter`/`assignee`/`collapsed`, giá trị mặc định bị lược khỏi URL),
`lib/board-filters.ts` (`assigneeChips`, `isFiltering`), `BoardFilterBar` (chip
trạng thái + chip giáo viên có số + overflow `HvSelect` + "Xoá lọc"), prop
`filtering` trên `TaskColumn` cho thông báo "Không có việc khớp lọc",
`QuickDoneCheckbox` + toast Hoàn tác 6 giây, cột thu gọn
(`CollapsedColumnRail`, chỉ desktop, lưu vào URL), và cập nhật MSW
(`tasks-handlers.ts`) để mock đúng ngữ nghĩa lọc phía server (đối chiếu trực
tiếp với `boardFilterFrom` trong `apps/api/internal/features/tasks/service.go`,
chỉ đọc, không sửa).

## File đã tạo

- `apps/web/src/features/tasks/lib/column-colors.ts`
- `apps/web/src/features/tasks/lib/board-filters.ts`
- `apps/web/src/features/tasks/hooks/use-board-url-state.ts`
- `apps/web/src/features/tasks/components/column-color-picker.tsx`
- `apps/web/src/features/tasks/components/board-filter-bar.tsx`
- `apps/web/src/features/tasks/components/quick-done-checkbox.tsx`
- `apps/web/src/features/tasks/components/collapsed-column-rail.tsx`
- `apps/web/src/features/tasks/__tests__/board-filters.test.ts` (6 test)
- `apps/web/src/features/tasks/__tests__/use-board-url-state.test.tsx` (6 test)

## File đã sửa

- `apps/web/src/lib/kanban/{types.ts,state.ts,selectors.ts,use-kanban.ts,README.md}`
  — generic `TColumn extends KanbanColumn = KanbanColumn`, mặc định giữ tương
  thích; `npx vitest run src/lib/kanban` vẫn 57/57, không đổi test lib.
- `apps/web/src/features/tasks/schemas/task-schemas.ts` — `ColumnColor`,
  `taskColumnSchema.color`, `BOARD_FILTERS`, `BoardCounts`.
- `apps/web/src/features/tasks/api/tasks-api.ts` — `GetBoardParams.filter/assignee`,
  `getBoard` chỉ gắn key có giá trị (đã sửa `assignee: params.assignee === "" ? undefined : params.assignee`
  để qua lint `prefer-nullish-coalescing` mà vẫn giữ đúng ngữ nghĩa: `??` không
  loại được chuỗi rỗng vì `""` không phải giá trị nullish).
- `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts` — `AppColumn.color`,
  `toAppColumn`, mutation `color`.
- `apps/web/src/features/tasks/components/{task-column,task-card,board-desktop,board-mobile,board-settings-modal}.tsx`
  — tint/chấm màu, prop `filtering`, `onQuickDone`, nút thu gọn (desktop-only),
  rail thay cột khi `collapsed.has(id)`, picker màu trong `ColumnRow` + form
  thêm cột.
- `apps/web/src/features/tasks/pages/task-board-page.tsx` — nối `useBoardUrlState`
  vào `boardParams`, render `BoardFilterBar` khi `canViewAll`, `handleQuickDone`
  (di chuyển sang cột done đầu theo `order`, toast Hoàn tác 6s trả về cột cũ),
  `handleToggleCollapse` (ghi vào URL).
- `apps/web/src/features/tasks/__tests__/tasks-handlers.ts` — cột mock có
  `color`; hàm `matchesBoardFilter` mô phỏng đúng `boardFilterFrom` (đặc biệt:
  `mine` không giới hạn `open-only`; `assignee` tường minh ghi đè, không AND,
  kể cả xoá `unassigned`); `boardColumnsPayload` áp lọc, `boardCounts` độc lập
  với lọc đang bật; thêm mảng bắt request `boardFilterRequests`,
  `boardAssigneeRequests` cho test.
- `apps/web/src/features/tasks/__tests__/task-board-page.test.tsx` — thêm 12
  test mới trong 3 `describe` block ("board filter bar", "quick done",
  "column collapse"), không đụng vào `describe("board summary")` sẵn có.
- `apps/web/src/features/tasks/__tests__/board-mobile.test.tsx` — cập nhật
  fixture cột (`color: "none"`) và truyền `filtering`/`onQuickDone` cho khớp
  props mới của `BoardMobile` (lỗi `tsc -b` phát hiện, không nằm trong danh
  sách sửa ban đầu của phase nhưng là hệ quả trực tiếp của việc mở rộng
  `AppColumn`/`BoardMobileProps`).
- `plans/260917-2142-task-modal-a-board-b/phase-05-board-b-dieu-phoi-filters-quick-done.md`
  — thêm mục "Validation Log" (bước 12) với bảng tỷ lệ tương phản WCAG.

## Quyết định đáng chú ý

- **Ngữ nghĩa lọc server-side** lấy trực tiếp từ `boardFilterFrom` trong
  `apps/api/internal/features/tasks/service.go` (chỉ đọc) thay vì suy diễn lại
  từ ghi chú — phát hiện `filter=mine` không kèm `open-only`, và `assignee`
  tường minh ghi đè (không AND) predicate mà `filter` ngụ ý.
- **`COLUMN_DOT.none`** đổi từ dự tính ban đầu `ink-300` sang `ink-500`: kiểm
  tra tỷ lệ tương phản trắng-trên-nền thực tế (`ink-300` < 3:1, không đạt
  ngưỡng phi văn bản WCAG 1.4.11) trước khi đưa vào `column-colors.ts`.
- **`board-filter-bar.tsx`** dùng hai `role="radiogroup"` lồng trong một
  `role="group"` ngoài cùng (không phải một `radiogroup` chung), khớp đúng hợp
  đồng của `HvChip`: chip `role="radio"` cần tổ tiên `role="radiogroup"`, và
  trạng thái với giáo viên là hai nhóm loại trừ lẫn nhau độc lập.
- **Test filter/assignee/quick-done/collapse** được viết thêm trực tiếp vào
  `task-board-page.test.tsx` (không tạo file test riêng) vì nó đã có sẵn hạ
  tầng render toàn bảng (`renderBoardPage`, MSW, `signInAs`) mà mọi hành vi
  mới cần tương tác qua — tránh nhân đôi hạ tầng test.

## Kết quả kiểm thử

- `npx vitest run src/features/tasks src/lib/kanban` → **191/191 pass** (17
  test file), gồm 12 test mới trong `task-board-page.test.tsx` và không có
  test lib kanban nào bị sửa.
- `make test-web` (toàn bộ `apps/web`) → **864 pass, 3 skip** (103 file), 3
  skip có từ trước, không liên quan phase này.
- `make lint-web` (eslint + prettier + `tsc -b --noEmit`) → **sạch** (0 lỗi;
  6 warning `react-hooks/incompatible-library` có sẵn từ trước ở các file
  ngoài phạm vi phase, không phải do thay đổi lần này).
- `npx tsc -b --noEmit` (toàn repo, phạm vi rộng hơn `tsconfig.json` đơn lẻ đã
  chạy nhiều lần trước đó) → phát hiện và đã sửa lỗi type ở
  `board-mobile.test.tsx` (props mới bắt buộc `filtering`/`onQuickDone`,
  `AppColumn.color` bắt buộc) — **sạch** sau khi sửa.
- Tương phản WCAG: `ink-500` trên cả 4 tint cột đều ≥ 4.51:1 (đạt AA văn bản
  thường ≥4.5:1); `ink-700`/`ink-900` đạt dư nhiều; chấm màu (kể cả `sun-600`
  thấp nhất ở 3.52:1) đạt ngưỡng phi văn bản ≥3:1. Chi tiết trong Validation
  Log của phase file.

## Vấn đề gặp phải và cách xử lý

- 2 test `use-board-url-state.test.tsx` thất bại do `MemoryRouter` không đồng
  bộ vào `window.location` — sửa bằng cách đọc lại `URLSearchParams` qua một
  `useSearchParams()` thứ hai trong cùng router thay vì `window.location.search`.
- 47 test thất bại (ZodError `color`) sau khi mở rộng `AppColumn`/schema — do
  fixture MSW thiếu trường `color` bắt buộc; đã thêm `color` vào seed data và
  2 handler cột.
- 2 lỗi eslint `prefer-nullish-coalescing` (`tasks-api.ts`, `quick-done-checkbox.tsx`)
  — cả hai đều là trường hợp `??` đổi ngữ nghĩa (một bên xử lý chuỗi rỗng, một
  bên là OR boolean thật sự), nên sửa bằng so sánh tường minh (`=== ""`) và
  `Boolean(disabled) || checked` thay vì đổi máy móc sang `??`.
- 7 file lệch định dạng prettier (2 file mới của phase này + 4 file lib kanban
  từ bước 1 + `board-filter-bar.tsx`) — chạy `prettier --write` đúng 7 file đó.
- 2 lỗi `tsc -b` ở `board-mobile.test.tsx` (không nằm trong lần chạy
  `tsc --noEmit -p tsconfig.json` hẹp hơn trước đó) — bổ sung `color` vào
  fixture cột và `filtering`/`onQuickDone` vào hai lần dựng `<BoardMobile>`.

## Việc chưa làm / ngoài phạm vi

Không có việc nào trong 13 bước của phase còn dang dở. e2e cho lọc/quick-done/
collapse, a11y check toàn diện, và docs thuộc Phase 6 theo đúng "Next Steps"
của phase file — không thuộc phạm vi phase này.

Status: DONE
Summary: Đã hoàn tất toàn bộ 13 bước implementation của Phase 5 (lib kanban generic, màu cột, URL state, dải lọc server-side, quick-done+undo, thu gọn cột) cùng test mới và WCAG validation log; `make lint-web` và `make test-web` đều sạch.
Concerns/Blockers: Không có. `board-mobile.test.tsx` được sửa ngoài danh sách file ban đầu của phase vì `tsc -b --noEmit` (phạm vi rộng hơn) phát hiện lỗi type là hệ quả trực tiếp của việc mở rộng `AppColumn`/`BoardMobileProps` trong đúng phạm vi phase — không phải thay đổi hành vi ngoài ý.
