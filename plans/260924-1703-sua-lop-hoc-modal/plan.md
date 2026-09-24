---
title: "Modal Sửa lớp học — thay màn Cài đặt lớp"
description: "Nút Sửa ở Danh sách lớp học (và các lối vào khác) mở modal Sửa lớp học theo prototype v5; xoá màn /classes/:id/settings trùng lặp, chuyển Nhân sự lớp + Bàn giao giáo viên sang tab Thông tin của chi tiết lớp."
status: completed
priority: P2
effort: "2d"
issue:
branch: feat/giang-day-menu
tags: [feature, frontend, refactor, roster, classes]
blockedBy: []
blocks: []
created: 2026-09-24
---

# Modal Sửa lớp học

## Mục tiêu

Prototype **So Lop v5** (`claude.ai/design` project `4a7e6c77…`, file `So Lop - Prototype v5.dc.html`, màn
"Danh sách lớp học", nút `r.onEdit`) sửa lớp **tại chỗ bằng modal**, không chuyển sang màn riêng. Hiện tại web có
hai đường sửa lớp song song:

| Đường sửa hiện có | Vị trí | Nội dung |
|---|---|---|
| Màn "Cài đặt lớp" `/classes/:id/settings` | `apps/web/src/features/roster/pages/class-settings-page.tsx` | Tên, khung giờ, đơn giá + 3 thẻ số liệu + `ClassStaffSection` + `TeacherHandoffCard` |
| `ClassOpsCard` trên chi tiết lớp | `components/class-ops-card.tsx` | Tuyển sinh, thẻ, ghi chú (giữ nguyên) |

Plan này thay màn `settings` bằng modal **"Sửa lớp học"**, dùng lại khung modal `modalClass` (đang là "Tạo lớp mới").
Mọi lối vào sẽ mở modal này, còn màn settings bị xoá, URL cũ được chuyển hướng.

## Quyết định đã chốt (với người dùng, 2026-09-24)

- **D1 — Thiết kế:** file v5 trả qua MCP bị cắt ở 256 KB (`truncated: true`, tải lại hai lần đều giống nhau), nên phần
  markup modal Sửa và phần logic nằm sau byte 262144 **chưa đọc được**. Plan dựng modal Sửa trên khung `modalClass`
  đọc được (tiêu đề, mô tả, tên lớp, lịch trong tuần với "KHUNG GIỜ n" + tóm tắt buổi/tuần, `popIn`, nút Hủy + primary).
  Phase 1 có **cổng đối chiếu thiết kế**: nếu lấy được phần đuôi file (người dùng dán markup hoặc export project), đối
  chiếu trường/nhãn trước khi code, còn không thì theo D4.
- **D2 — Khối chỉ chủ trung tâm:** `ClassStaffSection` ("Nhân sự lớp") và `TeacherHandoffCard` (`#teacher-handoff`)
  chuyển vào **tab Thông tin** của `/classes/:id`, đặt ngay dưới "Đội ngũ giảng dạy".
- **D3 — Phạm vi:** tất cả lối vào (nút Sửa ở danh sách, nút "Sửa lớp" ở header chi tiết, lối tắt "Thiết lập lớp",
  "⚙ Cài đặt" ở tab Lớp của "Lớp & học sinh") đều mở modal. `/classes/:id/settings` chuyển hướng
  `replace` sang `/classes/:id?edit=1`.
- **D4 — Trường của modal Sửa (mặc định khi chưa có markup):** Tên lớp · Lịch học trong tuần · Đơn giá / buổi. Đây
  đúng là hợp đồng mà màn settings đang có và đã được test. Khóa học, ngày khai giảng/kết thúc và thời lượng
  **không** đưa vào (xem Câu hỏi mở).
- **D5 — Kiến trúc:** `ClassDialog` nhận props dạng discriminated union `mode: "create" | "edit"`, theo tiền lệ
  `features/courses/components/course-dialog.tsx`. Hai form con (create và edit) tách riêng vì schema và đường lưu khác
  nhau: create gọi một `POST`, edit gồm `PUT` và một chuỗi lệnh diff lịch. Logic lưu của edit chuyển thành hook
  `useSaveClassSettings`.

## Tiêu chí nghiệm thu

1. Bấm **Sửa** ở `/classes` mở dialog "Sửa lớp học", điền sẵn dữ liệu lớp. Click vào hàng không bị kích hoạt, URL
   danh sách giữ nguyên (bộ lọc còn nguyên).
