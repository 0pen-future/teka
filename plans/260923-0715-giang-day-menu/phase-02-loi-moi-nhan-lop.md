---
phase: 2
title: "Lời mời nhận lớp"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: Lời mời nhận lớp

> Outline (deep mode). **Scout lại trước khi thực thi**: `classstaff/service.go` (`Assign` :85),
> `handoff/service.go` (`Reassign` :110-112, `MemberChecker`, `withCenterLock`), `routespec.go:20-50,153-200`,
> `centers/routes.go:10` (`/me/members/directory`), `centers` remove-member, `class-staff-section.tsx`,
> khóa của `center_members` (`left_at`).

## Context Links
- [plan.md](./plan.md) · D2, D5, D8, D9, D10 · Prototype màn `Lời mời nhận lớp` (`inv`), khối `Đội ngũ giảng dạy` trong `cdt`.
- Red team: [reports/redteam-security-260923.md](./reports/redteam-security-260923.md), [reports/redteam-assumptions-260923.md](./reports/redteam-assumptions-260923.md).

## Goal
Chủ trung tâm mời một thành viên nhận vai (giáo viên chính, trợ giảng, học vụ) trong lớp; người được mời
chấp nhận / từ chối trong app; chủ có thể nhắc lại, hủy, hoặc bấm **"GV nhận lớp"** để hoàn tất phân công
(kể cả khi người được mời đồng ý offline).

## Key decisions
- **Bảng riêng `class_invitations`**, không tái dùng `invitations` (đó là onboarding trung tâm, public token).
- **Lời mời là đề xuất, không tự ghi `class_staff`.** <!-- Red Team S1 F1 --> `accept` chỉ đổi `status = accepted`.
  Bước ghi stint là **hành động owner** qua route `confirm`: vai `giao_vien` → `handoff.Service.Reassign`
  (owner-only tại `handoff/service.go:111-112`; giữ `uq_class_staff_one_gv`; khóa trung tâm TryLock → 409 khi bận);
  vai `tro_giang`/`hoc_vu` → `classstaff.Service.Assign` (owner-only, `classstaff/service.go:85`). Caller thật là
  owner nên không cần "mượn" scope owner ở đâu; `classes.Get` own-rows 404 cũng không còn cản trở.
- **Không key quyền mới; không bump `CatalogVersion` ở phase này.** <!-- Red Team S1 F1 --> Gửi / hủy / nhắc /
  confirm là `KindOwnerOnly` — bề mặt `class_staff` không grantable (`routespec.go:32-34`, `docs/adding-permissions.md:42`).
  Xem-của-tôi / accept / decline là `KindService` (`routespec.go:45`, "service's own gates decide, fail closed"):
  service lọc theo `teacher_id = caller`; owner xem toàn trung tâm.
- **Cấm tự mời** (`teacher_id == caller` → 422 `SELF_INVITE`). <!-- Red Team S1 F1 --> Vai `hoc_vu` vẫn mời được vì
  stint chỉ được tạo bởi owner qua `Assign` — tương đương luồng `/classes/:id/staff` hôm nay, không mở thêm
  đường lộ số điện thoại (`classscope.go:74-110`).
- Người được mời phải là thành viên **đang hoạt động** (`center_members.left_at IS NULL`): kiểm khi gửi và khi
  accept/confirm qua `IsActiveMember` (port `MemberChecker` của `handoff`). `center_members` không xoá dòng khi rời
  (000007:63 "KHÔNG BAO GIỜ DELETE row membership") → **không dựa vào FK cascade**. Khi gỡ thành viên, hủy lời mời `pending`/`accepted` của người đó trong cùng tx
  (adapter nhỏ; scout `centers` remove-member). <!-- Red Team S1 F2 -->
- "Nhắc lại" = `reminded_at = now()` + audit; hiển thị "Đã nhắc lúc …" cho cả owner và người được mời
  (không SMS/Zalo — N1). <!-- Red Team S1 F15 -->
- Trạng thái: `pending → accepted | declined | cancelled`; `pending | accepted → assigned` (sau confirm);
  `accepted → cancelled` cũng hợp lệ. Chip "Đã nhận" trên UI = `accepted ∪ assigned`.

