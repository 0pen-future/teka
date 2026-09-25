# Red-team review — Scope & Complexity Critic

Plan: `plans/260923-0715-giang-day-menu/` (plan.md + phase-01..09)
Repo @ `95bb7f4`. Role: unrequested-scope detector + CONTRACT VERIFIER.
Requested scope is treated as a constraint, not a finding.

---

## Finding 1: `classes.code NOT NULL` + unique index breaks 159 test-fixture insert sites the plan never names

- **Severity:** Critical
- **Location:** Phase 1, "Requirements R1" and "Implementation Steps A.1"
- **Flaw:** R1 makes `code` `NOT NULL`, backfills, then `ALTER COLUMN code DROP DEFAULT`, and adds `CREATE UNIQUE INDEX uq_classes_center_code ON classes (center_id, code) WHERE deleted_at IS NULL`. Code generation lives only in `classes/service.go` (step B.8). But the repo's only other class-creation path inserts through GORM directly, bypassing the service, and will write `code = ''`. The first class per center passes; the **second** violates the partial unique index.
- **Failure scenario:** `make test-api` goes red across the suite, not in `classes` alone. `testutil.Class` inserts with `db.Omit("Schedules").Create(c)` and no `Code`. Every integration test that builds two classes under one center (owner + member class, class A + class B for cross-scope assertions) fails on the second insert with `duplicate key value violates unique constraint "uq_classes_center_code"`. The plan's Phase 1 Success Criteria claims "`make test-api` xanh cho package `classes` và `migrations`" — it never runs the other 34 files.
- **Evidence:**
  - `apps/api/internal/testutil/fixtures.go:409-425` — `func Class(t, db, teacherID, opts...)`: `c := &classes.Class{ID, TeacherID, CenterID, Name, StartDate, DefaultUnitPrice, Status}` then `db.Omit("Schedules").Create(c)`. No `Code`, and `ClassOption` has no `WithClassCode` (`fixtures.go:385-406`).
  - `testutil.Class(` — **159 call sites across 35 files** (`apps/api/internal/features/{statements,students,sessions,collections,centers,grading,handoff,payments,classes,notifications,teaching,zalo,attendance,contacts}/...`, `apps/api/internal/shared/classscope/integration_test.go`, `apps/api/internal/server/policy_integration_test.go`).
  - Only two creation paths exist: `classes/service.go:86` (`repo.CreateWithSchedules`) and `testutil/fixtures.go:411`. Grep for `testutil` / `fixtures.go` across `plans/260923-0715-giang-day-menu/*.md` returns **zero hits**.
- **Suggested fix:** Either add `Code` generation to `testutil.Class` in R1's file inventory (and state the 159-site blast radius), or drop `NOT NULL` and make the unique index `WHERE code <> '' AND deleted_at IS NULL`. The latter also removes the backfill-collision risk the plan's own risk table hedges on.

---

## Finding 2: D2's accept flow calls an owner-only service; self-accept of a `giao_vien` invite can only ever 403

- **Severity:** Critical
- **Location:** Phase 2, "Key decisions" (D2) and the API table row `POST /class-invitations/:id/accept`
- **Flaw:** The plan routes `giao_vien` acceptance through `handoff.Service.Reassign`, and marks the accept route `authenticated (self)` with the check "`teacher_id == caller` trong service". `Reassign` refuses any caller who is not the center owner, as its first statement. The invitee is by construction a non-owner member. The self-accept path is unreachable for the primary role the feature exists to move.
- **Failure scenario:** Owner invites a teacher as `giao_vien`. Teacher opens `/class-invitations`, taps "Chấp nhận", gets `403 chỉ chủ trung tâm được bàn giao lớp`. The invitation stays `pending` forever, and the only working path is the owner pressing `accept-on-behalf` — which is just the existing owner-only handoff with extra tables in front of it.
- **Evidence:**
  - `apps/api/internal/features/handoff/service.go:110-113`:
    ```go
    func (s *Service) Reassign(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) (*Result, error) {
        if !sc.IsOwner {
            return nil, apperror.Forbidden("chỉ chủ trung tâm được bàn giao lớp")
    ```
  - Plan quote (phase-02 "Key decisions"): "**Chấp nhận vai `giao_vien` đi qua `handoff.Service.Reassign`** (`handoff/service.go:110`)". The plan cites the exact line above the gate it contradicts.
  - Second, undersized side effect: `Reassign` is not a staff-row write. Inside one center-advisory-locked transaction it calls `classes.ReassignTeacher` (moves the class **and every schedule row**), `sessions.ReassignPlanned` (moves **all future planned sessions**), and `staff.SyncPrimaryTeacher` (`handoff/service.go:147-160`). Phase 2's Risks section reduces this to "Reassign trong handoff có side effect (thông báo/audit)". It has neither notification nor audit side effect; it has data movement.
  - `TryLockCenter` refuses rather than waits (`handoff/service.go:180-186`), so an accept colliding with an import returns `409 một thao tác khác của trung tâm đang chạy` — a state the plan's invitation UI does not model.
