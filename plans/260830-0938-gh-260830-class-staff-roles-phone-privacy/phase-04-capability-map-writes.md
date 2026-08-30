---
phase: 4
title: "Writes theo capability map"
status: completed
priority: P1
effort: "4d"
dependencies: [2, 3]
---

# Phase 4: Writes theo capability map

## Overview

Chuyển các đường GHI artifacts của lớp từ creator-anchor (`teacher_id = $self`
/ session-teacher) sang assignment ACTIVE + capability map
(`authctx.StaffRolesFor`). GV được gán (kể cả sau handoff) full write trên
artifacts lớp; GV cũ mất write (chỉ còn đọc); trợ giảng ghi điểm danh không
cần duyệt; học vụ thuần đọc + gửi statement per class. Audit các feature còn
giả định GV-owns-rows.

## Requirements

- Functional: theo capability matrix v1 (plan.md / brainstorm):
  attendance.write = GV + trợ giảng; scores.write, remarks.write,
  lesson_plan.write, enrollment.write (end/delete — create đã mở P3) = GV;
  statement.send per class = học vụ; owner mọi thứ.
- Functional: hoá đơn đếm ĐỦ điểm danh bất kể ai ghi (red-team F4: tally
  own-rows làm invoice thiếu tiền khi trợ giảng điểm danh).
- Non-functional: write bằng role không có capability → 403; không assignment
  → 404/empty (không leak tồn tại lớp); mọi write path có test 5 vai; gate cũ
  bị THAY THẾ, không OR thêm.

## Architecture

**Fragment ghi** — bổ sung `classscope.WriteExists(classIDExpr, roles []string)`:

```sql
EXISTS (SELECT 1 FROM class_staff cs
        JOIN classes c2 ON c2.id = cs.class_id AND c2.deleted_at IS NULL
        WHERE cs.class_id = <classIDExpr>
        AND cs.teacher_id = ? AND cs.center_id = ?
        AND cs.ended_at IS NULL AND cs.role_key = ANY(?))
```

Service tra `authctx.StaffRolesFor(cap)` rồi truyền role slice xuống repo
helper (`writeScoped(ctx, sc, roles)`) — repo không đọc capability map (giữ
scoping guard: repo chỉ nhận tham số, branch duy nhất trên `CenterWide()`).

**Semantics 403 vs 404** (P1 đã áp cùng luật cho classstaff): caller CÓ
assignment (kể cả ended) trên lớp nhưng thiếu capability → 403 honest. Không
có assignment nào → 404/empty.

**Nguyên tắc THAY THẾ, không OR (red-team F10):** gate cũ theo
`session.TeacherID` / `<table>.teacher_id = $self` bị GỠ và thay bằng
WriteExists — không giữ nhánh OR "cho an toàn". Giữ OR nghĩa là GV cũ vẫn
`session.TeacherID == self` trên mọi buổi ĐÃ DIỄN RA (handoff chỉ dời session
planned tương lai — handoff/service.go:139–146) → sửa điểm lịch sử được mãi.
Hệ quả có chủ đích (ghi thành decision D8 plan.md): GV MỚI sửa được điểm danh/
điểm/nhận xét của buổi TRƯỚC handoff (trước đây session-teacher chặn).

**Điểm chuyển đổi:**

| Write path | Gate hiện tại (đã verify) | Capability |
|---|---|---|
| attendance upsert/confirm | own-rows `scoped` (repository.go:80–86) — nhưng xem bảng method dưới | attendance.write (GV, trợ giảng) |
| grading UpsertScores | **session-teacher** `session.TeacherID != sc.TeacherID && !sc.IsOwner` (grading/service.go:318–324 — plan cũ ghi nhầm "class-teacher") | scores.write (GV) — thay thế |
| teaching lesson plans create/update/submit | teacher_id=self (+ session-teacher teaching/service.go:703) | lesson_plan.write (GV); owner review loop giữ nguyên |
| teaching session notes + marks (nhận xét) | teacher_id=self / session-teacher | remarks.write (GV) — thay thế |
| sessions lifecycle: hold/cancel/generate/pending confirm | teacher_id=self qua classes.Get port cũ | capability riêng `sessions.write` (GV) thêm vào map |
| enrollments end/delete | creator/owner | enrollment.write (GV active của lớp) + owner |
| statements list/detail cho học vụ + gửi | `ReportsOversight()` center-wide | nhánh học vụ per-class — kèm TARGET FILTER (dưới) |

