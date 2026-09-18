---
phase: 3
title: "Modal · Phương án A — form gọn, hành động đúng chỗ"
status: pending
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 3: Modal · Phương án A — "Form gọn, hành động đúng chỗ"

## Context Links

- Report mục "Modal · Phương án A" và "Trade-off: mở footer dính cho HvModal md"
- Quyết định D2, D5, D6, D8, D12 trong [plan.md](./plan.md)
- `apps/web/src/features/tasks/components/task-form-modal.tsx` (HvModal md; footer Xoá `variant="danger"` `mr-auto`; Cột chỉ ở create; ưu tiên = HvSegmented; Hạn = `<Input type="date">`)
- `apps/web/src/features/tasks/components/task-description-editor.tsx` (khung `border-2`, toolbar `size-7`, counter luôn hiện `text-ink-400`)
- `apps/web/src/features/tasks/schemas/task-schemas.ts` (`taskFormSchema.column_id` đã có)
- `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts` (9 mutation, `applyOptimistic`, `invalidateBoard`)
- `apps/web/src/features/tasks/api/tasks-api.ts`, `__tests__/tasks-handlers.ts` (MSW), `__tests__/task-form-modal.test.tsx`
- `apps/web/src/components/hv/hv-toast.tsx` (bọc sonner, `ExternalToast` → hỗ trợ `action`)
- Trang: `apps/web/src/features/tasks/pages/task-board-page.tsx:227-244` (props modal; chưa truyền `canEdit`/`onMoveTask`), `:163` (`canMove={canEdit}`)
- Quyền move phía server: `routespec.go:380` (`tasks.edit`), `pkg/kanban/policy.go:47-49` (`CanMoveTask` = owner ∨ creator ∨ assignee)

## Overview

Giữ form + HvModal md, sắp lại để "đọc nhanh, hành động đúng chỗ":

- Hàng ngữ cảnh dưới tiêu đề (edit): `Cột hiện tại · Người tạo · Tạo ngày dd/MM`.
- Trường **Cột** hiện ở cả edit (không read-only): đổi cột + Lưu → update rồi move.
- Editor một viền, nút toolbar 40px, counter chỉ hiện khi focus-within hoặc ≥ 1800.
- **Ưu tiên** = 4 `HvChip` có chấm màu (radiogroup), không bẻ dòng; 2×2 dưới 440px.
- **Hạn**: chip nhanh `Hôm nay · Ngày mai · Thứ 2 tới · Bỏ hạn`.
- Footer dính: `Xoá` (chữ đỏ, trái, chỉ khi `canDeleteThisTask`) · chip "Chưa lưu"
  (khi `isDirty`) · `Huỷ` ghost · `Lưu` primary.
- Đóng khi dirty → xác nhận "Bỏ thay đổi?"; Xoá → xác nhận; sau xoá toast có
  **Hoàn tác** (gọi `POST /tasks/:id/restore`).
- **Read-only (D12)**: người chỉ là assignee thấy mọi trường disabled trừ
  **Cột**; footer `Huỷ` + `Lưu` (Lưu enable khi Cột đổi); Lưu chỉ gọi move.

## Key Insights

- `toFormValues` đã đưa `column_id` vào form ở cả hai mode → chỉ cần bỏ điều kiện
  `mode === "create"` khi render và so `values.column_id !== task.columnId` lúc Lưu.
- Move là mutation riêng (`moveTaskMutation`), không nằm trong `updateTask` → Lưu
  chạy tuần tự: update → move (index 0), thất bại ở move vẫn giữ nội dung đã lưu
  và báo lỗi move.
- Hoàn tác xoá cần `restoreTaskMutation` mới với `applyOptimistic({type:"tasks/upserted"})`
  trên kết quả server (không lạc quan trước khi server trả, vì client không còn
  hàng để dựng lại chính xác).