## Migration sketch — `000026_class_invitations` <!-- Red Team S1 F5, F8 -->
```sql
CREATE TABLE class_invitations (
  id UUID PRIMARY KEY, center_id UUID NOT NULL, class_id UUID NOT NULL, teacher_id UUID NOT NULL,
  role_key VARCHAR(20) NOT NULL CHECK (role_key IN ('giao_vien','tro_giang','hoc_vu')),
  status VARCHAR(12) NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','accepted','declined','cancelled','assigned')),
  invited_by UUID NOT NULL, message TEXT,
  sent_at TIMESTAMPTZ NOT NULL DEFAULT now(), reminded_at TIMESTAMPTZ, responded_at TIMESTAMPTZ, assigned_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (class_id, center_id)   REFERENCES classes (id, center_id) ON DELETE CASCADE,                -- D8; classes đã có UNIQUE (id, center_id) từ 000007 (000009/000015 tham chiếu)
  FOREIGN KEY (teacher_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE CASCADE  -- PK thật của center_members (000007:72); cascade chỉ chạy khi xoá cứng tài khoản
);
CREATE UNIQUE INDEX uq_class_invitations_pending ON class_invitations (class_id, teacher_id) WHERE status = 'pending';
CREATE INDEX idx_class_invitations_center_status ON class_invitations (center_id, status, sent_at DESC);
CREATE INDEX idx_class_invitations_teacher ON class_invitations (teacher_id, status);
-- down: DROP TABLE class_invitations;
```
Không backfill quyền (không key mới). `ON DELETE CASCADE` theo khuôn 000009/000015 chỉ chạy khi xoá cứng tài khoản/lớp;
soft-delete lớp và soft-leave (`left_at`) **không** kích hoạt nó, nên việc hủy lời mời khi gỡ thành viên vẫn nằm trong
service (F2). <!-- Updated: Validation Session 1 - khóa center_members và quy ước cascade xác minh từ 000007 -->

## API — feature `internal/features/classinvites/`
| Route | Kind | Ghi chú |
|---|---|---|
| `POST /classes/:id/invitations` | `KindOwnerOnly` | body `{teacher_id, role_key, message?}`; 422 `SELF_INVITE`, 422 `MEMBER_INACTIVE`; 409 nếu đang `pending` hoặc đã có stint cùng vai đang mở |
| `GET /class-invitations?status=&class_id=` | `KindService` | owner → toàn trung tâm; member → `teacher_id = caller` (fail closed) |
| `POST /class-invitations/:id/accept` | `KindService` | chỉ người được mời, còn hoạt động; `pending → accepted`; **không ghi class_staff** |
| `POST /class-invitations/:id/decline` | `KindService` | chỉ người được mời; `pending → declined` |
| `POST /class-invitations/:id/cancel` | `KindOwnerOnly` | `pending|accepted → cancelled` |
| `POST /class-invitations/:id/remind` | `KindOwnerOnly` | `reminded_at = now()` |
| `POST /class-invitations/:id/confirm` | `KindOwnerOnly` | nút "GV nhận lớp": `pending|accepted → assigned`; giao_vien → `handoff.Service.Reassign`; vai khác → `classstaff.Service.Assign`; cùng tx với cập nhật lời mời; 409 khi khóa trung tâm bận |
Mọi route mutating khai `req(action, "class_invitation", "id")` trong routespec; cập nhật
`server/route_policy_snapshot_test.go` (+7) và `audit/action_test.go` (+6). <!-- Red Team S1 F7, F15 -->
Lookup lời mời không thuộc mình → 404 (không 403) để không lộ tồn tại. Đường dẫn `/class-invitations`
tránh đụng public `/invitations/*`.

