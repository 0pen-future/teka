---
title: "Code review — Prototype v2 teaching screens, phases 3–7"
date: 2026-08-14
reviewer: code-reviewer
scope: uncommitted working tree since d38f6fc (apps/web)
verdict: DONE_WITH_CONCERNS
---

# Code review — Prototype v2 teaching screens (phases 3–7)

## Scope

- Modified: 16 source files (~1230 lines changed), 30 new files under `apps/web/src/features/teaching/`.
- Touchpoints outside teaching: `features/roster` (students-page, class-settings-page, use-enrollments, index, msw handlers, new `lib/current-month.ts`), `features/attendance/index.ts`, `lib/utils/format.ts` + `index.ts`.
- Constraint check: no `apps/api` changes, no new endpoints, no schema changes. Confirmed — the only `apps/api` access was read-only inspection of `attendance/service.go` to verify the `billable` contract.

## Gate results

| Check | Result |
|-------|--------|
| `npx tsc --noEmit` | clean (exit 0) |
| `npx eslint src` | clean (exit 0) |
| `npm run build` | clean, 836ms; pages code-split (`classbook-page` 25.5 kB, `lesson-plans-page` 8.2 kB) |
| `npx vitest run --config vitest.config.ts src/features/teaching src/features/roster src/lib/utils` | 21 files / 149 tests passed |
| Design tokens | no raw hex/rgb in `features/teaching` or `features/roster` |

Harness note: running `npx vitest run` from the repo root (or without `--config vitest.config.ts`) picks up the wrong config, loses the jsdom environment, and produces 19 spurious "localStorage is not defined" failures with a *zero* exit code when piped. Use `npm test` from `apps/web`.

## Acceptance criteria

| Criterion | Status |
|-----------|--------|
| `/classbook` classbook: stats, sessions table, detail panel | Met |
| `/classbook` course view: curriculum, next-plan card, headcount chart | Met |
| `/records` + `/records/:studentId` | Met; NGÀY SINH "—" as decided |
| `/lesson-plans` owner-only review queue | Met; gate covered by `lesson-plans-gate.test.tsx` including the query-error path |
| Review loop closes (submit → approve/redo → teacher sees note) | Met for the happy path, but **integrity gap** — see H1 |
| Public contracts | No breaks. `useEnrollmentsList` gained an optional second arg (all 6 callers verified compatible); `attendance/index.ts`, `roster/index.ts`, `lib/utils/index.ts` are additive only |

Query-key sharing claim verified: `useMonthSessions`, students-page and class-settings-page all build the key from the same `currentMonth()` helper, so React Query dedupes rather than refetching.

---

## High

### H1. `savePlan` coerces plan status and drops `submittedBy` / `ownerComment`

`apps/web/src/features/teaching/components/course-view.tsx:74-90`

```ts
status:
  transitionLessonPlanStatus(current?.status ?? "none", "save") ?? current?.status ?? "draft",
```

`transitionLessonPlanStatus` is documented at `lib/teaching-store.ts:93` as "Next status for a legal move, null for an illegal one — **callers must not coerce**". This call coerces exactly that null back to the current status, and the returned object omits `submittedBy` and `ownerComment` entirely.

The edit button (`components/next-plan-card.tsx:53-59`) renders unconditionally, so both illegal-save paths are reachable from the UI:

- **Editing a `pending` plan**: content is replaced, status stays `pending`, `submittedBy` is erased. The owner's queue then shows teacher "—" and can approve a giáo án whose body changed after it was displayed.
- **Editing an `approved` plan**: content is replaced, status stays `approved` ("Đã duyệt"), `ownerComment` is erased. The teacher can rewrite an approved lesson plan with no re-review — which is the one thing the review loop exists to prevent.

`attachFile` (`course-view.tsx:92-98`) has the same gap for the attachment: `{ ...current, fileName }` on an approved plan keeps `approved`.

Suggested fix: treat a null transition as "not allowed" — either hide/disable the edit affordance for `pending`/`approved`, or route the save through an explicit re-review (`approved` + save → `draft`/`pending`). Preserve `submittedBy` and `ownerComment` by spreading `current` instead of rebuilding the object literal.

