# Architecture Decision Records — Plan Deviations

Ghi lại các điểm thực tế không khớp với plan trong quá trình thực thi, theo
chỉ thị goal: ghi nhận rồi tiếp tục làm việc.

## 2026-08-04 — Plan 01, Phase 1: "16 domain tables" thực tế là 17

Plan (`phase-01-baseline-schema-replacement.md`, step 10 và success criteria)
nói baseline tạo "16 domain tables". Đếm trực tiếp `docs/schema_design.sql`
ra **17 bảng**: user_accounts, teachers, contacts, students, classes,
class_schedules, enrollments, class_sessions, attendance_records,
billing_periods, invoices, invoice_lines, invoice_adjustments, payments,
payment_allocations, statements, notifications.

**Quyết định:** test `migrations_test.go` assert theo danh sách tên bảng đầy đủ
(17 bảng + refresh_tokens + schema_migrations), không assert theo con số 16.
Schema file là nguồn chân lý; con số trong plan là lỗi đếm.

## 2026-08-04 — Plan 01, Phase 1: xác minh migrate/seed bằng testcontainers thay vì dev stack

Plan step 1 và 11 yêu cầu `make dev-nuke` → `make dev` → `make migrate-up` →
`make seed` (2 lần) trên dev stack local. Dev stack không chạy sẵn; boot cả
stack chỉ để xác minh sẽ để lại process nền ngoài phạm vi phiên làm việc.

**Quyết định:** xác minh bằng integration test chạy testcontainers —
`migrations/migrations_test.go` (round-trip up→down→up, FK refresh_tokens →
user_accounts) và `seeds/seed_test.go` (seed 2 lần, lần 2 phải no-op). Cùng
mức bằng chứng, không ghost process. Fixture `testutil` được viết lại ở
phase 2 (khi model `teachers` tồn tại) thay vì phase 1 — plan liệt kê việc này
ở cả hai phase; làm một lần ở phase 2 tránh viết hai lần.

## 2026-08-04 — Plan 01, Phase 2: xoá `features/users/` + `cli/admin.go` ngay ở phase 2 (plan xếp ở phase 3)

Plan xếp việc xoá feature `users` cũ và lệnh `api admin create` vào phase 3.
Nhưng ngay khi `auth` chuyển sang phone-based ở phase 2, package `users`
(email-based, bảng `users` không còn tồn tại) không thể compile cùng schema
mới — giữ lại nghĩa là build đỏ suốt phase 2.

**Quyết định:** `git rm` toàn bộ `internal/features/users/` và
`internal/cli/admin.go` trong phase 2 để giữ build xanh liên tục. Không đổi
kết quả cuối — phase 3 chỉ còn phần chuyển `/me` và cập nhật docs.

## 2026-08-04 — Plan 01, Phase 2: `AccountService` trả `*teachers.Profile` thay vì `*teachers.Account`

Plan phác interface `AccountService` của `auth` quanh account row. Nhưng
`TokenResponse` phải nhúng `full_name` (nằm ở bảng `teachers`), nên nếu chỉ
trả `Account` thì `auth` phải gọi thêm một lượt lấy teacher row.

**Quyết định:** `teachers.Service` trả `*teachers.Profile` (struct nhúng cả
`Account` + `Teacher`); `AccountService` của `auth` khai báo theo đó. Một
lượt gọi, một transaction, response đủ dữ liệu.

## 2026-08-04 — Plan 01, Phase 2: `scoped()` trên bảng identity lọc theo `id`, không phải `teacher_id`

Snippet `scoped()` trong plan lọc `WHERE teacher_id = ?` — đúng cho các bảng
domain (classes, students, …) nhưng sai cho bảng identity: trên
`user_accounts`/`teachers`, khoá tenant chính là primary key, không có cột
`teacher_id`.

**Quyết định:** repo `teachers` implement `scoped()` với `Where("id = ?",
teacherID)`, kèm doc comment ghi rõ dạng chuẩn `teacher_id = ?` để các plan
02–06 copy cho bảng domain. Convention đã ghi vào mục Tenancy của
`docs/api-guidelines.md`.

## 2026-08-04 — Plan 01, Phase 3: gate 401 cho tài khoản đã xoá/disable đặt ở handler

Plan yêu cầu token của tài khoản soft-deleted hoặc disabled phải nhận 401 ở
`/me`. Nhưng `GetByID` trả NotFound cho soft-deleted (gorm.DeletedAt) — map
thẳng ra ngoài sẽ thành 404, sai ngữ nghĩa (lộ thông tin tồn tại tài khoản).

**Quyết định:** handler `teachers` có `currentProfile` gate: NotFound từ
service → 401, và check tường minh `Status != active` → 401. Middleware
`RequireAuth` vẫn stateless (chỉ verify chữ ký JWT); độ trễ thu hồi access
token 15 phút cho các route khác là trade-off đã chấp nhận, riêng `/me`
đối chiếu DB nên chết ngay.

## 2026-08-04 — Plan 01: web e2e (`make e2e`, job e2e của web-ci) đỏ tạm thời cho tới plan 07

`apps/web/e2e/auth.spec.ts` và `users.spec.ts` vẫn login bằng tài khoản email
admin cũ và điều hướng `/users` — API đó đã bị thay theo đúng scope plan 01.
Frontend hiện tại sẽ được thay toàn bộ ở plan 07 (teacher app) + plan 08
(parent statement page), nằm sau chuỗi plan API 02–06.

**Quyết định:** chấp nhận job e2e của web-ci đỏ trong khoảng giữa plan 01 và
plan 07; không vá frontend cũ (sẽ bị xoá) và không tắt gate CI. Nếu cần merge
gì đó phụ thuộc job này trước plan 07, xử lý tại thời điểm đó.

## 2026-08-04 — Plan 02: ngày trong DTO là chuỗi `YYYY-MM-DD`, không phải kiểu Date riêng

Plan phase 3 mô tả các trường ngày (`start_date`, `effective_from`,
`effective_to`) như date. Codebase không có kiểu Date dùng chung; thêm một
custom type mới chỉ để bind/format là YAGNI ở V1.

**Quyết định:** DTO nhận/trả chuỗi `"2006-01-02"` với
`binding:"datetime=2006-01-02"`, convert sang `time.Time` (UTC) ở tầng
service. Model vẫn dùng `time.Time` với cột Postgres `DATE`.

## 2026-08-04 — Plan 02, Phase 3: gộp CloseSchedule vào UpdateSchedule

Plan liệt kê thao tác "close schedule" tách riêng. Về dữ liệu, đóng một dòng
lịch chỉ là set `effective_to`; tách endpoint riêng tạo hai đường ghi cùng
một cột.

**Quyết định:** `PUT /classes/:id/schedules/:scheduleID` nhận cả
`effective_to` — đóng lịch là một dạng update. Service vẫn giữ ngữ nghĩa
"close and replace" (tạo dòng mới, đóng dòng cũ) khi đổi khung giờ.

## 2026-08-04 — Plan 02: key lỗi validation dùng json tag, không dùng tên field Go

Plan viết ví dụ lỗi 422 với key kiểu `FullName`. Shared validator đã đăng ký
`RegisterTagNameFunc` theo json tag từ plan 01, và client chỉ biết tên json.

**Quyết định:** mọi lỗi validation trả fields key theo json tag
(`full_name`, `contact_id`, …). Riêng body JSON sai kiểu (vd `contact_id`
không phải uuid) fail ở tầng decode → 400 BAD_REQUEST theo contract chung,
không phải 422.

## 2026-08-04 — Plan 02, Phase 2: routes students mount ở phase 4 thay vì phase 2

Plan phase 2 yêu cầu mount `students` vào router ngay trong phase. Nhưng
`students.Service` cần một `EnrollmentEnder` thật để delete đóng các ghi danh
đang mở — implementation nằm ở phase 4 (enrollments). Mount sớm nghĩa là phải
chế một ender tạm (no-op hoặc SQL trực tiếp) rồi thay lại ngay sau đó.

**Quyết định:** giữ nguyên interface `students.EnrollmentEnder`
(consumer-defined), toàn bộ feature students hoàn chỉnh và test xanh ở phase
2, nhưng `router.go` chỉ mount students ở phase 4 cùng enrollments (thứ tự
khởi tạo: enrollments service trước, students service nhận nó). Hai phase
nằm trong cùng một commit nên không có trạng thái trung gian nào bị lộ.

## 2026-08-04 — Plan 02: chuẩn "hôm nay" dùng UTC midnight, chưa theo timezone giáo viên

Code review chỉ ra `today()` (enrollments + students) tính theo timezone của
process. Container không set `TZ` nên chạy UTC, trong khi `teachers.timezone`
mặc định `Asia/Ho_Chi_Minh` và chưa được đọc ở đâu. Hệ quả: trong khung
00:00–07:00 giờ VN, kết thúc/xoá ghi danh không có ngày tường minh sẽ đóng với
`ended_on` = hôm qua theo giờ VN — lệch một buổi.

**Quyết định (V1):** giữ chuẩn "hôm nay" = UTC midnight, nhất quán với cách
DTO parse ngày (cột DATE không mang zone). Không resolve theo
`teachers.timezone` ở plan 02 vì đây là quyết định lan tới plan 03 (sinh
session) và plan 04 (tính tiền) — chuẩn "hôm nay" phải thống nhất toàn hệ
thống và nên được chốt một lần ở nơi timezone thực sự ảnh hưởng doanh thu.
Rủi ro thực tế ở plan 02 thấp: chưa có session/attendance nào tồn tại để
lệch buổi. Ghi nhận là việc cần giải quyết trước khi billing (plan 03/04)
dựng lên: hoặc set `TZ=Asia/Ho_Chi_Minh` cho container (V1 single-region),
hoặc thêm helper `today(teacherTZ)`.

## 2026-08-04 — Plan 02, Phase 4: hoãn guard attendance khi xoá enrollment

Review đề xuất `DELETE /enrollments/:id` nên trả 409 khi ghi danh đã có
`attendance_records`, tương tự guard của `DELETE /classes/:id`.

**Quyết định:** hoãn tới plan 03. Bảng `attendance_records` chưa có dữ liệu ở
plan 02 (plan 03 mới sinh session và điểm danh), nên guard hiện tại không bảo
vệ gì cả. Khi plan 03 tạo attendance, guard này sẽ được thêm cùng lúc với
logic sinh session, ở đúng nơi biết một enrollment "đã được dùng" hay chưa.
`DELETE` ở V1 chỉ dành cho ghi danh tạo nhầm (chưa có buổi nào), đúng với mô
tả API.

## 2026-08-04 — Plan 02: sửa 2 lỗi nhỏ từ code review (đã fix trong commit này)

- **End enrollment bỏ qua body khi chunked encoding:** handler đổi từ gate
  `ContentLength > 0` sang bind vô điều kiện và chỉ tha thứ `io.EOF`. Body có
  `ended_on` giờ luôn thắng, không bị âm thầm revert về hôm nay.
- **Double-end song song trả 404 thay vì 409:** `repository.End` khi
  `RowsAffected == 0` kiểm tra hàng có tồn tại không — tồn tại nhưng đã đóng →
  `ErrAlreadyEnded` (409), không có hàng → `ErrNotFound` (404). Kẻ thua trong
  double-submit nhận đúng ngữ nghĩa "đã kết thúc".

Nhóm Low còn lại (phân trang thiếu tiebreaker, ILIKE không escape, validate
`end_date >= start_date` ở classes, ghi danh vào lớp archived, `display_note`
trả `""` cho NULL, docs guidelines chưa nhắc validator mới) được ghi nhận và
hoãn dọn sau — không chặn nghiệm thu plan 02.

## 2026-08-04 — Plan 03: guard trạng thái nguồn cho Uncancel/Hold (đã fix)