**Attendance — tách "row filter" vs "permission check" từng method (red-team
F4 Critical; plan cũ chỉ nói đổi 'điều kiện được ghi' trong khi `teacher_id =
$self` còn là bộ lọc HÀNG):**

| Method | Hiện tại | Đổi thành |
|---|---|---|
| `UpsertMany` (repository.go:88–112) | KHÔNG scope; `DoUpdates` không set teacher_id | permission = WriteExists trên class; update SET `teacher_id = $self` (last-writer attribution — trợ giảng sửa row GV thì ghi công trợ giảng) |
| `SoftDeleteMissing` (:122–129) | row filter own-rows `scoped()` → không xoá nổi row người khác tạo → row billable mồ côi | row filter theo (session, class) + permission WriteExists — xoá được row mọi creator trong lớp |
| `ListBySession` / `StudentNames` | own-rows | theo class (readScoped P2) |
| `TallyByEnrollment` (:168, nguồn tính tiền `billing/service.go:157` chạy scope CALLER) | own-rows `scoped()` → **invoice thiếu buổi trợ giảng ghi** | theo enrollment/class trong center — KHÔNG lọc `attendance_records.teacher_id` |

Bất biến "writes stamp `teacher_id = $self`" GIỮ cho attribution (cả insert
lẫn update path); nó KHÔNG còn là điều kiện lọc hàng ở bất kỳ đường
đọc-để-tính-tiền hay đường xoá nào.

**Học vụ statement = BẢN THEO LỚP (D1 sửa tại validation) — migration
`000017_class_scoped_statements_runs`:**

- `statements` thêm `class_id UUID NULL` + composite FK `(class_id,
  center_id)` → classes. `class_id IS NULL` = statement gia đình (đơn vị
  hiện tại, owner/oversight — KHÔNG đổi hành vi); `class_id IS NOT NULL` =
  bản theo lớp cho đường học vụ. Unique bản lớp: `(contact_id, period,
  class_id)` partial `WHERE class_id IS NOT NULL` (song song với unique gia
  đình hiện có). Down: xoá rows class-scoped + drop cột.
- Generate bản lớp: chỉ invoice lines của lớp đó cho contact có student ghi
  danh active trong lớp; tổng = subtotal lớp. Token/URL (token.go:16–20 HMAC
  tất định) đưa `class_id` vào input derive — URL bản lớp chỉ mở nội dung bản
  lớp; học vụ ĐƯỢC nhận URL bản lớp (không bao giờ nhận URL bản gia đình —
  P3).
- Học vụ list/detail/preview/send: LUÔN đi đường class-scoped theo lớp gán;
  gia đình nhiều lớp → mỗi lớp một bản, phụ huynh có thể nhận nhiều tin
  (quyết định user, validation Q7). Owner/oversight giữ nguyên đường gia đình.
- Chồng lấn gia đình↔lớp cùng kỳ (oversight gửi bản gia đình + học vụ gửi bản
  lớp = phụ huynh nhận 2 tin có phần trùng): KHÔNG hard-block — UI cảnh báo
  khi kỳ đã có bản kia được gửi, ghi docs. Số liệu công nợ vẫn một nguồn
  (invoice lines) nên không lệch tiền, chỉ trùng tin nhắn.

**Notification run per-class (D9 — validation đảo khuyến nghị giữ-409):**
cùng migration 000017, `notification_runs` thêm `class_id UUID NULL`; thay
`uq_notification_runs_one_active_period` (000012) bằng 2 partial unique:
active per period `WHERE class_id IS NULL` (center-wide, giữ semantics cũ) và
active per `(period, class_id)` `WHERE class_id IS NOT NULL`. Hai học vụ 2 lớp
cùng kỳ gửi song song không đâm nhau; run học vụ stamp `class_id` = lớp gán.
Boot reconciler xử lý mọi run kẹt như cũ (per run, không phụ thuộc index).

