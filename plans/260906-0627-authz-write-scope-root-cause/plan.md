---
title: "Authz write-scope: view_all chỉ mở rộng đọc, tách row-anchor khỏi Scope"
description: "Sửa 9 repository đang để *.view_all mở rộng ghi, rồi loại bỏ các nguyên nhân sâu: Scope kiêm row-anchor, guard tenancy chỉ là grep, 3 bảng thuộc tính route, trục reports.send tách rời"
status: completed
priority: P1
effort: "10d"
tags: [api, security, authz, tenancy, refactor]
created: 2026-09-06
source: plans/reports/review-260905-2208-architecture-review.md
---

# Authz write-scope: `view_all` chỉ mở rộng đọc, tách row-anchor khỏi Scope

## Overview

Review kiến trúc 2026-09-05 (`plans/reports/review-260905-2208-architecture-review.md`,
finding 1–3 + "Root cause hệ thống" + "Nguyên nhân sâu hơn") chỉ ra một lỗi
gốc duy nhất: các key phạm vi `<resource>.view_all` được catalog
(`authctx/catalog.go:335-343` `WriteWide`), `docs/adding-permissions.md:118`
và quyết định phase-08 của plan catalog định nghĩa là **chỉ mở rộng đọc**, nhưng
9 repository (sessions, classes, enrollments, billing, payments, statements,
notifications; attendance thừa hưởng qua `sessions.GetWritable`) vẫn gọi
`sc.CenterWideFor(<resource>.view_all)` trong `scoped()`/`writeScoped()` mà mọi
đường ghi đi qua. Chỉ students và contacts rẽ nhánh đúng trên `sc.WriteWide()`.
Hậu quả đã được 3 verifier độc lập xác nhận (CONFIRMED, không phải by design):
member được cấp một key "Xem mọi ..." kèm các key vận hành mặc định
(`DefaultRoleKeys()` cấp `billing.close`, `payments.reverse`,
`statements.revoke`, ...) có thể đóng kỳ, void hoá đơn, đảo thanh toán, thu hồi
statement, huỷ/xoá buổi, sửa/xoá lớp, kết thúc enrollment và xác nhận điểm danh
trên dữ liệu của giáo viên khác. Thêm vào đó migration `000018` đã backfill
`data.view_center_wide` legacy thành 12 dòng `*.view_all`, nên trên prod có thể
tồn tại member đang ghi center-wide ngoài ý định tài liệu.

Plan này (1) vá lỗi gốc theo pattern `WriteWide()` đã có và pin bằng test cho
từng repo, rồi (2) sửa các nguyên nhân sâu khiến lỗi tái phát: `authctx.Scope`
kiêm định danh người gọi và bộ lọc hàng (22 chỗ dựng Scope giả, gồm hack
`IsOwner: true` ở `imports/service.go:156`), guard tenancy chỉ là grep token,
`reports.send` là trục authorization thứ hai với 26 call site
`ReportsOversight()`, và thuộc tính route rải trên 3 bảng chỉ một bảng có test
hai chiều. Phạm vi: **backend** (`apps/api`). Counsel kongming (Q1–Q6) đã được
tích hợp vào các quyết định dưới đây.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Không còn đường ghi nào trong `apps/api` được mở rộng bởi `*.view_all`; mỗi repo có test pin `TestViewAllWidens<Resource>ReadsNotWrites` | P1 |
| 2 | Không thể dựng `authctx.Scope` giả để "hành động trên hàng của người khác": có type `authctx.Anchor` riêng, hack `IsOwner: true` ở imports bị loại bỏ, compiler chặn nhầm lẫn | P1 |
| 3 | `reports.send` không còn là trục đọc riêng: nó suy ra các key `*.view_all` đọc tương ứng ngay tại `ResolveScope`; `ReportsOversight()` chỉ còn là gate gửi | P2 |
| 4 | Query repository quên `scoped()`/`writeScoped()` bị bắt khi chạy `go vet`/test (linter go/analysis), không chỉ bởi grep token | P2 |
| 5 | Route policy, audit action và các map bỏ qua audit đọc từ một manifest duy nhất có test hai chiều với `engine.Routes()` | P2 |

## Decisions (accepted 2026-09-06)

