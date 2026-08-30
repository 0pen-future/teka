---
phase: 3
title: "Phone privacy + data ownership"
status: completed
priority: P1
effort: "3d"
dependencies: [2]
---

# Phase 3: Phone privacy + data ownership

## Overview

SĐT người liên hệ thành bảo mật trung tâm: mask ở DTO trừ owner, caller
`ReportsOversight()` và học vụ có assignment active trên lớp liên quan.
Contact + student CRUD owner-only (import đi cùng); migration `000016` merge
contact trùng theo center, re-key unique index, rồi chuyển anchor `teacher_id`
của contacts + students về owner. GV ghi danh qua picker student toàn center
(tên, không phone).

## Requirements

- Functional: (a) member GV/trợ giảng/peer không thấy phone Ở BẤT KỲ response
  nào (kể cả statement URL — xem dưới); (b) contact + student
  create/update/delete → 403 cho mọi member; import (owner hoặc member được
  grant `imports.run` — D6) luôn stamp anchor owner; (c) GV có assignment
  active tạo enrollment cho student sẵn có
  qua picker; (d) học vụ assigned + owner + thư ký thấy phone — MỘT luật duy
  nhất cho mọi surface (list tổng lẫn detail).
- Non-functional: gửi báo cáo (statements/notifications/zalo) đọc phone
  server-side — KHÔNG gãy khi caller không thấy phone; migration có đường lùi
  (kể cả merge); có runbook thứ tự deploy/migrate.

## Architecture

**Nguyên tắc mask**: MỘT luật cho mọi surface: caller thấy phone của contact X
⟺ `IsOwner || ReportsOversight() || (có assignment active hoc_vu trên một lớp
có student của X ghi danh active)`. Không phân biệt list tổng vs list theo
class vs detail (red-team F-AD9: hai luật khác nhau cho cùng entity làm React
Query cache 2 shape, phone "nhấp nháy").

Cơ chế: repo trả thêm CỘT DẪN XUẤT `phone_visible bool` tính bằng EXISTS trong
CÙNG query (không N+1; không vi phạm scoping guard — guard chỉ cấm
`IsOwner`/`.Has(` trong repo, cột dẫn xuất nhận args từ service như fragment
P2). Service nulling phone theo cột đó + helper:

```go
// authctx: Scope.PhoneVisible(rowVisible bool) bool =
//   IsOwner || ReportsOversight() || rowVisible
// rowVisible = cột phone_visible do repo tính (hoc_vu-active EXISTS).
```

Mask ở SERVICE, field DTO: phone bị thay bằng `null` (không chuỗi rỗng giả).
CẤM chạy mask trên scope giả — dashboard forge scope (`centers/dashboard.go:82`)
phải truyền `sc` thật của caller cho mọi đường mask (P2 đã ghi quyết định).

**Điểm mask (danh sách đã sửa sau red-team — thêm 3 surface bị sót):**

| Surface | File | Xử lý |
|---|---|---|
| StudentResponse.ContactPhone | students/dto.go:34 (+ repository withContact 99–103) | null trừ PhoneVisible (một luật trên) |
| StatementResponse.Phone | statements/dto.go:17, service.go:205 | null trừ PhoneVisible |
| **StatementResponse.URL** | statements/service.go:204–211, token.go:16–20 | **URL là bearer token công khai (HMAC tất định, không revoke theo role)** — URL statement GIA ĐÌNH chỉ trả cho `ReportsOversight()`/owner. Học vụ làm việc trên statement BẢN THEO LỚP (P4, D1) với token/URL riêng của bản đó — không bao giờ nhận URL bản gia đình. |
| **TargetContact.Phone (Generate response)** | statements/repository.go:33 | cùng luật mask |
| NotificationResponse.Phone (ledger) | notifications/dto.go:103 | null trừ PhoneVisible — ledger là lịch sử gửi, cùng luật |
| BulkSendRow.Phone (preview gửi) | notifications/dto.go:43, service.go:285 | chỉ đường gửi: owner/oversight/học vụ-per-class (P4 mở học vụ send kèm TARGET FILTER — không chỉ gate) |
| ContactBalanceRow.Phone | collections/dto.go:11 | null trừ PhoneVisible (member sau migration không còn own rows — tự hết) |
| ContactResponse.Phone | contacts/dto.go:32–40 | contacts feature giờ owner + oversight only (dưới) |
| Zalo friends match + **zalo-mapping endpoints** | zalo/routes.go:13–21 (`/me/zalo/friends/match` hiện chỉ requireAuth), contacts/routes.go:15–16 (`PUT/DELETE /contacts/:id/zalo-mapping`) | match nhận phone từ CALLER: cho owner/oversight VÀ học vụ (giới hạn kết quả match trong contact lớp gán). Zalo-mapping set/unset: owner/oversight + học vụ cho contact lớp gán. Lý do: P4 bắt học vụ gửi từ zalo CỦA HỌ — khoá hết đường map là tự mâu thuẫn (red-team F5). Confirm ở validation. |

