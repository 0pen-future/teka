---
title: "Plan: secretary as delegated report sender"
date: 2026-08-29
summary: Hard-mode plan from accepted brainstorm contract; 15 red-team findings applied; plan.html editorial artifact generated
---

# Plan: secretary as delegated report sender

## What happened

Turned the accepted brainstorm contract (plans/reports/brainstorm-260828-0849-secretary-report-sender.md) into a full implementation plan at plans/260829-1020-secretary-report-sender/ (plan.md + 5 phase files + plan.html), via scout -> plan -> red team -> apply -> consistency sweep.

Red team (Assumption Destroyer, Security Adversary, Failure Mode Analyst) produced 15 evidence-backed findings (5 Critical, 8 High, 2 Medium); user approved "apply all". Highest-impact corrections:

- Shared `scoped()` repo helpers back writes (statement Revoke, billing close/void/adjust, contacts SoftDelete/setZaloMapping) — relaxing them would leak write power. Plan now uses dedicated `scopedRead` helpers with `ReportsOversight()`.
- Removing the cross-teacher 409 removes the only guard against two concurrent personal-send runs on one period — new migration 000012 partial unique index on `notification_runs(billing_period_id) WHERE status='running'` + pre-check.
- `OpenMembership` upsert would resurrect a previously granted flag on rejoin — reset in DO UPDATE + CloseMembership.
- Audit convention is the action registry (no body stored), so grant/revoke became split POST/DELETE routes.
- Client-side pre-send count intersection undercounts past the 100/page cap — replaced with a server-side preview endpoint (3 buckets + max_run_size).

## Decision

- Flag is member-only; owner can never hold `can_send_reports` (user decision 2026-08-29) — owner 409 stays unconditional.
- Denials follow the neutral not-found tenancy convention, not literal 403s.
- Revocation is truthfully non-instant: per-item flag re-check in the run loop.
- Contact phone/mapping exposure to the capability holder accepted as intrinsic to the granted purpose.
- Effort 5.5d -> 7d after applying findings. Validation interview skipped by user.

## Next steps

- Run /ak:cook plans/260829-1020-secretary-report-sender/plan.md (Phase 1 first; 2 and 3 parallel after 1).
- Note: `.claude/scripts/set-active-plan.cjs` referenced by tooling does not exist (MODULE_NOT_FOUND); plan was pinned via `ak plan use` instead.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