Code review chỉ ra `Uncancel`/`Hold` (sessions) không kiểm tra trạng thái
nguồn. Gọi `uncancel` trên một buổi đã `held` + đã confirm sẽ đưa `status` về
`planned` nhưng giữ nguyên `attendance_confirmed_at` và các bản ghi điểm danh —
tạo ra buổi "planned nhưng đã confirmed". Vì `CountBillableByEnrollment` (điểm
vào của plan 04) chỉ đếm `status = 'held'`, buổi bị lật sẽ rơi khỏi phần tính
tiền → thu thiếu. `Hold` cũng không chặn buổi `cancelled` nên tạo buổi `held`
còn ôm `cancel_reason` cũ.

**Quyết định (đã fix trong commit này):** `Uncancel` yêu cầu nguồn phải là
`cancelled` (khác → 409 `ErrInvalidTransition`); `Hold` từ chối buổi
`cancelled` (buộc uncancel trước) và truyền `nil` reason để dọn `cancel_reason`
cũ. Đúng theo bảng lifecycle ở phase-01 (`cancelled → planned` là chuyển hợp
lệ duy nhất từ cancelled). Có test unit cho cả hai path guard.

Nhóm Low hoãn (không chặn nghiệm thu plan 03):
- `CountBillableByEnrollment` chưa có consumer/test khẳng định predicate — sẽ
  được phủ khi plan 04 dựng consumer (đúng nơi biết hành vi mong đợi).
- Ad-hoc session không clamp theo `[start_date, end_date]` của lớp — cố ý cho
  buổi bù (phase-01), chỉ chặn trùng ngày qua unique index.
- Comment route `/sessions/pending` mô tả cơ chế Gin hơi sai bản chất (ưu tiên
  theo loại node, không phải thứ tự đăng ký); vô hại, route đúng + có test
  literal-path.
- `Confirm` giải roster hai lần (một truy vấn thừa mỗi lần confirm, bounded).

## 2026-08-04 — Plan 04, Phase 1: chữ ký entry point đếm billable khác plan

Plan 04 (phase 1 + phase 4) giả định `attendance.CountBillableByEnrollment(ctx,
teacherID, from, to)` trả tally **nhiều enrollment** trong một cửa sổ ngày, kèm
`billable_count`, `absent_count`, `present_count`. Thực tế plan 03 build
`CountBillableByEnrollment(ctx, teacherID, enrollmentID, from, to) (int64,
error)` — theo **một** enrollment và chỉ trả billable count. Ngoài ra template
layout plan 04 trỏ tới `internal/features/users/` đã bị xoá ở plan 01.

**Quyết định:** thêm entry point batched `attendance.TallyByEnrollment(ctx,
teacherID uuid.UUID, from, to time.Time) ([]attendance.EnrollmentTally, error)`
(EnrollmentTally = EnrollmentID, BillableCount, AbsentCount, PresentCount) —
đúng ý đồ plan ("một call vào attendance, một metadata query billing sở hữu,
zip theo enrollment_id"), một grouped query giữ toàn bộ luật đếm (status='held'
AND attendance_confirmed_at IS NOT NULL, deleted_at IS NULL trên cả record lẫn
session, cancelled loại trừ) ở đúng một nơi. `CountBillableByEnrollment` chưa
có consumer nào nên gỡ bỏ để tránh hai aggregate song song (đúng nguyên tắc
plan: "luật đếm tồn tại ở đúng một chỗ"). Template dùng các package hiện có
(`enrollments`/`sessions`/`attendance`) thay cho `users/` đã xoá; teacher id
lấy qua `authctx.TeacherID(c)`.

## 2026-08-04 — Plan 04, Phase 1: hai chỉnh sửa do linter (không đụng tiền)

1. **Đổi tên type `BillingPeriod` → `Period`.** `revive` no-stutter chặn
   `billing.BillingPeriod`; mọi package feature khác đặt tên model theo số ít
   của package (`sessions.Session`, `enrollments.Enrollment`,
   `classes.Class`). Đổi tên cho khớp quy ước và pass `make lint-api`; tên
   bảng giữ nguyên qua `TableName() → "billing_periods"`, không đổi schema.
2. **`gosec` G115 trên cast `int`→`int16` trong `EnsurePeriod`.** Thêm kiểm
   tra biên `year` 2020-2100 / `month` 1-12 (đúng ràng buộc binding của
   `EnsurePeriodRequest`) trước các cast, kèm chú thích `#nosec G115` viện dẫn
   kiểm tra đó. Đây là validation thật cho method exported khi gọi ngoài HTTP,
   không phải bịt cảnh báo. Không có tiền lệ `#nosec` trước đó trong repo.

## 2026-08-04 — Plan 04, Phase 2: `PreviewLine` cần `class_id`/`present_count` mà không có field nào mang sẵn

`ComputedLine` (calculator.go, phase 1) không có `ClassID`/`PresentCount`
(tối giản theo YAGNI của phase 1 vì lúc đó chưa có consumer nào cần). Bảng
`invoice_lines` persisted cũng không có hai cột này — đọc lại từ đó sau khi
ghi cũng không cứu được.

**Quyết định:** `ComputePeriod` (preview.go, mới) giữ một map phụ
`TalliesByEnrollment map[uuid.UUID]AttendanceTally` song song với
`[]ComputedInvoice`. `buildPreviewResponse` (dto.go) build từng `PreviewLine`
từ `ComputedLine` rồi lấy `class_id`/`present_count` từ tally cùng
`enrollment_id` trong map đó. Preview và Draft đều serialize từ cùng kết quả
compute trong bộ nhớ này — không endpoint nào đọc lại `invoice_lines` thô để
dựng response.

## 2026-08-04 — Plan 04, Phase 2: `AdjustmentTotals` khoá theo `invoice_id`, `Compute` cần khoá theo `student_id`

`Compute(tallies, opening, adjustments)` (phase 1) nhận `adjustments
map[uuid.UUID]int64` khoá theo `student_id` — hợp lý cho hàm thuần không biết
gì về bảng `invoices`. Nhưng dữ liệu thật (`invoice_adjustments.invoice_id`)
chỉ khoá được theo `invoice_id`, và một invoice chỉ tồn tại sau khi đã draft
lần đầu.

**Quyết định:** `Repository.AdjustmentTotals` trả `map[invoice_id]int64`
(đúng khoá tự nhiên của bảng). `ComputePeriod` tự liệt kê `ListInvoices` của
kỳ (mới, thêm vào `Repository`) để dựng `invoice_id → student_id`, remap
`AdjustmentTotals` sang `student_id` rồi mới gọi `Compute`. Lần preview/draft
đầu tiên của một kỳ: `ListInvoices` rỗng → map remap rỗng → đúng ngữ nghĩa
(chưa có invoice thì chưa thể có adjustment).

## 2026-08-04 — Plan 04, Phase 2: gộp bước 1 (ghi invoice) và bước 4 (áp adjustment_total) của Architecture thành một lượt ghi

Architecture doc mô tả 4 bước tuần tự: ghi invoice → ghi lines → zero lines
thừa → đọc lại adjustment_total và áp vào invoice. Đọc-sau-ghi ở bước 4 tạo
một round trip thừa, và vì CHECK `total_due = opening_balance +
current_charge + adjustment_total` không cho phép invoice tồn tại tạm thời ở
trạng thái sai tổng, bước 4 tách rời thực ra buộc phải là một UPDATE thứ hai
ngay sau INSERT/UPDATE ở bước 1 — hai lần ghi cho cùng một hàng trong cùng một
request.

**Quyết định:** `DraftPeriod` (preview.go) đọc `AdjustmentTotals` một lần
trong transaction *trước* khi build từng `*Invoice`, tính `total_due` đúng
ngay trong struct ban đầu, rồi mới gọi `UpsertInvoice` — một lượt ghi mỗi
invoice thay vì hai. Không đổi kết quả cuối: `adjustment_total` của một
invoice hoàn toàn không phụ thuộc `invoice_lines` (khác nguồn dữ liệu), và một
invoice vừa tạo mới trong chính request này chưa thể có adjustment nào (FK
`invoice_adjustments.invoice_id` bắt buộc invoice đã tồn tại từ trước) — nên
gộp bước không bỏ sót adjustment nào, kể cả trên đường tạo-mới.

## 2026-08-04 — Plan 04, Phase 2: `ComputePeriod` phải tái xét invoice đã tồn tại dù kỳ này không còn tally/carried-debt/adjustment nào cho học sinh đó

Phát hiện qua integration test "sửa điểm danh rồi draft lại": học sinh đã có
invoice draft từ lần trước (2 buổi điểm danh), sau đó cả hai attendance_record
bị xoá mềm (sửa sai). Draft lại: `TallyAttendance` trả 0 dòng cho enrollment
đó → học sinh không còn xuất hiện trong `tallies`, không có carried-debt (kỳ
đầu tiên), không có adjustment. `Compute()` chỉ sinh `ComputedInvoice` cho
student_id có mặt trong `tallies`, `opening`, hoặc `adjustments` — học sinh
này rơi khỏi cả ba, nên hoàn toàn biến mất khỏi kết quả compute dù invoice cũ
của họ vẫn còn trong DB với số tiền cũ (sai) và line cũ chưa bị zero.

**Quyết định:** `ComputePeriod` sau khi build `adjustmentsByStudent`, thêm
một lượt: với mọi `student_id` đã có invoice ở kỳ này (`existingByStudent`)
mà chưa là key trong `adjustmentsByStudent`, chèn thêm entry giá trị 0. Đây
không phải chỉnh sửa số liệu — giá trị đọc ra vẫn là 0 giống hệt trước đó khi
"absent key" — chỉ là buộc `Compute()` đi vào nhánh "extra" của nó (dùng đúng
cơ chế phase 1 đã có sẵn cho học sinh chỉ có carried-debt/adjustment, không
thêm nhánh mới) để invoice cũ được tái xét và `DraftPeriod` gọi
`ZeroUnmatchedLines` zero đúng line thay vì để invoice đứng yên với số tiền
cũ đã sai.

## 2026-08-04 — Plan 04, Phase 3: cửa sổ cảnh báo tương lai không lấy được từ `ListPending` nguyên trạng

`sessions.Service.ListPending` (plan 03 phase 3) khoá cứng `before=today`
(dateOnly của "hôm nay" theo múi giờ giáo viên) ngay trong hàm — hợp lý cho
dashboard, nhưng loại bỏ hoàn toàn buổi học nằm trong tương lai. Close pipeline
cần cả hai cửa sổ: buổi quá khứ chưa xác nhận (chặn đóng sổ, from=period_start,
to=min(period_end, today)) và buổi tương lai chưa xác nhận trong kỳ (chỉ cảnh
báo, from=today+1, to=period_end) — cửa sổ thứ hai không thể diễn đạt bằng
`before=today` cố định.

**Quyết định:** thêm một method mới `sessions.Service.ListUnconfirmedInWindow(
ctx, teacherID uuid.UUID, from, to *time.Time, before time.Time, limit int)
(*PendingResponse, error)` tái dùng đúng predicate của
`repo.ListPending` (teacher-scoped, `session_date < before`,
`attendance_confirmed_at IS NULL`, `status IN (held, planned)`,
`deleted_at IS NULL`) nhưng nhận `before` tường minh từ caller thay vì tự tính
`today`. `ListPending` refactor thành gọi `ListUnconfirmedInWindow` với
`before=today` tự tính như cũ — hành vi byte-identical, không đổi test hiện
có. Billing khai báo interface tiêu thụ hẹp `PendingSource` (một method
`ListUnconfirmedInWindow`) mà `*sessions.Service` thoả mãn; billing phụ thuộc
interface đó, không phụ thuộc `sessions.Repository`. Billing không viết bất kỳ
query quét session nào của riêng nó — đúng yêu cầu "no session-scanning method"
của phase 3.

