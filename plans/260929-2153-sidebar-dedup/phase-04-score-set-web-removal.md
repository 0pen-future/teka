---
phase: 4
title: "Bộ điểm: gỡ trang web và redirect"
status: completed
priority: P2
effort: "2h"
dependencies: [3]
---

# Phase 4: Bộ điểm — gỡ trang web và redirect

## Goal

Phase này gỡ trang `/center/class-config` cùng toàn bộ mã web chỉ phục vụ CRUD và gán bộ điểm. Đường dẫn cũ redirect về `/center`. Sau khi chủ trung tâm áp dụng mẫu, Sổ lớp hiển thị ngay đầu điểm mới mà không cần tải lại trang.

## Context

- `apps/web/src/features/center/routes.tsx:23-29` khai báo route `center/class-config`. Handle title của route là "Cấu hình lớp học".
- Đã grep importer và xác nhận các file sau chỉ được dùng bên trong nhóm file bị xoá:
  - trang `class-config-page.tsx`;
  - năm component bộ điểm;
  - `hooks/use-score-sets.ts`, `api/grading.ts`, `schemas/grading.ts` và `lib/score-set-components.ts`.
- `center-handlers.ts` (MSW) không có handler nào cho score-set.
- Các tham chiếu nằm ngoài `features/center` do phase khác xử lý:
  - mục nav ở `layouts/dashboard-layout.tsx:173-174` và `:216`, cùng test ở `:341`, thuộc phase 1;
  - lối tắt ở `roster/components/class-info-tab.tsx:124` và test ở `class-detail-page.test.tsx:178-180` và `:270`, thuộc phase 5;
  - `e2e/helpers/ux-routes.ts:58` thuộc phase 6.
- `roster/hooks/use-class-program.ts:43-53` (`useProgramWrite`) hiện invalidate `classProgramKeys.detail`, `classProgramKeys.lessons` và `teachingKeys.curriculum`. Sổ lớp đọc đầu điểm qua `teachingKeys.scoreComponents(classId)` (`teaching/hooks/teaching-keys.ts:13`, được dùng ở `use-component-scores.ts:22`).
- Trình soạn bộ điểm của mẫu ở `features/library/components/score-sets-editor.tsx` vẫn là nơi duy nhất để soạn bộ điểm, nên được giữ nguyên.

## Key insights

- Sau phase 3, áp dụng mẫu có thể đổi `class_score_components`. Nếu không invalidate thêm `scoreComponents`, Sổ lớp sẽ hiển thị đầu điểm cũ cho tới khi cache hết hạn.
- Redirect về `/center` là đủ. Trang đó dành cho chủ trung tâm, đúng với đối tượng từng dùng trang cấu hình.

## Requirements

- `/center/class-config` redirect (replace) tới `/center`.
- Không còn file hay import nào tới các API `/score-sets` và `/classes/:id/score-set`.
- `useApplyClassProgram` và `useRemoveClassProgram` invalidate thêm `teachingKeys.scoreComponents(classId)`.

## Related files

**Create**

- `apps/web/src/features/center/components/class-config-redirect.tsx`: component `ClassConfigRedirect` trả `<Navigate to="/center" replace />`, theo mẫu `roster/components/contacts-redirect.tsx`.

**Modify**

- `apps/web/src/features/center/routes.tsx`
- `apps/web/src/features/center/__tests__/center-page.test.tsx`: thêm ca redirect. Nếu file này không dựng router phù hợp thì đặt ca đó trong một file test mới `center-routes.test.tsx`.
- `apps/web/src/features/roster/hooks/use-class-program.ts`: thêm invalidate và sửa doc comment ở :40-42.
- `apps/web/src/features/roster/__tests__/` nếu đã có test cho `useApplyClassProgram`; grep trước khi sửa.

**Delete**

