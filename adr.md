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
