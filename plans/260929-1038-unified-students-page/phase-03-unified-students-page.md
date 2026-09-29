---
phase: 3
title: "Trang Học sinh hợp nhất"
status: completed
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 3: Trang Học sinh hợp nhất

## Goal

`/students` dùng được cho mọi thành viên có `students.list`, với các tab Tất cả /
Theo lớp / Chưa vào lớp / Người liên hệ; hành động ghi chỉ hiện cho chủ trung tâm;
tab Người liên hệ theo `contacts.view_all`, cột công nợ theo tháng theo `billing.view_all`.

## Context

- `apps/web/src/features/roster/pages/students-page.tsx` (owner guard `:63-69`, tabs `:25-29`, `resolveTab` `:39-53`).
- `apps/web/src/features/roster/pages/contacts-page.tsx` (danh sách + "Tự động ghép Zalo" + "Thêm người liên hệ").
- `apps/web/src/features/roster/components/roster-table.tsx` (link `/contacts/:id`, `tel:`, nút Ghi danh/Sửa/Xoá).
- `apps/web/src/features/roster/pages/student-detail-page.tsx` (Sửa/Xoá gate `isOwner`; "Ghi danh vào lớp" hiện cho mọi người `:78-80`; link người liên hệ).
- `apps/web/src/features/roster/pages/contact-detail-page.tsx` (owner gate `:121`).
- `apps/web/src/features/teaching/hooks/use-center-context.ts:55` — `has(key) = isOwner || permissions.includes(key)`.
- API danh sách học sinh: `query`, `class_id`, `unenrolled`, trả `total`. Contacts list: `student_count`, `phone`.

## Tab model

| id | Nhãn | Hiện khi | Query |
|---|---|---|---|
| `all` | Tất cả ({total}) | `students.list` | `GET /students?query` |
| `by-class` | Theo lớp | `students.list` | chọn lớp (pill), `class_id` |
| `unenrolled` | Chưa vào lớp ({total}) | `students.list` | `unenrolled=true` |
| `contacts` | Người liên hệ | `has("contacts.view_all")` | `GET /contacts?query` |

Map link cũ trong `resolveTab`:
- `tab=classes` → `by-class`; `tab=students` → `by-class`; `tab=unenrolled` → `unenrolled`.
- Không `tab`: `class_id=none` → `unenrolled`; `class_id=<id>` → `by-class`; không có gì → `all`.
- `tab=contacts` khi không có `contacts.view_all` → `all`.

Số đếm trên nhãn: lấy `total` của chính truy vấn tab đó khi đang mở; tab không mở
hiển thị không số (tránh bắn thêm request chỉ để đếm).

Phân trang (quyết định 2026-09-29): nút **"Xem thêm"** cho tab Tất cả / Chưa vào lớp /
Người liên hệ — `useInfiniteQuery` với `page`/`per_page=50` của API (theo mẫu
`features/audit/hooks/use-audit-logs.ts`); `getNextPageParam` dừng khi số hàng đã
tải ≥ `total`. Nhãn dùng `total` của API nên luôn khớp số thật dù mới tải 50 hàng.

## Implementation Steps

1. **Guard**: `StudentsPage` thay `isOwner` bằng `has("students.list")`; giữ cấu trúc shell để query không mount khi không có quyền.
2. **Quyền trên trang** (một chỗ, đầu `StudentsPageContent`):
   ```ts
   // Contacts are owner-managed (contacts/service.go rejects non-owner writes) and a
   // student must attach to a contact, so only the owner can add, edit or enroll.
   const canManageStudents = isOwner;
   const canSeeContacts = has("contacts.view_all");
   const canSeeDebt = has("billing.view_all");
   const canImport = has("imports.run");
   ```
3. **Tabs**: thay `PAGE_TABS` theo bảng trên; lọc tab `contacts` theo `canSeeContacts`. Xoá nhánh `ClassesTab`, `ClassDialog` (quản lý lớp ở `/classes`), xoá file `components/classes-tab.tsx` nếu không còn import nào. Tiêu đề trang "Học sinh"; giữ phụ đề tối thiểu dữ liệu.
4. **Header hành động** (chỉ một primary action):
   - `canManageStudents`: "+ Thêm học sinh" (primary); "+ Ghi danh học sinh" (secondary, tab by-class, `canWriteClass`).
   - `canImport`: nút secondary "Nhập từ Excel" → `/students/import`.
   - Không `canManageStudents`: `HvNotice tone="info"` "Liên hệ chủ trung tâm để thêm hoặc sửa học sinh."
