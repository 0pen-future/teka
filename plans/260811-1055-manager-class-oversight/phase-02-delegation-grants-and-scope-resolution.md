---
phase: 2
title: "Delegation Grants and Scope Resolution"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Delegation Grants and Scope Resolution

## Overview

Feature package `oversight` sở hữu bảng `management_grants`: API tạo/xem/thu hồi grant, và hàm scope-resolution mà Phase 4 dùng để verify mọi truy cập cross-teacher. Không đụng `authctx` — nó vẫn là nguồn duy nhất của identity; oversight thêm tầng "identity → managed set" có DB.

## Requirements

- Functional: GV cấp quyền cho manager theo SĐT; hai phía xem và thu hồi grant.
- Non-functional: bất biến authz — target teacher_id không bao giờ được tin từ request; luôn verify qua grant còn hiệu lực, mỗi request, từ DB.

## Architecture

- `internal/features/oversight/`: `model.go` (Grant), `repository.go`, `service.go`, `handler.go`, `routes.go`, `dto.go`, `integration_test.go` — đúng bố cục feature hiện có.
- **Consent model**: grant do **GV bị quản lý** tạo (caller = grantor = managed side), trỏ tới manager qua `manager_phone` (phone là định danh hệ thống, khớp `user_accounts.phone`). Manager không thể tự cấp cho mình. Grant hiệu lực ngay khi tạo (1 bước — quyết định user 260811, đã cân nhắc và từ chối mô hình 2 bước `accepted_at`); bù lại response tạo grant **không trả thông tin gì về manager ngoài phone caller tự nhập** (chống oracle dò danh bạ), và cả hai phía nhận notification.
- **Phone lookup**: KHÔNG gọi `teachers.Service.GetByPhone` trực tiếp — hàm này có hợp đồng ghi rõ *"must not back an endpoint directly"* (`apps/api/internal/features/teachers/service.go:73-76`). Oversight định nghĩa consumer interface riêng `TeacherLookup` (implement bởi teachers, inject qua router), hợp đồng: chỉ resolve tài khoản `role = teachers`, `status = 'active'`, chưa soft-delete.
- **Notification**: dùng feature `notifications` hiện có (đã wire tại `apps/api/internal/server/router.go:157`) — gửi cho cả manager lẫn managed khi grant được tạo và khi bị thu hồi, để chủ tài khoản phát hiện grant lạ (chống backdoor sau chiếm phiên / gõ nhầm số).
- **Scope resolution** (dùng nội bộ + Phase 4):

```go
// VerifyManages trả về nil khi caller có grant sống trên target; ngược lại
// trả apperror 403. Gọi MỖI request cross-teacher — không cache vào JWT/session
// để thu hồi có hiệu lực ngay.
func (s *Service) VerifyManages(ctx context.Context, managerID, targetID uuid.UUID) error

// ManagedTeacherIDs trả về tập id server-side cho các query roll-up IN (...).
// KHÔNG BAO GIỜ nhận tập id từ client.
func (s *Service) ManagedTeacherIDs(ctx context.Context, managerID uuid.UUID) ([]uuid.UUID, error)
```

**Điều kiện "grant sống" (bắt buộc, cả hai hàm):** không chỉ
`revoked_at IS NULL` — phải JOIN `user_accounts` cho **cả hai phía** và yêu cầu
`status = 'active' AND deleted_at IS NULL`. Lý do: FK `ON DELETE CASCADE` chỉ
chạy khi hard-delete, app dùng soft-delete; và tài khoản `disabled` được bật
lại không được tự động hồi sinh oversight (đối chiếu: login/refresh đã kiểm
`Status != StatusActive` tại `apps/api/internal/features/auth/service.go:90,151`
— oversight phải cùng chuẩn).

## API Design

Tất cả dưới `/api/v1`, `requireAuth`, caller lấy từ `authctx.TeacherID`.

