# Code Review — Phase 2: Row-Anchor Type

Ngày: 2026-09-07
Phạm vi: working tree diff (`git diff` + untracked `internal/shared/authctx/anchor_test.go`)
Reviewer: code-reviewer (read-only, không sửa code, không commit)

## Score: 8 / 10

Refactor sạch, có kỷ luật, tài liệu tốt, guard test thật. Điểm trừ đến từ một
chỗ phá vỡ chính bất biến mà phase này tạo ra, một câu hỏi mở bị hoãn trên
tiền đề sai, và vài bề mặt exported mới chưa có gate.

## Verification đã chạy

| Check | Kết quả |
|---|---|
| `go build ./...` | pass |
| `go vet -tags=integration ./...` | pass |
| `gofmt -l ./internal ./seeds ./cmd` | rỗng |
| `make lint-api` | 0 issues |
| `TestScopeLiteralsOnlyWhereResolved` | pass |
| `TestRepositoriesScopeThroughCenterWideOnly` | pass |
| `TestRepositoriesWidenWritesThroughWriteWideOnly` | pass |
| `go test ./internal/shared/authctx/` | pass |
| `make test-api` | KHÔNG chạy (tester agent đang chạy song song) |

## Success Criteria

| # | Tiêu chí | Trạng thái |
|---|---|---|
| 1 | `IsOwner: true` ngoài centers/tests rỗng | PASS (grep rỗng) |
| 2 | `TestScopeLiteralsOnlyWhereResolved` xanh, bắt 3 mutation | PASS (xanh; 3 mutation bắt được theo đọc AST — xem M-2 về lỗ còn lại) |
| 3 | `MintOwnerAnchor(` ngoài centers/authctx rỗng | PASS (grep rỗng) |
| 4 | Test imports: contacts/students anchor owner, audit actor = member | PASS (`imports/integration_test.go:388-428`) |
| 5 | Test trợ giảng confirm buổi thuộc kỳ đã đóng → adjustment | PASS (`billing/integration_test.go:1610-1653`) |
| 6 | Không test nào đổi expectation cho owner | PASS (chỉ expectation của member đổi ở contacts/students dedupe) |
| 7 | `make test-api` xanh, coverage không tụt | KHÔNG XÁC MINH ĐƯỢC (ngoài quyền chạy của review này) |

---

## Findings

### High

**H-1 — `CandidateInvoices` lọc `i.teacher_id = anchor.TeacherID` trong khi
invoice anchor theo giáo viên của kỳ, không theo contact.**
`apps/api/internal/features/payments/repository.go:298` (query tại `:266-289`),
`:408` (`RecalcInvoicePaid`).

Open question 3 của plan được hoãn khỏi phase này với lý do ghi tại
`plans/260906-0627-authz-write-scope-root-cause/phase-02-row-anchor-type.md:180-182`:
*"giữ `i.teacher_id = a.TeacherID`, đúng theo cấu trúc vì contacts luôn anchor
vào owner"*. Tiền đề đó nói về contacts, nhưng bộ lọc nằm trên **invoices**:

- Contact luôn anchor owner (`contacts/service.go:56-62` chặn non-owner;
  import mint `OwnerAnchor`).
- Student anchor theo **người tạo** (`students/service.go:64-70`, `sc.Self()`),
  không có gate owner.
- Billing period anchor theo caller (`billing/service.go:57`, `sc.Self()`), và
  invoice lấy `TeacherID: a.TeacherID` từ anchor kỳ (`billing/preview.go:230`).

Nên một member giữ `contacts.view_all` + `students.create` tạo student →
invoice mang `teacher_id = member` trong khi `contacts.teacher_id = owner`.
`ResolveContactAnchor` trả anchor owner, `CandidateInvoices` lọc
`i.teacher_id = owner` → **bỏ sót invoice của member**: auto-allocate không
tìm thấy hoá đơn nào và `RecalcInvoicePaid(anchor=owner, invoiceID=của member)`
cập nhật 0 dòng, im lặng, không lỗi.

Đây là lỗi **có sẵn**, không phải regression của phase 2 (đường cũ dùng
`ownerScope` literal `Perms=nil` nên `CenterWideFor` đã là false — filter
giống hệt). Nhưng phase 2 khoá nó vào kiểu `Anchor` và ghi lại một lý do sai.

