---
phase: 4
title: "Cleanup"
status: deployed (e2e follow-up pending)
priority: P2
effort: "0.5d"
dependencies: [3]
---

# Phase 4: Cleanup

## Overview

Remove the legacy `can_send_reports` column and send-reports endpoints once
phase 3 has soaked in production (deployed, owner has used the new UI, no
regressions). Column drop is irreversible in the append-only migration chain —
verify before dropping.

## Requirements

- Functional: `reports.send` override rows (plus role sets, once unlocked)
  are the sole source for `ReportsOversight()`; legacy endpoints and dialog
  removed; the phase-2 dual-life restriction is LIFTED — `reports.send`
  becomes assignable on the role matrix like any other key.
- Contract note: the JSON field `can_send_reports` in `/centers/me` responses
  (centers/dto.go, required by web `center-schemas.ts`) SURVIVES — computed
  from effective perms. Only the DB column and legacy endpoints die.
- Non-functional: verify parity BEFORE the drop:
  `SELECT count(*) FROM center_members cm WHERE cm.left_at IS NULL AND
  cm.can_send_reports <> EXISTS(SELECT 1 FROM center_member_permissions p
  WHERE p.teacher_id = cm.teacher_id AND p.center_id = cm.center_id AND
  p.permission_key = 'reports.send' AND p.allowed)` must be 0 in prod.

## Related Code Files

- Create: `apps/api/migrations/000014_drop_can_send_reports.up.sql` + `.down.sql`
  (down recreates the column and backfills from override rows)
- Modify: `apps/api/internal/features/centers/{routes,handler,service,repository}.go`
  (remove grant/revoke endpoints + dual-write; lift role-matrix restriction),
  `ResolveScope` query (drop `cm.can_send_reports` read; `CanSendReports`
  becomes pure `Has(reports.send)` — the phase-1 dual-life OR-read ends here),
  `apps/api/internal/features/notifications/repository.go:328` +
  `run_manager.go:343` — the MID-RUN permission probe SELECTs the raw column
  per send item; port it to compute effective perms on the LIVE stint
  (override rows are deleted on close, matching today's defence-in-depth),
  with tests — this is a behavior-bearing port, not a mechanical one,
  `apps/api/seeds/seed.go:263` (`scopeFor` reads the raw column),
  `internal/testutil/fixtures.go` (`ScopeFor`/`GrantSendReports` move to
  override rows), e2e seeds that set send-reports state
- Delete: `apps/web/src/features/center/components/send-reports-permission-dialog.tsx`
  + its tests/handlers; legacy API client functions

## Implementation Steps

1. Run the parity query against prod (read-only) — proceed only at 0 drift.
2. Migration 000014 drop column; down restores from override rows.
3. Remove dual-write, legacy endpoints, swagger regen (`make api-docs`).
4. Port notifications mid-run probe + seeds/fixtures to override rows.
5. Remove web dialog + client functions (specs already migrated in phase 3);
   clean legacy seed state.
6. `make test-api` + web vitest + isolated e2e stack green.

## Success Criteria

- [x] Parity check = 0 before drop (recorded in delivery notes) — achievable
      because the phase-2 dual-life rule kept every `reports.send` mutation
      mirrored into the column. Prod inventory 2026-08-31: `can_send_reports`
      ↔ `reports.send` drift = 0 (resource-action-rbac phase-08 execution
      design + parity snapshot 11:40).
- [x] No DB-column references to `can_send_reports` outside migration history
      (the computed JSON contract field keeps the name). Migration 000019 +
      code removal, commit 3d6a3cc; deployed 2026-08-31 ~11:46, column drop
      verified (`information_schema` count 0).
- [x] Role matrix accepts `reports.send` (restriction lifted, test flipped).
      Commit 3d6a3cc, deployed.
- [ ] Secretary/send-reports e2e flows pass against the new mechanism only.
      Not run this pass (Go unit/integration green; isolated e2e Playwright
      stack not exercised for this change).

## Follow-ups (deferred, not blocking)
- Run the isolated e2e (`teka-e2e`) secretary/send-reports Playwright specs
  against the deployed mechanism; no assertion evidence yet either way.

## Risk Assessment

- **Premature drop** → hard gate on soak + parity query; this phase must not
  ship in the same release as phase 3.
- **e2e seed drift** → the isolated e2e stack seeds send-reports state; update
  seed + specs together (see `plans/260829-1020-secretary-report-sender/`).
