# Code review — feat/backbone-hardening

Phạm vi: `master..HEAD` (5 commit code: c91ff2f, f3f6701, 0204764, 71b9e9a, c4c7ab5), 42 file, +1922/−118.
Kế hoạch đối chiếu: `plans/260908-1009-backbone-hardening/plan.md` + phase 01–05.
Không sửa file nào trong lúc review.

## Điểm cycle 1: 8/10

Chất lượng cao: test thật (không phải test giả chạy code mà không chứng minh hành vi), docs đầy đủ và chính xác, comment giải thích bất biến thay vì mô tả code, không có plan ID / finding ID trong code, không thêm dependency, không sửa migration. Điểm trừ chính là một đường vòng qua rate limiter theo phone — chính là lớp bảo vệ trung tâm mà phase 3 hứa — cộng vài chỗ lệch giữa code và tiêu chí đã ghi trong plan.

## Kết quả kiểm tra tự động

| Lệnh | Kết quả |
|---|---|
| `make lint-api` | 0 issues |
| `cd apps/api && go vet ./...` | sạch |
| `make test-api-unit` | toàn bộ package pass |
| `cd apps/web && npm run typecheck` | sạch |
| `cd apps/web && npm run lint` | 0 error, 5 warning (đều là `react-hooks/incompatible-library` ở `student-dialog.tsx` và `class-settings-page.tsx`, file branch không chạm — đúng 5 warning đã biết) |
| `cd apps/web && npm run test` | 85 file, 639 pass, 3 skip |
| `make api-docs` | không sinh diff (đúng như plan yêu cầu) |

`make test-api` không chạy theo yêu cầu.

## Findings

### MAJOR-1 — Rate limit login/forgot-password bị vòng qua bằng cách đổi hoa/thường tên field JSON

`apps/api/internal/middleware/ratelimit.go:137` (`payload[field]` trong `JSONBodyKey`), ảnh hưởng `PhoneKey` ở `ratelimit.go:147` và mọi limiter `JSONBodyKey("token")`.

**Kịch bản hỏng.** `JSONBodyKey` unmarshal body vào `map[string]any` rồi tra khoá **chính xác** `"phone"`. Trong khi đó `encoding/json` khi decode vào struct lại khớp tên field **không phân biệt hoa thường**. Vì vậy body `{"Phone":"0912345678","Password":"x"}`:

- `payload["phone"]` → không có → key rỗng → `RateLimit` bỏ qua hoàn toàn (`ratelimit.go:99-103`);
- `c.ShouldBindJSON(&LoginRequest{})` vẫn bind thành công (`Phone` có tag `json:"phone"`), handler gọi `Service.Login`, bcrypt chạy bình thường.

Đã xác minh bằng chương trình stdlib độc lập:

```
struct bind err: <nil> phone: 0912345678 pass: secret
JSONBodyKey("phone") -> ""
```

Hệ quả: brute-force một tài khoản không giới hạn số lần (tiêu chí phase 3 "lần thứ 11 → 429" bị vô hiệu chỉ bằng cách viết `Phone` thay vì `phone`); `forgot-password` 5/phút cũng bị vòng qua, tức có thể spam DM Zalo reset tới nạn nhân không giới hạn; hai limiter `JSONBodyKey("token")` (accept invitation, reset password) mất luôn giới hạn đoán token.

`bcryptGate` vẫn chặn cạn CPU, nên đây không phải lỗ hổng DoS — nhưng nó phá đúng lớp (a) trong ba lớp của D3.

Lớp lỗi này có từ trước ở `JSONBodyKey`, nhưng phase 3 mới đặt an toàn brute-force lên nó và comment ở `ratelimit.go:141-145` khẳng định "a caller cannot dodge the bucket", nên phải xử lý trong branch này.

**Đề xuất sửa.** Tra khoá theo đúng luật của `encoding/json`: ưu tiên khớp chính xác, sau đó khớp không phân biệt hoa thường.

```go
v, ok := payload[field].(string)
if !ok {
    for k, raw := range payload {
        if strings.EqualFold(k, field) {
            v, _ = raw.(string)
            break
        }
    }
}
return v
```

Thêm case test trong `TestPhoneKeyNormalizesLocalAndInternationalForms`: `{"Phone":"0901234567"}` phải ra `+84901234567`.

---

### MINOR-2 — Đổi thứ tự `Expired` / `Revoked` làm mất tín hiệu phát hiện replay

`apps/api/internal/features/auth/service.go:231-238`.

Trước đây `Refresh` kiểm tra `t.Revoked()` **trước** `t.Expired(now)`; nay ngược lại. Replay một token vừa bị revoke **và** đã quá `RefreshTTL` (30 ngày) giờ trả 401 mà không gọi `RevokeFamily`, trong khi trước đó nó giết cả family. Cửa sổ hẹp (chỉ token quá hạn) và family trong đa số trường hợp đã chết sẵn, nên tác động thấp — nhưng đây là thay đổi hành vi bảo mật không được nêu trong phần Requirements của phase 1 (phase 1 viết "giữ thứ tự hiện tại, chỉ chèn nhánh cứu", còn sơ đồ trong cùng file lại vẽ `Expired` trước; code theo sơ đồ).

