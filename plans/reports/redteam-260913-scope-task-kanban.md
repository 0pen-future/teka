# Red-team review — Scope & Complexity Critic

Plan: `plans/260913-1102-task-center-kanban/`
Contract: `reports/brainstorm-brief-rev3-extracted.md`
Reviewer role: unrequested-scope detector + CONTRACT VERIFIER
Date: 2026-09-13

Constraint honoured: the lib split (`apps/api/pkg/kanban`, `apps/web/src/lib/kanban`),
ports, and named patterns are user-requested scope and are NOT flagged as
over-engineering. Findings below target additions beyond the brief, patterns
applied where they cost without serving reuse or testability, duplication
between core and adapter, and phase steps doing unrequested work.

---

## Finding 1: The Observer port builds 9 event types to serve 1 consumer, and that one consumer is already served by a return value

- **Severity:** Critical
- **Location:** Phase 2, "Architecture → EventSink (Observer)" and Implementation Step 10; Phase 3, Implementation Steps 8 and 16
- **Flaw:** Phase 3 routes all task and column audit through the existing
  request-events middleware via `routespec` `req(...)` entries (Phase 3 step 11,
  Requirements/Functional). Only `task.handover` needs a service-level event.
  Yet Phase 2 mandates `events.go` with 9 event structs, an `EventSink` port, an
  after-commit publish ordering rule, and Phase 3 step 8 mandates an adapter
  type-switch onto `events.Bus`. Phase 3 step 16 then adds exactly ONE subscriber
  case. Eight of the nine events (`TaskCreated`, `TaskMoved`, `TaskUpdated`,
  `TaskDeleted`, `ColumnCreated`, `ColumnRenamed`, `ColumnDeleted`,
  `ColumnsReordered`) have no consumer anywhere in the plan.
  Worse, the one event that IS consumed is redundant: Phase 2 step 10 says
  `HandoverOnDeparture` returns `(unassigned, reassigned int)` AND publishes
  `TasksHandedOver`; Phase 3 step 7 and step 12 carry the same two ints back up
  through the facade and the `TaskHandover` interface. The same payload travels
  two paths.
- **Failure scenario:** A maintainer adds a tenth column operation and dutifully
  adds a tenth event struct plus a tenth adapter type-switch arm, then spends an
  afternoon debugging why no audit row appears — because the audit row was never
  coming from the bus. Meanwhile the handover count is authoritative in two
  places; when someone changes the counting rule in the core they update the
  return value and forget the event payload, and the audit trail silently
  disagrees with the API.
- **Evidence:**
  - Plan quote, Phase 2 line 98-100: "`EventSink` (Observer). `Publish(ctx, event any)`, gọi sau commit. Events: `TaskCreated`, `TaskMoved`, ... `TasksHandedOver`." (9 events)
  - Plan quote, Phase 3 line 160-161: "`audit/subscriber.go`: `case tasks.HandedOver`" (1 case, no others)
  - Plan quote, Phase 3 line 36-38: "Audit: `task.create/update/move/delete`, `task_column.create/update/reorder/delete` qua request-events" — the other 8 are already covered without the bus.
  - `apps/api/internal/features/audit/subscriber.go:209` — the `centers.RolePermissionsChanged` template the plan cites is a single case; the codebase has no precedent for a 9-event feature sink.
  - `apps/api/internal/shared/routespec/routespec.go:121` `req(action, entityType, idParam)` is the mechanism that already produces those 8 audit rows.
- **Suggested fix:** Keep the `EventSink` port (it is part of the reusable-lib
  request) but ship `TasksHandedOver` only, and delete the other 8 structs from
  Phase 2's create list until a consumer exists. Pick ONE transport for the
  handover counts: either the return value or the event, not both. Note in
  `pkg/kanban/README.md` that a consumer without a request-audit middleware can
  add its own events.

---

## Finding 2: Handover publishes its audit event before the enclosing transaction commits, so a rolled-back removal still logs a handover

- **Severity:** Critical
- **Location:** Phase 2, "Requirements → Non-functional" + Implementation Step 10; Phase 3, "Risk Assessment" row 3 and Success Criteria
- **Flaw:** Phase 2 declares two invariants: "Publish event **sau khi** UoW commit
  thành công" and "Không mở tx lồng nhau: chỉ method public của `Service` gọi
  `UnitOfWork`". Phase 2 step 10 has the core's `HandoverOnDeparture` open
  `uow.Within(...)` and publish `TasksHandedOver` after it returns nil. But this
  method is called from inside `centers.RemoveMember`'s existing transaction.
  `GormTxManager.WithinTx` joins an ambient transaction and returns as soon as
  `fn` returns — it does NOT commit. So the "after commit" publish fires while the
  outer transaction is still open, before `disabler.Disable` runs.
