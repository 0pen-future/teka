# Review — secretary report sender, phase 5

Ngày: 2026-08-29. Phạm vi: thay đổi chưa commit của phase 5
(`apps/api/seeds/seed.go`, `apps/api/internal/features/billing/repository.go`
+ `integration_test.go`, `apps/web/e2e/secretary-send.spec.ts`,
`docs/api-guidelines.md`). Read-only, không sửa code.

## Kiểm chứng đã chạy

- `go vet ./...` (apps/api): sạch.
- `golangci-lint run ./...` (apps/api): **1 issue** (xem H1).
- `npm run typecheck` (tsc -b, bao gồm `e2e/` qua `tsconfig.node.json`): sạch.
- `npx eslint e2e/secretary-send.spec.ts`: sạch.
- `gofmt -l seeds internal`: sạch.

## Critical

Không có.

## High

**H1 — Lint gate đang đỏ.**
`internal/features/billing/service_test.go:71` — revive `unused-parameter`:
`func (f *fakeRepository) GetPeriodRead(ctx context.Context, ...)`. Đây là
issue duy nhất của cả module; đổi `ctx` → `_` là xong. Không sửa thì CI lint
fail khi land.

**H2 — `docs/api-guidelines.md` mô tả sai phạm vi đọc đã widen.**
Đoạn mới viết: *"Everything outside that cluster (roster, classes, attendance,
payments) keeps the plain member scoping above."* Sai:

- `contacts/repository.go` — `List` dùng `scopedRead` → `ReportsOversight()`,
  tức thư ký đọc danh bạ (tên + SĐT phụ huynh, PII) của **mọi** giáo viên
  trong center. Chính comment trong file đó gọi endpoint này là "the roster
  List endpoint", nên câu docs tự mâu thuẫn với code.
- `notifications/repository.go` — `ListByPeriod` / `runsPeriodScoped` cũng
  branch trên `ReportsOversight()`, nhưng docs chỉ liệt kê "billing periods,
  statements, and debt views".

Tenancy section là surface authority về mô hình bảo mật; sai ở đây khiến
reviewer/implementer sau này giả định nhầm. Sửa: đưa contacts `List` và
notification ledger vào cluster, ghi rõ `contacts.GetByID` và mọi write vẫn
self-scoped.

## Medium

**M3 — `seedClosedPeriod` phá vỡ quy ước idempotent của seed và làm thao tác
không đảo ngược.** Mọi hàm seed khác đều `count > 0 → skip wholesale`. Hàm
này chạy vô điều kiện. Trên DB dev đã có dữ liệu của Thầy Minh (ví dụ dev tự
tạo buổi chưa điểm danh), `make seed` sẽ hoặc fail cứng
(`ErrUnconfirmedSessions` → `Run` trả error), hoặc **đóng vĩnh viễn** kỳ dev
đang làm việc (close là irreversible). Đề xuất: chỉ close khi lần chạy này
vừa sinh session, hoặc bắt `ErrUnconfirmedSessions` → log warn + skip.

**M4 — Seed/e2e phụ thuộc ngày trong tháng.** Minh chỉ có 1 lớp, lịch thứ Năm
(`Weekday: 4`). Session chỉ sinh cho tháng trước + tháng hiện tại. Nếu seed
chạy từ ngày 1 đến trước thứ Năm đầu tiên của tháng, kỳ hiện tại của Minh
không có buổi quá khứ → `Close` **thành công với 0 invoice** (close.go không
phản đối kỳ rỗng) → statements = 0 → `secretary-send.spec` fail ở assertion
"Chị Yến"/"Anh Sơn". Cửa sổ ~1–6 ngày mỗi tháng. Sửa rẻ nhất: thêm weekday
thứ hai cho lớp của Minh; kèm log/err rõ khi `IssuedCount == 0`.

**M5 — `ListPeriodsRead` không có tie-breaker.** Sort mặc định
`-period_start` (handler.go:103). Từ phase 5 trở đi nhiều giáo viên có kỳ
cùng tháng ⇒ `period_start` trùng hệt ⇒ thứ tự không xác định: phân trang có
thể lặp/nhảy row, và test dùng `.first()` dễ flake. Thêm `id` hoặc
`teacher_id` làm khóa phụ trong ORDER BY. (Phát sinh từ phase 2 nhưng chỉ lộ
ra nhờ seed đa giáo viên của phase 5.)

