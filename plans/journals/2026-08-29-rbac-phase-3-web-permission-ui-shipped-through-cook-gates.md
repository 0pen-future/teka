---
title: "RBAC phase 3: web permission UI shipped through cook gates"
date: 2026-08-29
summary: Matrix + member override dialog landed; review caught a real audit-page gate bug; e2e 26/26 on isolated stack
---

# RBAC phase 3: web permission UI shipped through cook gates

## What happened

Phase 3 of `plans/260829-1640-gh-260829-flexible-center-rbac/` executed end-to-end in `/ak:cook --auto`. New owner-side permission UI in `apps/web/src/features/center/`: `permission-matrix.tsx` (roles × 9-key catalog, save-per-role replace semantics, `reports.send` row disabled during dual-life) and `member-permissions-dialog.tsx` (immediate role assign + batched grant/deny overrides with effective-source badges). Member surfaces gate on effective permission keys from `/centers/me` via `useCenterContext().has()`.

Verification: typecheck clean, lint 0 errors, vitest 415 passed / 3 skipped. E2e on isolated `teka-e2e` compose stack (fresh volume + seed): 26/26. Two e2e fixes were needed: `getByRole("button", { name: "Đóng" })` collided with HvModal's X button aria-label "Đóng hộp thoại" (fix: `exact: true`), and the audit assertion moved from `center.member.send_reports_grant` to `center.member.overrides_update` because the new override endpoint is the audited write. Lesson repeated from memory: a reused e2e DB breaks statement/billing specs — the seeder is idempotent, not reset; always `down -v` before a full run.

## Review findings that mattered

Code-reviewer (DONE_WITH_CONCERNS) caught one real critical: nav gating moved to `audit.read` but `audit-page.tsx` still hard-gated `isOwner` — a granted member clicked the nav entry and got bounced to `/`. No test covered the grantee path, which is exactly why it slipped. Fixed the gate, added 4 grantee-path tests (nav, audit page, lesson-plans gate, `canAct=false` action hiding), added `isOwner` short-circuit in `has()` as deploy-skew tolerance, filtered `reports.send` from matrix payloads, MSW 204 defaults for the 3 PUT endpoints, dialog no longer returns silent `null`.

One major was a false positive worth remembering: reviewer claimed the legacy `SetSendReports` endpoint skips the dual-write. It doesn't — the mirror lives in the SQL CTE at `centers/repository.go:396-429`, not in the service layer. Rejected with source citation per review rules.

## Decision

- Frontend permission-gating convention documented in `docs/frontend-guidelines.md` (gate on `has(key)`, deep-link guard mandatory, labels only from API catalog).
- Review + tester + progress reports in `plans/reports/*260829*flexible-center-rbac*`.

## Next steps

- Phase 4 (drop `can_send_reports`, remove legacy endpoints, delete `send-reports-permission-dialog.tsx`) stays gated on prod soak + 0-drift parity query.
- Open product question: `center.manage`/`members.manage`/`invitations.manage` are grantable but unlock no member-facing web surface in v1 — needs an intent call before phase 4.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
