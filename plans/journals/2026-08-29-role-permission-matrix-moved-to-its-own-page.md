---
title: Role permission matrix moved to its own page
date: 2026-08-29
summary: Phân quyền vai trò left the /center section for an owner-only /center/permissions page with sidebar entry
---

# Role permission matrix moved to its own page

## What happened

The "Phân quyền vai trò" role-permission matrix lived as an HvCard section on the owner branch of `/center` (`center-page.tsx`). Per user request it moved to a dedicated page at `/center/permissions` (`apps/web/src/features/center/pages/center-permissions-page.tsx`), with an owner-only sidebar entry in the "Trung tâm" nav group and the old card reduced to an interactive link card. `PermissionMatrix` itself unchanged; no API change.

## Decision

- Gate copied from `audit-page.tsx`: `useCenterContext` → null while unresolved, `Navigate to="/" replace` for non-owners, so the owner-only read model never fires on a member deep link. Verified against the Go handler: `requirePermissionAdmin` is a bare `IsOwner` check, so `isOwner` (not `has(key)`) is the correct gate.
- Kept redirect-on-error (audit-page consistency) instead of an inline error message; repo has both patterns — worth settling once.
- `OVERFLOW_PATH_PREFIXES` untouched: `/center` prefix already covers `/center/permissions`.

## Outcome

Commit 45b411d on master (7 files, +137/−16). Typecheck clean, lint 0 errors, 419 tests pass. Code-reviewer subagent: DONE_WITH_CONCERNS; all medium findings applied (non-owner redirect test with request counter, nav most-specific-wins active-state test, `aria-label` on the link card, `HvCard interactive` + repo-standard focus ring).

## Next steps

None required. Optional: unify the owner-only page error pattern (redirect vs inline message) across audit and permissions pages.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