- **Failure scenario:** `RemoveMember` runs handover (2 UPDATEs, event published to
  the bus), then `disabler.Disable` fails. The outer transaction rolls back:
  `assignee_id` and `created_by` are restored, the member is not removed. The bus
  has already handed `tasks.HandedOver` to the audit subscriber, which writes a
  `task.handover` row on its own connection. The audit trail now records a
  handover of N tasks that never happened, on a member who is still active.
  Phase 3's success criteria — "rollback nguyên vẹn khi `Disable` lỗi" AND "audit
  `task.handover` có số việc" — can both pass while this bug is live, because they
  are asserted in different tests.
- **Evidence:**
  - `apps/api/internal/database/tx.go:23-30`:
    ```go
    func (m *GormTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
        if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
            return fn(ctx)
        }
        ...
    ```
    The nested branch returns `fn(ctx)` directly — no commit boundary.
  - `apps/api/internal/features/centers/service.go:298-303` — `RemoveMember` opens `s.tx.WithinTx` and calls `CloseMembership` then `s.disabler.Disable`; handover is to be inserted before `CloseMembership` (Phase 3 step 12), i.e. inside this closure.
  - Plan quote, Phase 2 line 54: "Publish event **sau khi** UoW commit thành công, đúng thứ tự gọi." — unachievable in this call path.
  - Plan quote, Phase 3 line 197: "Facade **không** gọi `uow.Within`" — but Phase 2 step 10 has the CORE call it, which the facade delegates to. The two phases contradict each other.
- **Suggested fix:** Decide explicitly whether the core's `HandoverOnDeparture`
  opens a UoW at all. Either (a) the core takes no UoW for this use-case and
  returns the counts, with `centers` responsible for the audit after its own
  commit, or (b) write the `task.handover` audit row inside the transaction, as
  the plan's own "Giả định có thể sai" note already contemplates. Do not leave the
  "publish after commit" invariant stated but unenforceable.

---

## Finding 3: Adding a "Công việc" nav entry to the first group breaks an exact-equality mobile tab assertion the plan never mentions

- **Severity:** High
- **Location:** Phase 5, "Related Code Files → Modify"
- **Flaw:** Phase 5 lists only `apps/web/src/layouts/dashboard-layout.tsx` as the nav
  change and specifies "nhóm đầu cạnh 'Tổng quan'". The mobile bottom tab bar
  derives its primary tabs by subtracting `OVERFLOW_LABELS` from the flattened nav
  groups. "Công việc" is not in that set, so it becomes a fourth primary tab. There
  is an exact-equality test on the tab list. Neither `OVERFLOW_LABELS`,
  `OVERFLOW_PATH_PREFIXES`, nor the test file appears anywhere in the plan.
- **Failure scenario:** Phase 5's own gate `make test-web` fails on a test the plan
  never predicted. The implementer then has to make an unplanned UX decision —
  is "Công việc" a primary mobile tab or a Thêm-sheet entry? — under pressure, with
  no brief guidance, and either answer changes the mobile information architecture
  the brief's mockup did not cover.
- **Evidence:**
  - `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx:111` test name "keeps three primary tabs plus a Thêm tab"
  - `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx:121`:
    ```js
    expect(tabLabels).toEqual(["Tổng quan", "Điểm danh", "Thu tiền"]);
    ```
  - `apps/web/src/layouts/dashboard-layout.tsx:170-182` `OVERFLOW_LABELS`
  - `apps/web/src/layouts/dashboard-layout.tsx:190-202` `OVERFLOW_PATH_PREFIXES`
  - `apps/web/src/layouts/dashboard-layout.tsx:71` the first group: `{ header: null, entries: [{ label: "Tổng quan", to: "/", Icon: HvHomeIcon }] }`
  - Plan quote, Phase 5 line 106-107: "`apps/web/src/layouts/dashboard-layout.tsx` — NavEntry 'Công việc' `/tasks` với `perm: 'tasks.list'`, nhóm đầu cạnh 'Tổng quan'." No mention of the two overflow sets or the test.
