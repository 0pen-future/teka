---
phase: 2
title: "Row-anchor type"
status: completed
priority: P1
effort: "4d"
dependencies: [1]
---

# Phase 2: Row-anchor type

## Overview

Tách "hành động trên hàng của giáo viên X" khỏi "người gọi là ai" bằng type
`authctx.Anchor{TeacherID, CenterID}`. 22 chỗ hiện dựng `authctx.Scope{TeacherID: X, CenterID: Y}`
(Perms rỗng, `IsOwner` giả) chuyển sang Anchor; hack `IsOwner: true` ở imports
được thay bằng service method nhận Anchor; guard cấm dựng Scope có field ngoài
nơi resolve scope.

## Requirements

- Functional:
  - `authctx.Anchor` không có `IsOwner`/`Perms`/method widen; không chuyển đổi
    ngầm sang `Scope` (compiler chặn truyền nhầm hai chiều).
  - Import bởi member có `imports.run`: contacts/students được tạo anchored vào
    owner qua `authctx.OwnerAnchor` (mint duy nhất ở centers) mà không cần Scope
    giả; enrollments tạo với actor = người import (audit
    `enrollment.create.actor_user_id` đúng người thao tác), anchor = giáo viên
    lớp.
  - `billing.SessionMeta` tìm buổi theo (center, id) — người gọi đã qua gate ghi
    của attendance (caller duy nhất: `attendance/service.go:215` sau
    `GetWritable(CapAttendanceWrite)`) — và trả `Anchor` của buổi; trợ giảng
    confirm điểm danh tạo được adjustment trên billing của giáo viên lớp. Đây là
    quyết định sản phẩm ghi ở D5, không phải hệ quả kỹ thuật.
  - Không đổi hành vi owner, không đổi route/DTO/migration.
- Non-functional:
  - Guard (grep, tạm cho tới phase 4 chuyển sang type-based): ngoài
    `features/centers/`, `middleware/`, `testutil/`, `*_test.go` cấm (a)
    `authctx.Scope{` có field, (b) gán `\.IsOwner\s*=`, `\.Perms\s*=`,
    `\.CanSendReports\s*=` (chặn bypass `var s authctx.Scope; s.IsOwner = true`
    hoặc sửa bản copy), (c) import authctx dưới alias (chặn `ac.Scope{`), (d)
    `MintOwnerAnchor(` ngoài `features/centers/`.
  - Repo method nhận `Anchor` luôn lọc `center_id = a.CenterID AND teacher_id = a.TeacherID`
    (helper `anchored(ctx, a)`); không có nhánh widen.

## Architecture

```go
// authctx
type Anchor struct{ TeacherID, CenterID uuid.UUID }
// AnchorTo names whose rows a service acts on; CenterID is always the caller's.
func (s Scope) AnchorTo(teacherID uuid.UUID) Anchor
func (s Scope) Self() Anchor // = AnchorTo(s.TeacherID)
// OwnerAnchor is an Anchor proven to point at the center owner. Only
// centers.Service.ResolveOwnerAnchor may mint it (grep guard on MintOwnerAnchor).
type OwnerAnchor struct{ Anchor }
func MintOwnerAnchor(ownerID, centerID uuid.UUID) OwnerAnchor
```

Ba loại tham số repo sau phase này:

| Tham số | Ý nghĩa | Helper |
|---|---|---|
| `sc Scope` | người gọi; quyết định đọc/ghi được gì | `readScoped`/`writeScoped` (phase 1) |
| `a Anchor` | hàng của giáo viên nào (đã được service quyết định) | `anchored(ctx, a)` |
| `centerID` qua `sc` với helper `centerScoped(ctx, sc)` | dữ liệu center-level (contacts/students dedupe, `SessionMeta`) | `centerScoped` |

Call site chuyển sang Anchor (từ grep `authctx.Scope{` ngoài handler/test):
billing `close.go:164`, `preview.go:40`, `service.go:178`, `adjustment.go:241`,
`repository.go:983` (TallyByEnrollment), `:1024` (SessionMeta return, hàm bắt đầu :996);
payments `reversal.go:94/212/301`, `repository.go:399` (ResolveContactScope →
trả `Anchor`); statements `service.go:143/212/422/530/609`,
`public_handler.go:132`; notifications `service.go:222/635`,
`run_manager.go:118` (`runJob.scope()` → `anchor()`); imports `apply.go:120`
`anchorFor`, `service.go:156` `ownerAnchor`; centers `dashboard.go:83`
`targetScope`; enrollments `service.go:44` `ownScope` → `sc.Self()`. Handler
`return authctx.Scope{}, false` (zero value) được guard cho phép.