- Read-only (assignee không phải creator, `task-form-modal.tsx:121-125`): theo
  D12 **cho đổi Cột ngay trong modal**. Điều kiện = `canEdit` (perm `tasks.edit`,
  cùng gate với `canMove` của bảng ở `task-board-page.tsx:163`); server đã cho
  phép (`CanMoveTask` gồm assignee). Nhánh submit read-only **không** gọi
  `updateTaskMutation` (server sẽ 403 vì `CanWriteTask` chỉ creator/owner) mà
  chỉ `onMoveTask`. Modal nhận prop `canEdit: boolean` thay vì tự suy.
- Chip hạn nhanh phải tính theo ngày **địa phương** (D8): `localIsoDate(now)`,
  `Thứ 2 tới` = thứ Hai kế tiếp (nếu hôm nay là thứ Hai → +7).

## Requirements

Functional
- Context row chỉ ở edit; người tạo tra từ `members` (`teacher_id === task.createdBy`), fallback "—".
- Đổi Cột + Lưu → `Đã cập nhật công việc X` và `Đã chuyển "X" sang cột Y` trong live region.
- `isDirty` → chip "Chưa lưu" (HvBadge `warning` sm); `onOpenChange(false)` khi dirty → `HvConfirmDialog` (Tiếp tục sửa / Bỏ thay đổi); không dirty → đóng ngay.
- Xoá → confirm → xoá → đóng modal → `hvToast("Đã xoá công việc", { action: { label: "Hoàn tác", onClick } , duration: 6000 })`; Hoàn tác → restore → upsert → announce `Đã khôi phục công việc X`.
- Ưu tiên chips: label `Không · Thấp · Trung bình · Cao`, chấm theo `PRIORITY_VARIANTS`; `role="radiogroup"` aria-label "Độ ưu tiên".
- Chip hạn nhanh set `due_on` với `shouldDirty: true`; `Bỏ hạn` → `""`.
- Read-only + `canEdit`: chỉ trường Cột enable; nút Xoá ẩn; chip "Chưa lưu" vẫn
  hiện khi Cột đổi; `Lưu` `disabled={!form.formState.dirtyFields.column_id}`;
  submit → `onMoveTask(task.id, values.column_id)` → announce `Đã chuyển "X" sang cột Y` → reset + đóng. Read-only + `!canEdit`: như hiện tại (chỉ Đóng, Cột disabled).

Non-functional
- Modal md giữ; footer dính qua `stickyFooter`; không thêm dependency.
- Test đơn vị bao phủ toàn bộ hành vi mới; MSW có handler restore.

## Architecture

```
TaskFormModal ─ useForm(taskFormSchema) ─┬─ onSubmit (edit, owner) → updateTaskMutation → (column changed) onMoveTask(taskId, columnId)
                                         ├─ onSubmit (edit, readOnly && canEdit) → onMoveTask(taskId, columnId) only
                                         ├─ dirty guard → HvConfirmDialog("Bỏ thay đổi?")
                                         └─ handleDelete → deleteTaskMutation → hvToast(action: Hoàn tác → restoreTaskMutation)
lib/due-state.ts: localIsoDate(now), quickDueOptions(now) → { today, tomorrow, nextMonday }
```

## Related Code Files

Sửa
- `apps/web/src/features/tasks/components/task-form-modal.tsx`
- `apps/web/src/features/tasks/components/task-description-editor.tsx`
- `apps/web/src/features/tasks/api/tasks-api.ts` (`restoreTask`)
- `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts` (`restoreTaskMutation`, export type)
- `apps/web/src/features/tasks/pages/task-board-page.tsx` (truyền `onMoveTask`, `restoreTaskMutation`, `canEdit`, `members` cho context row)
- `apps/web/src/features/tasks/__tests__/tasks-handlers.ts` (nhớ hàng đã xoá; `POST /tasks/:id/restore`)
- `apps/web/src/features/tasks/__tests__/task-form-modal.test.tsx`, `task-description-editor.test.tsx`

Tạo mới
- `apps/web/src/features/tasks/lib/due-state.ts` (phần `localIsoDate`, `quickDueOptions`; Phase 4 bổ sung `dueState`)
- `apps/web/src/features/tasks/__tests__/due-state.test.ts`

## Implementation Steps