- **Suggested fix:** Add `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`
  to Phase 5's Modify list and state the decision: whether `/tasks` joins
  `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES` (Thêm sheet) or becomes a fourth
  primary tab at 360px.

---

## Finding 4: Two audit/route manifest test surfaces are missing from Phase 3, and the one it does name is the wrong file

- **Severity:** High
- **Location:** Phase 3, "Related Code Files → Modify" (last line) and Success Criteria bullet 1
- **Flaw:** Phase 3 asserts `apps/api/internal/server/router_test.go` "không sửa;
  phải xanh nhờ Specs" and makes it the AC9 evidence for route coverage. That file
  contains no manifest coverage test at all. The bidirectional manifest test lives
  in `route_policy_test.go`. Separately, `audit/action_test.go` pins a hardcoded
  `actionSnapshot` and asserts bidirectionally that every `routespec.Specs` entry
  with a non-empty `Audit.Action` appears in that snapshot. The plan adds 8 such
  entries and never lists this file.
- **Failure scenario:** The implementer wires 11 routespec entries, runs
  `make test-api`, and gets 8 failures from a package (`features/audit`) that the
  plan's phase gates never named, in a test whose comment says a change "must be
  justified by an intentional decision to rename or re-scope an audit action, not
  by a refactor". Without the plan flagging it, the fastest path looks like editing
  a test that is deliberately hard to edit. AC9's stated evidence points at a file
  that cannot produce it.
- **Evidence:**
  - `apps/api/internal/server/router_test.go` — the only tests are `TestHealthzAlwaysOK` (:86), `TestUnknownRouteReturns404Envelope` (:102), `TestSwaggerServedOutsideProductionOnly` (:129), `TestOversizedBodyIsRefusedBeforeHandlers` (:152), `TestLoginIPLimiterOnlyMountedWithTrustedProxies` (:191), `TestRosterImportIsExemptFromGlobalBodyCap` (:210), `TestRequestIDGeneratedAndEchoed` (:225). None reads `routespec.Specs`.
  - `apps/api/internal/server/route_policy_test.go:16` `TestRoutePolicyCoversEveryRegisteredRoute` — the actual bidirectional manifest test.
  - `apps/api/internal/features/audit/action_test.go:42` `var actionSnapshot = []struct{...}` — hardcoded literal list.
  - `apps/api/internal/features/audit/action_test.go:136-143`:
    ```go
    for _, s := range routespec.Specs {
        if s.Audit.Action == "" { continue }
        id := s.Method + " " + s.Path
        if _, ok := want[id]; !ok {
            t.Errorf("%s: has Audit.Action %q but is missing from the snapshot", id, s.Audit.Action)
    ```
  - Plan quote, Phase 3 line 117: "`apps/api/internal/server/router_test.go` — không sửa; phải xanh nhờ Specs."
- **Suggested fix:** Correct the reference to `route_policy_test.go` and add
  `apps/api/internal/features/audit/action_test.go` to Phase 3's Modify list with
  the 8 new snapshot rows enumerated.

---

## Finding 5: Moving cross-feature wiring into `container.go` is justified by a claim about the operator CLI that the codebase contradicts

