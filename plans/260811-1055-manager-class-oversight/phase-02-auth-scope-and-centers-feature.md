---
phase: 2
title: "Auth Scope and Centers Feature"
status: pending
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: Auth Scope and Centers Feature

## Overview

Hai việc: (1) mở rộng `authctx` + middleware để mỗi request có `Scope{TeacherID, CenterID, IsOwner}` resolve từ DB; (2) feature package `centers` — xem/đổi tên center, join theo SĐT owner, rời/remove member, và auto-tạo center cá nhân khi đăng ký teacher mới.

## Requirements

- Functional: teacher join center của owner khác (consent — teacher khởi tạo); owner/chính teacher remove membership; member xem danh sách thành viên.
- Non-functional: membership đọc từ DB **mỗi request** — remove hiệu lực ngay request kế; không đưa `center_id`/`is_owner` vào JWT claims.

## Architecture

### authctx + middleware

- `authctx` thêm:

```go
// Scope là ngữ cảnh tenant đầy đủ của caller, resolve từ DB mỗi request —
// KHÔNG cache vào JWT để remove/kick membership có hiệu lực ngay.
type Scope struct {
    TeacherID uuid.UUID
    CenterID  uuid.UUID
    IsOwner   bool
}
func ScopeFrom(c *gin.Context) (Scope, bool)
```

- Middleware mới `middleware.ResolveScope(resolver)` gắn sau `RequireAuth` (`router.go:85`): 1 query JOIN `teachers` ⋈ `centers` ⋈ `user_accounts` (`status = 'active' AND deleted_at IS NULL` — carried finding #5, cùng chuẩn `auth/service.go:90,151`) → set Scope. Resolver là consumer interface implement phía `centers`, inject qua router (precedent setter injection `router.go:134-140`).
- `authctx.TeacherID()` giữ nguyên tồn tại trong phase này (callers hiện có chưa đổi — Phase 3 sweep chuyển toàn bộ sang `ScopeFrom` rồi xoá).

### Feature package `centers`

`internal/features/centers/`: `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go`, `dto.go`, `integration_test.go` — đúng bố cục feature hiện có.

- **Join model (consent)**: teacher tự gọi join với `owner_phone`. Phone resolve qua consumer interface `TeacherLookup` (KHÔNG gọi `teachers.Service.GetByPhone` trực tiếp — hợp đồng *"must not back an endpoint directly"* tại `teachers/service.go:73-76`; chỉ resolve tài khoản teacher `active`, chưa soft-delete). Điều kiện join (V1): center hiện tại của caller là center cá nhân họ own, **không có member khác và không có dữ liệu nghiệp vụ** (COUNT classes/students/contacts = 0) → nếu không: 409. Join = trong 1 tx: soft-delete center cá nhân rỗng + `UPDATE teachers SET center_id = $target WHERE id = $self AND center_id = $old` (RowsAffected guard — carried finding #13).
- **Remove/leave**: caller là owner của center **hoặc** chính teacher đó. Hiệu ứng: tạo center cá nhân mới cho teacher bị remove (cùng tx) + UPDATE membership scope-trong-WHERE. Owner không rời được khi center còn member khác (422). Dữ liệu Ở LẠI center cũ (quyết định plan.md).
- **Registration**: `auth` service đăng ký teacher mới → tạo center cá nhân trong cùng tx (owner = teacher mới).
- **Notification**: qua feature `notifications` hiện có (wire tại `router.go:157`) — hai phía khi join và khi remove (best-effort, không chặn response); chống chiếm phiên/gõ nhầm số như mô hình grant cũ.

## API Design

Tất cả dưới `/api/v1`, `requireAuth` (+ scope middleware).

| Method & Path | Request | Response |
|---|---|---|
| `GET /centers/me` | — | `200 {"center": {"id","name","is_owner"}, "members": [{"id","full_name","phone","is_owner"}]}` — mọi member xem được |
| `PATCH /centers/me` | `{"name": "..."}` | `200` — owner only, 403 cho member |
| `POST /centers/join` | `{"owner_phone": "0901234567"}` (binding `required,vnphone` + `NormalizePhone` — carried finding #12, pattern `auth/dto.go:11`) | `201 {"center_id","joined_at"}` — không trả tên owner/center ngoài những gì member GET được sau đó |
| `DELETE /centers/me/members/:teacherId` | — | `204` — caller = owner hoặc `:teacherId` = chính caller |

Errors: 404 chung cho phone không tồn tại/không phải teacher/không active (không phân biệt); 409 center cá nhân còn dữ liệu/member, hoặc target đã ở trong center của owner này; 422 tự join center mình đang own / owner rời khi còn member; remove `RowsAffected == 0` → 404 thống nhất (pattern `classes/repository.go:118-124`).

## Related Code Files

- Create: `apps/api/internal/features/centers/{model,repository,service,handler,routes,dto}.go`
- Create: `apps/api/internal/features/centers/integration_test.go`
- Modify: `apps/api/internal/shared/authctx/authctx.go` (Scope + ScopeFrom)
- Modify: `apps/api/internal/middleware/` (ResolveScope)
- Modify: `apps/api/internal/server/router.go` (wire resolver + `centers.RegisterRoutes`)
- Modify: `apps/api/internal/features/auth/service.go` + test (tạo center cá nhân khi đăng ký)

## Implementation Steps

1. Migration đã xong (Phase 1) — model + repository centers (membership qua `teachers.center_id`).
2. `authctx.Scope` + middleware `ResolveScope` + resolver interface, wire router.
3. Service: get-me, rename (owner check), join (điều kiện rỗng, tx, notification), remove/leave (tx tạo center cá nhân mới + single UPDATE), `TeacherLookup` interface implement phía teachers.
4. Auth registration tạo center cá nhân cùng tx.
5. Handler + routes + DTO; regenerate swagger.
6. Integration tests: join happy; join khi có dữ liệu/member → 409; phone sai/không active → 404; tự join → 422; remove bởi owner và bởi chính mình → 204, bởi member khác → 403; owner rời khi còn member → 422; **kick xong gọi API bất kỳ bằng token cũ → scope đã là center cá nhân mới ngay request kế**; đăng ký mới → có center.

## Success Criteria

- [ ] Mỗi request authenticated có Scope đúng (teacher thường: IsOwner=false trên center họ join; owner: true)
- [ ] Join/remove đúng ma trận lỗi; membership đổi hiệu lực ngay request kế tiếp
- [ ] Tài khoản `disabled`/soft-deleted không resolve được scope (401/403 như chuẩn auth hiện có)
- [ ] Đăng ký teacher mới tự có center cá nhân; test auth cũ pass
- [ ] Swagger cập nhật; tests pass

## Risk Assessment

- **+1 query mỗi request** (scope resolve): JOIN 3 bảng indexed theo PK — chấp nhận được; nếu thành bottleneck mới cache TTL ngắn (YAGNI bây giờ).
- **Enumeration qua join phone**: 404 chung, response 201 tối thiểu; rate-limit vẫn là backlog riêng (như quyết định cũ).
- **Join/remove race** (2 tx đổi `center_id` cùng lúc): single UPDATE có `AND center_id = $old` trong WHERE — bên thua RowsAffected=0 → 409/404, không bao giờ mất-center hoặc 2 center.
- **Center cá nhân "rỗng" định nghĩa thiếu bảng**: đếm qua classes/students/contacts là đủ (mọi bảng khác FK về 3 bảng này); assert bằng test tạo từng loại dữ liệu rồi join → 409.
