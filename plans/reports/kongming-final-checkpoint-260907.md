# Kongming — checkpoint cuối: go/no-go phase 4, merge-readiness, commit slicing, owner message, deploy-day

Ngày: 2026-09-07. Plan: `plans/260906-0627-authz-write-scope-root-cause/`.
Advisory only. Mọi khẳng định có sức nặng dưới đây đã đối chiếu trên cây
working tree chưa commit (file:line), 5 phase file, 5 review, 4 tester report,
checkpoint trước (`kongming-phase3-5-checkpoint-260907.md`) và các lệnh chạy
thật (`go build ./...`, `go vet`, `go test -count=1 ./tools/...`,
`go test -short` trên `shared/`, `middleware/`, `audit/`, sinh swagger ra
scratchpad để so drift, `git apply --check` các snapshot patch). Chạy trên
`fable`.

## TL;DR

1. **Phase 4: GO làm guard tenancy thường trực.** H1 (fail-open khi đổi tên
   file) đóng bằng nhận diện receiver theo hình dạng kiểu + sàn 18 package
   trong `tree_test.go:22`; H2/M2/M4/L1/L2 đã vào code và có testdata đỏ→xanh;
   doc comment nói rõ witness là "được nhắc tới", RLS là backstop
   (`analyzer.go:44-48`). CI **có** cưỡng chế: job `test` chạy `make test-api`
   → `go list ./...` gồm `tools/scopelint/scopelint` (có `TestGoFiles` +
   `XTestGoFiles`) → `tree_test.go` không gate `-short`/build tag. Job `lint`
   gọi `golangci-lint-action` trực tiếp nên prerequisite `lint-api: scopelint`
   chỉ chạy local — đúng như thiết kế, không cần đổi workflow.
2. **Toàn plan: MERGE-READY VỚI ĐIỀU KIỆN** — code xanh (build/vet/tools/lint,
   full suite 77.8% theo tester, swagger không drift), tiêu chí kỹ thuật đạt,
   nhưng còn 3 việc nhỏ trước commit (nhãn finding code trong comment, plan.md
   chưa cập nhật, một comment test nhắc "decision D5") và **2 quyết định owner
   (D9, OQ6) phải có câu trả lời trước khi merge** — không phải trước khi
   commit/mở PR.
3. **Commit slicing khả thi và đã kiểm chứng**: hai snapshot
   `phase12-tracked.patch` và `phase35-tracked.patch` đều là diff tích luỹ từ
   master, apply sạch; tôi đã sinh `p12-to-p35.patch` và chứng minh
   `--exclude`/`--include` tách được phase 3+H-1 khỏi phase 5 (kết quả
   byte-identical với snapshot 03:36). 5 commit, dùng `git apply --cached`
   nên working tree không bị chạm.
4. **Rủi ro chưa ghi**: cây 90 file sửa + 6 untracked chưa từng chạy CI; phase
   1 không còn tách được để ship riêng như Rollout §1 mô tả (chấp nhận được vì
   kiểm kê prod = 0 holder `view_all`); dữ liệu prod có thanh toán của member
   bị bỏ sót allocation (H-1) — fix chỉ áp cho ghi mới, cần chạy
   `POST /payments/:id/allocations/auto` cho các payment cũ sau deploy.

## Bằng chứng đã kiểm

