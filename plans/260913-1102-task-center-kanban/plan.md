---
title: "Trung tâm công việc — Kanban cột cấu hình"
description: "Bảng Kanban theo trung tâm với cột tự cấu hình, phân quyền qua catalog, lõi nghiệp vụ tách thành lib tái sử dụng (pkg/kanban + src/lib/kanban)"
status: completed
priority: P1
effort: "6.5d"
tags: [api, web, kanban, rbac, lib]
created: 2026-09-13
blockedBy: []
blocks: []
branch: master
---

# Trung tâm công việc — Kanban cột cấu hình

## Overview

Thêm mục điều hướng "Công việc" mở bảng Kanban của trung tâm. Mỗi trung tâm có 3
cột mặc định (Cần làm → Đang làm → Hoàn thành), owner hoặc người có
`tasks.manage_board` tự thêm/đổi tên/sắp xếp/xoá cột (trần 8). Thành viên tạo
việc, giao việc, đặt hạn/ưu tiên, chuyển cột. Phân quyền đi qua catalog + vai trò
đã có; mọi thao tác ghi để lại dòng audit.

Điểm khác so với các feature trước: **lõi nghiệp vụ Kanban tách thành lib biên
giới cứng** — `apps/api/pkg/kanban` (Go, chỉ stdlib + `uuid`) và
`apps/web/src/lib/kanban` (TS headless, chỉ `react`). Feature Teka chỉ là
adapter. Trích xuất sang repo khác sau này = `git mv`, không refactor.

Hợp đồng đã chốt: `./reports/brainstorm-brief-rev3-extracted.md`.
Bằng chứng repo: `plans/reports/scout-260913-1757-api-task-board.md`,
`plans/reports/scout-web-260913-1757-task-board.md`.
Nghiên cứu pattern: `plans/reports/researcher-260913-1757-go-kanban-lib-patterns.md`,
`plans/reports/researcher-260913-1757-react-headless-kanban-patterns.md`.

## Environment

- Monorepo, không workspace tooling. Docker build context riêng cho `apps/api`,
  `apps/web`; CI path filter theo app. Kế hoạch này **không** đụng Docker/CI.
- API: Go 1.25, module `teka/apps/api`, Gin + GORM + Postgres, golang-migrate.
- Web: React 19, TanStack Query 5, Zustand, Zod 4, Tailwind 4, Vitest + MSW 2,
  Playwright. Alias `@/*` → `apps/web/src/*`.
- Branch: `master`. Migration kế tiếp: `000022`.
- Gates: `make test-api`, `make test-api-unit`, `make scopelint`, `make lint-api`,
  `make api-docs`, `make test-web`, `make lint-web`, `make e2e`.

## Decisions (đã chốt, không mở lại)