**Đề xuất.** Hoặc trả lại thứ tự cũ (`Revoked` trước, để replay token hết hạn vẫn giết family), hoặc giữ nguyên và ghi một câu vào phase 1 / `docs/api-guidelines.md` rằng token hết hạn không còn kích hoạt kill-family. Đừng để plan và code nói khác nhau.

---

### MINOR-3 — Nhánh cứu không revoke family khi tài khoản không còn active

`apps/api/internal/features/auth/service.go:289-292`.

```go
p, err := s.activeProfile(ctx, t.UserID)
if err != nil {
    return nil, err          // thoát sớm, không RevokeFamily
}
```

Pseudocode của phase 1 dùng `if err == nil { ... }` rồi rơi xuống `RevokeFamily`; Success Criteria phase 1 ghi "401 **+ family chết** khi ... tài khoản không active". Test `TestRefreshReuseWithinGraceRejectsDisabledAccount` chỉ assert 401, không assert family chết — nên tiêu chí này chưa thực sự được chứng minh.

Thực tế hiện chưa lộ ra ngoài: `teachers.Service.Disable` gọi `RevokeAllForUser`, nên family đã hết token sống và `FamilyHasLive` trả false trước khi tới đây. Nhưng bất biến trong code yếu hơn bất biến trong tài liệu, và nếu sau này có đường đổi `status` mà không revoke token thì replay sẽ để family sống.

**Đề xuất.** Tách hai loại lỗi: lỗi 401 (không tìm thấy / không active) thì `RevokeFamily` rồi trả 401; lỗi transient (500) thì trả nguyên như hiện tại — trả 500 mà giết family là sai, và đây là điểm code đang làm đúng hơn plan. Sau đó bổ sung assert family chết vào test.

---

### MINOR-4 — Không giới hạn số token anh em một token cũ có thể sinh trong grace

`apps/api/internal/features/auth/service.go:279-296`; `POST /api/v1/auth/refresh` không có limiter nào (`routes.go`).

Mỗi lần trình lại token đã revoke trong grace đều `Create` một token sống mới trong family. Số lần lặp chỉ bị giới hạn bởi tốc độ request. Test tích hợp mới đã cho thấy điều này: ba lần refresh cùng một token → `liveTokenCount == 3`. Với vòng lặp, một client lỗi (hoặc kẻ giữ cookie trộm) có thể sinh hàng nghìn refresh token TTL 30 ngày trong 15 giây.

Plan có nhận rủi ro "số token sống trong family tăng theo số tab đua", nhưng ở mức 2–3 tab, không phải vòng lặp không chặn.

**Đề xuất.** Rẻ nhất: từ chối cứu khi family đã có quá N token sống (ví dụ 5) — dùng `FamilyHasLive` biến thể đếm thay vì `EXISTS`; hoặc mount một `RateLimit` theo cookie hash trên `/auth/refresh`. Không chặn merge, nhưng nên ghi vào plan nếu quyết định để lại.

---

### MINOR-5 — Lỗi context từ bcrypt gate thoát ra thành 500 và ghi log error

`apps/api/internal/features/auth/service.go:186-190`.

`gate.run` trả `ctx.Err()` khi caller bỏ đi. `Login` chỉ dịch `errBcryptBusy`, còn `context.Canceled` được trả nguyên; `response.Err` → `apperror.From` biến nó thành `INTERNAL_ERROR` 500 và ghi `logger.Error("request failed")` (`shared/response/response.go:56-61`). Khi có burst login mà client ngắt kết nối, log production sẽ đầy 500 giả.

**Đề xuất.** Bắt riêng `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` và trả `apperror.TooManyRequests` (hoặc một lỗi không log ở mức error), như cách gate-busy đang làm.

---

### MINOR-6 — Login giờ trả 429 nhưng annotation swagger không khai báo

`apps/api/internal/features/auth/handler.go:48-49` chỉ có `@Failure 401` và `@Failure 422`.

Sau branch này `/auth/login` trả 429 ở hai đường (limiter theo phone, gate bận). `forgot-password` và `reset-password` đã khai báo `@Failure 429` (`handler.go:125`, `handler.go:151`), nên đây là lệch chuẩn nội bộ. `make api-docs` không diff đã xác nhận spec OpenAPI hiện không nhắc tới 429 trên login, và cũng không nhắc tới 413 ở đâu cả.

**Đề xuất.** Thêm `@Failure 429` vào login (và cân nhắc một dòng 413 ở mục chung của spec), rồi chạy lại `make api-docs` — đây là diff swagger "có chủ đích" mà constraint của plan cho phép.

---

### MINOR-7 — `docs/architecture.md` vẫn ghi "no trusted proxies"

`docs/architecture.md:37`: `→ gin engine (no trusted proxies)`.

Branch đã sửa ngay dòng dưới (thêm body-limit) nhưng để lại mô tả đã lỗi thời: `API_HTTP_TRUSTED_PROXIES` giờ đổi được hành vi này.

**Đề xuất.** Sửa thành `gin engine (trusted proxies theo API_HTTP_TRUSTED_PROXIES, mặc định không tin ai)`.

---

### NIT-8 — Đường miễn body cap là chuỗi thứ ba, không ràng buộc với route thật

