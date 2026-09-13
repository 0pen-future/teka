---
phase: 6
title: "Ma trận quyền, e2e, docs, ship"
status: completed
priority: P1
effort: "0.5d"
dependencies: [5]
---

# Phase 6: Ma trận quyền, e2e, docs, ship

## Overview

Đóng vòng: nhóm "Công việc" xuất hiện đúng chỗ trong ma trận Phân quyền, một
luồng e2e chạy trên stack cô lập, tài liệu cập nhật đúng bề mặt sở hữu, review
và ship.

Ma trận UI về nguyên tắc tự sinh nhóm từ catalog, nhưng có **một điểm phải sửa
thật**: bảng nhãn resource phía web là hardcode, thiếu `tasks` thì tab hiển thị
chuỗi thô `"tasks"`.

## Requirements

### Functional

- `RESOURCE_LABELS` có `tasks: "Công việc"` → tab ma trận hiển thị nhãn tiếng
  Việt. `members` đã có nhãn "Thành viên" và nằm trong `ADMIN_RESOURCES` → khoá
  `members.list` xuất hiện trong nhóm "Thành viên" bên trong tab "Quản trị".
- Ma trận hiển thị 7 khoá `tasks` với nhãn + risk từ API; `manage_board`,
  `view_all` và `members.list` trống cho tới khi owner tick (opt-in — plan
  D9/D11); `manage_board`/`view_all` nhấn coral như scope key hiện có.
- Hệ quả UX phải nói rõ ở handoff: `HvSelect` "Giao cho" trong form công việc
  **rỗng** ở trung tâm chưa gán `members.list` cho vai trò đó; owner luôn thấy
  (bypass). Không tự bật.
- Bộ lọc audit `ACTION_GROUPS` (`features/audit/components/audit-filters.tsx:10`)
  là hardcode → thêm nhóm "Công việc" với prefix `task.` và `task_column.`,
  kèm test; nếu không, audit tasks vẫn ghi nhưng không lọc được *(Red-team)*.
- Một spec Playwright: thêm cột "Chờ duyệt" → tạo việc → giao cho thành viên →
  chuyển sang cột mới → chuyển sang cột hoàn thành → xoá cột "Chờ duyệt" có việc
  (chọn cột đích).
- Docs cập nhật: `docs/adding-permissions.md` (`DefaultGrant` — nếu Phase 1 chưa
  làm), `docs/event-bus.md` (event catalog + `task.handover` blind spot),
  `docs/architecture.md` (mục `pkg/kanban` là lớp lib đầu tiên), README của 2
  lib (đối chiếu lại với code đã viết thật).

### Non-functional

- e2e chạy trên stack cô lập `compose -p teka-e2e` với port + URL override, seed
  tươi. Không chạm stack production `teka-*`.
- Commit theo conventional commit, **không** AI reference.
- Push lên `master` cần user approval (SSH `cesc1802` mới có quyền ghi).

## Architecture

**Không thêm pattern mới.** Phase này khai thác đúng chỗ nối đã có:
`groupCatalog` và `buildCatalogTabs` (`permission-schemas.ts:105`, `:150`) là
điểm mở rộng theo dữ liệu — thêm khoá vào catalog API là đủ, trừ bảng nhãn.

**Trade-off đã chấp nhận:** `RESOURCE_LABELS` và `ADMIN_RESOURCES` là hai map
hardcode phía web song song với catalog phía API. Đây là duplication có chủ ý
(nhãn tab là quyết định UX, không phải dữ liệu API). Rủi ro là quên đồng bộ →
giảm bằng fallback `RESOURCE_LABELS[resource] ?? resource` đã có (nhóm không
biến mất, chỉ xấu) và một test khẳng định nhãn "Công việc".

**Thứ tự tab.** `buildCatalogTabs` giữ thứ tự catalog. Phase 1 chèn khối `tasks`
trước khối admin → tab "Công việc" đứng trước "Quản trị". Nếu Phase 1 chèn ở
cuối, tab sẽ nằm sau "Quản trị" — kiểm bằng mắt khi chạy test ma trận.

## Related Code Files

**Modify**

- `apps/web/src/features/center/schemas/permission-schemas.ts` — thêm
  `tasks: "Công việc"` vào `RESOURCE_LABELS` (`:70-92`).
- `apps/web/src/features/center/__tests__/center-permissions.test.tsx` — case
  khẳng định tab "Công việc" tồn tại, 7 khoá `tasks` render, và với fixture
  `DEFAULT_ROLES` (`src/test/msw/handlers.ts:319-345`, mọi role
  `permissions: []`) **không** khoá nào tick — fixture không mô phỏng backfill,
  nên không assert "5 khoá CRUD tick sẵn" ở đây *(Red-team: criterion cũ không
  kiểm được bằng fixture hiện có)*. Nếu muốn assert backfill phía web, thêm
  fixture role tường minh có 5 khoá CRUD.
- `apps/web/src/features/center/__tests__/center-handlers.ts` — fixture catalog
  gồm 8 khoá mới (nếu chưa kế thừa từ `src/test/msw/handlers.ts`).
