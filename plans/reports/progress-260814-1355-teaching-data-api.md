# Progress report: Teaching data API

Plan: `plans/260814-1345-teaching-data-api/plan.md` — status: completed (all 6 phases).

## Status per phase

| # | Phase | Status | Evidence |
|---|-------|--------|----------|
| 1 | DB migration — teaching tables | Done | `migrate up/down` clean, FK guard covered, `go test ./migrations/...` + integration test green |
| 2 | API: curriculum & lesson plans | Done | full 5x5 state-machine table-driven test, owner/member 403s, review queue tests, swagger updated |
| 3 | API: session notes & marks | Done | month read, upsert merge rules, roster check, owner read-only — all covered, tests green |
| 4 | Web: teaching API client & classbook swap | Done | classbook tests pass on msw data, debounce verified w/ fake timers, no component contract changes, eslint/tsc clean |
| 5 | Web: records, review queue & nav dot | Done | records/lesson-plans/nav-dot swapped, store retired, residual-reference grep clean |
| 6 | Verification & consistency | Done, with one downgraded item | full suites green; live-stack cross-device demo downgraded to msw per phase's own risk note |

## Verification evidence

- Web: 335/335 tests, tsc clean, eslint 0 errors (4 pre-existing warnings, unrelated to this plan).
- API: `go vet ./...`, `go test ./...`, `go test -tags integration ./...` all green, including migration up/down round-trip.
- Swagger regenerated via `make api-docs` (diff large, ~+4200 lines, because the previously committed spec was stale — not solely from this plan's endpoints).
- Residual-reference grep for old store APIs (`useTeachingStore|updateTeachingState|localStorage.*teaching`) — zero hits.
- Docs impact assessed: no evergreen docs surface mentions device-local teaching data, so no docs churn required.

## Deviations from plan.md (all recorded in plan.md and the relevant phase file)

1. `idx_session_marks_session` **not implemented** — redundant with `UNIQUE (session_id, student_id)`, which already backs a session_id-leading index (Phase 1).
2. `migrations_test.go`'s `TestCenterTenancyBackfill` now uses `m.Migrate(6)` instead of `MigrateDown(m, 2)` so later migrations don't shift the rollback target (Phase 1).
3. Request-redo with a blank comment returns **422** (validation error), not 400 as originally specified — matches repo convention for body validation (Phase 2).
4. Reopen clears **both** `redo_note` and `owner_comment` server-side (Phase 2).
5. Owner self-approve is allowed — no self-approve guard, per the plan's validated Q&A decision (Phase 2).
6. msw two-layer convention: central empty-state defaults in `src/test/msw/handlers.ts` (incl. review-queue `ok([])`) + stateful per-feature `__tests__/teaching-handlers.ts` with seed/reset/peek exports (Phase 4).
7. `/lesson-plans` page does **not** consume `useReviewQueue`. It reads per-class curriculum+lesson-plans queries (shared cache with classbook) to show all statuses and support reopen. `useReviewQueue`/`usePendingPlanCount` feed only the owner-gated nav-dot count (Phase 5, also reflected in plan.md's endpoint table and Web-swap section).
8. Nav-dot gating test proves members never fetch the review-queue endpoint via a 403-counting handler (Phase 5).
9. `StudentSessionsTable` personal notes save on blur — no debounce wiring needed there (Phase 5).
10. Swagger diff size explained by stale committed spec, not scope creep (Phase 6).

## Outstanding item

- **Live-stack cross-device demo not performed.** The local docker-compose stack is operator-managed and `.env` is absent in this environment, so Phase 6 step 3 (walk all four screens as teacher and owner against a live stack) was downgraded to msw-driven coverage — explicitly permitted by the phase's own risk note. The msw-backed lesson-plans-page test round-trips teacher submit → owner approve through the shared msw store as substitute evidence, but a true live-stack, two-account demonstration has not happened. This is the plan's one remaining open item; the plan's cross-device success criterion is checked in plan.md with a parenthetical noting this downgrade.

## Unresolved questions

None — the plan's own Open Questions section is empty, and the live-stack gap is a known, explicitly-permitted downgrade rather than an open question.

## Post-review fix pass (2026-08-14)

Code review returned no blockers but 2 High / 4 Medium / 6 Low. Fixed in the same
session: H1 (DTO size bounds via binding tags + service-side bounds for the
tri-state marks batch), H2 (success toast/draft reset now wait for `onSuccess`
in the session detail panel and student record page — a failed save keeps the
draft), M1 (concurrent first-save race now 409 instead of 500), M2 (dead
`useReviewQueue` export deleted — success criterion 5 fully met), M3
(`cancelQueries` before optimistic cache writes), and M4 (user decision
"always-correctable": the marks roster gate now only guards new-row creation,
so an existing row stays editable/clearable after unenrollment — covered by
`TestMarksExistingRowSurvivesUnenrollment`; swagger regenerated). Deferred: the
Low items. Full detail + re-verification results in
`code-review-260814-1355-teaching-data-api.md` (Resolution section).
