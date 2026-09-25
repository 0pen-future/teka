---
title: "Phase 1 Menu Giảng dạy: nav, danh sách và chi tiết lớp"
date: 2026-09-23
summary: "Hoàn tất Phase 1: nav Giảng dạy, danh sách lớp có mã/filter/stats, chi tiết lớp 3 tab; review đảo D7 sang readonly sessions."
---

# Phase 1 Menu Giảng dạy: nav, danh sách và chi tiết lớp

## What happened

- Triển khai Phase 1 của plan `260923-0715-giang-day-menu` theo TDD trên nhánh `feat/giang-day-menu`:
  migration 000025 thêm `code`/`end_date`/`level`/`room` cho lớp, helper `shared/classcode`,
  `GET /classes` có filter + `meta.total`, `GET /classes/stats`, 409 `CLASS_CODE_TAKEN`;
  web: nav "Giảng dạy", trang danh sách lớp (filter bar, chips, stats), chi tiết lớp với tab
  Thông tin / Học viên / Buổi học.
- Reviewer (`reports/review-phase-01-260923.md`) chấm DONE_WITH_CONCERNS: H1/H2 tab Buổi học
  materialise buổi `planned` khi chỉ duyệt và N+1 đếm roster; M1 unique index → 500; M3 nút tạo
  không gate quyền; M4 ô tìm kiếm không theo URL khi back; H3/M5 danh sách bị cắt trang im lặng.
- Sửa H1, H2, M1, M3, M4 và thêm thông báo cắt trang cho H3/M5; M2 (PUT full-replace) giữ nguyên
  theo R2/R9. Gate: lint, typecheck, test-api-unit, scopelint, vitest (908 pass), integration
  `-p 1` cho enrollments/sessions/classes đều xanh.

## Lessons / evidence

- Endpoint "đọc theo khoảng" có side effect ghi (`ListRange` sinh buổi) là bẫy cho mọi màn hình
  chỉ duyệt: cần đường read-only tường minh (`readonly=true` → `ListRangeExisting`) thay vì chia
  cửa sổ 400 ngày. Bằng chứng: `sessions/service_test.go`
  `TestListRangeExistingNeverGeneratesAndCountsRosterPerDate`.
- `TranslateError: true` của GORM biến unique violation thành `gorm.ErrDuplicatedKey`; service phải
  map sang lỗi domain (409) ở cả create trong tx lẫn update, không chỉ pre-check.
- Đếm roster theo ngày cho N buổi: một truy vấn `ActiveInRange` rồi đếm trong bộ nhớ thay vì
  N lần `ActiveOn`.
- Test gate quyền trên web: chờ `queryClient.getQueryState(centerKeys.me)?.status === "success"`
  thay vì tìm text trang không render.

## Decision

- Đảo cơ chế D7 dưới bằng chứng mới: tab Buổi học gọi một request
  `?from&to&readonly=true`, không materialise, không cap 400 ngày. Scope người dùng không đổi.
  Đường mặc định vẫn materialise cho classbook/lịch.
- M2 chấp nhận giới hạn PUT full-replace; filter `recruiting` phía API, phân trang danh sách/roster
  và L1–L8 là follow-up ngoài phase.

## Next steps

- Phase 2: tab Đội ngũ giảng dạy (mời/gỡ giáo viên) theo `phase-02-*.md`, re-scout trước khi làm.
- Chạy `make test-api` serial toàn bộ sau Phase 9.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
