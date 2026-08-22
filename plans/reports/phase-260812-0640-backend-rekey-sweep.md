# Phase 3 Report — Backend Re-key Sweep (Center Tenancy)

Plan: `plans/260811-1055-manager-class-oversight/` · Phase 3 · Status: **completed**

## Kết quả

Toàn bộ 13 feature package chuyển từ scope `teacher_id` sang `authctx.Scope`
(center + role). `authctx.TeacherID` đã bị xoá — `ScopeFrom` là accessor duy
nhất. Seeds provision center thật + resolve scope từ DB; swagger regenerate
không đổi contract. Full module suite xanh (17 test packages + migrations +
seeds), gate `gofmt`/`go build`/`go vet -tags integration` sạch.

## Commit range

Base `ae092c7` → 15 commits:

| Commit | Nội dung |
|---|---|
| c86bd91 | contacts + teachers profile |
| f0dc993 | models + test fixtures (center-aware) |
| cf710c9 | students |
| a4c8277 | classes |
| fa79c32 | enrollments |
| 22ecab2 | sessions |
| 272f07b | attendance |
| 73a46bf | billing |
| f7fad4b | payments |
| 0dd71be | collections |
| 486a9d1 | statements |
| 9ae8402 | notifications (three-tier scoped helpers) |
| 6e8f01e | zalo (identity plumbing; vẫn teacher-keyed by design) |
| 42e356f | xoá `authctx.TeacherID` + seeds re-key |
| 998bae5 | review-gate fixes (H1/M1/L2, xem dưới) |

## Review gate — findings và cách xử lý

Code-reviewer (range `ae092c7..HEAD`) verdict **approve** với 2 finding cần
quyết định sản phẩm; user chốt tại gate 260812: *"Chỉ enroll được vào lớp của
mình. Các lớp của teacher khác chỉ được xem."*

- **H1 (High, fixed)**: owner tạo enrollment/student tham chiếu class/contact/
  student của member → row gán owner nhưng nằm trong roster member, member
  không thấy khi điểm danh/billing, và `uq_enrollments_active` chặn member tạo
  lại. Fix: reference check khi create chạy với `IsOwner` strip
  (`ownScope`) — owner tham chiếu row member nhận 422 y như row lạ; mọi khả
  năng owner-với-row-của-mình giữ nguyên (test phủ cả hai chiều).
- **M1 (Medium, fixed)**: owner bulk-send `zalo_personal` cho kỳ của member →
  mapping thuộc strict scope member nên 100% rơi thầm lặng về manual. Fix:
  từ chối sớm 409 trong transaction (rollback nên không ghi gì, kể cả
  statement refresh); kỳ của chính owner gửi bình thường. `ResumeRun` vốn đã
  tự chặn bằng ownScope — không cần sửa.
- **L2 (Low, fixed)**: `testutil/fixtures.go` in `%v` của err nil khi thiếu
  center row — tách hai nhánh lỗi.
- **L1 (note, giữ nguyên)**: `sessions.ListPending` dùng timezone caller —
  viewer-dependent với owner; ghi nhận là hành vi đã biết, không đổi.

## Bất biến xác lập (nguồn: plan.md Authz Invariants)

- Read: owner → `center_id`; teacher → `center_id AND teacher_id`.
- Write: update/delete qua `scoped()` (owner sửa/xoá mọi row trong center);
  create luôn gán caller và **reference check strip IsOwner**.
- Bảng con suy anchor từ parent row (schedules/sessions←class,
  attendance←session, invoices/lines/adjustments←period/invoice,
  allocations←payment, statements←period).
- Không DTO nào nhận `teacher_id`/`center_id`; cross-tenant = 404/422/rỗng.
- Ngoại lệ: `zalo_accounts` teacher-keyed (tài khoản cá nhân); `auth` không
  tenant scope.

## Verification

- `go test -tags integration ./...` (Docker, testcontainers): toàn bộ xanh.
- `gofmt -l` sạch, `go build ./...` OK, `go vet -tags integration ./...` OK.
- Seeds: `TestRunIsIdempotent` xanh trên Postgres thật.
- Swagger regenerate: không diff.

## Còn lại của plan

Phase 4 (Owner Dashboard API) và Phase 5 (Center Management UI) — pending,
phụ thuộc đã thoả (4←3, 5←2).
