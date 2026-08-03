---
title: Plan 02 API Roster Management completed
date: 2026-08-03
summary: "Contacts, students+anonymisation, classes+schedules, enrollments — 4 phases xanh, 2 fix từ code review"
---

# Plan 02 API Roster Management completed

## What happened

Hoàn tất plan 02 (API Roster Management) — 4 feature packages Go dựng trên baseline schema của plan 01:

- **contacts**: CRUD, unique phone/teacher (partial index), xoá bị chặn 409 khi còn students tham chiếu.
- **students**: closed field list `{full_name, contact_id, display_note}` (ghim bằng reflection test); delete = anonymise (`full_name` → "Đã xoá", `display_note` NULL, stamp `anonymized_at` + `deleted_at`) và đóng ghi danh mở trong cùng một transaction; invoice snapshot + attendance sống sót; hard DELETE bị RESTRICT FK chặn.
- **classes** + **class_schedules**: CRUD, archive vs soft-delete, schedule close-and-replace gộp vào UpdateSchedule.
- **enrollments**: `unit_price` copy từ `classes.default_unit_price` tại thời điểm tạo, không có đường ghi từ request DTO; `uq_enrollments_active` → 409 index-driven; end flow (mặc định hôm nay, 409 double-end, 422 ended_on < started_on); `ActiveOn` inclusive hai đầu (contract cho plan 03); `EndOpenEnrollments` là EnrollmentEnder thật của students.

## Review & fixes

code-reviewer: DONE_WITH_CONCERNS — mọi tiêu chí nghiệm thu phase 2 & 4 đạt, không Critical/High, không regression. Sửa ngay 2 Medium:

- End enrollment bind body vô điều kiện (tha thứ `io.EOF`) thay vì gate `ContentLength > 0` — tránh mất body khi chunked encoding revert ngày nghỉ về hôm nay.
- `repository.End` phân biệt 409 (đã đóng) vs 404 (không có hàng) cho double-end song song.

Mỗi fix có test: handler test body không Content-Length, integration test repo path 409/404.

## Decisions (ADR)

- Chuẩn "hôm nay" = UTC midnight ở V1; resolve theo `teachers.timezone` hoãn tới plan 03/04 nơi timezone ảnh hưởng doanh thu.
- Guard attendance khi xoá enrollment hoãn tới plan 03 (attendance chưa tồn tại).
- Nhóm Low (tiebreaker phân trang, escape ILIKE, validate date range classes, ghi danh lớp archived, display_note "" vs NULL, docs guidelines) ghi nhận, dọn sau.

## Verification

`go build`/`vet` sạch, `golangci-lint` 0 issues, unit + integration (testcontainers postgres:16) toàn bộ xanh, `make test-api` coverage 66.9% (floor 60%), swagger regen.

## Next steps

Plan 03 (API Sessions and Attendance) — consume `enrollments.ActiveOn` để sinh attendance_records; thêm guard attendance vào DELETE enrollment; chốt chuẩn timezone.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