**Học vụ send — gate + TARGET FILTER + kênh (red-team F5
Critical: gate không đủ — `TargetContacts` lọc `center+teacher+period`, không
có chiều class (statements/repository.go:274–284, period là per-teacher
000001:271); thiếu filter là học vụ 1 lớp preview được phone+URL TOÀN
CENTER):**

- `TargetContacts` nhận thêm tập contact visible: contact có student ghi danh
  active trong lớp caller được gán `hoc_vu` (JOIN trong SQL, không load-all).
  Preview/bulk-send/resume/list statement dùng CÙNG filter.
- `ZaloMappings` (notifications/repository.go:422–444) hiện widen center-wide
  CHỈ khi `ReportsOversight()` — mở rộng: widen theo tập contact visible của
  học vụ (sau 000016 mọi contact anchor owner, không widen là resolve 0
  mapping → fallback manual im lặng, acceptance 5 fail âm thầm). Test phải
  assert channel `zalo_personal`, không chỉ HTTP 200.
- Run: theo D9 ở trên — run học vụ mang `class_id`, song song per lớp,
  không còn 409 giữa 2 học vụ khác lớp cùng kỳ.
- Sender attribution: rows ghi công học vụ (pattern thư ký: sender = caller,
  gửi từ zalo của caller — P3 đã mở đường map zalo cho học vụ).
  `reports.send` center-wide giữ nguyên song song.

**Audit GV-owns-rows** (làm trong phase này, sửa nếu lệch):

- dashboard: `apps/api/internal/features/centers/dashboard.go` (không phải
  package riêng) — P2 đã chốt giữ own-rows port; ở đây chỉ xác nhận số liệu
  không đổi sau khi write path đổi.
- notifications ledger/list own-rows (`runsOwnScoped`
  notifications/repository.go:203) — giữ (ledger là "đợt tôi gửi").
- imports điểm danh/điểm (nếu tồn tại đường import này) theo capability map.
- billing/payments/collections: owner + oversight domain — không mở cho GV;
  xác nhận không write nào dùng `classes.teacher_id`; tally đã xử lý ở bảng
  attendance trên.

**Web**: gate nút write theo `my_staff_roles` (P2 đã có): attendance edit cho
GV + trợ giảng, điểm/nhận xét/giáo án chỉ GV, tab thống kê/statement + nút gửi
cho học vụ. UI học vụ: trang lớp read-only + action "Gửi báo cáo".

## Related Code Files

- Create: `apps/api/migrations/000017_class_scoped_statements_runs.{up,down}.sql`
  (+ migrations_test.go case up/down)
- Modify: `apps/api/internal/shared/classscope/classscope.go` (WriteExists),
  `apps/api/internal/shared/authctx/class_staff.go` (thêm `sessions.write`)
- Modify: `apps/api/internal/features/statements/{token,service,repository}.go`
  (class-scoped variant + token derive theo class_id)
- Modify: `apps/api/internal/features/{attendance,grading,teaching,sessions,enrollments,statements,notifications}/{repository,service}.go` + integration tests
- Modify: `apps/api/internal/features/billing/…` (chỉ test: invoice đếm đủ
  buổi trợ giảng ghi)
- Modify: `apps/web/src/features/teaching` classbook (attendance/score/remark gates), statements UI học vụ
- Modify: e2e specs: attendance theo trợ giảng, học vụ send, GV-cũ-mất-write
- Modify: `docs/api-guidelines.md` — capability map section

## Implementation Steps

1. `WriteExists` + `writeScoped` helpers + guard test mở rộng.
2. Attendance theo bảng method (UpsertMany/SoftDeleteMissing/Tally) + test:
   trợ giảng ghi 8 buổi → invoice GV chủ nhiệm đếm đủ; GV mới lưu roster đổi
   trên buổi GV cũ ghi → không row billable mồ côi.
3. Đổi từng write path còn lại theo bảng chuyển đổi — THAY THẾ gate cũ; mỗi
   path integration test 5 vai + 2 case handoff: GV cũ write session TƯƠNG LAI
   → 403, GV cũ write session ĐÃ DIỄN RA trước handoff → 403 (regression bẫy
   OR-thêm); GV mới write session cũ → 200 (D8).