| Mục | Trạng thái |
|---|---|
| `go build ./...`, `go vet ./tools/... ./internal/shared/...` | xanh |
| `go test -count=1 ./tools/...` | xanh, 4.37s (`TestAnalyzer` + `TestNoDiagnosticsOnRepositoryTree`) |
| `go test -short` `internal/shared/...`, `middleware`, `features/audit` | xanh (`./internal/features` báo "no Go files" vì thư mục chỉ còn package con — `./...` bỏ qua bình thường) |
| Swagger drift | sinh lại ra scratchpad: `swagger.json`/`swagger.yaml` identical, `docs.go` chỉ khác tên package do output dir — không drift; diff trên cây chỉ là 404 mới của `mark-sent` (phase 3) |
| `git stash list` | rỗng; `git status` 117 dòng, khớp danh sách tester |
| `scopelint:unscoped` trên cây | 0 |
| `IsOwner:\s*true` non-test ngoài `centers/` | 0 |
| `authctx.Scope{` có field non-test ngoài 5 package miễn | 0 |
| `TestViewAllWidens*` | 9 file (attendance, billing, classes, enrollments, notifications, payments, sessions, statements, students) |
| `ReportsOversight()` non-test | 7 site, đều gate gửi (`statements/service.go:110,347`, `contacts/repository.go:113`, `notifications/service.go:129,398,499,678`) |
| D3 giữ nguyên | `sessions/service.go:211` `canGenerate := sc.CenterWideFor(PermSessionsViewAll)` |
| D9 trạng thái code | `payments/service.go:54` `ResolveContactAnchor` center-only; `Reverse`/`Reallocate`/`AutoAllocateRemainder` (`reversal.go:88,207,298`) qua `LockPayment` → `writeScoped` (`repository.go:167-170`, `WriteWide()` = owner); `readScoped` (`:158-162`) narrow theo `payments.teacher_id` → member không `payments.view_all` không GET lại được |
| OQ6 trạng thái code | `students/service.go:64-68` `Create` → `checkContact` rồi anchor `sc.Self()`; test `students/integration_test.go:282` vẫn khẳng định "member create must anchor to the member" |
| D6/L4 | `zalo/service.go:492-494` `MatchFriendsScoped` mở khi `CenterWideFor(contacts.view_all)` (gồm key suy ra từ `reports.send`); `impliedKeys` (`catalog.go:318`) = billing/statements/notifications/contacts `view_all`, không payments |
| Checkpoint trước Q3 | tuân thủ: helper theo hình dạng (`analyzer.go:278-298`), witness = helper hoặc selector `CenterID` (`:396-425`), R2 không cấm `.Table(`, R3 type-based, directive chỉ miễn R1, tree test qua `checker.Analyze`, `go.mod` chỉ chuyển `x/tools` sang direct, `go.sum` không đổi, guard cũ đã xoá |
| Checkpoint trước Q4 "không được làm" | tuân thủ: grading/classstaff chỉ đổi comment (3+/2−), không thêm Scope param, không soi service, không đổi tên helper, không đụng routespec trong phase 4, không chốt OQ6/D9 trong analyzer |
| Phase 4 file set | đúng như báo cáo: từ snapshot 03:36 chỉ khác `Makefile`, `apps/api/CLAUDE.md`, `go.mod`, `docs/{adding-permissions,api-guidelines}.md`, 2 comment repo, xoá guard, + `tools/` untracked |

## (1) Go/no-go phase 4 — GO

Spot-check H1: `findRepositoryReceiver` (`analyzer.go:145-170`) duyệt
`pass.Pkg.Scope().Names()` (sorted) tìm struct có field `*gorm.DB`, chỉ trong
package dưới `internal/features` (`isFeaturesPkg`, `:135`); `centers` miễn R1
(`:97`). Sàn: `minRepositoryPackages = 18` (`tree_test.go:22`), cây thật có 21
package chứa `*gorm.DB` (đếm độc lập bằng grep, khớp comment). Counter là
`atomic.Int64` reset trước `checker.Analyze` — an toàn với phân tích song
song. Testdata `otherRepo` chứng minh nhiều candidate trong một package không
làm lệch (`testcase/repository.go` cuối file).

Spot-check L2: `hasReadToken`/`splitCamelTokens` (`:431-464`) tách camelCase +
`_`, so token đúng "read"; testdata `alreadyClosed` flag, `readSomething` pass.
10 caller thật của `CenterWideFor` (`readWide`, `readScoped`,
`readNarrowContacts`, `scopedRead`, `GetPeriodStatusRead`, `readScopedFeed`,
`readNarrow`, `readScopedSchedules`, `contactsReadWide`, `readNarrowNames`)
đều có token `read` — tree test 0 diagnostic xác nhận.

Doc comment: `analyzer.go:44-48` "A witness proves the scope was mentioned
somewhere in the method, not that every query chain in it applied it… row-level
center_id/teacher_id predicates remain the backstop" — mirror ở
`docs/api-guidelines.md:135-139`. Đạt yêu cầu.

Hai câu hỏi mở của reviewer — quan điểm của tôi:

