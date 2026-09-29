# Code Review: course create/edit dialog aligned to the v5 prototype

## Scope
- Files: `apps/web/src/features/courses/components/course-dialog.tsx`, `components/course-delete-confirm.tsx` (new), `schemas/courses-schemas.ts`, `pages/courses-page.tsx`, `pages/course-detail-page.tsx`, `__tests__/courses-page.test.tsx`, `__tests__/course-detail-page.test.tsx`, `apps/web/e2e/courses.spec.ts`
- Reference pattern: `components/path-dialog.tsx`, `components/path-delete-confirm.tsx`
- Server contract checked: `apps/api/internal/features/courses/{service.go,dto.go,repository.go,routes.go}`, `internal/shared/classcode/classcode.go`
- The accepted decisions (Chặng dropped, code locked on edit, template and version saved in the same PUT/POST, archived stored version stays pickable, Xoá with 409 shown as a toast, PUT is a full replace) were taken as given and not questioned.

## Verification run
- `npm run typecheck`: clean
- `npx eslint src/features/courses e2e/courses.spec.ts`: 0 errors, 2 warnings (`react-hooks/incompatible-library` on `form.watch`, the same warning path-dialog already has; React Compiler is not enabled in `vite.config.ts`, so this is only a warning)
- `npx vitest run src/features/courses`: 5 files, 66 tests passed
- `prettier --check` on the changed files: clean

## Overall assessment
The dialog is solid. I checked each risk you listed and found no blocking bug. There are two medium findings. One is an orphaned template binding that the obvious control cannot clear. The other is duplicate DOM ids with the Setup tab. Both only show up in states the tests never set up.

## Critical Issues
None.

## High Priority
None.

## Medium Priority

### M1. An orphaned binding cannot be cleared with "— Chưa gắn —"; the save silently keeps it
`course-dialog.tsx:321-323`, `schemas/courses-schemas.ts:175-177`

When the stored version's template was deleted, `toCourseForm` sets `template_id: ""` and keeps `default_template_version_id` = the orphan id. In that state the template select already shows "— Chưa gắn —", because `value={templateId || NO_TEMPLATE}`. The user reads the course as unbound. The only hint is the coral note under the field. HvSelect calls `onValueChange` on every pick, including the current value. But `pickTemplate` does this:

```ts
const id = next === NO_TEMPLATE ? "" : next;
if (id === templateId) return;   // "" === "" -> nothing happens
```

Choosing "— Chưa gắn —" is therefore a no-op, and "Lưu thay đổi" sends the orphan id again. The API accepts it because the version is unchanged. The only way to unbind is a two-step workaround: pick some template, then pick "— Chưa gắn —". SetupView has an explicit "Bỏ chương trình mẫu" button for this case; the dialog has nothing equivalent.

Fix: let the early return compare the full binding, not only the template:
```ts
if (id === templateId && !(id === "" && versionId !== "")) return;
```
Or show a distinct selected label for the orphan state (for example "— Chương trình đã xoá —"), so that picking "— Chưa gắn —" is a real change. Add a test that starts from `default_template = null` with a stored version id, picks "— Chưa gắn —", saves, and asserts `default_template_version_id` is `null`.

### M2. Duplicate element ids when the dialog opens over the Setup tab
`course-dialog.tsx:342,354,385,402` vs `pages/course-detail-page.tsx:805,823,931`

SetupView always renders `id="course-template"` (a div, or the TemplatePicker `<select>`) and `id="course-version"` (a `<select>`). The "Sửa khóa" header button is available on every tab. The modal is portaled to the end of `<body>`, so `getElementById` and `<label for>` resolve to SetupView's elements first. Clicking the dialog's "Chương trình mẫu" or "Phiên bản mặc định" label moves focus to a page element behind the aria-hidden overlay. The accessible name still works only because both HvSelects also set `aria-label`. Playwright `getByLabel` for these fields would also be ambiguous across the page and the dialog.

This is new with this change. `course-duration` was already duplicated with the GeneralCard inline editor at `course-detail-page.tsx:471`, and that collision predates this change.

Fix: give the dialog its own ids (for example `course-dialog-template` and `course-dialog-version`, or `useId()`). Ideally do the same for `course-duration`.

## Low Priority

### L1. The locked code is still validated on edit
`schemas/courses-schemas.ts:96-107`, `course-dialog.tsx:157`

On edit the input is `readOnly`, and the submit replaces the value with `props.course.code`. Zod still runs `courseCodePattern` (`^[A-Z0-9][A-Z0-9-]*$`) on it. The server accepts `^[A-Z0-9-]{2,20}$` (`classcode.go:22`), which allows a leading dash. A course whose stored code starts with "-" (possible only if it was created outside this UI) could never be saved: the validation error sits on a field the user cannot change. The fix is to skip code validation in edit mode, for example by passing a relaxed schema to the resolver.

### L2. Misleading error while versions load or fail to load
`course-dialog.tsx:374-378`, `schemas/courses-schemas.ts:122-125`

If the user picks a template and submits before `useVersions` resolves, the form shows "Chương trình mẫu chưa có phiên bản đã phát hành — phát hành trước khi gắn". It clears once the auto-select runs. If the versions request fails, `isPending` is false, so the placeholder and the error both say "no published version" and there is no retry. Consider disabling submit while `versions.isFetching` for a picked template, and showing a separate message on `versions.isError`.

