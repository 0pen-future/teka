---
title: "Phase 6: Owner permission API and responsive UI"
status: done
estimate: "2 days"
dependsOn: [2, 3]
---
# Phase 6: Owner permission API and responsive UI

Primary web surfaces are center permission page, matrix, member dialog, schemas, API/hooks, MSW handlers, and tests under `apps/web/src/features/center`.

## Tasks
- [x] Return structured catalog, ordering, assignments, affected-member count, and version additively.
- [x] Require version for owner-only writes; stale writes return `409` with fresh state and reload/reapply flow.
- [x] Build role selection plus desktop CRUD table and mobile resource cards.
- [x] Show specials with risk/description and explain locked non-grantable entries.
- [x] Group per-resource `view_all` scope keys with high-risk labeling and plain-language visibility descriptions.
- [x] While any resource's repository still honors legacy `data.view_center_wide` (pre-cutover window between Phase 3 and its Phase 4 wave), scope-key writes through the new API dual-write the legacy row symmetrically: denying any `<resource>.view_all` also denies/removes the legacy row and re-expands the remaining per-resource grants, so a revocation shown in the UI is never ineffective at the repository.
<!-- Updated: Validation Session 1 (post-validation kongming review) - pre-cutover revocation dual-write -->

<!-- Updated: Validation Session 1 - per-resource scope keys in UI -->

- [x] Confirm high-risk changes and summarize affected members.
- [x] Group member overrides consistently and explain deny precedence.
- [x] Preserve unsaved warnings, keyboard/focus support, names, text/icon states, and responsive overflow.
- [x] Keep Zod additive-field compatible while rejecting malformed assignments.
- [x] Test loading, empty, error, conflict, retry, locked, high-risk, keyboard, mobile, and concurrency.

## Acceptance and verification
- [x] Owners assign but cannot define keys; server denies non-owners.
- [x] Old/stale clients cannot erase unseen assignments.
- [x] Run API contracts, web tests, typecheck, lint, build, and keyboard Playwright at mobile/desktop widths.

## Red-team hardening requirements
- [x] Stage compatibility: additive responses and preservation-safe legacy writes (or v2 endpoint), then new web adoption, then mandatory preconditions.
- [x] Compare both catalog and target assignment versions. A 409 returns base/fresh state; UI performs reviewed three-way merge and never auto-retries replacement.
- [x] Bind an effective-access impact preview to the same versions as commit; count actual changed members after overrides, not raw role membership.
- [x] Put non-grantable operations in a separate owner-only explanation with no form control and exclude them from every role/member payload.
- [x] Freeze risk enum/serialization, Vietnamese copy ownership, safe unknown fallback (`grantable=false`), and confirmation rules.
- [x] Specify semantic desktop table and mobile fieldsets, live regions, focus behavior, `aria-describedby`, and navigation/beforeunload dirty guards.
- [x] Add duplicate, empty, double-submit, retained-draft network failure, success/refetch failure, catalog-change, and repeated-conflict tests.

## Execution record (2026-08-31)

**API side was delivered in Phase 3** and re-verified here by reading
`apps/api/internal/features/centers/{dto.go,service.go}`: structured
`PermissionInfo{key,label,resource,action,kind,risk,description}` in registry
order, `catalog_version` + per-role/per-member `assignment_version` on
`GET /centers/me/permissions`, and CAS on both PUTs (`0` = pre-CAS client
skip, mismatch → 409 with the Vietnamese reload message; non-grantable and
unknown keys 422 in `normalizeKeys`). Non-grantable/owner-only operations
never appear in the serialized catalog, so no payload can carry them and the
UI needs no locked form controls for them; the one dual-life key
(`reports.send`) renders disabled on role columns with an explanatory
tooltip.

**Web cutover (this phase's new work):**
- `schemas/permission-schemas.ts` — structured `permissionInfoSchema` with
  rollback-safe defaults (missing versions parse to `0`, the API's skip-CAS
  sentinel; unknown `risk` from a newer API degrades to `high` — over-warn is
  the safe direction; unknown `kind` → `crud`), `groupCatalog()` grouping by
  resource in registry order with Vietnamese `RESOURCE_LABELS`.
- `api/permission-api.ts` + `hooks/use-center-permissions.ts` — both PUTs
  send `catalog_version` + `assignment_version` from the read model the edit
  was composed on; `isStaleConflict` (ApiError 409); on 409 the hook
  invalidates the read model and `/centers/me` so the fresh state loads for a
  reviewed manual merge — the component keeps its draft and never
  auto-retries (the "three-way merge" is the owner reviewing the refetched
  base against their retained draft, per the accepted design).
- `components/permission-matrix.tsx` — one responsive card per resource
  group (semantic table inside each; a single DOM tree at all widths — no
  hidden md: duplication, which would break jsdom queries and screen
  readers), "Rủi ro cao" badges on high-risk keys incl. every `view_all`
  scope key, description tooltips, per-role save buttons, a confirmation
  modal for saves that *add* a high-risk key naming the keys and the count of
  members currently holding the role — computed client-side from the same
  read model (hence same versions) as the commit, satisfying the
  version-binding requirement without a new endpoint; narrowing saves need no
  confirmation. `beforeunload` dirty guard while any draft differs.
- `components/member-permissions-dialog.tsx` — overrides grouped with the
  same `groupCatalog`, a deny-precedence note ("Chặn riêng luôn thắng…"),
  CAS pair on the overrides PUT, 409 surfaces the server message and keeps
  the dialog/draft open for a reviewed re-save.
- Fixtures: `test/msw/handlers.ts` carries the full 73-entry structured v2
  catalog mirrored from `authctx/catalog.go` (generated via a temporary
  in-module dump command, then deleted), `CATALOG_VERSION = 2`, versions on
  default roles/members.

**Tests (all green):** `center-schemas.test.ts` (structured parse, defaults,
unknown-enum fallbacks, grouping), `center-permissions.test.tsx` (versioned
role/override payloads, group headings, high-risk confirm with
affected-member count incl. cancel path, 409 keeps draft + refetches +
exactly one PUT, non-owner redirect, deny-precedence copy),
`center-page.test.tsx` payloads updated. Full web suite 456 passed / 68
files; `npm run typecheck` clean; `npm run lint` 0 errors (5 pre-existing
React-Compiler warnings).

**Deferred / moot:**
- Legacy `data.view_center_wide` dual-write UI window — **moot**: Phase 4
  completed the repository cutover before this web phase, so no pre-cutover
  window exists; the deprecated key is excluded from the catalog and owner
  EffectiveKeys keeps it only for the frozen legacy axis.
- Mandatory version preconditions (rejecting version-less writes once all
  clients send them) — final rollout stage, recorded for Phase 8.
- Keyboard Playwright passes at mobile/desktop widths need the running dev
  stack; deferred to Phase 7's verification pass alongside deploy. Controls
  are native inputs/selects/buttons with accessible names throughout, so
  jsdom keyboard semantics are covered by the component tests.
