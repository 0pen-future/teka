---
title: "Code review — class-students-center-tabs (phase 4 gate)"
plan: 260901-2035-class-students-center-tabs
branch: teka/260901-2035
reviewed: 2026-09-01
status: DONE_WITH_CONCERNS
---

# Code review — class-students-center-tabs

Pre-merge review of the full working-tree diff (19 modified files + 2 new
components) implementing `plans/260901-2035-class-students-center-tabs/`.

## Verification run

| Gate | Result |
|---|---|
| `npm run typecheck` (apps/web) | clean |
| `npx eslint` (roster, reports, dashboard, layouts, e2e) | 0 errors; 3 pre-existing React-Compiler warnings in untouched code |
| `npm run test` (apps/web) | 68 files, 479 passed, 3 skipped |
| `git diff/status -- apps/api` | empty — zero API diff confirmed |
| `CATALOG_VERSION` | still 3, not in the diff |
| `grep ClassSendPeriodsDialog` (`src/`, `e2e/`) | no remaining reference |
| `grep useReportPeriods / reportsKeys / listReportPeriods` | only `send-reports-page.tsx:43`, calling the no-arg form — no other consumer of the removed `classId` variant existed |
| `grep "Lớp & học sinh"` (`docs/`, READMEs) | 0 hits — no evergreen docs to update |

The three rewritten e2e specs were **not executed** — they need the isolated
stack. Their selectors were reviewed statically instead (see below).

## Overall assessment

No blocking defects. Every acceptance criterion in `plan.md` and in phases 1–3
is met by the diff.

- The guard in `apps/web/src/features/roster/pages/students-page.tsx:62-72`
  matches `center-permissions-page.tsx` exactly (shell component,
  `!isResolved && !isError` → `null`, `!isOwner` → `<Navigate to="/" replace />`),
  so roster queries genuinely never mount. The zero-request test proves this
  rather than merely executing the path.
- The tab strip copies the permission-matrix underline pattern, with the two
  tablist layers distinguished by `aria-label` ("Khu vực" vs "Lớp").
- All five deep-link resolution cases (bare URL, `class_id`, legacy
  `class_id=none`, explicit `tab` winning, unknown `tab`) have real assertions.
- `RosterTable` follows the plan's variant-prop rule rather than splitting into
  two files.

## Findings

### 1. Medium — `ClassesTab` reports a query failure as "you have no classes"

`apps/web/src/features/roster/components/classes-tab.tsx:34` branches on
`classes.length === 0` after checking only `isPending`. `useClassesList` also
exposes `isError`, and on a failed `/classes` request `classes` is `[]` while
`isPending` is false — so the owner sees "Chưa có lớp nào." plus a create-class
button. The sibling `class-overview-cards.tsx:110-118` gets this right, checking
`isError` before the empty case.

Realistic harm: an owner creates a duplicate class during a transient API
failure.

Fix: thread `isError` through the props and render an error line ahead of the
empty state.

### 2. Low — dashboard card hiding ignores `isResolved`

`class-overview-cards.tsx:37` returns `null` on `isNew && !isOwner`, but
`isOwner` is `false` while `/centers/me` is still in flight. `useCenterContext`
documents `isResolved` as the anti-flicker gate for exactly this, and
`dashboard-layout.tsx:134` uses `isResolved && isOwner`.

Result: a sessionless class's card is hidden, then pops into the owner's grid
once the role resolves — a layout shift, not a correctness bug. The chosen
direction is the safe one (a member never briefly sees a card that would bounce
them), so this is a polish call, not a reversal.

### 3. Low — the "Thêm" tab lights up on member routes with no matching entry

`dashboard-layout.tsx:199` widens `OVERFLOW_PATH_PREFIXES` from
`/students/import` to `/students`. Members still reach `/students/:id` from
`/contacts/:id` and from records, and `MoreTab` (`:346`) marks itself active
there even though the sheet holds no `/students` entry for them. Purely
cosmetic. The sidebar's own `useNavActive` "most specific wins" logic is
unaffected — `/students/import` still claims its own highlight.