- **Suggested fix:** Decide the authorization story before writing the migration. Either accept-of-`giao_vien` is owner-confirmed (drop self-accept for that role and keep only `accept-on-behalf`), or add an explicitly scoped `Reassign` variant that takes the invitation as its authority. Do not leave it as "route kind authenticated — scout cách `KindSelf` dùng hiện tại".

---

## Finding 3: one "phase" rule, four implementations

- **Severity:** High
- **Location:** Phase 1, R3/R4/R5 + "File inventory" (`classes/phase.go`, `classes/repository.go`, `roster/lib/class-phase.ts`)
- **Flaw:** The upcoming/running/ended/archived rule is specified to exist simultaneously as (1) Go `PhaseOf(class, today)`, (2) an SQL predicate inside `ListReadable` for `?phase=`, (3) a second SQL expression `SUM(CASE ...)` in `CountReadableByPhase`, and (4) a TypeScript `classPhase` in `roster/lib/class-phase.ts`. Nothing in the plan makes any one of them the source of truth.
- **Failure scenario:** Server timezone vs browser timezone at a date boundary. At 00:30 Asia/Saigon the Go `today` (UTC on the server) is still yesterday, so `PhaseOf` returns `upcoming` in `ClassResponse.phase`, the SQL `?phase=running` filter excludes the row, `/classes/stats` counts it under `upcoming`, and the web `classPhase` recomputes `running` for the chip. The table shows a class the chip count denies. Four sites means four places to fix and four places to forget.
- **Evidence:**
  - Plan, phase-01 R5: "`ClassResponse` thêm `code`, `tags`, `recruiting`, `note`, `phase`".
  - Plan, phase-01 inventory: `apps/api/internal/features/classes/phase.go | Create (PhaseOf(class, today), shiftOf(start_time)) | 50`; `apps/web/src/features/roster/lib/class-phase.ts | Create (classPhase, phaseLabel, shiftOf, weekdayOptions, shiftOptions) | 60`.
  - Plan, phase-01 step B.7: "`Phase` → điều kiện ngày tương ứng" and "Thêm `CountReadableByPhase(ctx, sc, today)` một query `SUM(CASE ...)`".
  - No existing derivation to collide with — `grep -rn "Sắp khai giảng\|Đang học\|Đã kết thúc" apps/web/src/` returns only `curriculum-card.tsx:57,77` (lesson progress) and `student-detail-page.tsx:91` (enrollment), so all four are new.
- **Suggested fix:** Keep the SQL predicate (needed for filtering and counting) and the server-emitted `phase` field. Delete `roster/lib/class-phase.ts#classPhase` and render `class.phase` from the API; keep only the label map and the weekday/shift option lists on the web.

---

## Finding 4: ~78 new routes land on two hand-maintained authorization test tables the plan never names

- **Severity:** High
- **Location:** All phases; Phase 1 "File inventory" (`routespec.go` "Modify | +1"), Phase 7 "Verification" ("Manifest tests cho 8 route mới")
- **Flaw:** Counting the route tables in phases 2-8: 7 + 15 + 15 + 8 + 11 + 9 + 12 ≈ **78 new endpoints**, plus `GET /classes/stats`. The plan tracks this only as `routespec.go` edits and a generic "manifest test". Two other hand-written tables must gain an entry per route, and neither appears anywhere in the plan.
- **Failure scenario:** The implementer finishes Phase 3, runs `make test-api-unit`, and gets a diff of 15 unexplained routes against a snapshot whose doc comment forbids mechanical updates. Effort estimates (2d for Phase 3, 2d for Phase 4) do not carry this. Worse, the snapshot's purpose is to force an explicit authorization decision per route; bulk-appending 78 entries to make the build green defeats the control it exists for.
- **Evidence:**
  - `apps/api/internal/server/route_policy_snapshot_test.go` — **140** hardcoded `{Method:...}` entries; header comment at `:5-9`: "If this test ever needs to change, the change must be justified by an intentional authorization decision, not by a refactor accidentally reshuffling a route's classification or permission key."
  - `apps/api/internal/features/audit/action_test.go` — **78** `ActionSpec{}` entries, one per mutating route (e.g. `:78 {"POST", "/api/v1/classes/:id/sessions", ActionSpec{Action: "session.create", ...}}`). Phase 2 says "Mọi route mutating: `req(action, "class_invitation", "id")` audit" without naming this table.
  - `grep -rn "route_policy_snapshot\|action_test\|snapshot" plans/260923-0715-giang-day-menu/*.md` → **no matches**.
  - `apps/api/internal/server/route_policy_test.go:16` `TestRoutePolicyCoversEveryRegisteredRoute` — the one test the plan does name, and the weakest of the three.
