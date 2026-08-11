---
phase: 4
title: "Owner Dashboard API"
status: pending
priority: P2
effort: "1.5d"
dependencies: [3]
---

# Phase 4: Owner Dashboard API

## Overview

Các endpoint đọc roll-up cho owner trong feature `centers`: tổng quan theo tháng (sĩ số, tái tục, doanh thu) GROUP BY teacher/lớp trên toàn center, và drill-down teacher → lớp → buổi. Kế thừa thiết kế "Manager Read-Only API" của plan cũ nhưng scope = center membership (không còn grants/VerifyManages) và caller = owner.

## Requirements

- Functional: goal #4 plan.md — owner xem sĩ số, retention, hai số doanh thu per teacher/lớp/buổi.
- Non-functional: GET-only và **GET không được ghi** (carried finding #1); mọi query qua helper scope; không N+1.

## Architecture

- Handlers thêm vào `internal/features/centers/` (cùng package Phase 2); mọi endpoint check `sc.IsOwner` → 403 cho member thường.
- **Drill-down qua consumer interface hẹp** — `ClassReader`, `SessionReader`, `AttendanceReader` chỉ chứa method đọc thuần, gọi với scope center + teacher đích (đã validate thuộc center).
- **`sessions.Service.ListRange` bị CẤM** — nó generate và INSERT session thiếu (`sessions/service.go:75-131`, `BulkInsertIgnoreConflicts` dòng 129): GET của owner mà dùng nó sẽ bơm row `planned` vào flow chốt sổ của teacher. Thêm `sessions.Service.ListRangeReadOnly` (chỉ `repo.ListByClassAndRange`, không generate); `SessionReader` chỉ expose method này. Không đổi chữ ký `ListRange` hiện có.
- **Helper `scopedToCenter(ctx, centerID)` bắt buộc cho mọi query roll-up** trong `centers/repository.go`: áp `center_id = ?` trên **từng bảng tham gia JOIN** + `deleted_at IS NULL` từng bảng viết tay (house rule `teachers/repository.go:48-53`). Pattern reporting giống `collections`: 1 query/khối metric, GROUP BY teacher/class.
- Metric definitions lấy nguyên văn từ `plan.md`; luôn trả cả `estimated_revenue` + `invoiced_revenue` (quyết định user, carried finding #8).
- Drill-down session list: batch `ActiveOn` theo nhiều `session_date` hoặc bỏ `StudentCount` khỏi DTO — không thừa hưởng N+1 của `toDetail` (`sessions/service.go:338-345`, carried finding #14).

## API Design

Tất cả GET, dưới `/api/v1`, `requireAuth` + scope middleware; 403 khi `!IsOwner`.

| Method & Path | Query params | Response 200 |
|---|---|---|
| `GET /centers/dashboard/teachers` | — | `[{"teacher": {"id","full_name","phone"}, "active_classes": 3, "active_students": 42}]` — mọi teacher trong center (kể cả owner) |
| `GET /centers/dashboard/overview` | `month=YYYY-MM` (default tháng hiện tại) | `[{"teacher_id","teacher_name","classes": [{"class_id","class_name","sessions_held","avg_attendance","present_rate","retention_rate","estimated_revenue","invoiced_revenue"}]}]` |
| `GET /centers/dashboard/teachers/:teacherId/classes` | `status=active\|archived` | class list DTO hiện có (qua `ClassReader`) |
| `GET /centers/dashboard/teachers/:teacherId/classes/:classId/sessions` | `from&to=YYYY-MM-DD` | `[{"session_id","session_date","status","attendance_total","present_count","estimated_revenue"}]` — qua `ListRangeReadOnly`, không generate row |
| `GET /centers/dashboard/sessions/:sessionId` | — | `{"session": {...}, "attendance": [{"student_name","status","billable","note"}], "estimated_revenue", "invoiced_revenue"}` (`invoiced_revenue` null khi buổi chưa vào kỳ chốt) |

Errors: `:teacherId` không thuộc center → 403 message chung (không tiết lộ tồn tại); member thường gọi bất kỳ endpoint dashboard → 403; center không có dữ liệu → `[]`.

## Authz Test Matrix (bắt buộc)

| Case | Expect |
|---|---|
| Owner gọi từng endpoint trên teacher trong center | 200 đúng dữ liệu |
| Member thường (không owner) gọi từng endpoint | 403 |
| Owner center A gọi với teacherId/classId/sessionId thuộc center B | 403 |
| Teacher bị remove khỏi center, owner gọi lại drill-down id cũ | dữ liệu ở lại center (vẫn 200 — data thuộc center, xem plan.md) |
| Owner bị đổi (center chuyển owner — ngoài scope V1) | n/a — ghi chú non-goal |
| Gọi drill-down sessions rồi đếm `class_sessions` | COUNT không đổi (assert no-write) |
| Fixture: teacher center khác + 1 row soft-deleted mỗi bảng (class/session/attendance/enrollment) | số roll-up khớp tính tay, không leak |

## Related Code Files

- Modify: `apps/api/internal/features/centers/{repository,service,handler,routes,dto}.go`
- Modify: `apps/api/internal/features/centers/integration_test.go`
- Modify: `apps/api/internal/features/sessions/{service,repository}.go` — thêm `ListRangeReadOnly` (không đổi chữ ký method hiện có)
- Modify: `apps/api/internal/server/router.go` (wire `ClassReader`/`SessionReader`/`AttendanceReader`)

## Implementation Steps

1. `sessions.Service.ListRangeReadOnly` + test riêng (assert không insert).
2. Consumer interfaces + DTO + routes GET-only, owner-check middleware-level trong handler.
3. Roll-up queries qua `scopedToCenter`: sessions_held, avg_attendance/present_rate, estimated_revenue, invoiced_revenue (loại void), retention — GROUP BY teacher, class.
4. Drill-down handlers (validate teacher ∈ center trước khi gọi reader; batch ActiveOn).
5. Integration tests theo matrix + đối chiếu số liệu tính tay trên fixture seed.
6. Regenerate swagger.

## Success Criteria

- [ ] Toàn bộ authz matrix pass (gồm no-write assert + fixture soft-deleted/cross-center)
- [ ] Số liệu overview khớp tính tay (sĩ số, present_rate, retention, hai số doanh thu)
- [ ] Không handler dashboard nào ghi DB — enforced bằng test assert COUNT
- [ ] Roll-up 1 tháng × 10 teacher × 20 lớp < 500ms trên seed local (không N+1); drill-down sessions bị chặn số query
- [ ] Swagger cập nhật

## Risk Assessment

- **JOIN thiếu `center_id`/`deleted_at` ở bảng con** → leak hoặc số sai: chốt bằng helper bắt buộc + fixture cross-center/soft-deleted, assert tính tay.
- **Retention mẫu số 0** (lớp mở giữa tháng): `retention_rate: null`, FE hiển thị "—".
- **Aggregate chậm khi center lớn**: index `center_id` partial từ Phase 1 đủ cho quy mô hiện tại; composite index để sau nếu đo thấy chậm (YAGNI).