4. Migration 000017 (statements.class_id + notification_runs.class_id +
   re-key uniques) + test up/down; token derive theo class_id.
5. Học vụ statements class-scoped: generate bản lớp + target filter +
   ZaloMappings widen + send + attribution; test: channel `zalo_personal`;
   bản lớp chỉ chứa lines lớp gán; học vụ lớp A không thấy/không gửi được
   contact lớp B (404/empty); 2 học vụ 2 lớp cùng kỳ gửi song song đều 200;
   URL bản lớp không mở được bản gia đình; cảnh báo chồng lấn khi kỳ đã có
   bản gia đình gửi.
6. Audit GV-owns-rows list trên — ghi kết quả vào delivery note của phase.
7. Web gates + vitest; e2e 3 kịch bản mới (gồm UI statement class-scoped
   cho học vụ).
8. Full suites + swagger regen.

## Success Criteria

- [x] Acceptance 2, 5, 6 của plan.md pass (handoff write-flip, học vụ
      read+send, trợ giảng attendance).
- [x] GV cũ sau handoff: mọi write 403 KỂ CẢ trên session đã diễn ra; đọc
      lịch sử 200 — integration + e2e.
- [x] Trợ giảng ghi điểm danh thẳng, không endpoint duyệt nào tồn tại; invoice
      đếm đủ buổi trợ giảng ghi.
- [x] Học vụ gửi statement BẢN THEO LỚP từ zalo của mình qua channel
      `zalo_personal` (assert channel), attribution đúng; bản gửi chỉ chứa
      phí lớp gán; lớp khác → 404; preview không chứa contact ngoài lớp gán.
- [x] 2 học vụ 2 lớp cùng kỳ gửi song song không đâm nhau (không 409);
      migration 000017 up/down sạch; URL bản lớp không đọc được bản gia đình.

## Delivery note — audit GV-owns-rows (bước 6, 2026-08-30)

Kết quả audit 4 mục + grep sweep (`teacher_id = ?` / `sc.TeacherID` /
`session.TeacherID` trong repo+service các feature lớp):

- **Dashboard** (`centers/dashboard.go`): giữ port per-teacher `targetScope`
  như P2 chốt. Số liệu không đổi sau write-flip: attendance đọc qua
  `attendance.Get` (readScoped theo lớp), còn `TallyByEnrollment` anchor trên
  `enrollments.teacher_id` (sở hữu dữ liệu) chứ KHÔNG lọc
  `attendance_records.teacher_id` — buổi trợ giảng ghi được đếm đủ.
- **Notifications ledger** (`runsOwnScoped`, notifications/repository.go:231):
  giữ own-rows đúng chủ đích — ledger là "đợt tôi gửi"; chiều class chồng lên
  bằng `runClassDimension`, không thay anchor.
- **Import điểm danh/điểm**: không tồn tại — feature `imports` chỉ import
  roster (contacts/students), không có đường ghi attendance/scores nào ngoài
  capability map.
- **billing/payments/collections**: không chỗ nào dùng `classes.teacher_id`;
  tiền anchor trên enrollment/period teacher (data-ownership), không write
  nào mở cho GV. Tally đã sửa ở bước 2.
- **Sweep**: các `teacher_id` còn lại đều thuộc 4 nhóm hợp lệ — (a) stamp
  attribution khi ghi (grading/teaching anchor row theo session teacher),
  (b) OR-widening trong readScoped (đọc lịch sử), (c) anchor sở hữu dữ liệu
  (enrollments: class anchor + stint giao_vien active), (d) `ReassignPlanned`
  của handoff chạy dưới flow owner. Sửa 1 doc comment cũ ở
  `sessions/repository.go` (readScoped) còn nói lifecycle writes dùng
  `scoped`.
- **Carryover P1 đã đóng**: `SyncPrimaryTeacher` điều kiện INSERT trên
  membership sống (classstaff/repository.go:222) — test
  `TestSyncPrimaryTeacherSkipsKickedMember` cover đúng kịch bản kick →
  handoff no-op → không sinh stint active.

## Delivery note — review cycle bước 7–8 (2026-08-30)

