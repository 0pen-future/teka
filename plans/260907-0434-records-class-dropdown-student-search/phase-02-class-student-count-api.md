---
phase: 2
title: "API: student_count trên danh sách lớp"
status: completed
priority: P1
effort: "4h"
dependencies: []
---

# Phase 2: API — `student_count` trên danh sách lớp (additive)

## Overview

Mockup hiện `Lớp 3A · 28 HS` trên trigger và `28 HS` ở từng mục. Danh sách lớp hiện không có sĩ số (`ClassResponse` chưa có, plan `260805-1516` từng hoãn vì thiếu backend). Phase này thêm trường additive `student_count` = số ghi danh **đang học** vào response danh sách/chi tiết lớp và đưa vào schema zod phía web. Phụ thuộc quyết định **D1**; nếu người dùng chọn "chỉ frontend", bỏ phase này và Phase 3 bỏ `{n} HS` ở mục.

## Requirements

- Functional:
  - `GET /classes` (mọi biến thể readable) và `GET /classes/:id` trả `student_count` (int ≥ 0) cho từng lớp.
  - Định nghĩa "đang học" **trùng** với bộ lọc `active=true` của `GET /enrollments` (đây là nguồn dòng bảng trên `/records`), để `student_count` == số dòng bảng khi không tìm kiếm.
  - Dashboard và các producer khác dùng lại mapper vẫn compile, gửi `0` (giống cách `my_staff_roles` đang làm).
- Non-functional: 1 truy vấn COUNT … GROUP BY cho cả trang (không N+1); truy vấn phải đi qua scope tenancy (analyzer `scopelint` trong `apps/api`, xem commit 7e783e8) — dùng cùng cơ chế scope như `CountOpenEnrollments`/`ListReadable`.

## Architecture

```
handler List/Get ─► service.ListReadable ─► repo.ListReadable (rows)
                                      └──► repo.CountActiveEnrollmentsByClass(ctx, sc, ids) → map[uuid]int64
                     FromModelWithRoles(class, roles) + resp.StudentCount = counts[id]
```

Web: `classSchema` thêm `student_count: z.number().int().nonnegative().default(0)` (default để response cũ/cached không vỡ, cùng lý do với `my_staff_roles`). MSW `GET /classes` tính từ `store.enrollments` (cùng predicate với handler `GET /enrollments?active=true` trong `roster-handlers.ts`).

## Related Code Files

- Modify: `apps/api/internal/features/classes/dto.go` (`StudentCount int \`json:"student_count"\``; comment nêu predicate)
- Modify: `apps/api/internal/features/classes/repository.go` (`CountActiveEnrollmentsByClass`; interface repo)
- Modify: `apps/api/internal/features/classes/service.go` (`ListReadable`/`GetReadableWithRoles` trả thêm counts, hoặc helper `studentCounts(ctx, sc, ids)`)
- Modify: `apps/api/internal/features/classes/handler.go` (gán `StudentCount`)
- Modify: `apps/api/internal/features/classes/{service_test,handler_test,integration_test}.go` (case: 2 lớp, 1 lớp có 3 ghi danh trong đó 1 đã kết thúc → `student_count` 2 và 0)
- Read only: `apps/api/internal/features/enrollments/repository.go` (predicate `active`), `apps/api/internal/features/sessions/dto.go` (đặt tên `student_count` cho nhất quán)
- Modify: `apps/web/src/features/roster/schemas/roster-schemas.ts` (`classSchema`)
- Modify: `apps/web/src/features/roster/__tests__/roster-handlers.ts` (`classWithSchedule.student_count`, handler `GET /classes` và `/classes/:id` tính count)
- Modify: mọi fixture typed `Class` khác (grep `: Class = {` và `as Class` trong `apps/web/src`) thêm `student_count`

## Implementation Steps

1. Đọc predicate `active` trong `enrollments/repository.go`; ghi lại dạng SQL chính xác (ví dụ `ended_on IS NULL OR ended_on > CURRENT_DATE`, `deleted_at IS NULL`).
2. Thêm `CountActiveEnrollmentsByClass(ctx, sc, classIDs []uuid.UUID) (map[uuid.UUID]int64, error)` vào repository: `SELECT class_id, COUNT(*) … WHERE class_id IN ? AND <predicate> GROUP BY class_id`, đi qua scope như các truy vấn readable khác; trả map rỗng khi `len(ids)==0` (không query).
3. Service: gom ids sau `ListReadable`, gọi count, trả map cùng roles (đổi chữ ký `ListReadable` → cập nhật mọi caller; `go build ./...` chỉ ra). Với `GetReadableWithRoles` (chi tiết lớp) gọi count cho 1 id.
4. Handler: gán `resp.StudentCount = counts[class.ID]` cạnh `FromModelWithRoles`.
5. Go test: service (mock repo trả map), handler/integration (seed enrollments, assert JSON). Chạy `go test ./internal/features/classes/... ./internal/features/enrollments/...` và analyzer scopelint (`go vet`/lệnh trong `apps/api` Makefile hoặc CI workflow — đọc `.github/workflows` để lấy đúng lệnh).
6. Web: cập nhật `classSchema`, fixtures, MSW handler; chạy `npm run typecheck && npx vitest run src/features/roster src/features/teaching src/features/attendance`.

## Todo

- [x] Predicate "đang học" trích từ enrollments repo, ghi vào comment dto
- [x] Repo `CountActiveEnrollmentsByClass` + scope + test
- [x] Service/handler nối counts, caller compile
- [x] Go test + scopelint xanh
- [x] Zod `classSchema.student_count` default 0 + fixtures + MSW tính count
- [x] Web typecheck/test xanh

## Success Criteria

- [x] `curl GET /classes` trên dev stack trả `student_count` đúng bằng `GET /enrollments?class_id=…&active=true` `meta.total` cho từng lớp.
- [x] Không thay đổi trường nào khác trong `ClassResponse`; dashboard test xanh.
- [x] `Class.student_count` có kiểu `number` phía web; không fixture nào thiếu trường (typecheck xanh).

## Risk Assessment

- **Predicate lệch** giữa count và list → tín hiệu: test integration so sánh 2 endpoint đỏ → sửa predicate, không sửa test.
- **Chữ ký `ListReadable` đổi** kéo theo caller ngoài feature classes (grep trước khi sửa). Nếu >3 caller, thay bằng helper riêng `StudentCounts(ctx, sc, ids)` gọi từ handler để không đổi chữ ký.
- **Người dùng chọn D1 = frontend-only** → đánh dấu phase này `skipped` bằng CLI, cập nhật Phase 3 bỏ sĩ số ở mục; sĩ số trigger lấy từ `rows.length`.

## Kết quả

Commit `fa326d1`. Chữ ký Go giữ nguyên, `student_count` additive; swagger regen. Bằng chứng SC1: integration test repo/service + trigger `Toán 8 - Tối Thứ Ba · 2 HS` khớp 2 dòng bảng trên stack e2e cách ly (không curl dev stack). Sau review (commit `a13f07d`): đếm theo đúng bộ lọc đọc của enrollments qua `readScopedEnrollments` thay vì center-wide, có test tích hợp `TestStudentCountsFollowEnrollmentReadScope`.
