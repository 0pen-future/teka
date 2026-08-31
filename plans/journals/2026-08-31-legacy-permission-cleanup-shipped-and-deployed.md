---
title: Legacy permission cleanup shipped and deployed
date: 2026-08-31
summary: "Phase 8: aliases retired, can_send_reports column dropped, stint-only class reads, deployed to prod with schema v20"
---

# Legacy permission cleanup shipped and deployed

## What happened

Closed out the resource-action RBAC catalog by removing every legacy compatibility surface, then deployed to production.

- Migration 000019 dropped `center_members.can_send_reports`; effective `reports.send` now resolves from role/member permission rows. Its down migration rebuilds the column from the full effective verdict (role grant ∪ member grant − member deny) after review caught it silently stripping role-granted senders on rollback.
- Migration 000020 deleted the three retired alias keys (`data.view_center_wide`, `scores.view_all`, `teaching.view_all`) from both assignment tables. Production held 0 alias rows (parity snapshot verified before and after), so the delete was a no-op on data.
- Classes read path became stint-only: the dead creator arm left `readScoped`, and the readable port split into `GetReadable` / `GetReadableWithRoles` so consumers that discard roles skip the extra query. Sessions keeps the roles variant — its classbook branches on them for session generation.
- Review blocker: the Zalo-mapping write rode the contacts read predicate, so the `contacts.view_all` visibility key would have let any granted member redirect a family's statement DMs. Fixed with a dedicated `scopedMappingWrite` predicate (reports oversight OR active hoc_vu stint) plus tests pinning both mapping writes to 404 for a view-all member.

Deploy: images rebuilt from 602a4cc, compose up with one-shot migrate (exit 0, schema v20 clean), binary provenance in the running container matches HEAD, /readyz and web 200, zero error log lines, denial baseline flat (0×403 in 24h). Pre-migration pg_dump backup kept at teka-backups/teka-prod-pre-phase08-260831-1143.dump.

Full `make test-api` showed 52 failures that were all container-start cgroup timeouts under ~37 parallel testcontainers packages; re-running the 22 affected packages at `-p 4` went 22/22 green, exit 0 — environmental, not code.

## Decision

Soak gate lifted by owner decision (recorded 2026-08-31 09:57); full fail-safe sequence executed in one pass since prod held zero alias rows and each step kept an independent rollback point (pg_dump + down migrations + rebuildable images via GIT_SHA).

## Next steps

- H1: `reports.send` effective-permission SQL algebra is duplicated across ~6 sites (centers, notifications, testutil, seeds) — extract a shared fragment with a parity guard test.
- L2: add a test pinning owner `can_send_reports=false` in ListMembers.
- Flexible-center-rbac phase-04 keeps one open box: secretary/send-reports Playwright e2e flows not yet run against the new mechanism.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
