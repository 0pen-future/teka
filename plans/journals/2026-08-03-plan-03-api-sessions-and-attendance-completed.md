---
title: Plan 03 API sessions and attendance completed
date: 2026-08-03
summary: "Session generation/lifecycle, one-touch attendance, pending-attendance feed; fixed uncancel/hold state guard"
---

# Plan 03 API sessions and attendance completed

## What happened
Hoàn thành plan 03 (API Sessions and Attendance) qua 3 phase, mỗi phase giao fullstack-developer rồi verify độc lập:

- **Phase 1 — Session generation & lifecycle:** package `sessions` (model/generator thuần/repository/service/handler). Sinh session từ `class_schedules` theo range trong timezone giáo viên (iterate `AddDate`), idempotent qua partial unique `uq_class_sessions_per_day` + `OnConflict{TargetWhere: deleted_at IS NULL}`. Lifecycle cancel/uncancel/hold/delete + ad-hoc, chặn cancel/delete khi đã confirm (409).
- **Phase 2 — One-touch attendance:** package `attendance`. Roster giải theo `enrollments.ActiveOn(session_date)`, present+absent đều `billable=true`, upsert khớp `uq_attendance_records`, chỉ soft-delete học sinh rời roster (không bao giờ absentee). `MarkHeldAndConfirmed` chạy atomic trong cùng `WithinTx`. Absent ngoài roster thì 422; session hủy thì 409. `CountBillableByEnrollment` chừa sẵn cho plan 04.
- **Phase 3 — Pending-attendance feed:** `GET /sessions/pending` (đăng ký trước `/sessions/:id`), predicate `session_date < today_in_teacher_tz AND attendance_confirmed_at IS NULL AND status IN ('held','planned') AND deleted_at IS NULL`, grouped-join đếm sĩ số (không N+1), `total` vs `limit`, `DaysOverdue`. Migration `000003_widen_pending_sessions_index` mở rộng index sang held+planned; baseline `000001` giữ nguyên byte-for-byte (D1), `docs/schema_design.sql` cập nhật cùng lúc.

## Decision
Code review phát hiện Medium M1: `Uncancel`/`Hold` thiếu guard trạng thái nguồn — uncancel một buổi đã `held`+confirmed sẽ lật về `planned` mà giữ `attendance_confirmed_at`, làm `CountBillableByEnrollment` (chỉ đếm `held`) thu thiếu tiền ở plan 04. Đã vá: `Uncancel` yêu cầu nguồn `cancelled`, `Hold` từ chối `cancelled` (409 `ErrInvalidTransition`), có test unit. Ghi vào adr.md kèm nhóm Low hoãn (test CountBillable chờ consumer plan 04, ad-hoc không clamp khung ngày lớp là cố ý, comment route, double roster-resolve).

## Verification
- Biên dịch/kiểm tra tĩnh (thường + integration) sạch; `make lint-api` 0 issues; `make api-docs` sinh swagger đủ.
- Full suite unit+integration xanh qua testcontainers Postgres. Coverage tổng 74.9% (floor 60%); sessions 83.8%, attendance 82.5% với integration.
- Baseline migration `000001` xác nhận không đổi (D1 giữ vững), round-trip up-down-up của 000003 xanh.

## Next steps
Plan 04 (Billing Engine) — tiêu thụ `attendance.CountBillableByEnrollment` và `sessions.Service.ListPending` (from/to) cho chặn chốt kỳ. Sau đó 05, 06, design-system-foundation, 07, 08.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
