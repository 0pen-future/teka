# Red-team review — Failure Mode Analyst (Murphy's Law)

Plan: `plans/260923-0715-giang-day-menu/` · Reviewer role: FLOW TRACER · 2026-09-23

---

## Finding 1: Phase 2 accept flow is dead on arrival — `handoff.Reassign` refuses every non-owner

- **Severity:** Critical
- **Location:** Phase 2, "Key decisions" + API table rows `POST /class-invitations/:id/accept`; plan.md D2
- **Flaw:** The plan routes the invited teacher's own accept through `handoff.Service.Reassign`. That method's first statement is an owner check. The invitee is by definition not the owner, so the accept can never succeed.
- **Failure scenario:** Owner invites member M as `giao_vien`. M opens "Lời mời nhận lớp", presses "Nhận lớp". Service calls `Reassign(ctx, M's scope, classID, M)` → `403 "chỉ chủ trung tâm được bàn giao lớp"`. The invitation stays `pending` forever. The e2e in plan.md Success Criteria ("mời giáo viên → chấp nhận → xuất hiện trong Đội ngũ") fails at the last step, and it is the only cross-phase acceptance test for Phase 2.
- **Evidence:**
  - `apps/api/internal/features/handoff/service.go:110-113` — `func (s *Service) Reassign(...)` then `if !sc.IsOwner { return nil, apperror.Forbidden(...) }`.
  - Second blocker on the same path: `apps/api/internal/features/handoff/service.go:117` calls `s.classes.Get(ctx, sc, classID)`, which resolves through the own-rows gate (`apps/api/internal/features/classes/repository.go:96-102, 213-225`). An invitee holding no stint yet cannot resolve the class at all → 404 before the owner check would even matter for a `accept-on-behalf` variant run under a member scope.
  - Third blocker: `apps/api/internal/features/handoff/service.go:176-189` `withCenterLock` uses `TryLockCenter` and returns `409` rather than waiting, so even an owner-run accept fails while any import holds the center lock.
  - Plan quote (Phase 2): "**Chấp nhận vai `giao_vien` đi qua `handoff.Service.Reassign`** (`handoff/service.go:110`)".
- **Suggested fix:** Do not call `Reassign` from the invitee's request. Either (a) make accept an owner-scope server-side operation by constructing an owner/system scope inside `classinvites` after verifying `invitation.teacher_id == caller`, or (b) extract the transactional body of `Reassign` into a scope-free internal method and have both `handoff` (owner path) and `classinvites` (self-accept path) call it. Decide and record which, because it changes the Phase 2 wiring and the `router.go` dependency graph.

---

## Finding 2: Migration 000025 aborts in production — the `code` backfill collides, and the stated mitigation misreads UUIDv7

- **Severity:** Critical
- **Location:** Phase 1, "Implementation Steps § A.1" and "Risk Assessment" row 1
- **Flaw:** The backfill derives `code` from the first 6 hex characters of a UUIDv7. In UUIDv7 those characters are the top 24 bits of the 48-bit millisecond timestamp, which change only once every 2^24 ms ≈ 4.66 hours. Every class created in the same ~4.7-hour window inside one center gets an identical code, so `CREATE UNIQUE INDEX uq_classes_center_code` in the same migration fails and the whole migration rolls back. The Risk row rates this "Thấp" and proposes `substr(..., 7, 6)` as "phần random" — characters 7–12 are the *low* 24 bits of the same timestamp, not random. The random bits start at hex character 14.
- **Failure scenario:** Production center created 5 classes in one afternoon session (normal, and guaranteed for any seeded or imported center). `make migrate-up` runs `UPDATE ... 'L' || upper(substr(...))`, then `CREATE UNIQUE INDEX` raises `duplicate key value violates unique constraint`. golang-migrate marks version 25 dirty. Every subsequent migration in the 000025–000032 chain is blocked, and the deploy needs manual `force` plus a hand-written repair.
- **Evidence:**
  - `apps/api/internal/shared/id/id.go:9` — `func New() uuid.UUID { return uuid.Must(uuid.NewV7()) }`, with the package doc at line 1-4 stating "Version 7 keys sort by creation time".
  - `apps/api/migrations/migrations_test.go:112-147` `TestMigrationRoundTrip` runs the full chain; a failing 000025 fails the whole suite too.
  - Plan quote (Phase 1 A.1): "-- Backfill mã lớp cho lớp cũ: 'L' + 6 hex đầu của id (UUIDv7 → gần như không trùng trong 1 trung tâm)". Plan quote (Risk table): "Dùng `substr(replace(id::text,'-',''), 7, 6)` (phần random)".
