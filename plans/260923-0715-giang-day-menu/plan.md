---
title: "Menu Giảng dạy: lớp học, lời mời, khóa học, lộ trình, kho học liệu, chuẩn bị tài liệu"
description: "Triển khai 100% nhóm menu 'Giảng dạy' của prototype So Lop v5 trên ba lớp api / migration / ui: danh sách + chi tiết lớp học, lời mời nhận lớp, kho học liệu (chương trình mẫu, học liệu, bài tập), danh mục khóa học, lộ trình học, chương trình học của lớp và luồng chuẩn bị tài liệu (chỉ tạo trên web, không import)."
status: in-progress
priority: P1
effort: "15.5d"
issue: ""
branch: feat/giang-day-menu
tags: [classes, curriculum, library, courses, invitations, web, api, migration, deep]
blockedBy: []
blocks: []
created: 2026-09-23
---

# Menu "Giảng dạy" (prototype v5) — kế hoạch Deep

## Overview

Prototype v5 thêm nhóm sidebar **Giảng dạy**. Plan này dựng nhóm đó thành sản phẩm thật, mỗi
phase là một lát dọc **migration → API (feature folder + routespec + catalog) → web (feature,
route, nav, test)**. Scout: [`reports/scout-260923-giang-day-menu.md`](./reports/scout-260923-giang-day-menu.md).

Deep mode: Phase 1 chi tiết đến file; Phase 2–9 là outline có quyết định thiết kế và sườn
migration, **mỗi phase phải scout lại trước khi thực thi** (`/ak:scout` trên các đường dẫn ghi
trong "Scout trước khi làm").

## Giả định & quyết định người dùng

| # | Nội dung | Trạng thái |
|---|---|---|
| A1 | Menu "Giảng dạy" gồm: Danh sách lớp học (+Chi tiết lớp học, Tạo lớp mới), Lời mời nhận lớp, Lộ trình học, Danh mục khóa học (+Chi tiết khóa học), Kho học liệu (+Chương trình mẫu, Buổi học mẫu), Chuẩn bị tài liệu (+Bảng chuẩn bị, Phân công & tiến độ, Chi tiết buổi chuẩn bị, Tạo chương trình) | **Xác nhận** — Validation Session 1 (2026-09-23), Q1 <!-- Updated: Validation Session 1 - Q1 --> |
| U1 | Chuẩn bị tài liệu: **chỉ tạo trên web, không import Excel**; bỏ màn "Nguồn dữ liệu" | Quyết định user 2026-09-22 |
| U2 | Các mục "Dạy học" hiện có (Điểm danh, Quản lý lớp học, Hồ sơ học sinh, Phụ huynh) giữ nguyên, không dựng lại | Quyết định user |
| N1 | Non-goal: đồng bộ Zalo OA cho kênh chat lớp; nhắc lời mời qua SMS/Zalo; import dữ liệu | Ghi nhận |

## Quyết định thiết kế xuyên phase

