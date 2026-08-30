# Code review — Phase 3: Phone privacy + data ownership

Reviewer: code-reviewer subagent, 2026-08-30 14:46. Read-only review of the
uncommitted working tree on `teka/260830-0506`, scoped to phase-3 surfaces.
Persisted verbatim by the controller session.

## Phạm vi

- API: `migrations/000016_owner_data_anchor.{up,down}.sql`, `migrations_test.go`, `internal/shared/{authctx,classscope}`, `features/{students,statements,notifications,collections,contacts,zalo,imports,enrollments,payments,audit}`, `seeds/seed.go`, `internal/testutil/fixtures.go`, `docs/api-guidelines.md`.
- Web: `features/roster/*` (schemas, pages, dialog, hooks, `__tests__`), `features/collections/{schemas,__tests__}`.
- Kiểm chứng bằng lệnh: `go build`/`go vet` (sạch), `gofmt -l`, `golangci-lint run`, `npx tsc --noEmit` (sạch), `npx eslint .`, `npx prettier --check src`.

## Đánh giá chung

Kiến trúc mask đúng chuẩn repo: repo chỉ sinh cột dẫn xuất `phone_visible`, không có `IsOwner`/`.Has(` trong repository, service mới quyết định null hoá, fragment nhận bind args từ service. Acceptance 3 và 4 có test thật (không phải phantom). Nhưng có **1 lỗi chặn deploy trong migration**, **1 lỗ mask trên scope giả**, **1 lỗi audit ghi trùng**, và **gate format/lint đang đỏ** — chưa nên land.

## Critical

**C1. Migration 000016 có thể abort giữa chừng khi 3+ contact trùng phone** — `apps/api/migrations/000016_owner_data_anchor.up.sql:75-88`

Guard xoá mềm statement chỉ soi survivor:

```sql
AND EXISTS (SELECT 1 FROM statements s2
            WHERE s2.contact_id = b.merged_into AND s2.period_id = st.period_id
              AND s2.deleted_at IS NULL);
```

Với A (survivor, không có statement kỳ P) + B + C cùng phone, mỗi loser có statement kỳ P: không loser nào bị xoá mềm, rồi `UPDATE statements SET contact_id = b.merged_into` đẩy cả hai về A → vi phạm `uq_statements ON statements(contact_id, period_id) WHERE deleted_at IS NULL` (`migrations/000001_baseline_schema.up.sql:428`) → migration fail, golang-migrate đánh dấu dirty ngay bước 3 của runbook. Hình dạng dữ liệu này hiếm (cần hai contact trùng phone cùng được invoice trong một kỳ, khả dĩ khi owner link student sang contact của member) nhưng có thật về mặt schema.

- Fix: dedupe theo cặp `(merged_into, period_id)` — giữ statement sớm nhất, xoá mềm phần còn lại, thay vì chỉ so với survivor.
- `migrations_test.go::TestOwnerDataAnchorBackfill` chỉ dựng case 2 contact → không bắt được. Cần thêm case 3 contact + 2 statement cùng kỳ.
- `plans/reports/dry-run-260830-phone-zalo-collision-queries.md` vẫn ghi "_chưa chạy_" cho cả hai query → **không có bằng chứng prod an toàn**. Dry-run cũng thiếu query đếm đúng hình dạng va chạm statement ở trên.

## High

**H2. Mask chạy trên scope giả ở `statements.Generate`** — `internal/features/statements/repository.go:283-299`, `service.go:106-108`, `service.go:217`

`TargetContacts` tính `phone_visible` bằng `periodScope` (giáo viên của kỳ), còn `ToResponse` lại lấy arm bypass theo `sc` của caller. Comment tại repository.go:284-287 khẳng định "Generate's only callers are the owner and the period's own teacher" — **sai**: `GetPeriodStatus` mở theo `sc.CenterWide()` = `IsOwner || Has("data.view_center_wide")`. Một member được cấp `data.view_center_wide` (không owner, không `can_send_reports`) gọi `POST /billing-periods/:id/statements` trên kỳ của người khác sẽ nhận `phone` được đánh giá bằng khả năng nhìn của người khác. Khai thác hẹp nhưng đúng là điều phase-03 cấm ("CẤM chạy mask trên scope giả").

Fix: truyền scope caller riêng cho fragment (tenant filter vẫn theo `periodScope`), hoặc chỉ trả `phone` trong response của `Generate` khi `sc.ReportsOversight()`.

**H3. `enrollment.create` bị ghi audit 2 dòng** — `internal/features/audit/action.go:79` + `internal/features/audit/subscriber.go:257`

Route `POST /api/v1/enrollments` vẫn nằm trong bảng `actions` của middleware, đồng thời subscriber mới sinh thêm một `Log` từ `enrollments.StudentEnrolled`; không có dedupe. Convention xử lý case này đã có sẵn: `internal/middleware/request_events.go:43-51` (`authSessionRoutes` — "publishing here would double-log them"). Không test nào assert số dòng audit cho enrollment create.

Fix: bỏ entry `POST /api/v1/enrollments` khỏi `actions` (hoặc thêm route vào danh sách middleware bỏ qua) + test đếm đúng 1 dòng.