- **Suggested fix:** Backfill from a per-center sequence instead of the id, e.g. `ROW_NUMBER() OVER (PARTITION BY center_id ORDER BY created_at, id)` formatted as `L###`. It is collision-free by construction and human-readable, which is the point of a class code. If an id-derived code is kept, take hex characters 14+ and still create the unique index in a separate later statement guarded by a `DO $$` retry, never in the same unguarded migration.

---

## Finding 3: Phase 7 "Gỡ chương trình" destroys the class's curriculum, and the gate does not match the capability

- **Severity:** Critical
- **Location:** Phase 7, "Key decisions" bullet 1 (`class_programs` apply/change/remove writes `class_curricula.lessons`); API row `PUT /classes/:id/program` gated `classes.edit`
- **Flaw:** Two separate defects on the same call.
  1. `PutCurriculum` is a whole-list replace with a pointer clamp. "Gỡ" (remove the program) has no defined lesson payload; writing `[]` wipes every lesson title the class ever had and silently resets `current_index` to 0. Even "áp dụng" overwrites a hand-typed curriculum with no backup and no confirmation — the plan only warns about `lesson_plans`.
  2. The route is gated on the center key `classes.edit`, but `PutCurriculum` re-gates internally on the class capability `CapLessonPlanWrite`, whose role map is `giao_vien` only. A `hoc_vu` member holding `classes.edit` passes the route and then gets 403 from inside the service.
- **Failure scenario:** Học vụ opens a class with a 20-lesson curriculum the teacher typed by hand, applies the course's default template by mistake, then presses "Gỡ". `class_curricula.lessons` goes from 20 titles to `[]`, `current_index` from 12 to 0. Sổ đầu bài now shows no lessons; the 12 approved `lesson_plans` rows survive but are orphaned by index with nothing to render against. There is no undo and no audit of the previous list.
- **Evidence:**
  - `apps/api/internal/features/teaching/service.go:93-116` — `PutCurriculum` resolves `authctx.CapLessonPlanWrite`, then `lessons := req.Lessons; if lessons == nil { lessons = []string{} }`, then `currentIndex := min(req.CurrentIndex, len(lessons)-1)` / `max(currentIndex, 0)` and an unconditional `UpsertCurriculum`.
  - `apps/api/internal/shared/authctx/class_staff.go:76-80` — `staffCapabilities` maps `CapLessonPlanWrite: {StaffRoleGiaoVien}`; `apps/api/internal/shared/authctx/class_staff_test.go:58-59` confirms `tro_giang` is false.
  - `apps/api/internal/features/teaching/dto.go:26-29` — `Lessons []string binding:"max=100,dive,max=200"`. A published template with more than 100 lessons, or any lesson title over 200 characters, cannot be mirrored at all through the handler path.
  - `apps/api/internal/features/teaching/model.go:68-71` — "plans reference lessons by index, so editing the list can shift which plan belongs to which title".
  - Plan quote (Phase 7): "Áp dụng / đổi phiên bản / gỡ **ghi đồng thời `class_curricula.lessons`** … **không** đụng `lesson_plans`".
- **Suggested fix:** Define remove as "clear `class_programs`, leave `class_curricula` untouched". For apply/change, require an explicit confirmation when the existing lesson list is non-empty and differs, and preserve `current_index` by title match rather than clamping. Gate the endpoint on `CapLessonPlanWrite` (or call `PutCurriculum` under an owner-equivalent internal scope) so the route gate and the service gate agree. Add a Phase 4 validation that a template version cannot exceed 100 lessons / 200-char titles, or the apply will 422 at the worst moment.

---

## Finding 4: `PUT /classes/:id` is a full replace on an own-rows gate — the Phase 1 ops card silently wipes fields and locks out the staff it targets