### L3. Focus drops to `<body>` after cancelling the stacked confirm
`course-delete-confirm.tsx:27`, `hv-confirm-dialog.tsx`

Radix drops focus on close when there is no Trigger (this is documented in `HvModalProps.onCloseAutoFocus`), and `HvConfirmDialog` does not expose that prop. After "Hủy", focus should go back to "Xoá", but it goes nowhere. The outer FocusScope pulls focus back on the next Tab. This is the same behaviour as `path-delete-confirm.tsx` and not a regression. Worth one design-system fix: forward `onCloseAutoFocus` through `HvConfirmDialog`.

### L4. Read-only template fields label a non-labelable `<div>`
`course-dialog.tsx:340-357`

`FieldLabel htmlFor="course-template"` / `"course-version"` point at `<div>`s, which a `<label for>` cannot reference. Use `aria-labelledby` on the div, or render a label-less heading. This mirrors SetupView.

### L5. Minor UX and copy notes
- The modal uses the default `size="md"` (448px) with 11 fields in two columns and no `stickyFooter`. On short viewports "Lưu thay đổi" scrolls out of view. Consider `size="lg"` or `stickyFooter`.
- The confirm says "Xoá vĩnh viễn", but `Service.Delete` soft-deletes. If this is prototype copy, keep it; otherwise it overstates what happens.
- The label "v5" appears in a test name (`courses-page.test.tsx:100`) and a component doc comment (`course-dialog.tsx:62`). It is a design-artifact version that will mean nothing later. It is in the spirit of the "no plan IDs in code" rule, and path-dialog does the same.
- The "Lớp đang chạy giữ phiên bản cũ" hint shows on edit even when no template is bound.

## Tests: do they assert real behaviour?
They mostly do. They check the store after POST/PUT/DELETE, the saved `default_template_version_id`, that the code is readonly and resent, that the draft v2 is not offered, that the confirm returns to the edit form on cancel, and that a 409 leaves the course in place with the edit dialog open. Gaps, ordered by how bug-prone the untested branch is:
1. No test for the orphan state in the dialog (see M1; a test there would have caught it).
2. No test that an archived stored version stays selected and resends unchanged (an accepted decision).
3. No test that switching away from the stored template and back restores the stored version rather than the newest one (`pickTemplate`).
4. No dialog test for an editor without `library.read`. The existing permission test (`course-detail-page.test.tsx:348`) covers SetupView only.
5. The name "newest published" is not proven: the fixture template has one published version (v1) and one draft, so "first published" would also pass. Add a second published version.
6. The 409 test does not assert that the confirm dialog closed.

## Checked and found correct
- **Reset / stale state:** `form.reset` runs on every open (`course-dialog.tsx:84-91`). HvModal is Radix Dialog, so the content (including TemplateFields) unmounts when closed. On reopen the child effect runs before the parent reset, and the reset wins. courses-page mounts a fresh edit dialog per row (`editing` goes to null and unmounts it), so there is no cross-row leakage. The detail page resets from the current `course` on each open.
- **Auto-select effect (`course-dialog.tsx:~312`):** no loop. The guard `versionId !== ""` exits after the first set, and `newestPublished` is a stable element reference from query data. It cannot override a user's pick, because no option sets the version to "". `useVersions` has no `keepPreviousData`, so after a template switch the effect cannot pick a version from the previous template's list.
- **Stale template or version on submit:** the refine forces a version whenever a template is set. `pickTemplate` always resets the version, and restores the stored one only for the stored template. Server field errors land on `default_template_version_id`, which is now a known form key (`use-api-form-errors.ts`).
- **Permissions:** templates and versions are fetched only when `library.read` is present (`open && canPick`, `canPick && templateId`). Without it the stored binding is resent unchanged, which the server accepts as not a new choice (`service.go:251-253`). Both callers gate the dialog on `courses.edit`. `Service.Delete` requires `PermCoursesEdit`, so the Xoá gating matches the API.
- **Stacked modal:** the confirm renders inside the parent Content's React tree. Radix layer stacking means Escape and outside-click affect only the top layer, and `HvConfirmDialog` refuses to close while the delete is pending. On success the order is confirm close, then parent close, then the page `onDeleted`. The detail-page `removeQueries` does not cause a refetch before `navigate`, because `setEditing` re-renders only CourseWorkspace, not the page that owns `useCourse`.
- **Callers:** `CourseDialog` is used only in `courses-page.tsx:259,269` and `course-detail-page.tsx:330`, and both pass `onDeleted`. `toCourseInput` is used only in the dialog. SetupView (`bind`, `course-detail-page.tsx:744`) and GeneralCard (`:432`) use `courseToInput`, which is unchanged and still carries `default_template_version_id`. The class-list and roster e2e specs use `"Đơn giá / buổi (đ)"` on the class dialog, not the course dialog, so they are unaffected.

## Recommended actions
1. Fix M1 (make clearing an orphan binding work) and add the orphan test.
2. Fix M2 (unique ids in the dialog).
3. Add tests for gaps 2 to 5.
4. Optional: L1, L2, then the design-system focus return (L3).

## Metrics
- Type errors: 0. Lint: 0 errors, 2 warnings (known RHF `watch` warning). Tests: 66/66 in `features/courses`.
- Coverage %: not measured.

## Unresolved questions
- Is "Xoá vĩnh viễn" deliberate prototype copy despite the soft delete?