- **Suggested fix:** Add both files to every phase's file inventory with a per-route count, and re-estimate. If 78 routes is genuinely the shape, say so in plan.md as a headline number rather than discovering it phase by phase.

---

## Finding 5: 12 new permission keys, 6 `CatalogVersion` bumps, 6 backfill migrations — for one menu

- **Severity:** High
- **Location:** plan.md D5; Phase 2/3/5/6/7/8 "Key decisions"
- **Flaw:** New keys: `class_invites.manage`; `library.read|edit|publish`; `courses.read|edit`; `paths.read|edit`; `class_messages.post`; `prep.read|edit|assign`. That is 12 keys on a 68-key catalog (**+18%**) and 6 separate `CatalogVersion` bumps (4 → 10) inside one feature branch. `CatalogVersion` is not a cosmetic counter — it is a compare-and-swap anchor whose mismatch is a 409 on permission writes. Six bumps is six forced client reloads, and a hard-pinned unit test edited six times.
- **Failure scenario:**
  - Each phase that merges alone ships a version bump. A center owner with the permission dialog open when the deploy lands gets `Cấu hình quyền đã thay đổi từ lần tải trước, hãy tải lại rồi lưu lại` on save, six times over the rollout.
  - `catalog_test.go:317-318` hard-asserts `CatalogVersion != 4` with the message "catalog version must be 4 after adding the tasks group". Every phase must rewrite this assertion and its message, so the test degrades into a line the implementer bumps reflexively.
  - The permission admin UI groups by resource and falls back to the raw slug for unknown resources. Six new resources (`library`, `courses`, `paths`, `prep`, `class_invites`, `class_messages`) render as raw English slugs as Vietnamese group headings.
- **Evidence:**
  - `apps/api/internal/shared/authctx/catalog.go:325-334` — "CatalogVersion is the CAS anchor for permission-assignment writes ... a mismatch is a 409"; `const CatalogVersion = 4`. Current catalog size: 68 `def(`/`optIn(` entries.
  - `apps/api/internal/features/centers/service.go:510-517` `checkCatalogVersion` → `staleAssignmentConflict()`; `service.go:505-507` message text.
  - `apps/api/internal/shared/authctx/catalog_test.go:316-319`.
  - `apps/web/src/test/msw/handlers.ts:17` `export const CATALOG_VERSION = 4;`, `:39` `PERMISSION_CATALOG`, `:355` `ALL_PERMISSION_KEYS`, `:390` `catalog_version: CATALOG_VERSION`.
  - `apps/web/src/features/center/schemas/permission-schemas.ts:70-92` `RESOURCE_LABELS` — 22 entries, none of them `library|courses|paths|prep|class_invites|class_messages`; `:113` `label: RESOURCE_LABELS[resource] ?? resource`. `grep -rn "RESOURCE_LABELS\|permission-schemas" plans/260923-0715-giang-day-menu/*.md` → **no matches**.
  - Naming collision worth noting: `RESOURCE_LABELS.teaching = "Giảng dạy"` already exists (`permission-schemas.ts:78`). The new menu is also called "Giảng dạy", so the permission screen will show a "Giảng dạy" group that contains none of the new menu's keys.
- **Suggested fix:** Bump `CatalogVersion` **once** at the end of the branch, not per phase, and say so in D5. Collapse read/edit pairs where a separate read key buys nothing (`paths.read` gates a catalog the whole center may see). Add `RESOURCE_LABELS` to the file inventory of every phase that adds a resource.

---

## Finding 6: Phase 8 explicitly declines two existing kanban implementations and duplicates Phase 3's table

- **Severity:** High
- **Location:** Phase 8, "Key decisions" and "Migration sketch — 000032_prep"
- **Flaw:** Two separate duplications in one phase.
  1. The plan states "Backend **không** tái dùng `pkg/kanban` (position float không cần; 4 cột cố định)" and then specifies a board: four statuses, `position INT`, `PUT /prep/projects/:id/items/order`, `PATCH /prep/items/:id` for status moves, assignees. That is a kanban, reimplemented, against a purpose-built in-repo library whose entire design rationale is host-independent reuse through ports.
  2. `prep_items` columns are `title, objectives TEXT, duration_min INT, homework_note TEXT, materials JSONB` — a **column-for-column copy** of Phase 3's `template_lessons` (`title, objectives TEXT, duration_min INT, homework_note TEXT`) plus a materials array. The two are then joined by a one-way copy endpoint `POST /library/templates/generate`.
