---
phase: 3
title: "Phone privacy + data ownership"
status: pending
priority: P1
effort: "2d"
dependencies: [2]
---

# Phase 3: Phone privacy + data ownership

## Overview

SĐT người liên hệ thành bảo mật trung tâm: mask ở DTO trừ owner, caller
`ReportsOversight()` và học vụ có assignment active trên lớp liên quan.
Contact + student CRUD owner-only; migration `000016` chuyển anchor
`teacher_id` của contacts + students về owner. GV ghi danh qua picker student
toàn center (tên, không phone).

## Requirements

- Functional: (a) member GV/trợ giảng/peer không thấy phone ở bất kỳ response
  nào; (b) contact + student create/update/delete → 403 cho mọi member, kể cả
  import path; (c) GV có assignment active tạo enrollment cho student sẵn có
  qua picker; (d) học vụ assigned + owner + thư ký thấy phone.
- Non-functional: gửi báo cáo (statements/notifications/zalo) đọc phone
  server-side — KHÔNG gãy khi caller không thấy phone; migration có đường lùi.

## Architecture

**Nguyên tắc mask**: quyết định ở SERVICE (không phải repo — giữ scoping guard
discipline), field DTO thành con trỏ/omit: phone bị thay bằng `null` (không
trả chuỗi rỗng giả — FE phân biệt "không có quyền" vs "chưa có SĐT" qua
`phone_visible bool` cạnh field khi cần; mặc định đơn giản: null = không
thấy).

Helper service-level dùng chung — `authctx`:

```go
// Scope.PhoneVisible(hocVuActiveOnRelevantClass bool) bool =
//   IsOwner || ReportsOversight() || hocVuActiveOnRelevantClass
```

Cờ `hocVuActive` do từng service tính từ assignment active (`role_key=hoc_vu`)
trên lớp ngữ cảnh (class của roster/attendance đang xem; với statement:
contact có enrollment active trong lớp học vụ gán — quyết định D1 plan.md).

**Điểm mask (từ research — đủ danh sách):**

| Surface | File | Xử lý |
|---|---|---|
| StudentResponse.ContactPhone | students/dto.go:34 (+ repository withContact 99–103) | null trừ PhoneVisible; list theo class → hocVuActive theo class đó; list tổng → chỉ owner/oversight |
| StatementResponse.Phone | statements/dto.go:17, service.go:205 | null trừ PhoneVisible (học vụ: theo D1) |
| NotificationResponse.Phone (ledger) | notifications/dto.go:103 | null trừ PhoneVisible — ledger là lịch sử gửi, cùng luật |
| BulkSendRow.Phone (preview gửi) | notifications/dto.go:43, service.go:285 | chỉ đường gửi: owner/oversight/học vụ-per-class (Phase 4 mở học vụ send) |
| ContactBalanceRow.Phone | collections/dto.go:11 | null trừ PhoneVisible (member thường vốn chỉ thấy own rows — sau migration anchor họ không còn row nào, tự hết) |
| ContactResponse.Phone | contacts/dto.go:32–40 | contacts feature giờ owner + oversight only (dưới) |
| Zalo friends match (nhận phone list từ client) | zalo feature | audit: endpoint match nhận phone từ CALLER — chỉ cho owner/oversight gọi (học vụ không cần map zalo) |

Web đồng bộ: ẩn cột/tap-to-call ở `students-page.tsx`, `student-detail-page.tsx`,
`contacts-page.tsx:104`, `contact-detail-page.tsx:133` theo `phone` null +
effective perms.

**Contact + student CRUD owner-only:**

- contacts: `service.Create/Update/Delete` (service.go:37–50 …) → owner gate
  (403 `FORBIDDEN` — honest, caller thấy feature nhưng thiếu quyền); GET
  list/detail: owner + `ReportsOversight()` (giữ cluster thư ký, docs
  api-guidelines 129–135); member thường mất luôn GET (họ không còn own rows
  sau migration — trả empty list thay vì 403 để khỏi gãy UI cũ đột ngột).
- students: `Create/Update/Delete` owner-only ở service; bỏ check
  "contact thuộc caller" (vô nghĩa khi owner-only — thay bằng contact thuộc
  center).
- imports: đã owner-only (`imports/handler.go:76`, `PermImportsRun`) — thêm
  integration test khóa contract "import contact/student/class là owner-only"
  (R4.3). Import điểm danh/điểm nếu có sau này theo capability map (ghi chú,
  không code).
