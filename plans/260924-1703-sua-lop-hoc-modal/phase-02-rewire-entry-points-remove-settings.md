---
phase: 2
title: "Nối mọi lối vào, dời khối owner, xoá màn settings"
status: done
effort: "0.75d"
dependsOn: [1]
---

# Phase 2 — Nối lối vào và xoá màn trùng lặp

Tất cả đường dẫn dưới đây tương đối với `apps/web/src/features/roster/`.

## Việc cần làm

### 1. Danh sách lớp — `components/class-table.tsx`, `pages/class-list-page.tsx`

- `ClassTable` nhận thêm `onEdit?: (klass: Class) => void`. Ô thao tác đổi `Link` → `<button type="button">` với
  nhãn "Sửa", `aria-label={\`Sửa lớp ${klass.name}\`}`, và `event.stopPropagation()` để không kích hoạt `onOpen`
  của hàng. Chỉ render khi có `onEdit`.
- `ClassListPage`: thêm state `editingId: string | null`. Truyền `onEdit` khi
  `canWriteClass(isOwner, klass)`. Hàm này kiểm tra theo từng lớp, vì `Class` có `my_staff_roles`, nên hàng của thành
  viên không có quyền sẽ không có nút. Render `<ClassDialog mode="edit" classId={editingId} open={editingId !== null} …/>`.
- Cập nhật docblock `ClassTable` (bỏ "goes straight to the settings screen").

### 2. Chi tiết lớp — `pages/class-detail-page.tsx`, `components/class-detail-header.tsx`, `components/class-info-tab.tsx`

- Trạng thái mở do URL quyết định: `edit=1` trong `searchParams`. Đây là đích của redirect và cũng cho phép chia sẻ
  link.
  - `openEdit()` gọi `params.set("edit","1")`, `closeEdit()` gọi `params.delete("edit")`, cả hai dùng
    `setSearchParams(…, { replace: true })`. Làm theo cách `selectTab` đang làm để không đụng `tab`.
  - `ClassDialog mode="edit"` render khi `canWrite`, với `open = canWrite && searchParams.get("edit") === "1"`.
- `ClassDetailHeader`: prop mới `onEdit`. `Link` "Sửa lớp" đổi thành `HvButton variant="secondary"` (giữ nhãn và
  kiểu dáng). Sửa docblock.
- `ClassInfoTab`: prop mới `onEdit`. Lối tắt "Thiết lập lớp" đổi thành button cùng style `ShortcutLink`, gọi
  `onEdit`.
- `ClassInfoTab` (D2): sau `<ClassTeamSection …/>` render `<ClassStaffSection classId={klass.id} />` (component tự
  ẩn khi không phải owner) và `TeacherHandoffCard` với gate `center && "members" in center` như màn settings đang làm.

### 3. `TeacherHandoffCard` → `components/teacher-handoff-card.tsx` (mới)

- Chuyển nguyên hàm và docblock từ `class-settings-page.tsx:288-406`, rồi export. Giữ `id="teacher-handoff"`, vì
  e2e và anchor trong `ClassStaffSection` phụ thuộc vào nó.
- Bỏ `max-w-[640px]` để khớp lưới tab Thông tin (dùng cùng layout thẻ với `SectionCard`, nếu cần thì bọc lại).
- Sửa docblock `ClassStaffSection` ("further down the settings page" → "below it on the class detail's info tab").

### 4. Tab Lớp của "Lớp & học sinh" — `components/classes-tab.tsx`

- Cả hai `Link` "⚙ Cài đặt" (card mobile và bảng desktop) đổi thành button gọi `onEditClass(cls.id)`. Giữ nguyên nhãn
  và class CSS.
- Component cha (`pages/students-page.tsx`) giữ state `editingClassId` và render `ClassDialog mode="edit"`. Tab này
  vốn chỉ dành cho owner nên không cần gate thêm.
- Sửa docblock `classes-tab.tsx:24` ("the per-class settings link" → "the per-class edit dialog").

### 5. Xoá màn settings và thêm redirect — `routes.tsx`, `pages/class-settings-page.tsx`

- Xoá `pages/class-settings-page.tsx`.
- Tạo `components/class-settings-redirect.tsx`:

  ```tsx
  /** Old `/classes/:id/settings` bookmarks land on the detail with the edit dialog open. */
  export function ClassSettingsRedirect() {
    const { id } = useParams<{ id: string }>();
    return <Navigate to={`/classes/${id ?? ""}?edit=1`} replace />;
  }
  ```

  Route `classes/:id/settings` giữ lại nhưng trỏ lazy tới component này, theo mẫu `BillingIndexRedirect`.
- `rg -n "classes/\\$\\{[^}]+\\}/settings|ClassSettingsPage|class-settings" apps/web/src` phải chỉ còn route
  redirect và comment của nó.

## Files

| File | Thao tác |
|---|---|
| `components/class-table.tsx` | sửa |
| `pages/class-list-page.tsx` | sửa |
| `pages/class-detail-page.tsx` | sửa |
| `components/class-detail-header.tsx` | sửa |
| `components/class-info-tab.tsx` | sửa |
| `components/teacher-handoff-card.tsx` | tạo (chuyển từ settings page) |
| `components/class-staff-section.tsx` | chỉ sửa docblock |
| `components/classes-tab.tsx` | sửa |
| `pages/students-page.tsx` | sửa (state + dialog) |
| `components/class-settings-redirect.tsx` | tạo |
| `routes.tsx` | sửa |
| `pages/class-settings-page.tsx` | xoá |

## Kiểm tra

```bash
cd apps/web && npm run typecheck && npm run lint
cd apps/web && npx vitest run src/features/roster
rg -n "/settings\`|ClassSettingsPage" apps/web/src   # chỉ còn route redirect
```

## Rủi ro và rollback

- Nhúng `?edit=1` vào URL có thể xung đột với `?tab=`. Cách tránh: luôn copy `searchParams` hiện có, chỉ set hoặc
  delete key `edit`.
- Ai đó mở `?edit=1` khi không có quyền: dialog không render. Có thể xoá param trong lúc render, nhưng không bắt buộc.
- Rollback: revert commit của phase để khôi phục màn settings và các Link cũ.