- **Failure scenario:** Two tables and two write surfaces for the same "a lesson being drafted" concept. Any later change to lesson fields (add `slides_url`, change `duration_min` semantics) must be made twice and the copy step updated, or generated templates silently lose the new field. Meanwhile `pkg/kanban`'s reorder semantics, column policy, and 
its boundary test are re-earned from scratch in `prep`.
- **Evidence:**
  - `apps/api/pkg/kanban/README.md:1-25` — "Pure Go Kanban domain library: configurable columns and tasks moving between them, for one tenant at a time ... every side effect (persistence, transactions, events, authorization) is a port the host implements", enforced by `import_boundary_test.go`. Package files: `columns.go entity.go errors.go events.go options.go policy.go ports.go service.go`.
  - `apps/web/src/lib/kanban/` — headless core (`state.ts positions.ts selectors.ts use-kanban.ts use-kanban-keyboard.ts data-source.ts`); Phase 8 does reuse this one, which makes the backend rejection less coherent, not more.
  - `apps/api/migrations/000022_task_board.up.sql:26-50` — `tasks` already carries `assignee_id UUID`, `position DOUBLE PRECISION`, `column_id`, with `fk_tasks_assignee_center ... ON DELETE SET NULL (assignee_id)` and `idx_tasks_assignee`. `prep_items` re-derives `assignee_id`, `due_date`, `position INT`, `status`.
  - Plan quote, phase-03 migration sketch: `template_lessons (id, version_id, center_id, position INT, title, objectives TEXT, duration_min INT, homework_note TEXT, ...)`.
  - Plan quote, phase-08 migration sketch: `prep_items (id, project_id, center_id, position INT, title, objectives TEXT, duration_min INT, homework_note TEXT, materials JSONB, status, assignee_id, due_date, ...)`.
- **Suggested fix:** Before writing 000032, answer in the plan why a draft lesson cannot simply be a `template_lessons` row on an unpublished draft version with a `status` + `assignee_id` column. If the answer is "it can", Phase 8 collapses to a board view over Phase 3's tables and the generate step disappears. If it genuinely cannot, state the distinguishing requirement; "4 cột cố định" is not one.

---

## Finding 7: four endpoints that exist to save the client an arithmetic operation

- **Severity:** Medium
- **Location:** Phase 5 `GET /courses/:id/stats`; Phase 7 `GET /library/versions/:vid/apply-preview`; Phase 2 `POST /class-invitations/:id/remind` and the nav badge
- **Flaw:** Each adds a route (and therefore a routespec entry, a snapshot entry, a swagger block, and a test) for data the page already has or for an action with no observable effect.
  - `GET /courses/:id/stats → {classes_running, classes_upcoming, students}`: the same detail page already renders tab "Vận hành" from `useClassesList({course_id})` per the same phase. Two counts of the same rows, from two endpoints, on one screen.
  - `GET /library/versions/:vid/apply-preview?class_id=` returns "số buổi mẫu vs số buổi thực". Phase 4 already specifies `GET /library/versions/:vid` returning the full lesson list, and `GET /classes/:id/sessions` already returns the real sessions. The preview is a subtraction the client can do.
  - `POST /class-invitations/:id/remind` sets `reminded_at = now()`. Under non-goal N1 there is no SMS or Zalo delivery, and the repo's `notifications` feature is tuition-period-scoped, not an in-app inbox. The endpoint, the column, and its permission path deliver nothing to the invitee.
  - Nav badge: "Badge số lời mời pending của tôi trên nav entry (tuỳ chọn, chỉ nếu rẻ)". An optional item with no acceptance criterion, in a plan that already runs 16 days.
- **Failure scenario:** Not an outage — a slow leak. Each costs a routespec entry, a snapshot entry, an `action_test` entry for the mutating one, a swagger regeneration, and a test, against Finding 4's already-underestimated route budget.
- **Evidence:**
  - Plan quote, phase-05 API: "`GET /courses/:id/stats` → `{classes_running, classes_upcoming, students}` (read port của classes/enrollments)" and, in the same file's Web section: "Vận hành (`useClassesList({course_id})` từ `@/features/roster`)".
  - Plan quote, phase-04 API: "`GET /library/versions/:vid` trả đầy đủ lessons + materials + exercises (một payload cho Phase 7 read-through)".
  - Plan quote, phase-07 API: "`library`: `GET /library/versions/:vid/apply-preview?class_id=` → số buổi mẫu vs số buổi thực".
  - Plan quote, phase-02: "\"Nhắc lại\" chỉ cập nhật `reminded_at` + audit (không SMS/Zalo — non-goal N1)". Delivery surfaces in repo: `apps/api/internal/features/notifications/` is period-scoped (`ListByPeriod(ctx, sc, periodID, filter)`, `repository.go:84`); `apps/api/internal/features/zalo/` links contacts, not members.