| # | Quyết định | Nội dung |
|---|-----------|----------|
| D1 | Lib trong app, biên giới cứng | Go `apps/api/pkg/kanban` import path `teka/apps/api/pkg/kanban`, chỉ stdlib + `github.com/google/uuid`. Web `apps/web/src/lib/kanban`, chỉ `react`. Không tạo go.mod/package.json riêng. |
| D2 | Enforce biên giới | Go: test `go list -deps` chặn `internal/*`, gin, gorm. Web: ESLint `no-restricted-imports` (precedent tại `apps/web/eslint.config.js:45`). Mỗi lib có README mô tả ports. |
| D3 | Policy 2 tầng | Tầng app: middleware + `routespec` kiểm capability key. Tầng core: `Policy`/`Actor` (Strategy) là **nơi duy nhất** giữ luật object-level: read = `view_all ∨ created_by=me ∨ assignee_id=me`; writeOwn = `owner ∨ creator`; writeParticipant = `owner ∨ creator ∨ assignee`; manage_board = `owner ∨ key`. Repository **không** nhận `Scope`, không đọc `IsOwner`/`Has`; core dịch luật read thành `Visibility` (all hoặc participant-of-actor) truyền xuống repo. *(Red-team: brief ghi predicate SQL trong repository; gộp về một nơi để không giữ hai bản luật.)* |
| D4 | ID types | `TenantID`, `ActorID`, `TaskID`, `ColumnID` là kiểu định nghĩa mới trên `uuid.UUID` trong core. |
| D5 | Default columns | Core expose factory `DefaultColumns(specs)` gán position; **tên cột** thuộc adapter Teka (`internal/features/centers/default_columns.go`, cạnh `CreateCenter`), lib không chứa chuỗi ngôn ngữ. Adapter seed trong `CreateCenter` (cùng tx) + migration backfill cho center hiện có; parity test so khối `-- default-columns` của 000022 với literal đóng băng. |
| D6 | Unit of Work | Core định nghĩa port `UnitOfWork`; adapter map sang `database.TxManager.WithinTx` + `database.FromContext`. |
| D7 | Domain events | Core phát `EventSink` **chỉ** trong use-case mà core mở `uow.Within` ở tầng ngoài cùng (v1: một event `ColumnDeleted{ColumnID, MoveTo, MovedCount}`). Bàn giao **không** phát từ core: `HandoverOnDeparture` trả counts, `centers.RemoveMember` publish `centers.MemberTasksHandedOver` sau khi `WithinTx` ngoài cùng trả nil (khuôn `ReplaceRolePermissions` `centers/service.go:463-490`). Audit: 2 `case` mới trong `audit/subscriber.go`. Import: `audit` → `tasks`/`centers`, không ngược lại. |
| D8 | TaskHandover | Interface khai báo phía consumer trong `centers`, **nil-safe** (chưa set → log Warn, offboarding vẫn chạy — thu hồi quyền quan trọng hơn bàn giao). Inject bằng setter trong `server.registerFeatures` (đúng brief §4) ngay sau khi dựng `tasksSvc`; `container.go` không đổi, `NewRouter` giữ chữ ký. Gọi **trước** `CloseMembership` trong cùng tx của `RemoveMember`. |
| D9 | Catalog | `CatalogVersion` 3 → 4; thêm 8 keys; thêm attribute `DefaultGrant` trên `PermDef` để loại `tasks.manage_board` **và `members.list`** khỏi backfill (`members.list` opt-in: mở danh bạ trên dữ liệu đã có cho mọi trung tâm mà owner không được hỏi — user chốt 13/09/2026). `DefaultRoleKeys()` 53 → 58. MSW fixture mirror version 4. |
| D10 | Đồng thời | Không CAS cho cột ở v1. Rename/toggle = last-write-wins; reorder và move fail-closed (409/404) → refetch. Lỗi ràng buộc DB (unique tên cột `23505`, FK RESTRICT `23503`) dịch qua `translateDBError` thành 422/409, không bao giờ lộ 500. Trần 8 cột giữ bằng advisory lock theo tenant trong adapter. |
| D11 | Web: một nguồn sự thật | Cache TanStack là nguồn sự thật duy nhất của board. `useKanban` không giữ `useReducer`; `kanbanReducer` thuần dùng để tính optimistic state cho `setQueryData`. Lỗi mutation bất kỳ → `invalidateQueries`, không rollback snapshot; 9 mutation dùng chung `scope` để chạy tuần tự. |

## Pattern map

| Pattern | Áp dụng ở | SOLID | Lợi ích / trade-off |
|---|---|---|---|
| Repository (Port) | `kanban.ColumnRepository`, `kanban.TaskRepository` (Phase 2) | DIP | Core test bằng fake in-memory, không cần Postgres. Trade-off: 2 lớp signature phải giữ đồng bộ. |
| Adapter | `internal/features/tasks/*_repository.go` (Phase 3), `features/tasks/kanban-data-source.ts` (Phase 5) | DIP, SRP | Cô lập GORM/HTTP khỏi lõi. Trade-off: thêm 1 lớp mapping. |
| Unit of Work | `kanban.UnitOfWork` → `database.TxManager` (Phase 2/3) | DIP | Atomicity xuyên nhiều repo mà core không biết SQL. Trade-off: cấm nested tx, chỉ top-level use-case mở tx. |
| Strategy | `kanban.Policy` từ `authctx.Scope` (Phase 2/3) | OCP, DIP | Đổi luật authz không sửa Service. Trade-off: dễ trùng lặp với gate ở middleware nếu không phân vai rõ (D3). |
| Observer | `kanban.EventSink` → event bus → `audit/subscriber.go` (Phase 2/3) | SRP, OCP | Audit tách khỏi use-case cho dữ liệu middleware không thấy (`move_to`). Trade-off: bus at-most-once, event có thể rơi khi buffer đầy; chỉ publish khi core sở hữu tx ngoài cùng. |
| Factory | `kanban.DefaultColumns(specs)`, `NewService(...)` (Phase 2) | SRP | Trung tâm mới và trung tâm backfill sinh cột giống hệt nhau — bảo đảm bằng parity test literal, không chỉ bằng tên hàm. |
| Functional Options | `kanban.Option`, `WithMaxColumns`, `WithClock` (Phase 2) | OCP | Thêm option không phá compat. Test bơm `Clock` cố định để assert `completed_at`. |
| Reducer + Selectors | `src/lib/kanban/state.ts` (Phase 4) | SRP | Test thuần input→output, không render. Reducer là hàm tính optimistic state cho cache, **không** là state trong hook (D11). Trade-off: cần memoize selector. |
| Prop Getters | `getColumnProps` / `getTaskProps` (Phase 4) | OCP | App compose thêm handler mà không chạm logic a11y. Trade-off: API "ẩn", cần README. |
| Command | mỗi method của `KanbanDataSource` (Phase 4/5) | ISP | Mỗi intent map 1 mutation, dễ test riêng. Trade-off: nhiều method hơn một API update chung. |
| Presenter (Strategy) | `board-desktop.tsx` / `board-mobile.tsx` dùng chung `useKanban` (Phase 5) | SRP | Không nhân bản logic move. Trade-off: presenter phải "dumb" tuyệt đối. |

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Board Kanban cột cấu hình chạy end-to-end, phân quyền fail-closed qua catalog | P1 |
| 2 | Lõi Kanban tách thành 2 lib biên giới cứng, có test enforce biên giới + README | P1 |
| 3 | Rời trung tâm không mất việc: bàn giao trong cùng tx `RemoveMember` | P1 |
| 4 | Mọi thao tác ghi (việc + cột) có dòng audit; manifest tests fail-closed xanh | P1 |
| 5 | Bàn phím dùng được toàn bộ board (không DnD), desktop + mobile | P2 |