`apps/api/internal/server/router.go:75` hard-code `"/api/v1/imports/roster"`; `internal/shared/routespec/routespec.go:207` có cùng path; route thật đăng ký ở `imports/routes.go`. `bodylimit_test.go` tự đăng ký chuỗi literal của chính nó nên không bảo vệ được liên kết này. Nếu path import đổi, upload 1.5 MiB sẽ âm thầm bị 413 và chỉ suite Docker mới bắt được. Rủi ro thấp vì `routespec_test` giữ path ổn định.

**Đề xuất.** Tham chiếu hằng dùng chung, hoặc thêm một test router-level: POST > cap tới `/api/v1/imports/roster` phải không bị 413 bởi middleware toàn cục.

---

### NIT-9 — `max(1, runtime.GOMAXPROCS(0))` là code phòng thủ chết

`apps/api/internal/features/auth/service.go:130`. `runtime.GOMAXPROCS(0)` luôn ≥ 1. (Go 1.25 nên GOMAXPROCS đã nhận biết giới hạn cgroup — kích thước gate trong container là đúng, không cần env riêng.)

## Đối chiếu Success Criteria

**Phase 1 — refresh reuse grace.** Đạt, trừ hai chỗ đã nêu.
- Env mặc định 15s / `0` tắt / âm lỗi: có, `config_test.go` phủ cả ba.
- Sibling trong grace, 401 + family chết ngoài grace / sau logout / grace 0: có unit test cho từng đường (`TestRefreshReuseWithinGraceIssuesSiblingToken`, `...AfterLogoutRejects`, `TestRefreshReuseGraceDisabledKeepsStrictRevocation`, `TestRefreshLostRotationRace` hai subtest).
- "tài khoản không active → family chết": **chưa chứng minh** (MINOR-3).
- Integration 2 goroutine + logout giết family: có (`TestRefreshConcurrentTabsAgainstRealSQL`), chưa chạy vì suite Docker đang bận ở process khác.
- Test cũ giữ nguyên assertion bảo mật, chỉ dịch mốc thời gian: đúng.

**Phase 2 — body cap.** Đạt đủ.
- 413 trước handler qua router thật: `TestOversizedBodyIsRefusedBeforeHandlers`.
- Chunked vượt cap → 413 qua cả `JSONBodyKey` lẫn handler thường: `TestBodyLimitCutsOffUndeclaredBodyOverCap`, `TestJSONBodyKeyOversizedBodySurfacesAsPayloadTooLarge`, `TestShouldBindJSONSurfacesMaxBytesError` (test cuối chứng minh gin trả `*http.MaxBytesError` nguyên vẹn — đúng thứ plan yêu cầu kiểm chứng ở bước 1).
- Route miễn đọc hết body: có, xem NIT-8 về độ bền của liên kết.
- Kiểm tra thêm của tôi: mọi DTO nhận mảng (`attendance.ConfirmRequest`, `payments` allocations) đều bị chặn bởi sĩ số lớp / số hoá đơn, cách xa 1 MiB — cap mặc định an toàn. Chỉ có một route multipart trong toàn API và nó nằm trong danh sách miễn.

**Phase 3 — login limit + gate + trusted proxies.** Đạt về mặt cấu trúc, **thủng ở tiêu chí 1** vì MAJOR-1.
- 429 ở lần thứ 11, hai cách viết số chung bucket: có test (`TestLoginRateLimitedPerNormalizedPhone`) nhưng bypass được (MAJOR-1).
- Gate bận → 429, không `LoginFailed`, phone lạ và phone đúng cùng 429: `TestLoginRefusesWhileBcryptGateIsBusy` assert đúng cả ba.
- Trusted proxies rỗng → không mount IP limiter; khác rỗng → có; giá trị sai → lỗi khởi động: `TestLoginIPLimiterOnlyMountedWithTrustedProxies` + `config_test.go`. Đây là test tốt: nó đếm 61 request từ một IP với 61 số khác nhau nên chỉ limiter theo IP mới trả 429 được.
- forgot-password dùng `PhoneKey`: có.
- Web hiện message 429: có test.

**Phase 4 — SessionCacheReset.** Đạt.
- Cả 5 nhánh trạng thái đều có test, kể cả end-to-end 401 → refresh 401 → cache rỗng.
- `queryClient.clear()` chỉ còn đúng một chỗ trong `apps/web/src` (đã grep).
- `useLogout` chỉ còn một call site (`dashboard-layout.tsx`), nằm trong `Providers` nên component luôn có mặt.

**Phase 5 — invalidate sessions.** Đạt đủ 5 đường (create, end, delete, import commit, anonymize) + một test khẳng định dry-run **không** invalidate.

## Kiểm tra riêng theo yêu cầu (b)–(f)

