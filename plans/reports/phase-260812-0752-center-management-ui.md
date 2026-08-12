# Phase Report: Center Management UI

Plan: `plans/260811-1055-manager-class-oversight/` — Phase 5 (final implementation phase).
Mode: TDD (tests written first, red → green), tester + code-reviewer subagents, review fixes applied.

## Outcome

Feature folder `apps/web/src/features/center/` delivering the "Trung tâm" settings page at `/center` (dashboard layout, overflow nav entry), fully role-gated on `center.is_owner` from `GET /centers/me`.

| Viewer | Capabilities |
|---|---|
| Owner (shared center) | Rename dialog, remove members (confirm: data stays with center), read roster |
| Owner (alone, personal center) | Above + join-another-center form (owner phone, VN validation) |
| Member | Read-only roster, "Rời trung tâm" (confirm: created data stays), no owner controls |

## Key decisions

- **Eviction, not invalidation, on tenancy swap** (review H1): `useJoinCenter`/`useLeaveCenter` call `queryClient.removeQueries()`. A filterless `invalidateQueries()` only marks stale and refetches *active* queries — inactive caches (students, classes, receipts of the old center) would stay renderable until/unless their refetch lands. Eviction guarantees no pre-swap row can paint. Side effects: confirm/toast no longer waits on a full refetch wave (was review M1), and tests assert `getQueryState(...) === undefined` instead of `isInvalidated`.
- **`removeMember` is 404-idempotent**: DELETE converging on "already gone" is success; matched on `ApiError.code === "NOT_FOUND"` (interceptor guarantees code presence; proxy HTML 404 degrades to UNKNOWN_ERROR and still propagates).
- **Rename returns full `MeResponse`** → `setQueryData`, no refetch.
- **Join error mapping by code**: NOT_FOUND → "Không tìm thấy chủ trung tâm với số này"; CONFLICT → copy includes retry hint (review M3: backend also emits CONFLICT for a retryable concurrent-membership race); VALIDATION_ERROR *without fields* → self-join message (client pre-validates the only field, and `validation.BindError` always attaches fields — the fields-less 422 on this route is uniquely the self-join rejection). Fields-carrying errors go through `useApiFormErrors` as usual.
- **Join section rendered only for a solo personal-center owner** — every other state can only produce API errors.
- **Owner row never gets a remove control; owner never mounts the leave dialog** — server rejects owner self-removal (422), so the UI doesn't manufacture the error.
- Leave/remove folded into one `RemoveMemberDialog` with `mode: "remove" | "leave"` (copy + hook differ; DELETE identical).

## Review findings & resolution

Reviewer: DONE_WITH_CONCERNS (0 Critical, 2 High, 5 Medium, 6 Low). Tester: DONE (95% stmt coverage page layer).

| Finding | Resolution |
|---|---|
| H1 stale cross-tenant cache after join/leave | Fixed — `removeQueries()`, comment corrected, tests assert eviction |
| H2 4 files fail Prettier CI gate | Fixed — formatted; also fixed pre-existing violation in `dashboard-layout.tsx` (file already in diff) |
| M1 modal open during full refetch wave | Resolved by H1 (`removeQueries` is sync) |
| M2 blanket invalidation triggers `POST /billing-periods` | Accepted — refetch of the active `useCurrentPeriod` after eviction still POSTs (create-or-get, idempotent server-side); inherent to that query's design, not this phase |
| M3 CONFLICT copy wrong for retryable race | Fixed — retry hint added |
| M4 remove test proved refetch, not removal | Fixed — scripted 2nd payload, asserts row disappears |
| M5 missing error-path coverage | Fixed — 3 tests added: page query error, rename server error (dialog stays open), remove failure toast |
| L2 `truncate` on flex container | Fixed |
| L4 leave dialog mounted for owners | Fixed (`!isOwner && self`) |
| L6 root error anchored inside phone field | Fixed — rendered below field |
| L1 unused `index.ts` exports | Rejected — feature public-surface convention (every feature ships `index.ts`); verified vs profile/roster pattern |
| L3 rename dialog reseeds on `currentName` change while open | Accepted — only the owner renames; narrow |
| L5 English fallback copy (network/unknown errors) | Accepted — pre-existing shared `useApiFormErrors` behavior, repo-wide concern |

Reviewer edge-checks that passed: self-detection via `user.id` sound (`teachers.id = user_accounts.id`), post-leave convergence to personal center, nav bottom-bar invariant kept, server-side authz is the real gate (client gating cosmetic).

## Validation

- `npm run lint` 0 errors (4 pre-existing warnings), `npm run typecheck` clean.
- Full suite **227/227** pass (39 files); center feature: 20 tests (17 TDD-first + 3 review-driven).
- Prettier clean on all touched files. Note: `format:check` remains red repo-wide from 5 files untouched by this phase (roster dialogs, layout test) — pre-existing on master, left for a separate cleanup.

## Files

- New: `apps/web/src/features/center/{schemas/center-schemas.ts, api/center-api.ts, hooks/use-center.ts, components/{member-list,rename-center-dialog,join-center-form,remove-member-dialog}.tsx, pages/center-page.tsx, routes.tsx, index.ts, __tests__/{center-handlers.ts,center-schemas.test.ts,center-page.test.tsx}}`
- Modified: `apps/web/src/app/router.tsx` (mount `centerRoutes`), `apps/web/src/layouts/dashboard-layout.tsx` (nav "Trung tâm" + overflow sets, import reformat)

Related: tester report `plans/reports/tester-260812-0740-center-management-ui.md`, review `plans/reports/review-260812-0740-center-management-ui.md` (if written by subagents).

## Unresolved questions

1. Repo-wide Prettier debt (5 pre-existing files) keeps CI `format:check` red — separate cleanup commit?
2. API-side test asserting the self-join 422 carries `fields == nil` (the client's self-join message depends on this shape) — small Go test, candidate for a follow-up.
