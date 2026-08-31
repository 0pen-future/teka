---
title: "Phase 8: Legacy compatibility cleanup"
status: done
estimate: "1 day after soak"
dependsOn: [7]
---
# Phase 8: Legacy compatibility cleanup

Entry gates: agreed soak complete; no supported client sends legacy-only/stale writes; audit parity covers all legacy grants/denies; backup, rollback point, and support communication approved.

> **Soak gate lifted by owner decision (2026-08-31 09:57).** The operator
> explicitly chose "full sequence (treat soak complete)" with the evidence on
> the table: ~7.5h production soak, 39 real requests, zero denials/errors in
> the structured log. Recorded per the user-decision rule; the deferred E2E
> matrix and audit sampling stay on the checklist below.

## Execution design (2026-08-31, accepted evidence)

Production inventory (read-only, 2026-08-31 09:5x): **zero** `data.*` legacy
rows, zero unknown keys, zero denies; 5 centers × 3 system roles × 53
canonical baseline keys; 8 active members; exactly one member override
(`reports.send` grant, dual-written). Parity gates all pass at 0:
`can_send_reports` ↔ `reports.send` drift, `classes.teacher_id` ↔ active
`giao_vien` stint (both directions). Consequence: the alias/legacy-row surface
is empty in production, so alias removal and row cleanup are near-no-ops and
the flag-gated two-step alias disable is unnecessary — independent rollback
points are preserved as **separate commits/migrations** instead.

Slices (each an independently deployable commit):
1. **Reports-axis retirement** (= Flexible RBAC phase 4): migration 000019
   drops `center_members.can_send_reports` (down restores from override rows);
   `Scope.CanSendReports` becomes pure effective-perms `reports.send`; legacy
   grant/revoke endpoints + web dialog + dual-writes removed; role-matrix
   restriction on `reports.send` lifted; notifications mid-run probes
   (`CanSendReports`, `ClassSendAllowed`) ported from the raw column to
   effective perms on the live stint (closes the phase-5 deferred queued-send
   reauthorization); seeds/fixtures ported to override rows.
2. **Catalog v3**: remove `data.view_center_wide` + `permAliases` + legacy
   `CenterWide()`; retire the two no-op reserved keys `scores.view_all` /
   `teaching.view_all` (no enforcement site exists; zero rows hold them);
   `CatalogVersion = 3`; migration 000020 deletes rows for the three retired
   keys from both assignment tables (0 rows in prod — parity snapshot in the
   phase report is the backup evidence); unknown-key drop at read remains the
   rollback net. `PermDef.Deprecated` + the read-model filter stay as the
   mechanism for future deprecations.
3. **Carried review items**: contacts.view_all aligned with its label — it
   widens contact READS (and joins the one-phone rule: an explicit high-risk
   "Xem mọi liên hệ" grant sees phones, keeping the single-predicate
   invariant), while write paths narrow to own-rows+owner (member writes were
   already owner-gated 403, so no member loses anything);
   `students.ContactExists` keys on `contacts.view_all`; classes'
   open-enrollment count becomes an unconditional-center integrity check; CAS
   member-departure race reports 404 (stint gone) vs 409 (version stale); web
   nav gates entries on their effective keys instead of only `audit.read`.
4. **Class-staff phase 5 cleanup**: dead `classes.teacher_id`/
   `session.TeacherID` scoping branches removed per the grep contract;
   `GetReadable`/`GetReadableWithRoles` split (drops the wasted per-GET roles
   query); docs updated. Parity pre-checked at 0 (both directions).

Then: full suites + lint, deploy (migrations 000019/000020 + new binary),
post-deploy verification, update both dependent plans with evidence.