- **Suggested fix:** Drop `apply-preview` and the badge. Fold the course counts into the course detail response or read them from the class list's `meta.total`. Keep "Nhắc lại" only if it writes something the invitee can see; otherwise leave the button out rather than shipping a silent no-op.

---

## Finding 8: Phase 1 reuses two components that will not render what the plan describes

- **Severity:** Medium
- **Location:** Phase 1, "Key Insights", R7, and Implementation Steps 16 and 18
- **Flaw:** Two concrete reuse claims are wrong against current source.
  1. `ClassStaffSection` returns `null` for any non-owner. The plan uses it for "Nhân viên phụ trách" on `/classes/:id` — a page whose stated audience is teachers reading their own classes.
  2. `StatusPill` accepts only payment statuses. Step 16 says "trạng thái dùng `StatusPill`" for class phase.
- **Failure scenario:** A teacher opens their class detail page. The "Nhân viên phụ trách" block and the "Đội ngũ giảng dạy" block are both empty — not "Lớp chưa có giáo viên.", just absent, because the component short-circuits before its own empty state. The Vitest quartet in the plan's test matrix renders as owner and never catches it. Separately, `<StatusPill status="running">` is a TypeScript error, caught at build time rather than review time — cheap, but it signals the reuse list was assembled without opening the files.
- **Evidence:**
  - `apps/web/src/features/roster/components/class-staff-section.tsx:33` — doc comment "\"Nhân sự lớp\" — owner-only roster of who works a class and in what role."; `:40` `const { isOwner } = useCenterContext();`; `:44-46`:
    ```tsx
    if (!isOwner) {
      return null;
    }
    ```
    Its only production caller today is `class-settings-page.tsx:277`, an owner-only settings page.
  - `apps/web/src/components/hv/status-pill-labels.ts:1` — `export type StatusPillStatus = "paid" | "partial" | "unpaid";`
  - Plan quote, phase-01 step 18: "`ClassStaffSection` lọc vai `hoc_vu` cho \"Nhân viên phụ trách\"" and R7 "khối Đội ngũ giảng dạy đọc `class_staff` hiện có"; step 16: "trạng thái dùng `StatusPill`".
  - Also note `ClassStaffSection` takes only `{ classId }` (`:39`) — there is no role-filter prop to pass `hoc_vu`.
- **Suggested fix:** Decide whether non-owners may see class staff, and if so put the `isOwner` relaxation (and its authorization consequence) in the plan rather than in the implementer's lap. Use `HvBadge` or `HvChip` for class phase; `StatusPill` is the collections widget.

---

## Finding 9: six nav entries enter the mobile overflow set, but the overflow path list is never updated

- **Severity:** Medium
- **Location:** Phase 1 R8/step 20, and the identical one-liner in phases 2, 3, 5, 6, 8
- **Flaw:** Every phase says "thêm vào `OVERFLOW_LABELS`". The mobile bottom bar decides which tab is *active* from a **second**, separate array of static path prefixes, because entry `to` values can be null. No phase mentions it. Six new routes therefore go into the "Thêm" sheet and never light it up.
- **Failure scenario:** On a 360px phone, a user taps Thêm → "Kho học liệu" and lands on `/library`. The bottom bar shows no active tab at all — Thêm is not highlighted because `/library` is not in the prefix list, and no other tab matches. Same for `/classes`, `/class-invitations`, `/courses`, `/paths`, `/prep`. Six silent navigation regressions, none covered by the plan's test matrix (which tests only "mobile overflow chứa nhãn").
- **Evidence:**
  - `apps/web/src/layouts/dashboard-layout.tsx:172-187` `OVERFLOW_LABELS`; `:189-193` doc comment: "Route families reachable only through the Thêm sheet. Static prefixes, not the entries' `to` values, because the period-scoped links are `null` until the current period resolves — **the tab must still light up on their routes**."; `:194-205` `OVERFLOW_PATH_PREFIXES`; `:350` `const active = OVERFLOW_PATH_PREFIXES.some(...)`.
  - `grep -rn "OVERFLOW_PATH_PREFIXES" plans/260923-0715-giang-day-menu/` → **no matches**. `OVERFLOW_LABELS` appears in 6 phase files.
  - Secondary nav-crowding point: `/classes` ("Danh sách lớp học", perm `classes.list`) joins two existing entries gated on the same key — `dashboard-layout.tsx:83` "Quản lý lớp học" → `/classbook` (perm `classes.list`) and `:139` "Lớp & học sinh" → `/students`. U2 pins the first two, so this is not a cut recommendation; it is a question the plan should answer in the nav section, since a user will now see three class-shaped entries in two groups.