Web đồng bộ: ẩn cột/tap-to-call ở `students-page.tsx`, `student-detail-page.tsx`,
`contacts-page.tsx:104`, `contact-detail-page.tsx:133` theo `phone` null.

**Contact + student CRUD owner-only:**

- contacts: `service.Create/Update/Delete` (service.go:37–50 …) → owner gate
  (403 honest); GET list/detail: owner + `ReportsOversight()`; member thường
  GET → empty list (khỏi gãy UI cũ đột ngột).
- students: `Create/Update/Delete` owner-only ở service; bỏ check
  "contact thuộc caller" (thay bằng contact thuộc center).
- **imports (red-team F3 + validation D6):** gate thật là
  `scope.Has(PermImportsRun)` tại `imports/service.go:207` (handler.go:76 chỉ
  là swagger comment). Quyết định validation: **GIỮ gate này grantable** —
  member được owner grant `imports.run` vẫn chạy import (owner grant = chấp
  nhận rủi ro, ghi docs). Nhưng anchor phải đổi: hiện `anchorFor`
  (apply.go:117–120) dựng scope theo teacher từ workbook rồi gọi
  `contacts/students.Create` → sẽ đâm owner-gate mới. Sửa trong CÙNG PR:
  - anchor: MỌI contact/student import stamp `teacher_id = owner` bất kể ai
    chạy; luồng create nội bộ của import đi đường bypass owner-gate có chủ
    đích (scope owner server-side), KHÔNG nới gate public. Cột "SĐT giáo
    viên" trong workbook chỉ còn dùng để gán GV cho class (P1 create-hook
    cover assignment). `FindIDByPhone`/dedupe resolve chạy scope owner —
    tránh dedupe-miss tạo contact trùng sau migration.
  - test contract: INTEGRATION chạy import THẬT ở cả 2 vai (owner; member
    được grant `imports.run`) → mọi row anchor owner; member KHÔNG grant →
    403; import lại cùng file → không sinh contact trùng.
- Web: ẩn nút tạo/sửa contact + student cho member; empty-state copy "Chủ
  trung tâm quản lý danh bạ & hồ sơ học sinh".

**Migration `000016_owner_data_anchor` — 3 bước, có merge (red-team F1
Critical: `UPDATE ... SET teacher_id = owner` đâm `uq_contacts_phone
(teacher_id, phone)` + `uq_contacts_zalo_user` — 2 GV cùng lưu 1 phụ huynh là
kịch bản ĐƯỢC HỖ TRỢ, có test khóa tại imports/integration_test.go:172):**

```sql
-- Bảng audit: đủ để down cả anchor lẫn merge
CREATE TABLE owner_anchor_backfill (
    table_name  TEXT NOT NULL,
    row_id      UUID NOT NULL,
    old_teacher UUID NOT NULL,
    merged_into UUID,            -- NOT NULL nếu row bị merge vào survivor
    PRIMARY KEY (table_name, row_id)
);

-- Bước 1 — MERGE contact trùng per (center_id, phone):
--   survivor = row created_at sớm nhất (tie-break: nhiều students hơn);
--   repoint students.contact_id, invoices.contact_id, payments.contact_id,
--   statements.contact_id, zalo mapping (giữ mapping của survivor; mapping
--   của loser ghi vào backfill) → soft-delete loser;
--   ghi (loser, old_teacher, merged_into=survivor) vào backfill.
--   Trùng (center_id, zalo_user_id) sau merge: giữ của survivor, NULL của
--   loser (đã soft-delete) — không bao giờ để 2 gia đình dính 1 zalo friend.

-- Bước 2 — RE-KEY unique: DROP uq_contacts_phone / uq_contacts_zalo_user
--   (per-teacher), CREATE lại theo (center_id, phone) / (center_id,
--   zalo_user_id) cùng partial WHERE cũ.

-- Bước 3 — ANCHOR: như cũ, sau merge không còn collision
INSERT INTO owner_anchor_backfill (table_name, row_id, old_teacher)
SELECT 'contacts', c.id, c.teacher_id FROM contacts c
JOIN centers ce ON ce.id = c.center_id WHERE c.teacher_id <> ce.owner_id
UNION ALL
SELECT 'students', s.id, s.teacher_id FROM students s
JOIN centers ce ON ce.id = s.center_id WHERE s.teacher_id <> ce.owner_id;

UPDATE contacts c SET teacher_id = ce.owner_id FROM centers ce
WHERE ce.id = c.center_id AND c.teacher_id <> ce.owner_id;
UPDATE students s SET teacher_id = ce.owner_id FROM centers ce
WHERE ce.id = s.center_id AND s.teacher_id <> ce.owner_id;
```

