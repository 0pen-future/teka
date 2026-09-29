---
title: can_send_reports removed; reports.send is the only send authority
date: 2026-09-29
summary: Dropped Scope.CanSendReports and the can_send_reports wire field; every check reads reports.send from the effective permission set
---

# can_send_reports removed; reports.send is the only send authority

## What happened
The DB column was already dropped in migration 000019, but the name survived in three layers: `authctx.Scope.CanSendReports` (a copy of `Perms.HasKey(reports.send)`), the `can_send_reports` JSON on `GET /centers/me` (member shape and owner roster rows), and the web (`canSendReports`, `canRunSends`, roster badge "Thư ký gửi báo cáo").

## Changes
- `Scope.ReportsOversight()` is now `Has(PermReportsSend)`; field removed from Scope, fixtures, seed, ResolveScope.
- `MemberResponse`/`MemberMeResponse` lose `can_send_reports`; roster SQL no longer computes it. Swagger regenerated.
- Notifications: cross-teacher `zalo_personal` send/preview and the delegated run grant use `delegatedSender(sc)` = `Perms.HasKey(reports.send)` (no owner bypass), so owner behavior is unchanged. RunStore probe renamed `HoldsReportsSend`.
- Web: gates use `has("reports.send")`; roster badge removed (grant is visible in the Phân quyền dialog).
- e2e `secretary-send.spec.ts`: the grant helper waited on the removed badge; it now reopens the Phân quyền dialog and asserts the persisted `reports.send` override value instead.

## Gotcha
`go vet ./...` without `-tags integration` missed 9 integration tests missing the `authctx` import; the first `make test-api` failed at build. Always vet with `-tags integration` after test edits.

## Verification
make test-api-unit, make test-api (78.6% coverage), web vitest 1188 pass, tsc, eslint. Full Playwright suite on a fresh isolated `teka-e2e` stack: 128 passed.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
