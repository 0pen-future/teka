---
title: "Class staff roles + phone privacy"
description: "Bảng class_staff chung cho GV/học vụ/trợ giảng với capability map code-owned; scoping đọc/ghi theo assignment (soft-close giữ quyền đọc lịch sử); SĐT liên hệ chỉ owner + học vụ được gán; contact/student CRUD owner-only + migration anchor về owner"
status: completed
priority: P1
effort: "11d"
tags: [api, web, db, security, authz, migration]
created: 2026-08-30
blockedBy: [260830-2310-resource-action-rbac-permission-catalog]
blocks: []
---

# Class staff roles + phone privacy

Contract source: [brainstorm report](../reports/brainstorm-260830-0825-GH-260830-class-staff-roles-phone-privacy.md)
(4 rounds decisions, complete 2026-08-30). Decision log R1–R4 trong report là
user decisions — không đảo ngược trong plan này.

## Overview

Mỗi lớp có staff theo role (`giao_vien`, `hoc_vu`, `tro_giang`, mở rộng bằng
code) gán qua bảng chung `class_staff` (soft-close `ended_at`). Đọc dữ liệu lớp
đi theo MỌI assignment (kể cả đã đóng — lịch sử); ghi đòi assignment ACTIVE +
role nằm trong capability map code-owned. SĐT người liên hệ thành dữ liệu bảo
mật của trung tâm: chỉ owner, caller `ReportsOversight()` (thư ký gửi báo cáo)
và học vụ được gán vào lớp liên quan thấy. Owner sở hữu toàn bộ dữ liệu gốc:
contact + student CRUD owner-only, migration chuyển anchor `teacher_id` của
contacts và students hiện có về owner của center.

## Cross-plan coordination

- `260830-0714-GH-260830-handoff-roster-visibility` (done): phần widening đọc
  roster GIỮ; riêng việc GV thấy `contact_phone` bị plan này ĐẢO ở Phase 3
  (mask). `docs/api-guidelines.md` đoạn "Class-teacher roster reads" sẽ viết lại.
- `260829-1640-gh-260829-flexible-center-rbac` phase-04 cleanup (pending, soak-
  gated): file phase đó ghi migration `000014_drop_can_send_reports` nhưng
  `000014_grading` đã chiếm số — cleanup đó phải tự renumber (≥ kế tiếp còn
  trống lúc chạy). Plan này lấy `000015_class_staff` (P1),
  `000016_owner_data_anchor` (P3) và `000017_class_scoped_statements_runs`
  (P4). Không block nhau.
- `260829-1020-secretary-report-sender` (done): `reports.send` center-wide GIỮ
  NGUYÊN, cộng gộp với quyền học vụ per-class (quyết định D1 dưới).

## Plan-level decisions

- **D1 — reports.send vs học vụ** (SỬA tại validation 2026-08-30): cộng gộp,
  không thay thế. Thư ký (`ReportsOversight()`) giữ đọc/gửi center-wide theo
  statement GIA ĐÌNH (contact×period) như hiện tại. Học vụ được gán làm việc
  trên **statement BẢN THEO LỚP** (class-scoped variant, P4): chỉ thấy và chỉ
  gửi dòng phí của lớp được gán; phụ huynh nhận bản chỉ gồm phí lớp đó — gia
  đình học nhiều lớp có thể nhận nhiều tin theo lớp. Bản gia đình đầy đủ vẫn
  là đơn vị chuẩn của owner/oversight. (Đảo lựa chọn family-unit của
  brainstorm — quyết định user tại validation interview.)
- **D2 — giao_vien chỉ đổi qua handoff** trong suốt cửa sổ dual-write (P1→P5):
  staff API từ chối gán/gỡ `giao_vien` (409 trỏ sang handoff). DB enforce
  1 GV active/lớp bằng partial unique index — giữ bất biến
  `classes.teacher_id` = assignment `giao_vien` active duy nhất.
- **D3 — `classes.teacher_id` sống sót**: sau P5 cột vẫn là con trỏ GV chính
  (denormalized, handoff sync); chỉ các đường SCOPING theo nó bị gỡ.
- **D4 — vocabulary role_key**: cùng chuỗi với `center_roles.key` nhưng là trục
  độc lập (role trung tâm ≠ vai trong lớp); validate bằng code trong
  `authctx`, không FK sang `center_roles`, không CHECK cứng. **Luật giao thoa
  với RBAC** (red-team): `class_staff` chỉ CẤP quyền trên lớp và luôn giao
  (AND) với RBAC — một `deny` trong `center_member_permissions` thắng
  assignment (test: member bị deny → gán hoc_vu → vẫn không thấy phone).
  Ngoại lệ hiện có giữ nguyên: member được grant `data.view_center_wide` đi
  đường `CenterWide()` bỏ qua class_staff — ghi vào docs, owner tự chịu khi
  grant key đó.