- **Suggested fix:** Add `OVERFLOW_PATH_PREFIXES` to the file inventory of every phase that adds a nav entry, and add one assertion per route to the nav test.

---

## Finding 10: the whole menu inventory is an unvalidated assumption, and a conditional phase is a hard dependency

- **Severity:** Medium
- **Location:** plan.md "Giả định & quyết định người dùng" A1; Phase 8 header and Risks; Phase 9 `dependencies: [1..8]`
- **Flaw:** A1 — the list of six menu items that defines all nine phases — is marked "**Giả định** — script v5 không lấy được; xác nhận ở Validation". The scout report confirms the v5 navigation script was truncated and the group does not exist in v3. So 16 days, 8 migrations (000025–000032), ~78 routes and 12 permission keys are sequenced against a menu composition nobody has confirmed. Phase 8 compounds it: its header says "**Chờ xác nhận ở Validation (Q2)**: có đưa flow này vào scope không", yet Phase 9 lists `8` as a blocking dependency and its seed task hedges "1 dự án prep (nếu Phase 8 giữ)".
- **Failure scenario:** Validation returns "Chuẩn bị tài liệu is out, and Lộ trình học belongs under Tuyển sinh". Phase 6 and Phase 8 (3 days, migrations 000030 and 000032, 5 permission keys) are already merged or half-built, and migration numbers 000030/000032 are burned in a sequence that other work is branching from. Phase 9's dependency list and seed script both need surgery.
- **Evidence:**
  - plan.md, row A1: "Menu \"Giảng dạy\" gồm: ... | **Giả định** — script v5 không lấy được; xác nhận ở Validation".
  - `reports/scout-260923-giang-day-menu.md:12-18` — "`prototype-v5.stripped.html` | Markup đủ 29 màn, **mất script** sau `</x-dc>`"; "Kết luận: nhóm \"Giảng dạy\" **không có trong v3**; v5 thêm nhóm này nhưng script không lấy được."
  - phase-08 header: "**Chờ xác nhận ở Validation (Q2)**"; phase-08 Risks: "nếu Validation trả lời \"để sau\" → chuyển phase sang `status: deferred` và bỏ nav entry"; phase-09 frontmatter: `dependencies: [1, 2, 3, 4, 5, 6, 7, 8]`.
  - Related, smaller factual slip in the same deferred-verification style — phase-07 writes the audit filter as `GET /audit?entity_type=class&entity_id=`. The actual route the web calls is `/audit-logs` (`apps/web/src/features/audit/api/audit-api.ts:22` `apiClient.get<unknown>("/audit-logs", {params})`). The columns already exist (`apps/api/migrations/000010_audit_logs.up.sql:25-26 entity_type TEXT NOT NULL DEFAULT '', entity_id TEXT NOT NULL DEFAULT ''`) and are already returned (`audit/dto.go:21-22`), but the only indexes are `(center_id, occurred_at DESC, id DESC)` and `(actor_user_id, occurred_at DESC)` (`000010:36,39`) — an entity filter has no supporting index and the plan calls it a compatible extension with no migration.
- **Suggested fix:** Resolve A1 and Q2 **before** Phase 3 starts, since phases 3-8 all branch from them. Until then mark Phase 8 `status: deferred` and remove `8` from Phase 9's dependency list, so a "no" answer does not require editing the plan's spine. Add an `(center_id, entity_type, entity_id, occurred_at DESC)` index to Phase 7's migration sketch if the entity filter stays.

---

## Verification Results

Contract Verifier — full consumer enumeration for every interface the plan changes.

### (a) `ClassResponse` / `classSchema` consumers — PASS (additive, type-compatible)

Schema: `apps/web/src/features/roster/schemas/roster-schemas.ts:177-192` (`classSchema`), `:192` `export type Class`.

Direct schema consumers (6):
| Site | Use |
|---|---|
| `roster/api/classes-api.ts:26` | `parseList(classSchema, res.data)` |
| `roster/api/classes-api.ts:31` | `parseData` |
| `roster/api/classes-api.ts:41` | `parseData` |
| `roster/api/classes-api.ts:47` | `parseData` |
| `roster/index.ts:17` | re-export |
| `roster/__tests__/roster-schemas.test.ts:5,52,68,74` | 3 assertions |

