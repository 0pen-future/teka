# Code Review — feat/unified-students-page

Date: 2026-09-29 (Asia/Saigon). Read-only review. Scope: `git diff master...HEAD` (9e3481b, 4ad86ac, eaf6455) plus uncommitted working tree (sidebar, layout tests, e2e specs, frontend-guidelines).

## Code Review Summary

### Scope
- Backend: `shared/authctx` (PhoneVisible), `classscope` fragment removal, students/contacts/statements/collections/notifications repos + services, zalo service, new `collections` contact-balances endpoint + routespec + integration test.
- Web: `students-page.tsx`, `contacts-tab.tsx`, `roster-table.tsx`, `contact-detail-page.tsx`, `student-detail-page.tsx`, `contacts-redirect.tsx`, `routes.tsx`, `use-collections.ts`, `collections-schemas.ts`, `message-card.tsx`, `notifications-page.tsx`, `dashboard-layout.tsx` (+ test), 8 e2e specs, `docs/frontend-guidelines.md`.
- Focus: permission/tenancy leaks, correctness, regressions, AC gaps.

### Verification run
- `go build ./...`, `go vet ./...`, `go test ./...` (unit, no integration tag): pass.
- `npm run typecheck`: pass. ESLint on touched areas: 0 errors, 5 pre-existing warnings (react-hooks/incompatible-library).
- Vitest (roster, layouts, collections): 329 pass, 11 fail — all in `roster-import-page.test.tsx` / `use-enrollments.test.tsx` import flow (`Cannot read properties of undefined (reading '_buffer')` at `imports-api.ts:52`). Fail in isolation too; neither tests nor import sources changed on this branch -> pre-existing env issue (Node 24 + MSW multipart), not this branch.
- NOT run per constraint: API integration tests (`make test-api`), Playwright e2e.

### Overall Assessment
Backend phone gating is correct and consistently reduced to one rule (`contacts.view_all`, implied by `reports.send`). No remaining references to removed fragments (`PhoneVisibleVia`, `HasActiveHocVu`, `ReachableContactPhones`). Bind-arg counts match rewritten SQL (e.g. notifications `listByPeriodQuery` 8 placeholders / 8 args). New contact-balances endpoint is double-gated (route + service), center-scoped, and well covered by integration tests. No tenancy or phone leaks found. One e2e spec edit will fail; several medium correctness/UX gaps.

### Critical Issues
None.

### High Priority
1. **e2e `courses.spec.ts` will fail** — `apps/web/e2e/courses.spec.ts:117-119`. Spec now creates the class from `/classes` and asserts `classRow.getByText("150.000 ₫/buổi")`. `class-table.tsx` has no unit-price column (STT, Lớp học, Lịch học, Gắn thẻ, Trạng thái, Ngày bắt đầu, Ngày kết thúc, actions; grep for `buổi|unit_price` returns nothing). The removed `classes-tab.tsx` rendered `formatMoney(default_unit_price)/buổi`. AC9 (edited e2e green) is therefore not met.
   Fix: assert inheritance elsewhere — e.g. open the class detail and check its unit price, or keep the dialog assertion at line 112 (`toHaveValue("150.000")`) and drop line 119.

### Medium Priority
1. **Unstable pagination order for "Xem thêm"** — `apps/api/internal/shared/pagination/pagination.go:66-74` orders by a single column; `students/handler.go:139` and `contacts/handler.go:104` default to `full_name`. The new page concatenates OFFSET pages via `useInfinite`, so ties on `full_name` (common in Vietnamese names) can duplicate rows across pages (duplicate React keys `student.id` / `contact.id`) or skip rows. Fix: append a unique tiebreaker in `Scope` (`db.Order(col+dir).Order(<table>.id ASC)`), or have the handlers pass one.
2. **Stale data shown under a new tab/month (`keepPreviousData`)** — `apps/web/src/features/roster/hooks/use-students.ts:32,50`, `use-contacts.ts:29,46`, `apps/web/src/features/collections/hooks/use-collections.ts` (monthly balances). On tab switch the previous tab's rows (and the count in `tabLabel`) render under the new tab — e.g. "all" rows appear under "Chưa vào lớp" with owner "Ghi danh vào lớp" buttons (`roster-table.tsx:69-73`) until refetch. Changing `?month=` shows the previous month's debt figures against the new month (`contacts-tab.tsx:51-53`). Fix: drop placeholder data when the filter identity changes (compare `isPlaceholderData` and render loading/dim state, or key placeholder on same-tab only).
3. **Enrollment write UI removed for non-owners (confirm intent)** — `apps/web/src/features/roster/pages/student-detail-page.tsx:87-89,105-107` now gate "Ghi danh vào lớp"/"Kết thúc ghi danh" on `isOwner`. API still allows the class's active teacher (`enrollments/service.go:143`) and default member grants include `enrollments.*`. There is no other enroll surface for teachers, so this is a functional loss vs master. Matches plan AC3 (user decision) — informational, but flag to the user. Related: class senders using `zalo_manual` now receive `phone: null` (`notifications/service.go:328`) and cannot reach families without `contacts.view_all` — intended, UX impact.
4. **Stale API doc** — `docs/api-guidelines.md:174-175` still documents `statements.TargetContacts(ctx, a Anchor, viewer Scope, periodID)` "judging phone_visible for the viewer"; signature is now `(ctx, a, periodID)` and phone visibility is no longer per-viewer there.

