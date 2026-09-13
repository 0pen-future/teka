---
phase: 1
title: "Catalog v4 + migration 000022"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Catalog v4 + migration 000022

## Overview

Mở đường cho toàn bộ feature: 8 khoá quyền mới, một attribute mới trên `PermDef`
để một khoá bất kỳ có thể opt-in khỏi backfill (v1: `tasks.manage_board`,
`members.list`), `CatalogVersion` 3 → 4, và
migration 000022 tạo 2 bảng + backfill cột mặc định + backfill quyền. Không có
endpoint, không có handler — phase này chỉ đổi vocabulary quyền và schema.

Đây là phase chạm vào contract dùng chung (catalog + CAS version + fixture web),
nên nó độc lập và đi trước mọi thứ khác về mặt dependency của Phase 3.

## Requirements

### Functional

- 8 khoá mới trong catalog: `tasks.create` (crud/low), `tasks.list` (crud/low),
  `tasks.read` (crud/low), `tasks.edit` (crud/low), `tasks.delete`
  (crud/medium), `tasks.manage_board` (special/high, opt-in),
  `tasks.view_all` (scope/high), `members.list` (crud/low).
- `PermDef` có field `DefaultGrant bool`. Mọi khoá hiện tại giữ hành vi cũ
  (default `true` khi khai báo qua helper `def`/`viewAll`); đúng **hai** khoá
  khai báo `DefaultGrant: false`: `tasks.manage_board` và `members.list`
  *(Red-team + user chốt: `members.list` mở danh bạ trên dữ liệu đã có của mọi
  trung tâm mà owner không được hỏi → opt-in; xem plan D9/D11)*.
- `DefaultRoleKeys()` lọc thêm `d.DefaultGrant`; kết quả: 53 → 58 khoá
  (5 CRUD `tasks.create/list/read/edit/delete`).
- Migration 000022 tạo `task_columns`, `tasks`, backfill 3 cột mặc định cho mọi
  center `deleted_at IS NULL`, backfill 5 khoá CRUD vào **hai** bảng theo khuôn
  000018: `center_role_permissions` cho 3 vai trò hệ thống (`:112`) **và**
  `center_member_permissions` cho stint sống có `role_id IS NULL` (`:174`),
  `ON CONFLICT DO NOTHING`. *(Red-team: thiếu nhánh member-level thì stint
  không role mất toàn bộ `tasks.*` và khối role-less trong
  `migrations_test.go:1765-1772` đỏ vì nó đếm theo `DefaultRoleKeys()`.)*
- Down migration đảo ngược đúng: drop 2 bảng; xoá **cả 8 khoá** `tasks.*` +
  `members.list` khỏi hai bảng quyền (gồm khoá opt-in owner đã gán tay — nếu để
  lại, catalog v3 gặp khoá lạ).
- MSW fixture web mirror `catalog_version: 4` và 8 entry catalog mới.

### Non-functional

- Migration bất biến (không sửa file 000018–000021).
- Parity test chốt theo đúng convention file: so SQL backfill của 000022 với
  một **literal đóng băng** trong test (5 khoá CRUD + khối `-- default-columns`
  3 dòng), không so với `DefaultRoleKeys()` sống — catalog đổi sau này không
  được làm đổi ý nghĩa của migration đã apply.
- Không đổi ngữ nghĩa của bất kỳ khoá cũ nào → không cần data migration cho
  assignment đang tồn tại.

## Architecture

**Pattern: Registry + Frozen Snapshot.** Catalog là registry code-owned
(`permCatalog` slice); DB chỉ lưu phép gán. Migration backfill là *snapshot đông
lạnh* của registry tại thời điểm apply — parity test so checksum giữa hai bên,
đúng khuôn `backfill_parity_test.go` đã dùng cho 000018.

**SOLID (OCP):** thêm `DefaultGrant` thay vì mở rộng `legacyIdentitySet` bằng
tay. Khoá `special` tương lai chỉ cần khai báo attribute, không sửa hàm lọc.

