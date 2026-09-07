---
phase: 1
title: "view_all write-scope hotfix"
status: completed
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: view_all write-scope hotfix

## Overview

Đưa 7 repository còn lại (sessions, classes, enrollments, billing, payments,
statements, notifications; attendance thừa hưởng) về đúng contract của
`WriteWide()`: `*.view_all` chỉ mở rộng đọc, mọi write rẽ nhánh owner hoặc stint
ACTIVE. Pin bằng test per-repo, mở rộng guard để `CenterWideFor` chỉ được xuất
hiện trong helper đọc, và chuẩn bị kiểm kê prod trước deploy. Ship riêng như
hotfix, không migration.

## Requirements

- Functional:
  - Member có `<resource>.view_all` (không stint, không owner) đọc được hàng của
    giáo viên khác nhưng mọi write trên hàng đó trả 404 (repo không thấy hàng)
    hoặc 403 (service gate), giống students/contacts hôm nay.
  - Member vẫn ghi được hàng của chính mình và hàng của lớp có stint ACTIVE với
    role đúng capability (`classscope.WriteExists`).
  - Owner không đổi hành vi ở bất kỳ đường nào.
  - Materialise buổi (`ListRangeReadable`) giữ nguyên dưới `sessions.view_all`
    (D3: dữ liệu dẫn xuất, hàng mang `class.TeacherID`); pin test parity.
  - Member có `payments.create`, không có `payments.view_all`, ghi được thanh
    toán cho contact của center (D9); payment anchor trên owner của contact.
- Non-functional:
  - Không đổi route, DTO, migration, swagger.
  - Không có `CenterWideFor(` ngoài helper có tên chứa `read`/`Read` trong
    `*/repository.go` (guard AST).
  - Comment sai sự thật về scoping được sửa cùng chỗ (không nhắc plan/phase).

## Architecture

Quy ước helper trong mỗi repository (giữ tên hiện có khi đã đúng):

| Helper | Điều kiện widen | Dùng cho |
|---|---|---|
| `readScoped(ctx, sc)` / `scopedRead` | `sc.CenterWideFor(<resource>.view_all)` (+ `ReadExists*` nếu có) | List/Get/Tally/Meta phục vụ đọc |
| `writeScoped(ctx, sc[, roles])` | `sc.WriteWide()`; else `teacher_id = sc.TeacherID` hoặc `classscope.WriteExists(roles)` | Mọi UPDATE/DELETE/INSERT phụ thuộc, `SELECT ... FOR UPDATE`, lookup dẫn tới write |
| `oversightRead` (billing `scopedRead`, statements `scopedRead`, notifications `runsPeriodScoped`) | `ReportsOversight()` | giữ nguyên ở phase này; phase 3 gộp vào `CenterWideFor` |

Nguyên tắc phân loại: một query là **write** nếu kết quả của nó cấp quyền ghi
(`CandidateInvoices` chọn hoá đơn để allocate, `ResolveContactScope` mở đường
`Record`, `GetPeriodStatus` mở đường `generate`, `GetWritableByID`), không chỉ
khi nó chứa UPDATE.

Thay đổi theo repo (số dòng theo HEAD 57986c5):

