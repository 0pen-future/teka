---
title: Hoàn thành trang Học sinh hợp nhất và quy tắc SĐT theo quyền
date: 2026-09-29
summary: "4 phase + sửa sau review trên feat/unified-students-page; e2e 96/96, test API 78.6%"
---

# Hoàn thành trang Học sinh hợp nhất và quy tắc SĐT theo quyền

## Chuyện gì đã xảy ra
- Hoàn tất 4 phase của plan `260929-1038-unified-students-page` trên nhánh `feat/unified-students-page` (9e3481b → 9e90113, chưa push).
- SĐT phụ huynh chỉ hiện qua `contacts.view_all`; API công nợ theo người liên hệ theo tháng gate bởi `billing.view_all`; một trang `/students` với các tab Tất cả / Theo lớp / Chưa vào lớp (`students.list`) và Người liên hệ (`contacts.view_all`); sidebar gộp ba mục cũ thành "Học sinh" trong nhóm "Lớp học", nhóm rỗng bị ẩn, `/contacts` chuyển về `/students?tab=contacts`.
- Review phát hiện: phân trang offset không có tiebreaker cho học sinh/liên hệ trùng tên (đã thêm `ORDER BY id` + test tích hợp); `keepPreviousData` làm hiện hàng của tab/tháng trước (thay bằng `keepWhileSearching` chỉ giữ khi đổi từ khoá); docs `api-guidelines` còn nêu chữ ký `TargetContacts` cũ.
- E2E lần đầu fail 5: `getByLabel("Số điện thoại")` trùng aria-label ô tìm kiếm của tab liên hệ (scope vào dialog); bảng lớp không có cột giá; axe `target-size` trên link tel (thêm `min-h-6`); hai route chung tiêu đề "Học sinh" (thêm `tabOf` cho UxRoute).
- `make test-api` song song lại dính "Timeout waiting for systemd to create docker-...scope" — tranh chấp, chạy lại `-p 1` thì 50/50 package đạt, coverage 78.6%.

## Quyết định
- Ghi danh/kết thúc ghi danh ở trang chi tiết học sinh giờ chỉ dành cho chủ trung tâm (theo AC3) — cần người dùng xác nhận.
- Commit tách: phase 4, sửa placeholder web, docs API, tiebreaker API, plan.

## Việc tiếp theo
- Người dùng duyệt push nhánh và mở PR.
- Các điểm thấp còn mở: role có `contacts.view_all` nhưng thiếu `students.list` mất màn liên hệ; `/contacts/:id` không highlight sidebar; chưa có unit test `ContactsRedirect`; giáo viên không có lớp chưa có empty state; `thisMonth()` dùng giờ local.
- Lỗi có sẵn trên HEAD, ngoài phạm vi: 11 test nhập Excel (`_buffer`) và 12 test library/tasks.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
