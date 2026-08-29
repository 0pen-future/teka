---
phase: 5
title: "E2E, regression & docs"
status: done
priority: P2
effort: "1d"
dependencies: [3, 4]
---

# Phase 5: E2E, regression & docs

## Overview

Prove the full secretary journey end-to-end, lock owner/member regressions,
and record the permission in the docs that own tenancy/auth behavior.

## Requirements

- Functional: Playwright spec covers grant → secretary login → center-wide
  period visibility → send (manual channel) → audit attribution to secretary.
- Non-functional: existing e2e suite (statement, collections, audit) passes
  unchanged; docs updated only in the owning surfaces.

## Architecture

- **Seed:** extend `apps/api/seeds/seed.go` with a second member ("secretary")
  AND a second teaching member owning their own class, contacts, and a
  billing period — today every seeded row hangs off the owner's scope
  (`seed.go:137-162`), so without cross-teacher data the spec has nothing
  delegated to send. The second teacher's period is seeded CLOSED (statement
  generation refuses an open period — `statements/service.go` returns
  Conflict "period is not closed" — so a send needs a closed period), which
  required his sessions seeded fully confirmed. The OWNER's period stays
  unseeded: `billing.spec.ts:64-75` depends on opening and closing it through
  the UI state machine. The grant happens through the UI inside the spec
  (exercises Phase 3) rather than seeding the flag, keeping the seed neutral
  for other specs.
- **Bug found by this phase's e2e run:** with two teachers holding periods
  for the same month, an owner's `EnsurePeriod` duplicate branch returned an
  arbitrary teacher's period — `GetPeriodByYearMonth` went through the
  owner-widened `scoped()` helper with a bare `Take`. Fixed by pinning the
  lookup to the caller's own `teacher_id` (the (teacher, year, month) unique
  index guarantees the conflict row is the caller's own); regression covered
  by `TestEnsurePeriodReturnsCallersOwnPeriodWhenMemberSharesTheMonth` in
  `billing/integration_test.go`.
- **Idempotency on a reused DB:** the e2e stack reuses its database between
  runs, so the spec must assert-then-set the toggle (grant only if not already
  granted) and revoke in `afterEach`, leaving the flag false for the next run
  and for unrelated specs.
- **E2E channel choice:** `zalo_manual` only — real Zalo sessions don't exist
  in the e2e stack; `zalo_personal` cross-teacher delivery is already covered
  by Phase 2 integration tests with `fakeZaloSender`
  (`notifications/auth_integration_test.go` pattern). The spec asserts the
  copy-paste bundle renders and notification rows attribute the secretary.
- **Audit check:** owner logs in, opens audit page (pattern:
  `apps/web/e2e/audit.spec.ts`), sees the bulk-send action with the
  secretary as actor.
- **Isolated stack:** run per repo convention — `docker compose -p teka-e2e`
  with port+URL overrides; statement specs need a fresh seed (recorded
  operational note).
- **Plain-teacher negative path (D8):** the seeded second teacher doubles as
  the plain-member probe — log in as them, assert their own billing review /
  collections view shows no "Nhắc nợ" or send link, and a direct visit to
  `/notifications/:periodId` for their own period renders the ledger with no
  send controls. (API-level 403s are covered in Phase 2 integration tests;
  e2e only proves the UI removal.)
- **Docs:** update `docs/api-guidelines.md` tenancy/auth section with the
  `can_send_reports` capability, the `ReportsOversight()` read cluster, AND
  the D8 send-exclusivity rule (only owner + capability holder create sends;
  teachers provide attendance/nhận xét input only); add a short paragraph to
  the architecture doc only if it already describes the owner-oversight
  model. Write the release note for the behavior removal (teachers lose
  self-send until the owner grants the flag) in the repo's existing release
  notes surface if one exists — otherwise a paragraph in the same docs
  update. No new doc files.

## Related Code Files

- Modify: `apps/api/seeds/seed.go`
- Create: `apps/web/e2e/secretary-send.spec.ts`
- Modify: `docs/api-guidelines.md` (+ architecture doc only if it owns the
  oversight model today)

## Implementation Steps

1. Seed secretary member + second teaching member with class/contacts/open
   period; verify `make seed` output and that existing specs still pass.
2. Write `secretary-send.spec.ts`: owner grants via UI (assert-then-set) →
   secretary login → nav entry visible → open the second teacher's period →
   manual send → message cards render; owner audit page shows the grant action
   and the bulk-send with secretary as actor; `afterEach` revokes the flag.
3. Negative e2e assertions in the same spec: secretary has no owner-only nav
   (Duyệt giáo án, Nhập từ Excel, Nhật ký hoạt động) and gets no roster edit
   affordances; the plain second teacher sees no send entry points and a
   send-control-free ledger on their own period (D8).
4. Full verification ladder: `make test-api-unit` → `make test-api` → web
   vitest → e2e suite on the isolated stack (fresh seed).
5. Docs edits + link check.

## Todo

- [x] Seed extended (secretary + second teacher with own period); other specs
      unaffected
- [x] `secretary-send.spec.ts` green on isolated stack, twice in a row on the
      same DB (idempotency)
- [x] Existing statement/collections/audit specs green (regression), with
      intentional updates only where a spec exercised plain-member self-send
- [x] Plain-teacher negative assertions green (no send entry points)
- [x] Docs updated in owning surfaces only, incl. D8 rule + release note
- [x] Full ladder green

## Success Criteria

- [x] Every acceptance criterion in plan.md checked off against a passing
      test or manual dev-stack verification

## Risk Assessment

- Seed change can break specs that assume a single-member roster — run the
  full e2e suite, not just the new spec.
- E2E cannot prove real Zalo delivery/throttling; that risk stays operational
  (befriend-first UX + pacing) and is documented, not tested here.
