## Relevant Files

1. **apps/web/src/layouts/dashboard-layout.tsx** — Định tuyến sidebar chính, cách quản lý permissions, active state
   - 37-49: NavEntry type definition (label, to, Icon, pending, perm)
   - 51-55: NavGroup interface
   - 57-66: Comment khái quát cấu trúc nav, CurrentPeriodDisc, period-scoped routes
   - 67-191: useNavGroups() hook tạo entry list
   - 104-108: Entry "Lớp cần tuyển sinh" → /classes/recruiting (classes.list perm)
   - 115-121: Group "Kho học liệu" với 3 entry (Lộ trình học, Khóa học, Kho học liệu)
   - 135-139: Entry "Gửi thông báo" → /notifications/{periodId} (reports.send perm, universal)
   - 162-164: Entry "Gửi báo cáo" → /reports (reports.send perm, Trung tâm group, chỉ non-owner)
   - 199-218: OVERFLOW_LABELS Set gồm "Lớp cần tuyển sinh", "Gửi thông báo", "Gửi báo cáo", "Kho học liệu"
   - 225-242: OVERFLOW_PATH_PREFIXES array (mobile sheet detection)
   - 258-272: useNavActive(to) — active state logic, "most specific wins" (parent nhưng sibling deeper destination sẽ override)
   - 478-493: CurrentPeriodDisc component — hiện tại KO gated, chỉ render nếu period exist
   - 644: CurrentPeriodDisc rendered trên rail (md–lg breakpoint)
   - 188: Permission filtering in useNavGroups: entries filtered nếu perm và caller doesn't have it

2. **apps/web/src/layouts/__tests__/dashboard-layout.test.tsx** — Test covering sidebar changes
   - 585-601: Test "Lớp cần tuyển sinh" entry — nằm giữa Danh mục lớp và Lời mời, active state "most specific wins"
   - 504-549: Tests "Gửi báo cáo" — visible chỉ non-owner + reports.send, hidden from owner
   - 647-725: "Kho học liệu" group tests — header + 3 entries, gates on individual keys
   - 204: "Lớp cần tuyển sinh" in OVERFLOW_LABELS
   - 179: "Gửi thông báo" in sheet (period-scoped)
   - 210-211: "Gửi báo cáo" in OVERFLOW_LABELS

3. **apps/web/src/features/reports/routes.tsx** — Route handler
   - 14-15: path "reports", handle title "Gửi báo cáo"
   - 10: Comment: no route guard; API enforces reports.send on nav entry

4. **apps/web/src/features/reports/pages/send-reports-page.tsx** — Page nội dung
   - 47: `<h1>Gửi báo cáo</h1>`
   - 48-50: Subtitle nói tới chọn kỳ giáo viên → /notifications/:periodId

5. **apps/api/internal/features/billing/repository.go** — API layer
   - 104-115: GetPeriodRead, ListPeriodsRead — center-scoped với billing.view_all (owner + explicit grant + reports.send holder)
   - Comment line 109-113 explains reports.send holder sees all teachers' periods

6. **apps/api/internal/shared/authctx/catalog.go** — Permission catalog
   - 386-387: impliedKeys: reports.send → {billing.view_all, statements.view_all, notifications.view_all, contacts.view_all}

7. **apps/web/src/features/roster/routes.tsx** — Class routes
   - 48-50: /classes → ClassListPage (title "Danh mục lớp")
   - 52-59: /classes/recruiting → RecruitingClassListPage (title "Lớp cần tuyển sinh")

8. **apps/web/src/features/roster/pages/class-list-page.tsx** — Variant logic
   - 21-39: ClassListVariant ("all" | "recruiting"), variantCopy object với title/subtitle/path
   - 62: recruitingPage = variant === "recruiting"
   - 65: view đảo chuyển từ "recruiting" → "all" trên recruiting page
   - 195-198: export RecruitingClassListPage = ClassListPage variant="recruiting"

9. **apps/web/src/features/roster/hooks/use-class-list-url-state.ts** — URL state
   - 7: classListViews = ["all", ...classPhases, "recruiting"]
   - 20: view schema mặc định "all", catch invalid → "all"

