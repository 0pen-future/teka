---
title: Modal sửa lớp học thay màn settings
date: 2026-09-24
summary: "Port logic lưu lịch vào dialog, chuyển nhân sự lớp sang tab Thông tin, giữ redirect và kiểm chứng Vitest/E2E cô lập."
---

# Modal sửa lớp học thay màn settings

## What happened
- Chuyển form sửa lớp từ màn `/classes/:id/settings` sang `ClassDialog` chế độ edit, giữ thứ tự lưu PUT → add → close → delete và lỗi lưu một phần.
- Chuyển khối Nhân sự lớp và Bàn giao giáo viên vào tab Thông tin, giữ anchor `#teacher-handoff`; redirect bookmark cũ tới `?edit=1`.
- Port test cũ trước khi xoá trang; viết test đỏ cho edit mode, lối vào modal và lỗi 404 rồi xác nhận xanh.

## Verification
- Web: typecheck, lint, Prettier, 1.155 test pass (3 skip), production build thành công. Test bổ sung mobile đạt 1/1.
- E2E trên compose cô lập: 4/4 test trong ba spec của plan và 2/2 test `class-staff-read`, stack được dọn sau chạy.

## Decision and follow-up
- Không có markup đuôi prototype v5 tại workspace, áp dụng hợp đồng D4 đã duyệt: tên, lịch, đơn giá. Nếu nhận được bản đầy đủ, đối chiếu UI sau.
- Không động vào diff `docs/architecture.md` đã tồn tại trước phiên.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