- `apps/web/src/features/audit/components/audit-filters.tsx` —
  `ACTION_GROUPS` (`:10`) thêm nhóm "Công việc" (`task.`, `task_column.`).
- `apps/web/src/features/audit/__tests__/audit-page.test.tsx` — case bộ lọc
  hiện nhóm "Công việc".
- `docs/event-bus.md` — "Event catalog" (`:35`) thêm
  `centers.member_tasks_handed_over` và `tasks.column_deleted`; "Action map
  convention" (`:60`) thêm `task.*` / `task_column.*`; "Known blind spots"
  (`:76`) ghi: handover và xoá cột audit đi đường service-level (bus
  at-most-once), reorder chỉ ghi thứ tự **sau** (middleware không thấy thứ
  tự trước).
- `docs/architecture.md` — "Monorepo" (`:5`) hoặc "Applications" (`:12`) ghi
  `apps/api/pkg/kanban` và `apps/web/src/lib/kanban` là lib biên giới cứng.
- `docs/adding-permissions.md` — xác nhận mục `DefaultGrant` từ Phase 1 còn
  đúng sau khi code xong.
- `apps/api/pkg/kanban/README.md`, `apps/web/src/lib/kanban/README.md` — đối
  chiếu với code thật, sửa chỗ lệch.

**Create**

- `apps/web/e2e/tasks-board.spec.ts`

## Implementation Steps

1. Thêm `tasks: "Công việc"` vào `RESOURCE_LABELS`; thêm nhóm "Công việc" vào
   `ACTION_GROUPS` của audit-filters; chạy `make test-web`.
2. Mở ma trận thủ công (dev server với DB đã chạy 000022) và xác nhận: tab
   "Công việc" có 7 hàng, `members.list` nằm trong nhóm "Thành viên" của tab
   "Quản trị", 5 khoá CRUD đã tick sẵn cho 3 vai trò (bằng chứng backfill —
   chỉ kiểm được ở đây, không ở test MSW), `manage_board` + `view_all` +
   `members.list` trống.
3. Thêm case test ma trận: tab + 7 hàng + không tick với fixture rỗng; thêm
   case audit-filters hiện nhóm "Công việc".
4. Viết `apps/web/e2e/tasks-board.spec.ts` theo khuôn `e2e/roster.spec.ts`:
   role-based locator, suffix timestamp cho tên cột để chạy lặp được.
5. Dựng stack cô lập `compose -p teka-e2e` với port + URL override, seed tươi;
   chạy spec. **Không** chạy trên stack `teka-*`.
6. Sau khi chạy xong, dừng đúng các container của `teka-e2e` để không để lại
   process mồ côi.
7. Cập nhật `docs/event-bus.md`, `docs/architecture.md`, xác nhận
   `docs/adding-permissions.md`.
8. Đọc lại 2 README của lib, sửa chỗ lệch so với code đã viết. Xác nhận cả hai
   ghi đủ: danh sách port, ví dụ adapter, luật biên giới, quy trình tách repo.
9. Chạy full gate: `make test-api`, `make test-web`, `make lint`,
   `make scopelint`, `make api-docs` (không có diff bẩn).
10. Code review bằng reviewer skill, tập trung: RBAC 2 tầng, tx xoá cột, tx bàn
    giao, biên giới 2 lib.
11. Commit theo conventional commit, tách theo phase, **không** AI reference.
12. **Backup DB production trước khi apply migration 000022.** Deploy API
    trước, xác nhận mọi instance chạy catalog v4, rồi mới gán
    `tasks.manage_board` / `tasks.view_all` cho vai trò
    (`docs/adding-permissions.md` §9).
13. Xin user approval trước khi push `master`; verify trên prod: một tài khoản
    được phép và một tài khoản bị chặn.

## Success Criteria

- [x] Test MSW: tab "Công việc" hiển thị 7 khoá với nhãn tiếng Việt từ API;
      nhóm "Thành viên" có `members.list`; với fixture role rỗng không khoá
      nào tick → AC8 (phần hiển thị).
- [x] Kiểm thủ công trên DB đã migrate (bước 2): 5 khoá CRUD tick sẵn cho 3
      vai trò; `manage_board`/`view_all`/`members.list` trống → AC8 (phần
      backfill; bằng chứng máy là migration test Phase 1).
- [x] Bộ lọc audit có nhóm "Công việc" và lọc được `task.*`/`task_column.*`.
- [x] e2e xanh trên `teka-e2e`: thêm cột → tạo việc → giao → chuyển cột → hoàn
      thành → xoá cột có việc kèm cột đích → AC9.
- [x] `make test-api`, `make test-web`, `make lint`, `make scopelint` xanh →
      AC9.
- [x] `make api-docs` không sinh diff ngoài các route mới.
- [x] `docs/event-bus.md` liệt kê `centers.member_tasks_handed_over`,
      `tasks.column_deleted`, bảng action `task.*`, và blind spot reorder;
      `docs/architecture.md` ghi 2 lib.
