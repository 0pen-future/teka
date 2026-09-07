---
phase: 4
title: "Repository scope linter"
status: completed
priority: P2
effort: "1d"
dependencies: [1, 2, 3]
---

# Phase 4: Repository scope linter

## Overview

Thay guard tenancy "grep token" bằng một analyzer `go/analysis` nhỏ chạy trong
`make test-api-unit`, `make lint-api` và CI: mọi method repository nhận
`authctx.Scope`/`authctx.Anchor` mà chạm DB phải đi qua helper scoping; truy cập
DB thô chỉ được nằm trong helper hoặc hàm có directive nêu lý do. Bắt được lỗi
"quên `scoped()`" mà grep hiện không thấy.

## Requirements

Spec gốc (R1 "mọi chuỗi `*gorm.DB` bắt nguồn từ helper allowlist" + R2 "gốc thô
chỉ trong helper") được checkpoint kongming 2026-09-07
(`plans/reports/kongming-phase3-5-checkpoint-260907.md`) đo lại trên cây thật:
221 method repository nhận Scope/Anchor, 54 helper, **78 method thường dựng
query từ gốc thô inline** (bind `center_id` ngay trong `Where`) → ~180 vi phạm
so với ngân sách 8 directive. Spec thay thế, giữ type-based, không đổi hành vi
repository:

- Functional:
  - Mọi rule dựa trên `types.Info` (go/types) và `types.Object`, không dựa
    chuỗi tên helper hay regex. **Helper nhận diện theo hình dạng kiểu**: method
    unexported trên receiver repository, trả `*gorm.DB`, có ≥1 tham số kiểu
    `authctx.Scope`/`Anchor`/`OwnerAnchor` (khớp đủ 54 helper hiện có ở 13
    package, kể cả `scopedRead`, `invoiceCenterScoped`, `contactBalanceQuery`,
    `withStudentCount`; loại đúng `teachers.scoped(ctx, teacherID)` và
    `payments.allocationsQuery` vì không có tham số scope).
  - R1 "scope witness" (thay R1+R2 cũ): trong tập file phân tích
    (`internal/features/*/repository.go` và file cùng package khai báo method
    trên `*gormRepository`; `centers/` miễn), mọi method có tham số
    Scope/Anchor/OwnerAnchor mà chứa **gốc thô** (call `database.FromContext`
    hoặc đọc field receiver kiểu `*gorm.DB`, xác định theo object) phải có ít
    nhất một **witness**: (i) call tới helper theo hình dạng trên, hoặc (ii)
    selector `<thamSốScope>.CenterID`. Helper gốc (nhận ctx, không nhận
    `*gorm.DB`) cũng phải tham chiếu `<param>.CenterID`; helper thu hẹp (nhận
    `*gorm.DB`) miễn. Method không có tham số scope nằm ngoài rule theo thiết
    kế (insert/update struct service đã dựng, lookup theo token/id đã gate ở
    service) — ghi trong doc comment analyzer; RLS là lớp bắt cho lớp này (D7).
  - R2 "no reset": cấm `.Session(`, `gorm.Session{NewDB: true}`, `.Unscoped()`
    trên `*gorm.DB` trong mọi file non-test dưới `internal/features` (hôm nay 0
    hit). **Không** cấm `.Table(` — là chọn bảng hợp lệ (3 chỗ đang dùng).
  - R3 "authority ở authctx" (port 3 guard AST sang type-based): trong file
    repository cấm đọc field `Scope.IsOwner`, gọi `PermSet.Has`,
    `Scope.CenterWide`, `StaffRolesFor`, `StaffRoleCan`; `Scope.CenterWideFor`
    chỉ trong hàm có `read` (lowercase) trong tên — rule tên duy nhất được giữ
    vì trục read/write không biểu diễn được bằng kiểu. Ngoài `centers/`,
    `middleware/`, `testutil/`, `authctx/`, `seeds/`: cấm CompositeLit
    `authctx.Scope` có phần tử, **mọi AssignStmt vào field của giá trị kiểu
    `Scope`** (bắt `s := sc; s.TeacherID = x` — carry-over phase 2), literal
    `OwnerAnchor`, call `MintOwnerAnchor` ngoài `centers/`. Không cần rule alias
    import (so object, không so tên).
  - Directive `//scopelint:unscoped <lý do>` ngay trên method chỉ miễn R1.
    Ngân sách ≤8; kỳ vọng trên cây hôm nay: 0–1.
  - Analyzer có testdata `analysistest` cho mỗi rule (positive + negative),
    với **stub package đặt đúng import path thật** dưới `testdata/src`
    (`gorm.io/gorm`, `teka/apps/api/internal/shared/authctx`,
    `teka/apps/api/internal/shared/database`) vì analyzer so object theo full
    path; không trỏ testdata vào package thật.
  - Tự cưỡng chế qua `go test`: `tools/scopelint/scopelint/tree_test.go` load
    `teka/apps/api/internal/...` bằng `packages.Load(LoadAllSyntax)` + chạy
    analyzer, fail khi có diagnostic — vì CI lint job dùng
    `golangci-lint-action` trực tiếp và test job chạy `make test-api` (không
    gọi `test-api-unit`), hook Makefile đơn thuần không bao giờ chạy ở CI.
    `make scopelint` (`go run ./tools/scopelint ./internal/...`) giữ cho output
    đọc được, nối vào `lint-api`.
- Non-functional:
  - Tree-run test < 8 giây warm (là một lần load ~40 package; không giấu sau
    `-short`).
  - Không phụ thuộc ngoài `golang.org/x/tools` (đã có indirect v0.49.0; `go mod
    tidy` chỉ bỏ `// indirect`, commit go.mod/go.sum cùng nhau).
  - Xoá `scoping_guard_test.go` **cùng PR** khi tree-run test xanh — không giữ
    bản "smoke".
- Non-goals (tường minh): method không có tham số scope; dataflow theo từng
  chuỗi (`go/ssa`) — chỉ `collections.PeriodSummary` có >1 gốc, ghi follow-up;
  service (ngoại lệ D3 ở sessions); soi chuỗi SQL thô tìm `center_id`; đổi tên
  helper về một quy ước; thêm tham số Scope cho method không scope; allowlist
  call-site `*Anchored` (không type-based được: mọi caller cross-package đi qua
  interface cục bộ, và tiền đề đã sai vì `billing/close.go` gọi
  `sessions.ListUnconfirmedInWindowAnchored`) — để review; chốt OQ6/D9 trong
  analyzer.

## Architecture

```
apps/api/tools/scopelint/
  main.go            // singlechecker.Main(scopelint.Analyzer)
  scopelint/
    analyzer.go      // R1–R3
    analyzer_test.go // analysistest.Run(t, testdata, Analyzer, "a")
      tree_test.go     // packages.Load(./internal/...) + checker → fail khi có diagnostic
    testdata/src/a/  // repository.go giả với case đúng/sai
    testdata/src/gorm.io/gorm, teka/apps/api/internal/shared/{authctx,database}  // stub
```

Nối vào Makefile: rule `scopelint` (`go run ./tools/scopelint ./internal/...`
hoặc `go vet -vettool=$(SCOPELINT_BIN) ./internal/...`), được `test-api-unit`
và `lint-api` gọi. CI `api-ci.yml` đã chạy `make test-api` nên tự bao phủ; kiểm
tra bước golangci-lint không xung đột. Directive `//scopelint:unscoped` là dạng
comment chuẩn Go (`//tool:directive`), không phải nhãn plan/phase.

Directive dự kiến trên cây hôm nay: 0–1 (chỉ 2/78 method inline không có
selector `CenterID`, cả hai đều pass nhờ call helper: `GetInvoiceWithLines` qua
`invoiceAnchored`, `SessionMeta` qua `centerScoped`). Method không nhận Scope
(`attendance.UpsertMany`, `audit.InsertBatch`, `auth`…) nằm ngoài R1 — **không**
thêm tham số Scope vô nghĩa chỉ để vừa linter.

## Related Code Files

- Create: `apps/api/tools/scopelint/main.go`, `apps/api/tools/scopelint/scopelint/{analyzer.go,analyzer_test.go,tree_test.go}`, `apps/api/tools/scopelint/scopelint/testdata/src/{a,gorm.io/gorm,teka/apps/api/internal/shared/authctx,teka/apps/api/internal/shared/database}/`
- Modify: `Makefile` (rule `scopelint`, nối `test-api-unit`, `lint-api`)
- Modify: `apps/api/go.mod`, `go.sum` (`go mod tidy` chuyển `golang.org/x/tools` sang direct)
- Modify: `apps/api/internal/features/*/repository.go` — thêm directive `//scopelint:unscoped` cho hàm hợp lệ
- Delete: `apps/api/internal/features/scoping_guard_test.go` (khi tree-run test xanh)
- Modify: `docs/api-guidelines.md` (Tenancy: cách analyzer kiểm và cách khai báo unscoped), `apps/api/CLAUDE.md` (một dòng: chạy `make scopelint` khi sửa repository)

## Implementation Steps

1. Dựng analyzer với R1 witness (helper theo hình dạng + selector `CenterID`) + testdata với stub package; chạy trên cây thật, liệt kê vi phạm; phân loại "lỗi thật" vs "cần directive".
2. Thêm R2, R3 (kể cả assign vào field Scope); testdata cho từng rule.
3. Gắn directive (0–1) với lý do một câu (không nhắc plan/phase).
4. `tree_test.go` + `main.go` + `make scopelint` nối vào `lint-api`; `go mod tidy`; xác nhận probe billing làm `go test ./tools/...` và `make test-api-unit` đỏ.
5. Xoá `scoping_guard_test.go`; docs; cập nhật phần Carry-over bên dưới theo kết quả.

## Carry-over từ phase 2 (2026-09-07)

- Lỗ guard AST còn lại: `s := sc; s.TeacherID = x` (copy Scope rồi gán lại
  identity) — chỉ analyzer type-based bắt được.
- Entry point exported nhận `authctx.Anchor` trần không proof, không gate:
  `classes.CreateAnchored`, `classes.AddScheduleAnchored`,
  `enrollments.CreateAnchored(actor, a, req)`. Caller hợp lệ duy nhất là
  `features/imports` (anchor resolve từ giáo viên nêu trong workbook nên không
  ép được `OwnerAnchor`). Analyzer cần rule allowlist call-site cho method có
  hậu tố `Anchored` nhận `Anchor` (cho phép: cùng package, `features/imports`).
- `students.Create` không gate `IsOwner` tường minh (open question 6 trong
  plan.md) — nếu chốt anchor về owner thì đi qua `ResolveOwnerAnchor`.

## Success Criteria

- [x] `analysistest` xanh: R1 flag (gốc thô, không helper, không `CenterID`) và (gốc thô + chỉ `sc.TeacherID`); pass (gốc từ helper), (`Where("center_id = ?", sc.CenterID)` thô), (helper thu hẹp `centerScoped(q, sc, col)`), (`Raw(sql, a.CenterID)`), (method có directive). R2 flag `.Session(&gorm.Session{NewDB: true})`, `.Unscoped()`. R3 flag `sc.IsOwner`, `.Has(`, `CenterWideFor` trong hàm không mang tên read, `authctx.Scope{TeacherID: x}`, `s := sc; s.TeacherID = x`, `MintOwnerAnchor` ngoài centers; và không flag cùng các case đó trong path miễn.
- [x] Tree-run: 0 diagnostic trên `./internal/...` với ≤1 directive (nêu tên); chèn `func (r *gormRepository) X(ctx context.Context, sc authctx.Scope, id uuid.UUID) error { return database.FromContext(ctx, r.db).Where("id = ?", id).Delete(&Period{}).Error }` vào `billing/repository.go` cho đúng một diagnostic.
- [x] Chạy dưới `go test ./...` thuần (tree-run test trong `tools/scopelint`) → tự bao phủ `make test-api-unit`, `make test-api`, CI mà không đổi workflow; `make scopelint` cho output riêng, `lint-api` gọi nó. Wall time < 8s warm.
- [x] `apps/api/internal/features/scoping_guard_test.go` xoá; `docs/api-guidelines.md` Tenancy giải thích witness rule và directive trong một đoạn; `apps/api/CLAUDE.md` một dòng.
- [x] `grep -rn 'scopelint:unscoped' apps/api/internal` ≤ 1, mỗi directive có lý do một câu (không nhắc plan/phase).

## Risk Assessment

- **False positive với helper đặt tên khác quy ước** (VD `periodStatus`). Tín hiệu: analyzer đỏ trên hàm đúng. Phản ứng: đổi tên hàm theo quy ước (ưu tiên) hoặc directive; không mở rộng regex thành quá lỏng.
- **Raw SQL string bỏ qua helper vẫn lọt** (VD `Raw("SELECT ...")` không có `center_id`). Chấp nhận có chủ ý (D7); RLS ở plan riêng là lớp bắt.
- **CI chậm hoặc go vet vettool không tương thích golangci-lint-action**. Tín hiệu: bước CI đỏ vì tool. Phản ứng: chạy analyzer như bước `go run` riêng trong `make`, không qua golangci plugin.

## Execution Notes

- Spec R1/R2 gốc đo lại trên cây (lead + checkpoint kongming 2026-09-07): 230
  hàm repository chạm gốc thô, 123 nhận Scope/Anchor, ~78 method thường bind
  `center_id` inline → spec gốc cần ~180 directive. Viết lại Requirements /
  Success Criteria theo rule "scope witness" + helper theo hình dạng kiểu trước
  khi implement (phần trên là bản đã sửa).
- Implement (`plans/reports/phase4-scopelint-260907.md`, TDD): analysistest
  đỏ → analyzer xanh → `tree_test.go` load `teka/apps/api/internal/...` qua
  `go/analysis/checker`. Cây thật: **0 diagnostic, 0 directive** (dự kiến
  0–1). Probe billing → đúng 1 diagnostic, đã revert. Wall time
  `go test ./tools/...` ~3–4.5s warm. Makefile: `scopelint` là prerequisite
  của `test-api-unit` và `lint-api`; CI bao phủ qua `make test-api` nhờ
  tree_test. `go.mod` chỉ chuyển `golang.org/x/tools v0.49.0` sang direct,
  `go.sum` không đổi. `scoping_guard_test.go` đã xoá.
- Hai điểm spec sai so với cây, đã sửa trong implement: `database.FromContext`
  ở `teka/apps/api/internal/database` (không phải `shared/database`); testdata
  case phải nằm dưới `testdata/src/teka/apps/api/internal/features/testcase/`
  vì rule internal-visibility của Go chặn import stub `internal/...` từ package
  `a`.
- Lead sửa thêm 2 comment cũ còn nhắc `scoping_guard_test` ở
  `grading/repository.go` và `classstaff/repository.go`. Implement cũng gỡ một
  câu docs đã sai từ trước ở `api-guidelines.md` (`Scope.CenterWide()` không
  còn tồn tại).
- Follow-up hardening (ngoài scope, ghi nhận): dataflow theo chuỗi cho method
  nhiều gốc (`collections.PeriodSummary`); allowlist call-site `*Anchored` chỉ
  làm được theo tên → để review.
- Tester (`plans/reports/tester-phase4-260907-scopelint.md`): full suite 37
  package xanh, coverage 77.8%, lint 0, `test-api-unit` 38 package xanh,
  probe billing → 1 diagnostic, revert byte-identical (lead đối chiếu diff
  `billing/repository.go` với snapshot trước phase 4).
- Review (`plans/reports/code-review-260907-phase-04-scopelint.md`, 8/10,
  không Critical). Disposition: H1 (fail-open khi đổi tên `repository.go`) →
  nhận diện receiver theo hình dạng kiểu (struct trong `features/` có field
  `*gorm.DB`) + sàn số package trong tree test; H2 (bỏ qua free function) →
  chạy R1/R3 cho mọi FuncDecl trong file set; M2 (pointer tới Scope) → bóc
  pointer; M4 (witness chỉ nhận ident tham số) → witness là selector
  `CenterID` trên mọi biểu thức kiểu Scope/Anchor/OwnerAnchor; M1 (witness
  chỉ là "được nhắc tới") → chấp nhận theo non-goal, ghi rõ trong doc comment
  và docs; M3 (docs nói "go vet-time") → sửa; L1 helper phải cùng receiver;
  L2 rule tên read tách token camelCase; L4 testdata exempt path + sibling
  file; L5 memory file kongming; L6 tree test load thêm `cmd/...`; L3 bỏ qua
  (spec chỉ cấm literal, 0 hit). Câu hỏi mở của review "narrowing helper bỏ
  qua tham số scope có thể rửa scope" → chấp nhận có chủ ý cùng M1 (dataflow
  theo chuỗi là non-goal, RLS là lớp bắt); allowlist call-site `*Anchored` để
  review theo counsel.
- Sau sửa review: `go test ./tools/...` ~4.4s, `make scopelint` 0, `make
  lint-api` 0, `make test-api-unit` xanh (7.4s tổng). Checkpoint kongming cuối
  (`plans/reports/kongming-final-checkpoint-260907.md`): **GO** — H1 đóng đúng
  (receiver theo hình dạng kiểu, sàn 18 package, cây thật 21), CI cưỡng chế
  qua `make test-api` → `tree_test`; prerequisite `scopelint` trong `lint-api`
  chỉ chạy local (CI dùng golangci-lint-action trực tiếp) — đúng thiết kế.
  Lead gỡ mã finding khỏi comment `analyzer.go` (5 chỗ) theo quy tắc Stable
  Code Artifacts. Rủi ro còn lại ghi ở plan.md Rollout: `tree_test` không
  truyền `-tags=integration` (file integration không được soi literal Scope —
  guard cũ cũng chỉ soi repository.go); docs Tenancy chưa có câu về
  `*Anchored`.