5. **RosterTable**: thêm prop `canManage: boolean` và `canOpenContact: boolean`. `canManage=false` → ẩn Ghi danh/Sửa/Xoá. `canOpenContact=false` → tên người liên hệ là text, không `Link`. `tel:` đã tự ẩn khi `contact_phone` null (backend phase 1). Cập nhật variant "all" nếu cần cột lớp — dùng cột hiện có của variant `students` khi có dữ liệu enrollment, còn tab `all` dùng variant `unenrolled`-style (không có cột buổi/ngày) để tránh query per-class.
6. **Tab Người liên hệ**: tách phần danh sách của `contacts-page.tsx` thành `components/contacts-tab.tsx` (tìm kiếm, danh sách: tên, SĐT, số con, "Xem thêm"). "Thêm người liên hệ" chỉ owner. "Tự động ghép Zalo" chỉ khi `has("reports.send")` (khớp gate ghi mapping ở phase 1 — hiện nút không có gate). Thêm:
   - `canSeeDebt`: bộ chọn tháng (`<input type="month">`, mặc định tháng hiện tại, lưu `?month=YYYY-MM` trên URL) và cột "Còn nợ" lấy từ hook mới `useContactMonthlyBalances(year, month)` (`features/collections/api` + `hooks`, export qua `features/collections/index.ts`, schema zod). Gia đình không có trong kết quả → "0 ₫"/"—". Dùng `formatMoney`.
   - Route `/contacts` (`features/roster/routes.tsx`) → redirect giữ `?q=`: `<Navigate to={"/students?tab=contacts" + (q ? "&q=" + encodeURIComponent(q) : "")} replace />`, đặt trong `components/contacts-redirect.tsx` (cùng kiểu `class-settings-redirect.tsx`). Sửa link nội bộ `/contacts` ở `features/collections/pages/notifications-page.tsx:502`.
   - Xoá `pages/contacts-page.tsx`; chuyển test sang `contacts-tab.test.tsx`.
7. **Student detail**: "Ghi danh vào lớp" và thao tác kết thúc ghi danh chỉ `isOwner`; link người liên hệ chỉ khi `has("contacts.view_all")`.
8. **Contact detail**: `ContactDetailPage` redirect về `/students` khi `!has("contacts.view_all")` (shell guard, cùng kiểu students-page). Liên kết/bỏ liên kết Zalo: gate `has("reports.send")` (khớp `scopedMappingWrite` = owner + `reports.send`). Sửa/Xoá người liên hệ giữ `isOwner`.
9. **Contact picker**: xác nhận nút inline "Tạo người liên hệ" (`contact-picker.tsx:20,38`) chỉ render trong wizard mà giáo viên không mở được; không cần sửa nếu đúng.
10. **Tests** (`features/roster/__tests__/`):
    - `students-page.test.tsx`: thay nhóm "classes tab" bằng: bare URL → Tất cả; legacy map (`tab=classes|students|unenrolled`, `class_id=<id>|none`, `q`); owner thấy Thêm/Sửa/Xoá/Ghi danh; teacher (`students.list` only) thấy danh sách + HvNotice, không có nút ghi, không có tab Người liên hệ, tên người liên hệ không phải link; member không có `students.list` bị redirect không bắn request; `imports.run` → nút Nhập từ Excel.
    - `contacts-tab.test.tsx`: `contacts.view_all` không `billing.view_all` → không có cột nợ và không gọi endpoint nợ; có `billing.view_all` → cột nợ theo tháng, đổi tháng gọi lại với `year/month` mới; owner thấy "Thêm người liên hệ", member view_all không thấy; "Tự động ghép Zalo" chỉ với `reports.send`.
    - "Xem thêm": `total=120` → tải trang 2 khi bấm, ẩn nút khi đủ.
    - Student detail: teacher không thấy "Ghi danh vào lớp".
    - MSW handlers: thêm `GET /collections/contact-balances` vào `roster-handlers.ts` (hoặc handler collections hiện có).
11. **Docs**: `docs/frontend-guidelines.md` — thêm một đoạn ngắn về trang Học sinh và gate quyền nếu mục sidebar đã được mô tả ở đó (phase 4 sửa tên nhóm).

## Todo

- [x] Guard + quyền trên trang
- [x] Tabs + map link cũ, xoá classes tab khỏi trang
- [x] Header hành động + HvNotice + Nhập từ Excel
- [x] RosterTable `canManage` / `canOpenContact`
- [x] "Xem thêm" (useInfiniteQuery)
- [x] Contacts tab + tháng + công nợ
- [x] Student detail / contact detail gating
- [x] Tests owner/teacher/view_all fixtures

## Verification

- `cd apps/web && npx vitest run src/features/roster`
- `make test-web && make lint-web && npm --prefix apps/web run build`
- Chạy thử (skill `run`): owner, giáo viên mặc định, vai trò có `contacts.view_all` + `billing.view_all`; mở `/contacts`, `/students?class_id=none`, `/students?tab=classes`.

## Risk Assessment

- **Mất lối vào tạo/sửa lớp từ trang này**: `/classes` (Danh mục lớp) đã có tạo/sửa (`class-list-page.tsx:170-182`). Link dashboard `students?class_id=<id>` vẫn mở tab Theo lớp.
- **Trung tâm lớn**: "Xem thêm" tải 50 hàng/lần; tìm kiếm server-side qua `query` nên không cần tải hết.
- **Implied keys**: đã xác minh — `EffectiveKeys()` gồm implied keys và `/centers/me` trả chúng, nên `has("contacts.view_all")` đúng với người giữ `reports.send`.
- **Merge**: phase 4 (sidebar) phụ thuộc phase này; không merge sidebar trước.

## Security Considerations

UI chỉ là lớp hiển thị; mọi gate thật nằm ở API (phase 1-2). Không hiện hành động
mà API sẽ từ chối.
