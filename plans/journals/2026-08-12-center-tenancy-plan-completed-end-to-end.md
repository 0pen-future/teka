---
title: Center tenancy plan completed end-to-end
date: 2026-08-12
summary: "All 5 phases of the center tenancy migration shipped: schema re-key, auth scope, backend sweep, owner dashboard API, center management UI"
---

# Center tenancy plan completed end-to-end

## What happened

Executed the full `plans/260811-1055-manager-class-oversight/` plan (Center Tenancy) in TDD mode across 5 phases, finishing with Phase 5 (commit `ce401cf`): the "Trung tâm" page at `/center` in `apps/web/src/features/center/` — roster with owner badge, rename dialog (owner), remove member with confirm, join-by-owner-phone form (solo personal-center owners only), leave flow for members. 20 feature tests written first (red → green); full suite 227/227, lint + typecheck clean.

## Decision

- **Cache eviction over invalidation on tenancy swap**: `useJoinCenter`/`useLeaveCenter` call `queryClient.removeQueries()` instead of filterless `invalidateQueries()` — invalidate only refetches *active* queries, leaving inactive caches (old center's classes/students) renderable. Tests assert `getQueryState(...) === undefined`.
- **Join error mapping by code**, including the fields-less 422 uniquely identifying self-join (backend `validation.BindError` always attaches fields), and a retry hint in the CONFLICT copy since the same code covers a retryable membership race.
- `DELETE /centers/me/members/:id` treated as 404-idempotent client-side.
- Docs: rewrote the stale Tenancy section in `docs/api-guidelines.md` (was still teacher-is-tenant) to the center model — `authctx.Scope` resolved per request by `middleware.ResolveScope`, center-scoped `scoped()` with teacher filter for non-owners (commit `dc07ad6`).

## Next steps

- Repo-wide Prettier cleanup: 5 pre-existing files keep `format:check` red on master (roster dialogs, dashboard-layout test).
- Small Go test asserting the self-join 422 carries `fields == nil` — the web client's self-join message depends on that shape.
- Owner dashboard UI (Phase 4 shipped API only) is a separate future plan.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
