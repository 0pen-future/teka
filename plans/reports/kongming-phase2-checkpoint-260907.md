# Kongming counsel — Phase 2 checkpoint: go/no-go Phase 3, H-1, M-3, Phase 3 risks

Ngày: 2026-09-07. Plan: `plans/260906-0627-authz-write-scope-root-cause/`.
Advisory only. Mọi khẳng định dưới đây đã đối chiếu working tree tại
`apps/api/internal` (file:line), migration, docs và báo cáo kiểm kê prod;
không dựa trí nhớ. Chạy trên `fable`.

## TL;DR

1. **GO Phase 3, với điều kiện commit cây hiện tại trước.** 67 file chưa
   commit gộp phase 1+2 (+3159/−1138); phase 3 sửa đúng các file
   billing/statements/notifications/contacts repository mà phase 1–2 vừa sửa.
   Không commit thì không còn ranh giới để revert hay review riêng. Cook ≠
   merge: reviewer chặn *merge/deploy* tới khi owner xác nhận D9 là đúng,
   nhưng không chặn cook.
2. **H-1 là lỗi prod đang sống, hotfix ngay, trước hoặc song song Phase 3,
   cùng PR với phase 1+2.** Không phải câu hỏi sản phẩm. Migration `000016`
   (2026-08-30) đã neo *mọi* contact và student về owner
   (`migrations/000016_owner_data_anchor.up.sql:1-8`); kiểm kê prod ghi 2
   member ở 2 center tạo 27 kỳ và draft 6 kỳ trong 60 ngày
   (`plans/reports/inventory-260906-prod-view-all-write-exposure.md:27-31`).
   Invoice của kỳ member mang `teacher_id = member`
   (`billing/preview.go:230`, enrollments lọc theo teacher kỳ
   `billing/repository.go:634`) nhưng `contact_id` là contact của owner →
   `CandidateInvoices`/`InvoicesByIDs`/`RecalcInvoicePaid` lọc
   `i.teacher_id = owner` không bao giờ chạm invoice member. Chính kịch bản
   D9 (member ghi thanh toán cho học sinh của mình) hôm nay allocate 0 dòng;
   test `TestMemberRecordsPaymentForCenterContactWithoutViewAll` xanh chỉ vì
   dùng kỳ của **owner** (`payments/integration_test.go:946-949`).
   Fix: candidacy theo `center_id + contact_id`, recalc theo `center_id + id`,
   bỏ arm teacher; đổi chữ ký ba method sang dạng center-keyed (không nhận
   `Anchor` nữa — bài học M-1: Anchor không dùng để lọc là chữ ký nói dối).
3. **M-3: Phase 4 (linter allowlist call-site). `students.Create`: đóng ngay,
   3 dòng.** Docs đã tuyên bố "student record writes are owner-only"
   (`docs/api-guidelines.md:194`, `:261`); code drift khỏi docs, không phải
   quyết định mới. Gate `IsOwner` như `contacts.Create`
   (`contacts/service.go:56-58`) + một test 403. Web không gate theo
   `students.create` (chỉ có trong MSW fixture), giống tình trạng
   `contacts.create` hiện tại — tiền lệ đã có.
4. **Phase 3: bảng phân loại có hai ô sai vị trí và một lỗ có sẵn.**
   `statements/service.go:347` không phải `AuthorizeClassSend` (đó là `:110`)
   mà là `resp.URL` — link public của statement, là *vật phẩm gửi*, phải giữ
   gate gửi. `contacts/repository.go:112` `scopedMappingWrite` là WRITE (nối
   lại Zalo của phụ huynh), comment tại chỗ cấm `contacts.view_all`, phải giữ
   trục gửi. `notifications/repository.go:558` `ZaloMappings` lọc
   `contacts.teacher_id = sc.TeacherID` cho non-oversight → sau 000016 luôn
   rỗng cho member (class-role sender không có mapping Zalo) — pre-existing,
   phase 3 phải re-key thay vì giữ arm teacher. Test duy nhất bắt sai phân
   loại: **parity hai principal** ở HTTP — member chỉ có `reports.send` phải
   trùng member giữ đúng 4 key `view_all` trên mọi GET, và khác nhau *đúng*
   trên các bề mặt gửi.
5. **Implied keys: đặt trong `BuildPermSet`, sau bước trừ deny.** Ba resolver
   (`centers/service.go:73`, `testutil/fixtures.go:195`, `seeds/seed.go:346`)
   đã lặp cùng 3 dòng; `WithImplied()` tách rời là dòng thứ tư dễ quên ở một
   trong ba nơi mà không guard nào bắt được. Chỉ 3 test dựng
   `CanSendReports: true` tay, sửa rẻ.

