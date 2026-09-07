# Review kiến trúc tổng thể — Teka

- Ngày: 2026-09-05 22:08 (Asia/Saigon)
- Nhánh: `master` @ `57986c5`
- Phạm vi: không có diff → review kiến trúc toàn repo (`apps/api`, `apps/web`, infra, docs, conventions)
- Quy trình: `/code-review high` — 8 finder song song → 61 ứng viên → dedup ~45 → 11 verifier (1 phiếu/ứng viên) → bác bỏ 3, còn 42 CONFIRMED/PLAUSIBLE → cắt còn 10 nghiêm trọng nhất
- Bác bỏ: zalo `MatchFriends` (rule đã tài liệu hoá); race `Save` teachers (chặn bởi `ResolveScope status=active`); import không giới hạn dòng (có `MaxRowsPerSheet = 500`)

## Điểm mạnh đã xác minh

- Monorepo Go + React tách bạch, tổ chức theo feature nhất quán.
- Swagger sinh 100% từ annotation (125/125 route khớp, regenerate không diff).
- Mọi feature web đi qua `src/lib/api`; mọi route mount qua `router.tsx`.
- Id từ body/query đều được re-verify trong scope; refresh token có family revocation.
- Migration được test round-trip; event bus có hợp đồng at-most-once rõ ràng.

## Top 10 finding (tất cả CONFIRMED)

### 1. `billing`/`payments` `view_all` mở rộng đường ghi
- `apps/api/internal/features/billing/repository.go:296` (và `payments/repository.go:141`)
- `scoped()`/`invoiceScoped()` bỏ điều kiện `teacher_id` khi caller có khoá đọc `billing.view_all` / `payments.view_all`, nhưng mọi đường ghi tiền (LockPeriod, ClosePeriod, IssueDraftInvoices, VoidInvoice, adjustments, LockPayment, ResolveContactScope, recalc) đều resolve target qua chúng. Trái `catalog.go:335-342` ("scope keys widen visibility, never writes") và `docs/adding-permissions.md:118`.
- Kịch bản: owner cấp "Xem mọi hoá đơn" cho member M → M chốt kỳ của giáo viên T, void/adjust hoá đơn T, reverse payment T, ghi lại `paid_amount`. Service chỉ kiểm tra trạng thái, không kiểm tra chủ sở hữu.

### 2. `writeScoped` bỏ gate stint ACTIVE khi có `view_all`
- `apps/api/internal/features/sessions/repository.go:133` (cùng `classes/repository.go:87`, `:118`, `enrollments/repository.go:132`)
- Bỏ qua `classscope.WriteExists` khi caller có `<resource>.view_all` → khoá đọc cấp quyền ghi lên mọi lớp/buổi/ghi danh trong trung tâm. `attendance/service.go:280` kế thừa qua `sessions.GetWritable`. `students`/`contacts` đã rẽ nhánh đúng bằng `sc.WriteWide()`.
- Kịch bản: M có `sessions.view_all + attendance.confirm`, không stint trên lớp X → POST `/sessions/S/attendance`: ghi attendance toàn roster, `SoftDeleteMissing` xoá mềm phần còn lại, `MarkHeldAndConfirmed` chuyển S sang held → số buổi tính tiền bị nguỵ tạo.

### 3. `statements`/`notifications` `view_all` cho phép revoke, mint token, mark-sent
- `apps/api/internal/features/statements/repository.go:269` (Revoke `:583`, GetPeriodStatus `:314`; `notifications/repository.go:212`, MarkSent `:293`)
- Comment `:275-284` khẳng định writes không mở rộng là sai.
- Kịch bản: M revoke bảng kê của T → mọi link phụ huynh T trả 404. M mark-sent hàng queued của thư ký S → `ResumeRun` không còn gì gửi, phụ huynh không nhận tin.

### 4. `SessionMeta` lọc `teacher_id` làm reconciliation sau chốt sổ mất tiền im lặng
- `apps/api/internal/features/billing/repository.go:1013`
- Billing lọc `class_sessions.teacher_id = caller`, attendance cho phép mọi stint ACTIVE (giao_vien lẫn tro_giang); `ReassignPlanned` chỉ chuyển buổi planned từ hôm nay. `adjustment.go:189-191` nuốt `ErrSessionNotFound` thành nil.
- Kịch bản: trợ giảng (hoặc giáo viên mới sau handoff trên buổi cũ) sửa điểm danh buổi thuộc kỳ đã chốt → `SessionMeta` 0 rows → không tạo `invoice_adjustment`, `resp.Warning` nil → hoá đơn sai so với điểm danh, không ai được báo.

