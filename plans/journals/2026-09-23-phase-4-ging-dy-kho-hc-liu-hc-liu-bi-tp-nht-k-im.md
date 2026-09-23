---
title: "Phase 4 Giảng dạy: Kho học liệu — học liệu, bài tập, nhật ký & điểm"
date: 2026-09-23
summary: "Hoàn thành phase 4: danh mục học liệu/bài tập, gắn vào buổi mẫu, trường nhật ký và bộ điểm theo phiên bản; review SHIP WITH FIXES đã sửa xong"
---

# Phase 4 Giảng dạy: Kho học liệu — học liệu, bài tập, nhật ký & điểm

## What happened
- Migration 000028: `library_materials`, `library_exercises` (xoá mềm, scope trung tâm), bảng nối `template_lesson_materials`/`template_lesson_exercises` (FK composite kèm `center_id`, cascade), `template_log_fields` (unique `(version_id, position)` DEFERRABLE) và cột `score_set` JSONB trên phiên bản.
- API: 15 route mới trong routespec (CRUD danh mục, `PUT lessons/:lid/materials|exercises`, `GET versions/:vid`, `PUT versions/:vid/log-fields|score-set`); mọi ghi đi qua `lessonWrite` nên chịu chung khoá publish của phase 3. Helper dùng chung `validation.Elements[T]` trả 422 theo key `"<i>.<field>"`.
- Web: tab Học liệu / Bài tập, khối chọn học liệu/bài tập trên trang buổi, tab "Nhật ký & Điểm" trên chi tiết chương trình mẫu; e2e `library.spec.ts`.
- Review (code-reviewer) kết luận SHIP WITH FIXES: 3 Medium, 7 Low, 6 nit. Đã sửa M1–M3, L1–L4, L6, L7 và 2 nit; L5 giữ tham chiếu.

## Lessons / evidence
- Xoá mềm mục danh mục và gắn mục vào buổi là hai đường ghi độc lập; nếu không khoá cùng một dòng thì mục đã xoá vẫn có thể được gắn và khối chọn kẹt 422 mãi. Giải bằng `FOR UPDATE` khi xoá, `FOR SHARE` khi gắn (`TestDeleteAndAttachSerialiseOnTheItemRow`).
- "Đang dùng" phải là thứ người dùng gỡ được. Link trong template đã xoá mềm hoặc trong phiên bản đã phát hành không gỡ được, nên đếm usage chỉ tính template sống và tách thông báo nháp / đã phát hành.
- `CreateVersion` phải sao chép cả gắn học liệu, trường nhật ký, bộ điểm chứ không chỉ buổi; ghi gom một lần thay vì N+1 trong lúc giữ khoá.
- Helper test trả về giá trị làm errcheck của golangci-lint bắt mọi lời gọi bỏ kết quả; tách `requireAppError` (void) và `appErrorOf` (trả về) sạch hơn là rải `_ =`.
- Override MSW cho danh sách phải dùng `listMeta(...)` đủ `total_pages`, nếu không zod parse lỗi và test thấy trạng thái lỗi thay vì dữ liệu.
- `make e2e-isolated` có thể fail "address already in use" trên 58080 trong lúc stack cũ chưa gỡ xong; kiểm tra `ss -ltn`/`docker ps` rồi chạy lại là đủ, không đổi cổng.
- Gate: unit + integration library (45.6s, `-p 1`) + migrations xanh; `make test-api-unit`, `scopelint`, `lint`, `test-web` (975 passed / 3 skipped) xanh; e2e 2/2 trên stack cô lập.

## Decision
- M2 theo phương án (a): link của template đã xoá mềm không chặn xoá học liệu/bài tập; link trong phiên bản đã phát hành/lưu trữ chặn với thông báo riêng. Phương án (b) "giữ chặn và chỉ ra chỗ đang dùng" bị bỏ vì người dùng không mở được template đã xoá để gỡ.
- L5: học liệu/bài tập là danh mục dùng chung, sửa nội dung áp dụng cho mọi phiên bản tham chiếu; không snapshot khi publish trong phase này.
- `url` học liệu chỉ nhận `http(s)://` ở cả API lẫn zod; trần 100 mục cho một lần gắn.

## Next steps
- Phase 5: khoá học (`courses`) — migration 000029, feature `internal/features/courses/`, `course_id` trên lớp, quyền `courses.read`/`courses.edit`, trang `/courses`, chọn khoá học ở roster.
- Phase 7 tham chiếu trường nhật ký theo `(version_id, id)` của bản published, không theo id của bản nháp (id đổi mỗi lần lưu nháp).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
