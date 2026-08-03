---
phase: 2
title: "Roster: Contacts, Students, Classes, Enrollments"
status: pending
priority: P2
effort: "2d"
dependencies: [1]
---

# Phase 2: Roster — Contacts, Students, Classes, Enrollments

## Overview

Build the data-entry surface the whole product stands on: the paying contact,
their children, the classes with their weekly schedule and unit price, and the
enrollment records that carry the price and the join/leave dates.

The single most important constraint here is PRD R1's **closed field list**: the
student form must contain nothing beyond full name, display note, contact, and
enrollment start date. Every additional field is a legal liability under Nghị
định 13/2023 (PRD Q2). The second constraint is that the phone lives on the
contact, never on the student (`docs/schema_design.sql:83-84` vs `:107`) —
merging notifications and debts depends on it.

## Requirements

- [ ] Contact CRUD: `full_name`, `phone`. Duplicate phone within one teacher is
      rejected with a readable message (unique index
      `docs/schema_design.sql:94`).
- [ ] Contact detail lists that contact's children and links to each.
- [ ] Student create/edit form exposes exactly: `full_name` (required),
      `display_note` (optional), contact (required, existing or created inline).
      No age, grade, birth date, address, school, or photo field
      (PRD R1 AC 1).
- [ ] `display_note` is presented as the sibling-disambiguation label with an
      explanatory hint, since same-name siblings in one class are the known
      mis-tick hazard (PRD §5 edge cases; `docs/schema_design.sql:109`).
- [ ] Student delete is an **anonymize** confirm flow, not a row delete: the
      dialog states that personal data is erased and anonymized financial
      records are kept (PRD R1 AC 2; `students.anonymized_at`,
      `docs/schema_design.sql:116`).
- [ ] Class CRUD: `name`, `start_date`, optional `end_date`,
      `default_unit_price`, `status`.
- [ ] Weekly schedule editor per class: weekday, start time, duration,
      `effective_from` / `effective_to` (`docs/schema_design.sql:145`).
- [ ] Enrollment: add a student to a class with `started_on` defaulting to
      today, showing explicitly that billing starts from the next session
      (PRD R1 AC "billed from the next session onwards").
- [ ] End an enrollment mid-cycle by setting `ended_on`, with a note that
      existing debt is preserved (PRD §5 edge case "nghỉ hẳn giữa chu kỳ").
- [ ] A student can be enrolled in several classes; the student detail page
      lists all enrollments.

## Architecture

One feature folder, `apps/web/src/features/roster`, holding four entities that
share types, query keys, and dialogs. Splitting into four features would
duplicate the contact/student cross-references and their cache invalidations;
one folder with four API modules is the DRY choice here.

```
features/roster/
  api/{contacts,students,classes,enrollments}-api.ts
  schemas/roster-schemas.ts
  types/roster-types.ts
  hooks/{use-contacts,use-students,use-classes,use-enrollments}.ts
  components/…dialogs, schedule-editor, contact-picker
  pages/{contacts,contact-detail,students,student-detail,classes,class-detail}-page.tsx
  routes.tsx
  index.ts
```

**Cache invalidation graph.** Mutations cross entity boundaries, so the query
keys must be invalidated together:

| Mutation | Invalidate |
|---|---|
| create/update contact | contacts lists, that contact's detail |
| anonymize student | students lists, that student's detail, its contact's detail |
| create enrollment | that class's enrollments, that student's detail, class detail |
| end enrollment | same as create |
| update class schedule | class detail; **not** sessions — session generation is server-side and phase 3 refetches on its own screen |

**Contact picker.** Creating a student requires a contact. Rather than forcing a
two-screen flow, the student dialog embeds a combobox that searches existing
contacts by name or phone and offers "Thêm người liên hệ mới" inline, which
opens the contact dialog on top and selects the result on success. Teachers
enter students in batches; a two-screen flow would double the taps.

