---
phase: 4
title: "Manager Read-Only API"
status: pending
priority: P1
effort: "1.5d"
dependencies: [2]
---

# Phase 4: Manager Read-Only API

## Overview

Các endpoint đọc cho manager trong feature `oversight`: roll-up theo tháng (sĩ số, tái tục, doanh thu) trên toàn bộ managed set, và drill-down chi tiết GV → lớp → buổi (attendance + doanh thu). Mọi handler đều đi qua `VerifyManages`/`ManagedTeacherIDs` của Phase 2 — không có đường đọc cross-teacher nào khác. (Đường đọc class_note/lesson plan/course template đã bị loại cùng phase Session Notes and Curriculum — scope change 260811.)

## Requirements

- Functional: yêu cầu #1 (sĩ số + tái tục), #4 (doanh thu/buổi) của user.
- Non-functional: GET-only; tập `IN` luôn server-built; JOIN nào cũng scope `teacher_id`; không N+1 trên roll-up.

## Architecture

- Handlers mới trong `internal/features/oversight/` (cùng package Phase 2).
- **Drill-down qua consumer interface hẹp, KHÔNG nhúng `*Service` nguyên khối** (red-team Critical): oversight định nghĩa `ClassReader`, `SessionReader`, `AttendanceReader` — chỉ chứa các method **đọc thuần**. Sau khi `VerifyManages(caller, targetID)` pass thì gọi với `targetID`.
- **`sessions.Service.ListRange` bị CẤM ở oversight** — nó không phải service đọc: nó sinh và INSERT session còn thiếu (`apps/api/internal/features/sessions/service.go:75-131`, `BulkInsertIgnoreConflicts` dòng 129). Dùng nó nghĩa là manager GHI `class_sessions` vào tenant GV khác qua một GET — bơm row `planned` vào cảnh báo pending + flow chốt sổ billing của họ. Thay bằng method mới `sessions.Service.ListRangeReadOnly` (chỉ `repo.ListByClassAndRange`, không generate) và `SessionReader` chỉ expose method này. Rà các service còn lại theo cùng tiêu chí trước khi wire (đã verify: các đường đọc của classes/attendance/enrollments không ghi).
- Roll-up viết query aggregate riêng trong `oversight/repository.go` (pattern reporting giống `collections`): 1 query/khối metric trên `teacher_id IN (managed)`, GROUP BY teacher/class.
- **Mọi query roll-up bắt buộc đi qua helper `scopedToManaged(ctx, ids)`** trong `oversight/repository.go` — không query nào viết ngoài nó. Helper áp: `teacher_id IN (?)` trên **từng bảng tham gia JOIN** (kể cả khi FK đã đảm bảo — pattern `apps/api/internal/features/sessions/repository.go:71-76`) và `deleted_at IS NULL` trên từng bảng (raw/Table() query không được GORM tự lọc — cảnh báo tại `apps/api/internal/features/teachers/repository.go:48-53`). "Review checklist" không phải cơ chế — helper + fixture test mới là.
- Metric definitions lấy nguyên văn từ `plan.md` — doanh thu derive, không denormalize; trả **cả hai số** `estimated_revenue` + `invoiced_revenue` (quyết định user 260811).

## API Design

Tất cả GET, dưới `/api/v1`, `requireAuth`; caller = manager từ `authctx.TeacherID`.

| Method & Path | Query params | Response 200 |
|---|---|---|
| `GET /oversight/teachers` | — | `[{"teacher": {"id","full_name","phone"}, "active_classes": 3, "active_students": 42}]` — từ `ManagedTeacherIDs`, tập rỗng → `[]` |
| `GET /oversight/overview` | `month=YYYY-MM` (default tháng hiện tại) | `[{"teacher_id","teacher_name","classes": [{"class_id","class_name","sessions_held","avg_attendance": 11.5,"present_rate": 0.93,"retention_rate": 0.87,"estimated_revenue": 12600000,"invoiced_revenue": 12100000}]}]` |
| `GET /oversight/teachers/:teacherId/classes` | `status=active\|archived` | class list DTO hiện có (qua `ClassReader`) |
| `GET /oversight/teachers/:teacherId/classes/:classId/sessions` | `from=YYYY-MM-DD&to=YYYY-MM-DD` | `[{"session_id","session_date","status","attendance_total","present_count","estimated_revenue"}]` — qua `SessionReader.ListRangeReadOnly`, **không generate row** |
| `GET /oversight/teachers/:teacherId/sessions/:sessionId` | — | `{"session": {...session DTO hiện có}, "attendance": [{"student_name","status","billable","note"}], "estimated_revenue": 1200000, "invoiced_revenue": 1200000}` (`invoiced_revenue` null khi buổi chưa vào kỳ chốt sổ nào) |

