---
phase: 1
title: "Modal Sửa lớp học (ClassDialog edit mode + hook lưu)"
status: done
effort: "0.75d"
dependsOn: []
---

# Phase 1 — Modal Sửa lớp học

## Bối cảnh

- `components/class-dialog.tsx` đang chỉ tạo lớp. Docblock ghi rõ việc sửa do `ClassSettingsPage` đảm nhận.
- Logic lưu của màn settings (`pages/class-settings-page.tsx:130-170`) là quy trình fan-out đã test kỹ: `PUT` tên/đơn
  giá, sau đó `diffSchedules` → add → close → delete, kèm cờ `applied` để phân biệt lỗi một phần với lỗi validate.
- Tiền lệ dialog hai chế độ: `features/courses/components/course-dialog.tsx` (props discriminated union, tiêu đề
  "Sửa khóa học" / "Tạo khóa học").

Tất cả đường dẫn dưới đây tương đối với `apps/web/src/features/roster/`.

## Bước 0 — Cổng đối chiếu thiết kế (bắt buộc, ≤15 phút)

1. Thử lấy phần đuôi `So Lop - Prototype v5.dc.html` (sau byte 262144): người dùng dán markup `modalClass` ở chế độ
   sửa, hoặc export project ra local.
2. Nếu có: đối chiếu tiêu đề, mô tả, danh sách trường, nhãn nút (VD "Lưu thay đổi"), cảnh báo đổi đơn giá. Ghi chênh
   lệch vào mục "Ghi chú đối chiếu" cuối file này, rồi chỉnh các bước dưới cho khớp.
3. Nếu không có: làm theo D4 trong `plan.md` và ghi rõ là "chưa đối chiếu".

## Việc cần làm

### 1. Hook `hooks/use-save-class-settings.ts` (mới)

Chuyển nguyên logic lưu từ `ClassSettingsPage` sang hook, để dialog không phải ôm bốn mutation:

```ts
export type SaveClassSettingsResult =
  | { ok: true }
  | { ok: false; partial: true }          // đã ghi được ít nhất một bước
  | { ok: false; partial: false; error: unknown };

export function useSaveClassSettings(klass: Class | undefined) {
  const id = klass?.id ?? "";
  const update = useUpdateClass(id);
  const add = useAddSchedule(id);
  const close = useUpdateSchedule(id);
  const remove = useDeleteSchedule(id);
  const isPending = update.isPending || add.isPending || close.isPending || remove.isPending;

  async function save(values: ClassSettingsInput, applyFrom: string): Promise<SaveClassSettingsResult> {
    // giữ nguyên thứ tự và cờ `applied` của class-settings-page.tsx:130-170
  }
  return { save, isPending };
}
```

- Giữ nguyên comment về bất biến "adds trước closes/deletes".
- Hook không gọi toast hay navigate. Việc hiển thị do caller quyết định, nhờ vậy test được độc lập.

### 2. `components/class-dialog.tsx` — thêm chế độ edit

```ts
export type ClassDialogProps =
  | { mode?: "create"; open: boolean; onOpenChange: (open: boolean) => void }
  | { mode: "edit"; classId: string; open: boolean; onOpenChange: (open: boolean) => void };
```

- `mode` mặc định là `"create"`, nên call site hiện tại (`class-list-page.tsx`, `classes-tab.tsx`) không cần sửa.
- Tách thân hiện tại thành `CreateClassForm`. Thêm `EditClassForm`:
  - `useClass(classId)`: đọc từ cache TanStack nên mở từ danh sách hầu như không phải chờ. Trong lúc pending thì hiện
    `HvStateBlock state="loading"` bên trong modal. Lỗi 404 thì hiện thông báo "Không tìm thấy lớp" cùng nút Đóng.
  - `useForm<ClassSettingsInput>` với `classSettingsInputSchema`. Hàm `toDefaults(klass)` chuyển từ màn settings
    sang (dùng `deriveScheduleSlots` + `emptySlot`).
  - **Reset một lần mỗi lần mở** (ref chứa `${open}:${klass.id}`), không reset theo mỗi lần fetch. Mutation lịch
    invalidate class detail, và nếu reset theo refetch sẽ xoá mất lỗi "lưu một phần" đang hiển thị.
  - Trường: Tên lớp (`id="class-edit-name"`), Lịch học trong tuần (`ScheduleSlotsEditor idPrefix="class-edit"`,
    tóm tắt "· n buổi/tuần", câu hướng dẫn nhiều khung giờ), Đơn giá / buổi (`MoneyInput`, `id="class-edit-unit-price"`).
  - Hiện cảnh báo đổi đơn giá (màu sun) khi `default_unit_price` khác giá trị đã lưu, lấy nguyên câu chữ từ màn
    settings.
  - `canWriteClass(isOwner, klass)` = false: hiện notice "Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được
    cài đặt lớp." và disable nút lưu. Đây là lớp phòng thủ thứ hai, vì call site đã ẩn nút Sửa.
  - Submit: `save(values, today())`. Nếu `ok` thì toast `Đã lưu ${name} — áp dụng từ buổi kế tiếp` rồi
    `onOpenChange(false)`. Nếu `partial` thì `setError("root", …)`. Các lỗi còn lại đi qua `useApiFormErrors`.
- `HvModal`: `title="Sửa lớp học"`, `description="Thay đổi áp dụng từ buổi kế tiếp — các kỳ đã chốt không đổi."`.
  Footer gồm Hủy (ghost) và "Lưu thay đổi" / "Đang lưu…" (`form="class-dialog-edit-form"`).
- Cập nhật docblock: dialog là `modalClass` cho cả tạo lẫn sửa, bỏ câu nhắc `ClassSettingsPage`.

### 3. Export

Nếu `features/roster/index.ts` đang export `ClassDialog` thì giữ nguyên. Không export hook mới ra ngoài feature.

## Files

| File | Thao tác |
|---|---|
| `hooks/use-save-class-settings.ts` | tạo |
| `components/class-dialog.tsx` | sửa (thêm edit mode) |
| `components/schedule-slots-editor.tsx` | chỉ sửa docblock (bỏ nhắc `ClassSettingsPage`) |
| `__tests__/class-dialog.test.tsx` | mở rộng (Phase 3) |

## Kiểm tra

```bash
cd apps/web && npx vitest run src/features/roster/__tests__/class-dialog.test.tsx
cd apps/web && npm run typecheck
```

## Rủi ro và rollback

- Rủi ro chính là thay đổi vô tình hành vi lưu khi chuyển logic sang hook. Giảm thiểu bằng cách port nguyên các test
  lưu của `class-settings-page.test.tsx` (Phase 3) trước khi xoá màn.
- Rollback: revert commit của phase. Màn settings vẫn còn nguyên cho tới Phase 2.

## Ghi chú đối chiếu

Chưa đối chiếu: bản prototype v5 chỉ có nội dung bị cắt ở 256 KB; không tìm thấy bản export/markup tại workspace. Theo quyết định D4 đã duyệt (tên, lịch, đơn giá), không suy đoán thêm trường.
