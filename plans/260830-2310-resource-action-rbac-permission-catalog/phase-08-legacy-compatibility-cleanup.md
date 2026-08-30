---
title: "Phase 8: Legacy compatibility cleanup"
status: blocked
estimate: "1 day after soak"
dependsOn: [7]
---
# Phase 8: Legacy compatibility cleanup

Entry gates: agreed soak complete; no supported client sends legacy-only/stale writes; audit parity covers all legacy grants/denies; backup, rollback point, and support communication approved.

> **Blocked on soak (assessed 2026-08-31).** Phases 1–7 are done and the
> enforcement binary ships via the deployment step, which starts the soak
> window. Entry gates are unmet by definition until that window passes:
> no production soak has occurred, legacy `data.*` rows and aliases are still
> live-by-design, and the frozen reports axis
> (`ReportsOversight`/`CanSendReports` + queued-send reauthorization) retires
> here. Soak checklist carried in from Phase 7 deferrals: browser E2E matrix
> at mobile/desktop widths, high-risk audit sampling against production data,
> denial-log review against the stop thresholds recorded in phase 7.

## Tasks
- [ ] Report legacy grants, denies, unknown keys, and alias decisions by center; resolve every exception.
- [ ] Wire or retire the two reserved scope keys with no enforcement site (`scores.view_all`, `teaching.view_all` — grading/teaching repositories scope via class/session resolution, so granting or denying them is currently a no-op; see the catalog comment).
- [ ] Resolve the carried-in review items: `contacts.view_all` widens `scoped` writes but not reads (reads stay on the frozen `ReportsOversight` axis — align key semantics with its label when that axis retires); cross-resource scope-key gating (`students.ContactExists` checks the contacts table under `students.view_all`; classes repository checks enrollments under `classes.view_all`); web nav gating on the new keys (`dashboard-layout.tsx` gates only `audit.read`, so a narrowed role sees entries that 403); permission CAS reporting a concurrent member departure as 409 instead of 404 (`centers/repository.go` bump matches `left_at IS NULL`).
- [ ] Remove alias acceptance in a separately deployable change.
- [ ] Add reversible cleanup deleting only mapped legacy rows after parity succeeds, including `data.view_center_wide` rows once per-resource `view_all` parity holds.
<!-- Updated: Validation Session 1 - legacy scope-key retirement -->
- [ ] Remove deprecated catalog/DTO/UI branches and fixtures.
- [ ] Complete superseded Flexible RBAC cleanup only where behavior is equivalent.
- [ ] Complete dependent class-staff cleanup without removing capabilities.
- [ ] Re-run the matrix and compare denial metrics after each removal.
- [ ] Update both blocked plans with commit and verification evidence.

## Acceptance and rollback
- [ ] No runtime path relies on legacy keys; canonical grants/denies remain unchanged.
- [ ] All suites pass and production metrics remain within thresholds.
- [ ] On breach restore alias-capable binaries/data; never reconstruct user assignments from defaults.

## Fail-safe cleanup sequence
- [ ] Take parity snapshot and exact legacy-row backup/provenance.
- [ ] Deploy the canonical-only-capable binary while aliases remain feature-enabled; verify the deployed version before disabling aliases.
<!-- Updated: Validation Session 1 - single-host wording -->
- [ ] Disable aliases, soak again, then delete legacy rows. Each transition has an independent rollback point.
