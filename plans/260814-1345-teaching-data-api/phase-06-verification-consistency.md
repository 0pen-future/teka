---
phase: 6
title: "Verification & consistency"
status: completed
priority: P1
effort: "0.5d"
dependencies: [5]
---

# Phase 6: Verification & consistency

## Overview

Whole-delivery gate: full suites on both apps, contract/docs consistency, and a manual no-UI-break pass over the four screens.

## Requirements

- Functional: teacher→owner review loop works across two accounts against a real local stack (docker-compose db + api + web).
- Non-functional: zero regressions in either suite; swagger matches implementation; docs updated only where user-visible behavior changed.

## Implementation Steps

1. API: `go vet ./... && go test ./...` in `apps/api`; migration cycle test on a fresh db.
2. Web: full vitest suite, eslint, tsc — all green, no skipped tests.
3. Live pass (docker-compose stack, seeded data): walk all four screens as teacher and owner; verify — classbook stats (incl. LÃI/LỖ with real `unit_price` revenue), score/note typing latency, plan save→submit→approve and →request-redo→resubmit loops cross-account, records trends/CSV, nav dot appears/clears.
4. Consistency sweep: reread this plan + all phases; confirm implemented DTO/field names match the contract table (fix the plan doc, not just code, if they diverged during implementation); swagger regenerated last.
5. Docs impact check per `documentation-management.md`: teaching data is now server-persisted (user-visible change) — update the smallest owning docs surface (API docs are swagger-owned; check `docs/` navigation for a feature overview page that mentions device-local teaching data).

## Success Criteria

- [x] Both suites + lint + typecheck green in one run each, no flakes. (web 335/335 tests, tsc clean, eslint 0 errors / 4 pre-existing warnings; `go vet ./...`, `go test ./...`, `go test -tags integration ./...` all green)
- [ ] Cross-account review loop demonstrated on the live stack. **Not done** — downgraded per this phase's own risk note; see Completion notes.
- [x] No stale "device-local"/localStorage claims remain in docs or code comments (grep `localStorage`, `device-local` under `apps/web/src/features/teaching` and `docs/`).
- [x] Plan Success Criteria in `plan.md` all checked (cross-device criterion checked with a documented downgrade — see plan.md).

## Risk Assessment

- **Env drift** — live pass depends on the operator-run db (see infra commit `28b52a8` convention); if the stack can't run locally, downgrade step 3 to msw-driven e2e-ish tests and report the gap honestly instead of skipping silently.

## Completion notes

- Live docker-compose pass (step 3) was downgraded to msw-driven coverage, exactly per this phase's own risk note: the local stack is operator-managed and `.env` is absent in this environment. The lesson-plans-page test round-trips teacher submit → owner approve through the shared msw store as the substitute evidence. **Live-stack cross-device demo remains outstanding** — flagged as the plan's one open item.
- Swagger diff regenerated via `make api-docs` came out large (+~4200 lines) because the previously committed spec was stale relative to existing (pre-teaching) endpoints, not solely due to this plan's additions.
- Stale "device-local" comments were cleaned from hooks/components as part of the consistency sweep.
- Residual-reference grep for the old store APIs (`useTeachingStore|updateTeachingState|localStorage.*teaching`) returned zero hits.
- Docs impact assessed per `documentation-management.md`: no evergreen docs surface mentions device-local teaching data, so no docs churn was needed.
