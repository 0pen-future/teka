# Phase 2 — Lớp & học sinh v2 alignment

Plan: `plans/260813-2128-prototype-v2-teaching-screens/` · Phase file: `phase-02-lop-va-hoc-sinh-v2-alignment.md` · Mode: TDD

## Result

`/students` now matches the Prototype v2 "Lớp và học sinh" screen: v2 subtitle,
`⚙ Cài đặt lớp` outline pill right-aligned in the CHỌN LỚP row (links to the
selected class's settings, hidden when none), and the table re-laid to
HỌC SINH / NGƯỜI LIÊN HỆ / NHẬP HỌC / BUỔI T{m} / actions with the prototype's
`2fr 2fr 1.1fr 1fr 1.6fr` proportions via `<colgroup>` percentages. All roster
behaviors (wizard, enroll, edit, anonymize, tabs, search) unchanged.

## Data

- One `useSessionsList(selectedClassId, currentMonth())` query; key identical
  to class-settings via the shared `features/roster/lib/current-month.ts`
  helper → React Query dedupes. Range is month-start → **today** because
  `GET /classes/:id/sessions` materializes missing rows server-side; future
  dates must not be requested.
- One `useEnrollmentsList({class_id, active: true, per_page: 100})` (gated on
  a valid selected class). Per-student BUỔI count = non-cancelled sessions
  inside the enrollment window; NHẬP HỌC = `started_on` as dd/MM.
- No N+1; a spy test asserts zero /sessions and /enrollments requests on the
  unenrolled tab.

## Review round (code-reviewer: DONE_WITH_CONCERNS → all fixed)

- H1 `keepPreviousData` leak across class switches → enrollment map building
  gated on `selectedClassId`.
- H2 whole-month range writes future planned session rows → shared
  `currentMonth()` (to = today) extracted from class-settings.
- M1 missing `active: true` (pagination could drop long-tenured students) → added.
- M2 transient "0" while sessions pending → "—" until data arrives.
- M3 display-note badge under an "actions"-only sr header → renamed to
  "Ghi chú và hành động".
- M4 criterion 3 untested → no-request spy test added.
- M5 false key-dedup comment → true after helper extraction.
- Low: ⚙ pill under 44px floor → `min-h-11 inline-flex items-center`.

Full findings: `code-reviewer-260813-2149-phase-02-students-v2.md`.

## Tests & gates

- v2 layout describe runs under a frozen clock (`vi.useFakeTimers({toFake:
  ["Date"]})`, system time 2026-08-20) with the store re-seeded under the fake
  clock, so month-dependent expectations never drift with the run date.
- Suite 280/280 · eslint 0 errors (4 pre-existing react-hooks warnings) ·
  `tsc -b` clean · `vite build` clean.

## Deviations

- BUỔI T{m} counts scheduled non-cancelled sessions month-to-date, not the
  prototype's attendance-based "5 học · 1 vắng" — plan-validated (workload
  view; attendance detail belongs to the classbook screen).
- Class tabs omit the prototype's per-class student counts — would require a
  per-class query fan-out the plan forbids.
