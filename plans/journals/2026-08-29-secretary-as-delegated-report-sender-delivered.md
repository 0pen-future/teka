---
title: Secretary as delegated report sender delivered
date: 2026-08-29
summary: "5-phase can_send_reports feature done: center-wide read oversight, delegated sends, D8 send exclusivity; e2e exposed a latent EnsurePeriod cross-teacher bug"
---

# Secretary as delegated report sender delivered

## What happened

Completed the full 5-phase plan for delegated report sending
(`plans/260829-1020-secretary-report-sender/`): a boolean membership
permission `can_send_reports` on the live `center_members` stint lets a
non-owner member read billing periods/statements/debt center-wide and run
statement/reminder sends from her own Zalo, while plain members are blocked
from creating sends on every channel with an honest 403 (D8 breaking change).
Owner behavior frozen; `Scope.ReportsOversight() = IsOwner || CanSendReports`
is the single gate.

The phase-5 e2e run exposed a latent production bug: with two teachers
holding periods for the same month, an owner's `EnsurePeriod` duplicate
branch returned an arbitrary teacher's period — `GetPeriodByYearMonth` went
through the owner-widened `scoped()` helper with a bare `Take`. Fixed by
pinning the lookup to the caller's own `teacher_id` (the unique index
guarantees the conflict row is the caller's own); regression test
`TestEnsurePeriodReturnsCallersOwnPeriodWhenMemberSharesTheMonth`.

Second e2e failure taught that statements only generate for CLOSED periods
(`statements/service.go` returns Conflict "period is not closed"), so the
seeded second teacher's period is now closed by the seed (all his sessions
confirmed), while the owner's period stays open for the UI state-machine spec.

## Review triage

code-reviewer returned DONE_WITH_CONCERNS; resolution recorded in
`plans/reports/review-260829-1150-secretary-report-sender-phase5.md`:
- Fixed: lint (unused `ctx`), docs read-cluster accuracy (contacts List +
  notification ledger widen on oversight — intentional, now documented),
  seed close made tolerant of unconfirmed sessions on reused dev DBs,
  `ListPeriodsRead` id tie-breaker (as a scope AFTER `p.Scope` — GORM runs
  scopes at Find time, so a chained Order would hijack the primary sort;
  verified via DryRun).
- Rejected with evidence: calendar-flake concern (existing seed backfill
  already guarantees a confirmed current-month session), test-2 ledger
  coupling (single-worker file-order suite; limitation documented in spec).

## Verification

golangci-lint 0 issues; billing unit + integration and seeds integration
green; web vitest 407 passed, typecheck clean; Playwright 26/26 on the
isolated `teka-e2e` stack, secretary spec green twice on the same DB.

## Next steps

Deploy to the local homelab Docker topology (project `teka`,
`docker-compose.prod.yml` + `docker-compose.homelab.yml`,
`--env-file .env.production`, images rebuilt via `make build` with
`VITE_API_URL=https://teka-api.cauchuyenlaptrinh.com/api/v1`), which also
applies the new membership migration via the one-shot migrate service.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