## Reframed problem

Câu hỏi thật không phải "phase 2 đủ tốt để đi tiếp chưa" (đủ: 33/33 package,
guard AST, không đổi contract) mà là **thứ tự và ranh giới**: những gì phải
nằm trong cùng đợt deploy với phase 1 (H-1, vì D9 vô nghĩa nếu allocation
không chạm invoice member), những gì độc lập về file với phase 3 (payments,
students — phase 3 không đụng), và những gì cần linter mới có bằng chứng
(M-3). Non-goal: mở lại D5/D9, đổi anchor của payment.

## Bằng chứng đã xác minh

- HEAD (`git show HEAD:…payments/repository.go:381-400`): `ResolveContactScope`
  lọc `teacher_id = sc.TeacherID` trừ khi có `payments.view_all`, và trả
  Scope `{TeacherID: contact.teacher_id, Perms: nil}` → candidacy đã lọc
  `i.teacher_id = owner` từ trước phase 1. Sau 000016, member không view_all
  bị 404 với **mọi** contact; owner ghi thanh toán chỉ chạm invoice của owner.
  Kết luận reviewer "pre-existing" đúng, nhưng mức độ là P1 chứ không phải
  edge case "member tự tạo student".
- `payment_allocations` FK theo `(invoice_id, center_id)` và
  `(teacher_id, center_id) → center_members` (`000007_centers.up.sql:186,211`);
  FK `(invoice_id, teacher_id)` đã bị drop (`:242`). Allocation của payment
  neo owner lên invoice neo member là **hợp lệ về schema**.
- `recalcInvoicePaidQuery` tổng allocation theo `pa.invoice_id` không lọc
  teacher (`payments/repository.go:378-385`); chỉ mệnh đề WHERE của UPDATE có
  arm teacher (`:387`). Billing `RecalcInvoiceTotals` chỉ đọc
  `invoices.paid_amount` đã lưu (`billing/repository.go:975-995`) → bỏ arm
  teacher ở payments không làm billing tính sai.
- Reallocate có guard chéo contact thật: `inv.ContactID != payment.ContactID`
  (`payments/reversal.go:148`) — đây mới là hàng rào, không phải arm teacher.
- `LockPayment` đi qua `writeScoped` (`payments/repository.go:208-212`): payment
  neo owner → member không `WriteWide` không thể Reverse/Reallocate bất kỳ
  payment nào sau 000016. Hệ quả D9 chưa ghi trong plan.
- Notification rows được stamp `TeacherID: sc.TeacherID` của người gửi
  (`notifications/service.go:303,345`) → `MarkSent` own-rows đã đúng cho
  secretary gửi zalo_manual; lỗ chỉ còn ở "0 dòng im lặng".
- `ClassSendAccess` chỉ tính stint (`statements/repository.go:329-349`);
  `AuthorizeClassSend` OR với oversight ở service (`:110`) — đúng là gate gửi.
- Test dựng `CanSendReports: true` tay: `authctx/phone_visibility_test.go:11`,
  `authctx/anchor_test.go:27`, `notifications/integration_test.go:509`
  (`scMemberSend.CanSendReports = true` trên scope đã `ScopeFor`). Còn lại 581
  chỗ dùng `ScopeFor`.
- `BuildPermSet` chỉ có 3 caller ngoài test, đều là resolver; không màn hình
  roles nào dựng PermSet từ nó → implied key không rò vào UI vai trò.

## What to do

### Q1 — Go/no-go

GO. Thứ tự:

1. `git switch -c authz-write-scope`; commit. Nếu tách hunk phase 1 / phase 2
   sạch được thì hai commit; nếu không, một commit (plan đã chấp nhận deploy
   2+3 cùng nhau, và deploy phase 1 riêng vốn đang chờ D9). Không push (cần
   người dùng duyệt push).
2. Chạy song song, file không giao nhau với phase 3: slice **H-1** (`payments/`),
   slice **students gate** (`students/`).
