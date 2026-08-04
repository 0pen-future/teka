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
