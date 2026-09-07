---
phase: 3
title: "reports.send implied read keys"
status: completed
priority: P2
effort: "1d"
dependencies: [1, 2]
---

# Phase 3: reports.send implied read keys

## Overview

Gộp trục đọc của `reports.send` vào trục `*.view_all`: tại `ResolveScope`,
`reports.send` suy ra `billing.view_all`, `statements.view_all`,
`notifications.view_all`, `contacts.view_all` trong `Perms`. Read path đổi
`ReportsOversight()` → `CenterWideFor(...)`; `ReportsOversight()` chỉ còn là gate
gửi. 26 call site giảm về nhóm gate gửi.

## Requirements

- Functional:
  - Member có `reports.send` (không có view_all nào) đọc được **tập mới** = mọi
    surface mà 4 key suy ra mở: contacts + phone, billing periods/invoices
    (kể cả `ListPeriods` qua `scoped()` hôm nay không mở cho oversight),
    statements list/get/period figures (kể cả `scoped()` :267), notifications
    List (:210-216, hôm nay chỉ view_all) + runs, collections. Đây là widening
    đọc có chủ ý (D6), test parity assert tập mới, không phải "như cũ".
  - Member có `reports.send` **không** được thêm reach ghi nào (view_all suy ra
    là read-only theo phase 1).
  - Gate gửi (statements `AuthorizeClassSend`, `GenerateForSend*`; notifications
    `BulkSend`, `SendPreview`, run grant probe `CanSendReports`) không đổi.
  - `PhoneVisible(rowVisible) = CenterWideFor(contacts.view_all) || rowVisible`
    cho kết quả y hệt trước với mọi tổ hợp (owner, reports.send, contacts.view_all, hoc_vu stint).
- Non-functional:
  - Key suy ra không ghi vào DB; chỉ tồn tại trong `Scope.Perms` sau resolve.
  - `EffectiveKeys()` (trả cho web qua `centers/service.go:219/228`) chứa key suy ra; ghi rõ trong docs.
  - Deny override không gỡ được key suy ra (khớp hành vi hôm nay: oversight thấy hết bất kể deny). Ghi vào `docs/adding-permissions.md` mục "Implied keys không bị deny gỡ"; UI roles hiển thị là follow-up web, không đổi thiết kế.

## Architecture

```go
// authctx/catalog.go
// impliedBy maps a special key to the scope keys it implies for READS only.
var impliedKeys = map[string][]string{
    PermReportsSend: {PermBillingViewAll, PermStatementsViewAll, PermNotificationsViewAll, PermContactsViewAll},
}
func (p PermSet) WithImplied() PermSet // thêm implied sau BuildPermSet
```

`centers.Service.ResolveScope`: `perms := BuildPermSet(...).WithImplied()`.
Không đổi `BuildPermSet` để test hiện có của nó giữ nguyên.

Call site `ReportsOversight()` (26) phân loại:

| Nhóm | Vị trí | Hành động |
|---|---|---|
| Đọc billing/collections | `collections/repository.go:59,104,204,263,341,359,387,393,394`; `billing/repository.go:307` (`scopedRead`); `billing/service.go:121` | → `CenterWideFor(PermBillingViewAll)`; gộp `scopedRead` vào `readScoped` |
| Đọc statements | `statements/repository.go:290,318` (`scopedRead`, `GetPeriodStatusRead`); `statements/service.go:110` (nếu là đọc) | → `CenterWideFor(PermStatementsViewAll)` |
| Gate gửi statements | `statements/service.go:347` (`AuthorizeClassSend`) và `GenerateForSend*` | giữ `ReportsOversight()` |
| Đọc contacts/phone | `contacts/repository.go:87,102`; `authctx.go:77` `PhoneVisible`; `zalo/service.go:492` | → `CenterWideFor(PermContactsViewAll)` |
| Đọc notifications | `notifications/repository.go:226,286` (`runsPeriodScoped`, ListByPeriod), `:556` (ZaloMappings) | → `CenterWideFor(PermNotificationsViewAll)` |
| Gate gửi notifications | `notifications/service.go:129,399,500,677` (BulkSend/SendPreview/run) | giữ `ReportsOversight()`; xem lại từng dòng: dòng nào là đọc thì đổi |

