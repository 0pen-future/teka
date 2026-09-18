# Báo cáo Phase 3 — Modal A: form gọn, nhánh D12, xoá/khôi phục

## Trạng thái các gate

| Gate | Kết quả | Tóm tắt |
|---|---|---|
| `npm run lint` (apps/web) | PASS | `0 problems` liên quan tới code do phase này sửa; còn 6 warning `react-hooks/incompatible-library` trên `form.watch()` — cùng mẫu đã tồn tại từ trước ở 4 file ngoài sở hữu (`score-set-editor-modal.tsx`, `profile-page.tsx`, `class-dialog.tsx`, `student-dialog.tsx`, `class-settings-page.tsx`), `lint` script không có `--max-warnings 0` nên không fail gate. |
| `npm run typecheck` (apps/web) | PASS | `tsc -b --noEmit` sạch, không lỗi. |
| `npm run test -- task-form-modal due-state task-description-editor` | PASS | 3 test file, 36/36 test xanh. |
| `make lint-web test-web` (repo root) | PASS | `eslint` 0 error/6 warning (như trên); `prettier --check` sạch (đã chạy `--write` một lần trên 2 file bị lệch format); `tsc -b --noEmit` sạch; `vitest run` toàn bộ web: **101 test file, 836 test pass, 3 skip** (skip có sẵn từ trước, không liên quan phase này). |

## Tóm tắt triển khai

Đã hiện thực đầy đủ phạm vi của `phase-03-modal-a-form-gon.md`:

- Dòng ngữ cảnh dưới tiêu đề (chế độ sửa): `{Cột hiện tại} · {Người tạo} · Tạo {dd/MM}`, dùng prop `description` của `HvModal`.
- Trường **Cột** sửa được ở cả tạo mới và sửa; ở chế độ sửa chỉ bật khi `!readOnly || canEdit` (`canChangeColumn`).
- Trình soạn thảo mô tả: viền 1px, nút toolbar 40px (`size-10`), bộ đếm ký tự chỉ hiện khi đang focus hoặc đã ≥ 1800 ký tự (`focusWithin` state qua `onFocus`/`onBlur` bubble — thay cho CSS `:focus-within` vì jsdom không load stylesheet đã biên dịch để test được).
- **Độ ưu tiên**: 4 `HvChip role="radio"` trong `role="radiogroup"`, chấm màu theo `PRIORITY_VARIANTS`, `grid grid-cols-4 gap-2 max-[440px]:grid-cols-2`.
- **Hạn**: 4 chip nhanh (Hôm nay · Ngày mai · Thứ 2 tới · Bỏ hạn) tính từ ngày lịch **local** (`quickDueOptions(new Date())`), mỗi lần bấm gọi `form.setValue("due_on", value, { shouldDirty: true, shouldValidate: true })`.
- Footer dính (`HvModal stickyFooter`): Xoá (ghost, chữ coral, `mr-auto`, chỉ hiện khi `canDeleteThisTask`) · badge "Chưa lưu" (`HvBadge variant="warning" size="sm"`, khi `isDirty`) · Huỷ (ghost) · Lưu (primary).
- Guard đóng khi dirty: `requestClose()` — nếu `isDirty` thì mở `HvConfirmDialog` "Bỏ thay đổi?" (nút xác nhận "Bỏ thay đổi", nút huỷ "Tiếp tục sửa", tone danger); nếu không thì đóng ngay. Nối vào `onOpenChange` của `HvModal` (nên tự động phủ cả Escape và click ra ngoài) và nút Huỷ.
- Luồng xoá: confirm dialog (mô tả "Có thể hoàn tác trong vài giây sau khi xoá.") → xoá → đóng modal → `hvToast("Đã xoá công việc", { variant: "success", duration: 6000, action: { label: "Hoàn tác", onClick: ... } })` → announce live-region → bấm Hoàn tác gọi `restoreTaskMutation`, announce khi thành công, toast danger khi lỗi.
- **Nhánh D12** (read-only + `canEdit=true`): chỉ Cột bật; footer chỉ còn Huỷ + Lưu (không có Xoá); `Lưu disabled={!dirtyFields.column_id || isSaving}`; submit chỉ gọi `onMoveTask`, **không bao giờ** gọi `updateTaskMutation` (server sẽ 403 qua `CanWriteTask`). Đoạn info: "Bạn chỉ có thể đổi cột của việc này." (so với "Bạn chỉ có thể xem việc này." khi `!canEdit`).
- Đổi cột khi lưu (nhánh chủ/người tạo): sau khi `updateTaskMutation` thành công, nếu `column_id` đổi thì gọi thêm `onMoveTask`; nếu move lỗi thì **không đóng modal** — `form.setError("column_id", ...)` để giữ nội dung đã lưu hiển thị (đúng như Risk Assessment của plan).