- Web: ẩn nút tạo/sửa contact + student cho member (`contact-dialog.tsx`,
  use-contacts hooks, students pages); empty-state copy giải thích "Chủ trung
  tâm quản lý danh bạ & hồ sơ học sinh".

**Migration `000016_owner_data_anchor`:**

```sql
CREATE TABLE owner_anchor_backfill (
    table_name  TEXT NOT NULL,
    row_id      UUID NOT NULL,
    old_teacher UUID NOT NULL,
    PRIMARY KEY (table_name, row_id)
);
INSERT INTO owner_anchor_backfill
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
  tx — verified). Composite FK các bảng con dùng `(x_id, center_id)`, không
  `(x_id, teacher_id)` → không FK con nào gãy.
- Down: restore từ `owner_anchor_backfill` rồi drop bảng đó.
- Enrollments/attendance/… giữ creator anchor (R4.2 chỉ contacts + students).
- Ship CÙNG PR với code owner-only (member write path chết trước khi anchor
  đổi, không có cửa sổ member-ghi-lên-row-owner).

**GV ghi danh (picker):**

- `GET /api/v1/classes/:id/enrollable-students?q=` — owner hoặc caller có
  assignment ACTIVE `giao_vien` trên lớp; center-wide theo tên; trả
  `{id, full_name}` — KHÔNG phone, không contact. Đặt trong enrollments
  feature (route theo class context).
- `POST /enrollments` mở cho GV active của lớp (capability `enrollment.write`
  — kéo phần này của capability map lên Phase 3 để luồng ghi danh không gãy
  giữa 2 phase; enrollment end/delete vẫn creator/owner tới Phase 4).
- Web: dialog ghi danh dùng picker (autocomplete tên) thay flow tạo student.

## Related Code Files

- Create: `apps/api/migrations/000016_owner_data_anchor.{up,down}.sql`
- Modify: `apps/api/internal/shared/authctx/{authctx,permissions}.go` (PhoneVisible helper)
- Modify: `apps/api/internal/features/{students,statements,notifications,collections,contacts,zalo}/…` service + dto mask; contacts/students service owner gates
- Modify: `apps/api/internal/features/enrollments/…` (picker endpoint + create gate)
- Modify: `apps/api/internal/features/imports/integration_test.go` (khóa owner-only contract)
- Modify: seeds `apps/api/seeds/seed.go` (contacts/students seed dưới owner), `internal/testutil/fixtures.go`
- Modify: web roster pages/components/hooks kể trên + e2e `apps/web/e2e/roster.spec.ts` (chạy vai owner cho CRUD; case member bị ẩn)
- Modify: `docs/api-guidelines.md` (ownership + phone privacy sections)

## Implementation Steps

1. `PhoneVisible` helper + unit test.
2. Mask từng surface theo bảng, integration test 5 vai (owner, GV, học vụ
   assigned, trợ giảng, thư ký oversight) cho students/statements/
   notifications/collections/contacts.
3. Owner-only gates contacts + students (+ import contract test).
4. Migration 000016 (+ test up/down trên dữ liệu seed member-owned).
5. Picker endpoint + enrollment create gate + web picker flow.
6. Seeds/fixtures/e2e cập nhật; full `make test-api` + web vitest + e2e stack.

## Success Criteria

- [ ] Acceptance 3 + 4 của plan.md pass bằng integration + e2e test.
- [ ] Gửi statement/reminder vẫn chạy khi sender không thấy phone (server-side đọc — e2e secretary-send xanh không đổi).
- [ ] Migration: 0 contact/student nào còn anchor ≠ owner; down khôi phục đúng.
- [ ] GV ghi danh được student sẵn có qua picker; picker không trả phone.

## Risk Assessment

- **Miss một surface phone** → grep guard test: integration test quét JSON
  response các endpoint chính bằng vai GV, assert không chứa field phone của
  contact (test "no-phone-for-teacher sweep").
- **UX shock member** → empty-state copy + release note; UI ẩn nút thay vì 403.
- **Zalo map của member** (member từng map zalo với contact mình tạo) — sau
  migration contact về owner, mapping giữ row nhưng member không còn thấy
  contact → xác nhận không có background job nào của member đọc contact list
  (zalo send là oversight-only rồi).
- **Migration chạy trên prod dữ liệu lớn** → 2 UPDATE set-based, bảng nhỏ
  (nghìn rows), lock ngắn — chạy trong maintenance window bình thường.