Đề xuất: hoặc đổi candidacy sang `contact_id + center_id` như Open question 3
gợi ý (và giữ `FOR UPDATE` trên đúng tập đó), hoặc — nếu quyết định giữ
nguyên — sửa Execution Note thành lý do đúng và ghi rõ điều kiện tiền đề
("chỉ đúng khi student cũng anchor owner"), kèm test pin hành vi khi student
anchor member.

### Medium

**M-1 — `ListBySessionAnchored` nhận `Anchor` nhưng không dùng `a.TeacherID`.**
`apps/api/internal/features/attendance/repository.go:146-153`.

```go
func (r *gormRepository) ListBySessionAnchored(ctx context.Context, a authctx.Anchor, sessionID uuid.UUID) ([]Record, error) {
	var records []Record
	err := database.FromContext(ctx, r.db).
		Where("attendance_records.center_id = ?", a.CenterID).
		Where("attendance_records.session_id = ?", sessionID).
		Find(&records).Error
	return records, err
}
```

Phase file ghi bất biến: *"Repo method nhận `Anchor` luôn lọc
`center_id = a.CenterID AND teacher_id = a.TeacherID` (helper `anchored(ctx, a)`);
không có nhánh widen"*. `docs/api-guidelines.md` (đoạn "Scope vs Anchor" vừa
thêm) khẳng định lại đúng câu đó cho toàn repo. Method này là ngoại lệ duy
nhất và nó phá đúng thứ mà phase tồn tại để dựng lên: đọc signature là biết
bộ lọc.

Hành vi thì **đúng và cần thiết**: `TestAssistantConfirmOnClosedPeriodPostsAdjustmentOnClassTeachersBilling`
(`billing/integration_test.go:1610`) sẽ đỏ nếu thêm bộ lọc teacher — dòng
attendance do trợ giảng ghi mang `teacher_id = tro_giang`, còn reconciliation
anchor trên giáo viên lớp. Đây cũng là một **nới rộng hành vi** so với trước
(đường cũ `readScoped` có nhánh stint-OR không phủ trợ giảng ghi hộ), không
phải refactor kiểu thuần tuý — nhưng có chủ đích, có comment interface
(`repository.go:50-57`) và có test.

Đề xuất: đổi signature sang `ListBySessionInCenter(ctx, centerID uuid.UUID, sessionID uuid.UUID)`
(hoặc tên nói rõ center-keyed), và bổ sung vào đoạn "Scope vs Anchor" của
`docs/api-guidelines.md` hạng mục thứ ba: truy vấn center-keyed **không có
caller scope trong tay**. Hiện docs chỉ mô tả `centerScoped(ctx, sc)` với
Scope thật, không có chỗ cho ca này.

**M-2 — Guard AST bỏ sót thư mục và composite literal lồng.**
`apps/api/internal/features/scoping_guard_test.go:137` (walk root `".."`),
`:178-186` (`isAuthctx(node.Type, …)`).

- Walk root là `internal/`, nên `apps/api/seeds/` và `apps/api/cmd/` không được
  quét. `seeds/seed.go:347` dựng `authctx.Scope{...}` có field. Đó là resolve
  hợp lệ từ DB, nhưng comment của guard (`:118-122`) không nói ra giới hạn
  root, nên người đọc sau sẽ tưởng độ phủ là toàn `apps/api`.
- Literal lồng trong slice/map (`[]authctx.Scope{{TeacherID: x}}`) có
  `CompositeLit.Type == nil` → `isAuthctx` trả false → lọt.
- Check `AssignStmt` (`:188-199`) bắt **mọi** selector tên
  `IsOwner`/`Perms`/`CanSendReports` trong file có import authctx, kể cả trên
  kiểu khác. Bảo thủ theo hướng an toàn (chỉ false positive), nhưng nên ghi
  chú để người sau không sửa nhầm.

Đề xuất: nêu rõ giới hạn root trong comment (hoặc thêm root thứ hai
`../../seeds`), và xử lý nhánh `node.Type == nil` bằng cách đọc kiểu phần tử
của composite literal bao ngoài.

**M-3 — Bốn entry point exported mới nhận `Anchor` trần, không proof, không gate.**
- `classes/service.go:56` `CreateAnchored(ctx, a authctx.Anchor, req)`
- `classes/service.go:303` `AddScheduleAnchored(ctx, a authctx.Anchor, …)`
- `enrollments/service.go:112` `CreateAnchored(ctx, actor Scope, a Anchor, req)`
- `enrollments/service.go:245` `ActiveOnClass(ctx, sc Scope, classID, on)` —
  center-keyed, không gate.

