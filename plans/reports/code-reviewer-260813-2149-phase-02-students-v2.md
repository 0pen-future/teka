# Code Review — Phase 2: "Lớp & học sinh v2 alignment"

Date: 2026-08-13
Reviewer: code-reviewer
Base: working tree vs `d38f6fc`
Verdict: **DONE_WITH_CONCERNS** — 2 High, 5 Medium findings; no Critical.

## Scope

- `apps/web/src/features/roster/pages/students-page.tsx` (+139/-45)
- `apps/web/src/features/roster/hooks/use-enrollments.ts`
- `apps/web/src/lib/utils/format.ts`, `index.ts`
- `apps/web/src/features/roster/__tests__/roster-handlers.ts`, `students-page.test.tsx`
- `apps/web/src/lib/utils/__tests__/format.test.ts`
- Total 281 insertions / 45 deletions across 7 files, all inside `apps/web`.

Checks run: `npx tsc -b` (clean), `npx eslint src/features/roster src/lib/utils`
(0 errors, 3 pre-existing react-hook-form compiler warnings in
`class-settings-page.tsx`), `npx vitest run src/features/roster src/lib/utils`
(12 files / 80 tests green).

## Acceptance criteria

| # | Criterion | Verdict |
|---|-----------|---------|
| 1 | v2 layout, tokens only | Met for markup/tokens; BUỔI cell semantics diverge from the prototype's `cnt` (see L4) |
| 2 | Pre-existing roster tests pass | Met |
| 3 | One sessions query, no fan-out, nothing on unenrolled tab | Met in code, **not covered by any test** (M4); see H1 for stale data on that tab |
| 4 | No behavior regressions | Met for the flows listed; new transient/stale render states introduced (H1, M2) |
| 5 | UI-only, no changes outside `apps/web` | Met for the *diff*; **not** true of server state (H2) |

---

## High

### H1 — `keepPreviousData` leaks the previous class's enrollments into the new tab

`students-page.tsx:113-125` derives `enrollmentByStudent` from
`useEnrollmentsList`, which sets `placeholderData: keepPreviousData`
(`use-enrollments.ts:23`). Nothing gates the derived map on
`selectedClassId`.

Two consequences:

1. **Class A → class B switch (guaranteed).** Students list and enrollments
   list both keep previous data, but they resolve independently. Between the
   two resolutions the table renders class B's rows against class A's
   enrollment map (or vice versa), so NHẬP HỌC can show a start date from the
   class the teacher just left. For a student enrolled in both classes the
   wrong window is also used for the count.
2. **"Chưa ghi danh" tab (very likely).** The query is disabled there, but a
   disabled query still receives `placeholderData`. The previous class's
   enrollment page is fetched *without* `active`, so it contains **ended**
   rows — exactly the students who now appear on the unenrolled tab. Result:
   a student who left class A shows A's `started_on` under NHẬP HỌC and `0`
   under BUỔI instead of `—` / `—`. (I could not run a probe test under the
   "no edits in apps/web" constraint; verify before/while fixing.)

Fix that is correct either way:

```ts
const enrollmentByStudent = new Map<string, Enrollment>();
if (selectedClassId) {
  for (const enrollment of classEnrollmentsPage?.items ?? []) { … }
}
```

Combined with M1 (`active: true`) the ended-row leak disappears entirely.

### H2 — the new sessions query performs server-side writes on a browse screen

`GET /classes/:id/sessions` is **not** read-only. `sessions.Handler.listRange`
(`apps/api/internal/features/sessions/handler.go:128-162`) calls
`Service.ListRange`, which expands the class's schedules over `[from, to]` and
`BulkInsertIgnoreConflicts` the missing rows
(`apps/api/internal/features/sessions/service.go:79-142`). The service even
carries a `ListRangeReadOnly` sibling documented as "the listing path … whose
GET must never write" (`service.go:158-172`), used by the dashboard.

`students-page.tsx:109-112` requests **1st → last day of the current month**,
so simply clicking through class tabs on the roster screen now materialises
that class's whole month of sessions, *including future-dated ones*. Knock-on
effects outside this screen:

- past-dated generated rows are `planned` + unconfirmed, which is exactly
  `ListPending`'s predicate (`service.go:174-191`) — they appear in the
  dashboard's pending-attendance feed;
- the same predicate backs the billing close gate's unconfirmed-session
  warning (`service.go:193-203`).

