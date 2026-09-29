---
title: "Trang Học sinh hợp nhất + quy tắc SĐT chỉ theo quyền được cấp"
description: "Một trang Học sinh cho chủ trung tâm và giáo viên (Tất cả / Theo lớp / Chưa vào lớp / Người liên hệ), SĐT phụ huynh chỉ hiện qua contacts.view_all, công nợ theo tháng qua billing.view_all."
status: completed
priority: P1
effort: 3.5d
branch: feat/unified-students-page
tags: [feature, frontend, backend, api, auth, security]
blockedBy: []
blocks: []
created: 2026-09-29
---

# Trang Học sinh hợp nhất

## Overview

Nguồn: [brainstorm Option A + quyết định người dùng](../reports/brainstorm-260929-1711-unified-students-page-option-a.md).
Thay ba mục sidebar "Quản trị học sinh", "Phụ huynh", "Nhập từ Excel" bằng một mục
"Học sinh" (nhóm "Lớp học", đổi tên từ "Giảng dạy"). Giáo viên chỉ xem học sinh
trong phạm vi API; mọi hành động ghi chỉ hiện khi API chấp nhận. Đồng thời siết
quy tắc SĐT toàn hệ thống: bỏ nhánh ngầm theo phân công `hoc_vu`, chỉ chủ trung
tâm hoặc vai trò có `contacts.view_all` mới thấy/gọi/gửi tới SĐT.

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Quy tắc SĐT chỉ theo quyền (backend)](./phase-01-backend-phone-rule.md) | Completed |
| 2 | [API công nợ theo người liên hệ theo tháng](./phase-02-contact-monthly-balances-api.md) | Completed |
| 3 | [Trang Học sinh hợp nhất](./phase-03-unified-students-page.md) | Completed |
| 4 | [Sidebar và route](./phase-04-sidebar-and-routes.md) | Completed |

Thứ tự: 1 → 2 → 3 → 4. Phase 3 cần endpoint của phase 2 và SĐT đã che của phase 1;
phase 4 chỉ lên sau phase 3 (mục sidebar dẫn giáo viên tới `/students`). Mỗi phase
một commit; cả bốn ra trong một PR.

## Quyết định đã chốt (validate 2026-09-29)

1. Ghi mapping Zalo (liên kết/bỏ liên kết, tự động ghép): chủ trung tâm + `reports.send`; bỏ nhánh học vụ. Ghép bạn Zalo (gửi SĐT ra Zalo) cần `contacts.view_all`.
2. Gửi sao kê `zalo_personal` theo lớp của học vụ: giữ (`ZaloMappingsClass` lọc theo ghi danh, không lộ SĐT); SĐT trong response gửi bị che.
3. Công nợ theo tháng theo `v_contact_balance` như trang Thu tiền (gồm hoá đơn nháp kỳ đang mở, loại hoá đơn huỷ).
4. Danh sách dài: nút "Xem thêm" (`useInfiniteQuery`), nhãn dùng `total` API.
5. Red-team: [reports/red-team-report.md](./reports/red-team-report.md) — đã áp dụng các phát hiện Critical/High (che `BulkSendRow.Phone`, giữ `ZaloMappingsClass`, gate nút ghép Zalo, cập nhật e2e, đổi thứ tự phase).

## Scope challenge

- Có sẵn: danh sách học sinh có `query`/`class_id`/`unenrolled`; danh sách người
  liên hệ có `student_count`; `useCenterContext().has()`; `canWriteClass`;
  view `v_contact_balance` theo kỳ.
- Phạm vi: giữ nguyên toàn bộ yêu cầu (HOLD SCOPE, không có `--yagni`).
- Độ phức tạp: ~25 file backend (chủ yếu test), ~12 file web, 1 endpoint mới.

## Acceptance criteria

1. Thành viên có `students.list` thấy "Học sinh" (nhóm "Lớp học", đứng đầu) và mở được `/students`; không có key thì không thấy mục và bị chuyển về `/`.
2. Giáo viên chỉ thấy học sinh trong phạm vi API; số đếm "Tất cả"/"Chưa vào lớp" bằng `total` của API.
3. "+ Thêm học sinh", Sửa, Xoá, "Ghi danh" chỉ hiện cho chủ trung tâm; người khác thấy HvNotice "Liên hệ chủ trung tâm để thêm hoặc sửa học sinh."
4. "Nhập từ Excel" là nút trên trang, chỉ hiện với `imports.run`; `/students/import` vẫn mở được.
5. Tab "Người liên hệ" chỉ hiện cho `contacts.view_all` (chủ trung tâm ngầm có): tên, SĐT, số con; cột công nợ theo tháng chỉ khi có `billing.view_all`. `/contacts` chuyển tới `/students?tab=contacts`; `/contacts/:id` chỉ cho `contacts.view_all`.
6. Người không có `contacts.view_all` không nhận SĐT phụ huynh ở bất kỳ API nào (học sinh, sao kê, thu tiền, thông báo, response gửi hàng loạt, danh bạ, ghép Zalo); UI không còn link `tel:` cho họ.
7. Sidebar không còn ba mục cũ; nhóm rỗng bị ẩn; `OVERFLOW_LABELS` và test layout cập nhật.
8. Link cũ còn chạy: `?tab=classes|students|unenrolled`, `?class_id=<id>|none`, `?q=`, `/students/:id`, `/contacts`, `/contacts/:id`, `/students/import`.
9. `make test-api` (chạy riêng), `npm run test`, lint, typecheck, build và các e2e spec đã sửa đều xanh.

## Non-goals

Option B/C; thay đổi grant mặc định; thêm quyền mới hoặc migration quyền; giáo
viên tạo học sinh (A2); tách nhóm "Trung tâm" thành "Hệ thống".

## Dependencies

- Không phụ thuộc plan khác. Không push master khi chưa được duyệt; commit không có tham chiếu AI.