- **Severity:** High
- **Location:** Phase 1, R2 / R9 / "Security Considerations" bullet 3; Implementation step 18
- **Flaw:** Three compounding mistakes about the existing update path.
  1. The plan's Security Considerations claims the update goes through `GetWritableByID` (the `class_staff` write port). It does not — `Service.Update` uses `GetByID`, whose filter is `classes.teacher_id = sc.TeacherID` unless the caller is the owner. A `giao_vien` stint holder who is not the row's `teacher_id` gets 404.
  2. The UI gate `canWriteClass` returns true for any `giao_vien` stint, so the toggle renders for users the API will reject. The plan also calls it with one argument; the real signature takes two.
  3. `UpdateClassRequest` is a full replace with `required` on `name`, `start_date`, `default_unit_price`. Adding `Tags`/`Note` in the same style means an omitted `tags` overwrites to empty. Nothing carries a version or timestamp, so two staff on the same detail page clobber each other last-write-wins — now across `name`, price, tags and note rather than just the three original fields.
- **Failure scenario:** Owner renames the class in one tab. Học vụ, on a page loaded two minutes earlier, flips "Cần tuyển sinh". The PUT echoes the stale `name` and `default_unit_price` it read at load, reverting the rename and the price with no warning. Separately, the "Sửa ghi chú" form posts `{name, start_date, default_unit_price, note}` without `tags`, and every tag on the class disappears.
- **Evidence:**
  - `apps/api/internal/features/classes/service.go:233-254` — `Update` calls `s.repo.GetByID(ctx, sc, classID)` and then unconditionally assigns `class.Name`, `class.StartDate`, `class.EndDate`, `class.DefaultUnitPrice`.
  - `apps/api/internal/features/classes/repository.go:96-102` — `scoped` adds `classes.teacher_id = ?` unless `sc.WriteWide()`; `repository.go:213-225` — `GetByID` uses `scoped`, not `writeScoped`.
  - `apps/api/internal/features/classes/dto.go:40-45` — `Name binding:"required,..."`, `StartDate binding:"required,..."`, `DefaultUnitPrice *int64 binding:"required,min=0"`.
  - `apps/web/src/features/roster/lib/class-permissions.ts:10-12` — `canWriteClass(isOwner: boolean, klass: Pick<Class, "my_staff_roles">)`.
  - Plan quote (Phase 1 Security Considerations): "Sửa `recruiting/note/tags` đi qua `GetWritableByID` (write port theo `class_staff`) như `Update` hiện tại — không mở rộng quyền ghi."