**(b) Hồi quy nghiệp vụ.** Không thấy.
- `Login`: thứ tự vẫn là lookup → phán xét → `TouchLastLogin` → `openSession` → `LoginSucceeded`. Bốn đường thất bại (phone lạ, không active, không có password, sai password) vẫn cùng 401 + `LoginFailed`; lỗi transient của `GetByPhone` vẫn thoát sớm không publish. `hash == nil` khi phone lạ nên `p` không bao giờ bị deref trên đường đó.
- `dummyBcryptHash` cost 12 khớp `bcryptCost` dùng để hash thật, nên `burnPassword` trong gate vẫn tương đương thời gian với compare thật.
- `Refresh`: nhánh `activeProfile` giữ nguyên ngữ nghĩa cũ (NotFound → 401, transient → 500, không active → 401).
- `RegisterRoutes` chỉ có 2 caller (`router.go:132`, `handler_test.go:43`), cả hai đã cập nhật; slice variadic được copy trước khi append nên không ghi đè backing array của caller.
- Interface `Repository` thêm method: chỉ `gormRepository` và hai fake trong test implement; build + vet sạch.

**(c) Public contract.** Ba env mới đều có default an toàn và tài liệu ở `.env.example` + `docs/deployment.md`. Một mã lỗi mới `PAYLOAD_TOO_LARGE` (413) đúng như constraint cho phép. Envelope không đổi. Cookie path không đổi. Chữ ký `RegisterRoutes` đổi nhưng variadic nên tương thích ngược ở mức nguồn. Thiếu sót duy nhất là annotation swagger (MINOR-6).

**(d) Bảo mật.**
- Timing của `Login`: mọi đường thất bại đều tốn đúng một bcrypt trong gate; đường 429 giống hệt nhau cho phone lạ và phone đúng (có test).
- Gate không bypass được: nó nằm trong service, không phụ thuộc middleware.
- Limiter theo phone: **bypass được**, xem MAJOR-1. Ngoài trường hợp đó thì kín — `vnPhonePattern` chỉ chấp nhận đúng hai cách viết và `NormalizePhone` gộp chúng, phone rỗng/thiếu thì handler 422 trước khi tới bcrypt, phone sai định dạng vẫn bị limit theo chuỗi thô.
- `X-Forwarded-For`: với danh sách rỗng, `ClientIP()` là địa chỉ socket và IP limiter không được mount, nên không có gì để giả mạo; với danh sách cấu hình, gin chỉ tin XFF từ peer nằm trong danh sách (test `TestClientIPKeyUsesGinResolvedIP` chứng minh cả hai chiều). Config validate từng phần tử là IP hoặc CIDR và loại bỏ mục rỗng.
- Body cap: `Content-Length` khai gian bị `MaxBytesReader` cắt; chunked bị cắt; route 404 vẫn bị cap (`FullPath()` rỗng không nằm trong map miễn); route miễn duy nhất nằm sau `RequireAuth` và tự bọc `MaxBytesReader` **trước** khi parse form.
- Replay trong grace: kẻ giữ cookie trộm nhận được token sống — đây là rủi ro đã được chấp nhận có chủ đích trong D1 và ghi trong `docs/api-guidelines.md`, tôi không mở lại. Chỉ bổ sung MINOR-4 về việc số lần cứu không bị chặn.

**(e) Concurrency.**
- `bcryptGate.run`: `defer func() { <-g.slots }()` đặt sau khi chiếm được slot, nên panic trong `fn` vẫn trả slot; `defer timer.Stop()` không rò timer; `select` ba nhánh không có đường nào chiếm slot rồi bỏ rơi. `matched` ghi trong closure và đọc sau khi `run` trả về, cùng goroutine — không data race.
- `SessionCacheReset`: `prevRef` khởi tạo bằng id lúc mount nên không clear khi mount đã đăng nhập; effect cập nhật `prevRef` sau khi so sánh nên StrictMode chạy effect hai lần không clear hai lần. Query đang bay bị huỷ khi `clear()` — đúng bằng hành vi cũ của `useLogout`, không phải hồi quy mới.
- `limiter` vẫn dùng mutex + sweep lazy, không thêm goroutine nền (đúng rule process-management).

**(f) Convention.** Dùng `response.Err` + `apperror` như phần còn lại; `slog` package-level như đường reset DM đã làm; test đặt tên theo hành vi; comment mô tả bất biến, không có ID plan/phase/finding trong code, test hay commit message; commit message theo conventional commit, không có tham chiếu AI; web import `sessionsKeys` qua barrel `@/features/attendance` đúng tiền lệ `use-classes.ts`.

## Recommended Actions

1. Sửa MAJOR-1 (`JSONBodyKey` tra khoá không phân biệt hoa thường) + test. Nên chặn merge.
2. Quyết MINOR-3: revoke family khi profile không active, và assert điều đó trong test — hoặc sửa Success Criteria của phase 1 cho khớp code.
3. Chốt MINOR-2: trả lại thứ tự `Revoked`/`Expired` hoặc ghi rõ thay đổi vào docs.
4. MINOR-5 (context error → 500 giả trong log) và MINOR-6 (`@Failure 429` cho login) — nhỏ, gộp vào một commit dọn.
5. MINOR-7 sửa một dòng `docs/architecture.md`.
6. MINOR-4 và NIT-8: ghi vào plan như rủi ro đã biết nếu không làm ngay.
7. Chạy `make test-api` (suite Docker) khi process kia xong — hai test tích hợp mới của phase 1 chưa được chạy trong lần review này.

## Câu hỏi chưa giải quyết