| # | Quyết định | Lý do đã kiểm chứng |
|---|---|---|
| D1 | Mở rộng feature `classes` (thêm `code`, `tags`, `recruiting`, `note`, `course_id`, `parent_class_id`) thay vì tạo feature lớp mới. Mã lớp sinh ở **một helper chung** `shared/classcode` dùng bởi `Create`, `CreateAnchored` (imports), `testutil.Class`, seeds; backfill bằng `ROW_NUMBER()` theo trung tâm <!-- Red Team S1 F3 --> | `classes` đã có read/write port theo `class_staff` (`repository.go:19-60`); web `roster` đã có `ClassDialog` = `modalClass`; `testutil.Class` chèn thẳng ở 35 file test |
| D2 | "Lời mời nhận lớp" là feature API mới `classinvites` + bảng `class_invitations`. **Lời mời là đề xuất**: người được mời chỉ đổi trạng thái (`accept/decline`, `KindService` fail-closed theo `teacher_id`); việc ghi `class_staff` là hành động **owner** qua route `confirm` ("GV nhận lớp") → vai `giao_vien` qua `handoff.Service.Reassign`, vai khác qua `classstaff.Service.Assign`. Gửi/hủy/nhắc/confirm là `KindOwnerOnly`; **không có key quyền mới**; cấm tự mời <!-- Red Team S1 F1 --> | `Reassign` và `Assign` đều owner-only (`handoff/service.go:111-112`, `classstaff/service.go:85`); bề mặt `class_staff` không grantable (`routespec.go:32-34`); `KindService` là kind hợp lệ (`routespec.go:45`) |
| D3 | Kho học liệu = feature API `library` (template, phiên bản bất biến sau publish, buổi mẫu, học liệu, bài tập, log fields, bộ điểm, **và** bảng chuẩn bị trên bản nháp); khóa học = `courses`; lộ trình = `paths`; điều phối lớp↔chương trình = `classprogram`; chat lớp = `classchat` <!-- Red Team S1 F4, F11 --> | Ranh giới domain rõ, mỗi feature một bảng gốc; feature điều phối đứng trên `classes`+`teaching`+`library` theo khuôn `handoff` vì `classes` dựng trước `teaching` trong `router.go` (:155 vs :220) |
| D4 | Kế thừa chương trình: `program_template_versions` → `courses.default_template_version_id` → `class_programs.template_version_id`; **áp dụng/đổi** ghi cả `class_programs` và `class_curricula.lessons` (tên buổi) qua `teaching.PutCurriculum`; **gỡ** chỉ xoá `class_programs`; áp dụng/đổi/gỡ **chỉ owner** (`KindOwnerOnly`) <!-- Red Team S1 F4 --> <!-- Updated: Validation Session 1 - Q5 --> | Classbook/giáo án chạy theo `lesson index` của `class_curricula` — gỡ chương trình không được xoá Sổ đầu bài |
| D5 | Quyền: mọi key `*.read` (+ `class_messages.post`) dùng `def()` + backfill migration 2 bảng (step label riêng); mọi key `*.edit`, `library.publish`, `prep.assign` dùng `optIn()`. **Bump `CatalogVersion` 4→5 đúng một lần** ở phase thêm key cuối (Phase 8 — xác nhận trong scope, Q2) cùng `catalog_test.go:316-318` và MSW `handlers.ts:17`; mỗi phase thêm key phải thêm nhóm vào `RESOURCE_LABELS` <!-- Red Team S1 F6, F7 --> | `docs/adding-permissions.md` §2, §4; `optIn` đang dùng cho `tasks.manage_board`, `members.list`; bump nhiều lần làm lệch mirror test |
| D6 | Web: lớp học ở feature `roster` (trang `/classes`, `/classes/:id`, `/class-invitations`); feature mới `library` (kho + prep), `courses` (khóa + lộ trình); nav nhóm "Giảng dạy" đặt sau "Dạy học"; mỗi entry nav phải vào cả `OVERFLOW_LABELS` và `OVERFLOW_PATH_PREFIXES` <!-- Red Team S1 F14 --> | Import chéo chỉ qua `index.ts`; `roster` đã sở hữu API lớp; `dashboard-layout.tsx:172,194` |
| D7 | Không có cột `source` cho buổi học; cột "NGUỒN" suy từ khớp `weekday + start_time` với schedule **hiệu lực tại `session_date`** (`effective_from ≤ date ≤ effective_to`); tab Buổi học gọi `GET /classes/:id/sessions?from&to&readonly=true` một lần cho cả khoảng của lớp: đường read-only không materialise, không cap 400 ngày, `student_count` đếm theo lô <!-- Red Team S1 F10 --> <!-- Updated: Validation Session 1 - cap 400 ngày --> <!-- Updated: Phase 1 review H1/H2 - readonly thay cho cửa sổ 400 ngày --> | `class_sessions` materialize khi đọc theo khoảng bắt buộc; schedule có hiệu lực theo thời gian nên so với schedule hiện tại sẽ sai với buổi cũ |
| D8 | **Mọi bảng mới có `center_id NOT NULL`**; bảng được tham chiếu có `UNIQUE (id, center_id)`; mọi FK là **composite `(fk_id, center_id)`** với `ON DELETE CASCADE` theo khuôn 000009/000015 (chỉ chạy khi xoá cứng); **logic nghiệp vụ không dựa vào cascade** vì lớp soft-delete và thành viên soft-leave (`left_at`) <!-- Red Team S1 F2, F5 --> <!-- Updated: Validation Session 1 - D8 theo quy ước repo 000007 --> | Hàng rào DB chống tham chiếu chéo trung tâm khi service quên kiểm; `center_members` PK `(teacher_id, center_id)`, không xoá dòng khi rời (000007:63) |
| D9 | **Mọi route mutating** Phase 2–8 khai `req(action, entity, id)` trong routespec; mỗi phase cập nhật `route_policy_snapshot_test.go` và `audit/action_test.go`; Lịch sử thay đổi đọc `GET /audit-logs?entity_type=&entity_id=` (additive) + index `(center_id, entity_type, entity_id, occurred_at DESC)` <!-- Red Team S1 F15 --> | Hai bảng snapshot là hand-maintained (140 / 78 entry) — quên là `make test-api-unit` đỏ; đường dẫn audit thật là `/audit-logs` |
| D10 | Mọi migration có `.down.sql` đầy đủ; backfill quyền dùng step label riêng để down xoá đúng dòng; **rollback sau khi production có dữ liệu = forward migration**, không chạy down <!-- Red Team S1 F8 --> | Khuôn `000022_task_board`; production `teka-*` đang chạy live |