## Web (feature `roster`)
- `api/class-invitations-api.ts`, `hooks/use-class-invitations.ts`, `schemas` bổ sung.
- `pages/class-invitations-page.tsx` (`/class-invitations`): chip Tất cả/Đang chờ/Đã nhận/Đã từ chối/Đã hủy (`HvChip`),
  bảng LỚP/GV/VAI/GỬI LÚC/TRẠNG THÁI; member thấy Chấp nhận/Từ chối trên lời mời của mình; owner thấy
  Nhắc lại/Hủy/GV nhận lớp (gate `isOwner` — đúng quy ước "write actions the API reserves for the owner stay behind `isOwner`").
- `components/class-team-section.tsx` thay placeholder Phase 1: danh sách `class_staff` + lời mời `pending/accepted`
  ("Chờ nhận" / "Đã đồng ý — chờ phân công"), nút `+ Mời GV` (owner) mở `InviteTeacherDialog` chọn thành viên từ
  `GET /centers/me/members/directory` (`centers/routes.go:10`; owner bypass key `members.list`). <!-- Red Team S1 F1 -->
- Nav "Lời mời nhận lớp" `/class-invitations` (không perm — ai cũng thấy lời mời của mình); `OVERFLOW_LABELS` +
  `OVERFLOW_PATH_PREFIXES` (`dashboard-layout.tsx:172,194`). <!-- Red Team S1 F14 --> Không làm badge đếm trên nav. <!-- Red Team S1 F15 -->

## Files (dự kiến)
Create: migration 000026 up/down; `classinvites/{dto,errors,handler,model,repository,routes,service}.go` + tests;
web 1 page, 2 components, 1 api, 1 hooks, tests.
Modify: `routespec.go`, `server/route_policy_snapshot_test.go`, `audit/action_test.go`, `router.go#registerFeatures`
(dựng `classinvites` **sau** `handoff` :203 và `classstaff`; inject `handoff.Service`, `classstaff.Service`, `MemberChecker`),
`centers` remove-member (hủy lời mời), `handlers.ts` (fixtures), `dashboard-layout.tsx`, `roster/routes.tsx`,
`class-info-tab.tsx`. **Không đụng `catalog.go`.**

## Completion notes (2026-09-23)
- Backend: `classinvites` (model, repo, service, handler, routes), migration 000026, 7 route mới trong routespec + snapshot/audit tests, hook `ClassInviteCanceller` trong `centers.RemoveMember`, swagger sinh lại.
- Web: `/class-invitations`, `ClassTeamSection` thay placeholder, `InviteTeacherDialog`, `ConfirmInvitationDialog`, nav + overflow, MSW fixtures, 2 suite vitest mới, e2e `class-invitations.spec.ts`.
- Review: [reports/review-phase-02-260923.md](./reports/review-phase-02-260923.md) — SHIP WITH FIXES; M1/M2/M3/L1/L3/L5/L6 và nit message đã sửa, L2/L4 giữ nguyên có lý do.
- Lệch spec đã chốt: accept của thành viên đã rời trả 404 (kiểm `IsActiveMember` trong `respond`), đúng verification (6).

## Verification
- `make test-api-unit` (manifest, snapshot, audit action), `make test-api` package `classinvites`, `handoff`, `classstaff`, `centers`.
- Integration: (1) member A không thấy / không accept được lời mời của B → 404; (2) member accept → `class_staff` **không đổi**;
  (3) owner confirm giao_vien khi lớp đã có GV khác → GV cũ đóng assignment, `uq_class_staff_one_gv` giữ;
  (4) non-owner gọi send/confirm/cancel/remind → 403; (5) tự mời → 422; (6) thành viên đã `left_at` → gửi 422,
  accept 404, lời mời mở bị `cancelled` khi gỡ thành viên; (7) migration 000026 up/down/up sạch.
- Vitest quartet trang lời mời; e2e `class-invitations.spec.ts` (owner mời member → member accept → owner "GV nhận lớp" →
  Đội ngũ hiển thị).

## Risks
- `Reassign` có side effect (đóng assignment cũ, audit) — đọc `Result` trước khi tái dùng; TryLock bận → 409 hiển thị "Thử lại".
- Confirm giao_vien khi lớp đã có GV chính khác là **bàn giao** — UI phải nêu tên GV bị thay trong `HvConfirmDialog` trước khi gọi.