- **Narrowing helper "rửa" scope** (M1 bullet 3): đồng ý chấp nhận là non-goal.
  Không thể ép narrowing helper tham chiếu `CenterID` vì các helper hợp lệ như
  `readNarrow(q, sc, col)` chỉ đụng `sc.TeacherID`/`sc.CenterWideFor` — rule
  đó sẽ false-positive ngay. Nếu muốn khép rẻ (tuỳ chọn, follow-up ~10 dòng):
  yêu cầu narrowing helper có **ít nhất một selector bất kỳ** trên tham số
  scope (bắt helper bỏ hẳn tham số), vẫn presence-based. Không chặn merge.
- **Allowlist call-site `*Anchored`**: đồng ý để review. Không type-based được
  (caller cross-package đi qua interface cục bộ), tiền đề "chỉ imports" đã sai
  (`billing/close.go` → `sessions.ListUnconfirmedInWindowAnchored`). Ghi vào
  docs Tenancy một câu "exported `*Anchored` chỉ được gọi từ service đã gate
  hoặc từ imports" là đủ.

Một điểm phải sửa **trước commit** (vi phạm `review-audit-self-decision.md`
"Stable Code Artifacts", review (i) chấm trước khi có review fixes nên không
bắt): comment trong `tools/scopelint/scopelint/analyzer.go` nhắc mã finding
`H1` (:79), `H2` (:176, :355, :468), `M1/M4` (:394). Viết lại theo hành vi
("so analysing nothing cannot pass as analysing everything", "receiverless
functions declared alongside repository methods are checked too", "a copy or
embedded field counts"). Chỉ comment, chạy lại `go test ./tools/...` là xong.

## (2) Merge-readiness toàn plan — READY WITH CONDITIONS

Đạt: mọi success criteria kỹ thuật trong `plan.md` có bằng chứng (bảng trên +
tester 77.8%/lint 0/`make api-docs` không diff). Không migration, không đổi
route contract, web không đổi.

Điều kiện trước **commit** (đều là docs/comment, không đổi hành vi):

1. Gỡ nhãn finding code trong `analyzer.go` (mục trên).
2. `billing/integration_test.go:1603` comment "proves the product decision D5
   records" → mô tả hành vi trực tiếp (trợ giảng có stint attendance confirm
   buổi trong kỳ đã đóng của giáo viên khác tạo adjustment trên billing của
   người đó). Test tên `TestPublicPaymentsByInvoiceMatchesD8UnderpaymentSplit`
   là pre-existing (D8 = quy tắc allocation trong PRD payments, có sẵn trên
   master) — không đụng.
3. `plan.md`: phase 4 → Completed; `status: in-progress` → completed sau merge;
   tick Success Criteria (đều đã có bằng chứng); **viết lại Rollout §1** (không
   còn "phase 1 lên master trước" — cả 5 phase là một PR; kiểm kê D4 đã chạy
   2026-09-06 = 0 holder nên phase 1 không cần ship riêng); thêm vào D6 câu:
   "`zalo.MatchFriendsScoped` giờ mở cho holder `contacts.view_all` trực tiếp
   (trước chỉ oversight/hoc_vu)"; `phase-04` status → completed; ghi sửa 2 chỗ
   spec sai (`internal/database` không phải `shared/database`; testdata phải
   nằm dưới `teka/apps/api/internal/...`).
4. Follow-up ngoài scope ghi vào plan (không chặn): web
   `member-permissions-dialog.tsx` badge không tính key suy ra; dataflow
   `collections.PeriodSummary`; narrowing-helper selector rule.

Điều kiện trước **merge** (không thể tự quyết): owner trả lời D9 và OQ6 (mục 4).
D6 là thông báo. Nếu owner chọn OQ6(a) owner-only: thêm gate `IsOwner` ở
`students.Create` + đổi 3 test là một commit nhỏ trên cùng branch, không cần
phase mới. Nếu owner từ chối D9 (không muốn member ghi thanh toán): giữ code,
bỏ `payments.create` khỏi `DefaultRoleKeys()` là plan riêng có migration —
merge plan này vẫn an toàn vì reach ghi đó đang bị `writeScoped` chặn trên
reverse/reallocate.

## (3) Commit slicing — công thức chính xác (đã kiểm chứng trên bản sao)