**M6 — Hai test trong `secretary-send.spec.ts` bị coupling.** Test D8 (dòng
183–184) assert ledger của Minh có "Zalo thủ công"/"Chị Yến" — dữ liệu do
test 1 tạo. Hiện `workers: 1` + `fullyParallel: false` nên thứ tự đúng, nhưng
chạy `--grep D8` hoặc retry lẻ trên DB sạch sẽ fail. Hai assertion đó không
đóng góp gì cho D8 (D8 = *không có* control gửi); bỏ hoặc thay bằng check
không phụ thuộc dữ liệu.

## Low

- **L7** — `GetPeriodByYearMonth` tự dựng query thay vì
  `r.scoped(ctx, sc).Where("billing_periods.teacher_id = ?", sc.TeacherID)`.
  Kết quả tương đương nhưng tách khỏi nguồn chân lý duy nhất về filter
  center/soft-delete → drift risk về sau.
- **L8** — `seeds.Run` không còn lặp `seedTeachers[1:]`, index cứng `[0]`/`[1]`:
  thêm entry vào slice sẽ bị bỏ qua âm thầm, bớt entry sẽ panic.
- **L9** — Comment `secretary-send.spec.ts:170` "His own **open** period's
  review" đã lệch: seed nay đóng kỳ của Minh. Assertion vẫn đúng.
- **L10** — `setSendReportsGrant` dòng 49 assert `getByText("Thư ký gửi báo
  cáo")` không scope theo hàng Cô Thu → pass nhầm nếu member khác giữ cờ.
- **L11** — Cô Thu vào dashboard → `useCurrentPeriod()` → `EnsurePeriod` tạo
  kỳ rỗng cho chính cô ấy, kỳ này hiện trong danh sách center-wide. Nhiễu UX,
  không phải lỗi bảo mật.
- **L12** — `docs/schema_design.sql` chưa có `can_send_reports`. Tiền lệ:
  `audit_logs` cũng chưa từng được thêm ⇒ file này không phải mirror bắt buộc.
  Ghi nhận, không chặn.

## Đã xác minh là đúng (hiệu chỉnh rủi ro)

- **Fix `GetPeriodByYearMonth` là đúng và cần thiết.** `/billing` →
  `BillingIndexRedirect` → `useCurrentPeriod` → `EnsurePeriod`. Không có fix,
  owner Cô Lan có thể bị điều hướng sang kỳ của Minh ⇒ `billing.spec`,
  `collections.spec`, `statement.spec` hỏng. Caller production duy nhất là
  `service.go:87`; 2 caller còn lại nằm trong integration test. Self-pin khớp
  unique index `uq_billing_periods(teacher_id, year, month) WHERE deleted_at
  IS NULL` (migrations/000001:271). Fake repo trong `service_test.go` đã pin
  `sc.TeacherID` nên khớp hành vi mới. Test mới copy đúng convention fixture
  của file (dòng 63–64).
- **Blast radius của seed close.** Chỉ chạm kỳ của Minh; owner giữ
  `pendingAttendanceCount = 2` nên `attendance.spec` và gate "Chưa thể chốt
  sổ" của `billing.spec` không đổi. Thứ tự spec theo alphabet
  (attendance < audit < auth < billing < collections < … < secretary-send <
  statement) không tạo xung đột; `invite-accept` thêm member nhưng selector
  grant/revoke gắn tên "Cô Thu" nên vẫn duy nhất.
- **Không có breaking public contract ngoài D8 đã chấp nhận.** Routes mới
  `POST`/`DELETE /centers/me/members/:teacherId/send-reports` khớp đúng mô tả
  docs; message 403 `"sending reports requires the send-reports permission"`
  khớp D8; `SetSendReports` từ chối owner ngay ở SQL
  (`c.owner_id <> cm.teacher_id`) đúng như docs khẳng định.
- **`scopeFor` trong seed mirror đúng `centers.ResolveScope`** (cùng LEFT JOIN
  `center_members … left_at IS NULL`, cùng `COALESCE(…, FALSE)`).
- Docs bỏ qua `architecture.md` là đúng: file đó không mô tả mô hình
  owner-oversight (grep không có kết quả).

## Trạng thái plan

- `phase-05-…md`: Todo tick đủ, Success Criteria tuyên bố "Every acceptance
  criterion in plan.md checked off".
- `plan.md` dòng 123–139: **toàn bộ acceptance criteria vẫn còn `[ ]`**. Mâu
  thuẫn với tuyên bố trên. Nội dung thì đã có bằng chứng phủ (secretary/D8
  integration test ở 10 feature + `send_reports_integration_test.go` +
  e2e spec), nhưng checkbox cần lead/planner cập nhật — reviewer không sửa
  plan.