- FK an toàn: owner luôn có row `center_members` (000007 backfill + register
  tx — verified). Composite FK các bảng con dùng `(x_id, center_id)` → không
  FK con nào gãy (verified 000007:225–265); rủi ro thật nằm ở unique index —
  đã xử lý ở bước 1–2.
- Down: restore anchor từ backfill; un-merge = un-soft-delete loser + restore
  mapping đã ghi (best-effort — repoint con giữ theo survivor, ghi rõ trong
  down comment); re-key index về per-teacher.
- Test: mở rộng harness `migrations_test.go` (pattern `TestCenterTenancy
  Backfill`) với dữ liệu 2 GV cùng phone + 2 GV cùng zalo friend; up → 0
  collision, down → khôi phục.
- **Dry-run trước cook**: query đếm collision `(center_id, phone)` và
  `(center_id, zalo_user_id)` chạy read-only trên prod, ghi số vào delivery
  note — chốt survivor-rule với user nếu số lớn.
- Enrollments/attendance/… giữ creator anchor (R4.2 chỉ contacts + students).
- **Payments/collections hệ quả**: `payments.ResolveContactScope`
  (payments/repository.go:110–119) dẫn xuất anchor từ `contacts.teacher_id` —
  sau migration payment MỚI anchor owner, payment CŨ giữ teacher cũ; view thu
  nợ của member (`collections/repository.go:59,103,255`) rỗng dần — nhất quán
  với "member không còn own contact", ghi vào release note + docs.

**Runbook deploy (red-team F1/FM10 — "cùng PR" ≠ atomic, migrate là lệnh rời
`make migrate-up`):**

1. Deploy code owner-only + mask (member write path chết trước).
2. Xác nhận không còn member write (smoke test 403).
3. Chạy migration 000016.
4. Verify: 0 contact/student anchor ≠ owner; collision = 0.
5. Rollback: revert code TRƯỚC, `migrate down` SAU và chỉ khi
   `owner_anchor_backfill` còn nguyên; không bao giờ để code cũ chạy trên
   schema mới quá bước 2 (member thấy danh bạ rỗng sẽ tạo contact mới bẩn).

**GV ghi danh (picker) — spec siết (red-team F-SA9):**

- `GET /api/v1/classes/:id/enrollable-students?q=` — owner hoặc caller có
  assignment ACTIVE `giao_vien` trên lớp; center-wide theo tên; trả
  `{id, full_name}` — KHÔNG phone, không contact. Ràng buộc: `len(q) >= 2`
  (q rỗng/ngắn → empty), `limit <= 20`, sort theo tên. Đặt trong enrollments
  feature (route theo class context).
- `POST /enrollments` mở cho GV active của lớp (capability `enrollment.write`
  — kéo lên Phase 3 để luồng ghi danh không gãy giữa 2 phase; end/delete vẫn
  creator/owner tới Phase 4). Ghi danh là hành vi tự mở rộng quyền đọc + sinh
  invoice_lines khi chốt kỳ → phát audit event (feature `audit` sẵn có) cho
  enrollment cross-class.
- Web: dialog ghi danh dùng picker (autocomplete tên) thay flow tạo student.

## Related Code Files

- Create: `apps/api/migrations/000016_owner_data_anchor.{up,down}.sql`
- Modify: `apps/api/internal/shared/authctx/{authctx,permissions}.go` (PhoneVisible helper)
- Modify: `apps/api/internal/features/{students,statements,notifications,collections,contacts,zalo}/…` service + dto mask (+ cột phone_visible ở repo); contacts/students service owner gates
- Modify: `apps/api/internal/features/imports/{service,apply,resolve}.go` (owner-only gate + owner anchor + dedupe owner-scope) + `integration_test.go`
- Modify: `apps/api/internal/features/enrollments/…` (picker endpoint + create gate + audit event)
- Modify: `apps/api/migrations/migrations_test.go` (merge/anchor up-down test)
- Modify: seeds `apps/api/seeds/seed.go` (contacts/students seed dưới owner), `internal/testutil/fixtures.go`
- Modify: web roster pages/components/hooks kể trên + e2e `apps/web/e2e/roster.spec.ts` (xác nhận account e2e là owner TRƯỚC khi cook — spec login helper dòng 7–8 dùng 1 tài khoản duy nhất)
- Modify: `docs/api-guidelines.md` (ownership + phone privacy sections)