`useClassesList` call sites — **12 across 8 features**:
`collections/pages/collections-page.tsx:52` · `center/pages/class-config-page.tsx:146` · `attendance/pages/sessions-page.tsx:61` · `dashboard/components/dashboard-stats.tsx:41` · `dashboard/components/class-overview-cards.tsx:101` · `profile/pages/profile-page.tsx:33` · `teaching/pages/records-page.tsx:52` · `teaching/pages/lesson-plans-page.tsx:33` · `teaching/pages/classbook-page.tsx:88` · `roster/components/enroll-student-dialog.tsx:80` · `roster/pages/students-page.tsx:139` · definition `roster/hooks/use-classes.ts:22`.

`listClasses` callers: `roster/hooks/use-classes.ts:25` only (definition `roster/api/classes-api.ts:24`).

`type Class` imported by name: `center/pages/class-config-page.tsx:5`, `dashboard/components/class-overview-cards.tsx:7`.

**Compat:** R5 adds `code`, `tags`, `recruiting`, `note`, `phase`, all with `.default()`. Precedent exists — `my_staff_roles` and `student_count` are already `.default()`ed at `:186-189` for exactly this reason. No consumer destructures exhaustively. **Additive and safe.** Phase 1's risk row "Fixture MSW đổi shape làm đỏ test teaching/dashboard | Trung bình" is over-rated: see (c).

### (b) `classes.ListFilter` / `ListReadable` / `List` — PASS (struct-additive), one undeclared cross-feature consumer

`ListFilter` definition: `apps/api/internal/features/classes/repository.go:17-20` (`{Status string}`).

Production consumers of `classes.ListFilter` outside the feature — **2, neither in the plan's inventory**:
- `apps/api/internal/features/centers/dashboard.go:22` — `ClassReader` consumer-defined interface embeds `classes.ListFilter` in its `List` signature.
- `apps/api/internal/features/centers/handler.go:422` — `filter := classes.ListFilter{}`, then `status` only (`:423-432`), feeding `TeacherClasses` (`dashboard.go:196,203`).

In-feature: `repository.go:30` (`List`), `:36` (`ListReadable`), `:255`, `:259`, `:263` (`list`), `service.go:143-144`, `service.go:202-203`, `handler.go:107,119`.

Tests: `classes/integration_test.go:214,384,421,479,514,526`; `classes/service_test.go:138,146`; `centers/integration_test.go:720,832,845,851`.

`ListReadable` callers — **4 production/test**: `service.go:203` (repo), `handler.go:119` (handler), `integration_test.go:514,526`. Note the service signature returns 4 values including a roles map (`service.go:202`), so `CountReadableByPhase` must not be modelled on it.

**Compat:** keyed struct literals everywhere, so adding `Q/Weekday/Shift/Tag/Phase/CourseID` compiles unchanged. **But** `centers`' teacher-classes endpoint will silently ignore the new query params while sharing the DTO that now carries `phase` — worth one line in the plan. Neither `centers/dashboard.go` nor `centers/handler.go` appears in any phase's file inventory.

### (c) MSW `GET /classes` fixture consumers — PASS (small blast radius)

Fixture: `apps/web/src/test/msw/handlers.ts:487-511` `makeClass()`; handler `:969` `http.get(${API_URL}/classes, ...)` returns an empty list by default.

`makeClass` consumers — **12 call sites in 3 files**: `test/msw/handlers.ts`, `features/center/__tests__/class-config-page.test.tsx`, `features/dashboard/__tests__/dashboard-page.test.tsx`.
`makeClassSession` (`handlers.ts:514-529`) — **2 files**: `handlers.ts`, `dashboard/__tests__/dashboard-page.test.tsx`.
`classResponse()` (a separate local builder) — `roster/__tests__/roster-schemas.test.ts` only, 3 uses.

The current fixture already omits `my_staff_roles` and `student_count` and passes, which proves `.default()` absorbs new fields. **Risk row in Phase 1 should be downgraded from Trung bình to Thấp.**

### (d) `useNavGroups` / `OVERFLOW_LABELS` — **FAILED (missing consumer)**

