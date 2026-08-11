---
title: "Manager Class Oversight"
description: "Delegation-based manager role (read-only oversight of managed teachers): grants API + web UI quản lý grant, và manager roll-up dashboard API"
status: pending
priority: P1
effort: "3.75d"
tags: [api, authz, oversight, web]
created: 2026-08-11
---

# Manager Class Oversight

## Overview

Cho phép một teacher account đóng vai **manager thuần giám sát** (không dạy lớp riêng) xem read-only dữ liệu của các giáo viên đã cấp quyền, qua mô hình **delegation grants** — không dựng tầng organization, không đổi tenant key. Kèm web UI cho giáo viên cấp/xem/thu hồi grant, và dashboard roll-up API (sĩ số, tái tục, doanh thu/buổi).

**Scope đã chốt với user (predict sessions 260811):**
- Manager: **chỉ đọc**, thuần giám sát, không có lớp riêng.
- Grant do **giáo viên bị quản lý tạo** (data owner consent), hai phía đều thu hồi được.
- **Không** điểm số học sinh, **không** export CSV/XLSX ở V1 (bỏ theo quyết định user).
- Doanh thu/buổi **derive**, không denormalize — trả **hai số**: ước tính (`enrollments.unit_price × billable attendance`) và chốt sổ (invoice_lines + adjustments, loại void).
- Luồng ghi giữ nguyên bất biến `WHERE teacher_id = $self`; chỉ read path oversight dùng `IN (managed set từ grants)`.

**Scope change 260811-1309 (quyết định user):** bỏ toàn bộ phase Session Notes and Curriculum (nhận xét buổi, curriculum template, lesson plan) khỏi plan; thay bằng phase **Delegation Grants UI** (web UI cho tính năng grants của Phase 2). Ripple đã áp dụng: Phase 1 chỉ còn migration 000007 (bỏ 000008 class_note + 000009 curriculum); Phase 4 bỏ đường đọc class_note/lesson plan/course template và chỉ phụ thuộc Phase 2.

**Non-goals:** org layer/multi-tenant mới, manager ghi thay GV, cây quản lý nhiều tầng, cost accounting (chi phí), session notes + curriculum (đã loại khỏi plan — làm plan riêng nếu cần), web UI cho dashboard giám sát của manager (chỉ UI grants trong scope; dashboard UI là plan riêng sau khi API chốt).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Manager xem được sĩ số buổi/tháng, tỷ lệ tái tục, doanh thu/buổi của GV được quản | P1 |
| 2 | GV cấp/xem/thu hồi quyền giám sát qua web UI | P1 |
| 3 | Isolation tuyệt đối: manager không grant = không thấy gì; mọi target đều verify qua bảng grant server-side | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Database Migrations](./phase-01-start.md) | Pending |
| 2 | [Phase 2: Delegation Grants and Scope Resolution](./phase-02-delegation-grants-and-scope-resolution.md) | Pending |
| 3 | [Phase 3: Delegation Grants UI](./phase-03-delegation-grants-ui.md) | Pending |
| 4 | [Phase 4: Manager Read-Only API](./phase-04-manager-read-only-api.md) | Pending |

Dependencies: 2←1, 3←2, 4←2. Phase 3 (UI) và Phase 4 có thể song song sau Phase 2.

## Metric Definitions (nguồn sự thật cho Phase 4)

Mọi metric đều lọc `deleted_at IS NULL` trên **từng bảng tham gia** — `classes`, `class_sessions`, `attendance_records`, `enrollments` (raw/Table() query không được GORM tự lọc, phải viết tay — house rule tại `teachers/repository.go:48-53`) — và scope `teacher_id` trên từng bảng con của JOIN.