So "UI-only, no backend changes" holds for the repo diff but not for
production data. Recommended: use `to = today` (which also matches
`currentMonth()` in `class-settings-page.tsx:43-51`, giving the key-sharing the
comment already claims) so nothing future-dated is generated, and raise the
"roster browse should not generate sessions" question with the API owner if
even past-month generation is unwanted.

Note the existing precedent: `class-settings-page.tsx:78` already calls this
endpoint for 1st→today, so the *kind* of side effect is not new — its reach is.

---

## Medium

### M1 — enrollment page can truncate an active student off the list

`students-page.tsx:113-116` sends `{ class_id, per_page: 100 }` with no
`active` filter. The API defaults to sort `-started_on` and includes ended rows
(`enrollments/handler.go:147`, `repository.go:16-22`). A class with more than
100 lifetime enrollment rows drops the **oldest-started** ones first — i.e. the
long-tenured students who are still active. Those rows silently render `—` /
`—`.

Meanwhile `GET /students?class_id=` filters to *open* enrollments only
(`students/repository.go:112-118`). Passing `active: true` makes the two
queries agree exactly, bounds the page to the visible roster, and makes the
latest-wins dedupe at `students-page.tsx:118-125` unnecessary (a student has at
most one open enrollment per class).

### M2 — BUỔI flashes `0` on every class switch

`useSessionsList` has no `placeholderData` (`use-sessions.ts:43-49`) while
students/enrollments do. During a switch, `monthSessions` is `undefined` →
`countableSessionDates` is `[]` → `monthSessionCount` returns `"0"` for every
enrolled student (`students-page.tsx:126-143`), then flips to the real number.
A teacher reading "0 buổi tháng này" for a whole class, even briefly, is a
misleading state next to billing-adjacent data. Render `—` (or `…`) while the
sessions query is pending:

```ts
const { data: monthSessions, isPending: sessionsPending } = useSessionsList(…);
…
if (!enrollment || sessionsPending) return "—";
```

### M3 — a11y: the sibling-disambiguating note now lives under an "Actions" header

`display_note` moved from a dedicated "Ghi chú" column into the actions cell
(`students-page.tsx:367-376`), whose header is `sr-only` "Hành động"
(`:331-333`). In a real `<table>`, header cells are programmatically associated
with data cells, so a screen-reader user hears the note announced as an
*action*. Since two students can share `full_name` (that is the documented
reason `display_note` exists — `roster-handlers.ts:8-10`), the note is the only
identity disambiguator and it is now mis-associated. WCAG 1.3.1.

The prototype puts badges in the last grid cell, but the prototype is `div`s
with no header semantics; the implementation chose a semantic table, which adds
the constraint. Options: keep the badge in the HỌC SINH cell (mobile card
already does this, `:268-270` — desktop/mobile are now inconsistent), or give
the last column a meaningful accessible name and keep the sr-only label off the
identity data.

### M4 — test gaps on the parts most likely to break

Existing tests are behavioral and readable; the gaps are on the new logic:

- **No test for acceptance criterion 3.** `roster-handlers.ts` serves both
  `/enrollments` and `/classes/:id/sessions`, so a regression that re-enables
  either query on the unenrolled tab passes silently. MSW runs with
  `onUnhandledRequest: "error"` (`src/test/setup.ts:14`) — a spy/counter on
  those handlers would make the criterion enforceable.
- **Latest-enrollment-wins is untested** (`students-page.tsx:118-125`). No
  fixture gives a student two enrollment rows in the same class.
- **`ended_on` is untested.** The inclusive upper bound at `:140` is the one
  half of the window predicate no test exercises.

### M5 — third divergent month-range implementation, and an inaccurate comment

`currentMonth()` already exists at `class-settings-page.tsx:43-51`
(module-private, `to = today`). `students-page.tsx:105-112` re-derives the same
thing with `to = last day of month`. Because the ranges differ, the comment at
`:101-104` — "One query per class, key shared with classbook via
`useSessionsList` → React Query dedupes" — is false today: navigating
students → class settings issues a second, differently-keyed request. Either
align on one range (see H2) and lift the helper into `lib/utils`, or drop the
dedupe claim from the comment.

---

## Low

- **L1** `students-page.tsx:113-114` — when `selectedClassId` is `undefined`
  the key serialises to `{per_page:100}` (React Query's hash drops `undefined`
  via `JSON.stringify`). No collision today; it would silently alias an
  unscoped enrollments list if one is ever added.
