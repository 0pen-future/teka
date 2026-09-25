---
title: "Phase 6 Giảng dạy: lộ trình học theo giai đoạn"
date: 2026-09-23
summary: "Lộ trình học với giai đoạn có thứ tự và khóa gợi ý, khoá đồng thời FOR UPDATE/FOR SHARE, guard COURSE_IN_PATH, web timeline và fix cache chéo sau review"
---

# Phase 6 Giảng dạy: lộ trình học theo giai đoạn

## Bối cảnh
Phase 6 của plan `260923-0715-giang-day-menu` thêm lộ trình học: chuỗi giai đoạn có thứ tự, mỗi giai đoạn gắn tối đa 20 khóa học, dùng cho tư vấn tuyển sinh và mục "Lộ trình học" trong chi tiết khóa.

## Đã làm
- API `4ae6166`: migration 000030 (ba bảng composite FK, unique position DEFERRABLE, backfill `paths.read` hai nhánh), feature `paths` với 10 route + `GET /courses/:id/paths`, guard xoá khóa `COURSE_IN_PATH`, `shared/idset` dùng chung với `library`.
- Web `2f6151f`: `/paths` và `/paths/:id` (timeline dọc, dialog lộ trình/giai đoạn, picker khóa `active`), mục lộ trình trong chi tiết khóa.
- Fix sau review `13f92b3`: invalidate cache chéo giữa `pathsKeys` và `coursesKeys`, refetch chi tiết khi mutation giai đoạn lỗi, nhãn a11y theo tên giai đoạn, chip khóa chỉ là link khi có `courses.read`, catalog lỗi báo một dòng.

## Bài học
- Gán khóa ghi đè cả danh sách nên phải nhận cả khóa `archived` đang có, nếu không client không thêm được khóa mới vào giai đoạn đã có khóa lưu trữ.
- Hai bộ query key trong cùng một feature vẫn cần invalidate chéo tường minh; `staleTime` 30s che lỗi này trong test tay.
- Mutation lỗi 422/404 do dữ liệu cũ phải refetch chi tiết, nếu không người dùng kẹt trong vòng lỗi lặp.

## Để lại
Review `review-phase-06-260924.md` để lại L2 (lost update, cần `version`), L4 (khóa `draft` trong lộ trình, quyết định sản phẩm), L6, L8 và các test đồng thời/binding cho Phase 9.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
