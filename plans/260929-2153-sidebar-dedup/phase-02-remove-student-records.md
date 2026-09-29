---
phase: 2
title: "Gỡ Hồ sơ học sinh"
status: pending
priority: P2
effort: "2h"
dependencies: []
---

# Phase 2: Gỡ Hồ sơ học sinh

## Goal

Phase này gỡ hai route `/records` và `/records/:studentId` cùng mọi file chỉ phục vụ chúng. Hai route được thay bằng redirect tới trang Học sinh hợp nhất. Mục sidebar đã được gỡ ở phase 1, còn lối tắt trong tab thông tin lớp thuộc phase 5.

## Context

- `apps/web/src/features/teaching/routes.tsx:15-27` khai báo `records` và `records/:studentId`.
- Trang đích:
  - `apps/web/src/features/roster/routes.tsx` có `students/:id` với param tên là `id`;
  - `students-page.tsx:34-61` chấp nhận `?class_id=` và mở tab "Theo lớp".
- Đã kiểm tra bằng grep import rằng các file sau chỉ có hai trang records dùng:
  - `components/records-toolbar.tsx` và `components/student-records-table.tsx`: chỉ `records-page.tsx` dùng;
  - `components/score-bar-chart.tsx` và `components/student-sessions-table.tsx`: chỉ `student-record-page.tsx` dùng;
  - `lib/student-search.ts`: chỉ `records-page.tsx` dùng;
  - `lib/student-stats.ts`: chỉ các file records kể trên dùng.
- Các file còn người dùng khác nên giữ lại: `use-media-query`, `use-class-marks`, `use-month-sessions`, `use-debounced-save`, `classbook-stats` và `csv`.
- Có các comment nhắc tới records:
  - `hooks/use-class-marks.ts:24`;
  - `schemas/teaching-schemas.ts:91`;
  - `lib/teaching-store.ts:4`;
  - `hooks/use-debounced-save.ts:13`.

## Key insights

- Tính năng xem biểu đồ điểm và bảng buổi học theo từng học sinh (`student-record-page.tsx`) sẽ mất hẳn. Người dùng đã chọn gỡ, và trang Học sinh là bề mặt thay thế.
- Redirect cần giữ `class_id` và `q`, vì trang đích hiểu cả hai. Các param khác của records như `sort` thì bỏ.

## Requirements

- `/records` redirect tới `/students`, giữ `class_id` và `q` nếu có.
- `/records/:studentId` redirect tới `/students/:studentId`.
- Sau khi xoá, `tsc` và eslint không còn import mồ côi nào.

## Related files

**Create**

- `apps/web/src/features/teaching/components/records-redirect.tsx`: export `RecordsRedirect` và `StudentRecordRedirect`.
- `apps/web/src/features/teaching/__tests__/records-redirect.test.tsx`

**Modify**

- `apps/web/src/features/teaching/routes.tsx`
- `apps/web/src/features/teaching/hooks/use-class-marks.ts`: comment ở :24
- `apps/web/src/features/teaching/schemas/teaching-schemas.ts`: comment ở :91
- `apps/web/src/features/teaching/lib/teaching-store.ts`: comment ở :4
- `apps/web/src/features/teaching/hooks/use-debounced-save.ts`: comment ở :13

**Delete**

- `apps/web/src/features/teaching/pages/records-page.tsx`
- `apps/web/src/features/teaching/pages/student-record-page.tsx`
- `apps/web/src/features/teaching/components/records-toolbar.tsx`
- `apps/web/src/features/teaching/components/student-records-table.tsx`
- `apps/web/src/features/teaching/components/score-bar-chart.tsx`
- `apps/web/src/features/teaching/components/student-sessions-table.tsx`
- `apps/web/src/features/teaching/lib/student-search.ts`
- `apps/web/src/features/teaching/lib/student-stats.ts`
- `apps/web/src/features/teaching/__tests__/records-pages.test.tsx`
- `apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx`
- `apps/web/src/features/teaching/__tests__/student-records-table.test.tsx`
- `apps/web/src/features/teaching/__tests__/student-search.test.ts`
- `apps/web/src/features/teaching/__tests__/student-stats.test.ts`

## Implementation steps

1. Tạo `records-redirect.tsx` theo mẫu `roster/components/contacts-redirect.tsx`.
   - `RecordsRedirect` đọc `useSearchParams`, chỉ chép `class_id` và `q`, rồi `<Navigate to={`/students${qs}`} replace />`.
   - `StudentRecordRedirect` đọc `useParams().studentId` rồi `<Navigate to={`/students/${studentId ?? ""}`} replace />`.
   - Doc comment chỉ giải thích rằng đường dẫn cũ vẫn mở được sau khi trang Hồ sơ gộp vào trang Học sinh. Không nhắc tới plan.
2. Trong `teaching/routes.tsx`, đổi `records` và `records/:studentId` sang `lazy` tới hai component trên. Handle title đổi thành "Học sinh" và "Chi tiết học sinh" để khớp với trang đích.
3. Xoá 13 file trong danh sách Delete.
4. Sửa bốn comment để bỏ chữ "records", ví dụ "classbook batch read".
5. Viết `records-redirect.test.tsx` với `MemoryRouter` và `Routes` cho hai ca:
   - `/records?class_id=c1&q=an&sort=x` → `/students?class_id=c1&q=an`;
   - `/records/s1` → `/students/s1`.
6. Chạy `npm run typecheck` và `npm run lint` để bắt import thừa. Nếu có helper nào mồ côi thêm thì xoá, sau khi đã grep lại để chắc không còn chỗ dùng.

## Todo

- [ ] `RecordsRedirect` và `StudentRecordRedirect`
- [ ] Đổi hai route sang redirect
- [ ] Xoá 8 file nguồn và 5 file test
- [ ] Sửa bốn comment
- [ ] Test redirect

## Verification

Chạy trong `apps/web`:

- `npx vitest run src/features/teaching`
- `npm run typecheck && npm run lint && npm run format:check`
- `rg -n "records-page|student-record-page|student-search|student-stats|score-bar-chart|student-sessions-table|records-toolbar|student-records-table" apps/web/src` không trả kết quả nào.

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| Giáo viên cần xem điểm theo từng học sinh | Trung bình × Trung bình | Đây là quyết định đã chốt. Sổ lớp vẫn có bảng điểm theo buổi và theo học sinh (`score-entry-by-student.tsx`) |
| Spec e2e còn gọi `/records` | Cao × Thấp | Phase 6 viết lại `class-staff-read.spec.ts:40`, `class-staff-write.spec.ts:33` và `:200`. Nhờ redirect, các spec này không crash nhưng sẽ sai assert |
| Xoá nhầm helper còn chỗ dùng | Thấp × Trung bình | Đã grep importer trước khi lập danh sách. `tsc -b` sẽ fail nếu sót |

## Security

Phase này không đổi quyền. `/students` tự gate bằng `students.list` (`students-page.tsx:68-78`), còn `/students/:id` dựa vào API.

## Rollback

`git revert` commit của phase. Phase không có dữ liệu nào bị ảnh hưởng.