`DaysOverdue` trong `PendingSessionResponse` (dùng bởi cả hai lời gọi) được
tính theo đúng `before` truyền vào — với lời gọi cảnh báo tương lai
(`before=period_end+1 ngày`) con số này không phải "số ngày trễ tính từ hôm
nay thật", nhưng `UnconfirmedSession` (DTO billing dùng cho payload 409 và
cảnh báo) không có field `days_overdue`, nên sai lệch này không lộ ra response
nào — chỉ ảnh hưởng nội bộ một response tạm thời billing map rồi bỏ.

## 2026-08-04 — Plan 04, Phase 3: 409 payload cần danh sách session, `response.ErrorBody.Fields` chỉ là `map[string]string`

R4 yêu cầu response chặn đóng sổ "chỉ rõ buổi nào" — cần một danh sách object
(session_id, class_id, class_name, session_date, status), không phải map
string phẳng `apperror.AppError.Fields` đang hỗ trợ. Widen `AppError` cho một
caller duy nhất sẽ ảnh hưởng toàn bộ error contract của các feature khác.

**Quyết định:** không đổi `apperror.AppError`. Thêm field `Details any
\`json:"details,omitempty"\`` vào `response.ErrorBody` và hàm
`response.ErrWithDetails(c *gin.Context, appErr *apperror.AppError, details
any)` cạnh `response.Err` hiện có — cùng cơ chế log lỗi 5xx, chỉ thêm details
vào envelope. Handler `close` bắt riêng lỗi chặn (kiểu `ErrUnconfirmedSessions`
định nghĩa trong `close.go`) và gọi `ErrWithDetails` với
`{"unconfirmed_sessions": [...]}`; mọi lỗi khác vẫn qua `response.Err` như cũ.
Swagger được generate lại sau (`make api-docs`).

## 2026-08-04 — Plan 04, Phase 3: `billing.NewService` nhận thêm `PendingSource`; thêm `VoidInvoice` vào Repository dù không có trong danh sách liệt kê

`NewService` đổi chữ ký thành `NewService(repo Repository, tx
database.TxManager, pending PendingSource) *Service`; `router.go` truyền
`sessionsSvc` (đã tồn tại đúng thứ tự khởi tạo, không cần đổi chỗ). Mọi test
dựng `Service` (service_test.go, preview_test.go, close_test.go mới) truyền
thêm một `fakePendingSource`.

Danh sách "Modify: repository.go" của phase 3 liệt kê `LockPeriod,
IssueDraftInvoices, VoidInvoices, ClosePeriod, GetInvoice` nhưng không có
method ghi void cho một invoice đơn lẻ — trong khi `Service.VoidInvoice` (được
yêu cầu tường minh ở Architecture và Implementation Steps) bắt buộc phải ghi
xuống DB. Đây là thiếu sót liệt kê, không phải chỉ thị tránh thêm method.
Thêm `Repository.VoidInvoice(ctx, teacherID, invoiceID uuid.UUID, reason
string, at time.Time) error` (UPDATE một hàng, guard `status IN (issued,
partially_paid)`) để hiện thực đúng ý đồ của Architecture. Tương tự,
`InvoiceResponse`/`FromInvoiceModel` (dto.go) được thêm làm response tối giản
cho `voidInvoice` — phase 2 chưa cần response invoice đơn lẻ nào, endpoint chi
tiết đầy đủ (kèm lines) là việc của phase sau.

## 2026-08-04 — Plan 04, Phase 4: `attendance.CountBillableByEnrollment` không tồn tại; billing tự lọc `TallyByEnrollment`

Phase 4's Architecture/Implementation Steps (bước 6) mô tả `LiveBillableCounts`
như "a filtered call to `attendance.CountBillableByEnrollment` over the period
window". Method đó chưa từng tồn tại: phase 1 (xem entry phía trên, "chữ ký
entry point đếm billable khác plan") đã build và giữ lại đúng một entry point
đếm, `attendance.Service.TallyByEnrollment(ctx, teacherID, from, to)
([]EnrollmentTally, error)` — batched theo mọi enrollment trong cửa sổ ngày,
không nhận danh sách enrollment cụ thể.