### 5. Refresh reuse không có grace window → 2 tab đăng xuất nhau
- `apps/api/internal/features/auth/service.go:248`
- Cookie `refresh_token` (Path=`/api/v1/auth`) dùng chung mọi tab; single-flight web (`interceptors.ts:22`, `session-restore.tsx:37`) chỉ in-memory per-tab.
- Kịch bản: 2 tab cùng refresh → tab thua gặp `ErrTokenAlreadyRevoked` → `RevokeFamily` xoá cả token vừa cấp cho tab thắng, 401 + Set-Cookie clear đè cookie mới → cả hai bị đăng xuất.

### 6. `io.ReadAll` body không giới hạn trên route public
- `apps/api/internal/middleware/ratelimit.go:119`
- Toàn server không có `http.MaxBytesReader` (chỉ `imports/handler.go:124` tự bọc); `server.go:19-26` không cap; nginx không proxy `/api`; Traefik labels không buffering/max body.
- Kịch bản: N kết nối POST `/auth/forgot-password` body hàng trăm MB → buffer toàn bộ vào RAM trước rate-limit/bind → API OOM.

### 7. `POST /auth/login` không rate limit, mọi nhánh chạy bcrypt cost 12
- `apps/api/internal/features/auth/routes.go:14`
- `router.go:130-132` chỉ bọc forgot/reset; nhánh thất bại vẫn chạy `burnPassword` với `dummyBcryptHash`.
- Kịch bản: 50 req/s → ~12 core-giây bcrypt/giây → container 1-2 vCPU bão hoà, p99 từ ms lên giây, không cần tài khoản hợp lệ.

### 8. Query cache sống sót qua mất phiên; key không chứa user/center id
- `apps/web/src/lib/api/interceptors.ts:37`
- Refresh thất bại chỉ `clearSession()`; `queryClient.clear()` duy nhất ở `useLogout` (`use-auth.ts:36`). Key: `['collections']`, `['dashboard']`, `['center','me']`, `['attendance','sessions']`, `['audit']`, `['reports']`; gcTime mặc định 5 phút.
- Kịch bản: giáo viên A hết phiên trên máy dùng chung → B đăng nhập cùng tab trong 5 phút → dashboard/danh sách học viên/audit của A render từ cache trước khi refetch bằng token B.

### 9. `StudentEnrolled` publish trong transaction trước commit
- `apps/api/internal/features/enrollments/service.go:84`
- Audit subscriber insert bằng `r.db` + `context.Background` nên không bị rollback. `imports.Service.Import` bọc nhiều Create trong một `WithinTx`; `apply.go:62-69` tiếp tục sau lỗi từng dòng rồi trả lỗi → rollback. `invitations/service.go:303-305` ghi rule ngược lại (publish sau commit).
- Kịch bản: import roster có 1 dòng lỗi cuối file → N hàng `audit_logs` `enrollment.create` trỏ tới id không tồn tại, 422 báo "không có gì được ghi". `apply.go:63` dựng anchor scope theo teacher của lớp → `ActorID` là giáo viên T chưa từng chạm file.

### 10. Mutation ghi danh không invalidate roster điểm danh
- `apps/web/src/features/roster/hooks/use-enrollments.ts:65`
- `invalidateEnrollmentSurfaces` (`:175-178`) và `useDeleteEnrollment` (`:207-209`) chỉ chạm key roster, không chạm `sessionsKeys.roster/detail`; server dựng sheet từ `roster.ActiveOn(classID, sessionDate)` và mặc định present; staleTime toàn cục 30s.
- Kịch bản: mở sheet hôm nay → ghi danh học sinh mới `started_on = hôm nay` → quay lại trong 30s → sheet thiếu em mới → xác nhận → backend ghi present và tính tiền dù giáo viên chưa thấy.

## Root cause hệ thống (finding 1-3)

Khoá `*.view_all` được catalog và docs định nghĩa là chỉ mở rộng **đọc**, nhưng 9 repository (billing, payments, statements, notifications, sessions, classes, enrollments; attendance kế thừa) dùng `CenterWideFor(view_all)` làm switch trong `scoped()`/`writeScoped()` mà mọi đường **ghi** đều đi qua. Chỉ `students` và `contacts` rẽ nhánh đúng trên `sc.WriteWide()`.

Hướng sửa: áp pattern `WriteWide()` như students cho từng repo; pin bằng test kiểu `TestViewAllWidensStudentReadsNotWrites` cho mỗi repo.

## Nguyên nhân sâu hơn (altitude, nên sửa để không tái phát)