- **Severity:** High
- **Location:** Phase 3, "Architecture → Consumer-defined interface + setter injection"; plan.md Decision D8
- **Flaw:** The plan overrides the brief ("Brief §4 ghi `server.registerFeatures`;
  đây là sửa lỗi có chủ đích của brief") on the grounds that "Container là nơi
  wiring dùng chung cho cả operator CLI — đặt ở router thì CLI mất bước bàn giao".
  No CLI path reaches `RemoveMember`. The three CLI commands are create-center,
  seed, and reset-password. The only caller of `RemoveMember` is the HTTP handler.
  The stated reason for the deviation does not exist.
  The cost is real: `container.go` currently builds only process-lifetime services
  and the identity trio, and its own comment states exactly why. Putting `tasksSvc`
  there means constructing a feature service (repository → service) outside
  `registerFeatures`, contradicting the documented split, and adds a new field or a
  new `NewRouter` parameter so the HTTP layer can still mount the handler.
- **Failure scenario:** A reviewer asks "why is tasks in the container?" and the
  answer in the plan is wrong. The next feature that needs cross-feature wiring
  copies the precedent, and `container.go` accretes feature services until the
  router/container boundary the codebase documents is gone. Meanwhile nothing was
  gained — the handover hook runs on exactly one code path either way.
- **Evidence:**
  - `apps/api/internal/cli/` contains only `create_center.go`, `seed.go`, `reset_password.go`; `app.NewContainer` callers: `internal/cli/create_center.go:59`, `internal/cli/seed.go:29`, `internal/cli/reset_password.go:54`, `internal/app/app.go:19`.
  - Only non-test caller of `RemoveMember`: `apps/api/internal/features/centers/handler.go:124`.
  - `apps/api/internal/app/container.go:38-45` — "Teachers, Centers, and Auth are built here … because the operator CLI's onboarding commands (create-center, reset-password) need the exact same **identity** wiring". Task handover is not identity wiring.
  - `apps/api/internal/server/router.go:103-106` — "registerFeatures wires feature modules into the versioned group. **Feature construction (repository → service → handler) happens here** so features stay decoupled from bootstrap".
  - Plan quote, Phase 3 line 58-63.
- **Suggested fix:** Either follow the brief and wire `SetTaskHandover` in
  `registerFeatures` alongside the other feature construction, or keep the
  container placement and replace the justification with the real one, whatever it
  is. Do not ship a deviation whose stated reason is checkable and false.

---

## Finding 6: Port signatures using `kanban.TenantID` switch off scopelint R1 for the new repositories, making "make scopelint xanh" vacuous

- **Severity:** High
- **Location:** Phase 2, Implementation Step 3; Phase 3, "Requirements → Non-functional" and "Architecture → Read-widening ở đâu"
- **Flaw:** Phase 2 mandates "Mọi method nhận `tenant TenantID` tham số đầu sau
  `ctx`" for both repository ports. scopelint R1 only fires on functions that
  accept an `authctx.Scope`, `Anchor`, or `OwnerAnchor` parameter. A repository
  method taking `kanban.TenantID` is invisible to the witness rule, so the
  guardrail that every other feature's repositories live under does not apply to
  the tasks repositories at all.
  Phase 3 then states the opposite of Phase 2: "mọi method repository nhận `Scope`
  phải bind `center_id`" and describes `taskRepository.readScoped(ctx, sc)` taking
  an `authctx.Scope`. Phase 2 says the same port method `ReadScoped` takes an
  `Actor`. The two phases specify incompatible signatures for the same method.
- **Failure scenario:** Every write method (`Update`, `SoftDelete`,
  `MoveAllToColumn`, `UnassignBy`, `ReassignCreator`, `UpdatePositions`, `Delete`)
  takes `TenantID` and is never checked by R1. A future edit drops the
  `center_id = ?` bind from `MoveAllToColumn` and `make scopelint` stays green —
  the only cross-tenant guardrail in the repo says nothing. Phase 3's success
  criterion "`make scopelint` xanh" is satisfied by a linter that examined nothing.
- **Evidence:**
  - `apps/api/tools/scopelint/scopelint/analyzer.go:30-40`: "Every function in that package's non-test files … **that accepts an authctx.Scope, Anchor or OwnerAnchor parameter** and touches a raw *gorm.DB root … must contain a witness".
  - `apps/api/tools/scopelint/scopelint/analyzer.go:61-64` (R3) and `:489-490` — `CenterWideFor` outside a read-named function is the only fork the plan accounted for.
  - `apps/api/internal/features/sessions/repository.go:126` `func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB` and `:168` `writeScoped(ctx, sc authctx.Scope, roles []string)` — the precedent the plan cites (Phase 3 step 1) passes `Scope`, not an opaque tenant id.
  - Plan quote, Phase 2 line 140: "Mọi method nhận `tenant TenantID` tham số đầu sau `ctx`."
  - Plan quote, Phase 3 line 41-42: "mọi method repository nhận `Scope` phải bind `center_id` hiển thị".
- **Suggested fix:** Resolve the signature contradiction explicitly, and if the
  ports keep `TenantID`, say so in Phase 3 and add a replacement guard —
  a test asserting every tasks repository query binds `center_id`, or a
  `//scopelint:` note — instead of letting the phase claim a linter pass it
  cannot earn.

---

## Finding 7: One PATCH endpoint is split into two core use-cases with no combined operation, so a request carrying both fields applies in two transactions

- **Severity:** Medium
- **Location:** Phase 2, "Requirements → Functional" (use-case list) versus Phase 3, Implementation Step 11 and the brief's endpoint table
- **Flaw:** The brief defines one endpoint, `PATCH /api/v1/task-columns/:id` with
  body `{name?, is_done?}`, producing one audit action `task_column.update`. Phase 2
  provides `RenameColumn` and `SetColumnDone` as two separate use-cases and no
  `UpdateColumn`. The adapter therefore has to call both for a request that sets
  both fields — two policy checks, two `uow.Within` calls, two commits, one HTTP
  response. `SetColumnDone` is also the only column write with no corresponding
  event in the 9-event list, so the Observer treatment is inconsistent across the
  very operations it was introduced for.
- **Failure scenario:** A user renames "Chờ duyệt" to "Đã duyệt" and ticks the done
  flag in one save. The rename commits, the done toggle fails on a constraint or a
  dropped connection, and the endpoint returns an error. The column is now renamed
  but not marked done, with a single audit row that says "update" and no record of
  which half landed. Refetch shows a half-applied change the user did not ask for.
- **Evidence:**
  - Brief, endpoint table: "PATCH /api/v1/task-columns/:id — tasks.manage_board — task_column.update / task_column / id — {name?, is_done?}"
  - Plan quote, Phase 2 line 38-41: use-case list contains `RenameColumn`, `SetColumnDone`, no `UpdateColumn`.
  - Plan quote, Phase 2 line 98-100: 9 events, containing `ColumnRenamed` but no done-flag event.
  - Plan quote, Phase 3 line 183-184: "`PATCH` cờ `is_done` không đụng `completed_at` cũ" — treats it as a single PATCH.
- **Suggested fix:** Add a single `UpdateColumn(ctx, tenant, actor, colID, patch)`
  use-case in Phase 2 that applies both optional fields in one transaction and
  emits one `ColumnUpdated` event, and drop the two split use-cases. This also
  removes one of the two mismatched event/use-case counts.

---

## Finding 8: Two Phase 1/6 verification claims cannot be produced by the fixtures and test conventions they name

- **Severity:** Medium
- **Location:** Phase 6, Requirements + Success Criteria bullet 1; Phase 1, "Requirements → Non-functional" and Implementation Step 12
- **Flaw (a):** Phase 6 requires the matrix test to show "5 khoá CRUD đã tick sẵn
  cho 3 vai trò". The default MSW read model gives all three system roles
  `permissions: []` by design — its comment says "born with empty permission sets
  (v1 parity)". Nothing in the web fixture models the SQL backfill, so no matrix
  test can show pre-ticked CRUD keys without new fixture work that neither Phase 1
  nor Phase 6 lists.
  **Flaw (b):** Phase 1 asks the parity test to prove "SQL backfill của 000022 khớp
  tập khoá mà `DefaultRoleKeys()` thêm ở generation này". The file it names as the
  template explicitly refuses to derive expectations from the live catalog and says
  so in its header comment. Following the plan's wording produces a test that
  breaks on catalog v5 — the exact failure the existing file was written to avoid.
- **Failure scenario:** (a) Phase 6 step 3 "thêm case test ma trận theo bước 2"
  produces a test that asserts unchecked boxes, silently weakening AC8 to "the tab
  renders" while the plan's checklist reads as if backfill was verified in the UI.
  (b) The next catalog bump makes an immutable-migration parity test fail, and the
  fix looks like editing shipped SQL.
- **Evidence:**
  - `apps/web/src/test/msw/handlers.ts:319-345` — `DEFAULT_ROLES`, all three with `permissions: [] as string[]`; header comment ":320-322" reads "the three system roles born with empty permission sets (v1 parity)".
  - `apps/web/src/test/msw/handlers.ts:348-353` `DEFAULT_CENTER_PERMISSIONS` composes those empty roles.
  - `apps/api/migrations/backfill_parity_test.go:13-19`: "the expectations here are **frozen literals** of that v2 generation, **not derivations from the live catalog** … Catalog drift is the live tests' job, not this file's."
  - `apps/api/migrations/backfill_parity_test.go:110` `TestBackfillSQLMatchesFrozenCatalogV2` — the cited template.
  - Plan quote, Phase 6 line 119: "5 khoá CRUD tick sẵn 3 vai trò".
  - Plan quote, Phase 1 line 44-45: "Parity test chốt: SQL backfill của 000022 khớp tập khoá mà `DefaultRoleKeys()` thêm ở generation này."
- **Suggested fix:** For (a), either add an explicit fixture with the 6 backfilled
  keys to Phase 6's Modify list, or restate the criterion as what the fixture can
  prove. For (b), reword Phase 1 to say the 000022 parity test freezes a literal
  6-key list (matching the existing convention), not a derivation from
  `DefaultRoleKeys()`.

---

## Open questions for the plan owner

1. Is "Công việc" a primary mobile bottom tab or a Thêm-sheet entry? The brief's
   mockup shows only the desktop sidebar. This is a UX decision the plan makes
   implicitly by choosing the first nav group.
2. Does the repository enforce `writeOwn` / `writeParticipant` in SQL as the brief's
   "Quy tắc scope trong repository" section states, or is the owner/creator/assignee
   rule enforced only by `kanban.DefaultPolicy` in the core? Phase 2's port list has
   plain `Update` / `SoftDelete` with no write-scoping methods, while the brief
   specifies SQL predicates. If both, the same rule lives in Go and in SQL and must
   be kept in sync.
3. `DefaultGrant` is the brief's suggestion, not one of its four chốt decisions, and
   Phase 1 acknowledges it becomes a third backfill-control mechanism alongside
   `Grantable` and `legacyIdentitySet`. Adding `tasks.manage_board` to
   `legacyIdentitySet` is one line. Is the extra attribute worth it now, or when the
   second opt-in special key arrives?

---

## Verification Results

**Tier:** Full
**Claims checked:** 31 · **Verified:** 24 · **Failed:** 6 · **Unverified:** 1

### Caller enumerations

| Interface change | Consumers found | Plan coverage |
|---|---|---|
| `PermDef` gains `DefaultGrant` | Struct literals only in `authctx/catalog.go:132` (`def`), `:139` (`viewAll`), `:147` (`permCatalog`), `:269` (zero value). Readers: `centers/service.go:330`, `:365`, `:393`; `centers/permissions_integration_test.go:67-68`; `authctx/catalog_test.go:48,108,117,125,134,205-211,257,276`. No external composite literals → additive field is safe. | VERIFIED (7 non-test + 11 test sites, none break) |
| `DefaultRoleKeys()` 53 → 59 | Callers: `internal/testutil/fixtures.go:115`, `internal/features/centers/repository.go:373`, `seeds/seed.go:439`, `migrations/migrations_test.go:1719`, `centers/permissions_integration_test.go:92,506,528`, `authctx/catalog_test.go:247,284,288`. All derive dynamically except the literal `53` at `catalog_test.go:248-249`. Empirically confirmed current count = 53 via a throwaway test run. | VERIFIED — plan names the one hardcoded site |
| `CatalogVersion` 3 → 4 | Go: `authctx/catalog.go:294`, `catalog_test.go:299-300` (hardcoded `3`), `centers/service.go:398,443`, `centers/permissions_integration_test.go:548,559,578,596,609,620` (all dynamic). Web: `src/test/msw/handlers.ts:17` (hardcoded), consumed dynamically at `handlers.ts:352`, `center-page.test.tsx:8,251,292`, `center-permissions.test.tsx:10,137,419`. Independent literals in `center-schemas.test.ts:98,103,134,167,183` are schema-parsing fixtures, version-agnostic. | VERIFIED — 2 hardcoded sites, both named by the plan |
| `centers.Service` gains `SetTaskHandover` / `RemoveMember` change | `AccountDisabler` `service.go:24`, `SetAccountDisabler` `:57`, `RemoveMember` `:277`, `CloseMembership` call `:299` — all four plan line refs exact. `RemoveMember` callers: `centers/handler.go:124` only. `NewContainer` callers: `cli/create_center.go:59`, `cli/seed.go:29`, `cli/reset_password.go:54`, `app/app.go:19`. | **FAILED** — plan's CLI justification (Finding 5); no CLI reaches `RemoveMember` |
| New `routespec.Specs` entries (11) | Helper line refs exact: `perm()` `:105`, `none()` `:117`, `req()` `:121`, `Specs` `:130`. Derived consumers: `server/route_policy_test.go:16` (bidirectional coverage), `server/route_policy_enforce_test.go:62-140` (owner / full-grant / exact-key), `routespec/routespec_test.go:26-241`, `audit/action_test.go:123-146`. `routespec_test.go:225` `TestKnownSkipSetsUnchanged` pins only AuthSession/Service/Anonymous → new routes unaffected. | **FAILED** — `audit/action_test.go:42` `actionSnapshot` (75 literal rows) not listed; `router_test.go` named instead of `route_policy_test.go` (Finding 4) |
| MSW fixture `apps/web/src/test/msw/handlers.ts` | `PERMISSION_CATALOG` `:39`, `ALL_PERMISSION_KEYS` `:317`, `DEFAULT_ROLES` `:319`, `DEFAULT_CENTER_PERMISSIONS` `:348`. Consumers: `handlers.ts:781,787,856`; `center-handlers.ts:3,39,91`; `center-permissions.test.tsx:11,75,234,236,349,379`; `center-schemas.test.ts:3,180`. | **FAILED** — `DEFAULT_ROLES` all `permissions: []`, cannot show Phase 6's pre-ticked CRUD keys (Finding 8a) |
| `RESOURCE_LABELS` / `ADMIN_RESOURCES` | `permission-schemas.ts:70-91` `RESOURCE_LABELS` (`members: "Thành viên"` present at `:84`, no `tasks`); `:124-132` `ADMIN_RESOURCES` (contains `members`); `groupCatalog` `:105`; `buildCatalogTabs` `:150`. Fallback `RESOURCE_LABELS[resource] ?? resource` at `:113`. Matrix test `center-permissions.test.tsx:142-166` asserts tab names by role query, tolerant of a new tab. | VERIFIED — all Phase 6 claims accurate |
| `eslint.config.js` changes | `no-restricted-imports` precedent block at `:38-61`; the rule itself `:45-59`, patterns `:48-58`. Plan cites `:45-57`. | VERIFIED (off by two lines, immaterial) |
| scopelint contract | R1 witness requires an `authctx.Scope`/`Anchor`/`OwnerAnchor` parameter — `analyzer.go:30-40`. R3 read-token rule — `analyzer.go:61-64`, `hasReadToken` `:431`, enforcement `:489-490`. Precedent `sessions/repository.go:126,168`. | **FAILED** — ports take `TenantID`, R1 never fires (Finding 6); Phase 2 and Phase 3 specify incompatible signatures |
| Transaction nesting contract | `database.TxManager` iface `internal/database/tx_manager.go:8-11`; `GormTxManager.WithinTx` `internal/database/tx.go:23-30` joins ambient tx and returns without committing; `FromContext` `:35-40`. | **FAILED** — Phase 2's "publish after commit" invariant unachievable in the handover path (Finding 2) |
| Event/audit wiring | `audit/subscriber.go:209` `case centers.RolePermissionsChanged` — cited template exact. Plan adds 9 core events, 1 subscriber case. | **FAILED** — 8 of 9 events have no consumer (Finding 1) |

### Other claims spot-checked

| Claim | Result |
|---|---|
| `apps/api/pkg/` does not yet exist | VERIFIED (`apps/api/` holds cmd, docs, internal, migrations, seeds, tools) |
| Next migration is 000022 | VERIFIED (`migrations/` ends at `000021_attendance_status_late`) |
| `catalog.go` line refs `:34` PermDef, `:294` CatalogVersion, `:330` DefaultRoleKeys | VERIFIED, all exact |
| `catalog_test.go` `:246` / `:248` (count 53) / `:298` | VERIFIED, all exact |
| `migrations/backfill_parity_test.go:110` template | VERIFIED file+line; semantics contradicted (Finding 8b) |
| `docs/adding-permissions.md` §2 `:44`, §4 `:108`, §9 `:211` | VERIFIED |
| `docs/event-bus.md` `:35` catalog, `:60` action map, `:76` blind spots | VERIFIED |
| `docs/architecture.md` `:5` Monorepo, `:12` Applications | VERIFIED |
| `features/roster/hooks/roster-keys.ts` exists | VERIFIED |
| `apps/web/e2e/roster.spec.ts` exists as e2e template | VERIFIED |
| `app/router.tsx` feature-route import block `:5-17` | VERIFIED |
| Endpoint count matches the brief (11, no creep) | VERIFIED — 1 board + 4 column + 5 task + 1 directory, exactly the brief's table |
| Phase 4 a11y claim about react-aria kanban roles | UNVERIFIED — external source; the plan already gates it behind Phase 4 step 1, which is the right handling |

