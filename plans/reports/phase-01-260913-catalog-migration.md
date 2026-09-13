# Phase 1 — Catalog v4 + migration 000022: báo cáo hoàn thành

**Trạng thái:** completed. 11/11 Success Criteria trong
`plans/260913-1102-task-center-kanban/phase-01-api-catalog-migration.md` đã
tick. `make test-web` xanh toàn bộ sau khi Phase 6 bổ sung nhãn `tasks` vào
`RESOURCE_LABELS` (chạy song song, ngoài quyền sở hữu của Phase 1 — xem mục
"Cập nhật sau khi nhận extension").

## File đã thay đổi

**Modify**

- `apps/api/internal/shared/authctx/catalog.go` — field `DefaultGrant bool`
  trên `PermDef`; helper `optIn()` mới (đặt `DefaultGrant: false`, còn lại
  giống `def()`); 8 khoá mới (`tasks.create/list/read/edit/delete/manage_board/
  view_all`, `members.list`) chèn ngay trước `PermReportsSend` (nhóm
  `members.list` nối vào group `members` có sẵn); `DefaultRoleKeys()` thêm điều
  kiện `&& d.DefaultGrant`; `CatalogVersion` 3 → 4.
- `apps/api/internal/shared/authctx/catalog_test.go` — cập nhật version, count
  `DefaultRoleKeys()` 53 → 58, nhánh bidirectional bỏ qua `!d.DefaultGrant`,
  test allowlist khẳng định đúng 3 khoá có `DefaultGrant == false`
  (`tasks.manage_board`, `tasks.view_all` — tự động vì là `scope`,
  `members.list`), `TestScopeKeysCompleteAndHighRisk` nhận `tasks.view_all`.
  (Hoàn tất từ phiên trước, re-verify xanh trong phiên này.)