1. Có chấp nhận để replay token đã hết hạn không còn giết family không (MINOR-2)? Đây là thay đổi ngữ nghĩa bảo mật cần người quyết, không phải lỗi rõ ràng.
2. Có muốn chặn số token anh em sinh trong grace (MINOR-4) trong branch này, hay để lại thành finding cho plan sau?

Status: DONE_WITH_CONCERNS
Summary: Branch đạt gần hết Success Criteria của cả 5 phase với test và docs chất lượng cao; lint/vet/typecheck/unit test đều xanh và `make api-docs` không diff. Một finding major: rate limit login và forgot-password bị vòng qua hoàn toàn khi client đổi hoa/thường tên field JSON (`{"Phone":...}`), làm vô hiệu lớp bảo vệ trung tâm của phase 3.
Concerns/Blockers: MAJOR-1 nên chặn merge. MINOR-2 và MINOR-3 là hai chỗ code lệch với Success Criteria/pseudocode đã ghi trong phase 1, cần quyết định sửa code hay sửa plan. Hai test tích hợp mới của phase 1 chưa chạy vì suite Docker đang bận.

---

# Cycle 2 — verify commit e8a9c1c

Phạm vi cycle 2: `e8a9c1c` (11 file, +193/−19) và `a8e3fc4` (không có trong bảng ánh xạ của lead nhưng nằm trên branch). Chỉ đọc code, không sửa file mã nguồn.

## Điểm cycle 2: 8.5/10

Tám trong chín finding đã được sửa đúng và đủ. MAJOR-1 **mới sửa được một nửa**: nó vá quy tắc *hoa/thường* nhưng không vá quy tắc *thứ tự khoá trùng*, nên đường vòng qua rate limiter login và forgot-password vẫn khai thác được với một body hợp lệ. Chưa đạt ngưỡng auto-approve ≥9.5.

## Trạng thái từng finding

| Finding | Trạng thái | Ghi chú |
|---|---|---|
| MAJOR-1 — vòng qua rate limit bằng tên field JSON | **CHƯA XONG (còn khai thác được)** | Xem phần dưới |
| MINOR-2 — thứ tự `Revoked()` trước `Expired()` | Đã sửa | `service.go:250-263`; có test `TestRefreshReplayOfExpiredRotatedTokenRevokesFamily` assert `liveInFamily == 0` |
| MINOR-3 — profile không active phải revoke family | Đã sửa | `service.go:289-303`; lỗi transient đi qua `isUnauthorized` và surface 500; test disabled-account đã assert `liveInFamily == 0` |
| MINOR-5 — ctx cancel trả 500 giả | Đã sửa | `service.go` nhánh gate map mọi lỗi về 429; test `TestLoginRefusesWhenCallerLeavesWhileWaitingForBcrypt` chứng minh 429 và không có `LoginFailed` |
| MINOR-6 — thiếu `@Failure 429` cho login | Đã sửa | `handler.go:50`; `make api-docs` không sinh diff |
| MINOR-7 — dòng "no trusted proxies" trong `docs/architecture.md` | Đã sửa | Nay ghi "trusted proxies from API_HTTP_TRUSTED_PROXIES; default none" |
| MINOR-4 — không có trần token anh em trong grace | Đã sửa dạng ghi nhận | `phase-01-refresh-reuse-grace.md` thêm mục Risk Assessment nêu quyết định chấp nhận, kèm tín hiệu cần xem lại; sơ đồ `Refresh` đã cập nhật đúng thứ tự mới |
| NIT-8 — exemption roster import chỉ được test ở mức middleware | Đã sửa | `router_test.go` `TestRosterImportIsExemptFromGlobalBodyCap` gửi body > 1 MiB vào route thật, assert không 413 |
| NIT-9 — `max(1, GOMAXPROCS(0))` thừa | Đã sửa | `service.go` còn `newBcryptGate(runtime.GOMAXPROCS(0), bcryptGateWait)` |

## MAJOR-1 vẫn mở — bằng chứng

`apps/api/internal/middleware/ratelimit.go:142-156`. `bodyField` trả về khoá khớp chính xác trước, sau đó khớp không phân biệt hoa thường. Nhưng `encoding/json` khi decode vào struct **gán lần lượt theo thứ tự khoá xuất hiện trong document**, nên khi có nhiều khoá cùng khớp thì **khoá cuối cùng thắng**. `map[string]any` giữ `phone` và `Phone` thành hai entry riêng, và `bodyField` luôn chọn entry khớp chính xác bất kể vị trí.

Đối chiếu trực tiếp `bodyField` với `json.Unmarshal` vào struct `LoginRequest`:

```
{"PHONE":"a","phone":"b"}          handler="b"      limiter="b"      OK
{"phone":"a","PHONE":"b"}          handler="b"      limiter="a"      MISMATCH
{"phone":"junk","Phone":"victim"}  handler="victim" limiter="junk"   MISMATCH
{"Phone":"victim","phone":"junk"}  handler="junk"   limiter="junk"   OK
```

Đã kiểm chứng lại qua router gin thật (`gin.Default()` + `RateLimit(PhoneKey("phone"),…)` + handler `ShouldBindJSON`), không chỉ ở mức stdlib:

```
status=200
handler bcrypts against phone = "0912345678"
rate-limit bucket key       = "0900000001"
```