- `dashboard-layout.tsx:63` `useNavGroups()`; `:542` call site; group definitions `:72` (Tổng quan), `:74-86` (Dạy học), `:89-111` (Học phí), `:114-156` (Trung tâm).
- `OVERFLOW_LABELS` `:172-187`; read at `:544-545`.
- **Missing from the plan: `OVERFLOW_PATH_PREFIXES` `:194-205`, read at `:350`.** See Finding 9.
- Nav test: `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx` (exists — Phase 1's inventory hedges "nếu có").
- Route mount point: `apps/web/src/app/router.tsx` aggregates each feature's `*Routes`; phases 3/5 mention it, phases 2/6/8 do not.

### (e) `CATALOG_VERSION` / `PERMISSION_CATALOG` / `CatalogVersion` — **FAILED (missing consumer)**

| Site | Current | Plan impact |
|---|---|---|
| `apps/api/internal/shared/authctx/catalog.go:334` | `const CatalogVersion = 4` | 6 bumps → 10 |
| `apps/api/internal/shared/authctx/catalog_test.go:316-319` | hard-pins `4`, message names the tasks group | rewritten 6× |
| `apps/web/src/test/msw/handlers.ts:17` | `CATALOG_VERSION = 4` | 6 bumps |
| `apps/web/src/test/msw/handlers.ts:39-354` | `PERMISSION_CATALOG` mirror | +12 `perm(...)` entries |
| `apps/web/src/test/msw/handlers.ts:355` | `ALL_PERMISSION_KEYS` | derived, auto |
| `apps/api/internal/features/centers/service.go:469,510-517` | CAS check → 409 | 6 stale-client windows |
| `apps/api/internal/features/centers/permissions_integration_test.go:548,559,578,596,609,620` | 6 assertions | revalidate each bump |
| `apps/web/src/features/center/schemas/permission-schemas.ts:70-92` | `RESOURCE_LABELS`, 22 entries | **not in plan** — 6 new resources render raw slugs |

Catalog size today: 68 entries. See Finding 5.

### (f) `GET /classes/:id/sessions` consumers — PASS (additive), wrong feature attributed

Route owner is **`sessions`, not `classes`**: `apps/api/internal/features/sessions/routes.go:10` `classGroup.GET("/:id/sessions", h.listRange)`; policy `routespec.go:270` (`sessions.list`); snapshot `route_policy_snapshot_test.go:91`. Phase 7 files this change under its "API — `classes`" section.

Web consumers — **4**:
| Site | Use |
|---|---|
| `attendance/api/attendance-api.ts:22-27` | `listClassSessions()` — the parse boundary |
| `attendance/hooks/use-sessions.ts:46` | `queryFn` |
| `dashboard/hooks/use-dashboard.ts:66` | per-class fan-out over the period |
| `teaching/pages/lesson-plans-page.tsx:41` | per-class fan-out over the month |
| re-export | `attendance/index.ts:8` |

**Compat:** Phase 7 adds `lesson_index` and `source` as derived fields — additive, and the three read sites select named fields. Safe. Note Phase 1's class-sessions tab therefore requires `roster` → `@/features/attendance` cross-feature import (legal via `index.ts:8`, but absent from Phase 1's inventory and from D6's import rules).

### (g) Audit list handler query params and its web consumer — **FAILED (wrong path, no index)**

- Handler params: `audit/handler.go:41` `actor_id`, `:48` `action`, `:49` `from`, `:56` `to`, `:63` `cursor`, `:64` `limit`. No entity params.
- Query struct: `audit/service.go:24-32` `ListQuery{ActorID, Action, From, To, Cursor, Limit}`; normalized to `ListSpec` `:37-46`.
- `Action` is already a **prefix** filter (`service.go:26` "filters by prefix, e.g. \"class.\" matches class.create"), exercised at `read_api_integration_test.go:200,204`.
- Columns exist: `migrations/000010_audit_logs.up.sql:25-26`; surfaced at `audit/dto.go:21-22`.
- Indexes: only `idx_audit_logs_center_time` (`000010:36`) and `idx_audit_logs_actor` (`000010:39`). **No entity index; Phase 7 adds no migration for one.**
- Web consumer: `apps/web/src/features/audit/api/audit-api.ts:16-22` — path is **`/audit-logs`**, params passed through as a `Record<string, string>`. Phase 7 writes `GET /audit?entity_type=class&entity_id=`.
- Test table: `audit/read_api_integration_test.go:168-171` `TestListFilters` "proves each query filter against real SQL" — a new filter needs a case here.

---

## Unresolved questions

1. Is a self-accept of a `giao_vien` class invitation intended at all, given `handoff.Reassign` is owner-only and moves schedules plus future planned sessions? (Finding 2)
2. Why can a prep item not be a `template_lessons` row on a draft version, with `status` and `assignee_id`? (Finding 6)
3. May a non-owner see class staff on `/classes/:id`? Today `ClassStaffSection` renders nothing for them. (Finding 8)
4. Can `CatalogVersion` bump once for the whole branch instead of six times? (Finding 5)
5. A1 and Q2 gate phases 3-8 — can they be resolved before Phase 3 rather than at Phase 9 validation? (Finding 10)