- **Suggested fix:** State the real gate in the plan (owner or the row's own teacher), or deliberately migrate `Update` to `GetWritableByID` and accept that as a scoped contract change with its own tests. Make the ops card a narrow `PATCH`-style request where absent means unchanged (pointer fields with explicit nil branches in the service), and add an `updated_at` precondition so a stale page 409s instead of clobbering.

---

## Finding 5: Six phases each bump `CatalogVersion` against a hard-coded test and one shared constant

- **Severity:** High
- **Location:** plan.md D5; Phases 2, 3, 5, 6, 7, 8 (every "Bump `CatalogVersion` + MSW mirror")
- **Flaw:** `CatalogVersion` is a single `const` with a test that asserts the literal value and a message naming the last change. Six phases each incrementing it means six edits to the same line plus six edits to the same assertion. plan.md explicitly says Phases 2 and 3 can run in parallel — both would write `5`, and a merge that keeps one side leaves the API and the test disagreeing, or leaves two distinct permission sets sharing one version number.
- **Failure scenario:** Phase 2 branch sets 5 and updates the test message to "must be 5 after class invites". Phase 3 branch sets 5 and updates it to "must be 5 after library". Git auto-merges the `const` cleanly (identical text) and conflicts only on the comment. Result: catalog generation 5 means two different key sets depending on which migrations ran. Any center whose permission matrix was written under one generation now passes the CAS check against the other, so a stale admin page can save an assignment set that silently drops the keys it never rendered.
- **Evidence:**
  - `apps/api/internal/shared/authctx/catalog.go:334` — `const CatalogVersion = 4`, with the doc at lines 325-333 ending "Bump on any change that alters what a stored assignment means."
  - `apps/api/internal/shared/authctx/catalog_test.go:316-318` — `if CatalogVersion != 4 { t.Fatalf("catalog version must be 4 after adding the tasks group, got %d", CatalogVersion) }`.
  - `apps/api/internal/features/centers/service.go:510-514` — `checkCatalogVersion` rejects a mismatch; `permissions_integration_test.go:596` exercises `authctx.CatalogVersion - 1` as the 409 case.
  - `apps/web/src/test/msw/handlers.ts:16-17` — `export const CATALOG_VERSION = 4;` must track it.
- **Suggested fix:** Bump once, at the end, in a dedicated step in Phase 9 that lands all new keys together, or serialize the bump by making Phase 2 the only one that touches the constant and having later phases add keys under the same generation with an explicit note that no stored assignment meaning changed. Either way add "update `catalog_test.go:317` literal and message" to the phase todo list — no phase currently mentions that file.

---

## Finding 6: No down migrations are specified for Phases 2–8, and the round-trip test demands them

- **Severity:** High
- **Location:** Phases 2–8 "Migration sketch" blocks; plan.md Success Criteria "Migration `000025`–`000032` up/down sạch"
- **Flaw:** Every sketch shows only `CREATE TABLE`/`ALTER TABLE`, no `.down.sql`. `TestMigrationRoundTrip` migrates down to zero and asserts exactly one table remains. Roughly fifteen new tables plus six `classes` columns land across these migrations; a single missing `DROP` fails the assertion. `domainTables` is a hand-maintained literal list that also has to grow.
- **Failure scenario:** Phase 3 lands `program_templates`, `program_template_versions`, `template_lessons` with a down file that drops the first two but forgets the third (or relies on `ON DELETE CASCADE`, which does not drop tables). `make test-api` fails with "want only schema_migrations after full down, got [template_lessons schema_migrations]" — and it fails inside the `migrations` package, not the `library` package, so the phase that broke it is not obvious.
- **Evidence:**
  - `apps/api/migrations/migrations_test.go:134-138` — `require.NoError(t, database.MigrateDown(m, 0))` then `require.Len(t, tables, 1, "want only schema_migrations after full down, got %v", tables)`.
  - `apps/api/migrations/migrations_test.go:26-40` — `domainTables` literal, currently ending at `"task_columns", "tasks"`.
  - `apps/api/migrations/migrations_test.go:112-147` also re-runs up after down and re-asserts, so a down that drops too much fails too.
  - Plan quote (Phase 1 A.1) is the only down spec anywhere: "`.down.sql`: drop 2 index, drop 4 cột." Phases 2–8 contain no `.down.sql` line at all.
- **Suggested fix:** Add an explicit "down: drop X, Y, Z (reverse order), delete backfill rows for step '<label>'" line to every migration sketch, and add "append new tables to `migrations_test.go` `domainTables`" to each phase's todo list. Note also that Phase 1's down drops `classes.code`, `tags`, `recruiting`, `note` — irreversible loss of every hand-entered class code. Say so in the file header the way `000022_task_board.down.sql:1-5` does.

---

## Finding 7: Permission-backfill rollback deletes owner-granted rows, and the backfill misses custom roles

- **Severity:** High
- **Location:** plan.md D5; Phases 3, 5, 6, 8 ("backfill quyền … theo khuôn 000022"); Phase 9 task 6
- **Flaw:** The 000022 pattern's down file deletes the new permission keys **unconditionally** from both `center_role_permissions` and `center_member_permissions`, including rows an owner granted by hand after the feature shipped. The existing file carries an explicit warning that it is only safe before production. Phase 9 plans a production `migrate-up` across all eight migrations with no matching statement about the one-way door. Separately, the up-side backfill only reaches system roles (`WHERE cr.is_system`) and role-less live stints (`role_id IS NULL`); members sitting on a custom role get nothing.
- **Failure scenario:** `library.read` ships. The owner grants it to their custom "Trợ lý học liệu" role by hand over the following week. A later unrelated rollback of one migration step runs `000027_program_templates.down.sql`, which deletes `library.read` and `library.edit` from every role and member row in every center. Rolling forward again re-runs only the default backfill, so the custom role's grants are gone permanently and every member on a custom role now sees an empty "Kho học liệu" with no error.
- **Evidence:**
  - `apps/api/migrations/000022_task_board.down.sql:1-16` — header "CẢNH BÁO: xoá KHÔNG điều kiện cả 5 khoá backfill mặc định lẫn 3 khoá opt-in … Chỉ an toàn để rollback TRƯỚC khi tính năng lên production" followed by unqualified `DELETE FROM center_role_permissions WHERE permission_key IN (...)`.
  - `apps/api/migrations/000022_task_board.up.sql:90` — `WHERE cr.is_system`; line 113-116 area — `WHERE cm.left_at IS NULL AND cm.role_id IS NULL`.
  - `apps/api/migrations/000013_center_rbac.up.sql:15` — `is_system BOOLEAN NOT NULL DEFAULT TRUE` (custom roles set it false).
  - Backfill rows are tracked in `rbac_backfill_rows` under a `step` label (`000022_task_board.up.sql` tail, `000022_task_board.down.sql:18-19`); no phase in this plan mentions `rbac_backfill_rows` or picking a unique step label.
- **Suggested fix:** Give each phase's backfill a unique `step` label and write the down to delete only rows recorded under that label, not by key. Add to plan.md D5 an explicit note that post-production rollback of a permission migration must be a new forward migration. Add a success-criteria row covering a member on a custom (non-system) role, since the current 8-row table in `docs/adding-permissions.md` does not cover it.

---

## Finding 8: The "Buổi học" tab calls an endpoint that requires a bounded range, gates on write, and inserts rows

- **Severity:** High
- **Location:** Phase 1, "Key Insights" bullet 5 and R7; plan.md D7; Phase 7 "Key decisions" bullet 3
- **Flaw:** `GET /classes/:id/sessions` takes `from` and `to` as **required** query parameters capped at 400 days, materialises every missing session row into the database as a side effect, and admits a caller either through the sessions write capability or through a read-only stint path. The plan describes it as a plain listing, specifies no range, and the MSW fixture it points at returns an unbounded array.
- **Failure scenario:** Three distinct breakages. (a) The tab issues `GET /classes/:id/sessions` with no range → 422, and the "Buổi học rỗng hiện câu prototype" test in the matrix passes against MSW while the real app shows an error. (b) A class running longer than 400 days cannot show its full session list at all. (c) An owner opening the detail page of a class that has never been viewed silently inserts hundreds of `class_sessions` rows, which then count in billing and attendance surfaces — a GET with a write side effect, triggered by navigation.
- **Evidence:**
  - `apps/api/internal/features/sessions/handler.go:136-137` — `@Param from query string true` / `@Param to query string true`; lines 153-158 — `from, ok := queryDate(c, "from")` (required form, not `optionalQueryDate` as used at lines 110-114 for `/sessions/pending`).
  - `apps/api/internal/features/sessions/handler.go:132` — "Generates any session rows missing for [from, to] … Range is capped at 400 days".
  - `apps/api/internal/features/sessions/service.go:143-168` — builds candidate rows then `s.repo.BulkInsertIgnoreConflicts(ctx, candidates)` inside the listing path.
  - `apps/api/internal/features/sessions/service.go:106` — `ListRange` resolves `GetWritable(..., authctx.CapSessionsWrite)`; `service.go:191-200` — `ListRangeReadable` splits by role.
  - The D7 "NGUỒN" derivation is also unreliable: generated rows copy `sw.StartTime` from the schedule (`service.go:149-151`) while ad-hoc rows accept a caller-supplied `start_time` (`service.go:348-350, dto.go:24`). After a schedule is replaced (`effective_to` closed, a pattern the code explicitly documents at `repository.go:58-61` / `dto.go:47-49`), historical generated sessions match no effective schedule and would be labelled "Thêm tay"; an ad-hoc session typed at the class's usual time is labelled "Lịch tuần".
  - Plan quote (Phase 1): "tab Buổi học = `GET /classes/:id/sessions` (fixture MSW đã có)". Plan quote (D7): "cột 'NGUỒN' suy từ khớp `start_time` với schedule".
- **Suggested fix:** Specify the range the tab sends (class `start_date` to `end_date`, clamped to 400 days, with paging or a term selector beyond that) and say in the phase that the call materialises rows. For NGUỒN, match against schedules effective **on the session's own date** rather than today, and accept that ad-hoc sessions at the habitual time are indistinguishable — or drop the column, since D7's premise ("thêm cột không mang giá trị mới") is what makes it unreliable.

---

## Finding 9: Phase 2's cascade assumption is wrong — leaving a center does not delete the member row

- **Severity:** Medium
- **Location:** Phase 2, "Risks" bullet 2 and "Migration sketch" FK on `center_members`
- **Flaw:** Leaving a center is a soft operation that sets `left_at`; the `center_members` row survives. The `ON DELETE CASCADE` the plan relies on therefore fires only on a hard teacher delete. A pending invitation to a departed member is not cleaned up, and the plan's accept path checks only `teacher_id == caller`, never live membership.
- **Failure scenario:** Owner invites M as `tro_giang`, then removes M from the center. M's session is still valid. M presses "Nhận lớp"; the non-`giao_vien` branch inserts straight into `class_staff` with no membership check, giving a removed member an active stint on a class. If the role were `giao_vien`, the `handoff` path would instead be a silent success-with-no-effect, because `SyncPrimaryTeacher` is explicitly a no-op for a kicked member — the UI reports "đã nhận lớp" while nothing changed.
- **Evidence:**
  - `apps/api/internal/features/centers/repository.go:481` — `UPDATE center_members SET left_at = now()`; `repository.go:453` — rejoin is `DO UPDATE SET left_at = NULL`, confirming the row is never deleted.
  - `apps/api/migrations/000007_centers.up.sql:66-75` — `PRIMARY KEY (teacher_id, center_id)` and `uq_center_members_active ON center_members(teacher_id) WHERE left_at IS NULL`.
  - `apps/api/internal/features/classstaff/repository.go:216-224` — `SyncPrimaryTeacher` … "Both statements are conditional on the target's LIVE membership: a kicked member's sync must be a complete no-op".
  - Plan quote (Phase 2 Risks): "Người được mời rời trung tâm → FK cascade xoá lời mời (chấp nhận)."
- **Suggested fix:** Check `center_members.left_at IS NULL` in the accept/decline service before any write, and return a clear 409 on a stale invitation. Add an explicit expiry or a cancel-on-leave rule rather than depending on a cascade that will not fire. Also fix the risk note so the next reader does not inherit the wrong model.

---

## Finding 10: `CountReadableByPhase` will fail `scopelint`, which the plan asserts it satisfies

- **Severity:** Medium
- **Location:** Phase 1, Implementation step B.7
- **Flaw:** The plan justifies calling `CenterWideFor` inside a repository method by claiming the method name contains `read`. The linter splits names on camelCase boundaries and compares whole tokens case-insensitively against `read`. `CountReadableByPhase` yields `Count`, `Readable`, `By`, `Phase` — none equal `read`. The existing code satisfies the rule by calling `CenterWideFor` only inside the helper `readScoped`, never in the public `ListReadable`.
- **Failure scenario:** The implementer follows the step literally, `make scopelint` fails with "CenterWideFor may only widen a read-named function", and the fix is a rename or restructure discovered at verification time instead of at planning time.
- **Evidence:**
  - `apps/api/tools/scopelint/scopelint/analyzer.go:431-438` — `hasReadToken` loops `splitCamelTokens(name)` and requires `strings.EqualFold(tok, "read")`; lines 440-462 define the split.
  - `apps/api/tools/scopelint/scopelint/analyzer.go:490` — `pass.Reportf(..., "CenterWideFor may only widen a read-named function; widen writes with Scope.WriteWide() instead")`.
  - `apps/api/internal/features/classes/repository.go:124-131` — `readScoped` is where `CenterWideFor(authctx.PermClassesViewAll)` actually lives; `repository.go:255-257` — `ListReadable` merely delegates.
  - `Makefile:72-73` — `scopelint: go run ./tools/scopelint ./internal/...`, and `test-api-unit` depends on it, so this blocks the fast test target.
  - Plan quote (Phase 1 B.7): "(tên hàm chứa `read` để `scopelint` cho phép `CenterWideFor`)".
- **Suggested fix:** Build the stats query on the existing `readScoped(ctx, sc)` helper instead of re-deriving the scope, matching `ListReadable`. Correct the parenthetical in the plan so it does not teach a wrong rule to the next phase.

---

## Verification Results

Flow Tracer — traced paths, all read directly from source.

**(a) `handoff.Service.Reassign` — TRACED.** `apps/api/internal/features/handoff/service.go:110-170`. Refuses non-owners at line 111. Reads the class through the own-rows gate at line 117. Runs `ReassignTeacher` + `sessions.ReassignPlanned` + `staff.SyncPrimaryTeacher` inside `withCenterLock` (lines 147-160), which is a transaction holding the center advisory lock and returning 409 rather than waiting (lines 176-189). Same-teacher call is a `SyncPrimaryTeacher`-only repair path (lines 122-135). **Not reusable from `classinvites` under an invitee's scope** — see Finding 1.

**(b) `teaching.PutCurriculum` and `lesson_plans` keying — TRACED.** `apps/api/internal/features/teaching/service.go:89-116`. Gate is `CapLessonPlanWrite` via `resolveClass` → `ClassSource.GetWritable` (`service.go:551-553`), and `CapLessonPlanWrite` maps to `giao_vien` only (`apps/api/internal/shared/authctx/class_staff.go:76-80`). Whole-list replace, nil becomes `[]`, `current_index` clamped into the new range. Binding caps at 100 lessons / 200 chars (`dto.go:26-29`). `lesson_plans` is keyed by `(class_id, lesson_index)` with no title reference (`model.go:90-93`), and the model comment at `model.go:68-71` states that editing the list shifts which plan belongs to which title.

**(c) `GET /classes/:id/sessions` generation and `start_time` matching — TRACED.** Route `apps/api/internal/features/sessions/routes.go:10` → `handler.listRange` (`handler.go:128-163`), `from`/`to` required, 400-day cap. `Service.ListRangeReadable` (`service.go:191+`) → `materialiseRange` (`service.go:119-186`), which expands schedules, copies `sw.StartTime` onto generated rows (lines 148-152), and calls `BulkInsertIgnoreConflicts` (line 168) before listing. Ad-hoc creation takes `start_time` from the request (`service.go:348-350`, `dto.go:24`), so `start_time` alone cannot distinguish the two origins. **Matching is possible but not reliable** — see Finding 8.

**(d) `pagination.Parse` + `response.List` — TRACED.** `apps/api/internal/shared/pagination/pagination.go:36-60` — `page`/`per_page` clamped (max 100, max page 1,000,000), unknown `sort` keys fall back to the default rather than erroring. `Scope` applies limit/offset/order (lines 66-76); `Meta` builds the envelope block (line 79+). `apps/api/internal/shared/response/response.go:49-52` — `List` always emits `meta`; a stats object must use `OK` (lines 45-47) instead. Note the fallback behaviour contradicts Phase 1's plan to 422 on unknown `weekday`/`shift`/`phase`; that is a defensible choice but it is a new convention in this handler, worth one sentence in the plan.

**(e) `useUpdateClass` invalidation and MSW under `onUnhandledRequest: "error"` — TRACED.** `apps/web/src/features/roster/hooks/use-classes.ts:48-57` invalidates `classesKeys.lists()` and `classesKeys.detail(id)` only. `useCreateClass` (lines 38-46) invalidates `lists()` only — Phase 1 step 14 adds a stats invalidation to `useUpdateClass` but **not** to `useCreateClass`, so the chip counts go stale immediately after "+ Lớp học" creates a class, which contradicts Phase 1 Success Criteria bullet 1. `apps/web/src/test/setup.ts:14` — `server.listen({ onUnhandledRequest: "error" })`, so any test rendering a tree that calls `useClassStats` fails until `handlers.ts` gets the `GET /classes/stats` handler; the plan does add it. The shared `makeClass` fixture (`apps/web/src/test/msw/handlers.ts:487-511`) is used across roster and teaching suites; `classSchema` (`apps/web/src/features/roster/schemas/roster-schemas.ts:177-190`) is a plain `z.object` with no `.strict()` and the file has zero `strict()` calls, so adding `.default()` fields is safe for existing fixtures — the Phase 1 "Trung bình" rating on that risk is, on this evidence, over-stated.

**(f) migration tests — TRACED.** `apps/api/migrations/migrations_test.go` (build tag `integration`) and `apps/api/migrations/backfill_parity_test.go` (no build tag, pure text check). `TestMigrationRoundTrip:112-147` asserts `domainTables` present after up, exactly one table after down-to-zero, and presence again after re-up. `backfill_parity_test.go:20-62` pins frozen literal key lists for 000018 only and guards marked SQL blocks; it does **not** derive from the live catalog, so it will not automatically cover the new phases' backfills. `apps/api/internal/shared/authctx/catalog_test.go:316-318` hard-codes `CatalogVersion != 4`.

---

Status: DONE_WITH_CONCERNS
Summary: Ten evidence-backed failure modes, three Critical — the Phase 2 accept path calls an owner-only method and cannot work as designed, the Phase 1 class-code backfill will abort migration 000025 on real data, and the Phase 7 program apply/remove destroys `class_curricula` while gating on a permission the underlying service does not accept.
Concerns/Blockers: Findings 1, 2 and 3 invalidate the stated design of their phases and need a decision before execution, not a re-scout. Findings 5, 6 and 7 are cross-phase process gaps that will surface as CI failures or one-way production doors unless plan.md records them now.