1. **sessions** — `repository.go:131-138 writeScoped`: `!sc.CenterWideFor(PermSessionsViewAll)` → `!sc.WriteWide()`. `ReassignPlanned` (:292) đang qua `scoped()`: chuyển sang nhánh write (owner hoặc `teacher_id = caller`); xác nhận route handoff trong `routePolicies` là `owner_only` nên owner không đổi. `service.go:189` `canGenerate`: **giữ** `CenterWideFor(PermSessionsViewAll)` (D3), thêm comment tại chỗ giải thích đây là cache fill dữ liệu dẫn xuất (hàng mang `class.TeacherID`, không phải write của caller) và test: member view_all gọi `ListRangeReadable` tuần chưa materialise → hàng sinh ra bằng hàng owner sinh ra (teacher_id, planned_at, status).
2. **attendance** — không đổi repo (`service.go:280` đi qua `sessions.GetWritable(..., CapAttendanceWrite)`). Hệ quả tự đóng lỗ hổng "roster rỗng → `SoftDeleteMissing` xoá toàn bộ điểm danh rồi đánh held+confirmed 0 dòng" (verifier C2). Chỉ thêm test pin.
3. **classes** — `repository.go:118 writeScoped`: → `!sc.WriteWide()`. `scoped()` (:87) và `scopedSchedules` (:120) hiện được dùng cho write: `Archive` (:225), `SoftDelete` (:238), `ReassignTeacher`, `GetSchedule`→`Save`, `SoftDeleteSchedule`. Thêm `writeScopedSchedules` (WriteWide hoặc `classes.teacher_id = caller` hoặc `WriteExists(CapClassSettingsWrite...)` theo roles mà service resolve). `Service.Update` (:214-230) phải lấy hàng qua `GetWritableByID` trước `repo.Update` (`Save` không scope). Sửa comment `repository.go:99`.
4. **enrollments** — `repository.go:132 writeScoped`: → `!sc.WriteWide()`; sửa comment :126-130. `ClassDefaultPrice` (:272-290, gate của `Create`): thay `CenterWideFor(PermEnrollmentsViewAll)` bằng `WriteWide()`; giữ điều kiện stint `giao_vien` hiện có (không mở rộng scope sang capability map ở phase này).
5. **billing** — tách `scoped()` (:294-300) thành `readScoped` và `writeScoped`; `invoiceScoped` (:316-322) → `invoiceReadScoped`/`invoiceWriteScoped`. Write: `GetPeriod` khi được `Close`/`Draft`/`AddAdjustment`/`EnsurePeriod` dùng để mutate (tách `GetPeriodForWrite`), `LockPeriod` (:770), `ClosePeriod` (:818), `IssueDraftInvoices`, `VoidInvoices`, `LockInvoice`, `VoidInvoice`, `RecalcInvoiceTotals` (SQL `(? OR invoices.teacher_id = ?)` → bind `sc.WriteWide()`), `ZeroUnmatchedLines` (:731). Read: `ListPeriods`, `GetPeriodRead`, `ListInvoices` (:743), `TallyAttendance` meta (:539), `OpeningBalances` (:583), `AdjustmentTotals` (:625), `CarriedDebtStudents` (:655), `SessionMeta` (:996), `StudentSnapshot`. Các hàm nhận periodScope anchor giữ read-widening tạm (phase 2 đổi sang Anchor).
6. **payments** — `scoped()` (:139-145) và `allocationScoped` (:148-152) tách read/write. Write: `LockPayment` (:172), `MarkReversed`, `InvoicesByIDs FOR UPDATE` (:304), `RecalcInvoicePaid` (:377), `CandidateInvoices` (:229/266, SQL-OR bind `WriteWide()`). Ba hàm allocation này được gọi với `paymentScope` (anchor của contact, `IsOwner=false`) nên `WriteWide()` = own-rows-of-anchor: đúng và **không được** đổi sang `sc` của caller (sẽ gãy owner ghi thanh toán cho contact của member, xem `service.go:46-51`). `ResolveContactScope` (:381-399): **không** phải write helper — theo D9 đổi thành lookup `centerScoped` (lọc `center_id` only, bỏ nhánh `teacher_id`/view_all); gate ghi là route `payments.create`. Read: `ListPayments` (:187), `ListAllocations*` (:420/437). Sửa comment sai `service.go:50-51`.
7. **statements** — `scoped()` (:267-273, dùng bởi `Revoke` :583/:598) → `WriteWide()`. `GetPeriodStatus` (:313-315, chỉ backs standalone generate) → `WriteWide()`; `GetPeriodStatusRead` giữ oversight. Sửa comment :275-284 (nói rõ view_all không mở rộng Revoke/generate).
8. **notifications** — `scoped()` (:210-216) dùng cho `List` (đọc) và `MarkSent` (:291, UPDATE): tách `readScoped` cho List, `MarkSent` dùng `WriteWide()`. Sửa comment :207-209 (MarkSent không phải "plain read").