- `apps/web/src/features/center/pages/class-config-page.tsx`
- `apps/web/src/features/center/components/assign-score-set-dialog.tsx`
- `apps/web/src/features/center/components/class-score-set-table.tsx`
- `apps/web/src/features/center/components/score-set-card.tsx`
- `apps/web/src/features/center/components/score-set-editor-modal.tsx`
- `apps/web/src/features/center/components/score-set-preview-strip.tsx`
- `apps/web/src/features/center/hooks/use-score-sets.ts`
- `apps/web/src/features/center/lib/score-set-components.ts`
- `apps/web/src/features/center/api/grading.ts`
- `apps/web/src/features/center/schemas/grading.ts`
- `apps/web/src/features/center/__tests__/class-config-page.test.tsx`
- `apps/web/src/features/center/__tests__/score-set-components.test.ts`
- `apps/web/src/features/center/__tests__/score-set-editor-modal.test.tsx`

## Implementation steps

1. Tạo `class-config-redirect.tsx`. Doc comment chỉ nói rằng đường dẫn cũ vẫn mở được sau khi bộ điểm chuyển về mẫu chương trình.
2. Trong `center/routes.tsx`, giữ path `center/class-config` nhưng đổi `lazy` sang `ClassConfigRedirect` và đổi handle title thành "Cài đặt trung tâm".
3. Xoá 13 file trong danh sách Delete. Nếu thư mục `center/lib/` trống thì xoá luôn thư mục.
4. Trong `useProgramWrite`, thêm `void queryClient.invalidateQueries({ queryKey: teachingKeys.scoreComponents(classId) });`. Sửa doc comment: áp dụng mẫu còn có thể đặt đầu điểm của lớp, nên cache đầu điểm cũng cũ.
5. Viết test:
   - redirect: `MemoryRouter` ở `/center/class-config` hiển thị route `/center`;
   - hook: nếu đã có test cho `useApplyClassProgram`, thêm assert `invalidateQueries` được gọi với key `scoreComponents`, bằng cách spy `QueryClient.prototype.invalidateQueries` hoặc qua test-utils sẵn có.
6. Chạy typecheck và lint để bắt import mồ côi.

## Todo

- [x] `ClassConfigRedirect` và route
- [x] Xoá 10 file nguồn và 3 file test
- [x] Invalidate `scoreComponents` khi áp dụng hoặc gỡ chương trình
- [x] Test redirect và test invalidate

## Verification

Chạy trong `apps/web`:

- `npx vitest run src/features/center src/features/roster`
- `npm run typecheck && npm run lint && npm run format:check`
- `rg -n "score-sets|ScoreSetEditorModal|AssignScoreSetDialog|useScoreSets|class-config-page" src` chỉ được trả về `features/library` (`score-sets-editor.tsx` và các chỗ dùng nó).

Kiểm tay trên stack e2e cô lập:

1. Chủ trung tâm áp dụng một mẫu có bộ điểm cho lớp chưa có điểm.
2. Mở Sổ lớp, lưới điểm hiện đúng các cột mới.
3. Mở `/center/class-config`, trang chuyển về `/center`.

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| Xoá nhầm file còn được dùng | Thấp × Trung bình | Đã grep importer. `tsc -b` sẽ fail nếu còn sót |
| Web lên trước API (phase 3) | Thấp × Thấp | Hai phase deploy cùng bản build. Web mới không còn gọi route cũ nào, nên thứ tự lên không gây lỗi |
| Chủ trung tâm mất chỗ xem bộ điểm từng lớp | Trung bình × Thấp | Tab thông tin lớp và Sổ lớp vẫn hiện đầu điểm. Việc soạn diễn ra ở Kho học liệu |

## Security

Phase này không đổi quyền. Trang bị gỡ vốn chỉ dành cho chủ trung tâm, và API tương ứng đã được gỡ ở phase 3.

## Rollback

`git revert` commit của phase. Nếu phase 3 cũng bị revert thì trang cũ chạy lại đầy đủ. Nếu chỉ revert phase 4 thì trang cũ sẽ nhận 404 từ API, vì vậy hai phase phải được revert cùng nhau.

## Open decisions

Redirect đích là `/center`. Phương án khác là `/library`, nơi soạn bộ điểm. Plan đề xuất `/center` vì đó là nơi chủ trung tâm quen vào từ nhóm Trung tâm.
