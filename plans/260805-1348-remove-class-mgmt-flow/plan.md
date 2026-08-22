# Remove "Quản lý lớp" flow (web only)

Status: implemented — verified (104/104 vitest, eslint 0 errors, tsc clean, build OK);
code-reviewed DONE_WITH_CONCERNS — no regressions; L1 fixed, M2/L2/L3 logged as follow-ups

## Brainstorm contract

**Outcome**: The legacy class-management flow is removed from the web app.
"Cài đặt lớp" (`/classes/:id/settings`) plus the "+ Tạo lớp mới" dialog on
`/students` become the only class-editing surfaces. Truly dead code
(`deleteClass`/`useDeleteClass`) and everything the removal orphans in cascade
are deleted in the same pass.

**Constraints**:
- Web only — no Go API changes; endpoints (`DELETE /classes/:id`,
  `POST /classes/:id/archive`, dates in `PUT /classes/:id`) stay on the server.
- Keep: `/classes/:id/settings` route + `ClassSettingsPage`; `ClassDialog`
  create mode ("+ Tạo lớp mới" on `/students`); `WeekdayChips` (create dialog)
  and `WeekdayChipsMulti` (settings); `useUpdateClass`/`updateClass` and
  `classUpdateInputSchema` (settings-page save + API contract type);
  `formatWeekday` (enroll dialog); schedule add/update/delete hooks (settings diff).

**Non-goals**:
- No replacement UI for what the flow uniquely offered. User explicitly
  accepted losing (web-side): archived-class list, start/end-date editing,
  archive-class action, per-row schedule editing.
- No redesign of `/students` or the settings page.

**Acceptance criteria**:
1. Routes `/classes` and `/classes/:id` no longer exist; `/classes/:id/settings`
   still renders and saves.
2. `/students` no longer shows the "Quản lý lớp" link; tab row, ⚙ "Cài đặt lớp"
   pill and "+ Tạo lớp mới" unchanged.
3. `ClassDialog` is create-only (no `klass` prop, no edit form, no "Lưu trữ lớp").
4. No references anywhere to `ClassesPage`, `ClassDetailPage`, `ScheduleEditor`,
   `deleteClass`, `useDeleteClass`, `archiveClass`, `useArchiveClass` (grep clean).
5. Full vitest suite, eslint (0 errors), tsc, vite build all green.

## Scout evidence

- `students-page.tsx` — `<Link to="/classes">Quản lý lớp</Link>` is the flow's only
  entry point; same page uses `ClassDialog` create mode (no `klass` prop anywhere
  after `class-detail-page.tsx:87` goes).
- `routes.tsx` — `classes` → ClassesPage, `classes/:id` → ClassDetailPage,
  `classes/:id/settings` → ClassSettingsPage (keep last).
- `schedule-editor.tsx` — imported only by `class-detail-page.tsx` → cascade orphan.
- `class-dialog.tsx` — edit branch (editForm, `useArchiveClass`, "Lưu trữ lớp",
  `toEditDefaults`) reachable only via ClassDetailPage → cascade orphan; create
  branch stays. Mentions ScheduleEditor only in a doc comment.
- `use-classes.ts` — `useDeleteClass` already has zero callers (the original
  "orphan" ask); `useArchiveClass` becomes zero-caller after the dialog strip.
- `classes-api.ts` — `deleteClass` dead today; `archiveClass` dead after strip.
- `roster-handlers.ts:313` — MSW `POST /classes/:id/archive` handler only exercised
  by `class-detail-page.test.tsx` → remove with it.
- `weekday-chips.tsx:28` — comment references ScheduleEditor; comment-only fix.
- No nav/menu elsewhere links `/classes`; dashboard-layout's "Quản lý lớp học"
  subtitle is the app tagline, not part of this flow — untouched.

## Steps

1. Delete files: `pages/classes-page.tsx`, `pages/class-detail-page.tsx`,
   `components/schedule-editor.tsx`, `__tests__/class-detail-page.test.tsx`.
2. `routes.tsx`: drop the `classes` and `classes/:id` entries.
3. `students-page.tsx`: remove the "Quản lý lớp" link.
4. `class-dialog.tsx`: strip edit mode — drop `klass`/`isEdit`, `editForm`,
   `useUpdateClass`/`useArchiveClass` usage, `toEditDefaults`, edit-modal JSX,
   now-unused imports (`HvBadge`, `classUpdateInputSchema`, `Class`,
   `ClassUpdateInput`); update the doc comment.
5. `use-classes.ts`: remove `useDeleteClass`, `useArchiveClass` (+ imports).
6. `classes-api.ts`: remove `deleteClass`, `archiveClass`.
7. `roster-handlers.ts`: remove the `POST /classes/:id/archive` handler.
8. `weekday-chips.tsx`: fix the stale ScheduleEditor comment.
9. Verify: grep for removed symbols → roster vitest → full suite → lint →
   typecheck → build.

## Discovered during implementation

- `EnrollStudentDialog`'s `mode: "class"` variant ("search a student for this
  known class") had `ClassDetailPage` as its only production caller — its doc
  comment said so explicitly; only its own tests still exercised it. Removed
  in the same pass per the task's orphan-removal intent: the component is now
  single-purpose (fixed student picks a class), the props union collapsed to
  one interface, `EntitySearchList` (class-branch-only) deleted, both callers
  dropped the now-gone `mode` prop, and the two class-mode tests were removed.
  Test count: 108 → 104 (−2 class-detail, −2 enroll class-mode).

## Code review outcome (2026-08-05)

Reviewer: DONE_WITH_CONCERNS — all acceptance criteria met, no security/data-loss/
contract regressions; gates re-run independently (tsc, vitest 104/104, eslint,
build). Handled:

- M1 (staging split would produce a non-building commit) — resolved by staging
  the whole roster feature together at commit time.
- L1 (`ClassDialogProps.onSuccess` had zero callers ever) — removed in this pass,
  along with the now-unneeded `Class` type import.

**Follow-ups (accepted, not blockers):**
- M2 — deleting `class-detail-page.test.tsx` left `StudentDetailPage`'s
  end-enrollment button and `EndEnrollmentDialog` with no test coverage; add a
  small `student-detail-page.test.tsx`.
- L2 — `login-page.test.tsx` still uses `/classes` as its synthetic post-login
  redirect target (own memory-router stub, passes); swap to `/students` when next
  touched.
- L3 — accepted end state: archived classes now have no web entry point at all
  (list, settings, or archive action); capability remains API-only until a future
  surface.

## Risk

- Losing web access to archive/date-editing is intentional and user-accepted;
  API keeps the capability for a future surface.
- `class-settings-page.test.tsx` renders at path `/classes/:id/settings` via
  memory router — unaffected by route removal but re-run to confirm.
- Roster `index.ts` public surface unchanged (nothing removed is exported).