- **D1 — `view_all` là read-only, không có ngoại lệ mới.** Bằng chứng: `catalog.go:335-343`, `docs/adding-permissions.md:116-118`, `docs/api-guidelines.md:128-130`, plan catalog phase-07:132-139 (students đã được sửa đúng cách này) và phase-08:47-50 (contacts). Ghi chú "Operation-vs-visibility invariant" ở `plans/260830-2310-.../inventory.md:128-144` là bản kiểm kê legacy đã bị các quyết định sau vượt qua; phase 1 gắn ghi chú superseded, không sửa lại lịch sử.
- **D2 — Sửa bằng pattern có sẵn, không thêm lớp mới.** Mọi helper mà write đi qua rẽ nhánh trên `sc.WriteWide()` (owner) hoặc `classscope.WriteExists` (stint ACTIVE); mọi helper đọc rẽ trên `CenterWideFor(<resource>.view_all)`. Billing và payments (47 lượt `CenterWideFor` đa số inline) gom về đúng hai helper mỗi bảng, đặt tên theo quy ước `readScoped`/`writeScoped` để guard AST kiểm được.
- **D3 — Materialise buổi từ schedule là "cache fill dữ liệu dẫn xuất", vẫn được phép dưới `sessions.view_all`.** `sessions/service.go:189` (`canGenerate := sc.CenterWideFor(PermSessionsViewAll)`) là INSERT nhưng hàng sinh ra mang `class.TeacherID` (service.go:149/309), nội dung hoàn toàn suy từ schedule, không có gì do người gọi tác giả; đổi sang `WriteWide()` sẽ làm đọc của view_all thiếu tuần chưa ai materialise. Giữ hành vi, ghi rõ trong comment tại chỗ và pin test "hàng do member view_all sinh ra giống hàng owner sinh ra". Đây là ngoại lệ duy nhất, nằm ở service (guard AST chỉ áp cho repository).
- **D4 — Không migration, có kiểm kê prod trước deploy.** Không thu hồi key nào; chỉ chạy truy vấn read-only kiểm kê ai đang giữ `*.view_all` (`center_role_permissions`, `center_member_permissions`) và member nào đã ghi trên billing/payments/statements 60 ngày qua (`audit_logs`). Có kết quả khác rỗng thì thông báo owner trung tâm trước khi deploy; mặc định chấp nhận mất reach ghi (đúng contract), owner gán lại stint nếu cần.
- **D5 — Row-anchor là type riêng `authctx.Anchor{TeacherID, CenterID}`**, dựng qua `sc.AnchorTo(teacherID)`; không có `IsOwner`/`Perms` nên không thể fake gate, compiler chặn truyền nhầm. Chỉ đổi chữ ký các repo method hiện nhận periodScope/paymentScope/runScope/ownerAnchor (~22 chỗ). Imports dùng `CreateAnchored` của contacts/students/enrollments thay hack `IsOwner: true`; enrollments nhận thêm actor thật để audit ghi đúng người import. Contacts/students là dữ liệu anchor owner nên `CreateAnchored` của hai service này chỉ nhận `authctx.OwnerAnchor` (mint duy nhất qua `centers.Service.ResolveOwnerAnchor`, guard grep cấm `MintOwnerAnchor(` ngoài `features/centers/`) để không service nào anchor danh bạ vào giáo viên tuỳ ý. **Quyết định sản phẩm đi kèm:** `billing.SessionMeta` tra buổi theo center (không theo caller) nghĩa là trợ giảng/giáo viên có stint attendance khi confirm buổi thuộc kỳ đã đóng của giáo viên khác sẽ tạo adjustment trên billing của người đó; đây là điều review finding 4 mô tả là hành vi mong muốn, không phải hệ quả kỹ thuật ngẫu nhiên.
- **D9 — Thanh toán là dữ liệu center; `ResolveContactScope` là lookup center-only, không phải write helper.** Contacts luôn anchor owner (`contacts/service.go:39-44`), nên nếu lookup này rẽ trên `WriteWide()` thì member không bao giờ ghi được thanh toán; hôm nay member ghi được là NHỜ `payments.view_all` (`payments/repository.go:381-399`), tức là escalation đang che một lỗ thiết kế. Sửa: lookup lọc `center_id` only, gate ghi là `payments.create` ở route (default key mọi system role), payment vẫn anchor trên owner của contact, allocation chạy với anchor đó (`CandidateInvoices`/`InvoicesByIDs`/`RecalcInvoicePaid` bind `WriteWide()=false` → own-rows-of-anchor, KHÔNG được đổi sang `sc` của caller). Đọc payments không đổi: member không có `payments.view_all` vẫn không thấy payment (kể cả payment mình vừa ghi) vì hàng anchor owner — quirk có sẵn, ghi nhận, không sửa ở đây. **Cần owner xác nhận** vì mở reach ghi cho member không có view_all (xem Open questions).
- **D6 — reports.send suy ra key đọc (option A, giới hạn).** Tại `ResolveScope`, `reports.send` inject `billing/statements/notifications/contacts.view_all` vào `Perms` (không payments: không có đường đọc payments nào dùng oversight). Read path đổi `ReportsOversight()` → `CenterWideFor(...)`; gate gửi giữ `CanSendReports`. Khi cook (2026-09-07) phát hiện thêm `zalo.MatchFriendsScoped` cũng chuyển sang `contacts.view_all` (trước đó gate oversight) — cùng loại widening đọc có chủ ý. Key suy ra xuất hiện trong `EffectiveKeys()` trả cho web (đúng sự thật: member đọc được). **Widening đọc có chủ ý:** member chỉ có `reports.send` sẽ thấy thêm những surface hôm nay chỉ `view_all` mở chứ oversight không mở — List notifications (`notifications/repository.go:210-216`), billing periods qua `scoped()` (:294) thay vì `scopedRead()` (:307), statements qua `scoped()` (:267) thay vì `scopedRead()` (:290). Chấp nhận vì read-only và nhất quán một trục; test parity phải assert tập mới này, không phải "như cũ". Deny override không gỡ được key suy ra (khớp hành vi oversight hôm nay), ghi trong docs. Không làm `PolicyAnyOf`.
- **D7 — Guard tenancy: linter go/analysis trước, RLS tách plan.** Linter bắt "method repo có Scope/Anchor mà không qua helper" và "`database.FromContext` ngoài helper". RLS (SET LOCAL app.center_id) có rủi ro thật với query ngoài `WithinTx` trả 0 hàng im lặng; cần spike đo trước, là plan riêng với go/no-go.
- **D8 — Manifest route hợp nhất thành package leaf `internal/shared/routespec`** (chỉ import authctx); `server.routePolicies`, `audit.actions`, 3 map trong `middleware/request_events.go` derive từ đó. Làm cuối vì giá trị là chống drift, không phải lỗ hổng.

