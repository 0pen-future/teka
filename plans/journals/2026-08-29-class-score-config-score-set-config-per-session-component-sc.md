---
title: "class-score-config: score-set config + per-session component scores"
date: 2026-08-29
summary: Delivered grading feature (owner score-set config + classbook component scores); fixed a web cache-merge bug and a TOCTOU that could silently cascade-delete grades.
---

# class-score-config: score-set config + per-session component scores

## What happened

Delivered the `class-score-config` feature end to end on branch `teka/260830-0506` (commit `2278b1c`):

- **Schema** — migration `000014_grading` adds the two-tier snapshot model: `score_sets`/`score_set_components` (center templates) copied per-class into `class_score_components` at assign time; `student_scores.component_id → class_score_components ON DELETE CASCADE`.
- **API** (`internal/features/grading/`) — owner-only score-set CRUD, snapshot assign/clear, and per-session component scores. Write gate diverges from `teaching.PutMarks` on purpose: the session's teacher OR the center owner may write (AC4); attribution is traced through the route-based audit log, so all six mutating routes are registered in `audit/action.go`.
- **Web** — owner config page (feature `center`, owner-gated sidebar entry "Cấu hình lớp học") and a classbook component-score grid (feature `teaching`) that replaces the general-score block when a class has components.

## Root causes fixed (code review)

- **H1 (web)** — `use-component-scores.ts` merged the PUT response into the cache treating `score === null` as a delete. But `PUT /sessions/:id/scores` echoes the session's FULL current set (a cleared cell is simply absent, never `null`), so the delete branch never ran and a cleared cell stayed on screen showing its old value. Fixed by wholesale-replacing the cached `scores` on success, mirroring `useSaveMarks`; added a grid assertion that the cleared input reads empty.
- **M2 (API)** — TOCTOU: `guardNoScores` ran outside the transaction, so a concurrent `PutSessionScores` committing the first score in the gap before `ReplaceClassComponents` was silently cascade-deleted. Fixed by moving the guard inside the tx and serializing assign/clear and the score write through a per-class `pg_advisory_xact_lock(hashtext(class_id))` (blocking, mirroring `imports`); `PutSessionScores` now reads components, validates, and writes all inside the locked tx, so the loser of the race gets a clean 422 instead of an FK 500 or a silent cascade. This also subsumed the reviewer's L4 (raw 500 on the reverse race).

## Decision

- Blocking advisory lock, not the TRY variant: per-class contention is rare and the held work is a few small statements, so a brief wait beats forcing the client to retry a 409.
- L3 (`ListComponentsForSets` filters by `set_id` only) documented as a caller invariant rather than adding a `center_id` param — actual exposure is zero (all callers pass center-scoped ids) and a signature change would ripple for no benefit.
- L5 (extra GET per session-panel mount) accepted: React Query caches it and it is how the grid knows whether to render.

## Verification

- `make test-api` green including grading unit + integration; total coverage 75.5% (floor 60%).
- Web: 428 tests pass / 3 skipped, typecheck clean, lint 0 errors.
- Swagger regenerated via `make api-docs`.

## Next steps

- `/ak-deploy`: local Docker deploy with `.env.production` per `docs/deployment.md`.
- Follow-up (non-goal this phase): surface component scores in the parent report / score charts.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