10. **apps/web/src/features/roster/components/class-status-chips.tsx** — Filter chips
    - 11-12: includeRecruiting param (default true)
    - 36-38: Recruiting chip render conditionally

11. **apps/web/src/features/roster/components/class-detail-header.tsx** — Back link
    - 22-32: backTarget() — nhận state.from === "/classes/recruiting" → return path + label

12. **apps/web/src/features/collections/routes.tsx** — Notifications route
    - 21-22: path "notifications/:periodId", title "Gửi thông báo"

13. **apps/web/e2e/helpers/ux-routes.ts** — E2E routes
    - 46: "classes-recruiting" path "/classes/recruiting"
    - 61: "reports" path "/reports"

14. **apps/web/e2e/secretary-send.spec.ts** — E2E scenarios
    - 41: "Quyền Gửi báo cáo học phí" permission dialog label
    - 101: secretary thấy link "Gửi báo cáo"
    - 106: Page heading "Gửi báo cáo"
    - 180: Teacher KO thấy "Gửi thông báo →" (footer link on billing page)

## Patterns

- **Entry filtering**: useNavGroups() dùng isResolved + has(perm) để ẩn entry ngoài timeframe resolve
- **Conditional rendering**: isOwner + has() điều kiện conditional array spread (lines 162-168, 166-178)
- **Active state**": useNavActive() implements "most specific wins" — deeper sibling entry overrides parent (line 271)
- **Period-scoped links**: Built once useCurrentPeriod() resolves, mặc dù to=null cho đến lúc đó
- **Variant pattern**: ClassListPage takes variant prop; RecruitingClassListPage wrapper exports specialized view
- **Back-link honor list**: class-detail-header checks state.from against known list paths
- **Permission implication**: reports.send chứa billing.view_all, statements.view_all, notifications.view_all, contacts.view_all
- **Group header suppression**: useNavGroups() filters out group headers nếu tất cả entry bị lọc (line 190)

## Risks

- **Active state concern**: Merging /reports và /notifications/:id vào một entry cần cẩn thận với active state — nếu entry to=/reports nhưng URL /notifications/:id, cần thêm /notifications/* vào OVERFLOW_PATH_PREFIXES hoặc dùng match-based logic
- **Title ambiguity**: Route handle title "Gửi báo cáo" (reports/routes.tsx:15) vs page h1 "Gửi báo cáo" (send-reports-page.tsx:47) hiện match. Nếu merge, title sẽ là "Gửi thông báo"? Cần confirm intent
- **E2E references**: secretary-send.spec.ts:101, :106 references "Gửi báo cáo" link/heading — đổi tên vào merge sẽ break test
- **Back-link branches**: Nếu remove /classes/recruiting route, class-detail-header:27-29 logic ngừng cần
- **Owner period visibility**: Hiện owner sees all periods qua Học phí group entries (Chốt sổ, Gửi thông báo, Thu tiền). Merge reports.send vào /reports cần confirm owner appears there (route comment line 10 says API answers plain member with own periods, nhưng ListPeriodsRead center-scoped cho owner)
- **CurrentPeriodDisc permission**: Ko gated hiện. Nếu gate với has("billing.read"), cần verify toàn bộ rail layout KO shift (644 gọi nó directly trong aside)
- **Kho học liệu naming**: Group header = "Kho học liệu", child entry label = "Kho học liệu" — đặc biệt nhất quá. OVERFLOW_LABELS line 208 include child. Cần đặt tên group khác (e.g. "Tài liệu giáo dục", "Kho tài nguyên", "Thư viện giáo dục")

## Unresolved Questions

- None — đủ evidence từ code. Cần clarify product decision cho E:
  1. Merged entry title: "Gửi thông báo" hay giữ "Gửi báo cáo" cho /reports?
  2. Group "Kho học liệu" đặt tên mới là gì?
  3. CurrentPeriodDisc gated logic: chỉ render khi has("billing.read")?

Status: **DONE**

Summary: Scouted full navbar, route, permission, active-state logic và e2e test references cho 5 changes (remove recruiting sidebar + route, merge reports entries, keep role permission, gate CurrentPeriodDisc, rename group). Tất cả file + line evidence recorded; risks recorded (active state matching, E2E breakage, owner period visibility, permission gating).
