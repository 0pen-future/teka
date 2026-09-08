---
phase: 1
title: "API: refresh reuse grace window"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: API — refresh reuse grace window (finding 5)

## Overview

Khi một refresh token vừa bị revoke bởi rotation trong vòng `RefreshReuseGrace` (mặc định 15s) và family vẫn còn token sống, `Refresh` cấp một token anh em trong cùng family thay vì giết family. Hai tab (hoặc một request retry) refresh cùng cookie đều nhận phiên mới; replay ngoài grace, sau logout, hoặc khi grace = 0 giữ nguyên hành vi hiện tại.

## Requirements

- Functional:
  - Cấu hình `JWTConfig.RefreshReuseGrace` (env `API_JWT_REFRESH_REUSE_GRACE`, mặc định `15s`, `0` = tắt hoàn toàn, âm → lỗi validate).
  - Cả hai đường thua race đều được cứu: (1) `GetByHash` trả token đã `Revoked()`; (2) `Revoke` trả `ErrTokenAlreadyRevoked` vì tab kia commit trước.
  - Điều kiện cứu: `grace > 0` **và** family còn ít nhất một token `revoked_at IS NULL` (nghĩa là chưa bị logout/disable/reuse-detect) **và** (với đường 1) `now - revoked_at <= grace`. Với đường 2 không cần so thời gian: `ErrTokenAlreadyRevoked` chỉ xảy ra khi đọc thấy sống rồi update thấy chết, tức revoke vừa xảy ra đồng thời.
  - Token cứu vẫn qua kiểm tra tài khoản (`GetProfile`, `Status == active`) như rotation thường; token cấp mới thuộc cùng `family_id`; access token mới phát hành như thường.
  - Ngoài grace hoặc family đã chết: `RevokeFamily` + 401 `invalid refresh token` (hành vi cũ).
  - Ghi log Info `refresh reuse within grace` kèm `family_id` (không log token) để đo tần suất thực tế (câu hỏi mở #2 của plan).
- Non-functional: không migration; không đổi hợp đồng HTTP (`handler.go` đặt cookie khi thành công, xoá khi `CodeUnauthorized` — không cần sửa); web không đổi; test hiện có giữ nguyên ý nghĩa bảo mật.

## Architecture

Luồng `Refresh` sau khi sửa (giữ thứ tự `Revoked()` trước `Expired()` như master để replay token cũ đã hết hạn vẫn giết family; chỉ chèn nhánh cứu):

```text
GetByHash(hash)
├─ not found → 401
├─ Revoked()
│    └─ recoverRotationRace(t, checkAge=true)
│         ├─ grace>0 && now-revoked_at<=grace && FamilyHasLive(family) && account active
│         │     → issueSession(profile, family)  (sibling)  → 200
│         └─ else (kể cả account không active / không tồn tại) → RevokeFamily(family) → 401
├─ Expired(now) → 401
├─ profile / status checks → 401
└─ WithinTx { Revoke(id) ; issueSession(profile, family) }
     └─ ErrTokenAlreadyRevoked → recoverRotationRace(t, checkAge=false)
```

Điểm tựa bảo mật: trong repo hiện tại `Revoke(id)` đơn lẻ **chỉ** được gọi trong rotation; mọi đường "ác ý" (`RevokeFamily`, `RevokeAllForUser` khi logout/disable/reset/anonymize) làm family không còn token sống. Vì vậy "revoke gần đây + family còn sống" đủ để kết luận race rotation mà không cần cột `replaced_by`. Nếu sau này có chỗ gọi `Revoke(id)` đơn lẻ khác, giả định này vỡ — ghi comment cạnh `Repository.Revoke` nêu bất biến này.

Cấu hình đi qua `TokenIssuer` (đã nhận `config.JWTConfig`) để **không đổi chữ ký `NewService`**: thêm `func (i *TokenIssuer) RefreshReuseGrace() time.Duration`.

## Related Code Files

- Modify: `apps/api/internal/config/config.go` — `JWTConfig.RefreshReuseGrace time.Duration \`env:"JWT_REFRESH_REUSE_GRACE" envDefault:"15s"\``; validate `>= 0` cạnh các validate JWT hiện có.
- Modify: `.env.example` (root repo) — dòng `API_JWT_REFRESH_REUSE_GRACE=15s` cạnh `API_JWT_REFRESH_TTL`.
- Modify: `apps/api/internal/features/auth/tokens.go` — getter `RefreshReuseGrace()`.
- Modify: `apps/api/internal/features/auth/repository.go` — interface + gorm impl `FamilyHasLive(ctx, familyID string) (bool, error)` (`SELECT EXISTS ... WHERE family_id = ? AND revoked_at IS NULL`); comment bất biến trên `Revoke`.
- Modify: `apps/api/internal/features/auth/service.go` — `Refresh` + helper `recoverRotationRace`.
- Modify: `apps/api/internal/features/auth/service_test.go` — `fakeTokenRepository.FamilyHasLive`, `staleReadRepository` uỷ quyền; chỉnh `TestRefreshReuseRevokesFamily`, `TestRefreshConcurrentRotationRevokesFamily`; thêm test grace.
- Modify: `apps/api/internal/features/auth/integration_test.go` — `newIntegrationService` nhận grace (biến thể `newIntegrationServiceWithGrace`); chỉnh `TestRefreshRotationAndReuseAgainstRealSQL`; thêm test 2 goroutine.
- Modify: `docs/api-guidelines.md` (đoạn refresh token ~dòng 448-454); `docs/architecture.md` chỉ khi có mục refresh rotation (kiểm tra bằng grep, không thêm mục mới).
- Không đổi: `handler.go`, `routes.go`, `model.go`, migrations, web.

## Implementation Steps

1. **Config.** Thêm trường + validate; cập nhật `.env.example`; nếu `config_test.go` có test default/validate, thêm case `RefreshReuseGrace` mặc định 15s và âm → lỗi.
2. **Repository.** Thêm `FamilyHasLive` vào interface `Repository` và gorm impl. Thêm comment trên `Revoke`: "Chỉ rotation gọi Revoke cho một token; mọi đường revoke khác dùng RevokeFamily/RevokeAllForUser. `Refresh` dựa vào bất biến này để phân biệt race rotation với replay." Cập nhật mọi fake implement interface (`fakeTokenRepository` trong `service_test.go`: duyệt `repo.byHash`, so `FamilyID` và `RevokedAt == nil`; `staleReadRepository`: uỷ quyền xuống repo thật, **không** stale — vì `EXISTS` trên Postgres READ COMMITTED sau khi UPDATE thất bại sẽ thấy commit của tab thắng).
3. **TokenIssuer.** Getter `RefreshReuseGrace()`; trong `service_test.go` `newTestAuthService` giữ `NewTokenIssuer(config.JWTConfig{...})` và bổ sung `RefreshReuseGrace: 15 * time.Second`; thêm helper `newTestAuthServiceWithGrace(t, grace)` cho test strict.
4. **Service.** Viết `recoverRotationRace(ctx, t *RefreshToken, now time.Time, checkAge bool) (*Session, error)`:
   ```go
   grace := s.issuer.RefreshReuseGrace()
   within := grace > 0 && (!checkAge || (t.RevokedAt != nil && now.Sub(*t.RevokedAt) <= grace))
   if within {
       alive, err := s.repo.FamilyHasLive(ctx, t.FamilyID)
       if err != nil { return nil, apperror.Internal(err) }
       if alive {
           p, err := s.activeProfile(ctx, t.UserID) // tách từ đoạn GetProfile/Status check hiện có trong Refresh
           if err == nil {
               slog.Info("refresh reuse within grace", "family_id", t.FamilyID)
               return s.issueSession(ctx, p, t.FamilyID)
           }
       }
   }
   _ = s.repo.RevokeFamily(ctx, t.FamilyID)
   return nil, apperror.Unauthorized("invalid refresh token")
   ```
   Gọi nó ở hai chỗ (nhánh `t.Revoked()` với `checkAge=true`; nhánh `errors.Is(err, ErrTokenAlreadyRevoked)` với `checkAge=false`). `service.go` đã dùng `slog` package-level (`slog.Warn` ở nhánh reset DM), dùng cùng cách.
   Lưu ý: `issueSession` ngoài tx chỉ có một `Create` — không cần `WithinTx`.

   **Cập nhật sau review (đóng MINOR-3):** pseudocode trên revoke family vô điều kiện khi `activeProfile` lỗi. Bản sửa cuối tách hai loại lỗi bằng helper `isUnauthorized(err)`: lỗi 401 thật (tài khoản không còn tồn tại/không active) mới rơi xuống `RevokeFamily` + 401 như cũ; lỗi transient (500, ví dụ DB lỗi tạm thời) trả nguyên `err` mà **không** revoke family — giết family vì một lỗi hạ tầng tạm thời là sai. Hàm cuối cùng tên `reuseAfterRotation` (không phải `recoverRotationRace`).
5. **Unit test** (`service_test.go`, dùng `svc.now` injectable):
   - Chỉnh `TestRefreshReuseRevokesFamily`: sau rotation, tiến `svc.now` thêm `grace + 1s` rồi replay T0 → 401 và family chết (giữ assertion cũ).
   - Thêm `TestRefreshReuseWithinGraceIssuesSiblingToken`: T0 → T1; replay T0 ngay → 200, T2 khác T1, cùng family, cả T1 và T2 sống; sau đó tiến `svc.now` quá grace, replay T0 lần nữa → 401 và **cả T1, T2** đều bị revoke.
   - Thêm `TestRefreshReuseWithinGraceAfterLogoutRejects`: T0 → T1; `Logout(T1)`; replay T0 ngay → 401 (family không còn token sống).
   - Thêm `TestRefreshReuseGraceDisabledKeepsStrictRevocation`: dùng `newTestAuthServiceWithGrace(t, 0)`; replay ngay → 401 + family chết.
   - Chỉnh `TestRefreshConcurrentRotationRevokesFamily` (dùng `staleReadRepository`): tách hai case — grace mặc định → phiên thứ hai thành công, 2 token sống; grace 0 → hành vi cũ (đổi tên test cho đúng ý nghĩa, ví dụ `TestRefreshConcurrentRotation...` với subtest `grace`/`strict`).
   - Thêm case tài khoản bị disable giữa chừng: `fakeAccountService` trả status inactive → nhánh cứu vẫn 401.
6. **Integration test** (`integration_test.go`, cần Docker): tham số grace vào `newIntegrationService`; giữ `TestRefreshRotationAndReuseAgainstRealSQL` với grace 0; thêm `TestRefreshConcurrentTabsAgainstRealSQL`: cùng plaintext token, 2 goroutine gọi `Refresh` đồng thời qua `sync.WaitGroup`, cả hai `err == nil`, `liveTokenCount(family) == 2`; sau đó `Logout` một trong hai → `liveTokenCount == 0` (RevokeFamily vẫn quét toàn family).
7. **Handler test**: `TestRefreshRotatesCookieOverHTTP` không đổi; thêm một test HTTP dùng lại cookie cũ lần hai trong grace → 200 và cookie mới được đặt (chứng minh handler không cần sửa).
8. **Docs.** `docs/api-guidelines.md`: bổ sung câu về grace window, env, và lý do (race giữa tab); nêu rõ tắt bằng `0`. Kiểm tra `docs/deployment.md` env table nếu có liệt kê `API_JWT_*` thì thêm dòng.
9. **Verify.** `make test-api-unit`, `make test-api`, `make lint-api`.

## Success Criteria

- [x] `API_JWT_REFRESH_REUSE_GRACE` mặc định 15s, `0` tắt, âm lỗi khởi động; `.env.example` có dòng mới.
- [x] Unit: sibling token cùng family khi replay trong grace; 401 + family chết khi ngoài grace, sau logout, tài khoản không active, hoặc grace 0.
- [x] Integration: 2 goroutine refresh cùng token đều 200, `liveTokenCount == 2`; logout sau đó giết cả family.
- [x] Tất cả test cũ vẫn pass với assertion bảo mật nguyên vẹn (chỉ đổi mốc thời gian / cấu hình).
- [x] `docs/api-guidelines.md` mô tả grace; `make lint-api` xanh.

## Risk Assessment

- **Số token anh em sinh trong grace không có trần** (quyết định sau review): chấp nhận. Cửa sổ chỉ 15s, người trình token đã bị rotate phải đang giữ một token từng sống, và mọi token anh em cùng family nên logout/disable/reuse-detect giết tất cả cùng lúc. Tín hiệu cần xem lại: log `refresh reuse within grace` lặp nhiều lần cho cùng `family_id` trong một cửa sổ → thêm trần đếm token sống per family hoặc limiter theo cookie hash trên `/auth/refresh`.

- **Nới lỏng phát hiện replay trong grace.** Kẻ có cookie trộm và replay trong 15s nhận token sống thay vì kích hoạt kill-family. Giảm thiểu: cửa sổ ngắn, cấu hình về 0 cho môi trường nhạy cảm; access TTL 15m vẫn giới hạn thiệt hại; phát hiện replay ngoài grace không đổi. Tín hiệu: log `refresh reuse within grace` xuất hiện với family có > 2 token sống lặp lại → hạ grace / tắt.
- **TOCTOU giữa `FamilyHasLive` và `Create` sibling** khi logout chạy chen giữa (mili-giây): sibling sống sót sau logout của chính user. Chấp nhận (yêu cầu vừa có token cũ vừa trúng cửa sổ ms); đường nâng cấp nếu cần: `pg_advisory_xact_lock(hashtext(family_id))` trong cả nhánh cứu và `RevokeFamily` (repo imports đã có mẫu advisory lock). Không làm trong phase này.
- **Số token sống trong family tăng theo số tab đua** (mỗi lần cứu +1). Bị chặn bởi `RefreshTTL` (30 ngày) và replay ngoài grace giết cả family. Nếu cần, thêm dọn dẹp định kỳ — ngoài phạm vi.
- **Giả định "chỉ rotation gọi `Revoke`"** — comment trên interface; nếu tương lai thêm chỗ gọi khác, test `...AfterLogoutRejects` là lưới đỡ một phần; cân nhắc thêm test tĩnh (grep) trong review.