- **D5 — migration anchor có đường lùi**: `000016` lưu mapping cũ (anchor +
  merge) vào bảng audit `owner_anchor_backfill` để down khôi phục được
  (creator gốc không mất).
- **D6 — import: gate GIỮ `imports.run` grantable, anchor owner cứng** (SỬA
  tại validation 2026-08-30 — nới R4.3 phần gate): gate run giữ nguyên
  `scope.Has(PermImportsRun)` (owner grant cho member = chấp nhận rủi ro,
  ghi docs); nhưng MỌI contact/student import stamp `teacher_id = owner`
  bất kể ai chạy, dedupe/`FindIDByPhone` resolve theo scope owner; cột SĐT
  giáo viên trong workbook chỉ để gán GV cho class.
- **D7 — merge contact trùng trong 000016**: unique phone/zalo chuyển từ
  per-teacher sang per-center `(center_id, phone)` / `(center_id,
  zalo_user_id)`; contact trùng được merge (survivor = created_at sớm nhất,
  repoint children, soft-delete loser, audit đủ để lùi). Dry-run đếm collision
  trên prod trước khi cook P3.
- **D8 — quyền theo assignment thay thế session-teacher**: sau P4, GV MỚI sửa
  được artifacts các buổi TRƯỚC handoff; GV cũ mất write kể cả trên buổi mình
  từng dạy (chỉ còn đọc). Gate cũ bị thay thế, không OR thêm.
- **D9 — notification run per-class** (thêm tại validation 2026-08-30, đảo
  khuyến nghị giữ-409 của red-team): `notification_runs` thêm chiều
  `class_id` nullable — run học vụ scope theo lớp, chạy song song không đâm
  nhau; run center-wide (owner/oversight) giữ `class_id IS NULL` và vẫn
  1-active-per-period với nhau. Migration `000017` trong P4.

## Phases

| # | Phase | Status | Depends |
|---|-------|--------|---------|
| 1 | [Schema `class_staff` + quản lý staff + handoff dual-write](./phase-01-class-staff-schema-and-management.md) | Done | — |
| 2 | [Read scoping theo assignment](./phase-02-read-scoping-by-assignment.md) | Done | 1 |
| 3 | [Phone privacy + data ownership](./phase-03-phone-privacy-and-data-ownership.md) | Done (trừ dry-run prod bước 4 — chờ user) | 2 |
| 4 | [Writes theo capability map](./phase-04-capability-map-writes.md) | Done | 2, 3 |
| 5 | [Cleanup scoping cũ](./phase-05-cleanup-legacy-scoping.md) | Done | 1–4 deployed + soak |

Mỗi phase shippable riêng, theo thứ tự — P4 phụ thuộc P3 thật sự (mask phone/
URL, zalo mapping cho học vụ, owner anchor cho target filter), không đảo.

**2026-08-31 — Phase 5 implemented dưới resource-action-rbac phase 8**
(commit fa8cfc8, branch `teka/260831-0016`): nhánh `classes.teacher_id`
readScoped chết đã gỡ, `GetReadable`/`GetReadableWithRoles` tách,
`docs/api-guidelines.md` chốt model. Parity `classes.teacher_id` ↔ active
`giao_vien` = 0 (prod inventory). Suites xanh.

**2026-08-31 — Deployed ~11:46, phase 5 và plan hoàn tất.** Binary
provenance = HEAD 602a4cc; `/readyz` 200; 0 log error/fatal/panic; denial
baseline 403s=0/24h — không regression.

## Acceptance criteria (từ contract, observable)

1. Owner gán/gỡ staff per class qua API + UI; `role_key` ngoài danh mục code →
   422; lớp thiếu role chỉ cảnh báo mềm trên UI.
2. Handoff = đóng assignment `giao_vien` cũ + mở mới (owner-only, cùng center,
   1 tx, dual-write `classes.teacher_id`); GV mới ghi được điểm danh/điểm/nhận
   xét/giáo án/ghi danh; GV cũ chỉ còn ĐỌC lịch sử lớp (mọi write → 403/404).
3. GV & trợ giảng: mọi response + UI roster/attendance KHÔNG chứa
   `contact_phone`; học vụ được gán + owner thấy đủ. Integration test per role.
4. Contact + student create/update/delete: member → 403; owner → OK. Import:
   owner hoặc member được grant `imports.run` chạy được; MỌI row import anchor
   owner (D6). Sau migration: mọi contact + student anchor thuộc owner.