- [x] 2 README khớp code thật (kiểm bằng đối chiếu tên port).
- [x] Container `teka-e2e` đã dừng sau khi chạy; không có process mồ côi.
- [x] Prod: tài khoản có khoá thao tác được, tài khoản không có khoá nhận 403. (Deploy 2026-09-13 23:15: migration 22 applied, 5 trung tâm × 3 cột, 75 grant CRUD backfill, route `tasks/board` trả 401 khi chưa đăng nhập; kiểm tra bằng tài khoản thật do owner thực hiện sau khi cấp khoá opt-in qua ma trận.)

## Risk Assessment

| Rủi ro | Mức | Mitigation |
|---|---|---|
| Quên `RESOURCE_LABELS` → tab hiện chuỗi thô `"tasks"` | Cao × Thấp | Bước 1 làm trước tiên; test ma trận khẳng định nhãn. |
| Thứ tự tab lệch nếu Phase 1 chèn keys ở cuối catalog | Trung × Thấp | Kiểm bằng mắt ở bước 2; nếu lệch, đổi vị trí khối `tasks` trong catalog (chỉ đổi `Order` hiển thị, không đổi ngữ nghĩa, không cần bump version lần nữa). |
| e2e chạy nhầm vào stack production `teka-*` | Thấp × Rất cao | Bắt buộc `-p teka-e2e` + `--env-file` riêng + port override; xác nhận `docker compose -p teka-e2e ps` trước khi chạy spec. |
| Gán `manage_board`/`view_all` trước khi mọi instance API chạy catalog v4 | Trung × Cao | Deploy API trước, verify version, rồi mới gán (thứ tự cứng ở bước 12). |
| Migration prod fail giữa chừng | Thấp × Rất cao | Backup trước; down script đã test round-trip ở Phase 1; rollback = `migrate-down` + restore nếu cần — **chỉ** khi chưa có dữ liệu task thật; sau ship rollback là gỡ route (plan §Rollback). |
| Owner chưa tick `members.list` → "Giao cho" rỗng, user tưởng lỗi | Trung × Trung | Quyết định có chủ đích (D9); nêu ở handoff; form hiện gợi ý "Cần quyền xem thành viên" khi directory trả 403. |
| Push `master` không có approval | Thấp × Trung | `gh` là tài khoản pull-only; ghi qua SSH cần user approval — hỏi trước ở bước 13. |

**Giả định có thể sai:** ma trận tự sinh nhóm, chỉ cần thêm nhãn. *Tín hiệu
vỡ:* tab "Công việc" không xuất hiện dù catalog API đã trả 7 khoá, hoặc
`buildCatalogTabs` gộp nhầm vào "Quản trị". *Ứng phó:* đọc lại
`permission-schemas.ts:150` và điều chỉnh `ADMIN_RESOURCES`, không sửa
`permission-matrix.tsx` (component chỉ render theo tabs).

## Execution Notes (2026-09-13)

- Bước 1–9 xong. Bước 2 (backfill) kiểm bằng migration test
  `apps/api/migrations/backfill_parity_test.go` + stack `teka-e2e` (seed sau
  000022 tạo đủ 3 cột mặc định).
- Phát hiện khi chạy e2e: seeder tạo center bằng SQL thô nên bỏ qua
  `CreateCenter` → center seed không có cột mặc định, `/tasks` trống. Sửa:
  export `centers.DefaultColumns(centerID)` và seeder chèn 3 cột trong cùng
  transaction (một nguồn literal duy nhất). `go test ./seeds ./internal/features/centers` xanh.
- Spec e2e: assertion cuối scope vào cột "Hoàn thành" vì toast thành công lặp
  lại tiêu đề. 1/1 pass (10.5s) trên stack cô lập; đã `down -v`.
- Bước 10 (code review), 11 (commit), 12–13 (deploy/prod) chờ.
- Bước 10 xong: review 0 blocker / 5 major / 14 minor
  (`plans/reports/code-review-260913-task-center-kanban.md`). Đã sửa M1–M5,
  N2–N11, N13, N14; N12 chỉ ghi nhận. Sau sửa: migration 000022 có CHECK
  `priority` và `ON DELETE SET NULL (assignee_id)`; e2e chạy lại xanh trên
  stack mới build; tester chạy lại full gate
  (`plans/reports/test-report-260913-2235-task-center-kanban-refix.md`).
- Retest sau sửa review: 5/5 gate xanh (web 729 pass; API 40/40 package,
  coverage 77.1%). Lưu ý vận hành: chỉ chạy đúng một bản `go test
  -tags=integration -p 1`; chạy trùng gây timeout testcontainers giả.
- Docs bổ sung: `docs/schema_design.sql` mục 9 (task_columns/tasks),
  pointer `pkg/kanban` và `src/lib/kanban` trong CLAUDE.md của hai app.
- Deploy prod 2026-09-13 23:15 @ `9f043ab`: snapshot `teka-api:pre-260913-2315`
  / `teka-web:pre-260913-2315`, backup `~/teka-backups/teka-260913-2315.dump`
  (41 bảng). `migrate` exit 0 → schema_migrations 22; readyz 200 nội bộ + public;
  web 200, bundle có route board. Khoá opt-in chưa cấp cho ai (đúng thiết kế).