- `authctx.Scope` vừa là identity caller vừa là row filter → 22 chỗ dựng Scope giả (`IsOwner:true`, `Perms` rỗng) để với tới hàng người khác; `imports/service.go:146-149` thừa nhận là cách vượt gate. Nên tách "row anchor" khỏi "caller scope".
- Tenancy scoping viết tay ở 19 hàm / 11 repo, chỉ bảo vệ bằng test grep token (`scoping_guard_test.go`) — không phát hiện query quên `scoped()`. Chưa có RLS (baseline chỉ ghi TODO).
- Thuộc tính route rải ở 3 bảng (`routePolicies`, `audit/action.go`, `request_events.go`); chỉ `routePolicies` có test hai chiều.
- `reports.send` là trục uỷ quyền thứ hai với 26 call site `ReportsOversight()` hard-code; route policy không hỗ trợ điều kiện tổ hợp.
- Map capability→role và permission key chép tay sang TS; `has(key: string)` không type-check.

## Ranh giới giữa feature (finding 4, 9)

Billing kiểm tra theo `teacher_id` của session, attendance theo class_staff stint → reconciliation sau chốt sổ mất tiền im lặng. Enrollments publish event trước commit → audit ghi hàng ma và sai actor khi import.

## Backbone / vận hành (finding 5-7 và ngoài top 10)

- Không cap body toàn server; login không rate limit dù bcrypt cost 12; refresh reuse không grace window.
- `SetTrustedProxies(nil)` sau Traefik → cột `ip` trong `audit_logs` vô giá trị.
- `X-Request-ID` inbound không cắt độ dài, ghi thẳng vào log + audit.
- WriteTimeout 30s cắt kết nối trong khi client import chờ 60s và transaction vẫn commit.
- Ownership "run" gửi tin nằm in-process; giả định single-replica được ghi chú nhưng compose không ngăn scale-out.

## Web state (finding 8, 10)

Query key không chứa user/center id, chỉ logout mới clear cache → rò dữ liệu giữa hai người dùng cùng tab. Thiếu invalidation chéo feature ở 3 chỗ (enrollments→attendance roster, reassign→classStaff, attendance→dashboard periodPreview/billing review) vì không có key factory dùng chung.

## Hiệu năng (ngoài top 10)

- `DraftPeriod` ghi từng hoá đơn/từng dòng (`preview.go:236-238`, `:255-257`).
- `TallyByEnrollment` và `ListPending` (cho owner) không có index cắt theo ngày phía center (`000003_widen_pending_sessions_index`).
- Route public `/public/statements/:token` và `/qr.png` không rate-limit, UPDATE `view_count` mỗi lượt xem.

## Cleanup đã xác nhận

- `pathID` copy nguyên văn ở 12 handler; `queryUUID` ×3; `queryDate` đã fork.
- `teacherLocation` ×2; 4 kiểu "today" (enrollments dùng giờ server, payments dùng UTC).
- Test harness `mintToken`/`fakeScopeResolver` ×12 với 5 biến thể.
- Web: hai `todayIso()` nghĩa ngược nhau + 9 inline UTC (người dùng UTC+7 trước 07:00 nhận ngày hôm qua).
- 18 route không có consumer; `shared/` có 4 component chết và một confirm-dialog trùng; `dashboard-layout.tsx` 643 dòng với hai bảng overflow phải đồng bộ tay.

## Conventions

- `ui/input.tsx|field.tsx|select.tsx` bị sửa tay (commit `72c8af1`) trái `apps/web/CLAUDE.md:20`.
- 6 commit trên master có trailer AI trái development-rules; không có hook chặn.
- 56 tham chiếu plan/phase/D-code trong comment code + migration; một cái lọt vào swagger public (`sessions/handler.go:338`).
- `apps/api/CLAUDE.md` mô tả layout sai với `handoff/` và `imports/`.
- `000019_drop_can_send_reports.down.sql` bị viết lại sau khi merge (PLAUSIBLE, trái rule không sửa migration đã có).

## Rủi ro cần chủ dự án quyết định

Link reset mật khẩu của member gửi qua Zalo của owner → owner có thể tự reset và đăng nhập thành member. Thiết kế có tài liệu nhưng phần risk chưa phân tích kịch bản này.

## Câu hỏi chưa giải quyết

1. `view_all` mở rộng writes ở billing/payments là cố ý theo `inventory.md:128-144` hay là lỗi theo `catalog.go:335-342` + `adding-permissions.md:118`? Hai tài liệu mâu thuẫn; cần chốt một hướng.
2. Traefik static config (ngoài repo) có forward `X-Forwarded-For` không — quyết định bật trusted proxies hay không.
3. Prod có bao giờ chạy >1 replica API / rolling deploy chồng lấn không (ảnh hưởng `RunManager`, session cache).
