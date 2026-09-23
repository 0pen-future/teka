---
title: "Phase 3 Giảng dạy: Kho học liệu chương trình mẫu"
date: 2026-09-23
summary: "Hoàn tất API + web kho học liệu (chương trình mẫu, phiên bản, buổi học mẫu); review chỉ ra 2 race, sửa bằng khoá hàng phiên bản trong transaction."
---

# Phase 3 Giảng dạy: Kho học liệu chương trình mẫu

## What happened

- Thêm feature package `library` (Go) với migration 000027: `program_templates`, `program_template_versions`, `program_template_lessons`; quyền `library.read/edit/publish` gắn vào catalog và routespec.
- Thêm trang web `/library` (danh sách chương trình mẫu, chi tiết chương trình với phiên bản và buổi học, trang soạn buổi học) gate theo quyền qua `useCenterContext`.
- Tách helper escape `ILIKE` thành package dùng chung `internal/shared/likeq`; `classes` và `audit` dùng lại.
- Review (code-reviewer) trả SHIP WITH FIXES: 1 High (race giữa ghi buổi học và Publish), 1 Medium (race vị trí buổi → 500), 7 Low, 7 Nit. Đã sửa tất cả trừ L5 (phát hành bản nháp 0 buổi) và L7 (trần 100 dòng).
- Playwright `e2e/library.spec.ts` chạy trên stack cô lập `teka-e2e`, pass.

## Lessons / evidence

- Kiểm tra trạng thái ngoài transaction là lỗ hổng race kinh điển: `requireDraft` đọc rồi mới `WithinTx` ghi, Publish chen giữa là bản published bị sửa. Sửa gốc: khoá hàng phiên bản (`FOR UPDATE OF program_template_versions`) *bên trong* transaction cho mọi ghi và mọi chuyển trạng thái, rồi mới kiểm tra trạng thái. Hai integration test `TestLessonWritesSerialiseWithPublish` và `TestConcurrentLessonWritesKeepPositionsContiguous` là bằng chứng.
- `position = len+1` đọc ngoài khoá tạo trùng/hổng; `COALESCE(max(position),0)+1` trong cùng transaction đã khoá là đủ, không cần sequence.
- Fake `TxManager` trong unit test phải mô phỏng join transaction lồng nhau (ctx key) giống manager thật, nếu không đếm transaction sẽ sai khi service lồng `WithinTx`.
- Mã chương trình theo regex `^[A-Z0-9-]{2,20}$` nên test escape `_` của ILIKE phải đặt dấu gạch dưới vào *tên* chứ không phải mã.
- Web: invalidate query trong `onSettled` thay vì `onSuccess` để 409 `VERSION_LOCKED` kéo lại danh sách phiên bản và trang tự chuyển sang chỉ đọc.

## Decision

- Bản nháp mới sao chép từ phiên bản released (không phải nháp) có `version_no` cao nhất, kể cả archived khi không còn published. Khác câu chữ spec ("published mới nhất") nhưng an toàn vì mọi phiên bản released đều bất biến; ghi rõ phương án thay thế trong Disposition của report review.
- Ghi lên phiên bản của template đã xoá mềm trả 404; đọc vẫn cho phép để lớp đã dùng xem được.
- Không chặn phát hành bản nháp 0 buổi ở Phase 3; để Phase 7 quyết định tại thời điểm áp vào lớp.

## Next steps

- Phase 4 theo plan `plans/260923-0715-giang-day-menu/plan.md`, re-scout trước khi làm.
- Phase 7: quyết định chặn áp chương trình rỗng vào lớp.
- Chạy full API integration suite tuần tự với coverage sau Phase 9.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