**H4. Check (e) FAIL — format/lint đang đỏ trên branch**

- `gofmt -l`: `internal/features/notifications/dto.go`, `internal/features/notifications/repository.go`, `internal/features/sessions/pending_test.go`.
- `golangci-lint run ./...`: 5 issues — gci ×3 (3 file trên), revive `redefines-builtin-id` ×2 tại `internal/shared/authctx/class_staff.go:89,99` (tham số tên `cap`).
- `prettier --check src`: 8 file, tất cả do branch này tạo/sửa (`roster/__tests__/{class-settings-page,class-staff-section,contacts-page}.test.tsx`, `roster-handlers.ts`, `components/{class-staff-section,enroll-existing-student-dialog}.tsx`, `hooks/use-class-staff.ts`, `pages/students-page.tsx`).
- Sạch: `go build`, `go vet`, `tsc --noEmit`; `eslint` 0 error (5 warning `react-hooks/incompatible-library` là pattern có sẵn).

## Medium

**M5. Member mất hẳn khả năng ghi nhận thanh toán, và fail bằng 404** — `internal/features/payments/repository.go:381-399`, `service.go:53-58`

`ResolveContactScope` thêm `teacher_id = sc.TeacherID` khi `!sc.CenterWide()`. Sau 000016 mọi contact neo owner ⇒ member gọi `POST /api/v1/payments` luôn nhận `404 contact`. Phase-03 chỉ ghi hệ quả "view thu nợ của member rỗng dần" và phase-04 chốt "payments là owner domain" — hướng đi có chủ ý, nhưng: (a) failure mode 404 gây hiểu nhầm thay vì 403 honest; (b) release note không nêu; (c) không có test đóng đinh hành vi mới.

**M6.** `testutil.Contact(t, db, member.ID)` không đổi, nên `payments/integration_test.go:678 TestOwnerHasFullOversightOfMembersPayments` vẫn assert invariant member-anchor — sau 000016 không thể xuất hiện trong prod; test xanh nhưng khoá cứng invariant đã bị thay thế.

**M7. Bẫy suy giảm im lặng cho đường gửi của học vụ (P4)**

- `notifications/service.go:352-359`: nil-guard biến "không có quyền" thành URL rỗng trong tin gửi đi thay vì lỗi. Hiện an toàn vì `BulkSend`/`ResumeRun` gate ở `ReportsOversight()`.
- `notifications/repository.go:441-447` `ZaloMappings` lọc `teacher_id` khi `!sc.ReportsOversight()` ⇒ map rỗng, không lỗi.
Cả hai nên fail-loud khi P4 mở gate cho `hoc_vu`.

**M8. Picker trả cả học sinh đã ghi danh trong lớp** — `internal/features/enrollments/repository.go:300-314`

`SearchEnrollableStudents` không nhận `classID`, chỉ lọc center + tên. Học sinh đang enrolled trong chính lớp đó vẫn hiện, chọn vào thì 409, chiếm chỗ cap 20. Fix rẻ: `NOT EXISTS (enrollment active của lớp)`.

## Low

- **L9.** Gate picker (`CenterWide` → mọi lớp) rộng hơn gate `Create` (ownScope) ⇒ owner tìm được học sinh trong picker lớp member rồi 422. Ràng buộc Create cố ý, hai gate lệch nhau.
- **L10.** `zalo.MatchFriendsScoped` vẫn gọi `sessionFor` khi không phone nào reachable ⇒ 404/409 thay vì danh sách toàn `matched=false`.
- **L11.** `contacts.withStudentCount` luôn center-wide ⇒ học vụ đọc số học sinh contact rộng hơn phạm vi phone.
- **L12.** `imports.resolve`: ô "phone giáo viên" trống giờ neo **lớp** về owner thay vì caller — thay đổi ngoài phạm vi "contacts + students", chưa ghi docs/release note.
- **L13.** `000016 down` best-effort — đã ghi rõ trong header file, chấp nhận được.

## Kết quả các mục kiểm tra

- **(a) Acceptance 3+4**: ĐẠT ở mức code + test (lỗ duy nhất còn hở là H2).
- **(b) Regression**: dashboard forge scope sạch (không DTO nào mang phone); đường gửi server-side vẫn đọc phone thô. Ngoại lệ: M5, H2.
- **(c) Public contract**: các quyết định nullability đều khớp thiết kế; `apps/api/docs/*` đã regenerate.
- **(d) Pattern repo**: ĐẠT.
- **(e) Lint/type/build**: KHÔNG ĐẠT — H4.

## Metrics

- Go: build/vet sạch; golangci-lint 5 issue; gofmt 3 file. Coverage `make test-api` 75.7% (floor 60%).
- Web: tsc sạch; eslint 0 error / 5 warning có sẵn; prettier 8 file lệch; vitest 68 file/448 pass; Playwright 28/28.

## Câu hỏi chưa giải quyết

1. Member mất `POST /payments` (404) — cố ý của P3 hay chờ capability map P4? Nếu cố ý: 403 + release note?
2. Dry-run collision trên prod chưa chạy — có deploy trước khi có số không?
3. Picker có nên loại học sinh đã ghi danh trong lớp?

Status: DONE_WITH_CONCERNS