**Money input.** `default_unit_price` and any price display use plain integer
đồng. The input is a numeric text field with thousands separators applied on
blur, parsed back with a `parseMoney` helper next to `formatMoney`
(`apps/web/src/lib/utils/format.ts`, phase 1). No float arithmetic anywhere —
the API stores `BIGINT` đồng (`docs/schema_design.sql:24`).

**Assumed API contract** (reconcile with plan 02):

| Method | Path | Notes |
|---|---|---|
| GET | `/contacts?page&q` | paginated envelope (`parseList`) |
| POST/PATCH | `/contacts`, `/contacts/:id` | `{ full_name, phone }` |
| DELETE | `/contacts/:id` | soft delete; 409 when children still reference it (FK is `ON DELETE RESTRICT`, `docs/schema_design.sql:117`) |
| GET | `/contacts/:id` | contact + `students[]` |
| GET | `/students?page&q&class_id` | paginated |
| POST/PATCH | `/students`, `/students/:id` | `{ full_name, display_note, contact_id }` |
| DELETE | `/students/:id` | anonymize semantics |
| GET | `/students/:id` | student + `enrollments[]` |
| GET/POST/PATCH | `/classes`, `/classes/:id` | `{ name, start_date, end_date, default_unit_price, status }` |
| GET/POST/PATCH/DELETE | `/classes/:id/schedules` | `{ weekday, start_time, duration_min, effective_from, effective_to }` |
| GET | `/classes/:id/enrollments` | enrollment + student name + `display_note` |
| POST | `/enrollments` | `{ student_id, class_id, started_on }`; server copies `default_unit_price` into `unit_price` |
| PATCH | `/enrollments/:id` | `{ ended_on }` |

## Design Spec (prototype `students` + `modalClass` + `modalAdd`)

The prototype consolidates the roster into **one nav screen "Lớp & học sinh"
(`/students`)**; contacts/classes/students detail pages stay as off-nav
drill-downs (see plan "Design Source" §3). All styling from DS tokens and
`@/components/hv`.

**Consolidated screen (`students`).**

- Header row: h1 `font-display` 700 24px "Lớp & học sinh" + two `HvButton
  size="sm"`: "+ Tạo lớp mới" (`secondary`) and "+ Thêm học sinh" (`primary`).
- Class tabs: pill row (`--radius-pill`), each "Tên lớp · N" — active
  `--mint-400` bg white text + `shadow-press-mint`; idle white bg `--ink-500`
  with `--line-200` border. An "Tất cả" tab first.
- Table inside an `HvCard flat` (padding 0): grid columns HỌC SINH / NGƯỜI LIÊN
  HỆ / NHẬP HỌC / BUỔI THÁNG NÀY / badge column. Header cells 11px `--ink-400`
  letter-spaced uppercase; body rows 15px, `--line-200` row separators,
  min-height ≥48px.
  - Student cell: name 700 + `display_note` as an `HvBadge` (sky) suffix when
    present — the prototype promotes the note to a visible badge for same-name
    siblings.
  - Contact cell: name 14px + phone 13px `--ink-400`; siblings share a "N anh
    em / 1 PH" sky pill on the first sibling row.
  - Badges: unpaid balance → `StatusPill unpaid` ("Nợ cũ 300.000 ₫" style
    label); mid-period join → `StatusPill partial` ("Giữa kỳ · 15/07").
- Responsive: under `md` (phone) rows collapse to stacked cards (name +
  contact + badges), same data, no horizontal scroll; at `md`–`lg` (tablet)
  the table drops the NHẬP HỌC column and centers at `--w-content`; at `lg+`
  all five columns render inside the 1080px content area — no horizontal
  scroll at any tier.
- Footer privacy note 13px `--ink-400`: "Chỉ lưu: họ tên · ngày nhập học · lớp
  · người liên hệ." — the visible face of the closed field list.

**Modals across tiers** (applies to every `HvModal` here): bottom sheet
(full-width, rounded top) under `sm`; centered panel from `sm`, `max-w-md`,
never wider than 480px on tablet/desktop.