**Kịch bản hỏng.** Kẻ tấn công gửi `{"phone":"<số rác thay đổi mỗi request>","Phone":"<số nạn nhân>","password":"..."}`. Handler bind số nạn nhân và chạy bcrypt trên tài khoản nạn nhân; limiter lại bỏ vào bucket của số rác, mỗi request một bucket mới. Trần 10 lần/phút của login và 5 lần/phút của forgot-password đều không còn tác dụng. Chỉ còn `bcryptGate` (concurrency, không phải rate) và IP limiter (chỉ mount khi có trusted proxies) đỡ.

**Test mới không bắt được lỗi này.** `ratelimit_test.go` `TestJSONBodyKeyMatchesFieldLikeStructBinding` chọn đúng thứ tự `{"PHONE":"a","phone":"b"}` — trường hợp duy nhất mà hai cách cài đặt tình cờ trùng kết quả. Đảo thứ tự thành `{"phone":"a","PHONE":"b"}` là test đỏ. Đây là test đi qua code mà không chứng minh được bất biến cần chứng minh.

**Đề xuất sửa (chọn một).**

1. *Dùng lại chính `encoding/json` thay vì cài lại quy tắc của nó.* Dựng một struct một field bằng `reflect.StructOf` với tag `json:"<field>"` ngay khi tạo `KeyFunc` (một lần cho mỗi limiter, không phải mỗi request), rồi `json.Unmarshal` vào đó. Theo định nghĩa là khớp tuyệt đối với thứ gin bind, kể cả các quy tắc tương lai của stdlib. Đây là phương án nên chọn: chi phí một lần, không có luật nào phải tự bảo trì.
2. *Duyệt token stream.* `json.Decoder.Token()` đọc object cấp cao nhất, giữ giá trị của khoá **cuối cùng** khớp chính xác-hoặc-không-phân-biệt-hoa-thường. Đúng nhưng phải tự bảo trì quy tắc.

Kèm test cả hai thứ tự khoá và cả biến thể toàn hoa/thường (`{"PHONE":…,"pHoNe":…}` — trường hợp này hiện còn không xác định do thứ tự duyệt map ngẫu nhiên).

## Finding mới ở cycle 2

### NIT-10 — comment doc của `activeProfile` bị đẩy sang `isUnauthorized`

`apps/api/internal/features/auth/service.go:306-316`. Helper `isUnauthorized` được chèn vào **giữa** comment doc "activeProfile loads the account behind a refresh token…" và khai báo `func (s *Service) activeProfile`. Kết quả: `isUnauthorized` mang doc của hàm khác và `activeProfile` không còn doc. Sửa bằng cách chuyển `isUnauthorized` (kèm hai dòng comment của chính nó) xuống dưới `activeProfile`.

### a8e3fc4 — commit không có trong bảng ánh xạ

`apps/api/internal/server/policy_integration_test.go` được thêm `HTTP: config.HTTPConfig{MaxBodyBytes: 1 << 20}` vì config dựng tay bỏ qua `Load()`. Thay đổi đúng và tối thiểu, comment giải thích bất biến. Đáng chú ý ở chỗ nó cho thấy một rủi ro thật của `BodyLimit`: cap bằng 0 từ chối mọi request có body. Ở đường production `Load()` validate `> 0` nên an toàn; chỉ harness dựng config bằng tay mới dính. Không cần sửa thêm.

## Kết quả kiểm tra tự động (chạy lại ở cycle 2)

| Lệnh | Kết quả |
|---|---|
| `make lint-api` | 0 issues |
| `cd apps/api && go vet ./...` | sạch |
| `make test-api-unit` | toàn bộ package pass |
| `cd apps/web && npm run typecheck` | sạch |
| `cd apps/web && npm run lint` | 0 error, 5 warning `react-hooks/incompatible-library` đã biết |
| `cd apps/web && npm run test` | 85 file, 639 pass, 3 skip |
| `make api-docs` | không sinh diff |

`make test-api` vẫn không chạy theo yêu cầu.

## Recommended Actions

1. Sửa `bodyField` theo phương án 1 (`reflect.StructOf` + `json.Unmarshal`), thêm test cả hai thứ tự khoá. Vẫn nên chặn merge cho tới khi xong.
2. Chuyển `isUnauthorized` xuống dưới `activeProfile` để trả lại comment doc (NIT-10).
3. Chạy `make test-api` khi suite Docker rảnh — hai test tích hợp phase 1 vẫn chưa được chạy trong cả hai cycle của review này.

Status: DONE_WITH_CONCERNS
Summary: Tám trên chín finding cycle 1 đã sửa đúng, có test chứng minh hành vi, lint/vet/typecheck/unit test/web test đều xanh và `make api-docs` không diff; nhưng MAJOR-1 chỉ được vá một nửa nên rate limit login và forgot-password vẫn bị vòng qua.
Concerns/Blockers: `bodyField` vá quy tắc hoa/thường mà không vá quy tắc khoá trùng, nên body `{"phone":"<rác>","Phone":"<nạn nhân>"}` vẫn khiến handler bcrypt tài khoản nạn nhân trong khi limiter đếm vào bucket của số rác — đã kiểm chứng qua router gin thật. Test mới đi kèm chọn đúng thứ tự khoá mà hai cách cài đặt trùng kết quả nên không bắt được lỗi. Điểm 8.5/10, dưới ngưỡng auto-approve 9.5.