1. `lib/due-state.ts`: `localIsoDate(date)` (yyyy-mm-dd theo giờ máy),
   `addDays`, `nextMonday(date)`, `quickDueOptions(now)` trả mảng
   `{ label, value }` cho 3 chip + `Bỏ hạn`. Test với `vi.setSystemTime` các mốc:
   23:30 giờ VN (chứng minh khác UTC), thứ Hai, chủ nhật.
2. `tasks-api.ts`: `restoreTask(taskId): Promise<Task>` → `POST /tasks/${id}/restore`.
3. `use-tasks-data-source.ts`: `restoreTaskMutation = useMutation<AppTask, KanbanError, TaskId>`
   với `scope: KANBAN_MUTATION_SCOPE`, `onSuccess` → `applyOptimistic({type:"tasks/upserted", task})`,
   `onSettled` → `invalidateBoard`. Trả về cùng object mutations.
4. MSW `tasks-handlers.ts`: `DELETE /tasks/:id` chuyển hàng vào `deleted` map thay
   vì xoá hẳn; thêm `POST /tasks/:id/restore` → 200 hàng đó / 404.
5. `task-description-editor.tsx`: khung `border` (1px) `border-line-200` +
   `focus-within:border-mint-400`; `ToolbarButton` `size-10`; bọc root
   `group`; counter `cn("...", !nearLimit && "hidden group-focus-within:block")`
   với `nearLimit = count >= 1800`. Cập nhật test: counter ẩn khi không focus,
   hiện khi focus, luôn hiện khi ≥ 1800.
6. `task-form-modal.tsx`:
   - `HvModal stickyFooter`; props mới `onMoveTask: (taskId: TaskId, columnId: ColumnId) => Promise<unknown>`, `restoreTaskMutation`, `canEdit: boolean`.
   - `const canChangeColumn = mode === "edit" && (!readOnly || canEdit)`.
   - Context row (edit, dưới tiêu đề, `text-[12.5px] text-ink-500`):
     `{currentColumn.name} · {creatorName} · Tạo {formatDayMonth(createdAt)}`.
   - Trường Cột render ở cả hai mode, `disabled={mode === "edit" && !canChangeColumn}` (không còn `disabled={readOnly}`).
   - Ưu tiên: thay `HvSegmented` bằng `div role="radiogroup" className="grid grid-cols-4 gap-2 max-[440px]:grid-cols-2"` của 4 `HvChip role="radio" dot=...`.
   - Hạn: dưới `<Input type="date">` thêm hàng chip `quickDueOptions(new Date())` → `form.setValue("due_on", value, { shouldDirty: true, shouldValidate: true })`.
   - Footer: `Xoá` = `HvButton variant="ghost" size="sm" className="mr-auto text-coral-500 hover:bg-coral-100"`; `isDirty && <HvBadge variant="warning" size="sm">Chưa lưu</HvBadge>`; Huỷ; Lưu.
   - `requestClose()`: `form.formState.isDirty ? setDiscardOpen(true) : onOpenChange(false)`; dùng cho `onOpenChange` của HvModal và nút Huỷ. `HvConfirmDialog` "Bỏ thay đổi?" (confirm "Bỏ thay đổi", tone danger) → reset + đóng.
   - `onSubmit` edit: `await update...`; nếu `values.column_id !== task.columnId` → `await onMoveTask(task.id, values.column_id)`; lỗi move → toast lỗi nhưng vẫn đóng? **Không**: giữ modal mở, `setError("column_id")` từ lỗi.
   - `onSubmit` edit + `readOnly && canEdit` (D12): **bỏ qua** update; nếu cột đổi → `await onMoveTask(...)`, announce, `form.reset(values)`, `onOpenChange(false)`; lỗi → `setError("column_id")`. Footer nhánh này: `Huỷ` (qua `requestClose`) + `Lưu` `disabled={!dirtyFields.column_id || submitting}`; không có Xoá.
   - `handleDelete`: sau thành công `onOpenChange(false)` rồi toast với `action` Hoàn tác (6000ms) → `restoreTaskMutation.mutateAsync(task.id)` → announce; lỗi → toast danger. Đổi mô tả confirm xoá: "Có thể hoàn tác trong vài giây sau khi xoá."
   - Câu read-only: khi `canEdit` → `Bạn chỉ có thể đổi cột của việc này.`; khi `!canEdit` → `Bạn chỉ có thể xem việc này.`
