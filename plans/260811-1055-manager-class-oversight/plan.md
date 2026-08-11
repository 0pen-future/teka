---
title: "Center Tenancy"
description: "Chuyển tenant từ teacher sang center: bảng centers mới, re-key toàn schema sang center_id, owner full quyền trong center, teacher/student giữ nguyên hành vi; kèm centers API + UI quản lý trung tâm + dashboard owner"
status: in-progress
priority: P1
effort: "9d"
tags: [api, authz, tenancy, migration, web]
created: 2026-08-11
updated: 2026-08-11
blockedBy: [260807-1935-zalo-auto-map-contacts]
---

# Center Tenancy

## Overview

Chuyển mô hình tenancy từ **teacher-là-tenant** sang **center-là-tenant**: tạo bảng `centers`, re-key toàn bộ schema nghiệp vụ sang `center_id`, mỗi center có **owner** (full read + write trên mọi dữ liệu trong center), **teacher** và **student** giữ nguyên hành vi hiện tại. Thay thế hoàn toàn mô hình delegation grants của bản plan trước (xem Lịch sử scope).

**4 quyết định user (AskUserQuestion 260811-1700):**

| Câu hỏi | Quyết định |
|---|---|
| Tenant key | **Re-key sang `center_id`**: thêm `center_id` vào mọi bảng nghiệp vụ, composite FK đổi `(id, teacher_id)` → `(id, center_id)`, mọi repo `scoped()` đổi sang center. `teacher_id` giữ lại làm attribution + scope phụ cho role teacher |
| Quyền owner | **Full read + write**: owner đọc và ghi thay bất kỳ teacher nào trong center (tạo lớp, điểm danh, chốt sổ billing...). Bất biến ghi `teacher_id = $self` chỉ còn áp cho role teacher |
| Backfill | **Auto center cá nhân**: migration tạo 1 center riêng cho mỗi teacher hiện có, chính họ là owner — không ai mất quyền, hành vi không đổi |
| Membership | **1 teacher = 1 center** tại một thời điểm (cột `teachers.center_id NOT NULL`); teacher nào cũng có center (cá nhân hoặc gia nhập) |

**Quyết định planner (user có thể đảo, xem Open Questions):**
- **Join do teacher khởi tạo** (consent — mirror mô hình grant cũ: bên trao quyền hành động): teacher gọi `POST /centers/join {owner_phone}`. Owner KHÔNG kéo teacher vào được — chiều ngược lại cho phép chiếm teacher qua SĐT.
- **Center sở hữu dữ liệu, không phải teacher**: teacher rời/bị remove khỏi center → dữ liệu ở lại center (hồ sơ, invoice của trung tâm phải toàn vẹn), teacher về center cá nhân mới rỗng. Hệ quả: V1 chỉ cho join khi center cá nhân của người join **rỗng** (không member khác, không dữ liệu nghiệp vụ) — chuyển dữ liệu giữa centers là plan riêng.
- **Membership resolve từ DB mỗi request, không cache vào JWT** — remove/kick hiệu lực ngay request kế (nhất quán quyết định revoke-ngay của plan cũ).
- **Ranh giới toàn vẹn DB chuyển từ teacher → center**: composite FK `(id, center_id)` cho phép ghép row cross-teacher trong cùng center (chủ đích — owner ghi thay); isolation teacher-với-teacher trong center chỉ còn enforce ở query layer (`scoped()`), test phải phủ.

**Non-goals:** teacher thuộc nhiều center; chuyển dữ liệu giữa centers; tài khoản student/parent đăng nhập (role `students` giữ nguyên trạng thái "schema-only" — student vẫn là data rows teacher/owner quản lý); web UI cho dashboard owner (API only — UI là plan riêng); RLS; rate-limit (backlog riêng như plan cũ).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Tenant = center: mọi bảng nghiệp vụ key theo `center_id`, cross-center deny tuyệt đối | P1 |
| 2 | Owner đọc + ghi mọi dữ liệu trong center; teacher giữ nguyên hành vi hiện tại (chỉ thấy/ghi dữ liệu của mình) | P1 |
| 3 | Teacher hiện có không mất gì: backfill center cá nhân, mọi flow cũ chạy nguyên trạng | P1 |
| 4 | Owner xem roll-up sĩ số/tái tục/doanh thu theo teacher/lớp trong center | P2 |
| 5 | Web UI quản lý trung tâm: xem/đổi tên, join, rời, remove member | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Centers Migration (re-key center_id)](./phase-01-centers-migration.md) | Done |
| 2 | [Phase 2: Auth Scope and Centers Feature](./phase-02-auth-scope-and-centers-feature.md) | Pending |
| 3 | [Phase 3: Backend Re-key Sweep](./phase-03-backend-rekey-sweep.md) | Pending |
| 4 | [Phase 4: Owner Dashboard API](./phase-04-owner-dashboard-api.md) | Pending |
| 5 | [Phase 5: Center Management UI](./phase-05-center-management-ui.md) | Pending |