## Phases

| # | Phase | Effort | Status |
|---|-------|--------|--------|
| 1 | [Nav "Giảng dạy" + Danh sách lớp học + Chi tiết lớp học (Thông tin, Học viên, Buổi học)](./phase-01-nav-danh-sach-chi-tiet-lop.md) | 2d | Completed |
| 2 | [Lời mời nhận lớp + Đội ngũ giảng dạy](./phase-02-loi-moi-nhan-lop.md) | 1.5d | Completed |
| 3 | [Kho học liệu: Chương trình mẫu, phiên bản, Buổi học mẫu](./phase-03-kho-hoc-lieu-chuong-trinh-mau.md) | 2d | Completed |
| 4 | [Kho học liệu: học liệu, bài tập, log fields, bộ điểm](./phase-04-kho-hoc-lieu-hoc-lieu-bai-tap.md) | 2d | Completed |
| 5 | [Danh mục khóa học + Chi tiết khóa học](./phase-05-danh-muc-khoa-hoc.md) | 1.5d | Completed |
| 6 | [Lộ trình học](./phase-06-lo-trinh-hoc.md) | 1d | Completed |
| 7 | [Chi tiết lớp học: Chương trình học, Bài tập, Tài liệu, Chat, Lịch sử](./phase-07-chi-tiet-lop-chuong-trinh-tab.md) | 2.5d | Completed |
| 8 | [Chuẩn bị tài liệu (bảng chuẩn bị trên bản nháp, web-only)](./phase-08-chuan-bi-tai-lieu.md) | 1.5d | Completed |
| 9 | [E2E, seed, docs, ship](./phase-09-e2e-docs-seed-ship.md) | 1.5d | In progress (chờ ship) |

Thứ tự: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9. Phase 2 và 3 độc lập file, chạy song song được;
Phase 5 cần 1 + 3; Phase 6 cần 5; Phase 7 cần 1 + 4 + 5; **Phase 8 cần 3** (bảng chuẩn bị nằm trên
`template_lessons`) <!-- Red Team S1 F11 -->; Phase 9 cần 1–8. <!-- Updated: Validation Session 1 - Q2 -->

## Success Criteria

- [ ] Sidebar có nhóm "Giảng dạy" đúng 6 mục A1, mỗi mục ẩn/hiện theo permission và có deep-link guard; mobile overflow nhận diện đúng route đang mở.
- [ ] Mọi route mới có `routespec.Spec`; mọi route mutating có `req()`; `make test-api-unit`, `make scopelint`, `make test-web`, `make lint` xanh sau từng phase; `make test-api` (serial, toàn bộ) xanh sau Phase 1 và Phase 9.
- [ ] Migration `000025`–`000032` up/down/up sạch trên DB rỗng và DB đã seed (`TestMigrationRoundTrip` + test backfill riêng cho từng migration mới trong `migrations_test.go`; `backfill_parity_test.go` chỉ đóng băng 000018, **không** phải tiêu chí cho migration mới); mọi FK mới là composite theo D8. <!-- Updated: Red Team S1 scope audit follow-up - backfill_parity -->
- [ ] Owner/member/member-bị-deny/cross-center đúng bảng 8 dòng của `docs/adding-permissions.md` cho từng key mới; `CatalogVersion` bump đúng một lần (4→5) và mirror khớp.
- [ ] Áp dụng chương trình mẫu vào lớp không làm đổi trạng thái giáo án đã duyệt (`lesson_plans`); gỡ chương trình không xoá `class_curricula`.
- [ ] Không có stint `class_staff` nào được tạo mà không có hành động của owner (accept lời mời chỉ đổi trạng thái). <!-- Red Team S1 F1 -->
- [ ] E2E: tạo lớp từ Danh sách lớp học; mời giáo viên → GV chấp nhận → owner "GV nhận lớp" → xuất hiện trong Đội ngũ; tạo template → khóa học → áp dụng vào lớp → tab Buổi học có tên buổi.

