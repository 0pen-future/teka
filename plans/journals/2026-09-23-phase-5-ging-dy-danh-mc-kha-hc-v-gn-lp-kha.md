---
title: "Phase 5 Giảng dạy: danh mục khóa học và gắn lớp ↔ khóa"
date: 2026-09-23
summary: "Đóng phase 5: feature courses, migration 000029, gắn lớp vào khóa, review SHIP WITH FIXES đã xử lý"
---

# Phase 5 Giảng dạy: danh mục khóa học và gắn lớp ↔ khóa

## What happened
- Hoàn thành phase 5 của plan `plans/260923-0715-giang-day-menu`: feature `courses` (API + web), migration 000029, cột `classes.course_id`, 7 route mới, quyền `courses.read` backfill / `courses.edit` opt-in.
- Review SHIP WITH FIXES: sửa M1–M3, L1–L3, L5, L6 và 2 nit theo TDD; L4 giữ đếm lớp toàn trung tâm kèm nhãn UI.

## Lessons / evidence
- validator v10 với `binding:"omitempty,uuid"` trên `*string` vẫn chạy `uuid` cho con trỏ tới `""`. Trường có quy tắc patch (`""` = gỡ) không được gắn tag định dạng; service tự parse.
- FK `FOR KEY SHARE` không cản UPDATE xoá mềm, nên race xoá khóa ↔ gắn lớp cần khoá tường minh: xoá `FOR UPDATE`, gắn `FOR SHARE` trong cùng tx ghi. Integration chạy race 8 vòng để chứng minh.
- Unique DEFERRABLE trên `(course_id, position)` khiến hai lần ghi gói học phí đồng thời fail ở COMMIT → khoá dòng cha trước khi thay danh sách.
- PUT với "chọn lại đúng thứ đang lưu" không được coi là lựa chọn mới: phiên bản đã lưu trữ sau khi chọn không được chặn sửa tên khóa.

## Decision
- Bộ đếm "lớp đang học / sắp mở" của khóa đếm toàn trung tâm (sự thật danh mục); tab Lớp học vẫn theo phạm vi đọc. Nhãn UI ghi rõ "toàn trung tâm".
- Template mặc định bị xoá mềm: API giữ id, bỏ embed; UI cảnh báo và gợi ý chọn lại. Không tự ghi đè dữ liệu người dùng.
- Khóa `archived` không nhận lớp mới; lớp đã gắn gửi lại đúng id vẫn lưu được.

## Next steps
- Phase 6: lộ trình học (`phase-06-lo-trinh-hoc.md`), giữ nguyên chuỗi re-scout → TDD → gates → review → journal → commit.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