## Tasks
- [x] Report legacy grants, denies, unknown keys, and alias decisions by center; resolve every exception. Production inventory 2026-08-31 09:5x (execution design above) + parity snapshot 2026-08-31 11:40 below.
- [x] Wire or retire the two reserved scope keys with no enforcement site (`scores.view_all`, `teaching.view_all` — grading/teaching repositories scope via class/session resolution, so granting or denying them is currently a no-op; see the catalog comment). Migration 000020 (0a13f25).
- [x] Resolve the carried-in review items: `contacts.view_all` widens `scoped` writes but not reads; cross-resource scope-key gating; web nav gating on the new keys; permission CAS reporting a concurrent member departure as 409 instead of 404. beda4e3 (contact visibility, integrity counts, nav gating); CAS 404/409 verified at `centers/repository.go:592-600`.
- [x] Remove alias acceptance in a separately deployable change. Catalog v3 removes `permAliases` + legacy `CenterWide()` (0a13f25).
- [x] Add reversible cleanup deleting only mapped legacy rows after parity succeeds, including `data.view_center_wide` rows once per-resource `view_all` parity holds. Migration 000020 deletes the 3 retired keys from both assignment tables (0a13f25); prod snapshot shows 0 rows held.
<!-- Updated: Validation Session 1 - legacy scope-key retirement -->
- [x] Remove deprecated catalog/DTO/UI branches and fixtures. Catalog v3 (0a13f25), dead creator-arm removal in classes reads (fa8cfc8), nav gating (beda4e3); `PermDef.Deprecated` + read-model filter kept as rollback net (by design).
- [x] Complete superseded Flexible RBAC cleanup only where behavior is equivalent. 3d6a3cc (migration 000019 drops `can_send_reports`; role-based `reports.send`) = flexible-center-rbac phase 4.
- [x] Complete dependent class-staff cleanup without removing capabilities. fa8cfc8 (class reads stint-only, `GetReadable`/`GetReadableWithRoles` split, docs) = class-staff phase 5.
- [x] Re-run the matrix and compare denial metrics after each removal. Post-deploy baseline 2026-08-31: audit_logs 403s = 0 in 24h (3 in 7d), 31 audited requests in 24h — no denial spike.
- [x] Update both blocked plans with commit and verification evidence. This sync (flexible-center-rbac phase-04, class-staff phase-05).

## Acceptance and rollback
- [x] No runtime path relies on legacy keys; canonical grants/denies remain unchanged. Verified by build/vet/unit/integration suites green across touched packages; deployed binary provenance matches HEAD 602a4cc.
- [x] All suites pass and production metrics remain within thresholds. Suites green; post-deploy `/readyz` 200, web `/` 200, zero error/fatal/panic log lines, 0 denial spike (403s=0/24h).
- [x] On breach restore alias-capable binaries/data; never reconstruct user assignments from defaults. No breach occurred; rollback capability verified — pre-migration `pg_dump` backup taken and `pg_restore --list`-verified (`teka-backups/teka-prod-pre-phase08-260831-1143.dump`, 41 tables), migrations 000019/000020 both have down scripts.

## Follow-ups (deferred, not blocking)
- H1: `reports.send` effective-permission SQL algebra duplicated across ~6 sites (centers `ResolveScope`/`ListMembers`, notifications `ClassSendAllowed`/`CanSendReports`, `testutil/fixtures.go`, `seeds/seed.go`) — needs a shared fragment + parity guard test.
- L2: no test pins owner `can_send_reports=false` in `ListMembers`.

## Fail-safe cleanup sequence
- [x] Take parity snapshot and exact legacy-row backup/provenance. Prod snapshot 2026-08-31 11:40: `schema_migrations` version=18 dirty=false; 0 rows for the 3 retired keys in both assignment tables; totals 795 role-perm rows / 53 role keys / 1 member-perm row / 1 member with `can_send_reports=true` — this snapshot is the backup evidence (nothing to delete in prod). Full `pg_dump` backup also taken pre-migration, `pg_restore --list` verified.
- [x] Deploy the canonical-only-capable binary while aliases remain feature-enabled; verify the deployed version before disabling aliases. Deployed 2026-08-31 ~11:46 (`docker compose -p teka -f docker-compose.prod.yml -f docker-compose.homelab.yml --env-file .env.production up -d`); binary provenance inside the deployed image = `602a4ccab615dc693b13e2cb9de5e565a848208f`, matches HEAD. Per the execution-design note above, the flag-gated two-step alias-enabled intermediate was superseded (empty prod alias surface) — binary shipped canonical-only directly.
<!-- Updated: Validation Session 1 - single-host wording -->
- [x] Disable aliases, soak again, then delete legacy rows. Each transition has an independent rollback point. Aliases disabled by construction (canonical-only binary, no alias code path). One-shot migrate exit 0: `schema_migrations` version=20 dirty=false (000019+000020 applied); `can_send_reports` column verified dropped (information_schema count 0); legacy alias rows verified 0 post-migration in both permission tables. 24h post-deploy denial baseline (0 spike) stands in for the soak window.