## Non-goals

- RLS thật trong Postgres (plan riêng sau spike, xem D7).
- `PolicyAnyOf` / điều kiện tổ hợp trong route policy.
- Web: kiểu hoá `has(key: string)`, sinh TS từ catalog Go (nguyên nhân sâu #5, ngoài phạm vi backend).
- Finding 4/9 của review (billing tally vs stint attendance, publish event trước commit) trừ phần `SessionMeta` chạm trực tiếp khi tách Anchor (phase 2).
- Đổi UI/route contract; không thêm/bớt endpoint.

## Ánh xạ nguyên nhân sâu → phase

| Nguyên nhân sâu (review) | Phase |
|---|---|
| `view_all` mở rộng ghi ở 9 repo (finding 1–3) | 1 |
| `Scope` kiêm caller identity và row filter; 22 Scope giả; hack `IsOwner: true` | 2 |
| `reports.send` là trục thứ hai, 26 `ReportsOversight()` | 3 |
| Scoping tay ở 19 hàm/11 repo, guard chỉ grep, không RLS | 4 (linter); RLS = plan riêng |
| Thuộc tính route rải 3 bảng, chỉ 1 có test hai chiều | 5 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: view_all write-scope hotfix](./phase-01-view-all-write-scope-hotfix.md) | Completed (2026-09-06) |
| 2 | [Phase 2: Row-anchor type](./phase-02-row-anchor-type.md) | Completed (2026-09-07) |
| 3 | [Phase 3: reports.send implied read keys](./phase-03-reports-send-implied-read-keys.md) | Completed (2026-09-07) |
| 4 | [Phase 4: Repository scope linter](./phase-04-repository-scope-linter.md) | Completed (2026-09-07) |
| 5 | [Phase 5: Route spec manifest](./phase-05-route-spec-manifest.md) | Completed (2026-09-07) |