## Phases

| # | Phase | Status | Dependencies |
|---|-------|--------|--------------|
| 1 | [Catalog v4 + migration 000022](./phase-01-api-catalog-migration.md) | Completed | — |
| 2 | [Go kanban core lib](./phase-02-go-kanban-core-lib.md) | Completed | — |
| 3 | [API tasks feature adapter](./phase-03-api-tasks-feature-adapter.md) | Completed | 1, 2 |
| 4 | [Web headless kanban lib](./phase-04-web-headless-kanban-lib.md) | Completed | — |
| 5 | [Web tasks feature](./phase-05-web-tasks-feature.md) | Completed | 3, 4 |
| 6 | [Matrix, e2e, docs, ship](./phase-06-matrix-e2e-docs-ship.md) | Completed | 5 |

Phase 1, 2, 4 độc lập nhau → chạy song song được. File ownership không chồng:
Phase 1 chỉ chạm `authctx` + `migrations`, Phase 2 chỉ chạm `pkg/kanban`,
Phase 4 chỉ chạm `src/lib/kanban` + `eslint.config.js`.

Ghi chú cross-plan: `plans/260829-1640-gh-260829-flexible-center-rbac` còn
`status: in-progress` nhưng mọi phase của nó đã deploy → không chặn.

## Acceptance Criteria

- [x] **AC1** Owner không cần gán gì vẫn CRUD việc, cấu hình cột, xem toàn board.
- [x] **AC2** Thành viên có `tasks.list` thấy việc mình tạo/được giao; thêm
      `tasks.view_all` thấy toàn trung tâm; trung tâm khác không thấy gì.
- [x] **AC3** Người được giao chuyển được cột việc của mình dù không phải người
      tạo; sửa nội dung/xoá chỉ creator hoặc owner, còn lại 403.
- [x] **AC4** Thiếu `tasks.manage_board` → mọi API cột 403 và nút "Cấu hình cột"
      ẩn; có khoá → tạo/đổi tên/sắp xếp/xoá cột thành công.
- [x] **AC5** Xoá cột đang có việc bắt buộc kèm `move_to`; di dời + xoá trong một
      transaction; thiếu `move_to` → 409; không xoá được cột cuối cùng; audit
      `task_column.delete` ghi `move_to` + số việc di dời.
- [x] **AC6** Chuyển việc vào cột `is_done` ghi `completed_at`; chuyển ra xoá;
      đổi cờ `is_done` của cột không viết lại lịch sử việc cũ.
- [x] **AC7** Trung tâm mới có sẵn 3 cột mặc định; migration backfill mọi trung
      tâm đang sống; parity test xanh.
- [x] **AC8** Ma trận Phân quyền hiển thị nhóm "Công việc" + `members.list` với
      nhãn từ API; 5 khoá CRUD backfill cho 3 vai trò và stint không role;
      `manage_board`, `view_all`, `members.list` trống (opt-in).
- [x] **AC9** Audit ghi `task.*` và `task_column.*`; manifest tests xanh;
      `make test-api`, `make test-web`, `make lint`, `make scopelint` xanh; e2e
      1 luồng đầy đủ.
- [x] **AC10** Xoá thành viên: cùng tx, việc được giao có `assignee_id = NULL`,
      việc họ tạo có `created_by = owner`; audit `task.handover` ghi số việc bàn
      giao **và chỉ ghi sau khi tx commit**; mời lại không hoàn tác.
