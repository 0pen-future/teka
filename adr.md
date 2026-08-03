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