- **Sĩ số buổi** = COUNT(attendance_records WHERE session_id, deleted_at IS NULL) — present + absent + excused đều tính vào sĩ số danh sách; `present_count` tách riêng.
- **Doanh thu ước tính buổi (`estimated_revenue`)** = SUM(enrollments.unit_price) trên attendance_records `billable = true, deleted_at IS NULL` của buổi; chỉ tính khi session `status='held'` và `attendance_confirmed_at IS NOT NULL` (khớp quy tắc billing hiện có). Có ngay sau điểm danh — phục vụ giám sát vận hành.
- **Doanh thu chốt sổ (`invoiced_revenue`)** = SUM(invoice_lines) + SUM(invoice_adjustments) của kỳ, loại invoice `void`. Chỉ có sau khi chốt sổ; `null` khi kỳ chưa chốt. API oversight luôn trả **cả hai số** với tên tách bạch (quyết định user 260811) — hai số có thể lệch nhau do adjustments/void, đó là thông tin, không phải bug.
- **Tỷ lệ tái tục tháng (retention)** per class = (# enrollments active ngày cuối tháng ∩ active ngày đầu tháng) / (# enrollments active ngày đầu tháng). Enrollment active = `started_on <= d AND (ended_on IS NULL OR ended_on >= d) AND deleted_at IS NULL`. HS nhập học giữa tháng không vào mẫu số. Metric này tunable — đổi định nghĩa chỉ đụng 1 query Phase 4.

## Success Criteria

- [ ] Migration 000007 up/down sạch trên DB có dữ liệu seed; `domainTables` + `docs/schema_design.sql` cập nhật
- [ ] Manager không có grant gọi mọi endpoint `/oversight/*` → 403/tập rỗng; test cross-tenant deny cho từng endpoint (gồm case tài khoản disabled/soft-deleted)
- [ ] Grant revoke có hiệu lực ngay request kế tiếp (không cache managed-set trong JWT)
- [ ] Mọi endpoint oversight đọc là đọc thuần — test assert không có row nào được ghi vào tenant của GV được quản (chặn lối `ListRange` generate)
- [ ] Web UI: GV cấp grant theo SĐT, xem hai chiều given/received, thu hồi từ cả hai phía; không hiển thị tên manager trước khi grant tồn tại; lint + typecheck + vitest pass
- [ ] Manager đọc được: overview roll-up theo tháng, chi tiết lớp, chi tiết buổi (attendance + estimated/invoiced revenue) của GV được quản
- [ ] Không endpoint ghi nào chấp nhận teacher_id từ request; luồng ghi giữ `authctx.TeacherID` nguyên trạng
- [ ] Swagger regenerate; integration tests theo pattern hiện có pass

## Open Questions

None — 4 quyết định scope đã chốt qua AskUserQuestion (read-only, thuần giám sát, bỏ điểm số, bỏ export), cộng 3 quyết định từ red-team review (xem bên dưới), cộng scope change 260811-1309 (bỏ notes/curriculum, thêm grants UI).

## Red Team Review

**260811 — 3 reviewer hostile (Security Adversary, Assumption Destroyer, Failure Mode Analyst), 15 findings sau dedupe, 13 Accept + 2 chuyển quyết định user. Toàn bộ đã áp dụng vào phase files.**

> **Ghi chú sau scope change 260811-1309:** các finding gắn với phase Session
> Notes and Curriculum đã bị loại — **#2, #7, #9, #11** không còn áp dụng (không
> còn class_note/lesson plan/curriculum trong plan); **#14** chỉ còn phần N+1
> batch `ActiveOn` + trần managed set (vẫn ở Phase 4); **#15** giờ chỉ còn 1
> bảng mới (`management_grants`). Quyết định user "Unit ↔ buổi" cũng moot.
> Bảng dưới giữ nguyên làm lịch sử review.

### Quyết định user (qua AskUserQuestion 260811)

| Câu hỏi | Quyết định |
|---|---|
| Doanh thu hiển thị cho manager | **Cả hai số**: `estimated_revenue` (attendance-based) + `invoiced_revenue` (invoice_lines + adjustments, loại void) |
| Luồng grant | **1 bước + response tối thiểu**: không thêm `accepted_at`; 201 không trả `full_name`; notification hai phía. Rủi ro enumeration còn lại chấp nhận ở V1; rate-limit là backlog riêng |
| Unit ↔ buổi | **1 unit 1 buổi**: unique partial index trên `session_lesson_plans(template_unit_id)` — *moot sau scope change 260811-1309* |

### Findings đã áp dụng