Bảng trên là điểm khởi đầu; cook phải đọc từng dòng và phân loại đọc/gửi trước
khi đổi. Sau phase, `grep -rn 'ReportsOversight()' apps/api/internal` chỉ còn
định nghĩa + gate gửi.

## Related Code Files

- Modify: `apps/api/internal/shared/authctx/{catalog,permissions,authctx}.go`
- Modify: `apps/api/internal/features/centers/service.go` (ResolveScope)
- Modify: `apps/api/internal/features/collections/repository.go`
- Modify: `apps/api/internal/features/billing/{repository,service}.go`
- Modify: `apps/api/internal/features/statements/{repository,service}.go`
- Modify: `apps/api/internal/features/contacts/repository.go`, `zalo/service.go`
- Modify: `apps/api/internal/features/notifications/{repository,service}.go`
- Modify (test): `authctx` unit tests (WithImplied, PhoneVisible bảng chân trị), `centers/permissions_integration_test.go`, `server/policy_integration_test.go` (`TestPolicyHTTPViewAllParity` thêm biến thể reports.send-only), test collections/statements/notifications hiện có với scope `CanSendReports: true` phải được dựng qua `ResolveScope`/`testutil.ScopeFor` để có implied keys
- Modify: `docs/adding-permissions.md` (mục "Implied keys"), `docs/api-guidelines.md` (phone-privacy rule, Tenancy)
- Check: `apps/web/src/features/teaching/hooks/use-center-context.ts` và MSW fixtures — `permissions` có thêm key suy ra; không đổi code web trong plan này, chỉ xác nhận không có test web assert danh sách chính xác

## Implementation Steps

1. Thêm `impliedKeys` + `PermSet.WithImplied()` + unit test; nối vào `ResolveScope`.
2. Viết test parity đỏ→xanh: member reports.send-only đọc được từng surface trong tập mới ở Requirements (HTTP qua `policy_integration_test.go`), gồm cả ba surface mở thêm; không ghi được (Revoke/Close/MarkSent → 404/403).
3. Đổi `PhoneVisible`; test bảng chân trị 8 tổ hợp.
4. Đổi từng call site theo bảng phân loại; chạy test tương ứng sau mỗi feature.
5. Rà `testutil.ScopeFor` và các test dựng `Scope{CanSendReports: true}` bằng tay: chuyển sang `ScopeFor` (resolve thật) hoặc thêm `.WithImplied()`.
6. Grep xác nhận `ReportsOversight()` chỉ còn ở gate gửi; cập nhật doc comment của `ReportsOversight`.
7. Docs; `make test-api`, `make lint-api`.

## Success Criteria

- [x] `grep -rn 'ReportsOversight()' apps/api/internal --include='*.go' | grep -v _test.go` chỉ khớp định nghĩa và các gate gửi được liệt kê.
- [x] Test parity reports.send-only xanh ở HTTP layer cho contacts (kèm phone), billing (kể cả ListPeriods), statements (kể cả list qua `scoped`), notifications (kể cả List), collections.
- [x] Test negative: reports.send-only → `statements.revoke`, `billing.close`, `notifications.mark_sent` trên hàng người khác bị từ chối.
- [x] `PhoneVisible` bảng chân trị xanh; `zalo.MatchFriends` hành vi không đổi.
- [x] Web: `permissions` trả về có thêm 4 key khi member giữ reports.send; test web hiện có (vitest/MSW) xanh.

## Risk Assessment