**`ClassDialog` = prototype `modalClass`** (via `HvModal`: overlay
`rgba(28,58,49,.4)`, white panel `--radius-xl`, popIn 220ms `--ease-soft`):
name input; weekday picker as **7 toggle chips T2…CN** (selected `--mint-400`
white, idle white + line border, ≥44px); time input; unit price with step
5.000 and live formatted preview; start date. Footer note: per-enrollment
price override arrives in P1 (sibling discounts).

**`StudentDialog` = prototype `modalAdd`**: three inputs (closed list), class
select, `ContactPicker` whose last entry "— Tạo người liên hệ mới —" expands
inline name+phone fields inside the same modal (prototype behavior; the
separate `ContactDialog` remains for edits from the contact detail page).
Privacy note repeated at the bottom of the modal.

**Detail pages (kit-composed, off-nav).** `HvCard` sections on `--cream-100`;
headers `font-display` 700; anonymize confirm uses `HvModal` with an
`HvButton variant="danger"` action. `ScheduleEditor` reuses the 7-chip weekday
row from `modalClass`.

## Related Code Files

**Create**

- `apps/web/src/features/roster/api/contacts-api.ts`
- `apps/web/src/features/roster/api/students-api.ts`
- `apps/web/src/features/roster/api/classes-api.ts`
- `apps/web/src/features/roster/api/enrollments-api.ts`
- `apps/web/src/features/roster/schemas/roster-schemas.ts` — `contactSchema`,
  `studentSchema`, `classSchema`, `scheduleSchema`, `enrollmentSchema`, plus the
  create/update input schemas.
- `apps/web/src/features/roster/types/roster-types.ts`
- `apps/web/src/features/roster/hooks/use-contacts.ts`
- `apps/web/src/features/roster/hooks/use-students.ts`
- `apps/web/src/features/roster/hooks/use-classes.ts`
- `apps/web/src/features/roster/hooks/use-enrollments.ts`
- `apps/web/src/features/roster/components/contact-dialog.tsx`
- `apps/web/src/features/roster/components/contact-picker.tsx`
- `apps/web/src/features/roster/components/student-dialog.tsx`
- `apps/web/src/features/roster/components/anonymize-student-dialog.tsx`
- `apps/web/src/features/roster/components/class-dialog.tsx`
- `apps/web/src/features/roster/components/schedule-editor.tsx`
- `apps/web/src/features/roster/components/enroll-student-dialog.tsx`
- `apps/web/src/features/roster/components/end-enrollment-dialog.tsx`
- `apps/web/src/features/roster/pages/contacts-page.tsx`
- `apps/web/src/features/roster/pages/contact-detail-page.tsx`
- `apps/web/src/features/roster/pages/students-page.tsx`
- `apps/web/src/features/roster/pages/student-detail-page.tsx`
- `apps/web/src/features/roster/pages/classes-page.tsx`
- `apps/web/src/features/roster/pages/class-detail-page.tsx`
- `apps/web/src/features/roster/routes.tsx`
- `apps/web/src/features/roster/index.ts`
- `apps/web/src/features/roster/__tests__/student-dialog.test.tsx`
- `apps/web/src/features/roster/__tests__/anonymize-student-dialog.test.tsx`
- `apps/web/src/features/roster/__tests__/enroll-student-dialog.test.tsx`
- `apps/web/src/features/roster/__tests__/class-detail-page.test.tsx`
- `apps/web/e2e/roster.spec.ts`

**Modify**

- `apps/web/src/app/router.tsx` — mount `rosterRoutes` inside the protected
  dashboard layout alongside `dashboardRoutes` (`:31`).
- `apps/web/src/test/msw/handlers.ts` — roster fixtures and handlers.
- `apps/web/src/lib/utils/format.ts` — add `parseMoney` and `formatWeekday`
  (0 = Chủ nhật, matching `class_schedules.weekday`,
  `docs/schema_design.sql:149`).

**Delete**

- None.

## Implementation Steps