**Trade-off:** `DefaultGrant` là field thứ 3 điều khiển backfill (cùng
`Grantable` và `Kind != scope`), tăng bề mặt suy luận. Bù lại nó tường minh tại
điểm khai báo, còn `legacyIdentitySet` là một map tách rời dễ quên đồng bộ.

**Testability:** hàm lọc thuần, không I/O → test bảng trong `catalog_test.go`.
Migration test chạy trên Postgres thật qua testcontainers.

**Thứ tự chèn keys:** đặt khối `tasks` **ngay trước** `PermReportsSend`
(`apps/api/internal/shared/authctx/catalog.go:232` vùng khối admin) để nhóm
"Công việc" nhận một tab riêng ở ma trận web, còn `members.list` nối vào nhóm
`members` sẵn có. Chèn ở cuối catalog sẽ đẩy nhóm mới xuống sau tab "Quản trị".

## Related Code Files

**Modify**

- `apps/api/internal/shared/authctx/catalog.go` — thêm 8 hằng khoá + 8 `def`
  entry, field `DefaultGrant` (`:34` PermDef), `DefaultRoleKeys()` (`:330`),
  `CatalogVersion` (`:294`).
- `apps/api/internal/shared/authctx/catalog_test.go` — `TestCatalogVersion`
  (`:298`) 3→4; `TestDefaultRoleKeysPreserveLegacyBaseline` (`:246`) count
  53→58 và nhánh bidirectional bỏ qua `!d.DefaultGrant`;
  `TestScopeKeysCompleteAndHighRisk` (`:123`) nhận `tasks.view_all`; test mới
  khẳng định mọi entry có `DefaultGrant == true` trừ allowlist tường minh
  `{tasks.manage_board, members.list}` (chặn quên set khi thêm helper).
- `apps/api/migrations/embed.go` — tự embed theo glob, kiểm tra không cần sửa.
- `apps/api/migrations/migrations_test.go` — `domainTables` (`:26`) thêm
  `task_columns`, `tasks`. **Không** thêm vào `centerTables` (`:44`): helper đó
  join `x.teacher_id` (`:515-522`), hai bảng mới không có cột này → panic SQL.
  Tenancy của bảng mới kiểm bằng assertion riêng theo `center_id`. Khối
  role-less (`:1765-1772`) tự xanh khi backfill member-level đúng, không sửa
  số kỳ vọng bằng tay.
- `apps/api/migrations/backfill_parity_test.go` — thêm hằng file 000022 và case
  parity cho tập khoá mới (mẫu: `TestBackfillSQLMatchesFrozenCatalogV2` `:110`).
- `apps/web/src/test/msw/handlers.ts` — `CATALOG_VERSION` (`:17`) 3→4; bổ sung
  8 entry vào fixture catalog.
- `docs/adding-permissions.md` — mục "2. Declare the key" (`:44`) và
  "4. Choose the database rollout policy" (`:108`) mô tả `DefaultGrant`.

**Create**

- `apps/api/migrations/000022_task_board.up.sql`
- `apps/api/migrations/000022_task_board.down.sql`

## Implementation Steps

1. Đọc `apps/api/internal/shared/authctx/catalog.go` toàn bộ; xác định helper
   `def()`, `viewAll()` và vị trí chèn ngay trước `PermReportsSend`.
2. Thêm field `DefaultGrant bool` vào `PermDef`; đặt `DefaultGrant: true` trong
   thân `def()` và `viewAll()` để mọi khai báo cũ giữ nguyên hành vi.
3. Thêm helper `optIn()` (hoặc biến thể của `def` nhận `DefaultGrant: false`)
   và khai báo 8 khoá mới đúng Kind/Risk/Label **và Description** tiếng Việt
   theo brief §4 (`TestCatalogWellFormed` `:59-61` fail nếu thiếu Description).
   `members.list` khai báo qua `optIn()`.
