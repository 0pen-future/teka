---
title: Modal Tạo/Sửa khóa học theo prototype v5
date: 2026-09-29
summary: "CourseDialog theo v5: chọn chương trình mẫu + phiên bản ngay trong modal, khoá mã khi sửa, Xoá xếp chồng; vitest courses 69/69, typecheck/lint/prettier sạch"
---

# Modal Tạo/Sửa khóa học theo prototype v5

## Chuyện gì đã xảy ra
- Lấy spec từ `handoff/04-logic-component.js` của project `So Lop - Prototype v5` (HTML chính bị cắt ở 256KB như lần trước).
- `CourseDialog`: tiêu đề/mô tả v5, thứ tự Mã → Tên → Thời lượng buổi → Giá / buổi → Chương trình mẫu → Phiên bản mặc định, rồi các trường thật còn lại (Trạng thái, Tổng số buổi, Môn học, Cấp / trình độ, Mô tả). Nút "Tạo mới"/"Lưu thay đổi"; khi sửa có "Xoá" mở `CourseDeleteConfirm` xếp chồng.
- Mã khóa học chỉ đọc khi sửa; submit gửi lại mã đã lưu.
- Chương trình mẫu + phiên bản được chọn trong modal và lưu cùng POST/PUT (`default_template_version_id`). Chỉ phiên bản đã phát hành được chọn; phiên bản đã lưu vẫn hiện dù đã lưu trữ sau đó. Không có quyền `library.read` thì hiển thị chỉ đọc.
- Xoá từ danh sách (toast, bỏ dòng) và từ trang chi tiết (toast, về `/courses`); 409 COURSE_IN_USE/COURSE_IN_PATH hiện thành toast với thông điệp server.
- Review (code-reviewer) tìm ra 2 lỗi trung bình, đã sửa: không bỏ được liên kết tới chương trình đã bị xoá (thêm lựa chọn "Chương trình đã bị xoá" và cho "— Chưa gắn —" xoá phiên bản mồ côi); id `course-template`/`course-version` trùng với tab Thiết lập phía sau modal (đổi sang `course-dialog-*`).

## Quyết định
- Bỏ ô Chặng của prototype (người dùng chọn): khóa ↔ chặng là nhiều-nhiều, gán ở màn Lộ trình.
- Giữ các trường thật ngoài v5, xếp sau (người dùng chọn).
- Không chặn xoá phía client như prototype; API quyết định và trả lời qua toast.
- Chưa sửa: mã đã lưu vẫn được kiểm tra theo pattern client (chặt hơn server ở dấu gạch đầu) dù ô bị khoá — hành vi này có từ trước khi mã còn sửa được.

## Việc tiếp theo
- Chưa chạy e2e `courses.spec.ts` trên stack `teka-e2e` (đã đổi nhãn "Giá / buổi (đ)", nút "Tạo mới"/"Lưu thay đổi").
- 23 test lỗi có sẵn trên HEAD (roster, library, tasks) vẫn còn; suite web đầy đủ không thêm lỗi mới.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