Repo method đổi chữ ký (ví dụ, danh sách đầy đủ do cook liệt kê từ call site):
billing `PeriodContainingDate`, `OpeningBalances`, `AdjustmentTotals`,
`CarriedDebtStudents`, `UpsertInvoice*`, `ZeroUnmatchedLines`, `StudentSnapshot`,
`TallyAttendance` meta; attendance `TallyByEnrollment(ctx, a Anchor, ...)`;
payments `CandidateInvoices`/`RecalcInvoicePaid`/allocation writes nhận Anchor
của payment owner; statements `UpsertStatement`, `InvoicesWithLines`,
`LiveSessions`, `Adjustments` (public render); notifications run snapshot
queries.

Imports: `contacts.Service.CreateAnchored(ctx, a OwnerAnchor, req)` và
`students.Service.CreateAnchored(ctx, a OwnerAnchor, req)` — không gate `IsOwner`
vì type đã chứng minh anchor là owner. `centers.Service.ResolveOwnerAnchor(ctx, sc)`
là nơi duy nhất mint (owner → chính mình; member → `CenterOwner`, imports đã có
sẵn ở `service.go:149-156`). `contacts.Create(ctx, sc, req)` giữ gate `IsOwner`
rồi gọi `CreateAnchored` với OwnerAnchor lấy từ centers.
`enrollments.Service.CreateAnchored(ctx, actor Scope, a Anchor, req)`: event
`StudentEnrolled.ActorID = actor.TeacherID`, hàng anchored vào `a`.
`FindIDByPhone`/`FindIDByName` nhận `centerScoped`.

`SessionMeta(ctx, sc Scope, sessionID) (..., Anchor, error)`: lọc
`class_sessions.center_id = sc.CenterID AND id = ?` qua `centerScoped`; doc
comment nêu rõ tiền điều kiện (caller đã settle gate ghi buổi). Mọi bước sau
trong `ReconcileSession` dùng Anchor trả về.

## Related Code Files

- Modify: `apps/api/internal/shared/authctx/authctx.go` (Anchor, AnchorTo, Self)
- Modify: `apps/api/internal/features/billing/{repository,service,close,preview,adjustment}.go`
- Modify: `apps/api/internal/features/attendance/{repository,service}.go` (TallyByEnrollment, ReconcileSession caller)
- Modify: `apps/api/internal/features/payments/{repository,service,reversal}.go`
- Modify: `apps/api/internal/features/statements/{repository,service,public_handler}.go`
- Modify: `apps/api/internal/features/notifications/{repository,service,run_manager}.go`
- Modify: `apps/api/internal/features/imports/{service,apply}.go`
- Modify: `apps/api/internal/features/contacts/{service,repository}.go`, `students/{service,repository}.go` (CreateAnchored, centerScoped dedupe)
- Modify: `apps/api/internal/features/enrollments/{service,repository}.go` (CreateAnchored, Self)
- Modify: `apps/api/internal/features/centers/dashboard.go`
- Modify: `apps/api/internal/features/scoping_guard_test.go` (thêm `TestScopeLiteralsOnlyWhereResolved`)
- Modify (test): `imports/*_test.go`, `attendance/integration_test.go`, `billing/*_test.go`, `audit` subscriber test cho actor
- Modify: `docs/api-guidelines.md` (Tenancy: mục "Scope vs Anchor")

## Implementation Steps

1. Thêm `Anchor`, `AnchorTo`, `Self` vào authctx với unit test (không chuyển đổi được sang Scope).
2. Thêm guard `TestScopeLiteralsOnlyWhereResolved` với bốn regex ở Requirements trên mọi `internal/**/*.go` non-test ngoài allowlist; test đỏ với 22 hit hiện tại (23 literal có field, 1 trong allowlist).
3. Billing: đổi các repo method nhận periodScope sang `Anchor` + helper `anchored`; sửa `SessionMeta` (centerScoped, trả Anchor); test trợ giảng confirm → adjustment.
4. Attendance `TallyByEnrollment(ctx, a Anchor, ...)`; cập nhật caller billing.
5. Payments: `ResolveContactScope` → `ResolveContactAnchor` trả `Anchor`; `paymentScope` → Anchor trong reversal/service; sửa comment `service.go:50-51` lần hai nếu cần.
6. Statements/notifications: periodScope/runScope → Anchor; `runJob.anchor()`.
7. `OwnerAnchor` + `centers.Service.ResolveOwnerAnchor`; imports: `CreateAnchored` ở contacts/students (OwnerAnchor) và enrollments (Anchor + actor); `anchorFor` trả Anchor; xoá `IsOwner: true`; test audit actor = importer, dữ liệu tạo ra giống owner import; unit test: `CreateAnchored` không nhận `Anchor` thường (kiểm bằng chữ ký, compile-time).
8. Centers dashboard `targetScope` → Anchor; enrollments `ownScope` → `sc.Self()`.
9. Guard xanh; `make test-api`; `make lint-api`; docs.

## Success Criteria