4. Sửa `DefaultRoleKeys()` thêm điều kiện `d.DefaultGrant`; giữ nguyên hai điều
   kiện cũ.
5. Bump `CatalogVersion` 3 → 4 kèm comment nêu lý do (thêm nhóm tasks +
   attribute `DefaultGrant`).
6. Cập nhật `catalog_test.go`: version, count 58, nhánh bidirectional bỏ qua
   `!d.DefaultGrant`, assert `tasks.manage_board` và `members.list` không nằm
   trong `DefaultRoleKeys()`, và test allowlist: duyệt catalog, mọi entry
   `DefaultGrant == true` trừ đúng 2 khoá trên.
7. Chạy `go test ./internal/shared/authctx/...` cho tới khi xanh.
8. Viết `000022_task_board.up.sql` đúng DDL trong brief §4: `task_columns` với
   `UNIQUE (id, center_id)`, unique index `(center_id, lower(name))`, index
   `(center_id, position)`; `tasks` với FK composite `(column_id, center_id)`
   RESTRICT, `(created_by, center_id)` và `(assignee_id, center_id)` CASCADE,
   `position DOUBLE PRECISION`, `deleted_at`, 3 index.
9. Thêm backfill cột mặc định trong khối đánh dấu `-- default-columns` (CROSS
   JOIN VALUES 3 dòng, tên tiếng Việt do adapter Teka sở hữu — cùng literal
   với `internal/features/centers/default_columns.go` ở Phase 3) và backfill 5
   khoá CRUD vào `center_role_permissions` cho 3 vai trò hệ thống **và**
   `center_member_permissions` cho stint `role_id IS NULL`, `deleted_at IS
   NULL` (copy cấu trúc 000018 `:112` và `:174`), `ON CONFLICT DO NOTHING`.
10. Viết `000022_task_board.down.sql`: `DELETE` cả 8 khoá `tasks.*` +
    `members.list` khỏi `center_role_permissions` **và**
    `center_member_permissions`, `DROP TABLE tasks`, `DROP TABLE task_columns`
    (thứ tự này vì FK RESTRICT). Ghi rõ đầu file: chỉ dùng trước khi ship —
    sau ship rollback bằng gỡ route (plan §Rollback).
11. Cập nhật `migrations_test.go` (`domainTables` — không `centerTables`) và
    thêm test khẳng định: unique tên cột không phân biệt hoa thường; FK
    composite chặn task trỏ cột khác center; RESTRICT chặn xoá cột còn việc,
    **kể cả khi việc đó đã soft-delete** (`deleted_at` không làm FK bỏ qua
    dòng — nền cho cách đếm ở Phase 2/3); stint không role của center có sẵn
    nhận đủ 5 khoá CRUD sau backfill; mọi dòng `task_columns`/`tasks` có
    `center_id` thuộc `centers`.
12. Mở rộng `backfill_parity_test.go` cho 000022 với literal đóng băng (5 khoá
    + 3 dòng default-columns) theo khuôn `:110`.
13. Cập nhật `apps/web/src/test/msw/handlers.ts`: `CATALOG_VERSION = 4` + 8
    entry catalog (key, resource, action, kind, risk, label) khớp API.
14. Cập nhật `docs/adding-permissions.md` mô tả `DefaultGrant` và khi nào dùng.
15. **Backup DB trước khi apply** (rule dự án), rồi `make migrate-up` trên môi
    trường dev để kiểm chứng thủ công.
16. Gates: `go test ./internal/shared/authctx/... ./migrations/...`,
    `make test-api`, `make test-web`, `make lint-api`, `make lint-web`.

## Success Criteria

- [x] `authctx.CatalogVersion == 4`; `len(DefaultRoleKeys()) == 58`.
- [x] `DefaultRoleKeys()` không chứa `tasks.manage_board`, `tasks.view_all`,
      `members.list` → AC8 (phần opt-in); test allowlist xanh.
