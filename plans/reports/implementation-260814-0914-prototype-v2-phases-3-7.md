# Implementation Report — Prototype v2 teaching screens, Phases 3–7

Plan: `plans/260813-2128-prototype-v2-teaching-screens/` · Mode: `--auto` · Date: 2026-08-14

## Result

All 7 phases completed. Four teaching screens live in `apps/web` on the Học Vui
Mỗi Ngày design system, UI-only (no `apps/api` changes, no new endpoints, no
schema changes). Real APIs for classes/students/sessions/attendance/enrollments;
scores, giáo án, curriculum, and notes in the client-side teaching store
(localStorage, keyed per center name).

## Gates

| Gate | Result |
|------|--------|
| Full web suite | 336/336 passed (53 files) — re-verified by `tester` subagent after final refactor |
| eslint `src` | 0 errors (4 pre-existing `react-hooks/incompatible-library` warnings in roster/auth react-hook-form pages — out of scope) |
| `tsc --noEmit` + `npm run build` (`tsc -b`) | clean / passes |
| DS token audit (`#hex`/`rgba(` grep over teaching + touched roster files) | clean |
| A11y | global `:focus-visible` ring (`src/styles/globals.css:229`); labeled inputs/textareas; queue rows are real buttons; status never color-only (text chips) |
| Responsive | tables use card-internal `overflow-x-auto` + inner `min-w` (sessions 560px, records 520px max-h, review queue 440px); page rows `flex-wrap`. Real-browser breakpoint pass not run — see unresolved. |

## What was built (per phase)

- **P3 Quản lý lớp học — buổi học & nhận xét**: class tabs, 5 stat cards
  (sĩ số, chuyên cần, tái tục, điểm TB, lãi/lỗ với `SESSION_COST_VND`
  300.000đ), sessions table (giáo án chip, điểm TB buổi, doanh thu từ
  `enrollment.unit_price`, nhận xét), session detail panel (nhận xét/giáo
  án/điểm tabs), CSV export (BOM + quoted cells).
- **P4 Chương trình & giáo án**: curriculum editor (HvModal width override),
  next-plan card with plan editor, submit flow captures `submittedBy` from the
  auth store; status machine `none→draft→pending→approved/redo` in
  `teaching-store.ts`.
- **P5 Hồ sơ học sinh**: records list (điểm TB, xu hướng với 4-score floor,
  vắng, NGÀY SINH luôn "—"), student detail (score bar chart, per-session
  marks, inline personal notes saved on blur, CSV), list CSV.
- **P6 Duyệt giáo án**: owner-gated (`GET /centers/me` shape) review queue
  across active classes; `useQueries` fan-out sharing attendance cache keys;
  approve / yêu cầu sửa (comment bắt buộc) / mở lại / nhắc; cross-page test
  proves the redo note reaches the teacher's classbook.
- **P7 Verification**: gates above; queue-table refactored to the shared
  overflow-scroll pattern; store isolation per center covered by unit tests.

## Deviations from phase-file text (all resolved toward the prototype or real data)

1. `redo:reopen→pending` added to the status machine (9 legal / 16 illegal
   transitions) — phase-02's original table lacked it but phase-06 + prototype
   `showReopen` require it.
2. Queue GIÁO VIÊN column = `plan.submittedBy`, fallback "—" — Class schema has
   no teacher field.
3. Honest toast copy (e.g. "chưa gửi Zalo tự động") instead of the prototype's
   fake delivery claims; panel subtitle drops the prototype's fake timestamp.
4. Queue draft chip label "Bản nháp" (prototype: "Bản nháp — chưa nộp").
5. P5: trend label "Đi xuống" (prototype) over phase-file "Cần kèm"; list CSV
   drops Phụ huynh/SĐT columns (data not in scope).
6. P4: `currentIndex` stored but display derives next lesson from held-session
   count (`nextLessonIndex` in `classbook-stats.ts`), shared by teacher and
   owner views so both target the same `lessonPlanKey`.

## Post-review fixes (code-reviewer: DONE_WITH_CONCERNS)

Review report: `code-reviewer-260814-0914-prototype-v2-phases-3-7.md`. Fixed
immediately (all cause-aligned with the accepted status machine, re-verified —
337/337 tests, eslint 0 errors, build clean):

- **H1 (high, fixed)**: plan editing was reachable while `pending`/`approved`
  — save coerced the illegal transition back and rebuilt the object, dropping
  `submittedBy`/`ownerComment`. Now: `savePlan`/`attachFile` bail when the
  status machine has no `save` transition, spread `current` to preserve
  review fields, and the next-plan card hides both edit affordances for
  `pending`/`approved` with an explanatory lock note. New test
  "locks editing while the plan is under or after review".
- **L6 (fixed)**: redundant double clamp removed in `lesson-plans-page.tsx`.
- **L7 (fixed)**: `saveCurriculum` index clamp guarded against `-1` so the
  schema can never reject (and drop) the stored center state.

Deferred (need product input or are follow-up scope — see unresolved):
H2 cross-tab storage reconciliation, H3 review-queue fan-out bound,
M1 revenue basis (`billable` vs `present`), M2 score clearing, M3 honest
persistence-failure toast, M4 score/roster reconciliation after attendance
edits, M5 CSV formula-injection prefix, L1–L5 cleanups.

## Docs impact

None. `docs/` files are convention/architecture-level and enumerate no screens
or nav entries; the teaching feature follows the documented
`src/features/<name>/` contract. Product decisions (device-local store, cost
constant) are recorded in the plan.

## Unresolved questions

- Real-browser responsive pass (3 breakpoints) not run in this environment —
  verified by code inspection only; worth a 5-minute manual check.
- LÃI/LỖ per-session cost stays a UI constant (300.000đ) until product decides
  on a center setting (plan open question, user-acknowledged).
- Untracked `test_cases.md` at repo root predates this work (payments edge
  cases) — left untouched, excluded from any commit of this delivery.
