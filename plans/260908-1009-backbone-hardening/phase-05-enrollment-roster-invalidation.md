---
phase: 5
title: "Web: ghi danh invalidate roster điểm danh"
status: pending
priority: P2
effort: "0.5d"
dependencies: []
---

# Phase 5: Web — mutation ghi danh invalidate roster/session điểm danh (finding 10)

## Overview

Mọi mutation thay đổi ghi danh (tạo, kết thúc, xoá, import roster commit, anonymize/xoá học sinh) invalidate thêm `sessionsKeys.all` để sheet điểm danh đang mở và `student_count` trên list/detail session refetch ngay, thay vì chờ hết 30s staleTime hoặc remount.

## Requirements

- Functional:
  - `invalidateEnrollmentSurfaces` (dùng bởi `useCreateEnrollment`, `useEndEnrollment`) và `useDeleteEnrollment` thêm `sessionsKeys.all`.
  - `useImportRoster` (`use-roster-import.ts`) chỉ khi kết quả là commit (không phải dry-run — kiểm tra cách hook phân biệt; nếu hook invalidate cả khi dry-run hiện tại thì giữ nguyên hành vi đó và chỉ thêm key) thêm `sessionsKeys.all`.
  - Mutation trong `use-students.ts` đang invalidate `enrollmentsKeys.all` (xoá/anonymize học sinh kéo theo ghi danh) thêm `sessionsKeys.all`.
  - Không đổi `sessionsKeys`, không tạo registry chung, không đụng hai chỗ thiếu khác review nêu.
- Non-functional: import qua barrel `@/features/attendance` (tiền lệ `use-classes.ts`); comment ngắn giải thích tại sao session phụ thuộc enrollment (roster + `student_count`).

## Architecture

```text
enrollment mutation (create | end | delete | import commit | student anonymize)
  onSuccess/onSettled → invalidate: enrollmentsKeys.all, studentsKeys.all, classesKeys.all (hiện có)
                                  + sessionsKeys.all   ← mới (roster, list, detail, periodForDate)
```

Dùng `sessionsKeys.all` thay vì ba key con (`rosters()`, `lists()`, `details()`) vì `sessionSchema` mang `student_count` trên cả list và detail, và `useReassignTeacher` đã dùng `sessionsKeys.all` làm tiền lệ. Refetch chỉ xảy ra với query đang mount nên chi phí không đáng kể.

## Related Code Files

- Modify: `apps/web/src/features/roster/hooks/use-enrollments.ts` — import `sessionsKeys`; `invalidateEnrollmentSurfaces` (~dòng 65-68) và `useDeleteEnrollment` (~97-99).
- Modify: `apps/web/src/features/roster/hooks/use-roster-import.ts` — `useImportRoster` (~35-48) thêm key vào vòng invalidate khi commit.
- Modify: `apps/web/src/features/roster/hooks/use-students.ts` — mutation gần dòng 77 đang invalidate `enrollmentsKeys.all`.
- Create: `apps/web/src/features/roster/hooks/__tests__/use-enrollments.test.tsx` (hoặc mở rộng test hook/dialog hiện có nếu đã có harness `renderHook` với QueryClientProvider).
- Không đổi: `attendance` feature, `sessionsKeys`, docs (không có mục "cache invalidation graph" trong `docs/` — comment trong code nhắc tới graph này là tham chiếu cũ; sửa comment thành mô tả trực tiếp phụ thuộc, không tạo doc mới).

## Implementation Steps

1. Grep `enrollmentsKeys.all` trong `apps/web/src` để liệt kê đủ mutation cần sửa (kỳ vọng: `use-enrollments.ts`, `use-roster-import.ts`, `use-students.ts`; nếu có thêm, bao gồm và ghi trong report).
2. `use-enrollments.ts`: thêm `void queryClient.invalidateQueries({ queryKey: sessionsKeys.all })` vào `invalidateEnrollmentSurfaces` và `useDeleteEnrollment`; cập nhật comment đầu helper: ghi danh ảnh hưởng roster session và `student_count`.
3. `use-roster-import.ts`, `use-students.ts`: thêm key tương tự (giữ cấu trúc vòng lặp nếu file đang dùng mảng key).
4. Test (MSW + `QueryClientProvider` với `queryClient` mới mỗi test, `renderHook` từ `@testing-library/react`; dùng `renderWithProviders` nếu cần router):
   - Seed `queryClient.setQueryData(sessionsKeys.roster("s1"), {...})` và `sessionsKeys.detail("s1")`; gọi `useCreateEnrollment().mutateAsync(...)` với MSW trả 201 → `queryClient.getQueryState(sessionsKeys.roster("s1"))?.isInvalidated === true` và detail cũng vậy.
   - Lặp cho `useEndEnrollment`, `useDeleteEnrollment`, `useImportRoster` (commit), mutation trong `use-students.ts`.
   - Kiểm tra dry-run import (nếu hook có nhánh) **không** invalidate session (giữ hành vi hiện tại của các key khác làm chuẩn).
5. Chạy thủ công nếu có stack e2e (`compose -p teka-e2e`, memory `teka-e2e-isolated-stack`): mở sheet điểm danh ở tab 1, ghi danh học sinh mới ở tab 2 trong cùng tab trình duyệt (cùng queryClient) → sheet refetch ngay. Không bắt buộc cho DoD; ghi kết quả trong report nếu chạy.
6. Verify: `cd apps/web && npm run typecheck && npm run test`, `make lint-web`.

## Success Criteria

- [ ] Sau mỗi mutation ghi danh (5 đường), `sessionsKeys.roster(id)` và `sessionsKeys.detail(id)` ở trạng thái invalidated.
- [ ] Không thay đổi key factory nào; import qua barrel; lint/typecheck/test xanh.
- [ ] Comment tại helper mô tả phụ thuộc thay vì tham chiếu tới doc không tồn tại.

## Risk Assessment

- **Vòng import roster ↔ attendance qua barrel**: nếu `attendance` cũng import từ `roster` barrel, ESM xử lý được nhưng có thể gây `undefined` khi module-level evaluation phụ thuộc lẫn nhau. `use-classes.ts` đã import cùng cách và pass → rủi ro thấp; tín hiệu: test hook lỗi `sessionsKeys is undefined` → import trực tiếp từ `@/features/attendance/hooks/use-sessions` (kiểm tra `apps/web/CLAUDE.md` cho phép không) hoặc chuyển key vào module không phụ thuộc.
- **Refetch `periodForDate` thừa** (staleTime 5 phút) — một GET nhỏ chỉ khi query đang mount; chấp nhận để giữ một dòng invalidate.