## Implementation Steps

1. `PhoneVisible` helper + cột dẫn xuất `phone_visible` pattern + unit test.
2. Mask từng surface theo bảng (gồm URL, TargetContact, zalo-mapping),
   integration test 5 vai (owner, GV, học vụ assigned, trợ giảng, thư ký
   oversight) cho students/statements/notifications/collections/contacts.
3. Owner-only gates contacts + students; imports owner-only + owner anchor +
   integration test import thật.
4. Dry-run collision queries (prod, read-only) → ghi số vào delivery note.
5. Migration 000016 (merge + re-key + anchor) + test up/down trên dữ liệu
   seed 2-GV-cùng-phone.
6. Picker endpoint (q>=2, cap 20) + enrollment create gate + audit event +
   web picker flow.
7. Seeds/fixtures/e2e cập nhật; full `make test-api` + web vitest + e2e stack.
8. Deploy theo runbook.

## Success Criteria

- [x] Acceptance 3 + 4 của plan.md pass bằng integration + e2e test.
- [x] "No-phone-for-teacher sweep": test quét JSON response các endpoint chính
      bằng vai GV/trợ giảng, không chứa phone VÀ không chứa statement URL.
- [x] Import: owner + member được grant `imports.run` chạy import thật OK,
      MỌI row anchor owner; member không grant → 403; re-import không sinh
      contact trùng.
- [x] Gửi statement/reminder vẫn chạy khi sender không thấy phone (server-side
      đọc — e2e secretary-send xanh không đổi).
- [x] Migration: collision = 0 sau merge; 0 contact/student anchor ≠ owner;
      down khôi phục anchor + un-merge (gồm dedupe statement trùng kỳ trong
      nhóm gộp nhiều loser — test `TestOwnerDataAnchorStatementDedupeAcrossLosers`).
- [x] GV ghi danh được student sẵn có qua picker; picker không trả phone,
      q rỗng → empty; student đã ghi danh active không xuất hiện lại trong
      picker của chính lớp đó.

## Delivery Status (2026-08-30)

- Steps 1–3, 5–7: DONE — full `make test-api` xanh (coverage 75.7%, sàn 60%),
  web vitest roster xanh, e2e 28/28 trên stack `teka-e2e` cô lập.
- Step 4 (dry-run prod): **CHỜ USER** — 3 query read-only (phone, zalo,
  statement-trùng-kỳ) đã soạn tại
  `plans/reports/dry-run-260830-phone-zalo-collision-queries.md`, chưa chạy
  (auto-mode không được phép truy cập prod DB).
- Step 8 (deploy): runbook ở trên là tài liệu — KHÔNG tự thực thi.
- Code review: `plans/reports/code-review-260830-1446-GH-260830-phase-03-phone-privacy.md`
  — đã áp và verify C1 (dedupe statement khi merge nhiều loser), H2
  (`TargetContacts` mask theo viewer scope), H3 (enrollment create chỉ 1 dòng
  audit — skip route ở middleware theo convention `authSessionRoutes`), H4
  (gofmt/lint/prettier sạch), M8 (picker loại student đã ghi danh active).
  Deferred: M5 (câu hỏi user: member POST /payments 404 vs 403 + release
  note), M6 → Phase 5, M7 → Phase 4, L9–L12 cleanup sau.

## Risk Assessment

- **Merge chọn sai survivor** → dry-run collision + survivor rule chốt với
  user trước; backfill giữ mapping để lùi; merge chỉ soft-delete.
- **UX shock member** → empty-state copy + release note; UI ẩn nút thay vì 403.
- **Zalo mapping của member cũ** — sau merge/anchor, mapping theo survivor;
  học vụ map lại được cho lớp gán (đường mở ở trên).
- **Migration prod** → 3 bước set-based, bảng nghìn rows, lock ngắn; chạy
  trong maintenance window; runbook ở trên là bắt buộc, không dựa "cùng PR".