### H2. Teaching store has no cross-tab reconciliation, so concurrent tabs clobber each other

`apps/web/src/features/teaching/lib/teaching-store.ts:153-167`

`updateTeachingState` serialises the **whole** center state and writes it, and nothing anywhere subscribes to `window`'s `storage` event (grep-verified: no listener in `src/`). Two tabs on the same device therefore hold independent in-memory snapshots, and whichever writes last silently discards everything the other wrote since it loaded.

This is not a hypothetical: `hooks/use-center-context.ts:16-24` states the center-name key exists specifically so "the lesson-plan review loop (teacher submits → owner approves) works across accounts on the same device" — i.e. two sessions on one device is the designed use case. An owner approving in one tab wipes scores and nhận xét the teacher entered in another.

Suggested fix: add a `storage`-event listener that drops the affected center's cached snapshot and notifies listeners, or merge per-key rather than replacing the whole state on write.

### H3. `/lesson-plans` fans out one sessions request per active class, and that GET has write side effects

`apps/web/src/features/teaching/pages/lesson-plans-page.tsx:34-44`

`useClassesList({ status: "active", per_page: 100 })` feeds `useQueries`, issuing up to 100 concurrent `GET /classes/:id/sessions` on mount. Two problems:

1. Unbounded fan-out on the owner's landing screen — the plan's own decision #3 chose a single-query design on the students page specifically to avoid this.
2. Per `lib/current-month.ts:5-9`, that endpoint **materialises** session rows for the requested range. Opening the review queue therefore writes rows for every active class at once, which is a side effect the page does not need — it only wants a held-session count.

Mitigating factor: the range is month-start → today, so the rows would be created anyway on first visit to each class. Still worth bounding (only fetch for classes that actually have a curriculum, or cap concurrency).

---

## Medium

### M1. Revenue is derived from `status`, not the API's `billable` flag

`apps/web/src/features/teaching/lib/classbook-stats.ts:43-50` filters `row.status === "present"`. The attendance row carries an authoritative `billable: boolean` (`features/attendance/schemas/attendance-schemas.ts:38`), which the backend populates from the persisted record (`apps/api/internal/features/attendance/service.go:244-250`) independently of status, and which billing counts (`repository.go:28 BillableCount`).

DOANH THU / LÃI/LỖ is presented as the number for the weekly meeting, so any present-but-not-billable row makes the classbook disagree with the invoice. Use `row.billable` for the revenue side; keep `status === "present"` for CHUYÊN CẦN.

### M2. A score cannot be cleared once saved

`components/session-detail-panel.tsx:107-126` skips any draft entry where `parseScoreInput` returns null, and `parseScoreInput("")` is null (`classbook-stats.ts:244-250`). Emptying the input and pressing save leaves the old score in the store and toasts "Đã lưu điểm 0 học sinh". A mistyped score can be overwritten but never removed.

### M3. Save toasts fire even when persistence failed

`teaching-store.ts:159-163` swallows the `localStorage.setItem` failure (quota, Safari private mode) and returns `void`, so every caller toasts success and the note panel shows "Đã lưu ✓". The data is gone on reload. The in-memory-only fallback is a deliberate trade-off, but the success copy should reflect it — have `updateTeachingState` return a boolean and downgrade the toast when persistence failed.

### M4. Stale scores survive an attendance correction

Scores are keyed `sessionId → studentId` and are never reconciled against the roster. If a student is scored and the session is later re-confirmed with that student absent, `classbook-stats.ts:98` (`meanScore(Object.values(scores))`) still includes the score in the session and class averages. The student-facing view does null it out (`student-stats.ts:76`), so the two screens disagree.

### M5. CSV formula injection in exports

`lib/csv.ts:9-16` quotes and escapes `"` but does not neutralise a leading `=`, `+`, `-` or `@`. Exports include teacher-authored nhận xét, personal notes, class and student names (`classbook-page.tsx:132-177`, `records-page.tsx:77-97`, `student-record-page.tsx:77-101`). Excel evaluates `"=..."` after unquoting, and these files are explicitly produced to be shared with owners and parents. Prefix such cells with `'` or a leading space.