Guard: mở rộng `features/scoping_guard_test.go` bằng `go/parser`: với mỗi
`*/repository.go` (trừ centers), mọi `CallExpr` có selector `CenterWideFor` phải
nằm trong `FuncDecl` có tên khớp `(?i)read`. Giữ nguyên danh sách token bị cấm.

Kiểm kê prod (read-only, chạy qua `docker compose -p teka ... exec db psql`, không
in dữ liệu cá nhân vào chat):

```sql
-- Ai giữ key phạm vi qua role
SELECT c.name AS center, r.name AS role, p.permission_key
FROM center_role_permissions p
JOIN center_roles r ON r.id = p.role_id
JOIN centers c ON c.id = r.center_id
WHERE p.permission_key LIKE '%.view_all';
-- ... và qua override member
SELECT center_id, teacher_id, permission_key, effect
FROM center_member_permissions
WHERE permission_key LIKE '%.view_all';
-- Member đã ghi trên money resources 60 ngày qua
SELECT actor_user_id, action, count(*)
FROM audit_logs
WHERE actor_role <> 'owner'
  AND action LIKE ANY (ARRAY['billing.%','payments.%','statements.%','sessions.%','classes.%','enrollments.%'])
  AND occurred_at > now() - interval '60 days'
GROUP BY 1, 2;
```

Điều chỉnh tên cột theo `migrations/000013_center_rbac.up.sql` và
`000010_audit_logs.up.sql` khi chạy.

## Related Code Files

- Modify: `apps/api/internal/features/sessions/repository.go`, `sessions/service.go`
- Modify: `apps/api/internal/features/classes/repository.go`, `classes/service.go`
- Modify: `apps/api/internal/features/enrollments/repository.go`
- Modify: `apps/api/internal/features/billing/repository.go`, `billing/service.go`, `billing/close.go`, `billing/adjustment.go`, `billing/preview.go` (đổi tên helper ở call site)
- Modify: `apps/api/internal/features/payments/repository.go`, `payments/service.go`, `payments/reversal.go`
- Modify: `apps/api/internal/features/statements/repository.go`
- Modify: `apps/api/internal/features/notifications/repository.go`
- Modify: `apps/api/internal/features/scoping_guard_test.go`
- Create/Modify (test): `*/integration_test.go` của sessions, classes, enrollments, billing, payments, statements, notifications, attendance — thêm `TestViewAllWidens<Resource>ReadsNotWrites` theo mẫu `students/integration_test.go:474`
- Modify: `apps/api/internal/server/policy_integration_test.go` — thêm 2–3 case HTTP write bị từ chối dưới view_all (billing close, payments reverse, sessions cancel) và 1 case member `payments.create` không view_all ghi thanh toán thành công (D9)
- Modify: `docs/adding-permissions.md` (§ "Preserve data scope": quy ước helper đọc/ghi), `docs/api-guidelines.md` (Tenancy: bảng helper), `plans/260830-2310-resource-action-rbac-permission-catalog/inventory.md` (ghi chú superseded ở :128-144, trỏ về plan này)

## Implementation Steps

1. Viết test pin trước cho từng repo (đỏ): member với `Perms = BuildPermSet(nil, []string{Perm<Resource>ViewAll}, nil)`, không stint; assert đọc OK, write trên hàng owner → `ErrNotFound`/403, write hàng mình OK. Với attendance: `Confirm` trên buổi của owner → 404 và không có bản ghi điểm danh nào bị xoá.
2. Sửa sessions/classes/enrollments (đổi điều kiện `writeScoped`, `ReassignPlanned`, `writeScopedSchedules`, `Service.Update`; `canGenerate` chỉ thêm comment + test parity).
3. Tách helper read/write ở billing, payments; đi qua từng method theo bảng phân loại ở Architecture; đổi call site trong service/close/adjustment/preview/reversal.
4. Sửa statements (`scoped`, `GetPeriodStatus`) và notifications (`MarkSent`, `readScoped`).
5. Sửa comment sai ở 5 vị trí liệt kê; không đưa số phase/plan vào comment.
6. Mở rộng guard AST; chạy `go test ./internal/features -run TestRepositoriesScope`.
7. Thêm case HTTP write-denial vào `policy_integration_test.go`.
8. Cập nhật docs (adding-permissions, api-guidelines Tenancy) và ghi chú superseded trong inventory.md.
9. `make test-api-unit`, `make test-api`, `make lint-api`, `make api-docs` (không drift vì không đổi swag).
10. Trước deploy: chạy kiểm kê prod; ghi kết quả (đếm, không dữ liệu cá nhân) vào `plans/reports/`; nếu khác rỗng, thông báo owner theo D4.

