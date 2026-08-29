# Code Review — Phase 3 Web Permission UI (Flexible Center RBAC)

Reviewer: code-reviewer subagent, 2026-08-29. Scope: 6 created + 12 modified
files in `apps/web` (contract cross-checked against `apps/api` centers
feature, authctx registry, migration 000013). Verification reran by reviewer:
typecheck clean, lint 0 errors, vitest 62 files / 411 passed / 3 skipped.

Verdict: DONE_WITH_CONCERNS → all critical/major findings resolved same
session (see Resolution), suite re-verified green after fixes (415 passed).

## Findings and resolution

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| C1 | Critical | `/audit` page still gated `isOwner` while nav gates `audit.read` → granted member clicks nav, gets redirected. Broke "granted permission usable end-to-end". | **Fixed**: `audit-page.tsx` enabled-gate + redirect now use `has("audit.read")`; member filter documented as roster-scoped (empty for grantee). |
| M1 | Major | Claimed legacy `SetSendReports` doesn't dual-write the override row → divergence during soak. | **Rejected (false positive)**: dual-write lives in SQL, `centers/repository.go:396-429` — CTE mirrors column into `center_member_permissions` (upsert on grant, delete on revoke) in the same statement. |
| M2 | Major | No tests covered the grantee path (nav shown, page renders, actions hidden). | **Fixed**: 4 new tests — dashboard-layout grantee nav (audit.read + imports.run), audit page renders for grantee, lesson-plans gate opens for `teaching.review_queue`, review actions hidden when `canAct=false`. |
| M3 | Major | `permissions` default `[]` turns "older API" into "owner loses nav" (web-before-api deploy skew). | **Fixed**: `has()` short-circuits `isOwner` (`use-center-context.ts`) — belt-and-braces; server already folds owner bypass. |
| Md1 | Medium | Dialog returned `null` on missing data → "Phân quyền" click silent no-op. | **Fixed**: renders modal with loading/error text instead of null. |
| Md4 | Medium | Matrix save sent full checked set incl. a stray server-side `reports.send` → 422. | **Fixed**: payload filters `REPORTS_SEND_KEY`. |
| Md5 | Medium | No MSW default handlers for the 3 new PUT endpoints. | **Fixed**: 204 defaults added in `test/msw/handlers.ts`. |
| Md2 | Medium | Matrix replace semantics last-write-wins, no version guard. | Accepted: owner-only editor, low concurrency risk; noted for future. |
| Md3 | Medium | `mutation.isPending` disables all role columns during one save. | Accepted: saves are quick and rare; `savingRoleId` already labels the busy button. |
| Md6 | Medium | Matrix lacks loading/error render tests. | Deferred: states implemented (`Đang tải…` / `Không tải được phân quyền.`); low-risk gap. |
| Md7 | Medium | `mockCenterPermissions` multi-payload affordance unused. | Accepted: mirrors `mockCenterMe` scripted pattern; harmless. |
| N1 | Nit | Override select `min-h-9` vs role select `min-h-11`. | **Fixed**: unified `min-h-11`. |
| N2-N6 | Nit | Tooltip reachability, `scope="col"`, remind-button gating, `has` closure identity, catalog fixture drift guard. | Deferred as nits; no behavior impact. |
| N7 | Nit | `mau-nhap-du-lieu-trung-tam.xlsx` untracked at repo root, not phase-3. | Flagged to git step: exclude from commit. |

## Acceptance criteria (reviewer-confirmed)

- Owner edits matrix / role / overrides with server-state re-render — met.
- Send-reports fully through new UI; legacy dialog unreferenced (file kept for
  phase 4) — met.
- Non-owner center page byte-identical — met.
- Contract matches phase-2 handlers (3 PUT bodies + path params) — met.
- Dual-life honored in UI (reports.send disabled on role rows, per-member
  only) — met.
- Suites green — re-verified post-fix: typecheck clean, lint 0 errors, vitest
  415 passed / 3 skipped, e2e 26/26 (isolated `teka-e2e` stack, fresh seed).

## Unresolved questions

- `center.manage` / `members.manage` / `invitations.manage` are grantable but
  no member-facing web surface opens for them in v1 (owner-branch UI only).
  Intent or phase-4 backlog? Consider a matrix hint so owners don't assume a
  grant unlocks UI.
- If a member surface for `center.manage` ever ships, `renameCenter()` parses
  the owner-shaped response only — revisit then.