- **Một read path bị xếp nhầm là gate gửi (hoặc ngược lại)** → member reports.send mất/đạt reach ngoài ý. Tín hiệu: test parity đỏ hoặc grep còn `ReportsOversight()` ở repo. Phản ứng: quay lại bảng phân loại, không nới implied sang `payments`.
- **Test cũ dựng Scope tay với `CanSendReports: true`** nhưng `Perms` rỗng → sau đổi sẽ mất reach đọc. Tín hiệu: test collections/statements đỏ hàng loạt. Phản ứng: dựng scope qua `ScopeFor`/`WithImplied` trong test, không thêm fallback `CanSendReports` vào read helper.
- **`MarkSent` đang bind `WriteWide()`** (`notifications/repository.go:292-304`, từ phase 1): sender chỉ có oversight update 0 hàng, không lỗi. Tín hiệu: test send-as-member xanh nhưng `status` vẫn `queued`. Phản ứng: mở gate gửi qua `ReportsOversight()` (trục send) hoặc assert `RowsAffected`; không mở qua key `view_all`.
- **Web hiển thị key suy ra ở màn quản lý quyền** làm owner tưởng đã cấp `*.view_all`. Tín hiệu: màn roles dùng `permissions` của context thay vì `knownKeysOf(role.Perms)`. Kiểm tra trước; nếu có, ghi follow-up web (ngoài scope) chứ không đổi thiết kế.

## Execution Notes

- Checkpoint kongming phase 2 (`plans/reports/kongming-phase2-checkpoint-260907.md`) sửa bảng phân loại:
  - `statements/service.go:347` là `resp.URL` (link public = vật phẩm gửi) → **giữ** `ReportsOversight()`; `AuthorizeClassSend` ở `:110`.
  - `contacts/repository.go:112` là `scopedMappingWrite` (nối lại Zalo = WRITE) → giữ trục gửi, không bao giờ `contacts.view_all`.
  - `notifications/repository.go:558` `ZaloMappings` lọc `contacts.teacher_id = sc.TeacherID` → sau migration 000016 luôn rỗng cho member (lỗi có sẵn) → re-key theo center + `CenterWideFor(contacts.view_all)` OR stint, bỏ arm teacher.
  - Quy tắc chọn key: `view_all` của resource mà ROUTE phục vụ read (collections → `billing.view_all`; `periodStatus` trong statements → `statements.view_all`). `SendPreview` là gate gửi.
  - `MarkSent`: giữ `writeScoped`, thêm kiểm tra `RowsAffected != len(ids)` → NotFound (không mở qua `view_all`).
  - Implied keys đặt **trong** `BuildPermSet` sau bước trừ deny (deny không gỡ được, đúng D6); `EffectiveKeys()` hưởng theo; thêm case vào `TestBuildPermSet`. Test dựng tay cần sửa: `authctx/phone_visibility_test.go` (thêm `Perms` có `reports.send`), `notifications/integration_test.go:509` (grant DB + `ScopeFor`).
  - Test then chốt: parity 3 principal ở HTTP cạnh `TestPolicyHTTPViewAllParity` — A = member chỉ `reports.send`, B = member đúng 4 `view_all` không `reports.send`, C = member thường. A ≡ B trên mọi GET (kể cả phone, `unallocated_credit`); A ≠ B đúng trên bulk send/preview/resume/statement `url`/PUT zalo mapping/mark-sent hàng người khác; C → 404.
- H-1 (open question 3) hotfix chạy song song trong `payments/` (slice riêng, không giao file với phase 3).
- Bước 1 (lead, TDD): `TestBuildPermSetImpliesReadKeysForReportsSend` đỏ → `impliedKeys` trong `catalog.go`, áp trong `BuildPermSet` sau bước deny → xanh. Không có `WithImplied()` riêng (theo counsel checkpoint: đặt trong `BuildPermSet` để mọi resolver và `EffectiveKeys()` hưởng theo).
- Bước 3 (lead, TDD): `TestPhoneVisible` bảng chân trị 8 tổ hợp + case `Scope{CanSendReports: true}` không có key → không thấy phone (trạng thái không resolve được) đỏ → `PhoneVisible = CenterWideFor(contacts.view_all) || rowVisible` → xanh.
- Bước 2, 4, 5, 6, 7 giao slice `phase3-reports-send-read-keys` (báo cáo `plans/reports/phase3-reports-send-read-keys-260907.md`).
- Slice phase 3 xong (`plans/reports/phase3-260907-0310-reports-send-implied-read-keys.md`):
  parity 3 principal `TestReportsSendImpliedKeysParity` ở `server/policy_integration_test.go`;
  `TestZaloMappingsFollowsCenterWideOrStint` phát hiện lỗi có sẵn (cột `id` mơ hồ
  trong nhánh stint của `ZaloMappings`, chưa từng có coverage) → sửa `contacts.id`;
  `MarkSent` trả 404 khi `RowsAffected != len(ids)` (swagger sinh lại, diff đúng
  endpoint đó). Grep `ReportsOversight()` còn định nghĩa + 7 gate gửi
  (statements :110/:347, contacts `scopedMappingWrite`, notifications :129/:398/:499/:678).
  Docs `adding-permissions.md` (mục implied key, đánh số lại 3→9) và
  `api-guidelines.md` cập nhật; sửa luôn hai chỗ docs cũ sai (route send-reports
  không tồn tại, cột `can_send_reports` đã drop ở migration 000019).