Sự thật về snapshot: `phase12-tracked.patch` (02:16, 67 file) và
`phase35-tracked.patch` (03:36, 86 file) đều là **diff tích luỹ từ master, chỉ
tracked file**, cả hai `git apply --check` sạch trên `git archive master`;
p35 không apply lên p12 (đúng vì tích luỹ). Phase 1 và 2 không tách được (không
có snapshot giữa) — chấp nhận một commit. Tôi đã sinh
`$S/p12-to-p35.patch` (40 file, `git diff --no-index m12 m35`, dùng `-p2`) và
chứng minh: apply phần exclude rồi phần include cho kết quả **identical** với
snapshot 03:36. `$S` =
`/tmp/claude-1000/-home-cesc-Documents-personal-workspace-teka/9ee7cacf-9e92-437d-a7cf-402023446623/scratchpad`
(cùng session với lead; ba file m12/m35/p12-to-p35.patch đang ở đó).

Cần user yêu cầu commit (harness); không push master (quy ước repo). Branch:
`authz-write-scope`. Cách dùng `git apply --cached` chỉ đổi index, working
tree không bị chạm; nếu bất kỳ bước nào lỗi, `git reset` index (không
`--hard`) và fallback 2 commit (C1 = p35, C2 = phần còn lại).

```
S=/tmp/claude-1000/-home-cesc-Documents-personal-workspace-teka/9ee7cacf-9e92-437d-a7cf-402023446623/scratchpad
git switch -c authz-write-scope                       # giữ nguyên working tree

# C1 — phase 1 + 2 (67 tracked + 1 untracked)
git apply --cached "$S/phase12-tracked.patch"
git add apps/api/internal/shared/authctx/anchor_test.go
git commit -m "fix(api): view_all keys widen reads only and row anchor is a separate type from Scope"

# C2 — phase 3 + hotfix candidacy theo center/contact (docs hunk có lẫn đoạn "Thêm route" của phase 5 — chấp nhận)
git apply --cached -p2 \
  --exclude='apps/api/internal/server/route_policy.go' \
  --exclude='apps/api/internal/features/audit/*' \
  --exclude='apps/api/internal/middleware/*' \
  --exclude='docs/event-bus.md' \
  --exclude='apps/api/CLAUDE.md' \
  "$S/p12-to-p35.patch"
git add apps/api/internal/features/notifications/zalo_mappings_scope_integration_test.go
git commit -m "fix(api): reports.send implies read keys and payment allocation keys on center and contact"

# C3 — phase 5
git apply --cached -p2 \
  --include='apps/api/internal/server/route_policy.go' \
  --include='apps/api/internal/features/audit/*' \
  --include='apps/api/internal/middleware/*' \
  --include='docs/event-bus.md' \
  --include='apps/api/CLAUDE.md' \
  "$S/p12-to-p35.patch"
git add apps/api/internal/shared/routespec apps/api/internal/server/route_policy_snapshot_test.go
git commit -m "refactor(api): derive route policy, audit actions and audit skip sets from one route manifest"

# C4 — phase 4 (phần còn lại của apps/api + Makefile + docs: tools/, go.mod, xoá guard, 2 comment, CLAUDE.md dòng scopelint)
git add -A apps/api Makefile docs
git commit -m "test(api): add scopelint analyzer enforcing repository tenancy scoping"

# C5 — plan + reports
git add plans
git commit -m "docs(plans): record authz write-scope root-cause plan and reports"

git status --short      # phải rỗng
git diff HEAD --stat    # phải rỗng
cd apps/api && go build ./... && go test -count=1 ./tools/... && cd ../.. && make lint-api
```

Ghi chú: `scoping_guard_test.go` xuất hiện sửa ở C1/C2 và xoá ở C4 — lịch sử
trung thực, giữ. Mỗi commit C1–C3 tự nó có thể không xanh 100% (test phase 3
sửa test phase 1 "List→404 sẽ lật"); chỉ HEAD của branch cần xanh — ghi trong
PR body. Không ghi tên agent/AI trong message (quy ước repo).

## (4) Tin nhắn gửi owner (tiếng Việt, dán được)