- [x] `grep -rn 'IsOwner:\s*true' apps/api/internal --include='*.go' | grep -v _test.go | grep -v features/centers/` rỗng.
- [x] `TestScopeLiteralsOnlyWhereResolved` xanh; thử thêm `authctx.Scope{TeacherID: x}`, `s.IsOwner = true`, hay `import ac ".../authctx"` vào một service → đỏ.
- [x] `grep -rn 'MintOwnerAnchor(' apps/api/internal --include='*.go' | grep -v _test.go | grep -v features/centers/ | grep -v shared/authctx/` rỗng.
- [x] Test imports: member có `imports.run` chạy import → contacts/students anchored owner, audit `enrollment.create` actor = member.
- [x] Test attendance/billing: trợ giảng (stint `tro_giang`) confirm buổi thuộc kỳ đã đóng → adjustment được tạo.
- [x] Không test hiện có nào đổi expectation cho owner.
- [x] `make test-api` xanh, coverage không giảm dưới floor.

## Risk Assessment

- **Signature churn lan rộng** (nhiều repo method). Tín hiệu: diff > ~40 file hoặc phải đổi handler. Phản ứng: giới hạn ở method hiện nhận anchor scope; phần còn lại giữ `Scope` (D5), không "Anchor hoá" toàn bộ.
- **Anchored query mất nhánh widen mà một owner-flow đang dựa vào** (VD statements public render dựng periodScope từ token, không phải caller). Tín hiệu: test public statements đỏ. Phản ứng: Anchor vẫn đúng ở đây vì token đã xác định giáo viên; sửa test/dữ liệu, không thêm widen.
- **`SessionMeta` centerScoped mở đường reconcile cho buổi không được phép** nếu một caller khác ngoài attendance gọi `ReconcileSession`. Tín hiệu: grep caller `ReconcileSession` > 1 ngoài test. Phản ứng: giữ tiền điều kiện trong doc và thêm gate ghi ở caller đó.
- **Import tạo enrollment với actor ≠ anchor làm audit consumer web hiểu sai** — kiểm tra màn audit chỉ hiển thị actor; chấp nhận vì đúng sự thật.

## Execution Notes

- **Quyết định thiết kế khi cook (2026-09-06, có kongming tư vấn —
  `plans/reports/kongming-phase2-design-260906.md`):**
  - Quy tắc chữ ký: *chữ ký theo caller*. Method mà mọi caller truyền giáo viên
    lấy từ hàng → đổi tại chỗ sang `Anchor` + `anchored(ctx, a)`. Method dùng
    lẫn (caller thật + anchor) là WRITE đã qua gate `*ForWrite`/`Lock*` → một
    method trên `Anchor`, caller thật truyền `sc.AnchorTo(row.TeacherID)`.
    Method dùng lẫn là READ phụ thuộc quyền hoặc entry point có gate riêng →
    `X(ctx, sc, …)` gate rồi gọi core `x(ctx, a, …)`; chỉ export `XAnchored`
    khi package khác cần. Không bao giờ thêm cầu nối `Anchor → Scope`.
  - Query theo lớp/center mà giáo viên không liên quan (roster của lớp cho
    reconcile: `enrollments.ActiveOnClass(ctx, sc, classID, on)`; dedupe
    contacts/students theo `centerScoped`) dùng loại tham số thứ ba, không
    Anchor hoá. `ActiveOnClass` là superset của `ActiveOn(fake)` (mất nhánh
    stint OR), nhánh dùng nó lọc theo `e.ID == sessionEnrollmentID` nên vô hại
    và đúng hơn sau khi bàn giao lớp.
  - **Dashboard: KHÔNG Anchor hoá** (bước 8 nửa dashboard bị điều khoản Risk
    thay thế). `targetScope` là "đọc như T đọc với tư cách member thường" —
    các read tiêu thụ (classes/sessions/enrollments/attendance) đều dựa stint,
    Anchor sẽ làm owner mất lớp T staff nhưng không sở hữu; view read-only, gate
    `dashboard.view`, không có bề mặt leo thang; cascade ~8 read method ở 4
    package. Giữ literal trong `centers/dashboard.go` (allowlist), đổi tên
    `viewAs`, doc comment nêu rõ vì sao là Scope không phải Anchor.
  - Cho phép literal `authctx.Anchor{…}` ở mọi nơi (Anchor không mang quyền);
    ưu tiên `sc.AnchorTo(x)` khi có `sc`. Không thêm `NewAnchor`.
  - Guard chuyển sang AST ngay ở phase này (file đã parse AST): bắt
    `CompositeLit` `authctx.Scope` có field và mọi `authctx.OwnerAnchor{`,
    gán `.IsOwner/.Perms/.CanSendReports`, import alias, `MintOwnerAnchor(`
    ngoài centers/authctx. Lỗ còn lại ghi cho phase 4: `s := sc; s.TeacherID = x`.
  - Payments `Record` đọc lại allocation qua `AllocationsOf(ctx, a Anchor,
    paymentID)` (chia sẻ `allocationRowSelect`), `ListAllocations(sc)` giữ cho
    Get/List. Open question 3 (candidacy theo contact+center) **không** gộp
    vào phase này; giữ `i.teacher_id = a.TeacherID`. **Tiền đề ghi lúc đầu
    ("contacts luôn anchor owner nên invoice cũng vậy") sai** — review phase 2
    chỉ ra invoice anchor theo giáo viên của kỳ (`billing/preview.go:230`), kỳ
    anchor theo caller; xem open question 3 (đã sửa) trong plan.md.
  - Chia việc: lead làm authctx + guard + docs; slice B (billing, attendance,
    sessions, enrollments) song song với C+D gộp (payments, statements,
    notifications; đồ thị import độc lập với B, gộp để giảm số container
    testcontainers dưới load cao); E (imports, classes, contacts, students,
    centers) sau B. Subagent chỉ test package của mình với `-p 1`, không chạy
    guard, không nới allowlist.