---

# Cycle 3 — verify commit 1efaa0f

Phạm vi: `1efaa0f` (3 file: `ratelimit.go`, `ratelimit_test.go`, `service.go`). Chỉ đọc, không sửa file mã nguồn.

## Điểm cycle 3: 9/10

MAJOR-1 như đã mô tả ở cycle 1 và cycle 2 **đã đóng đúng cách**: thay vì cài lại quy tắc khớp field của `encoding/json`, code nay decode qua chính `encoding/json` vào struct dựng bằng `reflect.StructOf` mang đúng tag của field. Cả hai chiều khoá trùng đều cho cùng kết quả với DTO. NIT-10 cũng đã sửa. Chưa đạt 9.5 vì còn một đường vòng cùng loại, xuất phát từ chỗ `JSONBodyKey` dùng `json.Unmarshal` trong khi gin bind bằng `json.Decoder` — lỗi này **có sẵn trên master**, không do branch tạo ra, và sửa bằng một dòng.

## Trạng thái finding

| Finding | Trạng thái |
|---|---|
| MAJOR-1 — vòng qua limiter bằng cách viết hoa/thường và khoá trùng | **Đã sửa** |
| NIT-10 — comment doc của `activeProfile` bị đẩy sang `isUnauthorized` | **Đã sửa** (`service.go:306-315`, hai comment về đúng hàm của nó) |
| MAJOR-11 — `json.Unmarshal` vs `json.Decoder`: byte thừa sau JSON object | **Mới, chưa sửa** (có sẵn trên master) |

Tất cả finding của cycle 1 và cycle 2 tới đây đều đã đóng.

## MAJOR-1 — xác minh đã đóng

`apps/api/internal/middleware/ratelimit.go:125-150`. `probe` dựng một lần lúc tạo `KeyFunc` (không phải mỗi request), mỗi request `json.Unmarshal` vào `reflect.New(probe)` rồi đọc `Field(0).String()`. Đây là phương án 1 đã đề xuất ở cycle 2: quy tắc khớp field không còn được cài lại nên không thể lệch.

Chạy lại đúng ma trận body của cycle 2 qua router gin thật với code hiện tại:

```
{"PHONE":"a","phone":"b"}                 bucket="b"       OK
{"phone":"a","PHONE":"b"}                 bucket="b"       OK
{"phone":"junk","Phone":"victim"}         bucket="victim"  OK
{"phone":123,"Phone":"victim",...}        bucket=""        OK (handler cũng 400)
```

Hai thứ tự khoá trùng nay cùng ra `"b"`, khớp giá trị DTO bind. Trường hợp giá trị không phải string: limiter trả `""` và handler cũng trả 400, nên không có request nào chạy bcrypt mà không bị đếm.

Test `TestJSONBodyKeyMatchesFieldLikeStructBinding` nay phủ cả hai thứ tự và ca giá trị không phải string — không còn là test chọn đúng ca thuận lợi như ở cycle 2.

Điểm đáng ghi nhận: mọi call site đều bind vào field kiểu string (`auth/dto.go:54`, `invitations/dto.go:57,71`, `phone`), nên `probe` kiểu string đúng cho toàn bộ 5 chỗ mount limiter ở `router.go:138,139,275,276,285`.

## MAJOR-11 (mới) — byte thừa sau JSON object vẫn vòng qua được limiter

`apps/api/internal/middleware/ratelimit.go:146`.

`JSONBodyKey` dùng `json.Unmarshal(raw, …)`, hàm này **bắt buộc toàn bộ input phải là đúng một giá trị JSON** và trả lỗi khi còn byte thừa. Gin thì bind bằng `json.NewDecoder(r).Decode(obj)` (`binding/json.go`), decoder chỉ đọc **một** giá trị rồi dừng và bỏ qua phần còn lại. Repo không bật `EnableDecoderUseNumber` hay `EnableDecoderDisallowUnknownFields` nên đây là hành vi mặc định.

Hệ quả: chỉ cần nối thêm byte rác sau object, `json.Unmarshal` lỗi → `JSONBodyKey` trả `""` → `RateLimit` bỏ qua hoàn toàn, trong khi handler vẫn bind và xử lý bình thường. Xác minh qua router gin thật:

```
{"phone":"victim","password":"x"} trailing   status=200 handler="victim" bucket=""  BYPASS
{"phone":"victim","password":"x"}{}          status=200 handler="victim" bucket=""  BYPASS
{"phone":"victim","password":"x"}\n{"phone":"z"}  status=200 handler="victim" bucket=""  BYPASS
```

**Kịch bản hỏng.** Kẻ tấn công gửi `{"phone":"<số nạn nhân>","password":"..."}{}` — thêm đúng hai ký tự. Handler chạy bcrypt trên tài khoản nạn nhân, limiter không đếm gì cả. Trần 10/phút của login, 5/phút của forgot-password và các limiter theo `token` (reset-password 10/phút, invitations 20 và 10/phút) đều mất tác dụng. Đây cùng một lớp tấn công với MAJOR-1, chỉ khác cách kích hoạt.