> Chào anh/chị, bên kỹ thuật vừa hoàn tất bản sửa phân quyền ghi dữ liệu. Trước
> khi đưa lên hệ thống, cần anh/chị chốt 2 điểm và biết 1 thay đổi:
>
> **1. Ai được ghi thanh toán?** (cần chốt)
> Hiện tại (bản đang chạy) chỉ thành viên có quyền "Xem mọi thanh toán" mới ghi
> được thanh toán cho phụ huynh không thuộc lớp mình — và thực ra đó là do lỗi
> quyền xem đang mở nhầm quyền ghi. Bản mới: **mọi thành viên có quyền "Ghi
> thanh toán" (quyền mặc định của mọi vai trò) ghi được thanh toán cho bất kỳ
> phụ huynh nào trong trung tâm.** Kèm theo: chỉ **chủ trung tâm** mới đảo/huỷ
> hoặc phân bổ lại thanh toán; thành viên không có "Xem mọi thanh toán" ghi
> xong sẽ không xem lại được phiếu đó trong danh sách (chỉ thấy kết quả ngay
> lúc ghi). Số tiền được tự động trừ vào hoá đơn đúng của giáo viên phụ trách
> (bản cũ bỏ sót hoá đơn của giáo viên không phải chủ — đã sửa).
> → Đồng ý như trên? Nếu anh/chị muốn chỉ chủ trung tâm hoặc thư ký được ghi
> thanh toán, bên em sẽ bỏ quyền "Ghi thanh toán" khỏi bộ quyền mặc định ở một
> đợt sau (không ảnh hưởng đợt này).
>
> **2. Thành viên có được tự thêm học sinh không?** (cần chốt)
> Sổ danh bạ (phụ huynh, học sinh) thuộc về chủ trung tâm. Tài liệu nói "thành
> viên không tạo/sửa phụ huynh và học sinh", nhưng hệ thống hiện vẫn cho thành
> viên có quyền "Thêm học sinh" tạo học sinh gắn vào chính mình (ứng dụng web
> hiện không có màn hình này cho thành viên, chỉ qua nhập Excel là đã gắn đúng
> về chủ). Chọn một:
> (a) Chỉ chủ trung tâm được thêm/sửa học sinh — khớp tài liệu, bên em khoá lại.
> (b) Giữ như hiện nay — bên em sửa tài liệu cho đúng.
> Đề xuất của bên em: **(a)** để danh bạ về một mối.
>
> **3. Quyền "Gửi báo cáo" đọc rộng hơn** (chỉ thông báo)
> Thành viên có quyền "Gửi báo cáo" từ nay **tự động xem được** danh sách kỳ
> thu, bảng kê, lịch sử gửi và số điện thoại phụ huynh của cả trung tâm (đúng
> bản chất: muốn gửi báo cáo cho mọi nhà thì phải đọc được) — chỉ đọc, không
> ghi. Đối chiếu Zalo theo số điện thoại cũng mở cho người có "Xem mọi phụ
> huynh". Trên màn hình quyền của thành viên, các quyền xem này chưa hiển thị
> là "có" (sẽ sửa sau), nhưng hệ thống đã áp dụng đúng.
>
> Ngoài ra, sau khi lên bản mới sẽ có vài thao tác đổi kết quả: người chỉ có
> quyền "Xem mọi …" mà không phải giáo viên/trợ giảng của lớp sẽ **không còn**
> đóng kỳ, huỷ buổi, sửa lớp, xác nhận điểm danh hộ người khác (trước đây lọt
> do lỗi). Kiểm tra dữ liệu ngày 06/09 cho thấy chưa thành viên nào đang giữ
> quyền "Xem mọi …", nên không ai bị mất thao tác đang dùng. Cảm ơn anh/chị.

## (5) Checklist ngày deploy

Trước deploy (sau khi PR merge, trước khi tag `vX.Y.Z` — `api-release.yml`
build theo tag):

1. Chạy lại đúng 3 truy vấn kiểm kê
   `plans/reports/inventory-260906-prod-view-all-write-exposure.md` (holder
   `*.view_all` theo role/override; write non-owner 60 ngày). Khác 0 → đã thông
   báo owner ở mục 4, không chặn.
2. Thêm 2 truy vấn mới (read-only, aggregate):
   - Số member (không owner) giữ `reports.send` theo center — đó là tập nhận
     widening đọc D6.
   - Số `payments` có `amount - allocated > 0` mà contact còn invoice
     `issued/partially_paid` **của kỳ member** (`invoices.teacher_id <> owner`)
     — đây là dữ liệu bị H-1 bỏ sót trước fix; fix chỉ áp cho ghi mới.