3. Phase 3.
4. Cổng merge: owner chốt D9 (bổ sung: "payment neo owner nên chỉ owner
   reverse/reallocate được") + test H-1 xanh + parity hai principal xanh.

### Q2 — H-1: fix cause-aligned

Nguyên nhân: ba query lọc theo teacher của **anchor payment** trong khi tập
đích là **invoice của contact trong center**, hai neo khác nhau từ 000016.
Fix ở đúng chỗ đó, không đổi anchor payment (một payment có thể trải lên
invoice của hai giáo viên — hai con học hai lớp — nên không thể neo theo
invoice).

- `CandidateInvoices(ctx, sc, contactID)`: `WHERE i.center_id = ? AND
  i.contact_id = ?` — bỏ cặp `(? OR i.teacher_id = ?)`; giữ `FOR UPDATE OF i`
  và `ORDER BY i.id` (kỷ luật lock không đổi).
- `InvoicesByIDs(ctx, sc, ids)`: `center_id = ? AND id IN ?`; guard contact ở
  `reversal.go:148` giữ nguyên.
- `RecalcInvoicePaid(ctx, sc, invoiceID)`: `WHERE i.id = ? AND i.center_id = ?`.
- Helper: `invoiceCenterScoped(ctx, sc)` chỉ bind `sc.CenterID` — đúng
  loại thứ ba của plan (`centerScoped`), tiền lệ M-1
  `attendance.ListBySessionCenter`. D9 nói "không đổi sang `sc` của caller"
  là để không widen theo perm; helper này không đọc perm nào. Nếu lead thấy
  `sc` trong chữ ký vẫn gây hiểu nhầm, dùng `centerID uuid.UUID` trần — nhưng
  phải khai directive cho linter phase 4; tôi khuyên `sc` + `centerScoped` để
  linter R1 nhìn thấy.
- Sửa doc comment interface (`payments/repository.go:69-75, 82-84, 116-121`)
  và câu ở `docs/api-guidelines.md:255-257`; plan OQ3 → "decided".
- `assertLedgerInvariant(t, db, teacherID)` (`integration_test.go:86`) phải
  chuyển sang center: tổng allocation theo center so với `paid_amount` theo
  center, vì allocation neo owner còn invoice neo member.

Test pin (tên gợi ý `TestPaymentSettlesAnotherTeachersInvoiceOfTheSameContact`):
contact `testutil.Contact(t, db, owner.ID)`; student
`testutil.Student(t, db, owner.ID, contact.ID)`; class + enrollment + session +
attendance neo **member** (`testutil.Class/Enrollment/Session/AttendanceRecord`
với `member.ID`); `EnsurePeriod`+`Close` bằng `scMember` → `getInvoice(t, db,
member.ID, student)` có `teacher_id = member`. Ghi thanh toán hai lần, mỗi
lần một subtest: bằng `scOwner`, và bằng `scMember` chỉ có `payments.create`.
Assert: `len(Allocations)==1`, `Payment.TeacherID == owner.ID`, invoice
`InvoicePaid` với `PaidAmount` đúng (chứng minh recalc cập nhật 1 dòng). Thêm
`Reverse` bằng `scOwner` → invoice về `issued`, `paid_amount 0`. Thêm một
Reallocate lên đúng invoice đó để phủ `InvoicesByIDs`. Test này đỏ trên cây
hiện tại (allocation rỗng, invoice vẫn `issued`).

### Q3 — M-3 và students.Create

- **M-3 → Phase 4.** `classes.CreateAnchored`/`AddScheduleAnchored`/
  `enrollments.CreateAnchored` có caller ngoài package duy nhất là imports
  (`imports/apply.go:143,174,285`); anchor lấy từ tên giáo viên trong workbook
  nên `OwnerAnchor` không áp được, unexport không được. Bằng chứng "không ai
  nối vào handler" chỉ có linter mới giữ được lâu dài: R4 "call `*Anchored`
  exported chỉ từ `features/imports/` hoặc cùng package". Không thêm type
  mint mới cho member (over-engineering so với một rule allowlist).
- **students.Create → đóng ngay** (`students/service.go:64-70`): `if
  !sc.IsOwner { return Forbidden("chỉ chủ trung tâm quản lý hồ sơ học viên") }`
  trước `checkContact`; test member có `students.create` → 403; owner không
  đổi. Ghi một dòng vào release note docs (đã có đoạn `:259-263`, chỉ thêm
  "students.create hiệu lực với owner"). Web: không có gate theo key này, cùng
  tình trạng `contacts.create` — ghi follow-up web, không chặn.
- Việc này không làm H-1 biến mất: premise của H-1 là invoice neo theo
  **kỳ**, không theo student.

### Q4 — Phase 3

Quy tắc chọn key khi đổi `ReportsOversight()` → `CenterWideFor(...)`: **key
`view_all` của resource mà route phục vụ read đó** (theo `route_policy.go`),
không theo tên bảng: collections routes là `billing.read`
(`route_policy.go:194-195`) → `billing.view_all`; statements list/get →
`statements.view_all` (kể cả `periodStatus` tại `statements/repository.go:322`
dù đọc `billing_periods`); notifications list/run → `notifications.view_all`.

Phân loại lại 27 site (đã đọc từng dòng):

| Site | Loại | Hành động |
|---|---|---|
| `billing/repository.go:363` `scopedRead`; `billing/service.go:131` | đọc | `CenterWideFor(billing.view_all)`; gộp `scopedRead` vào `readScoped` |
| `collections/repository.go:59,104,204,263,341,359,387,393,394` | đọc | `CenterWideFor(billing.view_all)` |
| `statements/repository.go:294` `scopedRead` | đọc, **giữ arm stint** | `!CenterWideFor(statements.view_all)` → teacher OR stint |
| `statements/repository.go:322` `GetPeriodStatusRead` | đọc | `CenterWideFor(statements.view_all)` |
| `statements/service.go:110` `AuthorizeClassSend` | **gate gửi** | giữ |
| `statements/service.go:347` `resp.URL` | **gate gửi** (link public là vật phẩm gửi; reader `view_all` không được cầm link) | giữ; plan table ghi nhầm dòng này là AuthorizeClassSend |
| `contacts/repository.go:97` `scopedRead` | đọc | bỏ arm oversight, giữ `CenterWideFor(contacts.view_all)` + stint |
| `contacts/repository.go:112` `scopedMappingWrite` | **WRITE trục gửi** | giữ `ReportsOversight()`; comment tại chỗ đã cấm `view_all` |
| `authctx.go:77` `PhoneVisible` | đọc | `CenterWideFor(contacts.view_all) \|\| rowVisible` |
| `zalo/service.go:492` `MatchFriendsScoped` | đọc có egress phone | `CenterWideFor(contacts.view_all)` — nhất quán với PhoneVisible (holder đã thấy phone) |
| `notifications/repository.go:228` `runsPeriodScoped`, `:288` `ListByPeriod` | đọc | `CenterWideFor(notifications.view_all)` |
| `notifications/repository.go:558` `ZaloMappings` | đọc **đang hỏng cho member** | re-key: `center_id` + (`CenterWideFor(contacts.view_all)` OR `PhoneVisibleViaContact` stint); không giữ `contacts.teacher_id = sc.TeacherID` |
| `notifications/service.go:129,499,675` BulkSend/SendPreview/ResumeRun; `:398` `runGrant` | gate gửi | giữ (SendPreview là preview *của một lần gửi*, có check `CanSendReports` chéo giáo viên ở `:512`) |

`MarkSent` (`notifications/repository.go:293`): giữ `writeScoped`; thêm
`RowsAffected != len(ids)` → `ErrNotificationNotFound` ở repo để chấm dứt "0
dòng im lặng". Không mở theo trục gửi: rows do secretary tạo đã mang
`teacher_id` của chính họ.

**Test duy nhất bắt sai phân loại** — thêm vào
`server/policy_integration_test.go` cạnh `TestPolicyHTTPViewAllParity`: cùng
fixture (owner có period đã close, statements, notifications queued, contact
có Zalo), ba principal: A = member chỉ `reports.send`; B = member giữ đúng
`billing/statements/notifications/contacts.view_all`, không `reports.send`;
C = member thường. Assert A ≡ B trên mọi GET (status + body, kể cả
`phone` và `unallocated_credit`), A ≠ B đúng trên: bulk send, preview, resume,
`url` trong statement response (A non-nil, B nil), PUT zalo mapping, mark-sent
hàng của owner. Read xếp nhầm vào gate gửi → A≠B trên một GET; gate gửi xếp
nhầm vào read → B qua được một POST gửi. Bổ sung C → 404 trên tất cả để
chứng minh implied key không rò.

**Implied keys**: trong `BuildPermSet`, sau `delete` deny:

```go
for key := range set { for _, k := range impliedKeys[key] { set[k] = struct{}{} } }
```

Lý do: một chỗ duy nhất, ba resolver hưởng tự động, `EffectiveKeys()` (đọc từ
`Perms`) trả key suy ra miễn phí, deny không gỡ được (đúng D6). Lý do phase
file giữ nguyên `BuildPermSet` ("giữ test hiện có") không đủ nặng: `TestBuildPermSet`
thêm một case. Sửa ba test dựng tay: `phone_visibility_test.go:11` →
`Scope{CanSendReports: true, Perms: BuildPermSet(nil, []string{PermReportsSend}, nil)}`;
`anchor_test.go:27` chỉ kiểm reflect, không cần đổi; `notifications/integration_test.go:509`
→ grant `reports.send` trong DB rồi `ScopeFor`, hoặc `Perms = BuildPermSet(...)`.
Ghi vào `TestScopeLiteralsOnlyWhereResolved`? Không — test được phép dựng
Scope; nhưng thêm một assertion vào `centers/permissions_integration_test.go`:
member có `reports.send` → `EffectiveKeys()` chứa 4 key.

## What to avoid

- Bắt đầu phase 3 trên cây chưa commit.
- Coi H-1 là "câu hỏi sản phẩm": schema, docs (`:255-257`) và kiểm kê prod
  đều nói cùng một điều — payment là tiền center cho một contact.
- Sửa H-1 bằng cách neo payment theo invoice teacher (vỡ khi một contact có
  con ở hai giáo viên) hoặc bằng `CenterWideFor(payments.view_all)` trên `sc`
  (lặp lại đúng lỗi phase 1 vừa chữa).
- Giữ tham số `Anchor` trên ba method payments sau khi bỏ arm teacher.
- Đổi `statements/service.go:347` hay `contacts/repository.go:112` sang
  `view_all` vì bảng plan xếp chúng vào "đọc".
- Nới implied sang `payments.view_all`.
- Sửa MarkSent bằng cách widen theo `ReportsOversight()` trong write helper.

## Alternatives & trade-offs

- **H-1 fold vào Phase 3** thay vì slice riêng: tiết kiệm một lần chạy suite,
  nhưng trộn một bugfix money P1 vào refactor trục đọc làm review/revert khó;
  file không giao nhau nên song song rẻ hơn. Không khuyên.
- **`WithImplied()` tách rời** (phương án phase file): giữ `BuildPermSet` thuần,
  nhưng phải nhớ gọi ở 3 resolver và mọi test tự dựng PermSet; không guard nào
  bắt được quên. Chỉ chọn nếu muốn hiển thị "key thật vs key suy ra" tách
  bạch ở web — plan đã nói UI là follow-up.
- **students.Create neo về owner qua `ResolveOwnerAnchor`** thay vì 403: giữ
  UX cho member, không có chu trình import (centers không import students),
  nhưng thêm dependency `centers` vào students và trái docs "member không tạo
  student". Chọn 403 trừ khi owner nói ngược lại.

## Work checklist

1. Branch + commit cây hiện tại (1–2 commit).
2. Slice H-1: 3 query + helper + chữ ký + `assertLedgerInvariant` center-level
   + test pin đỏ→xanh + doc comment + `docs/api-guidelines.md:255-257` + plan
   OQ3 = decided, D9 thêm câu "chỉ owner reverse/reallocate".
3. Slice students: gate `IsOwner` + test 403 + một dòng docs.
4. Phase 3 theo bảng trên; parity ba principal; `BuildPermSet` implied;
   `MarkSent` RowsAffected; `ZaloMappings` re-key; grep còn lại đúng 8 site
   gate gửi (`statements/service.go:110,347`, `contacts/repository.go:112`,
   `notifications/service.go:129,398,499,675`, định nghĩa).
5. Phase 4 nhận M-3 (rule R4 allowlist `*Anchored`).
6. Trước merge: owner chốt D9 (phiên bản đầy đủ), tester full suite.

## Success metrics

- Test H-1 đỏ trên cây hiện tại, xanh sau fix; `TestMemberRecordsPayment…`
  vẫn xanh.
- Sau phase 3: `grep -rn 'ReportsOversight()' apps/api/internal --include='*.go' | grep -v _test.go`
  đúng 8 dòng + định nghĩa; parity A≡B xanh; C → 404 toàn bộ.
- Prod sau deploy: invoice của member chuyển `paid`/`partially_paid` khi owner
  ghi thanh toán — xác nhận bằng một truy vấn đếm `invoices` status theo
  `teacher_id <> owner` trước/sau.

## Assumptions

- Hai member prod có invoice đã issued dưới kỳ của họ (kiểm kê chỉ đếm
  create/draft, không đếm close) — **medium**; kể cả chưa có, H-1 chặn D9 nên
  vẫn phải fix trước deploy.
- Owner chấp nhận "chỉ owner reverse/reallocate payment" — **medium**; nếu
  không, cần helper `payments` write theo trục `payments.reverse` + contact
  center, là scope mới.
- Không có test web assert danh sách `permissions` chính xác — **high** (plan
  đã ghi phải kiểm; MSW fixture chỉ là catalog).
- Không màn hình web nào cho member "thêm học sinh" phụ thuộc 200 — **low**;
  nếu có, 403 hiển thị như `contacts.create` hôm nay, follow-up web.
