---
phase: 5
title: "Web: records, review queue & nav dot"
status: completed
priority: P1
effort: "1d"
dependencies: [4]
---

# Phase 5: Web: records, review queue & nav dot

## Overview

Swap the remaining store consumers — `/records`, `/records/:studentId`, `/lesson-plans` (owner review), and the sidebar pending dot — to the API hooks, then retire the localStorage store.

## Requirements

- Functional: records pages read scores/notes via `useClassMarks` (same class+month queries as classbook → React Query cache reuse, no extra endpoints); lesson-plans page uses the review-queue query + plan-action mutations; nav dot count comes from the review-queue query.
- Non-functional: owner gating unchanged (`useCenterContext().isOwner` from `/centers/me`); nav-dot re-render behavior preserved (count-stable select, mirroring today's `usePendingPlanCount` semantics).

## Architecture

- `useReviewQueue()` (owner-only; `enabled: isOwner`) returns queue rows shaped for `ReviewQueueTable`; `usePendingPlanCount` reimplemented as `useReviewQueue` + `select: rows => rows.length` — `Object.is`-stable, same name so `dashboard-layout.tsx` changes only its import source.
- Records pages already fetch sessions + attendance from real APIs; only their `TeachingState` slice inputs switch to `useClassMarks`. `student-stats.ts`/CSV libs unchanged.
- Review actions from the queue reuse Phase 4's `usePlanAction` mutations; approve/request-redo invalidate both the queue and the affected class's `useClassTeaching` cache so a teacher tab on the same client converges.
- **Store retirement**: delete `teaching-store.ts` persistence (localStorage envelope, snapshots, subscribe machinery, `useTeachingStore`); keep the pure exports (`transitionLessonPlanStatus`, key helpers, types) wherever Phase 4 placed them. Remove the store unit tests that only tested persistence; keep/relocate state-machine tests.
- Old localStorage keys (`teka.teaching.*`) are simply orphaned — fresh-start decision; no cleanup code (YAGNI).

## Related Code Files

- Create: `apps/web/src/features/teaching/hooks/use-review-queue.ts`
- Modify: `pages/records-page.tsx`, `pages/student-record-page.tsx`, `pages/lesson-plans-page.tsx`, `components/plan-review-panel.tsx`, `src/layouts/dashboard-layout.tsx`
- Delete (or reduce to pure module): `apps/web/src/features/teaching/lib/teaching-store.ts`
- Modify: `__tests__/records-pages.test.tsx`, `__tests__/lesson-plans-*.test.tsx`, `__tests__/teaching-store.test.ts`, msw fixtures

## Implementation Steps

1. Swap records pages to `useClassMarks`; run records tests.
2. Implement `useReviewQueue` + swap lesson-plans page and `dashboard-layout` dot; run lesson-plans + gate tests (member must not fetch the queue — assert `enabled` gating).
3. Retire the store; fix imports; delete dead tests, keep state-machine coverage.
4. Grep for any residual `useTeachingStore|updateTeachingState|localStorage.*teaching` references — zero expected.

## Success Criteria

- [x] Records list + detail render identical stats (trend, điểm TB, bar chart) from API data; CSV exports unchanged.
- [x] Owner queue: approve/request-redo/reopen round-trip against msw; comment-required rule enforced by UI as today, and server 400 surfaces as toast fallback.
- [x] Member accounts trigger no review-queue request; dot renders only for owners with pending > 0 (existing behavior).
- [x] No `localStorage` teaching persistence remains; full teaching test directory green.

## Risk Assessment

- **Cross-account convergence expectations** — the old same-device loop was instant; now the owner's queue updates on refetch. React Query's defaults (refetch on focus) cover the demo flow; if staleness is noticed, tune `staleTime` on the queue query only — do not add polling preemptively.
- **Hidden consumers** — step 4's grep is the guard against a missed store import (e.g. `index.ts` re-exports).

## Completion notes

- Deviation from the plan's Architecture section: the `/lesson-plans` page table does **not** read `useReviewQueue`. It's derived from per-class curriculum+lesson-plans queries (shared React Query cache with classbook), so it can show all statuses and support reopen. `useReviewQueue`/`usePendingPlanCount` feed only the nav-dot count, owner-gated.
- Nav-dot gating test uses a handler that counts `403`s to prove members never fetch the review-queue endpoint at all.
- `StudentSessionsTable` personal notes save on blur; no debounce wiring was needed there (unlike classbook score/note fields).
- Residual-reference grep for `useTeachingStore|updateTeachingState|localStorage.*teaching` returned zero hits.