- Bước 1–2 xong: `anchor_test.go` xanh; guard AST đỏ đúng 22 site.
- Bước 3–9 xong (2026-09-07): 3 slice hoàn tất (báo cáo
  `plans/reports/slice-b-billing-anchor-260907-scope-to-anchor.md`,
  `slice-cd-payments-statements-notifications-260906.md`,
  `slice-e-imports-owner-anchor-260907.md`); docs `api-guidelines.md` mục
  "Scope vs Anchor". Xác minh: lint 0 issues, unit xanh, swagger không đổi,
  full suite 33/33 gói, coverage 76.4% (floor 60), guard đỏ đúng với 3 probe âm
  (`plans/reports/tester-phase2-260907-row-anchor.md`).
- Review (`plans/reports/code-review-260907-phase-02-row-anchor.md`, 8/10,
  không regression an ninh, public contract không đổi). Xử lý:
  - M-1 **đã sửa**: `attendance.ListBySessionAnchored` nhận Anchor nhưng chỉ
    lọc center+session — trái bất biến "Anchor luôn lọc center AND teacher".
    Đổi thành `ListBySessionCenter(ctx, sc, sessionID)` qua helper
    `centerScoped` (hạng mục center-keyed trong docs); `billing.SessionAttendance`
    nhận Scope của caller reconcile như `SessionMeta`. attendance + billing
    integration xanh.
  - M-2 **đã sửa**: guard quét từ module root (phủ `cmd/`, `seeds/`; `seeds/`
    exempt vì resolve từ cùng SQL membership), bắt literal lồng trong
    slice/map (`[]authctx.Scope{{…}}`), fail nếu không quét được file nào.
    Probe âm 2 hit rồi revert.
  - H-1 (có sẵn trên master, xác minh `ResolveContactScope` tại HEAD cũng lọc
    `i.teacher_id = contact.teacher_id`): candidacy/recalc bỏ sót invoice của
    member cho contact anchor owner. Không sửa trong phase 2; open question 3
    được viết lại với tiền đề đúng, chờ counsel chốt phase sửa.
  - M-3 ghi sang phase 4: `classes.CreateAnchored`/`AddScheduleAnchored`,
    `enrollments.CreateAnchored` exported nhận Anchor trần (caller duy nhất
    là imports, anchor resolve từ giáo viên trong workbook nên không thể ép
    `OwnerAnchor`) — cần allowlist call-site.
  - M-4: `ResolveContactAnchor` center-only là D9 (mặc định plan, chờ owner
    xác nhận trước deploy — open question 1); không đổi code.
  - L: `students.Create` không gate `IsOwner` tường minh → open question 6.
  - Tiêu chí "Anchor luôn lọc center AND teacher" giờ đúng với mọi method
    nhận Anchor; các query session/class-keyed đều nhận Scope + `centerScoped`.
- Checkpoint kongming (`plans/reports/kongming-phase2-checkpoint-260907.md`):
  **GO phase 3**. H-1 xác định là lỗi prod đang sống (kiểm kê: 2 member tạo
  27 kỳ → invoice member không bao giờ được allocate) → hotfix slice riêng
  trong `payments/` chạy song song phase 3, ghi ở open question 3. Đề xuất
  commit branch trước phase 3: không commit vì user chưa yêu cầu; thay bằng
  snapshot patch phase 1+2 trong scratchpad để tách commit sau. Đề xuất đóng
  gate `students.Create` ngay: **không áp dụng** vì test hiện có
  (`students/integration_test.go:165,282`, `service_test.go:327`) cố ý khẳng
  định member tạo được học sinh anchor vào mình — xung đột docs/test là
  quyết định sản phẩm, ghi open question 6.