- [x] `go test ./internal/shared/authctx/... ./migrations/...` xanh; test cũ
      chỉ đổi ở chỗ đã liệt kê trong Modify (không nới lỏng assertion).
- [x] Migration round-trip up→down→up xanh trên Postgres thật → AC7.
- [x] Sau `migrate-up`, mọi center sống có đúng 3 cột, position 0/1/2, cột thứ 3
      `is_done = TRUE` → AC7.
- [x] `INSERT` task trỏ cột thuộc center khác bị FK chặn.
- [x] `DELETE` cột còn việc (kể cả việc soft-deleted) bị RESTRICT chặn → nền
      cho AC5.
- [x] Stint `role_id IS NULL` của center có sẵn có đủ 5 khoá CRUD trong
      `center_member_permissions`; khối role-less `migrations_test.go:1765`
      xanh không sửa số.
- [x] Parity test 000022 xanh với literal đóng băng.
- [x] Down script xoá sạch 8 khoá khỏi hai bảng; round-trip không để khoá lạ.
- [x] `make test-web` xanh với `CATALOG_VERSION = 4`. Phase 6 đã bổ sung
      `tasks: "Công việc"` vào `RESOURCE_LABELS`
      (`apps/web/src/features/center/schemas/permission-schemas.ts`, ngoài
      quyền sở hữu của Phase 1 — không sửa file này) song song với phiên này;
      `center-schemas.test.ts > buildCatalogTabs` đã được cập nhật khớp (tab
      "Công việc" chèn trước "Quản trị") và `make test-web` nay xanh toàn bộ
      (89 file test, 694 pass, 3 skip, 0 fail).

## Risk Assessment

| Rủi ro | Mức | Mitigation |
|---|---|---|
| Nhánh bidirectional trong `TestDefaultRoleKeysPreserveLegacyBaseline` fail | Cao × Trung | Sửa test cùng commit; đây là điều chỉnh contract có chủ đích, ghi lý do vào comment test. |
| Chèn keys giữa catalog làm đổi `Order` của keys sau nó | Trung × Thấp | `Order` chỉ dùng để hiển thị, không lưu DB (`catalog.go:250` gán từ vị trí). Chạy `TestEffectiveKeysCoversCatalogInOrder` để chốt. |
| Backfill quyền chạy trên center có role bị owner đổi tên/xoá | Thấp × Trung | Backfill join theo `center_roles.key` hệ thống, `ON CONFLICT DO NOTHING`; không tạo role mới. |
| Quên nhánh `center_member_permissions` → stint không role mất `tasks.*` | Trung × Cao | Bước 9 copy đủ hai khối của 000018; test ở bước 11 khẳng định trực tiếp trên stint không role. |
| Parity test so với `DefaultRoleKeys()` sống → catalog v5 làm test 000022 đỏ hoặc che lệch | Trung × Trung | Literal đóng băng trong test (convention `:110`). |
| Unique `lower(name)` va chạm với tên cột mặc định đã tồn tại (chạy lại migration) | Thấp × Thấp | Backfill dùng `ON CONFLICT DO NOTHING` trên index tên; migration chỉ chạy một lần theo `schema_migrations`. |
| Web fixture lệch version → toàn bộ test permission 409 | Trung × Trung | Cập nhật fixture cùng phase; `make test-web` là gate bắt buộc của phase. |

**Giả định có thể sai:** `DefaultGrant` là cách đúng thay vì mở rộng
`legacyIdentitySet`. *Tín hiệu vỡ:* phải thêm nhánh điều kiện thứ tư cho khoá
tiếp theo, hoặc `DefaultGrant` và `legacyIdentitySet` cho kết quả mâu thuẫn.
*Ứng phó:* gộp `legacyIdentitySet` thành `DefaultGrant: false` tại điểm khai báo
trong một phase dọn dẹp riêng, không mở rộng thêm cơ chế thứ ba.