1. Write `roster-schemas.ts` mirroring the DB columns exactly. Cap string
   lengths to the DB's: `full_name` 100, `display_note` 50, class `name` 100
   (`docs/schema_design.sql:107,109,130`). Enforce `unit_price >= 0` and
   `ended_on >= started_on` client-side so obvious mistakes never reach the API.
2. Write the four API modules following `apps/web/src/features/users/api/users-api.ts:7`
   — `apiClient` + `parseData` / `parseList`, one exported function per endpoint.
3. Write the four hook modules with key factories mirroring
   `apps/web/src/features/users/hooks/use-users.ts:6` (`contactsKeys`,
   `studentsKeys`, `classesKeys`, `enrollmentsKeys`) and mutations wired to the
   invalidation graph in Architecture.
4. Build `ContactDialog` (create/edit) on the existing `Dialog` + `Field`
   primitives, following `apps/web/src/features/users/components/create-user-dialog.tsx`.
   Map the duplicate-phone 409 onto the phone field through `useApiFormErrors`.
5. Build `ContactPicker`: a searchable select over `useContactsList({ q })` with
   a debounce reusing the 300ms pattern at
   `apps/web/src/features/users/pages/users-page.tsx:34`, plus an inline
   "Thêm người liên hệ mới" entry that opens `ContactDialog` and auto-selects
   the created contact.
6. Build `StudentDialog` with exactly three inputs: "Họ và tên",
   "Ghi chú phân biệt" (hint: "Dùng khi hai anh em cùng lớp trùng tên, ví dụ:
   An lớp 9A"), and `ContactPicker`. Add a code comment stating the field list
   is closed for legal reasons so a future contributor does not casually extend
   it.
7. Build `AnonymizeStudentDialog` on `ConfirmDialog`
   (`apps/web/src/components/shared/confirm-dialog.tsx`) with destructive
   styling and copy: "Xoá dữ liệu cá nhân của {tên}. Phiếu thu và lịch sử thanh
   toán được giữ lại ở dạng ẩn danh." Require the teacher to press the
   destructive button; no type-to-confirm (extra friction for a routine action).
8. Build `StudentsPage` as the consolidated "Lớp & học sinh" screen per the
   Design Spec: class pill tabs (URL-driven `class_id` filter), the combined
   student × contact table styled per the prototype (DataTable machinery from
   `apps/web/src/components/shared/data-table.tsx` may be reused for
   sort/page state, but the cells follow the Design Spec, not the default
   table look), header buttons opening `ClassDialog` / `StudentDialog`, and
   the privacy footer note. `ContactsPage` becomes a simple off-nav list
   reachable from the table's contact cells (URL-driven search kept, styled
   as `HvCard` rows).
9. Build `ContactDetailPage`: contact header (name, phone, tap-to-call `tel:`
   link) then the children list, each row linking to the student detail page.
10. Build `StudentDetailPage`: student header, contact link, and the enrollment
    list (class name, `started_on`, `ended_on` or "Đang học", `unit_price`), with
    "Ghi danh vào lớp" and per-row "Kết thúc ghi danh" actions.
11. Build `ClassDialog` per the `modalClass` Design Spec (`name`, 7-chip
    weekday picker feeding the initial schedule entry, time, `start_date`,
    `end_date`, `default_unit_price` via the money input with step 5.000,
    `status`) and `ClassesPage` as an off-nav list (active classes first,
    weekday summary, student count) reachable from the class tabs' overflow
    ("Quản lý lớp") rather than primary nav.
12. Build `ScheduleEditor` on `ClassDetailPage`: a row per schedule entry with
    weekday select (Chủ nhật…Thứ 7 mapped to 0…6), time input,
    `duration_min` (default 90, matching `docs/schema_design.sql:151`), and
    effective dates. Adding or removing a row is one mutation each; warn in copy
    that changing the schedule affects only future generated sessions.