### 4. Informational — the handoff acceptance now runs through a different endpoint

`apps/web/e2e/class-staff-write.spec.ts:193-197` used to prove `GET /students`
still returns a handed-off teacher's roster; it now proves `GET /enrollments`
does, via `/records`.

The substitution was traced on the API side and is sound:
`apps/api/internal/features/enrollments/repository.go:115` and
`apps/api/internal/features/classes/repository.go:102` both widen reads with
`classscope.ReadExists`, which deliberately includes **ended** stints. So a
handed-off teacher keeps both the class pill (from `GET /classes`) and its
enrollment rows (from `GET /enrollments`) — the same invariant the
handoff-roster-visibility plan protected, reached through a different table.

The `GET /students` path that plan originally guarded keeps its own coverage in
`apps/api/internal/features/students/staff_read_integration_test.go`, so nothing
is left unguarded — only the e2e layer moved.

### 5. Informational — the notifications page's class mode is now dead UI

`apps/web/src/features/collections/pages/notifications-page.tsx:155-175`
(`canSendClassReports`, `useClass(classId)`, `useSendPreview(..., classId)`)
only activates for `/notifications/:periodId?class_id=`. Grepping every `Link`
and `navigate` call in `src/` confirms nothing produces that URL any more.

The code is still coherent and unit-tested, and the plan explicitly accepted
losing the UI path while keeping the API capability. Flagged only so the
eventual cleanup is a deliberate decision rather than a later discovery.

### 6. Low — two test-hygiene leftovers

- `apps/web/src/features/roster/__tests__/class-settings-handoff.test.tsx:50`
  still stubs `/students` as an extra route although the member back-link now
  targets `/records`. Nothing asserts navigation there, so it passes, but
  phase 3 listed it for update.
- The deleted member test took the "a masked `contact_phone` renders no `tel:`
  link" assertion with it, leaving `RosterTable`'s null-phone branch without
  direct coverage. The branch is trivial and the page is owner-only now — not
  worth blocking on.

## E2E selector review (static)

The selectors hold up:

- Playwright runs at the default 1280×720 viewport, so `sm:`-gated tables render
  and `getByRole("row")` works in both `ClassesTab` and `RosterTable`.
- `records-page.tsx` does expose the `role="tab"` class picker and does
  round-trip `class_id` through the URL with `{ replace: true }`, so
  `classIdFromRecordsTab` resolves for both the owner and Thầy Minh.
- `StudentRecordsTable` renders each name in its own leaf `<div>` with no mobile
  duplicate, so `getByText("Bé An", { exact: true })` is unambiguous under
  strict mode.
- **Name collisions:** page tabs and class pills share `role="tab"`, and
  Playwright's `name` option is substring-matching by default — but none of
  "Lớp học" / "Học sinh" / "Chưa ghi danh" is a substring of any seeded or
  generated class name, and vice versa. No collision in these three specs.
- Worth knowing rather than fixing: `roster.spec.ts` relies on `q` surviving tab
  switches (line 77 sets it, line 97 overwrites it on the unenrolled tab). That
  is the documented `setTab` behavior — it works, but it makes those steps
  order-dependent.

## Plan status

Phases 1–3 are complete as written.

Phase 4 is complete **except its first success criterion**: the three rewritten
specs have not been executed. Per the phase's own risk note ("Stack e2e không
sẵn: dừng và báo, không skip âm thầm"), the plan should not be marked done until
`npm run e2e` runs green on the `teka-e2e` stack.

## Recommended actions

1. Fix finding 1 (`classes-tab.tsx` error vs empty state) before merge.
2. Run `npm run e2e` on the isolated stack; do not mark phase 4 done without it.
3. Optional polish: findings 2, 3, 6.
4. Track finding 5 (notifications class-mode dead UI) as a follow-up cleanup
   decision.

## Unresolved questions

- None blocking. The only open item is empirical: the e2e run.
