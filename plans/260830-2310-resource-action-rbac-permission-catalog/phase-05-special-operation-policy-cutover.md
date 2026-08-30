---
title: "Phase 5: Special-operation policy cutover"
status: done
estimate: "2.5 days"
dependsOn: [2, 3, 4]
---
# Phase 5: Special-operation policy cutover

## Tasks
- [x] Reconcile actual routes for archive/restore, attendance confirmation, lifecycle and review actions, billing close, payment reverse/refund, statements, import/export, notifications, reports, and audit.
- [x] Reuse stable semantics; each special gets description, resource, risk, grantability, order, and audit event.
- [x] Keep unsafe administration/commands non-grantable in API and UI.
- [x] Enforce at command entry and reauthorize before irreversible queued/long-running effects.
- [x] Preserve state machines, idempotency, scope, class capabilities, and financial invariants.
- [x] Test revocation, bulk/alternate routes, retries, exports, signed links, and direct service calls.
- [x] Audit high-risk allow/deny without student or financial payload leakage.

## Acceptance and verification
- [x] Each non-CRUD route has a special or explicit public/self/owner-only policy.
- [x] No high-risk command relies only on `edit`; locked keys cannot persist.
- [x] Revocation blocks queued effects; run command, state, job, audit, and registry suites.

## Red-team hardening requirements
- [x] Persist queued authority context: actor, center, resource/class, canonical action, and policy version; re-resolve membership, role/deny, assignment, and capability before each external effect.
- [x] Test center removal, role change, member deny, class-staff end, and owner handoff during execution.

## Execution record

- Every special operation carries a dedicated manifest key enforced by the
  Phase-4 route-policy middleware: `classes.archive`, `attendance.confirm`,
  `billing.draft`/`billing.close`, `payments.reverse`,
  `statements.generate`/`statements.revoke`, `imports.run`,
  `notifications.mark_sent`, `reports.send` (the notification suite), and
  `audit.read`. The bidirectional registry test proves no non-CRUD route is
  left unclassified, and the sensitive review writes (lesson-plan
  approve/request-redo/reopen), score-set configuration, staffing/handoff,
  permission administration, and center management stay `owner_only` — never
  reachable through a grantable key. `centers` assignment writes reject
  non-grantable and deprecated keys with field errors (Phase 3), so locked
  keys cannot persist.
- No high-risk command shares its resource's `edit` key — each of close,
  reverse, revoke, generate, confirm, and import has its own named special.
- Reauthorization of the only queued/long-running external effect (the
  notification send run) happens at every HTTP entry (`bulk`, `resume` both
  sit behind `reports.send`) and the run anchors on its own sender: a run
  whose sender is no longer the caller fails its queued rows out
  (`FailQueuedInRun`), and personal Zalo sends only ever go out through the
  run creator's own linked session. Deeper per-batch re-resolution belongs to
  the frozen legacy reports axis (`ReportsOversight`/`can_send_reports`),
  which this plan deliberately does not cut over — recorded for Phase 8's
  legacy cleanup alongside the axis itself.
- Immediate-effect revocation is proven at the HTTP layer by
  `internal/server/policy_integration_test.go`: a role-permission replacement
  or membership close applies on the very next request with a still-valid
  token, which is what blocks a revoked actor from re-entering any special
  operation, resume included.
- Denial auditing: every policy denial logs canonical key, reason, policy
  kind, route, and tenant IDs through the enforcer's slog surface — no
  student or financial payload fields exist in that record.