- Web (chỉ đọc): `use-center-context.ts` và MSW không assert danh sách chính xác.
  **Follow-up web (ngoài scope):** `member-permissions-dialog.tsx` badge "hiệu lực"
  chỉ tính key thô của role + override, không tính key suy ra → member có
  `reports.send` hiển thị "Không có" cho 4 `view_all` dù đọc được. Chỉ là hiển thị,
  API vẫn là nguồn enforcement.
- Tester (`plans/reports/tester-phase3-5-260907-0312-test-run.md`): full suite
  77.5% coverage, lint 0, targeted test xanh; **đỏ** ở guard
  `TestRepositoriesWidenWritesThroughWriteWideOnly` (12 chỗ `CenterWideFor` trong
  hàm không mang tên read ở collections/notifications). Lead sửa: helper
  `readWide(sc)` ở collections (cả package là read), `readWide`/`contactsReadWide`
  ở notifications; guard, integration collections + notifications, lint xanh lại.
  Tester đọc nhầm 7 hit `ReportsOversight()` còn lại là "refactor chưa xong" —
  đó là gate gửi theo thiết kế.
- Review phase 3 (`plans/reports/code-review-260907-phase-03-reports-send-and-payments.md`, 7/10):
  Critical (guard đỏ) đã sửa ở mục trên. High: 8 doc comment còn mô tả gate
  `ReportsOversight` đã gỡ (billing repository/service, statements repository,
  notifications repository `ListByPeriod`/`LatestRunByPeriod`/`ZaloMappings`/
  `writeScoped`) → viết lại theo `*.view_all` (suy ra từ `reports.send`).
  M1 (`unallocated_credit` widen theo `billing.view_all`) → giữ, ghi quyết định
  ở plan.md open question 7. M2 (nhánh stint của `ZaloMappings` không tới được
  qua route vì mọi caller đứng sau gate gửi) → giữ làm defence in depth, ghi rõ
  trong doc comment. M3 (`MarkSent` 404 nhầm khi id trùng) → đếm theo tập id
  duy nhất, pin thêm vào test idempotency ở `notifications/integration_test.go`.
  Sau sửa: unit features/notifications/billing/statements, integration
  notifications, lint 0 xanh.
- Grep `ReportsOversight()` sau sửa: mọi hit còn lại trong comment/code là gate
  gửi (statements service/dto/handler, notifications service/handler/dto,
  contacts `scopedMappingWrite`, zalo match-phones) — đúng thiết kế D6.
- Checkpoint kongming phase 3+5 (`plans/reports/kongming-phase3-5-checkpoint-260907.md`):
  GO — 7 hit `ReportsOversight()` còn lại đều là gate gửi, mọi `CenterWideFor`
  trong repository nằm trong hàm mang tên read, spot-check routespec khớp
  handler. Rủi ro kế tiếp: merge big-bang khi 3 quyết định owner còn mở (D9 kể
  cả reverse/reallocate chỉ owner; OQ6 students.Create; D6 widening đọc gồm
  `MatchFriendsScoped` giờ mở theo `contacts.view_all`). Checkpoint yêu cầu
  commit theo phase trước phase 4 (lần thứ hai); lead giữ cây chưa commit vì
  chưa có yêu cầu commit từ user, thay bằng snapshot patch ở scratchpad làm mốc
  rollback và nêu lại ở báo cáo cuối.