Contacts/students làm đúng: `CreateAnchored` chỉ nhận `authctx.OwnerAnchor`,
và `contacts/service_test.go:579-583` có một interface assertion compile-time
chặn việc nới lỏng tham số đó. Bốn method trên không có lớp bảo vệ tương
đương: bất kỳ `Anchor` nào cũng gọi được, kể cả anchor trỏ sang giáo viên
khác.

Rủi ro hiện tại là **tiềm ẩn, chưa phơi ra**: đã grep, không handler nào gọi
`CreateAnchored`, `AddScheduleAnchored`, hay `ActiveOnClass`; caller duy nhất
là `imports/apply.go` (đã gate bằng `imports.run` + OwnerAnchor) và billing
`adjustment.go`. Nhưng đây là loại bề mặt mà một agent hoặc dev sau sẽ nối
thẳng vào handler mà không thấy gì sai.

Đề xuất: hoặc unexport (`createAnchored`) nếu chỉ dùng trong package, hoặc —
với `enrollments.CreateAnchored` — yêu cầu `actor` phải chứng minh quyền ghi
lên `a` (hiện `actor` chỉ dùng cho `ActorID` của event, không phải gate).

**M-4 — D9 mở reach ghi thanh toán, đang chờ owner xác nhận và đã lên
working tree.**
`payments/repository.go:414-437` (`ResolveContactAnchor` bỏ nhánh `teacher_id`).

Đây là quyết định đã ghi (plan.md D9, dòng 60) và có test
(`TestMemberRecordsPaymentForCenterContactWithoutViewAll`). Nhưng plan.md ghi
rõ **"Cần owner xác nhận"** và liệt kê nó ở Open questions #1; phase-01 dòng
102 xếp việc xác nhận trước khi deploy. Code đã nằm trong working tree.

Kèm theo là một bất đối xứng cần nói ra với owner: member có `payments.create`
nhưng không có `payments.view_all` **ghi được** thanh toán rồi **không đọc
lại được** nó qua `GET /payments` (`readScoped` vẫn lọc `teacher_id`). Plan
đã ghi nhận quirk này và chọn không sửa. Response của `Record` thì vẫn trả
allocation của owner qua `AllocationsOf` — cùng quyết định sản phẩm.

Đề xuất: không sửa code. Chặn merge cho tới khi có xác nhận của owner, đúng
như plan yêu cầu.

### Minor

**L-1 — `ListRangeReadable` materialise session cho người chỉ có
`sessions.view_all`.** `sessions/service.go` (`ListRangeReadable` → `materialiseRange`),
bỏ qua gate `classes.GetWritable` mà `ListRange` có.

Một visibility key gián tiếp lái một INSERT. Nhưng đây là materialise suy ra
từ lịch, idempotent, và có test pin chặt:
`TestViewAllMaterialisesSessionsExactlyAsOwnerWould`
(`sessions/integration_test.go`) chứng minh dòng sinh ra mang
`teacher_id` của giáo viên lớp, không phải của người xem, và giống hệt cái
owner sẽ tạo. `TestRepositoriesWidenWritesThroughWriteWideOnly` không thấy nó
vì logic nằm ở service.go chứ không phải repository file — đáng ghi vào
comment của guard.

**L-2 — `contacts.FindIDByPhone` / `students.FindIDByName` giờ center-wide cho
mọi caller.** `contacts/repository.go` (`centerScoped`), `students/repository.go`.

Đúng theo thiết kế dedupe (import chạy bởi member phải khớp contact anchor
owner, nếu không re-import sẽ nhân đôi cả danh bạ). Cả hai chỉ trả về
`uuid.UUID` + bool, không rò field nào. Caller duy nhất là
`imports/apply.go:188` và `:220`; không handler nào chạm tới. Không có vấn đề
thực tế, ghi lại để không bị phát hiện lại như finding mới ở lần review sau.

**L-3 — `students.Create` không có gate `IsOwner` tường minh.**
`students/service.go:64-70`. Không đổi trong diff này; chỉ dựa vào route
permission `students.create` và `checkContact`. Là tiền đề của H-1. Nằm ngoài
phạm vi phase 2.

---

## Verdict: side effects / regressions