- [x] **AC-LIB** `pkg/kanban` không import `internal/*`/gin/gorm (test chặn);
      `src/lib/kanban` không import `@/features/*`, `@/components/*`,
      `@/lib/api`, tanstack (ESLint chặn); mỗi lib có README ports.

## Progress (2026-09-13)

- Phase 1–5 hoàn tất; Phase 6 còn bước 12–13 (backup DB, migration 000022,
  deploy, verify prod). Hợp đồng API chốt tại
  `reports/api-contract-v1.md` (đã chỉnh 403/404 cho `GET /tasks/:id`).
- Code review: 0 blocker, 5 major + 14 minor → đã sửa M1–M5, N2–N11, N13,
  N14 (N12 chỉ ghi nhận: `getTask`/`tasks.read` chưa có màn hình dùng).
  Báo cáo: `plans/reports/code-review-260913-task-center-kanban.md`,
  `fix-review-api-…`, `fix-review-web-…`.
- Gate: scopelint, lint, test-web (729 pass), test-api (77.1%), api-docs
  không drift, e2e `tasks-board.spec.ts` xanh trên stack `teka-e2e` (đã gỡ).

## Non-goals

Nhiều board mỗi trung tâm, swimlane, WIP limit, màu/icon cột, workflow rules.
Bình luận, đính kèm, checklist con, nhắc việc qua Zalo. Liên kết việc với
lớp/học sinh/buổi. Kéo-thả chuột. CAS cho cấu hình cột. Báo cáo/thống kê công
việc, escalation quá hạn. Thay đổi Docker/CI/workspace tooling. Tách lib thành
module/package riêng ở v1.

## Risks

| Rủi ro | Phase | Mitigation |
|---|---|---|
| `DefaultGrant` phá `TestDefaultRoleKeysPreserveLegacyBaseline` (nhánh bidirectional) | 1 | Cập nhật test cùng lúc với catalog; count 53 → 58; test allowlist chốt đúng 2 khoá opt-in. |
| 000022 chỉ backfill role → stint không role mất `tasks.*`, khối role-less `migrations_test.go:1767` đỏ | 1 | Backfill hai bảng theo khuôn 000018 (`:174`); khối role-less tự xanh vì đếm theo `DefaultRoleKeys()`. |
| Backfill quyền trong SQL lệch với `DefaultRoleKeys()` | 1 | `migrations/backfill_parity_test.go` mở rộng cho 000022. |
| Quên wiring `TaskHandover` → việc trỏ stint đã đóng | 3 | Nil-safe + log Warn (không chặn thu hồi quyền); test wiring trong package `server` dựng router thật + integration test xoá thành viên. |
| Event bàn giao publish trước commit (WithinTx lồng chỉ `return fn(ctx)`, `tx.go:23-30`) | 2, 3 | Core không publish cho use-case chạy trong tx của consumer; `centers` publish sau `WithinTx` ngoài cùng. Test: `Disable` lỗi → 0 audit row. |
| scopelint R1 không bắn cho repo nhận `TenantID`; chỉ nhận 1 receiver/package (`analyzer.go:145-169`) | 3 | Một struct `gormRepository`; test tenancy 2-center cho mọi method port thay R1; không thêm `//scopelint:unscoped`. |
| Lỗi Postgres thô (23505/23503) lộ ra 500 = oracle tồn tại + UX vỡ | 3 | `translateDBError` theo constraint name (khuôn `centers/repository.go:872`); cross-tenant ≡ not-found. |
| Optimistic rollback snapshot xoá move đang bay của mutation khác | 5 | D11: không snapshot; lỗi → invalidate; `scope` chung cho 9 mutation. |
| ARIA kanban pattern chưa verify bằng source thật | 4 | Bước 1 của Phase 4 đọc source react-aria hoặc APG Listbox rồi mới chốt roles. |
| Board 8 cột × 50 việc render chậm trên mobile cũ | 5 | Mobile chỉ render 1 cột; selector memoize; cột giới hạn 50 + cursor. |
| Đổi vị trí keys trong catalog làm lệch `Order` của keys cũ | 1 | Chèn nhóm `tasks` ngay trước khối admin; chạy `catalog_test.go` toàn bộ. |

## Open questions

None — hợp đồng brainstorm đã đóng (rev 3, 13/09/2026).

## Rollback