Dependencies: 2←1, 3←2, 4←3, 5←2. Phase 5 (UI) song song được với Phase 3/4.

## Authz Invariants (nguồn sự thật cho mọi phase)

- **Scope đọc**: owner → `center_id = $ctx.center`; teacher → `center_id = $ctx.center AND teacher_id = $self`. Không query nào thiếu vế center.
- **Scope ghi**: create luôn gán `teacher_id = $self` cho MỌI role — owner cũng là teacher bình thường với resource của mình, **không tạo hộ** (quyết định validate 260811); owner sửa/xoá được mọi row đã tồn tại trong center. Không endpoint nào nhận `teacher_id`/`center_id` từ request. Composite FK `(teacher_id, center_id) REFERENCES teachers(id, center_id)` vẫn là chốt chặn toàn vẹn DB.
- **Nguồn scope duy nhất** = `authctx` + membership resolve từ DB mỗi request. Không bao giờ tin `center_id`/`teacher_id` từ request để scope.
- **GET không được ghi** — carried finding: `sessions.Service.ListRange` generate + INSERT session (`sessions/service.go:75-131`); mọi đường đọc dashboard phải dùng `ListRangeReadOnly`.

## Metric Definitions (nguồn sự thật cho Phase 4)

Mọi metric lọc `deleted_at IS NULL` trên **từng bảng tham gia** — raw/Table() query không được GORM tự lọc, phải viết tay (house rule tại `teachers/repository.go:48-53`) — và scope `center_id` trên từng bảng của JOIN, GROUP BY theo `teacher_id`/`class_id`.