## Unresolved questions

1. Kênh `zalo_manual` có tạo `notification_run` không, và run đó kết thúc ở
   trạng thái terminal ngay chứ? Nếu không, migration 000012 (one running run
   per period) có thể làm lần chạy thứ hai của spec trên cùng DB bị 409. Bạn
   báo spec xanh 2 lần liên tiếp ⇒ nhiều khả năng ổn, nhưng nên xác nhận rõ.
2. Việc `contacts.List` widen center-wide cho thư ký là chủ ý sản phẩm, hay
   chỉ là tác dụng phụ để render tên người nhận? Chủ ý ⇒ chỉ sửa docs (H2);
   không chủ ý ⇒ cần thu hẹp lại.
3. Có nên seed tường minh `can_send_reports = false` cho Cô Thu để chống sót
   trạng thái từ lần chạy trước, thay vì chỉ dựa vào `afterEach` revoke?

---

## Triage & resolution (controller, 2026-08-29)

| Finding | Quyết định | Chi tiết |
|---------|-----------|----------|
| H1 lint | **Đã sửa** | `service_test.go:71` `ctx` → `_`; `golangci-lint run ./internal/features/billing/... ./seeds/...` → 0 issues. |
| H2 docs | **Đã sửa** | Bullet read-cluster trong `docs/api-guidelines.md` giờ liệt kê thêm contact list (`GET /contacts`) và notification ledger; ghi rõ `GET /contacts/:id` và mọi write vẫn self-scoped. Đối chiếu `contacts/repository.go:123-124`, `notifications/repository.go:188,243,433`. |
| M3 seed close | **Đã sửa** | `seedClosedPeriod` bắt `billing.ErrUnconfirmedSessions` qua `errors.As` → log warn và để kỳ mở thay vì fail cả seed. Close vẫn chỉ chạy khi kỳ đang mở. |
| M4 calendar flake | **Bác** | Backfill hiện có (`seed.go:772-822`) tạo 1 buổi ad-hoc đã xác nhận trong tháng cho mọi lớp chưa có buổi xác nhận của tháng, bất kể ngày chạy seed → Minh luôn có invoice. Edge duy nhất còn lại (ngày 1 trùng thứ của lớp, DB sạch) thêm weekday thứ hai cũng không vá được; ghi nhận là operational note. |
| M5 sort tie-breaker | **Đã sửa** | `ListPeriodsRead` thêm `Order("billing_periods.id")` như một scope ĐỨNG SAU `p.Scope` (scope chạy lúc Find; Order chain thường sẽ đứng trước sort chính — xác minh bằng GORM DryRun: kết quả `ORDER BY period_start DESC, billing_periods.id`). |
| M6 test coupling | **Giữ, ghi chú** | Assertion ledger là nửa "read-only ledger" của D8. Suite chạy workers:1, file-order → full run luôn có dữ liệu từ test 1; retry cùng run cũng vậy. Comment trong spec giờ ghi rõ dependency và giới hạn `--grep D8` trên DB sạch. |
| L9 comment stale | **Đã sửa** | "open period" → "seeded closed". |
| L7/L8/L10/L11/L12 | **Chấp nhận, không sửa** | Đúng như reviewer đánh giá: không chặn, precedent sẵn có (schema_design.sql vốn không phải nguồn sự thật migration). |
| plan.md checkboxes | **Đã tick** | Dòng 123–139 tick đủ theo bằng chứng test đã ghi trong report tester + review. |

### Unresolved questions — trả lời

1. **`zalo_manual` không tạo run.** `runItems` chỉ nhận phần tử khi `toUID != ""` — chỉ xảy ra ở nhánh `personal` (`notifications/service.go:245-270`); `CreateRun` chỉ chạy khi `len(runItems) > 0` (dòng 299-314). Migration 000012 không thể 409 lần chạy thứ hai. Khớp thực nghiệm: spec xanh 2 lần liên tiếp cùng DB.
2. **`contacts.List` widen là chủ ý** (phase 2 đã duyệt): thư ký cần tên + SĐT người nhận để soạn/gửi tin. Docs đã cập nhật theo (H2).
3. **Không seed tường minh `can_send_reports=false` cho Cô Thu**: cột default false khi insert; spec đã assert-then-set khi grant và revoke ở `afterEach`, chống được trạng thái sót từ lần chạy trước.