Phase 5 không phụ thuộc code của phase khác; làm cuối theo D8 nhưng có thể cook song song.

Thứ tự bắt buộc: 1 → 2 → 3 (3 đổi PhoneVisible/read helpers đã được 1 và 2 định
hình) → 4 (linter cần tên helper ổn định từ 1–3) → 5 (độc lập về code, làm cuối
theo D8). Phase 1 ship và deploy riêng như hotfix; các phase sau gom thành 1–2
PR.

## Rollout

Cập nhật 2026-09-07 (checkpoint cuối): cả 5 phase cook trên một cây, phase 1
không tách được khỏi hotfix H-1 và phase 2–3, nên merge **một lần** (5 commit
theo phase trên branch `authz-write-scope`, công thức ở
`plans/reports/kongming-final-checkpoint-260907.md` §3).

1. Trước merge: owner trả lời D9 (member có `payments.create` ghi thanh toán;
   chỉ owner Reverse/Reallocate; member không xem lại phiếu nếu thiếu
   `payments.view_all`) và OQ6 (`students.Create` owner-only hay giữ hành vi
   test) — thông điệp dán sẵn ở report §4. CI phải xanh trên branch (cây này
   chưa từng chạy CI).
2. Ngày deploy: chạy lại kiểm kê prod (D4: ai giữ `*.view_all`, member đã ghi
   center-wide 60 ngày) cộng hai truy vấn mới: member giữ `reports.send`
   (sẽ đọc rộng hơn theo D6) và payment còn dư chưa allocate mà contact có
   invoice thuộc kỳ của member (dữ liệu H-1 tồn đọng) → sau deploy owner chạy
   `POST /payments/:id/allocations/auto` cho các payment đó. Rollback = revert
   merge commit, không có migration.
3. Hành vi thấy được sau deploy: member mất reach ghi center-wide qua
   `*.view_all` (403/404); `permissions` trả về web có thêm key suy ra khi giữ
   `reports.send`; member reports.send-only đọc được thêm List
   notifications/billing periods/statements của người khác và
   `MatchFriendsScoped` (zalo) theo `contacts.view_all` (D6, read-only);
   `mark-sent` trả 404 khi có id ngoài tầm. Theo dõi: tỉ lệ 403/404 trên write
   của member, 404 `mark-sent`, payload `permissions`.
4. Phase 4–5 không đổi hành vi runtime; `scopelint` chạy trong `go test`
   (CI qua `make test-api`).

## Success Criteria

- [x] Với mỗi repo trong {sessions, classes, enrollments, billing, payments, statements, notifications, attendance}: test integration chứng minh member có `<resource>.view_all` (không stint) đọc được hàng của owner nhưng mọi write trên hàng đó → 404/403, và vẫn ghi được hàng của mình.
- [x] `grep -rn 'CenterWideFor' apps/api/internal/features/*/repository.go` chỉ khớp trong hàm có tên chứa `read`/`Read` (guard AST fail ngược lại).
- [x] `grep -rn 'IsOwner:\s*true' apps/api/internal --include='*.go' | grep -v _test.go | grep -v centers/` rỗng; `authctx.Scope{` có field chỉ còn trong `centers/`, `middleware/`, `testutil/`, test.
- [x] Import bởi member có `imports.run`: contacts/students được tạo anchored vào owner, audit `enrollment.create` ghi `actor_user_id` = người import.
- [x] Trợ giảng xác nhận điểm danh trên buổi có kỳ đã đóng → `ReconcileSession` tạo adjustment (không còn `ErrSessionNotFound` im lặng).
- [x] `ReportsOversight()` chỉ còn ở gate gửi (statements `AuthorizeClassSend`/`GenerateForSend*`, notifications `BulkSend`/`SendPreview`/run grant) và định nghĩa; mọi đường đọc dùng `CenterWideFor`. (kiểm 2026-09-07: còn lại ở BulkSend/SendPreview/ResumeRun/runGrant, AuthorizeClassSend, lộ URL statement trong ToResponse, và gate **ghi** zalo-mapping `scopedMappingWrite` — không có đường đọc nào.)
- [x] Linter go/analysis chạy trong `make test-api-unit` và CI; testdata chứng minh bắt được method repo thiếu helper và `database.FromContext` ngoài helper.
- [x] Một manifest `routespec` với test: hai chiều với `engine.Routes()`; mọi route mutating có audit source; `audit.actions` và các map bỏ qua trong `request_events.go` không còn là literal tay.
- [x] `make test-api` (coverage floor 60%), `make lint-api`, `make api-docs` không drift; không sửa migration cũ; không có plan ID/phase number trong comment code. (kiểm 2026-09-07: `make api-docs` tái sinh giống hệt cây; comment `D8`/`D4`/"phase 2" trong payments/statements/sessions/testutil có sẵn trên master từ plan trước — ngoài phạm vi, ghi follow-up.)
- [x] Docs cập nhật: `docs/api-guidelines.md` (Tenancy: read/write helper, Anchor, linter), `docs/adding-permissions.md` (implied keys, quy tắc helper), ghi chú superseded ở `inventory.md:128-144`.

