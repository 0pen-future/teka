# Phase 3: Accept Flow and Offboarding Rewire — Báo cáo thực thi

- Plan: `plans/260812-0904-invite-only-onboarding/phase-03-accept-flow-and-offboarding-rewire.md`
- Trạng thái: **DONE**

## Phạm vi đã làm

1. **Public accept flow** (`internal/features/invitations`): `POST /invitations/preview`
   và `POST /invitations/accept` — mount ngoài `requireAuth`/`resolveScope`, token đi
   trong body (không lộ qua path/query log). Toàn bộ `Accept` chạy trong một
   `WithinTx` với `SELECT ... FOR UPDATE` khoá row invitation, chống double-accept.
   - Số điện thoại mới → tạo account qua `AccountOnboarder.CreateInCenter` rồi
     `MembershipOpener.OpenMembership`.
   - Account bị disable và từng là thành viên đúng center này (`WasEverMember`) →
     `Reactivate` + `OpenMembership` + `SwitchTeacherCenter`.
   - Mọi lý do từ chối khác (token lạ/hết hạn/đã dùng/bị thu hồi, account đang active,
     account disabled nhưng chưa từng thuộc center này) đều trả về đúng một giá trị
     `*apperror.AppError` dùng chung (`errAcceptRejected`) — chứng minh bằng
     `require.Same` (pointer identity) ở service test, không chỉ so sánh JSON.
2. `teachers.Service.CreateInCenter`/`Reactivate`, `auth` (xoá `/auth/register`, thêm
   `RevokeAllForUser` + `AccountDisabler`), `centers.RemoveMember` rewrite,
   `middleware.RateLimit` — đã hoàn tất từ phiên trước, xác nhận lại lần này.
3. `router.go`: nối `invitationsSvc` với chữ ký constructor 7 tham số mới
   (`teachersSvc` làm `AccountOnboarder`, `centersSvc` làm `MembershipOpener` — tham
   số constructor thường, không cần setter vì không có vòng phụ thuộc), và mount
   `RegisterPublicRoutes` với 2 rate limiter riêng (20/phút cho preview, 10/phút cho
   accept, key theo `token` trong body — IP không phù hợp vì invitee chưa đăng nhập).

## Lỗi phát hiện và sửa trong lần chạy verify này

`make test-api` phát hiện `TestCreateInCenterPhoneFormsCollide`
(`internal/features/teachers/repository_test.go`) FAIL với lỗi FK thật từ Postgres:
`violates foreign key constraint "fk_teachers_membership"`.

- **Nguyên nhân**: `teachers.center_id` có FK `DEFERRABLE INITIALLY DEFERRED` vào
  `center_members(teacher_id, center_id)` — check ở thời điểm commit transaction.
  `CreateInCenter` (mới thay `CreateTeacher`) chỉ ghi `user_accounts` +
  `teachers`, KHÔNG tự mở membership — theo đúng thiết kế interface tách
  `AccountOnboarder`/`MembershipOpener`, nơi caller (`invitations.Accept`) luôn gọi
  `CreateInCenter` rồi `OpenMembership` trong cùng một `WithinTx`. Test cũ (viết ở
  phiên trước, khi sửa từ `CreateTeacher`+`CenterProvisioner` sang `CreateInCenter`)
  gọi `svc.CreateInCenter` đơn lẻ, không mở membership, không trong transaction nào
  cả → vi phạm FK khi service tự commit ngầm qua GORM.
- **Sửa**: viết lại test để gọi `CreateInCenter` + `centersSvc.OpenMembership` trong
  cùng một `txMgr.WithinTx`, đúng như cách `invitations.Accept` dùng thật trong
  production. Đây là sửa test cho đúng hợp đồng thật của `CreateInCenter`, không đổi
  hành vi service.
- Xác nhận lại bằng `go test -tags integration ./internal/features/teachers/...` → pass.

## Audit theo Key Insights của phase spec

- `grep -rn "register\|/join"` trong `internal`: không còn route `/auth/register`
  hay `/centers/join` nào (chỉ còn các chuỗi "register"/"registered" vô hại trong
  comment/tên hàm Gin `registerFeatures`/`registerHealth`).
- `grep -rn "CenterProvisioner\|SetCenterProvisioner"`: không tìm thấy ký hiệu nào
  còn lại trong `internal` hay `seeds` — seam đã bị xoá hoàn toàn từ phiên trước,
  không có gì cần audit thêm.

## Kết quả kiểm thử

- `golangci-lint run` (v2.7.2): **0 issues** (đã sửa 1 lỗi `revive` unused-parameter
  trong `service_test.go` phát hiện ở lần chạy đầu).
- `make test-api` (toàn bộ pyramid, có integration test qua testcontainers-go,
  Postgres 16-alpine, ~44 test package chạy song song):
  - **Tất cả package `ok`**, bao gồm `invitations` (46.091s), `teachers` (37.799s),
    `centers` (46.698s), `auth` (43.004s), `server` (0.045s).
  - **Total coverage: 66.7%** (floor 60%) — đạt.
- Bộ test riêng của `invitations` (`go test -tags integration -v`): 13/13 test pass
  trong 6.241s, gồm double-accept concurrency test (đúng 1/2 accept thắng, đúng 1 row
  `user_accounts` được tạo — chứng minh row lock hoạt động thật trên Postgres) và
  round-trip đầy đủ invite→accept→login, remove→re-invite→reactivate→login.

## Files đã sửa/tạo trong toàn bộ Phase 3 (bao gồm phiên trước + phiên này)

- `apps/api/internal/features/invitations/{service,dto,errors,repository,handler,routes}.go`
- `apps/api/internal/features/invitations/{service_test,handler_test,integration_test}.go`
- `apps/api/internal/features/teachers/service.go`, `service_test.go`,
  `repository_test.go` (sửa trong lần verify này)
- `apps/api/internal/features/auth/{service,handler,routes,dto,repository}.go` (+tests)
- `apps/api/internal/features/centers/{service,handler,routes,dto}.go` (+tests)
- `apps/api/internal/middleware/ratelimit.go`, `ratelimit_test.go`
- `apps/api/internal/server/router.go`

Chưa commit gì — đúng theo ràng buộc được giao.

## Việc chưa làm / không thuộc phạm vi Phase 3

- Không có TODO/vấn đề còn treo trong phạm vi Phase 3.
- Phase 4 (Password Reset API), Phase 5 (Operator CLI), Phase 6 (Web UI) và Phase 7
  (OpenAPI docs + verification sweep cuối) vẫn đang `pending`, ngoài phạm vi được
  giao lần này.