13. Build `EnrollStudentDialog`: student picker (search by name), `started_on`
    date defaulting to today, and a read-only line showing the inherited unit
    price with the note "Đơn giá kế thừa từ lớp, V1 không sửa được"
    (`docs/schema_design.sql:163`). After success, show a toast stating billing
    starts from the next session on or after `started_on`.
14. Build `EndEnrollmentDialog`: `ended_on` date (default today) plus copy
    "Học phí được tính tới buổi cuối cùng. Nợ cũ (nếu có) vẫn được giữ."
15. Write `routes.tsx` exporting `rosterRoutes` with lazy imports following
    `apps/web/src/features/users/routes.tsx:7`, paths `contacts`,
    `contacts/:id`, `students`, `students/:id`, `classes`, `classes/:id`.
    Register them in `apps/web/src/app/router.tsx:31`.
16. Add msw fixtures: two contacts (one with two children, one of them a
    same-name sibling pair) and one class with a weekly schedule.
17. Write the vitest suites: the student dialog renders exactly three inputs
    (assert by counting `textbox`/`combobox` roles — this is the guard for R1 AC
    1), the anonymize dialog copy mentions retained financial records, enrolling
    surfaces the next-session note, and the class detail page renders the
    schedule rows.
18. Write `apps/web/e2e/roster.spec.ts`: create a contact, create two students
    under it, create a class with one weekly slot, enroll both students, end one
    enrollment. Use a `Date.now()` suffix in names to stay re-runnable under the
    single-worker config (`apps/web/playwright.config.ts:11`).
19. Run typecheck, lint, and vitest.

## Success Criteria

- [ ] The student form renders exactly three inputs; a test asserts the count so
      a future field addition fails CI.
- [ ] Creating a contact with an already-used phone shows the error on the phone
      field, not as a generic toast.
- [ ] A contact detail page shows both children of a two-child family.
- [ ] Deleting a student shows anonymize wording and, after confirming, the
      student disappears from the roster while their invoices remain reachable
      from the collections screens.
- [ ] Enrolling a student into a class that already has sessions shows the
      "billed from the next session" note, and the resulting enrollment carries
      the class's default unit price.
- [ ] Ending an enrollment sets `ended_on` and the student still appears in the
      class history.
- [ ] A student enrolled in two classes shows both on their detail page.
- [ ] Two same-name siblings are distinguishable everywhere they are listed,
      through `display_note` rendered as a visible badge (prototype treatment).
- [ ] The consolidated screen matches the prototype `students` layout: pill
      class tabs with press shadow on active, the 5-column table with
      uppercase 11px headers, sibling/nợ-cũ/giữa-kỳ pills, and the privacy
      footer note — all via DS tokens (no raw hex in `features/roster`).
- [ ] Create-class and add-student run entirely inside `HvModal` dialogs
      matching `modalClass` / `modalAdd`, including the 7-chip weekday picker
      and the inline new-contact expansion.
- [ ] typecheck, lint, vitest, and `roster.spec.ts` pass.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| A future contributor adds a field to the student form and creates legal exposure | Medium | High | Test asserting the input count, plus a comment at the schema stating the list is closed. |
| Contact delete fails with a FK error the teacher cannot interpret | Medium | Medium | `contacts` is `ON DELETE RESTRICT` from `students` (`docs/schema_design.sql:117`); map the 409 to "Còn học sinh đang trỏ về người liên hệ này" with a link to the children. |
| Schedule editor implies retroactive session changes | Medium | Medium | Explicit copy that only future sessions are affected; `effective_from` / `effective_to` are exposed rather than hidden. |
| Teacher enters price in thousands ("150") meaning 150.000 | Medium | High | Money input shows a live formatted preview under the field ("150 ₫") so the mistake is visible before saving. |
| Contact picker search is slow with 300 contacts | Low | Medium | Server-side `q` filter plus the 300ms debounce; never fetch the full list into the client. |

**Rollback:** the feature is additive — one new folder plus a route
registration. Reverting the route registration hides all roster screens without
affecting auth, dashboard, or later phases.