5. Học vụ: đọc điểm danh/điểm/nhận xét lớp được gán (mọi write → 403); đọc +
   gửi statement BẢN THEO LỚP cho contact lớp đó (D1) — phụ huynh nhận bản chỉ
   gồm phí lớp gán; lớp không gán → 404/empty.
6. Trợ giảng: đọc roster + read/write điểm danh lớp gán (không duyệt); lớp
   khác → 404/empty.
7. Peer cùng center không gán: không thấy gì (404/empty, không phải 403 leak).

## Success criteria

- [x] 7 acceptance criteria trên có integration/e2e test tương ứng, xanh trên
      isolated e2e stack (`teka-e2e`) — full Playwright 31 passed 2026-08-30.
- [x] `make test-api` + web vitest xanh; coverage floor giữ (60%) — total
      coverage 75.7%, vitest 449 passed (2026-08-30).
- [x] `scoping_guard_test.go` mở rộng cover helper assignment mới; không repo
      nào branch trên `IsOwner`/`Has` trực tiếp.
- [x] `docs/api-guidelines.md` cập nhật: mục scoping theo `class_staff`, phone
      privacy, ownership model mới + mục "Class-staff writes (capability map)".

## Risks (tổng hợp — chi tiết per phase)

- Blast radius ~8 feature (classes, sessions, attendance, enrollments,
  students, grading, teaching, statements/notifications) + imports + seeds +
  e2e → phasing bắt buộc, mỗi phase một PR.
- Dual-write invariant (D2) gãy → GV mới mất quyền hoặc 2 GV active. Index DB
  + test tx handoff + create-hook P1 + no-op-repair chặn; P5 có đường
  reconcile, không có trạng thái chết.
- Migration 000016: merge contact trùng + re-key unique + anchor — deploy
  theo RUNBOOK P3 (code trước, migrate sau, rollback định nghĩa rõ), không
  dựa "cùng PR" làm atomicity.
- UX regression: member mất đường tạo contact/student — cần copy/hướng dẫn UI
  rõ ràng trong P3.

## Red Team Review

Session 2026-08-30 — 3 hostile reviewers (Security Adversary + Fact Checker,
Failure Mode Analyst + Flow Tracer, Assumption Destroyer + Scope Auditor),
Full tier. 30 finding thô → dedupe còn 15 (tất cả qua evidence filter
file:line) → **user accept cả 15** → đã áp vào phase files.

| # | Sev | Finding | Áp vào |
|---|-----|---------|--------|
| 1 | Crit | 000016 abort vì `uq_contacts_phone`/`uq_contacts_zalo_user` per-teacher; thiếu runbook deploy/rollback | P3 (merge + re-key + runbook), D7 |
| 2 | Crit | Tạo lớp mới không sinh assignment; backfill snapshot 1 lần; handoff no-op ngoài tx; drift → 500 vĩnh viễn, không đường chữa | P1 (create-hook, self-heal), P5 (reconcile) |
| 3 | Crit | Owner-gate giết import (anchor `IsOwner=false`); `imports.run` grantable — claim "đã owner-only" sai (handler.go:76 là swagger comment, gate thật service.go:207) | P3, D6 |
| 4 | Crit | Attendance: `TallyByEnrollment` own-rows → invoice thiếu buổi trợ giảng ghi; `UpsertMany` không scope; `SoftDeleteMissing` own-rows → row billable mồ côi; P4↔P5 mâu thuẫn own-rows | P4 (bảng method), P5 (reword) |
| 5 | Crit | Học vụ send: `TargetContacts` không có chiều class → leak phone+URL toàn center; `ZaloMappings` không widen → gửi fail im lặng; run-per-period 409; P3 khoá zalo-match mà P4 đòi gửi từ zalo học vụ | P4 (target filter, widen, 409), P3 (zalo cho học vụ) |
| 6 | Crit | `classes.Get` là write-auth port dùng chung — nới in-place ở P2 là mở write teaching/sessions/grading trước capability map | P2 (port split + write-freeze test) |
| 7 | Crit | `class_staff` không đóng theo membership stint — kick → mời lại hồi sinh full write lớp cũ | P1 (Close/OpenMembership hook) |
| 8 | Crit | FK `class_staff→center_members` thiếu `ON DELETE CASCADE` — phá tx xoá cứng PII; test hiện tại vẫn xanh vì không seed bảng mới | P1 (FK + test seed) |
| 9 | High | Fragment `ReadExists(alias)` không khớp 4/7 surface; đánh rơi `classes.deleted_at IS NULL`; backfill gồm lớp đã xoá | P2 (chữ ký expr + join classes), P1 (backfill filter) |
| 10 | High | Gate grading/teaching là session-teacher (không phải class-teacher); handoff không dời session quá khứ → GV cũ sửa điểm lịch sử; phải thay thế không OR; grep P5 quá hẹp | P4 (replace + test), P5 (grep 3 pattern), D8 |
| 11 | High | `StatementResponse.URL` = bearer token HMAC tất định, không mask — ended assignment lưu URL là đọc statement vĩnh viễn | P3 (URL oversight-only, học vụ không nhận URL) |
| 12 | High | Không có đường thu hồi assignment gán nhầm (soft-close + đọc-mọi-assignment = không thu hồi gì) | P1 (`?mode=void` hard-delete + audit) |
| 13 | High | Dashboard forge scope (`dashboard.go:82`) — P2 làm KPI đếm trùng; P3 làm PhoneVisible luôn false đường dashboard | P2 (giữ own-rows port; mask nhận sc thật) |
| 14 | High | `my_staff_roles` không vừa `FromModel` (mapper thuần dùng chung dashboard) — N+1 hoặc vỡ parse web | P2 (WithRoles + batch + `.default([])`) |
| 15 | Med | P1 thiếu contract 403/404 — member 403 mâu thuẫn acceptance 7 | P1 (kéo semantics từ P4 lên) |