**Hai số doanh thu (quyết định user 260811):** `estimated_revenue` = định nghĩa
attendance-based trong `plan.md` (có ngay sau điểm danh, phục vụ giám sát vận
hành); `invoiced_revenue` = SUM từ `invoice_lines` + `invoice_adjustments`,
loại invoice `void` (số thật sau chốt sổ). API luôn trả cả hai, đặt tên rõ để
FE không trình bày số ước tính như số đã thu.

Errors: mọi `:teacherId` không có grant sống → 403 (message chung, không tiết lộ teacher tồn tại hay không). Không endpoint nào nhận danh sách teacher_ids từ client.

## Authz Test Matrix (bắt buộc, mirror Risk "Critical" từ predict)

| Case | Expect |
|---|---|
| Manager có grant gọi từng endpoint | 200 đúng dữ liệu target |
| Manager không grant gọi từng endpoint với teacherId bất kỳ | 403 |
| Grant bị revoke, gọi lại ngay | 403 |
| Tài khoản managed bị `disabled`/soft-delete, manager gọi lại | 403 ngay request kế |
| `overview` khi không quản ai | `[]` |
| Managed teacher gọi ngược endpoint oversight với id của manager | 403 |
| Gọi endpoint sessions drill-down rồi đếm `class_sessions` của target | COUNT không đổi (assert no-write) |
| Fixture roll-up chứa: GV thứ 3 không được quản + 1 lớp, 1 session, 1 attendance, 1 enrollment soft-deleted | số liệu tính tay loại trừ hết; không leak |

## Related Code Files

- Modify: `apps/api/internal/features/oversight/{repository,service,handler,routes,dto}.go`
- Modify: `apps/api/internal/features/oversight/integration_test.go`
- Modify: `apps/api/internal/features/sessions/{service,repository}.go` — thêm `ListRangeReadOnly` (đọc thuần, không generate); không đổi chữ ký method hiện có (13 consumer non-test ngoài package)
- Modify: `apps/api/internal/server/router.go` (wire qua các consumer interface `ClassReader`/`SessionReader`/`AttendanceReader`)

## Implementation Steps

1. `sessions.Service.ListRangeReadOnly` + test riêng (không insert row nào).
2. Consumer interfaces `ClassReader`/`SessionReader`/`AttendanceReader` trong oversight; DTO + routes GET-only.
3. Drill-down handlers: `VerifyManages` → gọi reader với `targetID`. Đường session-list dùng batch `ActiveOn` theo nhiều `session_date` (hoặc bỏ `StudentCount` khỏi DTO oversight) — không thừa hưởng N+1 của `toDetail` (`apps/api/internal/features/sessions/service.go:338-345`).
4. Roll-up repository queries qua `scopedToManaged`: sessions_held, avg_attendance/present_rate, estimated_revenue (join enrollments qua attendance billable), invoiced_revenue (invoice_lines + adjustments, loại void), retention (định nghĩa trong plan.md) — GROUP BY class. Trần managed set: 50 GV, vượt → 422 (chặn IN-set không giới hạn).
5. Overview handler ghép các khối metric theo class/teacher.
6. Integration tests theo authz matrix (gồm no-write assert + fixture soft-deleted) + đúng số liệu trên fixture seed.
7. Regenerate swagger.

## Success Criteria

- [ ] Toàn bộ authz matrix pass (gồm case disabled-account, no-write, soft-deleted fixture)
- [ ] Số liệu overview khớp tính tay trên fixture (sĩ số, present_rate, retention, estimated + invoiced revenue)
- [ ] Không handler oversight nào ghi DB — **enforced bằng test assert COUNT**, không phải review; router chỉ đăng ký GET (trừ grants Phase 2)
- [ ] Roll-up 1 tháng × 10 GV × 20 lớp chạy < 500ms trên seed local (không N+1)
- [ ] Drill-down sessions 1 lớp × range tối đa: số query bị chặn trên (assert query count hoặc < 100ms trên seed local) — success criteria phủ cả drill-down, không chỉ roll-up

## Risk Assessment

- **JOIN thiếu scope teacher_id / deleted_at ở bảng con** → lộ chéo hoặc số sai: chốt bằng cơ chế — helper `scopedToManaged` bắt buộc + fixture chứa GV thứ 3 và row soft-deleted mỗi bảng, assert số tính tay.
- **Retention gây hiểu nhầm** với lớp mới mở giữa tháng (mẫu số 0): trả `retention_rate: null` thay vì 0/0 — FE hiển thị "—".
- **Aggregate chậm khi dữ liệu lớn**: index hiện có trên teacher_id các bảng chính đủ cho quy mô trung tâm; nếu chậm mới thêm index tổng hợp (YAGNI).