**Không có regression an ninh.** Cụ thể, đã kiểm từng nhánh mà lead yêu cầu:

- **5 read helper có nhánh stint-OR** (enrollments, sessions, attendance,
  classes, students): không nhánh nào bị bỏ âm thầm khi chuyển sang Anchor.
  - enrollments → billing dùng `ActiveOnClass` (center-keyed superset), vô hại
    vì nhánh `!hasSessionLine` lọc `e.ID == sessionEnrollmentID && e.StudentID == studentID`.
  - sessions → `readScopedFeed` vốn không có nhánh stint; `anchoredFeed` giống
    hệt hành vi fake-scope cũ.
  - attendance → `ListBySessionAnchored` là superset **có chủ đích**, cần cho
    D5 (xem M-1).
  - classes, students → lookup import-only, trước đó chạy dưới fake scope không
    quyền nên `CenterWideFor` đã false; filter y nguyên.
- **`CreateAnchored` gọi được với anchor không phải owner**: đúng với
  classes/enrollments (M-3), nhưng chưa reachable từ handler nào.
- **`ResolveContactAnchor`**: mở rộng có chủ đích theo D9, chờ owner (M-4).
- **`AllocationsOf` trong `Record`**: đúng — đọc lại theo anchor của contact,
  không theo visibility của caller; cùng quyết định D9.
- **notifications `RunStore` anchors**: `runsOwnScoped(sc)` → `anchored(sc.Self())`
  tương đương từng bit. `runJob.anchor()` giữ nguyên teacher/center của job.
  Không đổi hành vi.
- **statements public render anchors từ token**: `RenderPublic`
  (`service.go:422`) và `touchView` (`public_handler.go:132`) dựng `Anchor`
  literal thẳng từ `stmt.TeacherID/CenterID` của dòng đã resolve, không từ
  input HTTP. Mọi query public vẫn mang đủ 4 khoá
  (center + teacher + contact + period). Không có owner bypass trên đường này.

**Public contracts:** không đổi. `apps/api/docs` (swagger), `migrations/`,
`routes.go` của mọi feature, và DTO đều vắng mặt trong danh sách file thay
đổi. `seeds/seed.go` đúng một dòng (`FindActiveByName(ctx, sc.Self(), …)`),
chính xác. Env/config không đụng tới.

**Pattern conformance:** đạt. `anchored` / `centerScoped` / hậu tố `Anchored`
dùng nhất quán; không có plan ID, số phase, hay mã finding nào trong comment
hoặc tên test; gofmt sạch.

**Test quality:** cao, không phải phantom test. Các test mới đều phân biệt
được đúng/sai:
- `TestAssistantConfirmOnClosedPeriodPostsAdjustmentOnClassTeachersBilling`
  assert `adj.TeacherID == gv.ID` và `Warning == nil`.
- `TestViewAllMaterialisesSessionsExactlyAsOwnerWould` so sánh dòng DB thật
  giữa hai đường.
- `imports/integration_test.go:388-428` assert anchor của contacts/students/
  classes/enrollments **và** `ActorID` của audit event, ba trục khác nhau.
- `contacts/service_test.go:579-583` là compile-time proof cho `OwnerAnchor`.
- `anchor_test.go` dùng reflect để pin cả cái không tồn tại (không có
  `IsOwner`/`Perms`, `NumMethod()==0`, không convert được hai chiều với Scope).

## Recommended Actions

1. Chặn merge cho tới khi owner xác nhận D9 (M-4) — plan yêu cầu, chưa có.
2. Quyết Open question 3 hoặc sửa lý do đã ghi trong phase file (H-1).
3. Đổi signature `ListBySessionAnchored` sang center-keyed và bổ sung hạng mục
   thứ ba vào đoạn "Scope vs Anchor" của docs (M-1).
4. Unexport hoặc thêm proof cho 4 entry point anchored trần (M-3).
5. Vá lỗ guard: root, composite literal lồng, ghi chú giới hạn (M-2).
6. Thêm một dòng comment vào `TestRepositoriesWidenWritesThroughWriteWideOnly`
   nói rõ nó chỉ quét repository file, không thấy write ở service (L-1).

## Unresolved Questions

- `make test-api` và coverage floor chưa được xác minh trong review này.
- Kiểm kê production theo D4 (member nào đang giữ `*.view_all`, đã ghi
  center-wide chưa) là điều kiện deploy của phase 1, chưa thấy kết quả.