## Chữ ký public mới

- `restoreTask(taskId: string): Promise<Task>` — `apps/web/src/features/tasks/api/tasks-api.ts`.
- `restoreTaskMutation: UseMutationResult<AppTask, KanbanError, TaskId>` — thêm vào kết quả trả về của `useTasksDataSource`. Không có `onMutate` optimistic — có comment giải thích: client không còn giữ dòng đã xoá để build lại, nên task khôi phục chỉ vào board sau khi server xác nhận.
- `TaskFormModalProps` thêm: `canEdit: boolean` (từ quyền `tasks.edit`, trang truyền vào — không suy ra từ `task`), `onMoveTask: (taskId: TaskId, columnId: ColumnId) => Promise<unknown>`, `restoreTaskMutation: UseMutationResult<AppTask, KanbanError, TaskId>`.

## Sai lệch so với văn bản phase — vì sao

1. **Dòng "Tạo {dd/MM}" dùng `formatDateTime(iso).slice(0, 5)` thay vì `formatDayMonth(createdAt)`** như chữ trong plan. `createdAt` là instant RFC3339 (`2026-09-10T08:00:00Z`), còn `formatDayMonth` chỉ nhận `YYYY-MM-DD` trần — gọi thẳng sẽ ra kết quả sai lặng lẽ. `formatDateTime(...).slice(0,5)` đúng và đã có tiền lệ (dùng y hệt cho `completedAt` ở `task-card.tsx`).
2. **Bộ đếm ký tự dùng state JS (`focusWithin`) + render có điều kiện, không phải CSS `hidden group-focus-within:block`** như bước 5 mô tả nghĩa đen. jsdom không nạp stylesheet Tailwind đã biên dịch nên không thể test `:focus-within` qua `toHaveClass`; cách JS state tương đương về hành vi và test được.
3. **Sửa 1 test ngoài danh sách sở hữu gốc**: `apps/web/src/features/tasks/__tests__/task-board-page.test.tsx`, test `"shows a read-only form with no delete button for a task the caller did not create"` (dòng ~309). Mock mặc định `/centers/me` luôn cấp `ALL_PERMISSION_KEYS` cho bất kỳ giáo viên đăng nhập nào, nên test này (mở `foreignTaskId` bằng `testPrimaryTeacher`) giờ rơi đúng vào nhánh D12 (`readOnly && canEdit`) chứ không còn là read-only chặn hoàn toàn như hành vi cũ. Đây là hệ quả cơ học tất yếu của thay đổi hành vi mà chính phase này yêu cầu (D12), không phải mở rộng phạm vi tuỳ ý. Đã thay 2 assertion cũ (`Lưu` vắng mặt, có nút `Đóng`) bằng assertion khớp nhánh D12 mới (Cột bật, `Lưu` có mặt nhưng disabled, có nút `Huỷ`); các assertion còn lại của test giữ nguyên.
4. **Sửa lỗi type-strict phát sinh khi thêm state `deletedTasks`** trong `tasks-handlers.ts` (không có trong bước implementation của plan nhưng cần để `restore` hoạt động đúng kiểu dưới `noUncheckedIndexedAccess`): destructuring kết quả `splice()` cho kiểu `T | undefined`; đã sửa bằng cách lấy phần tử qua index trước, guard `if (!x) return 404`, rồi mới `splice`.
5. **Xoá một type assertion thừa** (`event.relatedTarget as Node | null` → `event.relatedTarget`) trong `task-description-editor.tsx` — ESLint báo `@typescript-eslint/no-unnecessary-type-assertion`; type của `relatedTarget` đã tương thích với `Node | null` nên cast là dư, không đổi hành vi.

## Test mới

Viết 12 test mới trong `task-form-modal.test.tsx` (tổng file giờ 18 test, tất cả xanh), theo đúng cấu trúc mount `<TaskBoardPage />` có sẵn của file (không mount `TaskFormModal` cô lập):