- **Sĩ số buổi** = COUNT(attendance_records WHERE session_id, deleted_at IS NULL) — present + absent + excused đều tính; `present_count` tách riêng.
- **Doanh thu ước tính buổi (`estimated_revenue`)** = SUM(enrollments.unit_price) trên attendance_records `billable = true, deleted_at IS NULL`; chỉ tính khi session `status='held'` và `attendance_confirmed_at IS NOT NULL`. Có ngay sau điểm danh.
- **Doanh thu chốt sổ (`invoiced_revenue`)** = SUM(invoice_lines) + SUM(invoice_adjustments) của kỳ, loại invoice `void`; `null` khi kỳ chưa chốt. API luôn trả **cả hai số** tên tách bạch (quyết định user 260811 — giữ nguyên từ plan cũ).
- **Tỷ lệ tái tục tháng (retention)** per class = (# enrollments active ngày cuối tháng ∩ active ngày đầu tháng) / (# enrollments active ngày đầu tháng). Enrollment active = `started_on <= d AND (ended_on IS NULL OR ended_on >= d) AND deleted_at IS NULL`. Mẫu số 0 → `retention_rate: null`.

## Success Criteria

- [ ] Migration 000007 up/down/up sạch trên DB seed; mỗi teacher hiện có đúng 1 center cá nhân, mọi row nghiệp vụ có `center_id` khớp; `domainTables` + `docs/schema_design.sql` cập nhật
- [ ] Cross-center deny: mọi endpoint với id thuộc center khác → 403/404/tập rỗng; test phủ từng feature package
- [ ] Teacher A không thấy dữ liệu teacher B **cùng center** (hành vi cũ giữ nguyên); owner thấy và ghi được cả hai
- [ ] Remove/kick member hiệu lực ngay request kế tiếp (không cache membership trong JWT)
- [ ] Đăng ký teacher mới tự tạo center cá nhân; mọi integration test cũ pass sau sweep (fixture thêm center)
- [ ] Owner dashboard: overview roll-up theo tháng, drill-down teacher → lớp → buổi, hai số doanh thu; GET không ghi row nào (assert COUNT)
- [ ] Web UI: xem center + members, join theo SĐT owner, rời/remove với confirm; lint + typecheck + vitest pass
- [ ] Swagger regenerate; `grep authctx.TeacherID` ngoài authctx = 0 sau sweep (thay bằng Scope)

## Open Questions

1. **Red-team pass mới**: plan đổi mô hình gốc rễ — review cũ chỉ còn giá trị một phần (xem dưới). Nên chạy `/ak:plan red-team` lại trước khi cook, đặc biệt Phase 1 (migration big-bang) và Phase 3 (nới bất biến ghi).

## Validation Log

### Session 1 — 260811-1700 (validate interview, 3 câu)

| Câu hỏi | Quyết định user |
|---|---|
| Unique nghiệp vụ `contacts(teacher_id, phone)`, `billing_periods(teacher_id, year, month)` | **Giữ per-teacher** — không đổi ngữ nghĩa; 2 teacher cùng center có thể trùng SĐT contact; billing chốt sổ per teacher như hiện tại |
| Điều kiện join center V1 | **Center cá nhân rỗng mới được join** (không member khác, không classes/students/contacts) — có dữ liệu → 409; chuyển dữ liệu giữa centers là plan riêng |
| Owner tạo resource hộ teacher khác | **Không tạo hộ** — create luôn gán chính người tạo (owner cũng là teacher); owner chỉ đọc + sửa/xoá row tồn tại của teacher khác. Đơn giản hoá Phase 3: không DTO nào nhận `teacher_id` |

### Verification Results

- Claims checked: 16 (file paths, symbols, line refs, schema columns, web patterns)
- Verified: 16 | Failed: 0 | Unverified: 0
- Tier: Full (5 phases)
- Failures: none — mọi tham chiếu `service.go`/`repository.go`/`router.go`/schema/web pattern khớp codebase hiện tại

### Whole-Plan Consistency Sweep

Sau propagate quyết định "không tạo hộ": plan.md Authz Invariants + Phase 3 (Requirements, Architecture Write, Implementation Steps, Success Criteria) đã đồng bộ "create = $self mọi role, không DTO teacher_id"; Phase 1/2/4/5 không tham chiếu create-hộ — không còn mâu thuẫn tồn đọng.

## Lịch sử scope

- **260811-1055**: plan gốc "Manager Class Oversight" — delegation grants, manager read-only, không org layer. Đã qua red-team (15 findings) + validate.
- **260811-1309**: bỏ Session Notes & Curriculum, thêm Delegation Grants UI.
- **260811-1700 (bản này)**: user đổi hướng sang **center tenancy** — thay grants bằng centers, re-key toàn schema, owner full quyền. Non-goal "không org layer/multi-tenant mới" của bản gốc bị đảo bởi quyết định user. Toàn bộ phase viết lại; giữ dir slug cũ.

### Findings red-team cũ còn hiệu lực (đã nhúng vào phase mới)

| Finding gốc | Áp vào |
|---|---|
| #1 Critical: `ListRange` INSERT trong đường GET | Phase 4: `ListRangeReadOnly` + no-write test |
| #5: scope phải kiểm `user_accounts.status`/`deleted_at` hai phía (`auth/service.go:90,151`) | Phase 2: membership resolve JOIN user_accounts active |
| #6: raw query thiếu `deleted_at`/scope từng bảng JOIN | Phase 4: helper `scopedToCenter` + fixture soft-deleted |
| #8: hai số doanh thu | Metric Definitions + Phase 4 |
| #10: `teachers.GetByPhone` "must not back an endpoint" (`teachers/service.go:73-76`) | Phase 2: consumer interface `TeacherLookup` |
| #12: validator `e164` không tồn tại, dùng `vnphone` + `NormalizePhone` (`validation.go:33`) | Phase 2 + Phase 5 |
| #13: revoke = single UPDATE scope-trong-WHERE, RowsAffected | Phase 2: remove/leave member |
| #14: N+1 `toDetail`→`ActiveOn`; trần tập IN | Phase 4 |
| #15: `domainTables` (`migrations_test.go:23-29`) + `docs/schema_design.sql` bắt buộc | Phase 1 |

Findings gắn với grants (#3 oracle 201, #4 notify grant) chuyển hoá: notification hai phía áp cho join/remove; response join không trả thông tin owner ngoài phone caller nhập.

<!-- slug: manager-class-oversight -->