### Low Priority
- Role with `contacts.view_all` but without `students.list` loses all contacts UI: `/contacts` -> `/students` -> `/`. Also `contacts.view_all` does not imply route keys `contacts.list`/`contacts.read`; a revoked `contacts.list` would show the tab and then 403. Unlikely with default grants.
- `/contacts/:id` highlights no sidebar entry (`dashboard-layout.tsx` nav matching).
- `StudentDetailPage` has no `students.read` shell guard (API still gates; UX only).
- `ContactsRedirect` (preserves `q`) has no unit test; only e2e coverage.
- `notifications-page.tsx:502` "Kết bạn trước" links to `/students?tab=contacts`, which a class sender without `contacts.view_all` cannot see (tab falls back).
- `docs/frontend-guidelines.md` "each gated on their own key" is imprecise — three tabs share `students.list`.
- By-class tab: teacher with zero classes gets no empty state.
- `contacts-tab.tsx:16-19` `thisMonth()` uses browser-local time; fine for Asia/Saigon users, off-by-one at month boundary for other TZs.

### Edge Cases Found by Scout (verified non-issues)
- Contacts search by phone: blocked because `scopedRead` applies `Where("false")` when `!PhoneVisible`.
- Collections search: name only. `BulkText` carries no phone. FriendMatch echoes only caller-supplied phones. Import template doesn't read DB. Teacher/invitation phones are staff phones.
- `ZaloMappings` returns empty unless `PhoneVisible`; all non-class callers already behind `ReportsOversight` (no regression). `MatchFriendsScoped` 403 unless `PhoneVisible`.
- Contact balances: opening balances are per teacher anchor (`billing/repository.go:650`), so summing across teachers does not double count; `HAVING <> 0`, center/year/month filtered; route + service both check `billing.view_all`.
- `bulkSendRow.phone` now nullable in schema and `message-card.tsx` renders conditionally — contract change handled on the client.

### Positive Observations (risk calibration)
- Integration test `contact_balances_integration_test.go` covers two teachers, month isolation, center isolation, settled families, 403/grant, 422/400.
- Route-policy snapshot updated alongside `routespec.go:345`.

### Recommended Actions
1. Fix `courses.spec.ts:119` assertion (High).
2. Add id tiebreaker to pagination ordering (Medium).
3. Suppress cross-tab / cross-month placeholder data (Medium).
4. Confirm with user that teachers losing enroll/end-enrollment UI is intended (Medium, decision).
5. Update `docs/api-guidelines.md:174-175` (Medium).
6. Run `make test-api` and the edited e2e specs once Docker is free (AC9).

### Metrics
- Type coverage: typecheck clean.
- Test coverage: not measured; new unit tests on layout + students page; backend integration tests added.
- Lint: 0 errors, 5 pre-existing warnings.

### Plan follow-ups
- Phase files marked completed, but `plans/260929-1038-unified-students-page/plan.md` still shows every phase "pending" — lead should reconcile.
- AC1-8: substantially met (notes above). AC9: not verified here; `courses.spec.ts` will fail.

### Unresolved Questions
- Is removing teacher-side enrollment create/end from the UI intended given the API still permits it?

Status: DONE_WITH_CONCERNS
Summary: Phone/permission gating and the new contact-balances endpoint are correct with no tenancy or phone leaks found; one edited e2e spec will fail, and pagination ordering plus stale placeholder data need fixes.
Concerns/Blockers:
1. High — apps/web/e2e/courses.spec.ts:119 asserts "150.000 ₫/buổi" in the /classes row; class-table has no price column -> spec fails (AC9).
2. Medium — apps/api/internal/shared/pagination/pagination.go:73 single-column ORDER BY (full_name via students/handler.go:139, contacts/handler.go:104) -> duplicate/skipped rows with "Xem thêm".
3. Medium — apps/web/src/features/roster/hooks/use-students.ts:32,50 / use-contacts.ts:29,46 / collections use-collections.ts monthly balances: keepPreviousData shows prior tab rows/count and prior month debt under new selection.
4. Medium — apps/web/src/features/roster/pages/student-detail-page.tsx:87,105 enroll/end-enrollment owner-only; teachers lose functionality API still allows (confirm per AC3).
5. Medium — docs/api-guidelines.md:174-175 stale TargetContacts signature/viewer description.
6. Low — contacts.view_all without students.list loses contacts UI; /contacts/:id no nav highlight; no ContactsRedirect unit test; notifications-page.tsx:502 link to hidden tab; guidelines wording; no empty state for teacher with no classes.