| # | Sev | Finding | Áp vào |
|---|---|---|---|
| 1 | Critical | Oversight tái dùng `ListRange` → GET của manager INSERT `class_sessions` cross-tenant (`sessions/service.go:129`) | Phase 4: `ListRangeReadOnly` + consumer interfaces + no-write test |
| 2 | Critical | Session soft-delete + regenerate id mới → mất `class_note`/lesson plan (`sessions/integration_test.go:104-119`) | *Moot — scope change 260811-1309* |
| 3 | High | Grant creation = oracle dò danh bạ (201 trả `full_name`), không rate limit | Phase 2: response tối thiểu, `TeacherLookup`, notification; rate limit → backlog |
| 4 | High | Grant không notify, comment migration claim "audit" sai | Phase 1 (sửa comment) + Phase 2 (notification hai phía) |
| 5 | High | Scope resolution bỏ qua `user_accounts.status`/`deleted_at` (đối chiếu `auth/service.go:90,151`) | Phase 2: JOIN user_accounts hai phía active + authz case Phase 4 |
| 6 | High | Roll-up bỏ `scoped()`, thiếu `deleted_at` trong metric defs (`teachers/repository.go:48-53`) | plan.md Metric Definitions + Phase 4 `scopedToManaged` + fixture soft-deleted |
| 7 | High | Import cycle classes ⇄ curriculum | *Moot — scope change 260811-1309* |
| 8 | High | Revenue ước tính lệch số invoice thật | plan.md + Phase 4: hai số (quyết định user) |
| 9 | High | Reorder "shift trong tx" bất khả thi — partial unique không DEFERRABLE | *Moot — scope change 260811-1309* |
| 10 | High | Gọi `teachers.GetByPhone` vi phạm hợp đồng "must not back an endpoint" (`teachers/service.go:73-76`) | Phase 2: `TeacherLookup` interface |
| 11 | Medium | Template/unit guard thiếu: unit bị xoá còn lesson plan trỏ; lớp archived | *Moot — scope change 260811-1309* |
| 12 | Medium | Validator `e164` không tồn tại — repo chỉ có `vnphone` (`validation.go:33`) | Phase 2: `vnphone` + `NormalizePhone`; Phase 3 UI mirror rule này client-side |
| 13 | Medium | Revoke fetch-then-check → đọc cross-tenant + TOCTOU | Phase 2: single UPDATE scope-trong-WHERE + RowsAffected |
| 14 | Medium | Drill-down thừa hưởng N+1 (`toDetail`→`ActiveOn` per session), managed set không trần | Phase 4: batch ActiveOn, trần 50 GV, success criteria phủ drill-down |
| 15 | Medium | Bỏ sót `docs/schema_design.sql` + `domainTables` bắt buộc (`migrations_test.go:23-29`) | Phase 1: Related Code Files + Success Criteria (còn 1 bảng sau scope change) |

### Rejected (đã verify không phải lỗi)

Swagger prod exposure (chỉ mount khi `!IsProduction` — `router.go:60-62`); IDOR nested id (mọi repo đọc đều `scoped()`); manager tự cấp quyền (chiều consent grantor=managed đúng, giữ nguyên); XFF/CORS (đã hardened).

### Whole-Plan Consistency Sweep

Rà lại toàn bộ sau scope change 260811-1309 (sweep gốc 260811 vẫn đúng cho phần còn giữ):

- Phase 1 chỉ còn 000007 `management_grants`; không còn tham chiếu class_note/curriculum ở bất kỳ phase nào ngoài ghi chú scope-change.
- Phase 4 dependencies `[2]`; readers còn `ClassReader`/`SessionReader`/`AttendanceReader` (bỏ `TemplateReader`); response không còn `class_note`/`lesson_plan_status`/`latest_class_note`; endpoint course-templates đã gỡ.
- Phase 3 mới (UI) phản chiếu đúng contract 3 endpoint grants của Phase 2 (201 response tối thiểu, 404 chung, revoke 404-idempotent).
- `revenue` → `estimated_revenue`/`invoiced_revenue` nhất quán; `ListRange` chỉ còn dưới dạng "bị cấm"; binding thống nhất `vnphone`.
- Không còn mâu thuẫn tồn đọng.

<!-- slug: manager-class-oversight -->