## Success Criteria

- [x] 8 test `TestViewAllWidens<Resource>ReadsNotWrites` xanh; xoá tạm fix để xác nhận từng test đỏ đúng chỗ.
- [x] Guard AST đỏ khi chèn `sc.CenterWideFor(...)` vào một hàm write bất kỳ; xanh trên cây hiện tại sau fix.
- [x] Test parity materialise: hàng do member `sessions.view_all` sinh ra trùng hàng owner sinh ra; comment tại `canGenerate` nêu lý do giữ.
- [x] Test D9: member `payments.create` (không view_all) ghi thanh toán cho contact center → 201, payment `teacher_id` = owner; member không có `payments.create` → 403.
- [x] Owner: toàn bộ test integration hiện có xanh không sửa expectation.
- [x] `make test-api` qua coverage floor; `make lint-api` sạch.
- [x] Báo cáo kiểm kê prod tồn tại trước deploy; quyết định thông báo owner được ghi.

## Execution Notes (2026-09-06)

Sai lệch so với bảng Architecture, đều có lý do tại chỗ:

- **classes `Service.Update` giữ `GetByID`** thay vì `GetWritableByID`: catalog
  `class_staff.go` không có capability "class settings" nên không có role list
  để truyền; `scoped()` đã đổi sang `WriteWide()` nên `GetByID` chính là cổng
  write own-rows (Update/Archive/SoftDelete/ReassignTeacher đi chung). Không
  thêm `CapClassSettingsWrite` ở phase này (ngoài scope).
- **sessions: tách `materialiseRange`** khỏi `ListRange`. Sau khi
  `classes.writeScoped` chỉ mở cho owner, đường
  `ListRangeReadable → ListRange → classes.GetWritable` sẽ 404 cho member
  view_all; giờ `ListRangeReadable` tự resolve lớp qua read port + roles và gọi
  `materialiseRange(class)`; `ListRange` (route write) vẫn qua `GetWritable`.
  `classes.ListEffectiveSchedules` pre-check đổi sang `GetReadableByID`.
- **Mã trạng thái khi bị chặn**: sessions Cancel/Hold/Delete, attendance
  Confirm, enrollments End/Delete trả **403** (lớp nhìn thấy qua key nhưng
  không có role — đúng contract `GetWritable`), enrollments Create trả **422**
  (ref-validation `class_id`), classes/billing/payments/statements trả 404.
  Test pin ghi đúng từng mã.
- **billing `Service.GetPeriod` không nằm trên trục view_all** (nó dùng
  `GetPeriodRead`/ReportsOversight); test pin đọc qua `Preview`. Thêm
  `GetPeriodForWrite`/`GetInvoiceForWrite` vào interface `Repository` (fake
  repo trong `service_test.go`/`close_test.go` uỷ quyền về getter đọc).
- **Helper `readNarrow(q, sc, col)`** ở billing/payments cho các predicate
  inline; attendance (`readNarrowNames`, `readNarrowTally`) và students
  (`readNarrowContacts`, `scoped` → `readScopedByCreator`) tách tương tự để
  guard AST áp dụng được toàn cây.
- **notifications** không cần `readScoped`: `List` đã đi `ListByPeriod`
  (oversight); `scoped` → `writeScoped` cho `MarkSent`.
- **Guard** `TestRepositoriesWidenWritesThroughWriteWideOnly`: đã xác nhận đỏ
  bằng probe tạm (`lockThing` gọi `CenterWideFor`) rồi xoá probe.
- **Kiểm kê prod**: 0 member giữ `*.view_all` (role lẫn override); write
  non-owner 60 ngày chỉ là `billing.period.create/draft` trên kỳ của chính họ.
  Báo cáo: `plans/reports/inventory-260906-prod-view-all-write-exposure.md`.
  Không cần thông báo owner, không migration.