- Dòng ngữ cảnh dưới tiêu đề đúng định dạng.
- Thứ tự nút footer Xoá, Huỷ, Lưu.
- Ẩn Xoá khi thiếu `tasks.delete` (dùng `memberCenterMe` cục bộ, nhân bản từ helper cùng tên trong `task-board-page.test.tsx` vì không export).
- Badge "Chưa lưu" xuất hiện khi dirty; Huỷ khi dirty mở confirm "Bỏ thay đổi?"; "Tiếp tục sửa" giữ modal mở và giữ giá trị đã gõ.
- Escape khi dirty mở confirm, xác nhận "Bỏ thay đổi" mới đóng; Escape khi không dirty đóng ngay không hỏi.
- Đổi Cột rồi Lưu → task chuyển đúng cột trên board (qua `onMoveTask` → `POST /tasks/:id/move`).
- Xoá → toast "Đã xoá công việc" có nút "Hoàn tác" → bấm Hoàn tác → task xuất hiện lại trên board (qua handler MSW `POST /tasks/:id/restore` mới).
- 2 test chip Hạn nhanh dùng `vi.useFakeTimers({ toFake: ["Date"] })` + `vi.setSystemTime` (chỉ đóng băng `Date`, giữ `setTimeout` thật cho MSW/userEvent — theo đúng mẫu đã có ở `students-page.test.tsx`/`score-table-modal.test.tsx`): "Hôm nay" ra đúng ngày local, "Thứ 2 tới" ra đúng thứ Hai kế tiếp.
- Nhánh D12 (`foreignTaskId`, đăng nhập `testPrimaryTeacher` — vốn đã có `tasks.edit` theo mock owner mặc định): Tiêu đề disabled, Cột bật, Lưu disabled tới khi đổi cột; sau khi đổi cột và Lưu, `taskWriteRequests` (bắt cả `POST /tasks` lẫn `PATCH /tasks/:id`) vẫn rỗng — chứng minh không gọi update — và task xuất hiện đúng cột mới.
- Nhánh read-only + `!canEdit` (override quyền còn `["tasks.list"]`): Cột disabled, chỉ có nút Đóng, không có Lưu/Huỷ/Xoá.

## Files thay đổi/tạo mới

**Sở hữu, sửa**: `apps/web/src/features/tasks/components/task-form-modal.tsx` (viết lại toàn bộ), `apps/web/src/features/tasks/components/task-description-editor.tsx` (fix cast thừa), `apps/web/src/features/tasks/api/tasks-api.ts` (thêm `restoreTask`), `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts` (thêm `restoreTaskMutation`), `apps/web/src/features/tasks/pages/task-board-page.tsx` (chỉ truyền prop mới vào `<TaskFormModal>`), `apps/web/src/features/tasks/__tests__/tasks-handlers.ts` (giữ task đã xoá + endpoint restore), `apps/web/src/features/tasks/__tests__/task-form-modal.test.tsx` (thêm 12 test), `apps/web/src/features/tasks/__tests__/task-description-editor.test.tsx` (sửa số ký tự sai ở phiên trước).

**Sở hữu, mới**: `apps/web/src/features/tasks/lib/due-state.ts`, `apps/web/src/features/tasks/__tests__/due-state.test.ts`.

**Ngoài sở hữu, sửa vì hệ quả cơ học (đã giải trình ở mục 3 trên)**: `apps/web/src/features/tasks/__tests__/task-board-page.test.tsx` (2 assertion trong 1 test).

Không commit — controller session sẽ commit theo phase.

---

Status: DONE
Summary: Cả 4 gate (lint, typecheck, test phạm vi hẹp, và `make lint-web test-web` toàn bộ web) đều PASS; Modal A đầy đủ context row, Cột sửa được ở D12, chip ưu tiên/hạn, footer dính với guard đóng dirty, và luồng xoá/khôi phục đúng đặc tả; 12 test mới + 1 test hiện có được sửa hợp lý bao phủ toàn bộ nhánh D12.
Concerns/Blockers: Không có blocker. Một test ngoài danh sách sở hữu gốc (`task-board-page.test.tsx`) đã sửa 2 assertion do hệ quả cơ học của hành vi D12 — đã giải trình chi tiết ở trên, cần controller xác nhận không xung đột với agent khác trước khi commit.
