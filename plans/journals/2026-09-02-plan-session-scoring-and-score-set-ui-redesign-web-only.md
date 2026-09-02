---
title: "Plan: session scoring and score set UI redesign (web-only)"
date: 2026-09-02
summary: "Deep-mode plan for score entry + score set screens; red-team GO WITH FIXES, all findings patched; no API/DB changes"
---

# Plan: session scoring and score set UI redesign (web-only)

## What happened

- Created `plans/260902-1209-session-scoring-ui-redesign/` (plan.md + 6 phases) from
  `plans/reports/ui-redesign-260902-1029-session-scoring-and-score-sets.md`, in deep mode:
  two researchers (grid UX, set-editor patterns), per-phase scout, red-team, validation.
- Verified web-only scope against `apps/api/internal/features/grading/dto.go` and
  migration `000014_grading`: no `has_scores`/`class_count`/`source_set_id` on the wire,
  so those report items became non-goals with UI substitutes (409 lock memory, drop
  "used in N classes").
- Red-team (`plans/reports/red-team-260902-1209-session-scoring-ui-redesign.md`) returned
  GO WITH FIXES. Notable catches patched into the plan:
  - `useMediaQuery("(min-width…)")` negated would flip every jsdom test to mobile because
    `src/test/setup.ts` stubs `matchMedia` to `matches:false`; use `(max-width: 639px)`.
  - `useDebouncedSave.schedule` replaces the pending payload and the hook flushes on
    unmount; autosave must snapshot the whole dirty set and `discard()` must `cancel()` first.
  - `HvButton.icon` is `ReactNode`, not an icon name.
  - Per-class score-set columns in the class table would be N+1 requests; dropped to non-goal.
  - Research claims corrected: `useDebouncedSave` has no production caller; TanStack
    `mutate` does not queue.
- Task hydration skipped: no `TaskCreate` tool and no `ak task` command in this session.

## Decision

- Order: kit primitives (P1) → consistency pass (P2) → by-student entry + mobile sheet (P3)
  → xl table modal (P4) → set editor (P5) → list/assign (P6). Grid deletion and old
  `parseScoreInput` removal live in P3, not P2.
- Assumptions flagged for product owner: `late` students are scoreable (D2); general score
  empty cell stays ignored (D8); absent students with prior scores show read-only.

## Next steps

- Confirm D2/D8 with product owner before merging Phase 3.
- Run `/ak:cook plans/260902-1209-session-scoring-ui-redesign/plan.md`.
- Open a follow-up API plan for `class_count`, `has_scores`, batch `score-components`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