---

## Low

- **L1** `csv.ts:19-26` revokes the object URL synchronously after `click()` on an anchor never added to the DOM. Fragile outside Chromium; the jsdom "Not implemented: navigation to another Document" warnings confirm no test covers the real download path.
- **L2** `lesson-plans-page.tsx:112-114` — `reopen()` shows no toast, while approve and requestRedo both do. Silent success reads as a dead button.
- **L3** `plan-review-panel.tsx:93` — button label "Nhắc giáo viên nộp qua Zalo" still promises Zalo delivery, while the toast correctly admits nothing was sent. The honesty fix landed on the toast only.
- **L4** `currentMonth` lives in `features/roster/lib/current-month.ts` but is consumed by `features/teaching` (`use-month-sessions.ts:10`, `lesson-plans-page.tsx:7`). It is a generic date helper with no roster semantics; `lib/utils` is the established home (that is where `formatDayMonth` went in this same change).
- **L5** All store writes no-op while `centerId` is null (before `/centers/me` resolves), with no user feedback — e.g. `session-detail-panel.tsx:95-98` returns early while the "Lưu nhận xét" button still renders in its active style.
- **L6** `lesson-plans-page.tsx:51` — `nextLessonIndex(total, Math.min(doneCount, total))`; `nextLessonIndex` already clamps (`classbook-stats.ts:70-72`).
- **L7** `course-view.tsx:124-127` — `Math.min(currentIndex, lessons.length - 1)` yields `-1` for an empty lesson list, which fails the `nonnegative()` schema and would make `loadState` discard the **entire** center's stored state on next load. Unreachable today because `curriculum-editor-modal.tsx:25` enforces ≥4 lessons; worth a `Math.max(0, …)` so the invariant does not depend on a distant caller.

## Edge cases checked and found sound

- Stale/mistyped `class_id` in the URL: gated to `undefined` before any per-class query on all three screens.
- `keepPreviousData` bleed between classes: both classbook and students-page gate the derived map on the driving id with an explicit comment.
- Mid-month joiners: roster-row absence keeps them out of averages rather than counting as absent.
- Owner gate: unresolved renders nothing, query error degrades to redirect (not a permanent blank), non-owner redirects. All three covered by tests.
- Tests are behavioural (real user interactions, msw-backed), not phantom coverage.

## Latent, not currently reachable

`excused` is parsed but never offered in the UI (`attendance-schemas.ts:27-30`). When it ships, the two screens will disagree: `classbook-stats.ts:83` counts an excused row against class attendance, while `student-stats.ts:71` (`absent = status === "absent"`) counts it as attended for the student's CHUYÊN CẦN. Worth aligning now while both formulas are fresh.

## Recommended actions

1. Fix H1 — close the plan-edit bypass and stop dropping `submittedBy`/`ownerComment`. This is the review loop's core invariant.
2. Fix H2 — add `storage`-event reconciliation; the cross-tab loop is the store's stated purpose.
3. Bound H3 — the review queue should not fan out (and materialise sessions) across every active class.
4. M1 — switch the revenue basis to `billable`.
5. M2/M3/M4 — score clearing, honest persistence-failure copy, roster/score reconciliation.
6. M5 — sanitise CSV formula-leading cells.
7. L1–L7 as cleanup.

## Unresolved questions

1. H1: should saving an already-`approved` giáo án reset it to `draft`/`pending` (re-review) or should editing simply be blocked at that status? This is a product call, not a mechanical fix.
2. M1: is a present-but-not-billable row actually producible today (trial session, makeup)? If the backend never emits one, this is documentation-only; if it does, the classbook is already wrong.
3. H2: is same-device multi-tab (teacher tab + owner tab) an expected workflow, or is sequential login the assumed usage? The fix's shape depends on the answer.
4. `SESSION_COST_VND` remains the plan's open question — still a UI constant, still surfaced in the table footnote as agreed.
