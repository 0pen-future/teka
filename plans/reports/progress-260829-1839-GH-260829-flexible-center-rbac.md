# Progress Sync — Flexible Center RBAC

Plan: `plans/260829-1640-gh-260829-flexible-center-rbac/`. Sync-back after
phase 3 complete. Phases 1-3 done, phase 4 pending on prod soak.

## Phase status

| Phase | Status | Notes |
|---|---|---|
| 1 Foundation | Completed | zero-behavior-change, verified this session |
| 2 API surface | Completed | gate swap + 4 mgmt endpoints, verified |
| 3 Web permission UI | Completed | reviewer + tester both PASS, post-review fixes applied |
| 4 Cleanup | Pending | gated on prod soak — NOT started, do not start |

## Evidence per phase (reverified this session, code-level)

**Phase 1**
- `ReportsOversight()` body unchanged: `authctx.go:59-61` = `IsOwner ||
  CanSendReports`. Zero-test-edit guard holds.
- grep `sc\.IsOwner\|scope\.IsOwner` in `*/repository.go` → 0 hits repo-wide
  except `centers/repository.go` (SQL alias, expected).
- grep `\.Has(` in `*/repository.go` → 0 hits. `CenterWide()` used instead in
  10 repo files (billing, sessions, enrollments, statements, contacts,
  classes, notifications, students, attendance, payments).
- Guard test file exists: `internal/features/scoping_guard_test.go`.
- Integration tests exist: `centers/rbac_integration_test.go`,
  `send_reports_integration_test.go`, `secretary_integration_test.go`.

**Phase 2**
- Audit events wired: `centers/events.go` + `service.go` define
  `RolePermissionsChanged`/`MemberRoleChanged`/`MemberOverridesChanged`;
  `audit/subscriber.go` persists them.
- `permissions_integration_test.go` confirms: role-matrix rejects
  `reports.send` w/ 422 (:120-122 comment "per-member only while column
  authoritative"), override grant/revoke dual-writes legacy column both
  directions (:205-227).
- 4 mgmt endpoints + owner-only gate present per file.

**Phase 3**
- Reviewer report (`review-260829-1714-...-phase3.md`): verdict DONE_WITH_CONCERNS
  → all critical/major fixed same session, suite reverified 415 passed after
  fixes. M1 (claimed missing dual-write) rejected as false positive — SQL CTE
  at `centers/repository.go:396-429` mirrors column<->override row same stmt.
- Tester report (`tester-260829-1830-...-phase-3.md`): PASS, typecheck clean,
  lint 0 errors (4 pre-existing warnings), vitest 411/3 skipped/62 files
  (pre-post-review-fix count; final count 415 passed post-fix per reviewer).
- Post-review fixes (this session, verified applied): audit-page.tsx gates on
  `has("audit.read")` not isOwner (fixed C1); `use-center-context.ts` has()
  short-circuits isOwner (M3); permission-matrix.tsx filters `reports.send`
  from save payload (Md4); member-permissions-dialog.tsx renders loading/error
  instead of null + unified min-h-11 (Md1, N1); MSW 204 defaults for 3 PUT
  endpoints (Md5); 4 new grantee-path tests added (M2).
- e2e: `secretary-send.spec.ts` rewritten to drive `MemberPermissionsDialog`;
  audit assertion now `center.member.overrides_update`; 26/26 green on
  isolated `teka-e2e` stack, fresh seed, stack torn down after run.

## plan.md top-level Success Criteria — reverified and ticked this session

All 8 phase-1-3-scoped criteria now `[x]` (test-suite green, both greps 0
hits, revoke/grant integration test, reopen-reset test, owner UI flow, 
`/centers/me` effective perms + web gating, dual-life 422 + dual-write parity,
audit events). Column-removal criterion stays `[ ]` — phase 4 only.
Frontmatter `status: in-progress` kept as-is (correct — phase 4 pending).

## What phase 4 waits on

Hard gate per phase-04-cleanup.md: must NOT ship in same release as phase 3;
requires prod soak (deployed, owner has used new UI, no regressions) +
parity query (`can_send_reports` column vs `reports.send` override row, must
= 0 drift) run against prod before the column drop migration. This is a
business/ops decision (time in prod), not something verifiable from source —
do not start phase 4 work until soak window + parity check both clear.

## Open items / unresolved questions (from phase-3 review report)

- `center.manage`/`members.manage`/`invitations.manage` are grantable via the
  matrix but no member-facing web surface consumes them in v1 (owner-branch
  UI only) — intent or phase-4 backlog? consider a matrix hint.
- If a member-facing `center.manage` surface ever ships, `renameCenter()`
  client fn currently parses owner-shaped response only — revisit then.
- Md2 (matrix last-write-wins, no version guard), Md3 (all role columns
  disabled during one save), Md6 (matrix loading/error render untested), Md7
  (unused multi-payload mock affordance), N2-N6 (nits: tooltip reachability,
  `scope="col"`, remind-button gating, `has` closure identity, catalog
  fixture drift guard) — all accepted/deferred by reviewer, not blocking.

**Next action: finish the plan.** Phase 4 is the only remaining phase — once
soak window elapses and parity query = 0, run phase-04-cleanup.md steps
(migration 000014 drop column, remove dual-write/legacy endpoints, port
notifications mid-run probe + seeds/fixtures to override rows, delete web
dialog, full test suite + e2e green). Do not consider this plan closed until
phase 4 lands — the column drop and legacy-endpoint removal are load-bearing
parts of the original scope (Goal 4: no `IsOwner` left in repositories is
otherwise already met, but the dual-life parity contract is only fully
resolved after phase 4).