3. Xác nhận owner đã trả lời D9 + OQ6; nếu OQ6 = (a), commit gate đã vào branch.
4. `make test-api`, `make lint-api`, `make api-docs` xanh trên CI của PR (lần
   chạy CI đầu tiên của cây này).

Deploy: rollout code thuần, không migration; rollback = deploy lại tag trước.

Sau deploy (24–48h):

- Với các payment ở mục 2b: owner gọi `POST /api/v1/payments/:id/allocations/auto`
  (route `payments.allocate`, `routespec.go:323`) để phân bổ phần dư — chỉ
  owner đảo/phân bổ được. Kiểm chứng: invoice của member chuyển sang
  `paid/partially_paid`.
- Theo dõi log/audit: 403/404 trên `sessions.cancel/hold/delete`,
  `attendance.confirm`, `billing.close`, `payments.reverse`,
  `statements.revoke`, `enrollments.end` của actor không owner → nếu xuất hiện
  là member legacy đang bị đóng escalation (đúng contract) → owner gán stint.
- 404 trên `POST /notifications/mark-sent` (trước là no-op im lặng).
- Payload `permissions` trong center-context của member `reports.send` có thêm
  4 key `view_all` — web `has()` hoạt động, badge dialog hiển thị "Không có"
  (follow-up đã ghi).
- Feed `GET /sessions/pending` cho member `sessions.view_all` (readScopedFeed).

## (6) Rủi ro còn lại chưa được ghi

1. **CI chưa từng chạy trên cây này** (0 push). Điểm dễ vỡ nhất: `packages.Load`
   trong `tree_test.go` trên runner (cần module cache đầy đủ — `setup-go` cache
   theo `go.sum`, không đổi → ổn), và golangci-lint v2.7.2 trên `tools/` (local
   cùng version 2.7.2, 0 issue). Mở PR sớm, để CI chạy trước khi chờ owner.
2. **Phase 1 không tách được để hotfix riêng** — Rollout §1 của plan.md đã lỗi
   thời; chấp nhận vì kiểm kê = 0 holder (lỗ hổng tiềm ẩn, chưa bị khai thác).
   Cập nhật plan.md, đừng để ai đọc §1 rồi cố cherry-pick.
3. **Dữ liệu H-1 tồn đọng trên prod** (mục 5, 2b/sau deploy): kiểm kê 06/09 ghi
   2 member tạo 27 kỳ → invoice member có thể chưa từng được allocate; cần thao
   tác owner, không phải migration.
4. **`impliedKeys` không có test chống chuỗi** (key suy ra lại là key suy ra
   khác — `BuildPermSet` duyệt map nên phụ thuộc thứ tự). Hôm nay chỉ 1 entry;
   3 dòng test, tuỳ chọn.
5. **Nhãn finding code trong `analyzer.go`** (mục 1) và "decision D5" trong
   `billing/integration_test.go:1603` — vi phạm quy tắc repo, chưa ai ghi.
6. **Doc `docs/api-guidelines.md` Tenancy** không nói đến ràng buộc gọi
   `*Anchored` exported (carry-over M-3 phase 2 chốt "để review") — thêm một
   câu để reviewer tương lai có căn cứ.
7. **`tree_test.go` không truyền `BuildFlags: -tags=integration`** — hôm nay
   không file non-test nào có build constraint; nếu sau này thêm file
   `//go:build integration` chứa repository code, analyzer sẽ bỏ qua nó. Ghi
   một dòng comment trong tree_test.

## Assumptions

- Lead sẽ chỉ commit khi user yêu cầu (harness); công thức mục 3 viết cho lúc
  đó — **cao**. Nếu user chọn "một commit", vẫn dùng branch `authz-write-scope`
  và giữ C5 riêng.
- Owner chưa trả lời D9/OQ6; merge chờ, PR/CI không chờ — **cao**.
- Prod chưa đổi grant từ 06/09 — **trung bình**; vì thế mới có bước kiểm kê lại.
- Payment tồn đọng H-1 tồn tại trên prod — **trung bình** (suy từ kiểm kê 27
  kỳ member; chưa đếm trực tiếp) → truy vấn 2b sẽ trả lời.
