---
title: "Hồ sơ học sinh: dropdown lớp + tìm học sinh (cook --auto)"
date: 2026-09-07
summary: "8 commit trên feat/records-class-dropdown-student-search: dropdown lớp có sĩ số, tìm học sinh theo URL, 4 finding review đã sửa"
---

# Hồ sơ học sinh: dropdown lớp + tìm học sinh (cook --auto)

## What happened

Chạy `/ak:cook plans/260907-0434-records-class-dropdown-student-search/plan.md --auto` trọn 6 phase, mỗi phase một commit (`2e39587` → `6776e0a`): helper tìm kiếm gập dấu + URL state `?q=`, `student_count` trong `ClassResponse` (một query grouped, không N+1), `RecordsClassSelect` (popover ≥ sm, bottom sheet < sm, lọc khi >5 lớp), toolbar + wiring trang, bảng `<mark>` + trạng thái rỗng/skeleton + layout compact, và e2e/QA thị giác/docs.

Code review (`plans/reports/code-review-260907-records-class-dropdown.md`) trả DONE_WITH_CONCERNS với 8 finding. Sửa 4 finding đầu trong 2 commit:

- `a13f07d` — F1: `CountActiveEnrollmentsByClass` đếm center-wide nên member có `classes.view_all` nhưng thiếu `enrollments.view_all` vẫn thấy sĩ số của roster họ không được xem. Thêm `readScopedEnrollments` trong repo classes mô phỏng đúng read port của enrollments (own + stint `class_staff` + `enrollments.view_all`). Lần đầu đặt `CenterWideFor` thẳng trong hàm `Count…` thì scopelint chặn ("may only widen a read-named function"), nên phải tách helper mang tên `read…` đúng convention. Test tích hợp mới `TestStudentCountsFollowEnrollmentReadScope` (member có stint chỉ đếm lớp đó; grant `enrollments.view_all` mở rộng bằng owner).
- `f1d3515` — F2: `isPrintableKey` coi Space là ký tự in được nên khi >5 lớp, Space bị đẩy vào ô lọc thay vì kích hoạt option; loại `" "`. F3: phím `/` của trang cướp focus khi picker đang mở; `isTypingTarget` bail khi `activeElement` nằm trong `[role=listbox]`/`[role=dialog]`. F4: bộ đếm và skeleton chỉ chờ `sessionsPending`; giờ chờ cả `isPending` của enrollments để không nháy "0 học sinh".

Gate sau sửa: web lint 0 lỗi / typecheck / vitest 84 file 628 pass 3 skip / build OK; api build / test / scopelint OK; 2 test tích hợp `TestStudentCounts*` pass trên Docker. E2E 5/5 đã chạy trước đó trên stack `teka-e2e` cách ly (đã `down -v`).

## Decision

- Sĩ số lớp là dữ liệu trả về client nên phải bám phạm vi đọc roster của người gọi, không được coi là "count không phải row read" như `CountOpenEnrollments` (guard nội bộ cho lệnh xoá).
- Chọn lại đúng lớp đang chọn vẫn ghi `class_id` lên URL (giữ hành vi pill cũ), chỉ xoá `q` khi lớp thực sự đổi.
- F5–F8 (wording khi chưa có lớp, `mockViewport` không reset, hai `role=status` lúc tải, IME telex Android) để đợt sau, ghi trong plan.md.

## Next steps

- Push nhánh `feat/records-class-dropdown-student-search` và mở PR lên `master` (cần người dùng duyệt; gh chỉ pull, push qua SSH).
- Thử telex/VNI trên Android thật trước khi ship vì ô tìm là controlled input đồng bộ URL mỗi keystroke.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
