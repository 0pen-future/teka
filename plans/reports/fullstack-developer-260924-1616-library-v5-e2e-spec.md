# library.spec.ts rewrite for Kho học liệu v5

## Scope

Rewrote `apps/web/e2e/library.spec.ts` to match the v5 hub redesign. No app
source, seed, or unit test files were touched.

## Journey covered

- Hub with three tabs (Chương trình mẫu / Ngân hàng nội dung / Ngân hàng bài
  tập), card counts, and archiving a material to confirm it drops out of the
  lesson content picker.
- Creating an exercise without a code, confirming the server-generated
  `BT-` code, and its copy-to-clipboard button.
- Template detail: three lessons across two units, duplicating a lesson,
  switching to tree view to confirm unit grouping, creating an exercise
  group, attaching a bank exercise to a lesson and assigning it to that
  group, two score sets, one student-picker log field, publish ("Kích
  hoạt") producing the locked banner, and "Tạo bản nháp" carrying the
  exercise group forward into the new draft.
- `afterEach` deletes the RUN_CODE template, exercise, and material (each
  step tolerant of an already-cleaned state).

## Fixes made during verification

- The content picker's search-scoped empty state is `Không có nội dung nào
  khớp từ khoá.`, not `Kho chưa có nội dung nào.` (that text is only shown
  when the search box is empty). The spec had the two reversed; corrected
  the assertion after archiving the material.
- The full journey exceeds the suite's default 30s per-test timeout, so the
  test calls `test.setTimeout(120_000)`. This is an intentional trade-off:
  the task calls for one comprehensive journey test rather than several
  smaller ones, and 120s is a safe budget over the observed ~44s run.

## Verification

- `make e2e-isolated E2E_ARGS="library.spec.ts"` on the isolated `teka-e2e`
  stack: 1 passed (44.7s). Stack built, seeded, ran, and was torn down
  (`down -v`) by the Makefile target itself.
- `npx prettier --write e2e/library.spec.ts` and `npx eslint
  e2e/library.spec.ts`: clean.
- No leftover `teka-e2e` containers/volumes/network after the run.
- `courses.spec.ts` was not touched or run: it does not exercise the
  library UI this rewrite changed.
