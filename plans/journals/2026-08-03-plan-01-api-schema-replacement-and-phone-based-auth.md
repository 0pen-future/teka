---
title: "Plan 01: API schema replacement and phone-based auth"
date: 2026-08-03
summary: "Replaced the email-based users schema with the So Lop V1 baseline and rebuilt auth as phone-based; all gates green, coverage 72.8%"
---

# Plan 01: API schema replacement and phone-based auth

## What happened

Executed all 3 phases of `plans/260803-2244-01-api-schema-replacement-and-auth/` via /ak:cook (auto):

- **Phase 1**: migration `000001` = `docs/schema_design.sql` verbatim (17 domain tables + 2 views), `000002` refresh_tokens; embedded golang-migrate; seeds re-keyed by phone (idempotent). Verified via testcontainers round-trip (up→down→up) instead of the local dev stack — no ghost processes.
- **Phase 2**: new `features/teachers` (Account+Teacher over user_accounts/teachers, shared UUIDv7, `scoped()` tenancy helper), `features/auth` rewritten phone-based: `vnphone` validator + `NormalizePhone` (E.164 storage), dummy-bcrypt burn on every login failure branch (empirically ~270ms vs ~253ms real compare), rotating refresh tokens with family revocation preserved byte-for-byte. Deleted `features/users/` + `cli/admin.go`.
- **Phase 3**: `/me` GET/PUT moved to teachers feature; handler-level `currentProfile` gate maps soft-deleted/disabled/non-teacher tokens → 401. Docs: `api-guidelines.md` rewritten (new Tenancy section is the canonical `scoped()` convention for plans 02–06), `local-development.md` seed line fixed.

## Verification

- `make test-api` green, coverage **72.8%** (floor 60%); `make lint-api` 0 issues; `make api-docs` zero diff; migration file diffs empty vs schema source.
- code-reviewer: DONE_WITH_CONCERNS — H1 fixed in-session (rollback test could never fail; replaced with a direct `Service.Register` call using a 101-char full_name so the teachers INSERT fails after the account row is written — now actually proves the transaction rolls back). M2 fixed (stale doc). M1 accepted: web e2e stays red until plan 07 replaces the frontend — recorded in `adr.md`.
- tester: DONE — no failures, no flaky tests, containers cleaned up.

## Decisions (recorded in adr.md)

17-vs-16 table count, testcontainers verification instead of dev stack, `AccountService` returns `*teachers.Profile`, early deletion of users/admin in phase 2, `scoped()` filters `id` on identity tables (`teacher_id` form documented for domain tables), handler-level 401 gate, web e2e red window until plan 07.

## Next steps

Plan 02 (roster management) → 03 → 04 → 05 → 06, then design-system foundation, then plans 07/08. Known deferred findings from review: no test pins the identical-401 invariant across login failure modes (M3), no test decodes issued token claims (M4), bcrypt-in-transaction + no rate limiting as a DoS surface (M5) — candidates for plan 02+ hardening.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