Code review trả DONE_WITH_CONCERNS với 12 finding; đã fix in-slice 11, còn 1
info-only:

- **Web gates lệch capability map (đã fix)**: attendance-page gate theo
  per-capability — `canRecordAttendance` cho confirm/toggle, `canWriteClass`
  cho "Huỷ buổi học" (lifecycle write); students-page ẩn "+ Ghi danh học sinh"
  khi thiếu `canWriteClass`; confirm bar nhận `accessResolved` để không nháy
  nhãn từ chối khi class chưa tải, nhãn từ chối thêm trợ giảng.
- **e2e (đã fix)**: read-spec assert trợ giảng KHÔNG thấy "Huỷ buổi học";
  write-spec flip điểm danh restore qua afterEach idempotent (bắt aria-pressed
  trước khi flip) — không còn dựa vào happy path; các assert nhãn theo copy
  mới.
- **API runSnapshot thiếu gate class (đã fix)**: `RunSnapshot` chiều class giờ
  qua `AuthorizeClassSend` như SendPreview/ResumeRun — trước đó mọi member
  cùng center poll được tiến độ gửi của lớp bất kỳ. Test pin:
  `TestClassRunSnapshotRequiresTheClassSendGate` (403 trợ giảng, 404 outsider).
- **Coverage (đã bổ sung)**: billing class-periods — stint ĐÃ KẾT THÚC vẫn
  đọc, GV bị handoff vẫn đọc, class không tồn tại/khác center → 404 trung
  lập, invoice void rơi khỏi list
  (`TestListPeriodsByClassEndedStintsForeignClassesAndVoids`).
- **Dọn dẹp (đã fix)**: gộp `class-permissions.test.ts` trùng về
  `features/roster/__tests__/`; notifications-page reset mutation bulk-send
  khi đổi scope (period/purpose/class) để banner kết quả không rò giữa tab;
  api-guidelines thêm mục "Class-staff writes (capability map)".
- **Info-only (không sửa)**: `classInvoiceLinesExist` chưa có index hỗ trợ —
  ghi nhận perf note, bảng nhỏ, chưa cần.

Hai test attendance fail sau deploy chẩn đoán là TEST CŨ (giữ semantics
phase-2/anchor cũ), không phải bug code: cập nhật theo capability map +
last-writer attribution đã chốt, xanh lại với `-count=1`. Production không
cần redeploy.

## Review carryover từ P1 (code review 2026-08-30)

- Handoff no-op path (`handoff/service.go`) cố ý bỏ qua `IsActiveMember`
  ("re-affirm không được fail") nhưng giờ là một WRITE: owner PUT teacher về
  chính GV hiện tại của lớp có thể TÁI TẠO stint active cho member đã bị kick
  (row `center_members` còn, account disabled → hôm nay vô hại). Khi phase này
  biến stint active thành write capability, phải chặn: hoặc guard
  `SyncPrimaryTeacher` INSERT bằng `WHERE EXISTS` membership sống, hoặc mọi
  capability check đòi thêm account/membership sống. Test case: kick member
  còn giữ lớp → handoff no-op → không được sinh capability ghi.

## Risk Assessment

- **Đây là phase behavior-flip lớn nhất** (GV cũ mất write, kể cả lịch sử):
  ship sau khi P1–P3 soak; release note cho user. Rollback = revert code
  trước; migration 000017 chỉ THÊM cột/index nullable nên code cũ chạy được
  trên schema mới — migrate down chỉ khi cần dọn hẳn.
- **Phụ huynh nhận trùng tin** (bản gia đình + bản lớp cùng kỳ, hoặc nhiều
  bản lớp): quyết định user (validation Q6–Q7) — cảnh báo UI + release note;
  tiền không lệch vì cùng nguồn invoice lines.
- **Miss một write path / OR-thêm lén** → grep sweep `teacher_id = ?` /
  `sc.TeacherID` / `session.TeacherID` trong repo+service các feature lớp
  trước khi đóng phase; guard test liệt kê write methods.
- **Re-key `uq_notification_runs_one_active_period`** khi đang có run active
  → chạy migration lúc không có run chạy (index partial mới cover ngay);
  reconciler không phụ thuộc index nên không cần code gate thêm.