- **Test cũ `TestGenerateMasksPhoneForCenterWideReader` (statements) bị xoá**:
  nó dựa đúng vào lỗi vừa sửa (view_all mở `Generate` trên kỳ của giáo viên
  khác) để quan sát mask số điện thoại. Sau fix, không còn đường nào để một
  caller không stint/không oversight chạm hàng statement của kỳ người khác
  (Generate 404, List 404 vì đọc theo trục oversight), nên hình dạng rò rỉ đó
  không thể tái hiện; ý định "không reach" được ghim trong
  `TestViewAllWidensStatementReadsNotWrites` (thêm assertion List 404).
- **Checkpoint cố vấn bắt được một read bị thu hẹp nhầm**: `sessions.ListPending`
  (feed `GET /sessions/pending`, cũng là predicate của cổng đóng kỳ billing)
  đi qua `scoped` nên mất reach `sessions.view_all`. Sửa bằng helper
  `readScopedFeed` (own rows + view_all, cố ý không có nhánh stint để cổng
  đóng kỳ anchored trên giáo viên của kỳ không bị chặn bởi lớp họ chỉ làm
  staff); pin test sessions thêm 2 assertion (view_all thấy feed; stint đơn
  thuần không thấy). Guard `repositoryPaths` mở rộng sang mọi file non-test
  có receiver `*gormRepository` (trước đó `sessions/pending.go` nằm ngoài
  glob `*/repository.go`).

## Risk Assessment

- **Regression thấy được cho member legacy** (backfill 000018): họ mất reach ghi center-wide. Tín hiệu: kiểm kê prod khác rỗng hoặc ticket "không đóng được kỳ". Phản ứng: đã quyết ở D4 — thông báo owner, owner gán stint; không rollback vì đây là đóng escalation.
- **Phân loại nhầm read/write ở billing/payments** làm owner-flow gãy (VD `GetPeriod` cho Close dùng helper read). Tín hiệu: test integration billing/payments hiện có đỏ. Phản ứng: sửa phân loại, không nới `WriteWide`.
- **`ReassignPlanned` nếu route handoff không phải owner_only** thì nhánh `teacher_id = caller` chặn member được giao. Tín hiệu: `TestRoutePolicy*` cho thấy kind khác. Phản ứng: dùng `classscope.WriteExists` với roles do handoff service resolve.
- **Trợ giảng/giáo viên mới không tạo được adjustment khi confirm** (`SessionMeta` còn lọc theo caller) — lỗi có sẵn, không do phase này; sửa ở phase 2. Không mở rộng `SessionMeta` bằng view_all ở đây.
- **D9 mở reach ghi thanh toán cho member không view_all.** Tín hiệu: owner phản hồi không muốn hoc_vu/giao_vien ghi thanh toán. Phản ứng đã định: giữ code phase này, bỏ `payments.create` khỏi default keys bằng plan riêng; không quay lại rẽ trên view_all.
- **Chốt phase 1 (2026-09-06):** `make lint-api` 0 issue; `GOFLAGS=-p=2 make test-api`
  37/37 package xanh, coverage 76.4% (sàn 60%); `make api-docs` không diff.
  Reviewer 8/10, 0 critical → auto-approve kèm cảnh báo; tester DONE; kongming GO.
  Follow-up đã làm: pin tầng repo `TestTargetContactsRowsByAnchorPhoneByViewer`
  (hàng theo anchor, bit phone theo stint hoc_vu của viewer, widening owner/
  oversight nằm ở `Scope.PhoneVisible`); `seedChild` trả `*classes.Class`;
  ghi chú trên assertion List→404 rằng phase 3 sẽ lật nó; sửa bảng markdown
  hỏng ở `plans/260830-2310-resource-action-rbac-permission-catalog/inventory.md`;
  Open questions 3–5 và bổ sung câu 1 trong `plan.md`. Bỏ qua pin grading vì
  `AssignScoreSet`/score sets đã gate `IsOwner` tường minh và
  `TestSessionScoreWriteAuthorizationAndValidation` đã phủ ghi điểm.