- `apps/api/migrations/migrations_test.go` — `domainTables` thêm
  `task_columns`, `tasks` (không thêm `centerTables` vì helper đó join
  `teacher_id`, hai bảng mới không có cột này); 5 test mới (xem mục "Test mới
  cho migration 000022"); sửa 2 assertion cũ (xem mục "Sai lệch").
- `apps/api/migrations/backfill_parity_test.go` — thêm hằng
  `taskBoardUpFile`, `frozenTaskDefaultKeys` (5 khoá CRUD), `frozenDefaultColumn`
  + `frozenDefaultColumns` (3 dòng cột mặc định), test
  `TestTaskBoardSQLMatchesFrozenKeysAndColumns` tái dùng `markedBlocks`/
  `keysIn` sẵn có.
- `apps/web/src/test/msw/handlers.ts` — `CATALOG_VERSION` 3 → 4; 8 entry
  `perm()` mới chèn đúng vị trí/thứ tự khớp `catalog.go` (7 khoá `tasks.*`
  trước `reports.send`, `members.list` sau `members.manage`). Đã chạy
  `prettier --write` để qua `make lint-web`.
- `docs/adding-permissions.md` — mục "2. Declare the key" bổ sung đoạn giải
  thích `DefaultGrant`/`optIn()` và lưu ý `viewAll()` báo `DefaultGrant: true`
  nhưng không bao giờ vào `DefaultRoleKeys()` vì bị lọc bởi `Kind != scope`;
  mục "4. Choose the database rollout policy" viết lại quy trình backfill hai
  bảng (`center_role_permissions` + `center_member_permissions`, tham chiếu
  `000022_task_board.up.sql` làm mẫu) và làm rõ `optIn()` không bao giờ xuất
  hiện trong migration backfill.

**Create**

- `apps/api/migrations/000022_task_board.up.sql` — bảng `task_columns`
  (`UNIQUE (id, center_id)`, unique index `(center_id, lower(name))` phân biệt
  hoa/thường-insensitive), bảng `tasks` (FK composite đặt tên
  `fk_tasks_column_center` RESTRICT, `fk_tasks_creator_center`/
  `fk_tasks_assignee_center` CASCADE); backfill 3 cột mặc định
  (`Cần làm`/`Đang làm`/`Hoàn thành`) cho mọi center sống trong khối đánh dấu
  `-- teka:default-columns-begin/-end`; backfill 5 khoá CRUD vào
  `center_role_permissions` (3 role hệ thống) và `center_member_permissions`
  (stint sống `role_id IS NULL`, không phải owner) trong khối
  `-- teka:task-keys-begin/-end`, `ON CONFLICT DO NOTHING`, ghi audit vào
  `rbac_backfill_rows` (step `task_role_defaults`/`task_member_defaults`).
- `apps/api/migrations/000022_task_board.down.sql` — xoá cả 8 khoá (5 CRUD +
  3 opt-in) khỏi cả hai bảng quyền vô điều kiện, xoá dòng audit tương ứng,
  drop `tasks` rồi `task_columns` (thứ tự vì FK RESTRICT); có cảnh báo đầu
  file: chỉ an toàn để rollback trước khi feature lên production.

## Quyết định thiết kế / lệch có chủ đích so với phase file

1. **Không ghi vào `rbac_backfill_ledger`.** 000018 dùng bảng này làm snapshot
   một-lần của chính SQL bất biến nó ship; 000022 chỉ ghi vào
   `rbac_backfill_rows` (bảng audit tổng quát, không phải ledger theo-migration
   của riêng 000018). Quyết định này giữ nguyên từ phiên trước; phase file
   không bắt buộc ghi ledger, chỉ yêu cầu "copy khuôn 000018" cho cách backfill
   hai bảng, đã tuân thủ đúng phần đó.
2. **Sửa `TestResourceActionCatalogBackfill`** (dòng ~1813-1817 trước sửa):
   assertion cũ so `ledger.RoleDefaultRows`/`MemberDefaultRows` với
   `len(defaults)` — tức `authctx.DefaultRoleKeys()` **sống**, giờ đã lên 58 vì
   có 5 khoá `tasks.*` mới. Ledger của 000018 là con số cố định ghi một lần bởi
   SQL bất biến của chính nó (53 khoá tại thời điểm 000018 chạy), không bao giờ
   được cập nhật lại bởi migration sau. Sửa thành so với `len(frozenDefaultKeys)`
   (literal đóng băng đã có sẵn trong `backfill_parity_test.go`, = 53). Đây là
   sửa lỗi tính đúng của test (assertion cũ vô tình gắn vào giá trị sống), không
   phải nới lỏng — đã ghi lý do bằng comment tại chỗ.
3. **Sửa `TestDownFoldsPersonalChannelIntoManual`**: comment của chính test này
   ghi rõ số bước `MigrateDown` phải tăng mỗi khi có migration cộng thêm phía
   trên; tăng 17 → 18 và cập nhật comment "000008-000021" → "000008-000022" vì
   thêm 000022. Cùng loại sửa bảo trì đã được chính test dự liệu, không phải
   nới lỏng assertion.

## Test mới cho migration 000022 (`migrations_test.go`)

- `TestTaskColumnsUniqueNameCaseInsensitive`
- `TestTaskGuardsRejectCrossCenterRows`
- `TestTaskColumnDeleteRestrictedBySoftDeletedTask` (RESTRICT chặn cả khi task
  đã soft-delete — Postgres FK không biết `deleted_at`)
- `TestTaskBoardBackfillGrantsRoleLessMemberCRUD` (dùng closure `member` an
  toàn với head schema, không dùng helper `rbacMember` cũ vì cột
  `can_send_reports` đã bị 000019 xoá)
- `TestTaskBoardBackfillSeedsThreeDefaultColumns`

## Gates

| Gate | Kết quả |
|---|---|
| `go test ./internal/shared/authctx/...` | Xanh, mọi subtest pass. |
| `go test ./migrations/...` (Docker, testcontainers) | Xanh — 27/27 test pass sau 2 lần sửa (mục "Quyết định thiết kế" #2, #3). |
| `go vet ./...` | Xanh. |
| `make lint-api` | Xanh (0 issue). |
| `make test-api-unit` | Xanh. |
| `make lint-web` | Xanh (0 error, 5 warning có sẵn không liên quan). |
| `make test-web` | Xanh toàn bộ — 89 file test, 694 pass, 3 skip, 0 fail. |

## Cập nhật sau khi nhận extension quyền sở hữu

Team-lead mở rộng quyền cho Phase 1 được sửa
`apps/web/src/features/center/__tests__/center-schemas.test.ts` (chỉ để thêm
tab/tài nguyên mới vào danh sách kỳ vọng), vẫn giữ nguyên lệnh cấm sửa
`permission-schemas.ts`. Khi đọc lại hai file này để thực hiện, phát hiện cả
hai **đã có sẵn thay đổi** trong working tree không phải do phiên này tạo ra:
`permission-schemas.ts` đã có `tasks: "Công việc"` trong `RESOURCE_LABELS`, và
`center-schemas.test.ts` đã có `"Công việc"` đúng vị trí (trước "Quản trị")
trong mảng kỳ vọng của `buildCatalogTabs`. Đây rõ ràng là kết quả của Phase 6
chạy song song trong cùng worktree. Do đó:

- Không sửa `permission-schemas.ts` (đúng theo lệnh cấm, và nó đã đúng).
- Không cần sửa thêm `center-schemas.test.ts` — trạng thái hiện tại của nó đã
  khớp với `permission-schemas.ts` và test đã xanh; sửa lại sẽ là thao tác
  thừa/trùng lặp với việc Phase 6 đã làm.
- Chạy lại `npx vitest run center-schemas.test.ts` (12/12 pass) và toàn bộ
  `make test-web` (694/694 pass, 3 skip) để xác nhận bằng thực nghiệm, không
  chỉ dựa vào suy luận từ diff.
- `make lint-web` chạy lại vẫn xanh (0 error).

Bước 15 của Implementation Steps (`make migrate-up` thủ công trên môi trường
dev sau khi backup DB) không thực hiện được trong phiên agent này — không có
môi trường dev chạy sẵn/DB thật ngoài container test tạm thời của testcontainers.
Migration round-trip (up→down→up) đã được verify tự động qua Postgres thật
trong bộ test `./migrations/...` (không phải "dev" theo nghĩa vận hành thủ
công) — đề nghị team-lead/chủ dự án chạy bước 15 thủ công trước khi merge lên
nhánh chính nếu cần thêm một lớp xác nhận vận hành.

## Câu hỏi chưa giải quyết

Không có. `RESOURCE_LABELS` cho `tasks` đã được Phase 6 bổ sung (song song,
ngoài phiên này) và `make test-web` đã xanh toàn bộ; mục theo dõi trước đó đã
đóng.

Status: DONE
Summary: Catalog v4 (79 khoá, 8 khoá mới, `DefaultGrant`/`optIn()`) và migration 000022 (2 bảng, backfill cột + quyền 2 bảng) hoàn chỉnh; toàn bộ gate API và web đều xanh (bao gồm `make test-web` sau khi Phase 6 bổ sung `RESOURCE_LABELS` cho `tasks` song song).
Concerns/Blockers: Bước 15 (migrate-up thủ công trên môi trường dev thật sau backup DB) chưa thực hiện được trong phiên agent này — không có DB dev thực ngoài testcontainers; đề nghị người có quyền truy cập môi trường dev xác nhận thủ công trước khi merge nếu cần thêm một lớp kiểm chứng vận hành.