- Phase 1 **trước khi ship** (bảng chưa có dữ liệu thật): `make migrate-down` về
  000021 (down script drop 2 bảng + xoá cả 8 khoá `tasks.*`/`members.list` khỏi
  `center_role_permissions` **và** `center_member_permissions`, kể cả khoá owner
  đã gán tay), revert `CatalogVersion` về 3. **Backup DB trước khi apply.**
- Phase 1 **sau khi ship**: KHÔNG chạy `migrate-down` (mất toàn bộ việc). Chỉ gỡ
  route + revert `CatalogVersion`; bảng và dữ liệu để nguyên như rollback Phase 3.
- Phase 2, 4: xoá thư mục lib; chưa có consumer nên không cascade.
- Phase 3: gỡ `registerFeatures` entry + `routespec` entries + setter
  `SetTaskHandover`; bảng để nguyên (không mất dữ liệu).
- Phase 5: gỡ route + nav entry; API vẫn chạy, không ai gọi.
- Phase 6: revert commit docs/matrix; e2e spec xoá riêng.

## Red Team Review

| Session | Ngày | Reviewer | Finding thô | Sau dedupe | Accept | Reject | Báo cáo |
|---|---|---|---|---|---|---|---|
| 1 | 2026-09-13 | security, failure-mode, assumption, scope (Tier Full, 161 claim kiểm) | 32 | 16 | 15 + 1 quyết định user (`members.list` opt-in) | 4 (đảo cực `DefaultGrant`; lộ `DefaultGrant` ra `PermissionInfo`; ghi audit thẳng trong tx; dùng `legacyIdentitySet` thay `DefaultGrant`) | `plans/reports/redteam-260913-{security,failure,assumption,scope}-task-kanban.md` |

Finding đã áp vào phase file được đánh dấu *(Red-team)* tại chỗ. Lý do reject:
`DefaultRoleKeys()` đã loại `PermKindScope` nên 10 khoá `view_all` là opt-in trống
không tín hiệu từ trước — precedent; test allowlist đủ thay đảo cực; audit qua
bus là khuôn `RolePermissionsChanged` đang dùng; `legacyIdentitySet` mang ngữ
nghĩa "identity key tiền catalog", không phải opt-in.

### Whole-Plan Consistency Sweep

Sau khi áp finding, quét chéo plan.md + 6 phase file. Kết quả:

| Đại lượng | Giá trị thống nhất | Nơi xuất hiện |
|---|---|---|
| Khoá catalog mới / backfill / opt-in | 8 / 5 (`tasks.create,list,read,edit,delete`) / 3 (`tasks.manage_board`, `tasks.view_all`, `members.list`) | D9, AC8, Phase 1, Phase 6 |
| `DefaultRoleKeys()` | 53 → 58 | D9, Risks, Phase 1 |
| Use-case core | 11 (`UpdateColumn` gộp rename + toggle) | Phase 2, Phase 3 bước 7 |
| Port core | 8 (thêm `MemberChecker`) | Phase 2, Phase 3 |
| Sentinel error | 10 (`ErrAssigneeNotMember` thay `ErrCrossTenant`) | Phase 2, Phase 3 bước 6 |
| Event core / event Teka | 1 (`ColumnDeleted`) / 2 (`tasks.ColumnDeleted`, `centers.MemberTasksHandedOver`) | D7, Phase 2, Phase 3, Phase 6 docs |
| Wiring `TaskHandover` | `server.registerFeatures`, nil-safe, `container.go` không đổi | D8, Phase 3 |
| Literal tên cột mặc định | Go: `centers/default_columns.go`; SQL: khối `-- default-columns` 000022; parity literal đóng băng | D5, Phase 1, Phase 3 |
| Board web | cache TanStack duy nhất; hook không `useReducer`; lỗi → invalidate; `scope` chung | D11, Phase 4, Phase 5 |
| Nav mobile | "Công việc" trong sheet "Thêm", 3 tab chính giữ nguyên | Phase 5 |
| Manifest test | `server/route_policy_test.go:16` (không phải `router_test.go`) | Phase 3 |
| Rollback | 2 chế độ: trước ship `migrate-down`; sau ship gỡ route, giữ bảng | Rollback, Phase 1, Phase 6 |

Đã xoá khỏi mọi file: `ErrCrossTenant`, `RenameColumn`/`SetColumnDone`,
`ReadScoped(Actor)`, `readScoped`/`CenterWideFor` trong repo tasks,
`tasks.HandedOver`, rollback snapshot, wiring ở `container.go:100`, đếm 59/6.
Lệch còn lại so với brief (có chủ đích, ghi ở D3/D8): luật read nằm ở
`DefaultPolicy` thay vì predicate SQL trong repository; wiring quay về
`registerFeatures` đúng brief.

<!-- slug: task-center-kanban -->