## Red Team Review

**Session 1 — 2026-09-23.** Bốn reviewer (Security, Assumptions, Failure, Scope) → 40 finding thô, gộp
thành 15 cụm; user duyệt "Áp dụng toàn bộ". Báo cáo gốc: [`reports/redteam-security-260923.md`](./reports/redteam-security-260923.md),
[`reports/redteam-assumptions-260923.md`](./reports/redteam-assumptions-260923.md),
[`reports/redteam-failure-260923.md`](./reports/redteam-failure-260923.md),
[`reports/redteam-scope-260923.md`](./reports/redteam-scope-260923.md). Marker trong file: `<!-- Red Team S1 F# -->`.

| # | Finding | Severity | Disposition | Applied To |
|---|---|---|---|---|
| F1 | Phase 2 cho invitee tự ghi `class_staff` qua `Reassign` (owner-only) và key `class_invites.manage` "authenticated" không có kind tương ứng | Critical | Applied — lời mời là đề xuất; confirm owner-only; bỏ key; `KindService` fail-closed; cấm tự mời | plan D2; phase-02 Key decisions, API, Web |
| F2 | `center_members` soft-leave (`left_at`) → FK cascade không bao giờ chạy, lời mời "mồ côi"; member đã rời vẫn accept được | High | Applied — `IsActiveMember` khi gửi/accept/confirm; hủy lời mời mở khi gỡ thành viên; không cascade | plan D8; phase-02 |
| F3 | Backfill `code` từ 6 hex đầu UUIDv7 (timestamp) trùng trong cùng cửa sổ thời gian; `code NOT NULL` phá `CreateAnchored`/`imports`/`testutil.Class` (35 file) | Critical | Applied — `ROW_NUMBER()`; helper `shared/classcode` dùng chung; `imports` vào ma trận test | plan D1; phase-01 R1/R2, A1, B4–B5, inventory, risks |
| F4 | Phase 7 bơm `teaching.Service` vào `classes` tạo vòng dựng (`classes` :155 trước `teaching` :220); "Gỡ" xoá `class_curricula` mất Sổ đầu bài; gate `classes.edit` lệch `CapLessonPlanWrite` | Critical | Applied — feature điều phối `classprogram`/`classchat`; gỡ chỉ xoá `class_programs`; gate owner/giao_vien stint mở | plan D3, D4; phase-07 |
| F5 | Bảng mới thiếu `center_id`/FK đơn cột → tham chiếu chéo trung tâm nếu service quên kiểm | Critical | Applied — D8 composite FK, `UNIQUE (id, center_id)` | plan D8; phase-02..08 migration sketch |
| F6 | `library.edit`, `courses.edit`, `paths.edit` là `def()` → mọi GV sửa được nội dung/giá | High | Applied — `*.edit` optIn; `*.read` def | plan D5; phase-03..06, 08 |
| F7 | Bump `CatalogVersion` mỗi phase làm lệch `catalog_test.go:316-318`/MSW; quên `route_policy_snapshot_test.go`, `audit/action_test.go`, `RESOURCE_LABELS` | High | Applied — bump một lần; inventory mọi phase có 3 file test + `RESOURCE_LABELS` | plan D5; phase-01..09 |
| F8 | Không có down migration; backfill không có step label; rollback production không rõ | High | Applied — D10; mỗi sketch có down; Phase 9 chạy down/up toàn bộ | plan D10; phase-01..09 |
| F9 | `PUT /classes/:id` full-replace → toggle `recruiting` xoá `code/tags/note`; Security bullet trích `GetWritableByID` không tồn tại | High | Applied — trường mới pointer nil=giữ; gate thật `repo.GetByID`; UI gate `canWriteClass` | phase-01 R2, R9, B3(dto), B4, Security |
| F10 | Tab Buổi học gọi `sessions` không có `from/to` (bắt buộc) và endpoint materialize hàng; NGUỒN so với schedule hiện tại sai với buổi cũ | High | Applied — luôn gửi khoảng của lớp; NGUỒN theo schedule hiệu lực tại ngày buổi | plan D7; phase-01 Key Insights, C20, risks; phase-07 |
| F11 | Phase 8 `prep_items` sao chép cột `template_lessons` rồi cần bước "generate" copy ngược — hai nguồn sự thật | High | Applied — prep = cột trên `template_lessons` của bản nháp; bỏ `prep_items`, bỏ generate; wizard tạo N buổi trống; tái dùng `src/lib/kanban` | plan D3; phase-08; phases order |
| F12 | Quy tắc phase lớp tính ở cả Go, SQL và web → lệch số đếm chip vs bảng | High | Applied — `phase.go` + `phasePredicate` một nguồn, web chỉ nhãn | phase-01 Key Insights, B2, C17 |
| F13 | `class_messages` đọc qua read port `classes` giữ cả stint đã đóng → GV cũ đọc chat | High | Applied — predicate stint `ended_at IS NULL` hoặc owner | phase-07 |
| F14 | Chi tiết Phase 1: scopelint theo helper `readScoped` không theo tên hàm; ILIKE không escape; JSONB không bind; `StatusPill` là widget `collections`; invalidate chỉ `lists()`; MSW `/classes/:id` nuốt `/classes/stats`; thiếu `OVERFLOW_PATH_PREFIXES` | Medium | Applied | plan D6; phase-01 R3, R8, B3, C16, C18, C22–23; phase-02..08 nav |
| F15 | Thiếu `req()` audit ở route mutating; đường dẫn audit `/audit` sai (`/audit-logs`); gold-plating `apply-preview`, badge nav, `GET /courses/:id/stats` | Medium | Applied — D9; bỏ `apply-preview` (409 + confirm), bỏ badge; số lớp embed vào `GET /courses` thay endpoint riêng; giữ "Nhắc lại" với `reminded_at` | plan D9; phase-02, 05, 07 |
| R1 | Scope: "A1 chưa xác minh từ script v5" | Medium | Rejected — không có bằng chứng mã mới; đã là câu hỏi Validation Q1 | — |
| R2 | Scope: gộp cặp key `*.read`/`*.edit` thành một key | Low | Rejected — nav/gating web dùng `has(key)` theo từng key; tách read/edit khớp catalog hiện có (`tasks.*`, `members.*`) | — |