Gộp thêm 3 điểm nhỏ được accept cùng: picker `len(q)>=2` + cap 20 + audit
event (P3); mask bằng cột dẫn xuất `phone_visible` trong SQL — một luật mọi
surface (P3); D4 luật deny-thắng + ghi nhận `data.view_center_wide` bypass.

### Consistency sweep (sau khi áp finding)

Đọc lại cả 6 file, đối chiếu chéo. 3 lệch tìm thấy, đã sửa hết — 0 mâu thuẫn
còn lại:

1. plan.md bảng Phases còn ghi chú dependency P4 kiểu cũ + câu "P3/P4 có thể
   đảo thứ tự" — sai sau red-team (P4 cần mask URL, zalo mapping học vụ, owner
   anchor của P3) → sửa thành thứ tự cứng.
2. P1 capability map ghi "dùng từ Phase 4" — P3 đã dùng `enrollment.write`
   cho enrollment create → sửa comment.
3. P1 không nhắc `sessions.write` mà P4 bổ sung vào map → thêm ghi chú để
   vocabulary một chỗ không gây bất ngờ lúc cook.

Đã đối chiếu khớp: tổng effort 9.5d = Σ phase (2+1.5+3+2.5+0.5); P2 port-split
↔ P4 bảng chuyển đổi (grading = session-teacher cả hai nơi); P2 write-freeze ↔
P3 mở enrollment create (ngoài phạm vi freeze, P3 phụ thuộc P2); P3 zalo cho
học vụ ↔ P4 gửi từ zalo học vụ; P4 bảng attendance ↔ P5 grep contract +
reword own-rows; P1 backfill idempotent ↔ P5 lệnh reconcile; D1–D8 không bị
phase nào mâu thuẫn. Điểm chờ validation: zalo match/mapping cho học vụ (P3),
D1 statement family-unit, D7 survivor rule, D8, 409 run-per-period, D6 import
owner-only cứng.

## Validation Interview (2026-08-30)

7 câu, trả lời của user — 3 quyết định ĐẢO so với bản sau red-team, đã áp
ngược vào D1/D6/D9 + phase files:

| # | Câu hỏi | Trả lời | Áp vào |
|---|---------|---------|--------|
| 1 | Mở zalo match/mapping cho học vụ (giới hạn lớp gán)? | **Mở giới hạn lớp gán** (theo đề xuất) | P3 giữ nguyên |
| 2 | D8 — GV mới sửa được buổi trước handoff, GV cũ mất write lịch sử? | **Giữ D8** (theo đề xuất) | P4 giữ nguyên |
| 3 | D6 — import owner-only cứng theo IsOwner? | **ĐẢO: giữ `imports.run` grantable**, anchor owner cứng | D6, P3 |
| 4 | D7 — survivor merge = created_at sớm nhất? | **created_at sớm nhất** (theo đề xuất) | P3 giữ nguyên |
| 5 | Run gửi: giữ 409 per-period hay per-class run? | **ĐẢO: per-class run ngay** | D9, P4 (000017) |
| 6 | D1 — học vụ thấy nguyên statement gia đình? | **ĐẢO: chỉ dòng phí lớp gán** | D1, P4 |
| 7 | (Follow-up D1) học vụ gửi → phụ huynh nhận bản nào? | **Bản chỉ lớp gán** (statement variant per-class) | D1, P4 (000017) |

Hệ quả effort: P4 2.5d → 4d (statement class-variant + per-class run);
tổng 9.5d → 11d.