| Method & Path | Request body | Response |
|---|---|---|
| `POST /oversight/grants` | `{"manager_phone": "0901234567"}` (binding `required,vnphone`, chuẩn hoá qua `NormalizePhone` — validator `e164` KHÔNG tồn tại trong repo, chỉ có `vnphone` tại `apps/api/internal/shared/validation/validation.go:33`) | `201 {"id": "...", "manager_phone": "0901234567", "created_at": "..."}` — **không trả `full_name`/id của manager** (chống oracle dò tên theo SĐT); phone là giá trị caller tự nhập echo lại |
| `GET /oversight/grants` | — | `{"given": [{"id","manager":{"id","full_name","phone"},"created_at"}], "received": [{"id","managed":{...},"created_at"}]}` — `given`: grant tôi cấp (tôi là managed); `received`: GV tôi được quản. Tên chỉ hiện ở đây — sau khi quan hệ đã tồn tại |
| `DELETE /oversight/grants/:id` | — | `204` — hợp lệ khi caller là manager **hoặc** managed của grant đó |

Errors: 404 phone không tồn tại/không phải teacher/tài khoản không active (một message chung, không phân biệt); 409 grant sống đã tồn tại cho cặp này; 422 tự cấp cho chính mình.

**Revoke = một UPDATE duy nhất, scope trong WHERE** (không fetch-then-check —
tránh đọc row cross-tenant và TOCTOU ghi đè `revoked_at`):

```sql
UPDATE management_grants SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL
  AND (manager_teacher_id = $caller OR managed_teacher_id = $caller)
```

`RowsAffected == 0` → 404 thống nhất cho mọi trường hợp (không tồn tại / không
thuộc caller / đã revoke) — pattern `RowsAffected` như
`apps/api/internal/features/classes/repository.go:118-124`.

## Related Code Files

- Create: `apps/api/internal/features/oversight/{model,repository,service,handler,routes,dto}.go`
- Create: `apps/api/internal/features/oversight/integration_test.go`
- Modify: `apps/api/internal/server/router.go` (đăng ký `oversight.RegisterRoutes`)

## Implementation Steps

1. Model + repository (GORM; điều kiện grant sống = `revoked_at IS NULL` + JOIN `user_accounts` hai phía active/chưa xoá).
2. Consumer interface `TeacherLookup` trong oversight; implement phía teachers (lookup lọc `status='active'`), inject qua router (precedent setter injection `apps/api/internal/server/router.go:134-140`).
3. Service: create (resolve phone qua `TeacherLookup`, chặn self, chặn trùng, từ chối target không active), list hai chiều, revoke (single UPDATE scope-trong-WHERE ở trên), `VerifyManages`, `ManagedTeacherIDs`.
4. Notification: bắn qua feature `notifications` cho cả hai phía khi create/revoke (best-effort, không chặn response).
5. Handler + routes + DTO validation (`binding:"required,vnphone"` + `NormalizePhone` — theo pattern `apps/api/internal/features/auth/dto.go:11`).
6. Đăng ký router; regenerate swagger.
7. Integration tests: happy path 3 endpoint; deny: tự cấp, trùng grant, revoke grant người khác → 404, revoke idempotent lần 2 → 404, `VerifyManages` sai target → 403, revoke rồi gọi lại → 403, target `disabled`/soft-deleted → tạo grant fail 404 và `VerifyManages` trên grant cũ → 403.

## Success Criteria

- [ ] GV A cấp grant cho B qua phone; B thấy A trong `received`; A thu hồi → B mất quyền ngay request kế
- [ ] Manager không thể tự tạo grant cho mình (mọi hướng)
- [ ] `VerifyManages`/`ManagedTeacherIDs` chỉ trả dữ liệu từ grant sống
- [ ] Swagger cập nhật; tests pass

## Risk Assessment

- **Phone lookup lộ danh tính** (dò SĐT có tài khoản): 404 chung chung, response 201 không chứa tên. Nhánh 404/409/201 vẫn phân biệt được "số này là teacher hay không" — rủi ro enumeration còn lại đã được user chấp nhận ở scope V1 (quyết định 260811, chọn 1 bước thay vì 2 bước accepted_at). **Backlog ngoài plan này:** rate-limit middleware per-caller cho endpoint này (repo hiện chưa có limiter nào — grep 0 kết quả trong `apps/api/internal/`); là middleware đầu tiên loại này nên làm task riêng.
- **Grant nhầm số / phiên bị chiếm**: mitigation = notification hai phía khi create/revoke; chủ tài khoản thấy grant lạ và revoke được ngay.
- **Race 2 grant cùng cặp**: unique partial index là chốt chặn cuối; service map lỗi unique → 409.