7. `task-board-page.tsx`: truyền `onMoveTask={(taskId, columnId) => kanban.moveTask(taskId, columnId, 0)}`, `restoreTaskMutation` và `canEdit`.
8. Tests `task-form-modal.test.tsx`: (a) dirty + Escape → dialog "Bỏ thay đổi?", chọn Tiếp tục sửa → modal còn; (b) chip "Hôm nay" set ISO địa phương; (c) Xoá không hiện khi `canDelete=false` hoặc read-only; (d) footer thứ tự Xoá…Huỷ, Lưu; (e) đổi Cột + Lưu gọi `onMoveTask`; (f) xoá → toast Hoàn tác → restore → task quay lại board (qua MSW); (g) context row hiện tên cột/người tạo; (h) read-only + `canEdit`: tiêu đề disabled, Cột enabled, Lưu disabled cho tới khi đổi Cột, submit gọi `onMoveTask` và **không** gọi update (spy MSW `PATCH /tasks/:id` = 0); (i) read-only + `!canEdit`: Cột disabled, chỉ có nút Đóng.
9. `make lint-web test-web`.

## Todo

- [ ] `lib/due-state.ts` (localIsoDate, quickDueOptions) + test
- [ ] `restoreTask` api + `restoreTaskMutation` + MSW
- [ ] Editor: viền đơn, toolbar 40px, counter theo focus/ngưỡng + test
- [ ] Modal: context row, Cột ở edit, chips ưu tiên, chip hạn, footer dính, dirty guard, Hoàn tác xoá
- [ ] Modal read-only + `canEdit`: chỉ Cột enable, Huỷ + Lưu, submit chỉ move (D12)
- [ ] Page truyền `onMoveTask`, `restoreTaskMutation`, `canEdit`
- [ ] Tests modal (a–i) xanh; `make lint-web test-web`

## Success Criteria

- Mô tả dài 30 dòng: footer luôn nhìn thấy ở 375px và desktop.
- Không đóng được modal dirty bằng Escape/overlay/Huỷ nếu chưa xác nhận.
- Xoá → Hoàn tác trong 6s → việc hiện lại đúng cột, live region thông báo.
- Chip ưu tiên đủ 4, chấm màu trùng badge trên board; không bẻ dòng ở 440px+.
- Assignee (không phải creator) mở việc: đổi Cột + Lưu → việc sang cột mới, không có request PATCH nội dung.

## Risk Assessment

- **Toast action của sonner** không nhận focus tự động → người dùng bàn phím
  cần Tab tới; bổ sung câu live region "Đã xoá. Nhấn Hoàn tác trong 6 giây."
- **Lưu + move hai request**: nếu move lỗi, form giữ mở với lỗi ở trường Cột;
  nội dung đã lưu — chấp nhận, ghi trong docs.
- `form.reset` sau save để `isDirty` về false trước khi đóng (tránh dialog Bỏ thay đổi nhảy ra).
- **Nhánh read-only gọi nhầm update** → server 403 và form báo lỗi khó hiểu; test (h) khoá bằng spy MSW. `canEdit` phải đến từ page (perm), không suy từ `task`.

## Security Considerations

- Hoàn tác chỉ khả dụng nếu server cho phép (`tasks.delete` + creator/owner) — UI ẩn khi `!canDeleteThisTask` vì Xoá cũng ẩn.
- Đổi Cột ở nhánh read-only đi qua `POST /tasks/:id/move`; server vẫn kiểm `tasks.edit` + `CanMoveTask` — UI chỉ phản chiếu quyền, không mở rộng.

## Next Steps

Phase 4 đổi tên menu "Chuyển cột" → "Thao tác"; câu read-only ở đây không còn nhắc menu.