**Quyết định:** `Repository.LiveBillableCounts(ctx, teacherID, enrollmentIDs,
period)` gọi đúng `TallyByEnrollment` (qua `AttendanceSource`, interface
billing tự khai báo) rồi lọc kết quả xuống còn `enrollmentIDs` billing cần,
trả `map[enrollment_id]billable_count`. Billing không viết thêm một query đếm
thứ hai lên `attendance_records` — đúng tinh thần của Implementation Steps
("Billing writes no second counting query, so `live_charge` and
`current_charge` can never be computed by two different rules"), chỉ khác ở
chỗ bộ lọc nằm ở phía billing (sau khi nhận kết quả) thay vì ở phía attendance
(qua tham số enrollment list truyền vào).

## 2026-08-04 — Plan 04, Phase 4: `BillingReconciler`/`Reconciliation` khai báo trong `attendance`, không phải `billing`, để tránh import cycle

`billing` đã import `attendance` từ phase 1 (cho `TallyByEnrollment`) và phase
3 (cho pending feed). `ReconcileSession` — method billing implement để phản
hồi một thay đổi điểm danh đã commit — buộc `attendance.Service` phải giữ một
tham chiếu ngược về billing. Go không cho phép hai package import lẫn nhau.

**Quyết định:** interface callback `BillingReconciler` và các kiểu dữ liệu nó
trả về (`Reconciliation`, `ReconciliationEntry`) khai báo trong package
`attendance` (đúng như Architecture đã ghi rõ), không phải `billing`.
`billing.Service` implement interface đó một cách cấu trúc (không import
ngược `attendance.BillingReconciler` — implicit satisfaction). `attendance.
Service` giữ field `reconciler BillingReconciler` (nil-able, mặc định nil nếu
không ai gọi `SetReconciler`) và gọi nó sau khi `Confirm` commit thành công;
lỗi từ `ReconcileSession` không rollback điểm danh đã ghi (không thể, đã
commit) mà chỉ nổi lên như `Response.Warning *string` — không phải 5xx.
`registerFeatures` (router.go) nối hai service lại sau khi cả hai đã khởi tạo:
`billingSvc := billing.NewService(...)` rồi `attendanceSvc.SetReconciler(billingSvc)`.
Kết quả: `attendance` không bao giờ import `billing` (xác nhận bằng
`grep -r "features/billing" internal/features/attendance` rỗng).

## 2026-08-04 — Plan 04, Phase 4: `deriveInvoiceStatus` là bản tạm của billing, chưa phải bản chính thức plan 05 sở hữu

Architecture (bước "recompute in the same transaction") ghi "status = derived
from paid_amount vs total_due (plan 05 helper)" — plan 05 (payments) chưa
build, chưa có hàm đó để gọi.

**Quyết định:** implement `deriveInvoiceStatus(currentStatus string,
paidAmount, totalDue int64) string` cục bộ trong `billing/adjustment.go`: chỉ
chuyển trạng thái trong nhóm issued/partially_paid/paid theo paid_amount so
với total_due, không bao giờ đụng draft hay void (hai trạng thái đó do
`Draft`/`VoidInvoice` sở hữu riêng). Hàm này có unit test riêng
(`TestDeriveInvoiceStatusNeverTouchesDraftOrVoid`,
`TestDeriveInvoiceStatusTransitionsAmongIssuedPartiallyPaidPaid`) nhưng
**không** được `RecalcInvoiceTotals` gọi trực tiếp — `RecalcInvoiceTotals`
(repository.go) mirror cùng luật đó bằng SQL `CASE` ngay trong một `UPDATE ...
FROM (SELECT sum ...)` duy nhất, để `adjustment_total`, `total_due`, và
`status` cùng commit trong một lượt ghi atomic (không thể vừa đọc `total_due`
mới vừa gọi hàm Go rồi ghi lại mà không tạo ra một cửa sổ ghi từng phần vi
phạm CHECK `total_due = opening_balance + current_charge + adjustment_total`).
Hệ quả: luật status hiện tồn tại ở hai nơi (Go và SQL) phải giữ đồng bộ thủ
công — rủi ro trôi luật được chấp nhận có ý thức vì `deriveInvoiceStatus` là
deliverable tường minh của phase 4 và là bản để plan 05 thay thế toàn bộ, không
phải nguồn ghi dữ liệu thật.

## 2026-08-04 — Plan 04, Phase 4: `billing.NewService` nhận thêm `EnrollmentSource`; `ensureAdjustmentTarget` đọc `class_sessions`/`students` trực tiếp

Hai việc Architecture yêu cầu không có sẵn dependency nào billing đang giữ
đáp ứng được:

1. **Trường hợp hiếm "enrollment có điểm danh trong `P` nhưng không có dòng
   trên `I`"** (học sinh join giữa kỳ, backfill điểm danh sau khi kỳ đã đóng)
   bắt buộc xác nhận roster qua đúng
   `enrollments.ActiveOn(ctx, teacherID, classID, session.session_date)` —
   Architecture ghi rõ "never a hand-written started_on/ended_on comparison".
   Không có tiền lệ direct-table-read nào thay thế được vì đây là một luật
   nghiệp vụ (roster membership), không phải một metadata join.
2. **`ensureAdjustmentTarget`** cần tên lớp của session đang sửa (cho
   `adjustmentReason`) và snapshot tên học sinh/liên hệ hiện tại (khi phải
   tạo invoice draft mới trên kỳ đích) — cả hai đều là metadata thuần.

**Quyết định:** (1) thêm interface `billing.EnrollmentSource` (billing tự
khai báo, `*enrollments.Service` thoả mãn cấu trúc) và tham số thứ 4 cho
`NewService(repo, tx, pending, enrollmentSource)`. Điểm chạm đổi: router.go,
service_test.go, preview_test.go, integration_test.go (tất cả gọi
`NewService` bằng constructor); `close_test.go` không đổi vì nó dựng
`Service{}` bằng struct literal. (2) thêm `Repository.SessionMeta` (join
`class_sessions`→`classes`) và `Repository.StudentSnapshot` (join
`students`→`contacts`) như hai method đọc bảng trực tiếp — theo đúng tiền lệ
`TallyAttendance`/`CarriedDebtStudents` (phase 1) đã dùng cho metadata của
feature khác — thay vì thêm dependency service mới, vì đây chỉ là hai lượt
đọc tên hiển thị, không phải luật nghiệp vụ.

## 2026-08-04 — Plan 04, Phase 4: cách hiểu "kỳ của tháng hiện tại... nếu đã đóng thì tháng kế tiếp" trong bước 2 của `ensureAdjustmentTarget`

Architecture bước 2 viết: "ensure the period for the current calendar month
(teacher timezone) when it is after P; if that month is already closed,
ensure the following month" — không nói rõ phải làm gì khi tháng hiện tại
KHÔNG sau P (trường hợp hiếm: sửa điểm danh của một kỳ đã đóng từ rất lâu,
trong khi hệ thống đã kịp mở và đóng thêm các kỳ ở giữa nhưng không kỳ nào
trong số đó còn ở trạng thái `open` — nói cách khác `NextOpenPeriod` trả rỗng
dù có kỳ nằm giữa P và hiện tại).

**Quyết định** (`resolveTargetPeriod`, adjustment.go): thử `NextOpenPeriod`
trước (kỳ `open` sớm nhất sau `P.PeriodEnd`); nếu không có, lấy "hôm nay" theo
timezone giáo viên (`teacherLocation`, tái dùng từ phase 1/3) — nếu tháng hiện
tại sau `P` thì dùng chính tháng đó làm ứng viên, ngược lại nhảy thẳng tới
tháng kế tiếp `P` (không dùng tháng hiện tại); sau đó `EnsurePeriod` ứng viên
đó, và nếu nó hoá ra đã `closed`, nhảy thêm một tháng kế tiếp và
`EnsurePeriod` lần nữa. Cách này giữ đúng bất biến "kỳ đích luôn sau P và luôn
`open` khi trả về" cho mọi trường hợp, kể cả trường hợp Architecture không nói
rõ.

## 2026-08-04 — Plan 04, review chốt sổ: chống double-count khi reconcile đồng thời + đồng bộ validate lý do void

Code review toàn diff plan 04 (money/immutability/tenancy) xác nhận các bất
biến lõi đều đúng cho luồng tuần tự; phát sinh một số điểm ngoài phạm vi mô tả
của plan, xử lý như sau.

**H1 — race double-count khi reconcile đồng thời (đã vá).** `reconcileStudent`
đọc `already_adj` (`AdjustmentsBySourcePeriod`) rồi mới post adjustment, không
có row lock; dưới READ COMMITTED (mặc định của `database.TxManager.WithinTx`)
hai `ReconcileSession` cho cùng student + cùng kỳ đã đóng (ví dụ giáo viên xác
nhận hai buổi khác nhau của một học sinh gần như đồng thời) đều đọc
`already_adj` cũ rồi cùng post một delta → phụ huynh bị tính dư. Trường hợp
tuần tự ("sửa đi sửa lại cùng một buổi") vốn đã đúng nhờ số học tích luỹ; đây
là cạnh còn lại của tính đồng thời.

**Quyết định:** thêm `Repository.LockInvoice` (`SELECT ... FOR UPDATE` trên
hoá đơn của kỳ đã đóng — natural key của cặp `(student, period)`) và gọi nó
trong `reconcileStudent` TRƯỚC khi đọc `already_adj`; lần reconcile thứ hai
block tại đây tới khi lần đầu commit, rồi đọc `already_adj` đã bao gồm carry
đầu tiên nên tự thành no-op. `ReconcileSession` duyệt student theo thứ tự đã
sort (theo string id) để hai giao dịch chồng lấn không thể ôm hai row lock
theo thứ tự ngược nhau mà deadlock. Regression:
`TestConcurrentReconcileSameStudentDoesNotDoubleCount` (flip cả hai buổi →
non-billable, reconcile song song, khẳng định carry net đúng một lần
−200.000, không phải −400.000), chạy xanh dưới `-race`.

**L4 — `Service.VoidInvoice` thiếu validate lý do ở tầng Go (đã vá).** Cột
`invoices.void_reason` không có CHECK ở DB (chỉ `invoice_adjustments` có), và
`VoidInvoice` chỉ dựa vào binding `min=3,max=500` của handler — một caller
không qua Gin có thể ghi lý do rỗng. Thêm `validateAdjustmentReason(reason)`
đầu `VoidInvoice` cho đồng bộ với `AddAdjustment` (cùng bound 3–500, cùng field
key `reason`).

**M2 / M3 / L5 — giới hạn đã biết, giữ nguyên cho V1 (ghi nhận, không đổi
hành vi).** (M2) Gỡ học sinh khỏi một buổi của kỳ đã đóng (`SoftDeleteMissing`
trong `Confirm`) không tự sinh carry giảm trừ — học sinh biến mất khỏi tập
attendance sống nên reconcile không thấy; sửa bằng adjustment thủ công. (M3)
Attendance mới trở nên billable trên một hoá đơn kỳ đã đóng vốn bị void/không
có → `reconcileStudent` trả `nil` (Architecture: skip), là under-bill thầm cho
một chỉnh sửa thật; cũng sửa bằng adjustment thủ công. (L5) Reconcile lỗi được
gộp thành `Warning` trong response chứ không có hàng đợi retry bền — vì số học
tích luỹ nên một lần sửa sau tự chữa; nếu không có lần sửa nào thì cần rà thủ
công. Cả ba là quyết định sản phẩm (auto-credit/charge hay giữ đường sửa thủ
công) vượt phạm vi plan 04; ghi lại để chủ động quyết ở plan 05/06, không chặn
giao hàng plan 04.

## 2026-08-04 — Plan 05, Phase 1: template layout trỏ `internal/features/users/` đã bị xoá; dùng `internal/features/contacts/` thay thế

Phase file (`phase-01-payment-recording-and-auto-allocation.md`) tham chiếu
`internal/features/users/` làm khuôn mẫu bố cục file (model/repository/
service/dto/handler/routes/errors + test). Package đó đã bị `git rm` từ plan
01 phase 2 (xem entry phía trên) khi `auth` chuyển sang phone-based; không còn
tồn tại để soi theo.

**Quyết định:** dùng `internal/features/contacts/` làm khuôn mẫu — cùng hình
dạng file, cùng convention xử lý lỗi (`apperror`), pagination, tenancy qua
`authctx.TeacherID`, và là package feature gần nhất về độ phức tạp (CRUD +
soft delete) còn tồn tại trong repo. Không đổi kết quả cuối, chỉ đổi nguồn
tham chiếu bố cục.

## 2026-08-04 — Plan 05, Phase 1: `ContactExists` không lọc `deleted_at`

Phase file không nói rõ payments write path có nên chặn ghi nhận thanh toán
cho một contact đã soft-delete hay không.

**Quyết định:** áp dụng đúng ngoại lệ D4 đã ghi ở plan 04 ("nợ của contact đã
xoá mềm vẫn phải thu được") sang chiều ngược lại — `ContactExists` (repository
mới của payments) không lọc `deleted_at`, nên `Record` vẫn nhận thanh toán cho
một contact đã xoá mềm miễn còn thuộc đúng giáo viên. Chặn ghi nhận thanh toán
chỉ vì contact bị xoá mềm sẽ mâu thuẫn trực tiếp với lý do D4 tồn tại: xoá mềm
là dọn danh sách hiển thị, không phải xoá nợ.

## 2026-08-04 — Plan 05, Phase 1: thêm hai method repository (`ListAllocations`, `ListAllocationsForPayments`) ngoài danh sách liệt kê ở bước 4

Bước 4 của phase file liệt kê interface `Repository` với 7 method
(`CreatePayment`, `GetPayment`, `ListPayments`, `CandidateInvoices`,
`InsertAllocations`, `RecalcInvoicePaid`, `ContactExists`). Nhưng bước 6 (hình
dạng response) đòi `AllocationResponse` phải mang `invoice_id`, `student_id`,
`student_name`, `period_id`, `total_due`, `paid_amount` — dữ liệu chỉ có được
qua join `payment_allocations` → `invoices`, không method nào trong 7 cái trên
trả được.

**Quyết định:** thêm `ListAllocations` (một payment) và
`ListAllocationsForPayments` (nhiều payment cùng lúc, dùng ở `List` để tránh
N+1) — cùng một `allocationRowSelect` SQL dùng chung. Đây là mở rộng cần thiết
để thoả mãn hợp đồng response mà bước 4 không liệt kê, không phải method thừa.

## 2026-08-04 — Plan 05, Phase 1: `queryDate` — helper query param ngày mới, chưa có tiền lệ ở feature khác

`GET /payments` cần filter khoảng ngày `received_from`/`received_to` (bước 6).
Các handler hiện có (`queryUUID` ở enrollments/attendance) chỉ có tiền lệ parse
uuid optional; chưa có helper parse ngày optional nào để tái dùng.

**Quyết định:** thêm `queryDate(c, name) (*time.Time, bool)` trong
`payments/handler.go`, cùng khuôn dạng lỗi với `queryUUID` (absent = unset,
malformed = 422 nêu tên field), dùng `dateLayout` ("2006-01-02") đã có sẵn ở
`dto.go`. Cục bộ trong package payments; chưa nâng lên shared vì mới có một
consumer.

## 2026-08-04 — Plan 05, Phase 2: sửa `recalcInvoicePaidQuery` (phase file nói giữ nguyên)

Phase file ghi rõ `recalcInvoicePaidQuery` ở `repository.go` giữ nguyên từ
phase 1, chỉ tái dùng. Nhưng test tích hợp
`TestReallocateRebalancesATwoChildSplitOntoOneInvoice` phát hiện câu SQL này
có lỗi tiềm ẩn: derived table `x` chỉ có hàng khi hoá đơn còn ít nhất một dòng
allocation; mệnh đề `UPDATE invoices i ... FROM (subquery) x WHERE i.id = ?
AND i.teacher_id = ?` không có điều kiện join nào giữa `i` và `x`, nên khi
Reallocate xoá sạch allocation của một hoá đơn (đưa nó về 0), `x` trả về 0
hàng, phép cross-join giữa `i` (1 hàng) và `x` (0 hàng) ra 0 hàng, và UPDATE
âm thầm không khớp hàng nào — hoá đơn giữ nguyên `status`/`paid_amount` cũ
(vd: vẫn "paid" dù đã bị rút hết tiền). Lỗi này nằm im ở phase 1 vì `Record`
chỉ insert allocation, chưa bao giờ gọi `RecalcInvoicePaid` trên một hoá đơn
có 0 dòng allocation; `DeleteAllocations` (Reallocate, phase 2) là đường đi
đầu tiên tạo ra hoàn cảnh đó.

**Quyết định:** sửa câu SQL — chốt một hàng neo `target` cho đúng
`invoiceID` trước, rồi `LEFT JOIN` bảng tổng hợp allocation vào `target` thay
vì để `i` cross-join thẳng với subquery. `COALESCE(x.paid, 0)` giờ luôn tính
được kể cả khi không còn allocation nào, và `UPDATE` luôn khớp đúng một hàng
hoá đơn. Thứ tự 3 tham số gọi `Exec` (`invoiceID, invoiceID, teacherID`) giữ
nguyên, không đổi call site nào khác. Đây là sửa lỗi thật trong file thuộc
quyền sở hữu của phase 2 (`repository.go`), không phải viết lại thiết kế.

## 2026-08-04 — Plan 05, Phase 3: kịch bản test collection board — `status=unpaid` không thể trả về một contact vừa "underpaid"

Phase file (`phase-03-collection-board.md`, bước 7) mô tả contact B "đóng
thiếu nên một con `partially_paid`" rồi liệt kê tiêu chí "`status=unpaid` trả
về đúng B, C và D". Nhưng công thức `payment_status` cũng do chính phase file
đặt ra (bước 4): `total_paid == 0` mới là "unpaid", còn lại là "partial" khi
còn nợ. Một contact có bất kỳ con nào `partially_paid` — tức hoá đơn đó có
`paid_amount > 0` — luôn kéo `total_paid` cấp contact lên khác 0, nên B không
bao giờ tính ra "unpaid" theo đúng công thức đã nêu. Đây là mâu thuẫn nội tại
của phase file, không phải một cách diễn giải khác đi tới cùng kết quả — đã
thử mọi cách dựng số liệu cho B (một con trả đủ/một con thiếu, hay cả hai con
đều thiếu một phần) và `total_paid` cấp contact luôn khác 0 khi có bất kỳ
khoản thanh toán nào được ghi nhận.

**Quyết định:** triển khai `payment_status` đúng y nguyên công thức ở bước 4
(nguồn tham chiếu duy nhất, cũng là công thức tái dùng cho lọc theo trạng thái
lẫn hiển thị theo lớp — nơi B2 phải hiện `partially_paid` để giáo viên biết
đúng đứa trẻ nào còn thiếu, tiêu chí kiến trúc quan trọng hơn). Test tích hợp
(`integration_test.go`,
`TestContactViewMergesFamiliesAndClassViewShowsPerChildStatus`) dựng đúng kịch
bản tường thuật ở bước 7 (B đóng 150.000/200.000, B1 đủ, B2 thiếu 50.000) và
thay hai tiêu chí lọc bằng cặp đúng với công thức: `status=partial` trả về
đúng B, `status=unpaid` trả về đúng C và D. Không đổi công thức, không đổi kịch
bản tường thuật — chỉ sửa lại đúng vế lọc theo trạng thái cho khớp công thức
mà chính phase file đã chốt.

## 2026-08-04 — Plan 05, review chốt: đồng nhất thứ tự khoá chống deadlock ở đường sửa thanh toán + bảo toàn draft khi recompute

Rà soát tiền tệ/tenancy/đồng thời trên `payments` + `collections` xác nhận lõi
đúng (không lỗi làm sai/mất tiền), và nêu một khiếm khuyết đồng thời thật cùng
vài điểm nhỏ.

**MEDIUM (đã sửa) — deadlock giữa các thao tác sửa trên cùng một contact.**
`Reallocate` chỉ khoá `FOR UPDATE` các hoá đơn đích *mới*, nhưng lại recompute
cả hoá đơn *bị rời khỏi split* (union old ∪ new); `Reverse` không khoá hoá đơn
nào trước recompute; và vòng recompute lặp trên Go map (thứ tự ngẫu nhiên).
Cộng với `candidateInvoicesQuery` khoá theo `period_start` còn `InvoicesByIDs`
theo `id`, hai giao dịch đồng thời có thể giành cùng hai hoá đơn theo thứ tự
ngược nhau → deadlock (Postgres rollback một giao dịch, trả 500). Không sai
ledger (rollback sạch) nhưng vi phạm bất biến "lock ordering deadlock-free"
(cùng lớp lỗi đã xử lý ở H1 plan 04).

**Quyết định:** đồng nhất mọi đường ghi theo phạm vi contact về *một* trật tự
khoá duy nhất là `invoice_id`:
- `Reallocate` khoá toàn bộ union old ∪ new qua `InvoicesByIDs` trước khi ghi.
- `Reverse` khoá các hoá đơn bị ảnh hưởng (theo allocation gốc) trước recompute.
- Recompute chạy qua helper `recalcTouched` lặp theo id tăng dần (thay cho map).
- Đổi `ORDER BY` của `candidateInvoicesQuery` từ `(period_start, earliest_class_start NULLS LAST, id)` sang `id`. An toàn cho D8 vì `Allocate` tự sort lại theo comparator D8 trước khi phân bổ; `ORDER BY` ở query chỉ chi phối thứ tự *khoá*, không chi phối kết quả phân bổ. Đây là sai lệch có chủ đích so với thứ tự khoá mô tả ở phase-01 (đổi để chống deadlock, không đổi hành vi phân bổ).
- Test hồi quy `TestConcurrentReallocationsOnSameContactDoNotDeadlock`: hai reallocation đồng thời cùng đụng hai hoá đơn, khẳng định không call nào lỗi internal/deadlock và ledger vẫn cân, chạy xanh dưới `-race`.

**LOW-1 (đã sửa) — recompute chỉ bảo toàn `void`, chưa bảo toàn `draft`.**
`recalcInvoicePaidQuery` chỉ có `WHEN i.status = 'void'`. Hiện an toàn (draft
không bao giờ tới được recompute), nhưng bất biến ghi rõ "void/draft không bị
recompute chỉnh". Sửa thành `WHEN i.status IN ('void','draft')` để chặn bẫy
nếu về sau có caller truyền id draft.

**LOW-2 (quyết định V1) — dòng đảo (reversal) không chép `reference_code`.**
Counter-entry của một reversal không mang lại mã tham chiếu chứng từ gốc. Không
sai ledger (reference chỉ là ghi chú đối soát). Giữ nguyên ở V1: reversal là
một *chứng từ mới* độc lập, và mã tham chiếu gốc vẫn tra được trên payment gốc
(không bị xoá). Xem lại nếu đối soát ngân hàng cần bắc cầu mã này.

**LOW-3 (quyết định V1) — `unallocated_credit` mang tính global theo contact.**
Summary cộng `payment.amount − Σ allocations` cho payment sống của contact có
hoá đơn trong kỳ; tín dụng chưa phân bổ vốn không thuộc kỳ nào nên cùng một
khoản credit hiện ở summary của mọi kỳ mà contact có hoá đơn. Không âm, không
sai từng con số. Giữ nguyên ở V1 như một "cửa sổ hiển thị" credit tồn (OQ-2);
gắn credit vào một kỳ cụ thể là quyết định sản phẩm, để lại cho khi có màn hình
đối soát credit.

## 2026-08-04 — Plan 06, Phase 2: VietQR field 38 dùng thẳng `BankCode` làm acquirer id, chưa tra BIN NAPAS thật

EMVCo field 38 sub-tag 01 (trong `merchantAccountInfo`) đúng chuẩn phải mang mã
BIN số của NAPAS cho ngân hàng thụ hưởng (ví dụ Vietcombank = `970436`), không
phải mã ngắn tuỳ ý của ngân hàng. V1 chưa có bảng tra BIN nào, và phase file
cũng không cấp một bảng như vậy.

**Quyết định:** `emvQRBuilder.Payload` (qr.go) dùng thẳng `cfg.BankCode` (chuỗi
giáo viên tự nhập vào config, ví dụ `"TESTBANK"` trong fixture test — cố ý giả,
không phải BIN thật) làm giá trị sub-tag 01. Payload vẫn đúng cấu trúc TLV và
checksum CRC-16/CCITT-FALSE — cái `qr_test.go`/`public_integration_test.go`
xác nhận — chỉ khác ở chỗ một ví quét QR thật sẽ không tự resolve đúng tên
ngân hàng nếu `BankCode` không trùng BIN NAPAS thật. Không chặn giao hàng V1 vì
QR chỉ là tiện ích gợi ý chuyển khoản, không phải đường thanh toán tự động đối
soát; số tiền/nội dung chuyển khoản vẫn đúng để phụ huynh tự nhập tay nếu ví
không tự nhận diện ngân hàng. Cần một bảng tra BIN NAPAS thật (hoặc để giáo
viên nhập thẳng BIN thay vì mã ngắn) trước khi tính năng này rời V1.

## 2026-08-04 — Plan 06, Phase 2: hình dạng DTO `carried_adjustment` là `{amount, session_dates}`, không phải lý do gốc của giáo viên

Phase file mô tả khối "carried forward" cần giải thích cho phụ huynh "vì sao
số tiền kỳ này không khớp số đã thấy tháng trước", nhưng không chốt hình dạng
JSON chính xác. `invoice_adjustments.reason` (text tự do giáo viên gõ khi sửa
điểm danh) là ứng viên tự nhiên nhất để "giải thích", nhưng đây chính là
trường bị cấm tuyệt đối xuất hiện trên payload công khai (không xác thực).

**Quyết định:** `PublicCarriedAdjustment{Amount int64, SessionDates
[]string}` (dto.go) — giải thích bằng NGÀY của (các) buổi học nguồn gây ra
carry-forward (qua `invoice_adjustments.source_session_id` → `class_sessions.
session_date`, đọc trong `adjustmentsQuery`'s union thứ hai, repository.go),
cộng dồn theo tổng `amount` — không phải chuỗi lý do gốc. Phụ huynh thấy "buổi
06/01 đã được sửa lại, chênh lệch -100.000", đủ để đối chiếu mà không lộ ghi
chú nội bộ của giáo viên (có thể chứa tên học sinh khác, nhận xét riêng tư,
…). `TestPublicAdjustmentReasonNeverAppearsInResponseBody` khẳng định
`reason` không bao giờ xuất hiện trong response body.

## 2026-08-04 — Plan 06, Phase 2: 404 trung lập chỉ cho token/outstanding, lỗi hạ tầng thật vẫn là 500

Phase file yêu cầu "mọi nhánh thất bại" của route công khai trả 404 trung lập
byte-giống-hệt nhau, nhưng không tách rõ "thất bại" nghĩa là gì — chỉ tính
token không hợp lệ (không tồn tại/sai định dạng/đã thu hồi/hết hạn/xoá mềm) và
"đã trả hết nợ", hay tính cả một lỗi DB/mạng thật sự giữa chừng.

**Quyết định:** `LookupPublic`/`RenderPublic` (service.go) chỉ trả
`ErrNotFound` (→ 404 trung lập qua `writeNeutralNotFound`) cho đúng sáu lý do
đã liệt kê ở docstring của `LookupPublic`/`RenderPublic`: token không tồn tại,
sai định dạng (hash không khớp), đã thu hồi, hết hạn, xoá mềm, hoặc
`Totals.Outstanding <= 0`. Một lỗi đọc DB thật (`apperror.Internal(err)`) vẫn
nổi lên như 500 thật qua `response.Err` — che nó thành 404 sẽ khiến một sự cố
hạ tầng thật (mất kết nối DB, timeout, …) bị nuốt thầm lặng và không bao giờ
lên log/alert đúng mức nghiêm trọng của nó. "Trung lập" nghĩa là không phân
biệt được LÝ DO token thất bại với nhau — không có nghĩa là mọi loại lỗi đều
phải giả vờ thành 404.

## 2026-08-04 — Plan 06, Phase 2: một method `Adjustments` bằng UNION ALL, không tách `CarriedAdjustments` như phase file gọi tên

Phase file (Architecture) nhắc tới một method riêng tên `CarriedAdjustments`
bên cạnh việc đọc adjustment trực tiếp trên hoá đơn của kỳ. Tách hai round trip
riêng (một đọc adjustment trực tiếp, một đọc carried-forward) sẽ vi phạm ngay
bất biến "không tăng số lượng query theo số con" mà chính phase file đặt ra
cho toàn bộ route này (test `TestPublicRenderIssuesTheSameQueryCountRegardlessOfFamilySize`
khẳng định đúng 3 query cố định: InvoicesWithLines + LiveSessions +
Adjustments).

**Quyết định:** `Repository.Adjustments` (repository.go) là MỘT method, thân
là một câu SQL `UNION ALL` hai nhánh — nhánh 1 đọc adjustment đăng trực tiếp
trên hoá đơn của kỳ này (`Carried=false`), nhánh 2 đọc adjustment đăng trên
hoá đơn của một kỳ SAU nhưng có `source_session_id` rơi vào đúng khoảng ngày
của kỳ này (`Carried=true`, kèm `session_date`) — đúng ngữ nghĩa
`CarriedAdjustments` mà phase file mô tả, chỉ khác ở chỗ gộp vào cùng một
round trip DB thay vì một method Go riêng. `AdjustmentRow.Carried` là cờ phân
biệt hai nhánh khi `render.go` build payload; không có method

## 2026-08-04 — Plan 06, Phase 3: `notifications` không có cột `message_text`/`contact_id`; `purpose` DB là số nhiều

Phase file (Architecture, bước 5 "Insert one `notifications` row per contact")
mô tả việc ghi `message_text`, `statement_id`, `contact_id` lên mỗi dòng
`notifications`, và request body mẫu dùng `purpose: "statement" | "reminder"`.
Đối chiếu trực tiếp với `docs/schema_design.sql:434-451` — nguồn chân lý duy
nhất cho schema, vì schema đã đóng băng, không có migration nào trong phase
này — bảng `notifications` thực tế chỉ có các cột: `id, teacher_id,
statement_id, channel, purpose, status, provider_msg_id, error_message,
sent_at, created_at, updated_at, deleted_at`. Không có `message_text`, không
có `contact_id`. Liên kết tới contact đi gián tiếp qua
`statement_id → statements.contact_id`. Đồng thời `purpose`'s CHECK constraint
(`schema_design.sql:440-441`) chỉ chấp nhận `'statements'` (số nhiều) và
`'reminder'` — không có giá trị `'statement'` (số ít) nào từng tồn tại trong
DB.

**Quyết định (hai điểm):**

1. **Message text là response-only, không bao giờ persist.** `notifications`
   model (`model.go`) chỉ map đúng các cột thật của bảng — không thêm field
   `MessageText`/`ContactID` giả vào struct GORM chỉ để rồi không bao giờ ghi
   được xuống DB (không có cột để ghi). `notifications.Service.BulkSend` build
   text bằng `statements.Build` tại thời điểm response, trả thẳng trong
   `BulkSendResponse.rows[].message_text` cho client — không lưu lại. Hệ quả
   trực tiếp: `GET .../notifications` (danh sách ledger) không thể trả lại
   nguyên văn tin nhắn đã gửi trước đó — chỉ trả `channel/purpose/status/
   sent_at` cùng tên/điện thoại contact (join qua statement_id). Đây là giới
   hạn thật của schema đã đóng băng, không phải thiếu sót của
   implementation; ghi chú lại ở đây thay vì âm thầm bỏ qua.
2. **API nhận `"statement"`, DB lưu `"statements"`.** DTO request
   (`dto.go`) validate `purpose` bằng `binding:"oneof=statement statements
   reminder"` (chấp nhận cả hai dạng số ít lẫn số nhiều đúng theo yêu cầu của
   nhiệm vụ), rồi map cả hai về hằng số Go `purposeStatements = "statements"`
   trước khi truyền xuống Service/Repository — không bao giờ có chuỗi
   `"statement"` chạm tới câu SQL hay bị ghi xuống cột `purpose`. Toàn bộ giá
   trị `channel`/`purpose`/`status` đều là hằng số Go khớp đúng chữ với CHECK
   constraint của schema (`zalo_zns|zalo_manual|sms`,
   `statements|reminder`, `queued|sent|delivered|failed`) — không có string
   literal rời ở call site nào.
`CarriedAdjustments` riêng nào trong `Repository` interface.

## 2026-08-04 — Plan 06 review chốt (statements & notifications)

Rà soát code-reviewer: 0 CRITICAL/0 HIGH. Các bất biến trọng yếu của endpoint
công khai (neutral 404 hợp nhất một helper, ba security header trên cả 200 lẫn
404 kể cả qr.png, token bị redact khỏi access log, không rò teacher/phone/bank/
reason/dữ liệu gia đình khác, mọi query scoped teacher_id∧contact_id∧period_id)
đều được trace xác nhận đúng. Xử lý các phát hiện:

- **L2 (đã sửa).** `qr.tlv` dùng độ dài 2 chữ số; note = `"HP {tên} {MM/YYYY}"`
  mà `full_name` tối đa 100 ký tự nên note có thể ≥100 → `%02d` sinh 3 chữ số,
  phá cấu trúc EMVCo và làm QR không quét được. Thêm `clampRunes(note, 25)`
  (đúng giới hạn field 08 của NAPAS, cắt theo ranh giới rune nên không vỡ ký tự
  đa byte; 25 rune ≤ 75 byte → độ dài luôn 2 chữ số). V1 chưa có đối soát
  ngân hàng tự động nên việc cắt ghi chú không ảnh hưởng nghiệp vụ.
- **L4 (đã sửa).** Bỏ chữ "phase 2" trong comment test `publicRouter` —
  không đưa số phase vào artifact mã.
- **L5 (đã bổ sung tài liệu).** Thêm `API_BANK_*` và `API_NOTIFICATIONS_*`
  (đều optional, default rỗng/zalo_manual/1000) vào `.env.example`,
  `docker-compose.yml`, `docker-compose.prod.yml`. Bank rỗng ở prod là chủ ý
  V1: thiếu cấu hình thì statement chỉ đơn giản bỏ khối QR, không placeholder.
- **M1 (chấp nhận).** `render.buildPublicStatement` và
  `service.assemblePeriodFigures` là hai vòng tổng hợp riêng trên cùng shape
  `InvoiceLineRow`; đã xác minh cho kết quả identical và integration test khẳng
  định tổng tin nhắn bulk == `total_due` của endpoint công khai cho cùng
  contact. Giữ nguyên cho V1; rủi ro là bảo trì (đồng bộ bằng quy ước), không
  phải sai số học.
- **M2 (chấp nhận).** Package `notifications` chỉ có integration test theo đúng
  phạm vi phase (phase file chỉ liệt kê `integration_test.go`). Coverage tổng
  repo 71.6% > sàn 60%.
- **L1 (chấp nhận).** `qr.png` trả 500 (không phải neutral 404) khi `RenderQR`
  lỗi encode PNG — đây là lỗi server thật, chỉ tới được sau khi token đã hợp lệ
  (vốn đã trả 200 cho JSON), không phải token oracle; giữ 500 để không che một
  lỗi hạ tầng thật.
- **L3 (chấp nhận).** Bulk `zalo_zns` khi 0 contact đủ điều kiện trả 200
  QueuedCount=0 (không ghi gì) thay vì báo "not configured"; chỉ khác thông
  điệp, không sai dữ liệu.
- **display_note trong payload công khai — không phải rò rỉ.** Schema
  (`schema_design.sql:109`) định nghĩa đây là "nhãn phân biệt anh em cùng lớp
  trùng họ tên (vd 'An lớp 9A')" — chủ đích hiển thị cho phụ huynh, không phải
  ghi chú riêng tư của giáo viên.

## 2026-08-04 — Web Design System Foundation: nguồn chuẩn & các deviation

Tích hợp design system "Học Vui Mỗi Ngày" (hướng "Dịu Mát") vào `apps/web`.
Nguyên tắc: khi lời văn phase spec và `_ds_bundle.js` (recipe gốc của design
project) khác nhau, **bundle là nguồn chuẩn** (rule "100% design system").
Các điểm đã reconcile về phía bundle, ghi lại để không bị "sửa ngược" ở audit
sau:

- **Ring vs focus shadow (collision).** `effects.css` định nghĩa `--ring` là
  full box-shadow `0 0 0 4px var(--mint-200)`, nhưng shadcn kỳ vọng `--ring`
  là một *màu*. Giải: trong `@theme inline` map `--color-ring: var(--mint-200)`
  (màu cho ring utilities của shadcn) và **không** khai báo lại `--ring` ở
  `:root`, để token DS `--ring` giữ nguyên làm box-shadow cho global
  `:focus-visible { box-shadow: var(--ring) }`. Nút HV dùng
  `focus-visible:ring-4` (compose cùng press shadow).

- **HvCard shadow.** Lời văn phase-02 nói raised=`shadow-sm`, interactive
  hover=`shadow-md`; nhưng bundle `hv-card` dùng `shadow-md` base + `shadow-lg`
  hover. Giữ theo bundle (`shadow-soft-md`/`shadow-soft-lg`).

- **ProgressBar easing + track.** Bundle dùng `transition ... var(--ease-out)`
  và track `bg cream-200`. Giữ theo bundle (không đổi sang ease-soft/line-200
  như lời văn spec gợi ý). Bổ sung `color="missing"` → fill `coral-400` để
  màn thu học phí (plan 08) biểu thị "còn thiếu" — đây là phần spec phase-02
  thêm ngoài bundle, được giữ vì consumer cần.

- **HvButton ghost press = 5px (không dùng `shadow-press-line`).** Mọi biến thể
  nút depress đúng `--press-depth` = 5px và `active:translate-y` = 5px. Token
  `--press-line` lại là `0 4px 0` (4px, dùng cho ngữ cảnh khác). Nếu ghost dùng
  `shadow-press-line` (4px) sẽ lệch 1px so với translate 5px → hở đáy. Vì vậy
  ghost hardcode `0 var(--press-depth) 0 var(--line-300)` (qua token, không
  hex) để đồng bộ 5px với các biến thể khác. Nút `sm` dùng `--radius-md` (16px)
  đúng theo recipe DS.

- **HvModal.** Bọc `DialogPrimitive` trực tiếp (không qua `ui/dialog`
  `DialogContent`) vì `DialogContent` hardcode canh giữa (`top-1/2 left-1/2
  -translate-*`) không thể override per-breakpoint để thành bottom-sheet dưới
  `sm`. Vẫn tái dùng hành vi radix (focus trap, esc, portal). A11y: luôn render
  `DialogPrimitive.Title` (sr-only "Hộp thoại" khi không truyền `title`) để
  dialog luôn có accessible name; truyền `aria-describedby={undefined}` để tắt
  cảnh báo Description. Animation: `max-sm:slideUp` (bottom sheet trượt lên) /
  `sm:popIn` (panel canh giữa).

- **Fonts self-hosted.** Thay Google Fonts CDN của DS bằng `@fontsource`
  (Baloo 2 600/700/800, Nunito 400/600/700/800), woff2 bundle kèm subset
  vietnamese — 0 request bên thứ ba.

- **Dark mode.** DS không định nghĩa dark. Giữ block `.dark` (map tạm về
  surface-dark) để `dark:` variant của shadcn không lỗi, nhưng không dùng.

## 2026-08-04 — Plan 07, Phase 1: đối chiếu "Assumed API contract" với backend Go thật

Phase file `phase-01-auth-shell-dashboard.md` giả định ba hình dạng response
khác với API thật (`apps/api`). Đối chiếu trực tiếp mã nguồn Go, không suy đoán
từ tài liệu:

- **`TokenResponse` nhúng `teacher`, không phải `user`.** Phase file viết
  `session.user`. `apps/api/internal/features/auth/dto.go` (`TokenResponse`)
  khai báo field JSON `teacher` — đúng như ADR "Plan 01, Phase 2: `AccountService`
  trả `*teachers.Profile`" phía trên đã ghi: response auth nhúng thẳng hồ sơ
  giáo viên để khỏi gọi thêm `/me`. Client (`auth-schemas.ts`) parse
  `sessionSchema.teacher`, không phải `.user`; store, hook, và mọi test đổi
  theo (`useAuthStore.getState().user` vẫn giữ tên field state cũ, chỉ nguồn
  gán đổi).

- **`GET /sessions/pending` trả `{total, items: [...]}`, không phải mảng trần
  với field `id`.** Phase file giả định response là một mảng session object có
  field `id`. `apps/api/internal/features/sessions/dto.go`
  (`PendingResponse`/`PendingSessionResponse`) trả object bọc
  `{total int, items []PendingSessionResponse}`, và mỗi phần tử dùng khoá
  `session_id` (không phải `id`), kèm `class_id`, `class_name`, `session_date`,
  `start_time`, `status`, `expected_student_count`, `days_overdue`. Client
  (`dashboard/schemas/dashboard-schemas.ts`) khai `pendingSessionsResponseSchema`
  đúng hình dạng này rồi lấy `.items`; mọi liên kết "Điểm danh ngay" trỏ
  `/sessions/{session_id}/attendance`.

- **Không có `GET /billing/periods/current`.** Route đó không tồn tại trong
  `apps/api/internal/features/billing/routes.go`. API chỉ có
  `POST /billing-periods` (`EnsurePeriodRequest{year, month}` →
  `handler.go`/`service.go` gọi `EnsurePeriod`, tạo-hoặc-lấy kỳ, idempotent).
  Client không thể hỏi "kỳ hiện tại" trực tiếp; `features/billing/api/billing-api.ts`
  (`getCurrentPeriod`) POST kỳ của tháng/năm hiện tại (giờ máy client) mỗi lần
  gọi — an toàn vì `EnsurePeriod` là ensure, không phải create-or-fail.

**Quyết định:** sửa toàn bộ ba điểm ở tầng client (schema, API call, mọi test
liên quan) để khớp API thật; không đổi API. Phase file mô tả sai vì được viết
trước khi plan 04 (billing engine) chốt endpoint thật; các plan API 01–06 (đã
merge trước plan 07) là nguồn chân lý.

## 2026-08-04 — Plan 07, Phase 2: đối chiếu bảng "Assumed API contract" của roster với backend Go thật

Phase file `phase-02-roster.md` tự đánh dấu bảng API là ASSUMED, cần đối chiếu
mã nguồn thật (`apps/api/internal/features/{contacts,students,classes,
enrollments}`) trước khi viết client. Đối chiếu trực tiếp `routes.go`/`dto.go`
của cả bốn package, các điểm khác với bảng giả định:

- **`PUT`, không phải `PATCH`.** `contacts`/`students`/`classes` update đều
  đăng ký `g.PUT("/:id", ...)`. Client (`*-api.ts`) gọi `apiClient.put`.
- **`GET /contacts/:id` không lồng `students[]`.** `ContactResponse`
  (`contacts/dto.go`) chỉ có `student_count int64`, không có mảng con.
  `ContactDetailPage` gọi riêng `useStudentsList({ contact_id: id })` thay vì kỳ
  vọng danh sách con nằm sẵn trong response contact.
- **`POST /classes` bắt buộc `schedules` không rỗng ngay trong request tạo
  lớp**, không phải một luồng "tạo lớp rồi thêm lịch riêng" như bảng giả định
  ngụ ý. `CreateClassRequest.Schedules` là `binding:"required,min=1,dive"` — lớp
  không lịch sẽ không sinh buổi học nào nên bị chặn tạo. `ClassDialog` gom
  name/weekday-chip/time/duration/dates/price cho MỘT schedule khởi đầu vào
  một request `POST /classes` duy nhất (đúng field `implementation-steps` bước
  11 đã mô tả, không phải bảng contract).
- **Đổi trạng thái lớp đi qua `POST /classes/:id/archive` riêng, không qua
  `PATCH`/`PUT` với field `status`.** `UpdateClassRequest` không có field
  `status`. `ClassDialog` (edit mode) hiện trạng thái bằng `HvBadge` +
  nút "Lưu trữ lớp" gọi endpoint archive riêng, tách khỏi form sửa
  name/dates/price.
- **Sửa một dòng lịch dùng `PUT /classes/:id/schedules/:scheduleID`**, và
  đóng lịch (set `effective_to`) là MỘT dạng của chính update đó — không có
  endpoint "close schedule" riêng (xem ADR "Plan 02, Phase 3: gộp
  CloseSchedule vào UpdateSchedule" phía trên). `ScheduleEditor` gọi
  `updateSchedule` cho cả sửa giờ/thứ lẫn đóng lịch.
- **Kết thúc ghi danh là `POST /enrollments/:id/end`**, không phải
  `PATCH /enrollments/:id`. Double-end trả 409 (không phải cho phép ghi đè
  ngày kết thúc lần hai) — `EndEnrollmentDialog` không có đường retry ngầm,
  lỗi 409 nổi lên qua `form.setError("root", ...)` như mọi lỗi mutation khác.
  API còn có `DELETE /enrollments/:id` (dành cho ghi danh tạo nhầm, chưa có
  buổi học nào) — roster UI ở phase này không có nút "Xoá ghi danh" vì
  Design Spec chỉ yêu cầu "Kết thúc ghi danh"; endpoint tồn tại nhưng chưa có
  consumer ở tầng web, ghi nhận ở đây để không bị tưởng là bỏ sót.

**Quyết định:** toàn bộ bốn module `api/*.ts` viết đúng theo API thật ở trên
(không viết theo bảng giả định); không đổi API. `docstring` trên từng hàm
`*-api.ts` trỏ thẳng file Go tương ứng để lần sau đối chiếu không phải lặp lại
việc đọc mã nguồn.

## 2026-08-04 — Plan 07, Phase 2: đường dẫn tham chiếu `features/users/` trong phase file không còn tồn tại

Implementation Steps (bước 2, 15) chỉ định khuôn mẫu file theo
`apps/web/src/features/users/{api/users-api.ts, hooks/use-users.ts,
components/create-user-dialog.tsx, pages/users-page.tsx, routes.tsx}`. Thư mục
này không tồn tại trong `apps/web/src/features/` hiện tại (chỉ có `auth`,
`billing`, `dashboard`, `roster`) — feature "users" thuộc bản scaffold email/
admin cũ, đã được thay bởi phone-based auth (xem ADR "Plan 01, Phase 2: xoá
`features/users/`..." phía trên, phía backend) và không có tương ứng phía web
kể từ Web Design System Foundation.

**Quyết định:** dùng `apps/web/src/features/dashboard/routes.tsx` làm khuôn
mẫu `route.lazy` (đã ghi chú thẳng trong `roster/routes.tsx`), và
`apps/web/src/features/auth` (form + dialog patterns, `useApiFormErrors`, cấu
trúc `api/`+`hooks/`+`schemas/`) làm khuôn mẫu cho các file còn lại — cùng
shape file, cùng convention, là feature gần nhất còn tồn tại trong repo.
Không đổi kết quả cuối, chỉ đổi nguồn tham chiếu bố cục.

## 2026-08-04 — Plan 07, Phase 2: `ContactPicker` tạo liên hệ mới inline, không mở `ContactDialog` lồng

Design Spec không chốt cách "tạo người liên hệ mới ngay trong form học sinh"
nên thực hiện: mở một `HvModal` lồng bên trong `HvModal` của `StudentDialog`
(modal-trong-modal), hay một form rút gọn ngay tại chỗ.

**Quyết định:** dòng cuối danh sách kết quả tìm kiếm của `ContactPicker`
("— Tạo người liên hệ mới —") mở rộng hai input (họ tên, số điện thoại) ngay
tại chỗ, không phải một `HvModal` thứ hai. Radix `Dialog` không hỗ trợ lồng
tốt (portal/focus-trap chồng nhau), và giáo viên đang ở giữa việc điền form
học sinh — một modal thứ hai đè lên sẽ ngắt mạch nhập liệu. Form rút gọn dùng
lại `contactInputSchema`/`useCreateContact` y hệt `ContactDialog`, chỉ khác nơi
render.

## 2026-08-04 — Plan 07, Phase 2: `EnrollStudentDialog` gộp hai chiều gọi (bước 10 và bước 13) thành một component `mode: "student" | "class"`

Bước 10 (Implementation Steps) mô tả `StudentDetailPage` có hành động "Ghi
danh vào lớp" — học sinh đã cố định, cần tìm LỚP. Bước 13 mô tả
`EnrollStudentDialog` với "student picker (search by name)" — ngụ ý lớp đã cố
định (gọi từ `ClassDetailPage`), cần tìm HỌC SINH. Hai bước mô tả cùng một tên
component nhưng hai chiều tìm kiếm ngược nhau; phase file không nói rõ đây là
một component hay hai.

**Quyết định:** một `EnrollStudentDialog` union-type theo `mode`: `mode:
"student"` (từ `StudentDetailPage`, `studentId` cố định, tìm/lọc lớp qua
`useClassesList({status:"active"})`) và `mode: "class"` (từ `ClassDetailPage`,
`classId` cố định, tìm học sinh qua `useStudentsList({query})` debounce
300ms). Cả hai đường đều chia sẻ cùng field `started_on`, cùng dòng đơn giá
kế thừa read-only, cùng toast sau khi thành công — chỉ khác ô tìm kiếm nào
hiện ra. Tránh nhân đôi gần như toàn bộ logic mutation/toast/validate giữa hai
component riêng.

## 2026-08-04 — Plan 07, Phase 2: `parseMoney`/`formatWeekday` đặt cục bộ trong feature, không thêm vào `lib/utils` dùng chung

`money-input.tsx` cần parse ngược chuỗi đã format nhóm nghìn về số nguyên;
`schedule-editor.tsx`/`class-dialog.tsx` cần nhãn tiếng Việt cho `weekday`
(0=Chủ nhật…6=Thứ 7). `apps/web/src/lib/utils/format.ts` (dùng chung) đã có
`formatMoney` (chiều ngược lại) nhưng không có hai hàm này, và không feature
nào khác ngoài roster cần chúng.

**Quyết định:** thêm `apps/web/src/features/roster/lib/roster-format.ts`
(feature-local, không đụng `lib/utils` — file dùng chung nằm ngoài phạm vi sở
hữu file của phase này) chứa `parseMoney`/`formatWeekday`. Nếu một feature
khác sau này cần dùng lại, nâng lên `lib/utils` lúc đó; giữ cục bộ bây giờ là
YAGNI, không phải giới hạn kỹ thuật.

## 2026-08-04 — Plan 07, Phase 3: xác nhận điểm danh là `POST /sessions/:id/attendance`, không phải `PUT`

Phase file giả định bảng API contract theo hướng REST thuần: xác nhận/ghi đè
điểm danh dùng `PUT`. Đọc thẳng `apps/api/internal/features/attendance/
handler.go` và `router.go` thì endpoint thật là `POST /sessions/:id/attendance`
(hàm service `attendance.confirm`) — dùng cho cả lần xác nhận đầu tiên lẫn
lần sửa lại sau khi mở lại một buổi đã chốt, dù ngữ nghĩa là "ghi đè toàn bộ".

**Quyết định:** `confirmAttendance` (`apps/web/src/features/attendance/api/
attendance-api.ts`) gọi `POST`, gửi nguyên danh sách `absent_student_ids` mỗi
lần (không chỉ phần thay đổi) — khớp với cách server tính lại toàn bộ dòng
present cho những học sinh còn lại rồi chuyển session sang `held` trong một
lượt gọi. Backend là nguồn chân lý, không phải bảng REST giả định trong phase
file.

## 2026-08-04 — Plan 07, Phase 3: không có `GET /sessions` liệt kê toàn cục — luôn liệt kê theo lớp

Phase file giả định một endpoint danh sách buổi học phẳng kiểu `GET
/sessions?from=&to=`. Backend thật (`sessions.listRange`,
`apps/api/internal/features/sessions/handler.go`) chỉ có `GET
/classes/:id/sessions?from=&to=` — luôn đi kèm `class_id`, và tự sinh thêm
các dòng còn thiếu trong khoảng `[from, to]` từ lịch học của lớp trước khi trả
về (giới hạn 400 ngày phía server).

**Quyết định:** `SessionsPage` (`apps/web/src/features/attendance/pages/
sessions-page.tsx`) giữ nguyên thiết kế "tab chọn lớp" của Design Spec — vốn
đã ngầm định một lớp được chọn tại một thời điểm — nên việc API bắt buộc
`classId` không đổi hành vi UI, chỉ đổi chữ ký hàm `listClassSessions(classId,
params)` trong `api/attendance-api.ts` so với giả định `listSessions(params)`
phẳng ban đầu.

## 2026-08-04 — Plan 07, Phase 3: trường lý do huỷ buổi học là `reason`, không phải `cancel_reason`

Phase file mô tả request huỷ buổi gửi field `cancel_reason` (trùng tên với
field đọc lại trên `SessionResponse`). Handler thật
(`apps/api/internal/features/sessions/handler.go`, `cancelSessionRequest`)
nhận field `reason` trong body `POST /sessions/:id/cancel`; response trả về
mới có tên `cancel_reason`. Hai field đọc/ghi khác tên nhau.

**Quyết định:** `cancelSessionInputSchema` (`schemas/attendance-schemas.ts`)
định nghĩa form field là `reason` để khớp request thật, còn `sessionSchema`
đọc `cancel_reason` từ response — `CancelSessionDialog` submit đúng field
`reason`, `SessionListItem`/`AttendancePage` hiển thị đúng field
`cancel_reason`. Không đổi tên nào cho khớp phase file vì phase file sai, không
phải backend.

## 2026-08-04 — Plan 07, Phase 3: không có field `period_status` trên session/attendance — cảnh báo kỳ đã chốt phải tự suy ra qua `POST /billing-periods`

Phase file giả định `SessionResponse` hoặc attendance sheet mang sẵn một field
`period_status` để hiển thị cảnh báo "kỳ đã chốt" trước khi giáo viên bấm xác
nhận. Đọc `apps/api/internal/features/attendance/handler.go` và
`apps/api/internal/features/sessions/handler.go` thì không endpoint nào trả
field này — tín hiệu "kỳ đã chốt" chỉ xuất hiện *sau khi* xác nhận, dưới dạng
chuỗi `warning` tự do trên response của `POST /sessions/:id/attendance`, tức
là quá muộn để cảnh báo trước khi ghi.

**Quyết định:** thêm `getPeriodForDate`/`usePeriodForDate`
(`api/attendance-api.ts`, `hooks/use-sessions.ts`) gọi lại `POST
/billing-periods` (`billing.ensurePeriod`, endpoint idempotent create-or-get
mà feature `billing` đã dùng cho `getCurrentPeriod`) nhưng truyền năm/tháng
của `session.session_date` thay vì tháng hiện tại. `AttendancePage` dùng
`period?.status === "closed"` để hiện `ClosedPeriodWarning` và đổi nhãn nút
xác nhận thành "LƯU VÀ TẠO ĐIỀU CHỈNH" *trước* khi giáo viên bấm, đồng thời vẫn
hiển thị `warning` trả về sau khi lưu như một lớp xác nhận thứ hai. Đây là giải
pháp tạm dùng API sẵn có, không phải endpoint được thiết kế cho mục đích này —
nếu backend sau này thêm `period_status` trực tiếp, có thể bỏ lượt gọi phụ
này.

## 2026-08-04 — Plan 07, Phase 3: `SessionResponse` không có số liệu có mặt/vắng — danh sách buổi học không hiện được "N có mặt · M vắng" cho buổi đã chốt

Design Spec mô tả mỗi dòng buổi học đã điểm danh trong danh sách hiện dạng "N
có mặt · M vắng". `SessionResponse` (`apps/api/internal/features/sessions/
handler.go`) chỉ có `student_count` — tổng sĩ số roster tại thời điểm đó,
không phải số đã điểm danh — vì phần chia present/absent chỉ tồn tại trên
từng dòng của attendance sheet (`GET /sessions/:id/attendance`), không phải
trên chính session.

**Quyết định:** `SessionListItem` (`components/session-list-item.tsx`) hiện
"Đã điểm danh" (không kèm số) cho buổi đã có `attendance_confirmed_at`; con số
"N có mặt · M vắng" chính xác chỉ hiện khi mở panel điểm danh của buổi đó
(`AttendancePage` đã tải `rows` đầy đủ). Không gọi thêm request roster cho mỗi
dòng trong danh sách chỉ để lấy số liệu hiển thị — vi phạm trực tiếp ngân sách
tương tác/độ trễ của PRD R2 khi danh sách có nhiều buổi.

## 2026-08-04 — Plan 07, Phase 4: hợp đồng API "giả định" của phase chốt sổ / thu tiền / thông báo lệch nhiều so với backend Go thật

Phần "Assumed API contract" của phase-04 là phỏng đoán; khi dựng zod schema đã
đối chiếu trực tiếp với handler/route/dto Go thật của `billing`, `collections`,
`payments`, `notifications`. Các điểm lệch và cách xử lý (đều implement theo
thực tế của backend, không theo spec):

**Màn chốt sổ (`features/billing`).**

1. **Nguồn dữ liệu review:** spec giả định `GET /billing/periods/:id/review` —
   không tồn tại. Dùng `POST /billing-periods/:id/draft` (`billing.Service.
   Draft`, idempotent, là endpoint duy nhất trả `invoice_id` thật).
2. **Hình dạng response:** `billing.PreviewResponse` thật chỉ là `{ invoices[],
   totals }` — không kèm `period` hay `blocking_sessions`. Tách thành các query
   tổ hợp: `usePeriod` (`GET /billing-periods/:id`) và `useBlockingSessions`
   (`GET /sessions/pending?from=&to=`).
3. **Tên trường dòng/line:** `PreviewInvoice`/`PreviewLine` thật có thêm
   `enrollment_id`, `class_id`, `present_count`, `current_charge` (chứ không chỉ
   `total_due`). Schema dựng khớp DTO thật.
4. **Đường dẫn:** tài nguyên là `/billing-periods/...` (có gạch nối), không
   lồng dưới `/billing`. Đóng kỳ: `POST /billing-periods/:id/close`. Bộ chuyển
   kỳ: `GET /billing-periods?per_page=2&sort=-period_start`. Điều chỉnh: `POST
   /invoices/:invoiceId/adjustments` (điểm duy nhất khớp spec).
5. **Chặn chốt sổ:** không có trường `blocking_sessions` nhúng trong response.
   Dựng query chủ động `GET /sessions/pending?from=period_start&to=period_end`
   phản chiếu đúng predicate `blockingSessions()` của `close.go`, còn lỗi 409
   (`unconfirmed_sessions`) khi gọi close là chốt chặn phía server.

**Màn thu tiền / thông báo (`features/collections`).**

6. **Không có endpoint xem trước phân bổ:** spec giả định `GET /contacts/:id/
   allocation-preview`. Thật: `POST /payments` luôn ghi VÀ tự phân bổ (D8, nợ cũ
   trước) trong một bước; không có lời gọi chỉ-xem-trước. UX "xem trước" trở
   thành ghi-rồi-sửa: dialog ghi nhận thu thật ngay, hiện split server trả về,
   cho sửa qua `PUT /payments/:id/allocations` (nơi duy nhất `allocated_by` đổi
   sang `manual`). Không tái tính phân bổ ở client.
7. **Nhắc nợ / thông báo hàng loạt:** thật là `POST /billing-periods/:id/
   notifications/bulk { purpose: "statements"|"reminder" }` — không idempotent,
   không lọc `contact_ids`, luôn nhắm mọi contact đủ điều kiện của kỳ. Nút "Nhắc
   nợ" trên dòng contact vì thế điều hướng tới `/notifications/:periodId` thay
   vì gửi lẻ từng contact (endpoint gửi lẻ không tồn tại). Vẫn đảm bảo một tin
   nhắn/một gia đình vì bulk gom theo contact.
8. **Sổ thông báo (ledger):** `GET .../notifications` chỉ trả bookkeeping
   (`status`/`sent_at`); `message_text`/`url` chỉ có trong response `bulk` mới
   tạo. `NotificationsPage` giữ `rows` cục bộ làm nguồn text, dùng ledger để
   quyết định tự sinh một lần (ledger rỗng) hay yêu cầu "Tạo lại" (ledger đã có,
   vì tạo lại không idempotent). `message_text` do server render, không dựng ở
   client.
9. **Thu tiền theo lớp:** `GET /billing-periods/:id/collections?view=class` bắt
   buộc `class_id` (422 nếu thiếu). Thêm tablist chọn lớp lấy từ
   `@/features/roster` `useClassesList`.
10. **Đánh dấu đã gửi:** toàn cục `POST /notifications/mark-sent { ids[] }`
    (không lồng theo kỳ). Không có route index `/collections` trần — luôn điều
    hướng kèm `periodId` cụ thể (từ liên kết "Gửi thông báo →" của màn chốt sổ).

## 2026-08-04 — Plan 08, Phase 1+2: payload `PublicStatement` thật lệch nhiều so với shape giả định trong plan

Plan 08 giả định một shape payload (`{ contact_name, period: {year,month,...},
children:[{classes:[{attended_dates, absent_dates, cancelled_dates}], ...}],
grand_total, payment:{bank_name, account_number, account_holder, transfer_note,
qr_image_url} }`). Backend Go thật (`apps/api/internal/features/statements/
dto.go` — `PublicStatement`) khác ở nhiều điểm; feature dựng zod schema theo DTO
thật, không theo shape giả định:

1. **Một mã lỗi duy nhất — 404 cho mọi trường hợp.** `GET /public/statements/
   :token` trả 404 trung tính cho token không tồn tại, sai định dạng, đã thu
   hồi, hết hạn, đã xoá mềm, HOẶC đã thanh toán đủ (`public_handler.go`,
   `writeNeutralNotFound`). Không có 401/403/410. Trang vẫn gộp mọi non-200 về
   một `StatementError` như plan yêu cầu.
2. **`period` là chuỗi `"MM/YYYY"`**, không phải object `{year, month,
   period_start, period_end}` (`render.go`: `fmt.Sprintf("%02d/%d", month,
   year)`). Nhãn tháng lấy trực tiếp từ chuỗi này.
3. **Buổi học là danh sách hợp nhất `sessions: [{date, status, counted}]`** trên
   mỗi lớp, không phải ba mảng `attended_dates`/`absent_dates`/`cancelled_dates`
   riêng. Client tự nhóm: `status="present"` → Có mặt, `"absent"` → Vắng,
   `"cancelled"` → Buổi huỷ. `counted` (=billable) phân biệt buổi tính tiền.
   Trả lời OQ3: buổi huỷ nằm trong danh sách này qua `status`, không cần field
   riêng.
4. **Tổng gia đình = `totals.total_due`**, không phải `grand_total`. `totals`
   còn có `opening_balance, current_charge, adjustment_total, paid,
   outstanding`. Suy ra trạng thái "✓ Đã thanh toán" khi `outstanding === 0`.
   Có thêm `payments.by_invoice: [{student_name, total_due, paid, outstanding}]`
   cho trạng thái trả tiền từng con.
5. **QR chỉ mang `{image_url, amount, note}`** (`PublicQR`), KHÔNG có
   `bank_name`/`account_number`/`account_holder`. Chi tiết ngân hàng chỉ được
   nhúng trong ảnh QR (`/public/statements/:token/qr.png`, chuỗi VietQR dựng
   server-side). Vì vậy phần "chi tiết ngân hàng copy được" trong phase 2 rút
   lại còn: ảnh QR + số tiền + `note` (copy được). `qr` là `null` khi thầy/cô
   chưa cấu hình ngân hàng → chỉ hiện phần văn bản, không ảnh vỡ. `image_url`
   trỏ tới chính endpoint qr.png của server.
6. **Điều chỉnh không lộ lý do (OQ5 chốt).** `adjustments: [{amount, kind}]` với
   `kind ∈ {"manual","correction"}` suy từ có/không `source_session_id`; free-
   text `reason` của thầy/cô không bao giờ vào payload công khai. Ngoài ra
   `carried_adjustment: {amount, session_dates[]} | null` giải thích chênh lệch
   do sửa điểm danh sau chốt sổ bằng danh sách ngày buổi, không phải lý do.
7. **`display_note` (nullable) trên mỗi con** — ghi chú hiển thị do server dựng
   (ví dụ tên đã ẩn danh); hiện dưới tên con khi có.
8. **Robots/cache/referrer đã được server đặt bằng header** (`X-Robots-Tag:
   noindex, nofollow, noarchive`, `Cache-Control: no-store`, `Referrer-Policy:
   no-referrer`). Meta `noindex` phía client vẫn giữ làm phòng thủ nhiều lớp cho
   SPA như plan yêu cầu (một `index.html` chung).

**Đối soát khi trả một phần (làm rõ trên màn thanh toán).** `qr.amount` bằng
`totals.outstanding` (số còn phải trả), trong khi tiêu đề tổng vẫn là
`totals.total_due` (tổng cả kỳ). Với gia đình đã trả một phần
(`outstanding !== 0 && paid > 0`), `GrandTotal` hiện thêm hai dòng "Đã thanh
toán" (`paid`) và "Còn lại" (`outstanding`) để mã QR bên dưới (yêu cầu số nhỏ
hơn) khớp với con số trên màn. Mọi giá trị lấy nguyên từ server, không cộng trừ
phía client. Khi `outstanding === 0` giữ nguyên nhãn "✓ Đã thanh toán".

**Mục tiêu ngân sách bundle không đo được trong môi trường này.** Non-Functional
Target của plan yêu cầu đo route chunk `< 30 KB` gzip bằng
`npm run build:analyze`. Lệnh build bị hook môi trường chặn nên không chạy được
treemap ở đây. Thiết kế vẫn tuân mục tiêu về mặt cấu trúc: route lazy-load, layout
công khai không import code dashboard, client `axios` riêng không interceptor.
Cần đo lại `stats.html` trong CI/máy dev trước khi phát hành production.
