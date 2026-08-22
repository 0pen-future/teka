---
phase: 4
title: "Web: teaching API client & classbook swap"
status: completed
priority: P1
effort: "1.5d"
dependencies: [2, 3]
---

# Phase 4: Web: teaching API client & classbook swap

## Overview

Add the teaching API client + React Query hooks that reproduce `TeachingState`-shaped slices, and swap `/classbook` (curriculum, lesson plans, session detail panel) from the local store to the API. UI must not change.

## Requirements

- Functional: classbook screens read/write via API; per-keystroke typing stays instant (optimistic + debounced mutations).
- Non-functional: components and stats libs unchanged; msw handlers for all new endpoints; no new visual elements (no spinners the store-backed UI didn't have — prefer keeping cached data during refetch).

## Architecture

- `features/teaching/api/teaching-api.ts` + `features/teaching/schemas/teaching-schemas.ts` following the roster feature layout (`roster/api/*-api.ts`, zod parse on the envelope).
- Hooks in `features/teaching/hooks/`:
  - `useClassTeaching(classId)` → `{curriculum, lessonPlans}` where `lessonPlans` is `Record<lessonPlanKey, LessonPlan>` with missing → `status: "none"` mapped in the adapter — **the existing `LessonPlan`/`Curriculum` types from `teaching-store.ts` are the contract**; components keep their props.
  - `useClassMarks(classId, month)` → `{sessionNotes, sessionScores, personalNotes}` in the store's record-map shapes, assembled from the batch read. Query keys shared/coordinated with `use-month-sessions.ts` conventions.
  - Mutations: `useSaveCurriculum`, `useSavePlan`, `usePlanAction(submit|approve|requestRedo|reopen)`, `useSaveSessionNote`, `useSaveMarks` — text/score mutations debounced (~500ms) with optimistic cache writes so keystroke latency matches the store; on 409 from plan actions, invalidate and surface the repo's standard toast (`hvToast`) — check how roster mutations report errors and match.
- **Editing model decision**: keep local component state as the typing buffer (most inputs already hold local state or write per keystroke); the mutation syncs behind the debounce. Flush pending debounced writes on blur/unmount so navigation can't drop input.
- `teaching-store.ts`: this phase stops classbook consumers from importing store read/write; the store file itself is retired in Phase 5 (records/lesson-plans still use it until then). Keep `lessonPlanKey`, `personalNoteKey`, `transitionLessonPlanStatus`, types — move them to `lib/plan-status.ts`-adjacent module if the store file would otherwise survive only for them.

## Related Code Files

- Create: `apps/web/src/features/teaching/api/teaching-api.ts`
- Create: `apps/web/src/features/teaching/schemas/teaching-schemas.ts`
- Create: `apps/web/src/features/teaching/hooks/use-class-teaching.ts`, `use-class-marks.ts`, `use-teaching-mutations.ts`
- Modify: `pages/classbook-page.tsx`, `components/session-detail-panel.tsx`, `components/course-view.tsx` (+ any classbook child reading the store)
- Create: `apps/web/src/features/teaching/__tests__/teaching-handlers.ts` (per-feature msw handlers convention, cf. `attendance-handlers.ts`); register in `apps/web/src/test/msw/handlers.ts`
- Modify: `__tests__/classbook-*.test.tsx`, `__tests__/teaching-store.test.ts` (trim to what remains)
- Read (evidence): `apps/web/src/features/roster/api/classes-api.ts`, `apps/web/src/lib/api/*`

## Implementation Steps

1. Read the roster api/schema/hook trio + msw setup; mirror structure and naming.
2. Implement schemas + api functions + queries; msw handlers with in-memory fixture state (so flows round-trip in tests).
3. Swap classbook reads to the hooks; verify rendered output identical against existing tests before touching writes.
4. Swap writes to debounced optimistic mutations; flush-on-blur; run classbook + course test files.
5. Targeted suites: `classbook-page`, `classbook-course`, `classbook-stats`, `session-detail` tests; then eslint + tsc.

## Success Criteria

- [x] Existing classbook tests pass with msw-backed data (assertions unchanged unless they seeded the store directly — those switch to msw fixtures, expectations stay identical).
- [x] Typing a score/note produces no visible lag and at most ~2 requests/second/field (debounce verified in a test with fake timers).
- [x] No component prop/type changes in `components/`; stats libs untouched.
- [x] eslint 0 errors, tsc clean.

## Risk Assessment

- **Optimistic divergence** — server rejects a write (409/validation) after the UI showed it: invalidate + toast; scores/notes have no legal-transition issues so risk is confined to plan actions.
- **Test seeding rewrite** — tests that seeded localStorage must move to msw fixtures; keep a small fixture builder so the diff stays mechanical.

## Completion notes

- msw follows a two-layer convention: central empty-state defaults in `src/test/msw/handlers.ts` (including review-queue returning `ok([])`), plus stateful per-feature handlers in `__tests__/teaching-handlers.ts` with seed/reset/peek exports for tests that need fixture data.
- Classbook swap landed with no component prop/type changes; stats libs untouched as planned.