## Related plans

- `plans/260830-2310-resource-action-rbac-permission-catalog` (completed): nguồn của `WriteWide`, phase-07/08 đã sửa students/contacts; plan này hoàn tất phần còn lại.
- `plans/260829-1640-gh-260829-flexible-center-rbac` (in-progress, chỉ còn e2e follow-up): mục "Decisions" còn nhắc `data.view_center_wide`/`Scope.CenterWide()` đã bị catalog plan thay thế; không có phụ thuộc chặn, không cần `blockedBy`/`blocks`.

## Open questions

Hai điểm sản phẩm cần owner xác nhận **trước deploy phase 1**, không chặn việc cook (1–2); các điểm kỹ thuật phát hiện khi cook phase 1 cần chốt trước khi phase 2/3 khoá thiết kế (3–5); mục 7 là quyết định đã ghi khi cook phase 3:

1. **D9 — ai được ghi thanh toán?** Mặc định kế hoạch: mọi member có `payments.create` (default key) ghi được thanh toán cho mọi contact của center, thay vì chỉ member có `payments.view_all` như hôm nay. Nếu owner muốn hạn chế hơn, phương án thay thế là bỏ `payments.create` khỏi `DefaultRoleKeys()` (cần migration nhỏ trên `center_role_permissions`) — ngoài scope plan này. Review phase 1 (2026-09-06) bổ sung: response của `Record` trả field invoice của owner (`student_name`, `total_due`, `paid_amount`) qua `ListAllocations(ctx, ownerScope, …)` (`payments/service.go:120`) cho member chỉ có `payments.create` — cùng quyết định sản phẩm này; lọc theo caller sẽ trả allocation rỗng cho chính người vừa ghi, nên giữ nguyên chờ owner chốt. Checkpoint phase 2 (2026-09-07) bổ sung hệ quả cần owner biết: payment/allocation vẫn anchor owner nên chỉ owner `Reverse`/`Reallocate` được (`LockPayment` đi qua `writeScoped`); member `payments.create` ghi được nhưng không đọc lại được qua GET nếu không có `payments.view_all`.
2. **D4 — member legacy giữ `*.view_all`:** nếu kiểm kê prod cho thấy member đang giữ `*.view_all` (backfill 000018) và
đã ghi center-wide, hành vi của họ thay đổi; kế hoạch mặc định là chấp nhận và
để owner gán stint/quyền lại.
3. **Candidacy invoice lọc theo `i.teacher_id` của anchor** (`payments/repository.go` `CandidateInvoices`, `RecalcInvoicePaid`): **lỗi có sẵn trên master, không phải regression** (review phase 2, 2026-09-07). Tiền đề cũ "contact anchor owner nên invoice cũng anchor owner" sai: contact anchor owner (`contacts/service.go`), nhưng kỳ anchor theo caller (`billing/service.go:57`) và invoice lấy `teacher_id` từ kỳ (`billing/preview.go:230`). Member có kỳ riêng ghi thanh toán cho contact anchor owner → `ResolveContactAnchor` trả owner, candidacy lọc `i.teacher_id = owner` bỏ sót invoice của member: auto-allocate không thấy hoá đơn, recalc cập nhật 0 dòng, im lặng. Phase 2 giữ nguyên bộ lọc (khoá vào `Anchor`) để không đổi hành vi. Đề xuất: candidacy + recalc theo `contact_id + center_id` (thanh toán là tiền cấp center của một contact), kèm test pin "kỳ của member + contact anchor owner + ghi thanh toán → allocation vào invoice member"; Checkpoint phase 2 (2026-09-07) chốt: **hotfix ngay, slice riêng trong `payments/` song song phase 3**, trước merge. Cách sửa: bỏ arm teacher, key `center_id + contact_id` (candidacy) / `center_id + id` (recalc, `InvoicesByIDs`), giữ `FOR UPDATE` + `ORDER BY`, chữ ký 3 method sang `sc` + helper `invoiceCenterScoped` (chỉ bind CenterID, không đọc perm); payment/allocation vẫn anchor owner; `assertLedgerInvariant` chuyển sang center-level. Test pin: contact+student owner, class/enrollment/session/attendance member, member Close kỳ → invoice teacher=member; Record bằng owner và bằng member chỉ có `payments.create` → 1 allocation, invoice paid; Reverse trả issued; Reallocate lên invoice đó. **Đã sửa (2026-09-07)** trong slice `plans/reports/slice-payments-candidacy-center-keyed-260907.md`: helper `invoiceCenterScoped` (chỉ bind `center_id`), ba lookup key theo center + contact/id, test `TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod` đỏ (0 candidate) → xanh; `assertLedgerInvariant` chuyển sang tổng theo center; payments unit + integration + lint xanh. Guard cross-contact ở Reallocate vẫn là chốt chặn duy nhất và còn test.
4. **`MarkSent` bind `WriteWide()`** (`notifications/repository.go:292-304`): sender chỉ có oversight (không owner) sẽ update 0 hàng mà không lỗi. Phase 3 phải mở gate gửi qua trục send (`ReportsOversight`) hoặc thêm kiểm tra `RowsAffected` — không được mở qua `view_all`. **Đã chốt (2026-09-07):** giữ `writeScoped`, đếm hàng sống với tới được qua `writeScoped` trước khi update, thiếu hàng nào → 404 (không mở qua `view_all`); id trùng trong một request đếm theo tập id duy nhất (review phase 3 M3), có pin ở `notifications/integration_test.go`.
7. **`unallocated_credit` trong route collections đọc bảng payments dưới `billing.view_all`** (review phase 3 M1, 2026-09-07): aggregate `unallocated_credit` (`collections/repository.go`) tổng hợp `payments`/`payment_allocations` nhưng widen theo `billing.view_all`, không phải `payments.view_all`. **Chốt:** giữ theo quy tắc "`view_all` của resource mà ROUTE phục vụ" (checkpoint phase 2): collections là surface đọc billing, số credit là một cột dẫn xuất của bảng thu, không phải danh sách payment; member có `billing.view_all` hoặc `reports.send` thấy tổng credit mà không đọc được từng payment. Không đổi.
5. **Key chết tới phase 3:** `statements.view_all` và `notifications.view_all` không mở thêm surface đọc nào sau phase 1 (read path còn rẽ trên `ReportsOversight`); pin test `TestViewAllWidensStatementReadsNotWrites` ghi rõ assertion List→404 sẽ lật khi phase 3 chuyển trục.
6. **`students.Create` không có gate `IsOwner` tường minh** (phát hiện khi cook phase 2, pre-existing): service chỉ chạy `checkContact` (contact phải đọc được bởi caller) rồi tạo học sinh anchor vào `sc.Self()`; route gate là perm `students.create`. Member có `ReportsOversight` (đọc được contact) và perm `students.create` có thể tạo học sinh anchor vào chính mình, trái với quy tắc sổ danh bạ (contacts/students anchor owner). Không đổi trong phase 2 (ngoài scope, không đổi hành vi); phase 3/4 chốt: ép `IsOwner` như contacts hoặc anchor về owner qua `ResolveOwnerAnchor`. **Xung đột bằng chứng** (2026-09-07): docs `api-guidelines.md` ("student record writes are owner-only", release note "members no longer create or edit contact and student records") nói owner-only, nhưng test hiện có cố ý khẳng định ngược lại (`students/integration_test.go:165` "contacts.view_all must let the member anchor to a visible contact… the student still belongs to its creator", `:282` "route policy (students.create) is what decides", `service_test.go:327`). Cần owner chọn: (a) owner-only như docs → thêm gate `IsOwner`, đổi 3 test, web không đổi (không có màn member thêm học sinh); (b) giữ hành vi test → sửa docs. Không tự quyết trong plan này.

<!-- slug: authz-write-scope-root-cause -->