- **L2** `students-page.tsx:230-237` — the ⚙ pill is ~37px tall
  (`px-4 py-2`, 13px text) next to 44px tab pills and `HvButton size="sm"`'s
  documented 44px floor (`hv-button.tsx:34`). Add `inline-flex items-center
  min-h-11`.
- **L3** `students-page.tsx:368` — the actions column is ~21% (≈134px at the
  640px min-width) and holds up to two badges plus three buttons on the
  unenrolled tab; expect heavy wrapping and tall rows there.
- **L4** A bare `4` under "BUỔI T8" is ambiguous — the prototype renders
  `"{att} học · {abs} vắng"` (prototype line 1489). The plan's Risk section
  sanctions the divergence (scheduled, not attended), but combined with H2's
  future-dated sessions the number reads as "buổi đã học" and over-reports.
  A `title`/`aria-label` such as "buổi theo lịch trong tháng" would cost
  nothing.
- **L5** `students-page.tsx:372-387` — two consecutive `isUnenrolledTab`
  ternaries; merge into one fragment.
- **L6** `roster-handlers.ts:244-253` (pre-existing) — the mock `class_id`
  filter ignores `ended_on`, while the API requires `ended_on IS NULL`
  (`students/repository.go:117`). Any test of the "left the class" path would
  be testing the wrong contract.
- **L7** `roster-handlers.ts` fixtures mix a fixed `enrollmentActive.started_on
  = "2026-01-05"` with dynamic current-month sessions; correct under a real
  clock, wrong if a future test fakes a date before 2026-01-05.
- **L8** `format.ts:47-53` — `formatDayMonth("2026-1-5") → "5/1"` and
  `"a-b-c" → "c/b"`; acceptable and consistent with `formatSessionDate`'s
  stated degradation contract.

---

## Verified correct (no action)

- **Enrollment-window predicate matches the server.** `students-page.tsx:138-141`
  (`date >= started_on && (!ended_on || date <= ended_on)`) is exactly
  `enrollments.Repository.ActiveOn`'s documented both-ends-inclusive rule
  (`enrollments/repository.go:43-49`), including the departure-day session.
- **Month math is correct at the year boundary.** `new Date(y, 12, 0)` →
  31 Dec of year `y`; `to`'s year/month come from the same `now`, so no
  December/January mismatch (`students-page.tsx:105-112`).
- **ISO string comparison** is safe: every date is zero-padded `YYYY-MM-DD`
  from the server (`sessionSchema.session_date`) or from `dayOfCurrentMonth`.
- **No month-length flakiness** in the fixtures: days 5/8/12/19/26 all exist in
  February, and the query range 1st→last day covers them in every month.
- **Hook contract is backward compatible.** `useEnrollmentsList`'s second
  parameter is optional with a default, `enabled` defaults to `true`; the two
  existing callers (`student-detail-page.tsx:20`,
  `class-settings-page.tsx:76`) are unchanged and typecheck clean.
- **No N+1.** Exactly two additional queries for the page regardless of roster
  size; per-row work is an in-memory filter over ≤ ~30 dates.
- **Timezone.** Local-clock month boundaries can disagree with the teacher's
  configured timezone by a day at month edges, but this matches every existing
  `today()` helper in the app; not a regression.

## Recommended actions (in order)

1. Gate the enrollment map on `selectedClassId` (H1).
2. Decide the sessions range: `to = today` avoids generating future sessions,
   fixes the false dedupe claim, and needs a one-line product confirmation that
   BUỔI means "so far this month" (H2, M5).
3. Add `active: true` to the enrollments query and drop the latest-wins dedupe
   (M1).
4. Render `—` while the sessions query is pending (M2).
5. Move the `display_note` badge back into the HỌC SINH cell, or name the
   actions column (M3).
6. Add the three missing tests: no-fetch on the unenrolled tab, `ended_on`
   upper bound, and (if the dedupe survives step 3) two enrollment rows for one
   student (M4).

## Unresolved questions

1. Is generating a class's sessions as a side effect of *browsing the roster*
   acceptable to the API owner, given it feeds the dashboard pending feed and
   the billing close gate?
2. Should BUỔI count scheduled sessions for the whole month (current) or only
   up to today? This changes the number teachers see mid-month.
3. Does `placeholderData: keepPreviousData` apply to a disabled query in the
   pinned React Query version? Confirm before closing H1's second case.