### Whole-Plan Consistency Sweep
- **Files reread**: plan.md, phase-01 … phase-09 (10 file) sau khi áp dụng.
- **Decision deltas checked**: D2 (proposal + owner confirm) ↔ phase-02 routes/web/verification ↔ Success Criteria e2e; D5 (bump một lần) ↔ phase-03..08 "không bump" + phase-08/07 điều kiện + phase-09 task 4; D3/D4 ↔ phase-07 `classprogram`; D8 ↔ mọi migration sketch; D7 ↔ phase-01 C20 và phase-07; Phase 8 deps `[3]` ↔ phases order; Phase 9 deps `[1..7]`.
- **Reconciled stale references**: `class_invites.manage`, `accept-on-behalf`, "authenticated", `GetWritableByID`, `StatusPill`, `/centers/me/members` (không `/directory`), `GET /audit?`, `apply-preview`, `prep_items`, `prep_checklist`, `templates/generate`, `substr(...,1,6)`, `ON DELETE CASCADE`, "CatalogVersion 4→5" ngoài Phase 8 — grep toàn thư mục phải trả về 0 ngoài bảng Red Team Review này.
- **Unresolved contradictions**: none ghi nhận; hai điểm mở chuyển sang Validation (A1 menu membership; Phase 8 in/out).

## Validation Log

### Session 1 — 2026-09-23
**Trigger**: ak-plan `--deep` pipeline, sau Red Team Session 1. **Tier**: Full (red-team đã có bằng chứng `file:line`; phần này chỉ kiểm các mục `[UNVERIFIED]` còn lại).

### Verification Results
| Mục | Kết quả | Bằng chứng |
|---|---|---|
| Khóa của `center_members` để làm FK composite | Verified: PK `(teacher_id, center_id)`; không xoá dòng khi rời | `apps/api/migrations/000007_centers.up.sql:66-72, :63` |
| `classes` có `UNIQUE (id, center_id)` cho FK composite | Verified (từ 000007; 000009/000015 đã tham chiếu) | `000009_teaching.up.sql:29`, `000015_class_staff.up.sql:21` |
| Quy ước cascade của repo | Verified: FK guard dùng `ON DELETE CASCADE`, chỉ chạy khi xoá cứng → D8 tinh chỉnh | `000007_centers.up.sql:60-65`, `000009`, `000015` |
| Bảng/cột audit cho Lịch sử thay đổi | Verified: `audit_logs` (`entity_type`, `entity_id` TEXT, `occurred_at`) | `000010_audit_logs.up.sql:18-26` |
| Giới hạn khoảng `GET /classes/:id/sessions` | Verified: cap 400 ngày, vượt → 422 | `internal/features/sessions/service.go:20-32` |
| A1 menu membership | Không thể xác minh từ mã (markup v5 bị strip) → hỏi user (Q1) | — |