**Không do branch này gây ra.** `git show master:…/ratelimit.go` cho thấy `json.Unmarshal` đã có sẵn trong `JSONBodyKey` trên master. Tuy vậy nó phá đúng tiêu chí "limiter thấy đúng thứ handler bind" mà commit này đặt ra, và phá Success Criteria của phase 3 ("lần login sai thứ 11 trong 1 phút cùng số → 429").

**Sửa: đổi một dòng** sang đúng đường decode của gin.

```go
if err := json.NewDecoder(bytes.NewReader(raw)).Decode(v.Interface()); err != nil {
    return ""
}
```

Đã kiểm chứng: với thay đổi này, cả ba body có byte thừa ở trên đều cho `bucket="victim"` khớp handler, và bốn ca khoá trùng/không-phải-string vẫn giữ nguyên kết quả đúng. Nên thêm test một body có byte thừa.

## Kết quả kiểm tra tự động (cycle 3)

| Lệnh | Kết quả |
|---|---|
| `make lint-api` | 0 issues |
| `cd apps/api && go vet ./...` | sạch (exit 0, không output) |
| `make test-api-unit` | không có FAIL; `middleware`, `auth`, `server` đều ok |
| `make api-docs` | không sinh diff |

`make test-api` vẫn không chạy theo yêu cầu, nên hai test tích hợp phase 1 chưa được chạy trong cả ba cycle. Commit này chỉ chạm file Go trong `apps/api`, không chạm `apps/web`, nên kết quả typecheck/lint/test của web ở cycle 2 (0 error, 5 warning đã biết, 639 pass / 3 skip) vẫn còn hiệu lực.

## Recommended Actions

1. Đổi `json.Unmarshal` thành `json.NewDecoder(bytes.NewReader(raw)).Decode(...)` trong `JSONBodyKey`, thêm test body có byte thừa (MAJOR-11). Một dòng code, đóng nốt lớp tấn công.
2. Chạy `make test-api` khi suite Docker rảnh.

Status: DONE_WITH_CONCERNS
Summary: MAJOR-1 đã đóng đúng cách bằng `reflect.StructOf` + `encoding/json` nên limiter và DTO khớp theo cấu trúc thay vì theo luật tự cài, NIT-10 cũng xong, toàn bộ finding của hai cycle trước đã đóng và mọi check tự động đều xanh.
Concerns/Blockers: Còn MAJOR-11 cùng lớp tấn công: `JSONBodyKey` dùng `json.Unmarshal` (đòi toàn bộ input là một giá trị) trong khi gin bind bằng `json.Decoder` (bỏ qua byte thừa), nên body `{"phone":"<nạn nhân>","password":"x"}{}` khiến handler chạy bcrypt mà limiter không đếm. Lỗi có sẵn trên master, không do branch tạo ra, sửa bằng một dòng. Điểm 9/10, dưới ngưỡng auto-approve 9.5.

---

# Xác nhận MAJOR-11 — commit 735965f (ngoài 3 cycle)

**Điểm cuối: 9.5/10 — đạt ngưỡng auto-approve, không còn finding nào mở.**

`apps/api/internal/middleware/ratelimit.go:145-150` nay decode bằng `json.NewDecoder(bytes.NewReader(raw)).Decode(v.Interface())`, đúng đường gin dùng trong `ShouldBindJSON`, kèm comment giải thích vì sao phải khớp. Chạy lại toàn bộ ma trận body của cycle 3 qua router gin thật với code hiện tại: **7/7 ca khớp, không còn MISMATCH**.

```
{"PHONE":"a","phone":"b"}                        bucket="b"       OK
{"phone":"a","PHONE":"b"}                        bucket="b"       OK
{"phone":"junk","Phone":"victim"}                bucket="victim"  OK
{"phone":"victim","password":"x"} trailing       bucket="victim"  OK  (trước: "")
{"phone":"victim","password":"x"}{}              bucket="victim"  OK  (trước: "")
{"phone":"victim","password":"x"}\n{"phone":"z"} bucket="victim"  OK  (trước: "")
{"phone":123,"Phone":"victim","password":"x"}    bucket=""        OK  (handler cũng 400)
```

Test bổ sung ca `{"phone":"0901234567"}{}` đúng trọng tâm lỗi. Kiểm tra: `make lint-api` 0 issues, `go vet ./...` sạch, `go test -count=1 -run 'TestJSONBodyKey|TestPhoneKey|TestRateLimit' ./internal/middleware/` pass (chạy cưỡng bức, không dùng cache), unit test `middleware`/`auth`/`server` xanh.

Status: DONE
Summary: MAJOR-11 đã đóng bằng đúng một dòng như đề xuất; limiter nay đi cùng đường decode với gin nên mọi body handler chấp nhận đều rơi vào bucket, xác minh 7/7 ca qua router thật và toàn bộ finding của ba cycle đã đóng.
Concerns/Blockers: Không còn finding mở. Việc còn lại nằm ngoài phạm vi review: chạy `make test-api` khi suite Docker rảnh, vì hai test tích hợp phase 1 chưa được chạy trong cả ba cycle.