2. Lưu hợp lệ: gọi `PUT /classes/:id` (chỉ khi tên/đơn giá đổi), rồi thêm lịch mới → đóng lịch cũ → xoá lịch.
   Thứ tự này giữ bất biến "không bao giờ để lớp không có thời khoá biểu". Sau đó toast "Đã lưu … — áp dụng từ buổi kế
   tiếp", đóng dialog, danh sách tự cập nhật (invalidate có sẵn).
3. Lưu thất bại giữa chừng: hiện lỗi root "Chỉ lưu được một phần…", dialog **không** đóng. Lỗi trước khi ghi gì thì
   map vào field qua `useApiFormErrors`.
4. Người không có quyền ghi (`canWriteClass` = false) không thấy nút Sửa, `?edit=1` bị bỏ qua.
5. Header chi tiết, lối tắt "Thiết lập lớp", "⚙ Cài đặt" ở tab Lớp mở cùng dialog, không còn `Link` nào trỏ
   `/settings`.
6. `/classes/:id/settings` → `/classes/:id?edit=1` (replace), dialog tự mở. Đóng dialog thì xoá `edit` khỏi URL.
7. Chủ trung tâm thấy "Nhân sự lớp" và "Giáo viên phụ trách" (`#teacher-handoff`) trong tab Thông tin, thành viên thì
   không. Link `#teacher-handoff` trong `ClassStaffSection` vẫn cuộn đúng.
8. `class-settings-page.tsx`, hai file test của nó và route `classes/:id/settings` (dạng lazy page) bị xoá. Không còn
   tham chiếu `ClassSettingsPage`.
9. `npm run lint`, `npm run typecheck`, vitest roster, e2e `class-list`, `class-staff-write`, `class-invitations`
   đều pass.

## Non-goals

- Không đổi API, schema DB hay permission key. `PUT /classes/:id` và các endpoint lịch giữ nguyên.
- Không gộp `ClassOpsCard` (tuyển sinh / thẻ / ghi chú) vào modal.
- Không làm lại 3 thẻ số liệu (HỌC SINH / BUỔI KỲ / ĐƠN GIÁ) của màn settings. Tab Thông tin đã có thông tin
  tương đương, nên chúng bị bỏ cùng màn.
- Không thêm nút "Nhân bản lớp" hay tính năng mới nào ngoài modal.

## Phases

| # | Phase | Trạng thái | Phụ thuộc |
|---|---|---|---|
| 1 | [Modal Sửa lớp học (ClassDialog edit mode + hook lưu)](phase-01-class-edit-dialog.md) | pending | — |
| 2 | [Nối mọi lối vào, dời khối owner, xoá màn settings](phase-02-rewire-entry-points-remove-settings.md) | pending | 1 |
| 3 | [Test, e2e, docs](phase-03-tests-e2e-docs.md) | pending | 2 |

## Kiến trúc sau thay đổi

```text
/classes (ClassListPage) ─ Sửa ─────────┐
/classes/:id header "Sửa lớp" ──────────┤
ClassInfoTab lối tắt "Thiết lập lớp" ───┼──► ClassDialog mode="edit" classId
/students tab Lớp "⚙ Cài đặt" ──────────┤        └─ EditClassForm ─► useSaveClassSettings(klass)
/classes/:id/settings ─redirect─► /classes/:id?edit=1 ┘                PUT → add → close → delete

ClassInfoTab (owner): … ClassTeamSection → ClassStaffSection → TeacherHandoffCard(#teacher-handoff)
```

## Rủi ro

| Rủi ro | Mức | Giảm thiểu |
|---|---|---|
| Thiết kế thật của modal Sửa khác D4 (thêm khóa học/ngày) | Trung bình | Cổng đối chiếu ở Phase 1. Các trường thêm đều có trong `classUpdateInputSchema`, nên thêm vào không phải đổi API |
| Refetch sau mutation lịch reset form đang sửa | Trung bình | Giữ cơ chế "reset một lần mỗi lần mở / mỗi class id" của màn settings (ref guard) |
| Bookmark / e2e cũ trỏ `/settings` | Thấp | Route redirect, cập nhật 3 e2e spec |
| Dialog cao hơn viewport khi có nhiều khung giờ | Thấp | `HvModal` có vùng body cuộn; giới hạn chiều cao danh sách khung giờ như prototype (`max-height:280px`) |

## Câu hỏi mở

1. Modal Sửa trong v5 có cho sửa **Khóa học**, **Khai giảng/Kết thúc** hay không? Chưa xác nhận được vì file bị cắt
   (D1). Mặc định là không (D4).