### Questions & Answers
| # | Loại | Câu hỏi | Options | Answer | Rationale |
|---|---|---|---|---|---|
| Q1 | Assumptions | Menu "Giảng dạy" đúng 6 mục A1? | Đúng 6 mục / Thiếu / Thừa | **Đúng 6 mục** | A1 chuyển từ giả định sang xác nhận; 9 phase không đổi |
| Q2 | Scope | Phase 8 Chuẩn bị tài liệu trong scope? | Giữ / Để sau | **Giữ trong scope** | Phase 9 phụ thuộc 8; Phase 8 là nơi bump `CatalogVersion` duy nhất |
| Q3 | Architecture | Lời mời = đề xuất + owner "GV nhận lớp"? | Đề xuất + owner xác nhận / Accept là phân công ngay | **Đề xuất + owner xác nhận** | Không mở bề mặt `class_staff` cho non-owner; D2 giữ nguyên |
| Q4 | Tradeoffs | Key `*.edit`, `library.publish`, `prep.assign` opt-in? | Opt-in / Mặc định mọi vai | **Opt-in** | D5 giữ nguyên; seed cấp sẵn cho GV demo |
| Q5 | Architecture | Ai áp dụng/đổi/gỡ chương trình lớp? | Owner + GV chính / Thêm hoc_vu / Chỉ owner | **Chỉ owner** | Route `PUT`/`DELETE /classes/:id/program` → `KindOwnerOnly`; web gate `isOwner` |
| Q6 | Scope | Chat lớp nội bộ giữ trong Phase 7? | Giữ / Để sau | **Giữ trong Phase 7** | `classchat` + `class_messages.post` giữ nguyên |

### Confirmed Decisions
- A1 xác nhận; D2, D5, D6 giữ nguyên; D4 bổ sung "chỉ owner"; D7 bổ sung cap 400 ngày; D8 tinh chỉnh theo quy ước cascade của repo.

### Action Items
- [x] Phase 7: `PUT`/`DELETE /classes/:id/program` → `KindOwnerOnly`, web gate `isOwner`, verification cập nhật.
- [x] Phase 7: `audit_logs` + `idx_audit_logs_entity`, bỏ placeholder scout.
- [x] Phase 1: tab Buổi học cắt khoảng theo cap 400 ngày. <!-- Thay bằng readonly=true sau review Phase 1 (H1/H2): không materialise, không cap -->
- [x] Phase 2, 7, 8: FK composite đúng khóa `(teacher_id, center_id)`; cascade theo khuôn repo; logic hủy lời mời vẫn ở service.
- [x] Phase 8 xác nhận; Phase 9 `dependencies` thêm 8, thêm `prep.spec.ts`, seed cấp `prep.assign`.
- [x] Marker `<!-- Updated: Validation Session 1 - ... -->` tại mọi chỗ sửa.

### Impact on Phases
| Phase | Tác động |
|---|---|
| 1 | Key Insights + risk tab Buổi học (cap 400 ngày → `readonly=true` sau review Phase 1) |
| 2 | Migration FK/cascade; ghi chú F2 |
| 7 | Gate chương trình chỉ owner; audit_logs cụ thể; cascade |
| 8 | Trạng thái xác nhận; FK assignee đúng khóa |
| 9 | deps `[1..8]`; seed + e2e prep |

### Whole-Plan Consistency Sweep (sau Validation Session 1)
- **Files reread**: plan.md, phase-01, 02, 07, 08, 09 (các file có delta); phase-03..06 không đổi.
- **Decision deltas checked**: Q5 ↔ D4 ↔ phase-07 Key decisions/API/Web/Verification; Q2 ↔ D5 ↔ phase-07 dòng bump ↔ phase-08 header/risks ↔ phase-09 deps; D8 cascade ↔ phase-02/07/08 sketch; D7 cap 400 ↔ phase-01.
- **Reconciled stale references**: "chờ Validation Q2", "nếu Phase 8 deferred", "scout khóa thật", "<bảng audit>", `canWriteClass` trong khối Chương trình học Phase 7 — grep phải trả về 0 ngoài Validation Log này.
- **Unresolved contradictions**: none.
