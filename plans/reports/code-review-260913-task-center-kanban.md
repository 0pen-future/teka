# Code review — Trung tâm công việc (Task-center Kanban)

Ngày 2026-09-13 · nhánh `master`, toàn bộ chưa commit · review chỉ đọc, không sửa code.

## Phạm vi

- Go: `apps/api/pkg/kanban` (11 file mới), `apps/api/internal/features/tasks` (19 file mới),
  migration `000022_task_board.{up,down}.sql`, sửa `features/centers`, `features/audit`,
  `shared/authctx`, `shared/routespec`, `server/router.go`, `seeds/seed.go`.
- Web: `apps/web/src/lib/kanban` (9 file mới), `apps/web/src/features/tasks` (19 file mới),
  sửa `eslint.config.js`, `app/router.tsx`, `layouts/dashboard-layout.tsx`,
  `test/msw/handlers.ts`, `features/center/*`, `features/audit/components/audit-filters.tsx`,
  e2e `e2e/tasks-board.spec.ts`.
- Docs: `docs/architecture.md`, `docs/event-bus.md`, `docs/adding-permissions.md`, swagger sinh lại.

## Cổng kiểm tra đã chạy

| Lệnh | Kết quả |
|---|---|
| `make lint-api` | pass |
| `make lint-web` | pass |
| `make test-api-unit` | pass |
| `make test-web` | 93 file / 726 pass, 3 skip |
| `make scopelint` | pass |

Không chạy `make test-api` và không chạy lệnh docker nào (agent khác đang giữ Docker).

## Kết luận tổng quan

Chất lượng cao và bám sát kiến trúc repo. Ranh giới lib (Go allowlist qua `go list -deps`,
TS qua ESLint override) là thật, không phải trang trí. Tách `pkg/kanban` (nghiệp vụ thuần)
khỏi `features/tasks` (adapter) làm đúng: mọi luật object-level nằm một chỗ trong
`kanban.DefaultPolicy`, adapter chỉ dịch. Không có lỗi chặn phát hành.

Năm vấn đề mức **major** cần xử lý trước khi lên production, đều sửa nhỏ. Không có
lỗ hổng phân quyền, không có rò rỉ tenant, không có rò rỉ PII.

---

## Major

### M1 — `ErrInvalidInput` trả 400, hợp đồng chốt 422 + `fields`

`apps/api/internal/features/tasks/errors.go:68-69`

```go
case errors.Is(err, kanban.ErrInvalidInput):
    return apperror.BadRequest("invalid input")
```

`api-contract-v1.md:46` ghi rõ `ErrInvalidInput` thuộc nhóm **422 + `fields`**, cùng nhóm với
`ErrDuplicateColumnName`/`ErrColumnLimit`/`ErrInvalidPermutation`/`ErrAssigneeNotMember` —
bốn khoá kia đều dùng `apperror.Invalid`. Chỉ khoá này lệch.

Trả lời câu hỏi "có request path thật nào chạm tới không sau khi DTO validate": **có, bốn đường**.
Gin `binding` chỉ đo độ dài chuỗi thô, core mới `TrimSpace`:

| Đường | Vị trí sinh lỗi | Body vượt được binding |
|---|---|---|
| `POST /task-columns` | `pkg/kanban/columns.go:39` | `{"name":"   "}` — qua `required,min=1,max=40` |
| `PATCH /task-columns/:id` | `pkg/kanban/columns.go:39` | `{"name":"   "}` — qua `omitempty,min=1,max=40` |
| `POST /tasks`, `PATCH /tasks/:id` | `pkg/kanban/service.go:226`, `:279` | `{"title":"   "}` — qua `required,min=1,max=200` |
| `DELETE /task-columns/:id?move_to=<chính nó>` | `pkg/kanban/service.go:178` | UUID hợp lệ, qua parse ở handler |

Hệ quả phía web: `apps/web/src/features/tasks/lib/map-api-error.ts:25` cho 400 rơi vào
`default` → `{kind:"unknown"}` → `applyKanbanFormError` đặt lỗi `root` với câu chung
"Đã xảy ra lỗi, vui lòng thử lại." Người dùng gõ toàn dấu cách vào ô tên cột nhận thông báo
chung chung thay vì lỗi inline dưới ô — trái hẳn với UX mà
`board-settings-modal.test.tsx:92` ("shows a duplicate-name validation error under the input
instead of a toast") chứng minh là chủ ý.

Sai lệch này còn bị **test khoá lại**: `column_delete_integration_test.go` có
`TestDeleteColumnMoveToItselfIsBadRequest` assert `apperror.CodeBadRequest`.

Đề xuất sửa (giữ hợp đồng, không sửa hợp đồng): tách sentinel để adapter đặt được tên field.

```go
// pkg/kanban/errors.go
ErrEmptyName   = errors.New("kanban: column name is empty")
ErrEmptyTitle  = errors.New("kanban: task title is empty")
ErrMoveToSelf  = errors.New("kanban: move target is the column being deleted")

// internal/features/tasks/errors.go
case errors.Is(err, kanban.ErrEmptyName):
    return apperror.Invalid("validation failed", map[string]string{"name": "không được để trống"})
case errors.Is(err, kanban.ErrEmptyTitle):
    return apperror.Invalid("validation failed", map[string]string{"title": "không được để trống"})
case errors.Is(err, kanban.ErrMoveToSelf):
    return apperror.Invalid("validation failed", map[string]string{"move_to": "phải là một cột khác"})
```

Phương án tối thiểu nếu không muốn đụng core: đổi dòng 69 thành
`apperror.Invalid("invalid input", nil)` — đúng 422 nhưng mất `fields`, web vẫn rơi vào nhánh
`entries.length === 0` → "Dữ liệu không hợp lệ." (tốt hơn hiện tại). Kèm sửa test và swagger.

### M2 — `eventSink.Publish` gọi `bus` không kiểm tra nil

`apps/api/internal/features/tasks/events.go:87`

```go
s.bus.Publish(ColumnDeleted{...})
```

`events.Bus` là interface. `newEventSink(nil)` giữ interface nil → gọi method là panic.
Toàn repo dùng helper nil-safe; `centers/service.go:59-63` còn ghi rõ "Nil is a supported
state — publish goes through the nil-safe helper below". Package này bỏ guard.

Không nổ ở production (`server/router.go` truyền bus thật), nhưng trạng thái nguy hiểm
**đã tồn tại trong test suite**: `centers/rbac_integration_test.go:184` gọi
`tasks.NewService(e.db, e.tx, nil)`. Test đó chỉ chạy `HandoverOnDeparture` (không publish) nên
chưa nổ. Bất kỳ test hay wiring nào sau này xoá cột với bus nil sẽ panic **sau khi transaction
đã commit** — cột đã mất mà request 500.

Sửa: thêm guard đúng khuôn `centers`.

```go
func (s *eventSink) publish(e events.Event) {
    if s.bus != nil {
        s.bus.Publish(e)
    }
}
```

### M3 — `UpdatePositions` chạy N update ngoài transaction

`apps/api/internal/features/tasks/column_repository.go:138-152` lặp từng cột một, mỗi cột một
`UPDATE`, không có transaction bao ngoài. `pkg/kanban/service.go:152` (`ReorderColumns`) cũng
không bọc `s.uow.Within` — khác hẳn `DeleteColumn` ở dòng 197 vốn bọc cẩn thận và có comment
giải thích.

Lỗi giữa chừng (mất kết nối, deadlock, timeout) để lại thứ tự áp dụng một phần. Cột `position`
không có unique constraint nên kết quả là nhiều cột trùng `position` → thứ tự board thành tuỳ ý
cho đến lần reorder tiếp theo. Trần 8 cột giới hạn thiệt hại nhưng không loại bỏ nó.

Sửa (một dòng, ở core cho nhất quán với `DeleteColumn`):

```go
return s.uow.Within(ctx, func(ctx context.Context) error {
    return s.repos.Columns.UpdatePositions(ctx, tenant, order)
})
```

`UpdatePositions` đã dùng `database.FromContext(ctx, r.db)` nên tự join tx ambient, không cần
sửa adapter.

### M4 — Đọc toàn bộ task của tenant, không có LIMIT ở SQL

`pkg/kanban/service.go:372-374`:

```go
func (s *Service) tasksInColumn(ctx context.Context, tenant TenantID, col ColumnID) ([]Task, error) {
    tasks, err := s.repos.Tasks.ListBoard(ctx, tenant, Visibility{All: true})
```

`tasksInColumn` chạy trên **mọi** `CreateTask` và `MoveTask`, chỉ để tính `topPosition` —
tức là mỗi lần tạo/chuyển một việc, hệ thống load và deserialize toàn bộ task còn sống của
trung tâm. `apps/api/internal/features/tasks/task_repository.go:29-45` không có `LIMIT`,
không có `column_id` trong `WHERE`.

`Board` cũng vậy: adapter load hết rồi mới cắt 50/cột **trong Go** tại
`internal/features/tasks/service.go:70-73`. Trần 50 chỉ là trần hiển thị, không phải trần truy vấn.

Với 8 cột × 50 việc thì vô hại. Với một trung tâm tích luỹ vài nghìn task (không có TTL,
không có archive trong v1) thì mọi thao tác ghi đều thành full scan theo tenant.

Sửa: thêm port `MinPositionInColumn(ctx, tenant, col) (float64, bool, error)` —
`SELECT min(position) FROM tasks WHERE center_id = ? AND column_id = ? AND deleted_at IS NULL`,
dùng đúng `idx_tasks_board`. Cho `Board`, thêm tham số `limit` vào `ListBoard` và lấy 51 việc
mỗi cột bằng window function (`row_number() OVER (PARTITION BY column_id ORDER BY position, created_at)`)
để `has_more` vẫn chính xác.

### M5 — Thông báo trợ năng bằng tiếng Anh lọt vào UI tiếng Việt

`apps/web/src/lib/kanban/use-kanban-keyboard.ts:234` và `:237`

```ts
setAnnouncement(`Moved task to ${columnName(targetColumnId)}.`);
...
setAnnouncement("Could not move task.");
```

`apps/web/src/lib/kanban/README.md:59-62` nói rõ chuỗi của lib là tiếng Anh và **feature tiêu thụ
phải bọc hoặc format lại**. Feature không làm: `pages/task-board-page.tsx` render thẳng
`{announcement || kanban.announcement}` vào vùng `aria-live="polite"`. Người dùng screen reader
bấm `[`/`]` nghe tiếng Anh giữa một giao diện tiếng Việt.

Đây là vi phạm hợp đồng do chính lib đó ghi, không phải tranh cãi thiết kế.

Sửa: cho `useKanbanKeyboard` nhận `messages` (hoặc trả về `{ kind: "moved" | "move-failed", columnName }`
thay vì chuỗi), rồi feature cấp "Đã chuyển việc sang cột {tên}." / "Không chuyển được việc."
Không có test nào phủ vùng live region nên lỗi này lọt qua CI.

---

## Minor

### N1 — `GET /tasks/:id` trả 403 thay vì 404 khi ngoài visibility

`pkg/kanban/service.go:261` trả `ErrForbidden`. `api-contract-v1.md:35` ghi
"`Task` (404 ngoài visibility)" và dòng 45 ghi "Cột/việc không thuộc tenant hoặc ngoài
visibility → 404". Test `internal/features/tasks/rbac_integration_test.go`
(`TestUnrelatedMemberCannotReadTask`) khoá hành vi 403.

Hợp đồng tự mâu thuẫn: dòng 48 lại nói 403 cho "không phải owner/creator/assignee theo luật D3".
Về mặt bảo mật, 403 là một existence oracle nhẹ — một thành viên dò UUID phân biệt được việc
tồn tại trong trung tâm mình với việc không tồn tại. UUID v4 không đoán được nên rủi ro thực tế
gần bằng không.

Tác động thực tế: **không có** — `getTask` ở `apps/web/src/features/tasks/api/tasks-api.ts:28`
không được gọi ở đâu (xem N11). Cần chốt: sửa code về 404 cho khớp dòng 35, hoặc sửa hợp đồng
dòng 35/45 cho khớp dòng 48. Tôi nghiêng về sửa hợp đồng vì 403 nhất quán với AC3.

### N2 — Swagger `deleteColumn` ghi sai hai mã lỗi

`apps/api/internal/features/tasks/handler.go:185`

```
@Failure 422 ... "cannot delete the last column, or move_to names the column being deleted"
```

"Cột cuối cùng" thực tế là **409** (`errors.go:60-61`, `apperror.Conflict`); "`move_to` trỏ vào
chính nó" thực tế là **400** (`errors.go:68`). Sai số đã lan vào `docs/swagger.json`,
`docs/swagger.yaml`, `docs/docs.go` đã sinh lại. Sửa annotation rồi chạy lại swag.

### N3 — `priority` mặc định `'normal'`, giá trị ngoài enum, không có CHECK

`apps/api/migrations/000022_task_board.up.sql:32`

```sql
priority VARCHAR(8) NOT NULL DEFAULT 'normal',
```

`'normal'` không thuộc enum hợp đồng (`none|low|medium|high`). `internal/features/tasks/model.go:60-62`
đã tự nhận đây là "dead weight left over" vì mọi insert qua Go đều set tường minh. Nhưng không có
CHECK constraint: một insert SQL thô nào bỏ sót cột (fixture, script vận hành, migration tương lai)
ghi được `'normal'` vào DB, và Zod `priority` enum phía web sẽ ném lỗi khi parse board → hỏng
cả trang, không chỉ một thẻ.

Migration chưa lên production (file down tự ghi "chỉ an toàn rollback TRƯỚC khi lên production")
nên sửa tại chỗ được:

```sql
priority VARCHAR(8) NOT NULL DEFAULT 'none'
    CHECK (priority IN ('none', 'low', 'medium', 'high')),
```

### N4 — `fk_tasks_assignee_center ON DELETE CASCADE` xoá cả việc thay vì gỡ người được giao

`apps/api/migrations/000022_task_board.up.sql:43-44`. Nếu một dòng `center_members` bị xoá cứng,
**mọi task được giao cho người đó bị xoá theo**, chứ không phải chỉ `assignee_id` về NULL.
`assignee_id` là cột nullable — CASCADE không diễn tả đúng ý định.

Hiện chưa có đường xoá cứng `center_members` nào trong production (grep `DELETE FROM center_members`
chỉ ra một chỗ trong `migrations/migrations_test.go:787`), nên đây là mìn ngủ. Ta chạy Postgres 16
nên cú pháp cột-cụ-thể dùng được:

```sql
CONSTRAINT fk_tasks_assignee_center
    FOREIGN KEY (assignee_id, center_id) REFERENCES center_members (teacher_id, center_id)
    ON DELETE SET NULL (assignee_id),
```

`fk_tasks_creator_center` giữ CASCADE là hợp lý (`created_by` NOT NULL, và handover đã chuyển
quyền tác giả cho owner trước khi thành viên rời).

### N5 — Dòng audit `task.handover` không có actor và không có entity id

`apps/api/internal/features/centers/events.go` — `MemberTasksHandedOver` không mang `ActorID`,
dù `centers.RemoveMember` có sẵn `scope.TeacherID`. `audit/subscriber.go` do đó để
`ActorUserID` nil và `EntityID` rỗng, khác mọi case sự kiện lân cận. Ai gỡ thành viên chỉ suy ra
được bằng cách đối chiếu dòng `center.member.remove` cùng thời điểm. Thêm `ActorID` vào struct
và set từ `scope.TeacherID`.

### N6 — Publish `MemberTasksHandedOver` cả khi không bàn giao gì

`apps/api/internal/features/centers/service.go` — `s.publish(ev)` chạy mọi lần `taskHandover != nil`,
kể cả `Unassigned == 0 && Reassigned == 0`. Mỗi lần gỡ thành viên đều sinh thêm một dòng audit
`task.handover` rỗng. Thêm điều kiện `if ev.Unassigned > 0 || ev.Reassigned > 0`.

### N7 — Comment `Directory` sai sự thật về owner

`apps/api/internal/features/centers/repository.go:348`

> The owner has no center_members row and so never appears here

Sai. `migrations/000007_centers.up.sql:77-78` chèn `center_members` cho **mọi** teacher kể cả owner,
và `fk_teachers_membership` (dòng 83-85) bắt buộc dòng đó tồn tại. Owner **có** xuất hiện trong
directory — và đó chính là hành vi đúng: nhờ vậy `ReassignCreator` chuyển quyền tác giả cho owner
mới không vi phạm `fk_tasks_creator_center`. Chỉ comment sai, hành vi đúng. Sửa comment để người
sau không thêm case đặc biệt cho owner trong picker.

### N8 — `taskRepository.Create` bỏ qua tham số `tenant`

`apps/api/internal/features/tasks/task_repository.go:65`

```go
func (r *taskRepository) Create(ctx context.Context, _ kanban.TenantID, task kanban.Task) (kanban.Task, error) {
```

13/14 method còn lại đều lọc `center_id` tường minh; riêng method này tin vào `task.TenantID`.
An toàn hôm nay (core luôn set `TenantID: tenant` ở `service.go:241`) và composite FK chặn được
dòng lai tenant, nhưng scopelint không phủ package này nên đây là chỗ duy nhất một caller sai
sẽ ghi im lặng sang tenant khác. Thêm `row.CenterID = uuid.UUID(tenant)` trước `Create`.

### N9 — `defaultColumnsSvc` là một `kanban.Service` rác với 4 port nil

`apps/api/internal/features/centers/default_columns.go:26`

```go
var defaultColumnsSvc = kanban.NewService(kanban.Repositories{}, nil, nil, nil, nil, kanban.WithIDGen(id.New))
```

Dựng cả một Service với toàn port nil chỉ để gọi `DefaultColumns` — method này (`pkg/kanban/columns.go:21`)
chỉ đọc `s.cfg.idGen`. Một refactor thêm bất kỳ lời gọi port nào vào `DefaultColumns` biến đây
thành nil-panic ở đường tạo trung tâm. Đưa nó thành hàm tự do:

```go
func DefaultColumns(idGen func() uuid.UUID, specs []DefaultColumnSpec) []Column
```

### N10 — Optimistic delete cột làm việc biến mất tạm thời

`apps/web/src/features/tasks/hooks/use-tasks-data-source.ts:281` dispatch `columns/removed`;
`apps/web/src/lib/kanban/state.ts:80-82` chỉ lọc `columns`, không đụng `tasks`. Task của cột đó
thành mồ côi — `selectTasksByColumn` (`selectors.ts:34-36`) nhét chúng vào một bucket theo
`columnId` không còn trong `board.columns`, nên chúng **không render ở đâu cả** cho tới khi
refetch xong. Người dùng vừa được hộp thoại hứa "chuyển các việc đó sang" cột khác lại thấy
chúng biến mất rồi hiện lại. Không mất dữ liệu, chỉ nhấp nháy.

Sửa: trước `columns/removed`, dispatch `tasks/moved` cho từng task của cột sang `moveTasksTo`;
hoặc bỏ hẳn optimistic ở mutation này và để refetch lo (đúng tinh thần D11).

### N11 — `useMemo` + `eslint-disable` không có tác dụng, và lý do ghi trong comment sai

`apps/web/src/features/tasks/hooks/use-tasks-data-source.ts:408-415`. Dep là `[params]`,
nhưng `pages/task-board-page.tsx:53` truyền literal `useTasksDataSource({ scope })` — object mới
mỗi render, nên memo vỡ mỗi render và `eslint-disable-next-line react-hooks/exhaustive-deps`
không mua được gì.

Comment biện minh cũng sai: nó nói tránh "re-run `useKanban`'s memoized `tasksByColumn`", nhưng
`apps/web/src/lib/kanban/use-kanban.ts:58` cho thấy `tasksByColumn` phụ thuộc `[board]`, hoàn toàn
không phụ thuộc `dataSource`. Sửa: đổi dep thành `[params.scope]` (rồi bỏ được suppression), hoặc
bỏ luôn `useMemo`.

### N12 — `getTask` là code chết; `tasks.read` không có consumer nào trên UI

`apps/web/src/features/tasks/api/tasks-api.ts:28` không được gọi ở bất kỳ đâu trong `src`.
Route `GET /tasks/:id` và khoá `tasks.read` vẫn tồn tại đầy đủ ở API và trong ma trận phân quyền.
Không sai, nhưng nên biết là v1 ship một khoá quyền không có màn hình nào dùng.

### N13 — Thông báo lỗi của `markedBlocks` ghi nhầm tên file

`apps/api/migrations/backfill_parity_test.go:86` hardcode `backfillUpFile` (000018) trong
`t.Fatalf`, kể cả khi được gọi cho `000022_task_board.up.sql`. Nếu ai xoá marker ở 000022,
thông báo lỗi chỉ nhầm sang file khác. Truyền tên file vào helper.

### N14 — Docs mâu thuẫn với chính migration vừa thêm

`docs/adding-permissions.md` (mới thêm) viết: một khoá `optIn()` "must never appear in a backfill
migration: ... its down migration (if any) only needs to undo schema it introduced, not sweep grants."
Nhưng `migrations/000022_task_board.down.sql:6-16` xoá **cả ba khoá opt-in**
(`tasks.manage_board`, `tasks.view_all`, `members.list`) khỏi cả hai bảng, kể cả những dòng owner
đã tự tay gán. File down có ghi cảnh báo tiếng Việt rất rõ nên đây không phải bẫy giấu mặt — chỉ là
tài liệu và ví dụ đối chọi nhau. Chỉnh câu trong docs, hoặc dẫn 000022 làm ngoại lệ "pre-ship rollback".

---

## Nit

- `idx_tasks_assignee` và `idx_tasks_creator` (`000022...up.sql:48-49`) thiếu vị từ
  `WHERE deleted_at IS NULL` mà `idx_tasks_board` có. Nhánh participant của `ListBoard` lọc
  `deleted_at IS NULL` nên index phần sẽ nhỏ và hữu ích hơn.
- Override ESLint cho `src/lib/kanban` là **denylist** (`@/features/*`, `@/components/*`,
  `@/lib/api`, tanstack, zod, axios), không phải allowlist như phía Go. Một import mới kiểu
  `@/lib/utils` hay `date-fns` lọt qua. Đúng khuôn đã có của repo (`src/features/statement`
  dùng cùng kiểu) nên tôi không tính là lỗi — chỉ ghi nhận ranh giới TS yếu hơn ranh giới Go.
- `eventSink.Publish` (`events.go:78`) dùng `caller, _ := ctx.Value(...)`. Nếu một caller tương lai
  quên `withDeleteColumnCaller`, `caller` là struct zero → dòng audit ghi `CenterID`/`ActorID` nil
  một cách im lặng thay vì báo lỗi.

---

## Đối chiếu tiêu chí nghiệm thu

| AC | Kết quả | Bằng chứng |
|---|---|---|
| AC1 owner không cần gán quyền | Đạt | `authctx.Scope.Has` bypass cho owner; `kanban.DefaultPolicy` kiểm `actor.IsOwner` đầu tiên ở cả 4 method (`pkg/kanban/policy.go`); `TestBoardScopeCenterForOwner` |
| AC2 visibility theo `tasks.list`/`view_all`, cách ly tenant | Đạt | `Policy.Visibility` → `ListBoard`; `TestBoardScopeDegradesToMineWithoutViewAll`; `tenancy_integration_test.go` (4 test) |
| AC3 assignee chuyển được cột, sửa/xoá chỉ creator+owner → 403 | Đạt | `TestNonCreatorNonOwnerAssigneeCannotEditOrDeleteButCanMove`; `TestAssigneeAloneCannotEditButCanMove` |
| AC4 thiếu `manage_board` → 403 mọi API cột, ẩn nút | Đạt | `TestOrdinaryMemberCannotManageBoardWithoutCapability` (4 endpoint); `task-board-page.test.tsx` "hides the scope switch and board settings button" |
| AC5 xoá cột: bắt buộc `move_to`, một transaction, 409, không xoá cột cuối, audit đủ | Đạt | `pkg/kanban/service.go:197-206` bọc `uow.Within`, publish sau commit; `column_delete_integration_test.go` (7 test); `audit/subscriber.go` case `tasks.ColumnDeleted` ghi `move_to` + `moved_count` |
| AC6 `is_done` stamp/clear `completed_at`, đổi cờ không viết lại lịch sử | Đạt | `TestDeleteColumnMovesTasksAndStampsCompletedAt`; `TestDeleteColumnMoveClearsCompletedAtForNonDoneDestination`; `UpdateColumn` (`service.go:104-132`) không đụng bảng `tasks` |
| AC7 trung tâm mới có 3 cột, backfill, parity test | Đạt | `centers/repository.go:407+` trong `CreateCenter`; migration backfill dòng 56-67; `TestTaskBoardSQLMatchesFrozenKeysAndColumns` |
| AC8 ma trận có nhóm "Công việc" + `members.list` | Đạt | `RESOURCE_LABELS.tasks = "Công việc"`; `tasks` không nằm trong `ADMIN_RESOURCES` nên có tab riêng; `members.list` gộp vào "Quản trị"; 5 khoá CRUD backfill 2 nhánh, 3 khoá opt-in để trống |
| AC9 audit `task.*`/`task_column.*`, manifest test, e2e | Đạt (một phần chưa kiểm được) | `action_test.go` +8 dòng, `route_policy_snapshot_test.go` +11 dòng, cả hai xanh; `e2e/tasks-board.spec.ts` phủ một luồng đầy đủ. **`make test-api` chưa chạy** (Docker do agent khác giữ) nên tầng integration chưa được xác nhận trong phiên này |
| AC10 handover cùng tx, audit sau commit, mời lại không hoàn tác | Đạt | `centers/service.go` — handover **trước** `CloseMembership`, cùng `WithinTx`; `s.publish(ev)` **sau** khi `WithinTx` trả nil; `handover_integration_test.go`; `TestRemoveMemberHandsOverTasks` |
| AC-LIB ranh giới hai lib + README | Đạt | `pkg/kanban/import_boundary_test.go` (`go list -deps`, allowlist stdlib + `google/uuid`); `eslint.config.js` override (denylist, xem Nit); hai README có mục Ports |

Ghi chú AC9: mọi cổng được phép đều xanh, nhưng 43 test Go của package `tasks` đều mang build tag
`integration` nên **không** chạy trong `make test-api-unit`. Kết luận về chúng dựa trên đọc code,
không dựa trên lần chạy nào.

---

## Không hồi quy nghiệp vụ cũ (đã kiểm từng mục)

- **`centers.RemoveMember`** — handover nằm trong `WithinTx`, **trước** `CloseMembership`, nên
  handover lỗi thì cả việc gỡ thành viên rollback. `s.publish(ev)` chỉ chạy sau khi `WithinTx`
  trả nil. Nhánh `taskHandover == nil` chỉ log warn và đi tiếp — hành vi cũ được giữ nguyên.
- **`centers.CreateCenter`** — hai bước cũ (3 system role, backfill `DefaultRoleKeys`) không đổi;
  chèn 3 cột là bước thứ ba, nằm sau, cùng `database.FromContext` nên cùng tx với caller.
- **Audit subscriber** — hai case mới không đụng case cũ. Việc `task_column.delete` sinh **hai**
  dòng (một từ request-log, một từ event) là chủ ý và đúng khuôn `centers.RolePermissionsChanged`
  ở dòng 211-228, phân biệt bằng trường `Method`. Đã đối chiếu, không phải nhân bản ngoài ý muốn.
- **`routespec` + `route_policy_test.go`** — 11 spec mới, snapshot cập nhật đủ 11 dòng,
  `TestRoutePolicySnapshotUnchanged` xanh. `GET /tasks/board` và `GET /tasks/:id` không lẫn key:
  `enforceRoutePolicy` khớp theo `(method, c.FullPath())`, mà Gin trả `/api/v1/tasks/board`
  cho route tĩnh và `/api/v1/tasks/:id` cho route tham số.
- **Nav dashboard** — thêm "Công việc" vào `useNavGroups`, **và** vào cả `OVERFLOW_LABELS`
  lẫn `OVERFLOW_PATH_PREFIXES`. Cả ba danh sách đồng bộ; `dashboard-layout.test.tsx` xanh.
- **MSW default handlers** — thuần bổ sung, không sửa handler cũ. `CATALOG_VERSION` 3 → 4 và
  8 khoá mới mirror đúng thứ tự catalog Go. 93 file test web xanh, không có test feature khác gãy.
- **`RESOURCE_LABELS` / `buildCatalogTabs`** — chỉ thêm một entry; `tasks` không thuộc
  `ADMIN_RESOURCES` nên có tab riêng đúng như AC8, không xáo trộn tab "Quản trị" hiện có.

## Không phá vỡ hợp đồng công khai

- **Route** — 11 route mới, không đổi/xoá route nào.
- **Tên field DTO / envelope** — trùng khớp `api-contract-v1.md` cho cả `Column`, `Task`,
  `DirectoryEntry`, `BoardResponse` (`scope` + `columns[].tasks[]` + `has_more`),
  `{moved_count}`, `{columns}`. Zod schema web khớp một-một.
- **Catalog** — 8 khoá thêm mới, không sửa/xoá khoá cũ; `CatalogVersion` 3 → 4 đúng quy trình;
  `DefaultGrant` là field bổ sung trên `PermDef` (nội bộ, không lộ ra JSON catalog);
  `DefaultRoleKeys()` 53 → 58, chỉ mở rộng.
- **Migration** — có `.down.sql`; tự khai báo rõ giới hạn "chỉ rollback trước khi lên production"
  vì nó quét cả grant tay của owner. Chấp nhận được với ghi chú N14.
- **Bề mặt lib export** — cả hai lib đều mới, không có consumer cũ để phá.
- **Lệch duy nhất so với hợp đồng**: M1 (400 vs 422) và N1 (403 vs 404). Xem chi tiết ở trên.

## Tuân thủ khuôn mẫu repo

Đạt trên mọi trục đã kiểm: layout feature (`handler/routes/service/repository/dto/model/errors/events`),
`apperror`, chữ ký repository tenant-first, `WithinTx` join tx ambient qua `database.FromContext`,
routespec điều khiển cả authz lẫn audit, `Optional[T]` mirror `teaching/dto.go`, TanStack keys
phân tầng, Zod `parseData`/`parseArray`, MSW 2, component design-system (`HvModal`, `HvButton`,
`HvSelect`, `hvToast`).

Hai chỗ lệch khuôn đã nêu: M2 (thiếu nil-guard cho bus) và N9 (Service rác).

## Bảo mật và độ bền

| Mục | Kết luận |
|---|---|
| Hai tầng authz | Đúng. Tầng capability ở `routespec` + middleware; tầng object-level **chỉ** ở `kanban.DefaultPolicy`, không rải rác. Không tìm thấy chỗ nào tự suy lại luật owner/creator/assignee. |
| `actorFrom` | An toàn. `internal/features/tasks/policy.go` dựng map `Perms` **mới** mỗi lần gọi, chỉ 2 khoá core cần đọc. Không có state chia sẻ giữa request. |
| Cách ly tenant | Đạt. 13/14 method repository lọc `center_id` tường minh (xem N8 cho method còn lại). Kiểm tay vì scopelint không phủ package này; `tenancy_integration_test.go` phủ cả cột lẫn việc lẫn memberChecker. |
| Ưu tiên toán tử GORM | **Không rò rỉ.** `q.Where("center_id = ? AND deleted_at IS NULL").Where("created_by = ? OR assignee_id = ?")` — GORM bọc ngoặc cho một `Expr` chứa OR khi có nhiều mệnh đề Where, nên câu lệnh là `... AND (created_by = ? OR assignee_id = ?)`, không phải `... OR ...` phẳng. |
| `scope=center` degrade | Đúng. `internal/features/tasks/service.go:85-88` echo `"mine"` khi thiếu quyền, không 403; web đọc `boardQuery.data?.scope` chứ không đọc lại request. |
| Directory không lộ PII | Đạt. Câu truy vấn (`centers/repository.go:352-360`) chỉ select `t.id`, `t.full_name`, `cr.name`; không có phone/email; chỉ stint sống; join `user_accounts` đang active. DTO `DirectoryEntry` cũng chỉ có 3 field. |
| Trần 8 cột | Chính xác. `columnRepository.Create` re-check trong transaction sau `pg_advisory_xact_lock(hashtext(?::text))` theo tenant — hai request đồng thời không vượt trần được. |
| `DeleteColumn` + `MoveAllToColumn` | Đúng. `MoveAllToColumn`/`CountInColumn` **cố ý** tính cả dòng soft-deleted vì FK `RESTRICT` không phân biệt; `UnassignBy`/`ReassignCreator` **cố ý** bỏ qua chúng vì handover không nên viết lại lịch sử. Hai lựa chọn trái chiều nhưng cùng đúng, và có test cho cả hai. |
| `PATCH /tasks/:id` tri-state | Đúng. `Optional[T]{Set, Value}` phân biệt vắng/`null`/có giá trị; `UpdateTask` load task hiện tại rồi merge vào input full-replace của core. `TestUpdateTaskMergesTriStateAssigneeAndDueOn` phủ đủ 3 trạng thái. |
| `HandoverOnDeparture` | Đúng. Không mở tx, không publish (`pkg/kanban/service.go:341`); consumer sở hữu commit và event. |
| Ranh giới lib | Đạt cả hai. Go: allowlist thật qua `go list -deps`. TS: denylist qua ESLint (xem Nit). |
| D11 không rollback trên web | Đạt. Không có `onError` nào khôi phục snapshot; cả 9 mutation dùng chung `scope: { id: "kanban-board" }` nên TanStack serialize chúng; `onSettled: invalidateBoard` luôn chạy. |

## Chất lượng test

43 test Go của package `tasks` (20 unit + 23 integration) và 22 test `pkg/kanban` **assert hành vi,
không assert cách cài đặt**: chúng gọi service thật, đọc lại DB bằng SQL thô, và kiểm mã lỗi/số
dòng/thứ tự — không có test nào chỉ đếm số lần gọi mock. Fake repo trong `service_test.go` là
in-memory thật, chạy qua đúng `kanban.DefaultPolicy` và đúng use-case, không phải stub trả sẵn.

Web: 726 test xanh. Test board tập trung vào hành vi người dùng (bấm nút, mở modal, xác nhận,
kiểm kết quả trên màn hình) chứ không snapshot DOM.

**Ca còn thiếu đáng chú ý:**

1. **Không có test nào chứng minh D11 hồi phục sau lỗi.** Luận điểm cốt lõi — "mutation lỗi thì
   `onSettled` invalidate và refetch kéo về trạng thái server" — không có test. Nên có một test
   cho mutation fail rồi assert board query bị invalidate.
2. **Không có test nào chứng minh handover lỗi thì rollback cả việc gỡ thành viên**, dù comment
   của `TaskHandover` interface (`centers/service.go`) khẳng định đúng điều đó.
   `TestRemoveMemberHandsOverTasks` chỉ phủ đường thành công.
3. **Không có test cho vùng `aria-live`** — chính vì thế M5 lọt lưới.
4. **Không có test cho việc web hiển thị đúng scope server trả về** khi server degrade
   `center` → `mine` (`effectiveScope = boardQuery.data?.scope ?? scope`).
5. Không có test cho nhánh `has_more` phía web (server có `TestBoardCapsAt50TasksAndReportsHasMore`,
   UI thì không).

**Về import chéo package trong test** (`centers/rbac_integration_test.go` import `features/tasks`
từ package ngoài `centers_test`): **chấp nhận được**. Ba lý do đã kiểm:
`features/tasks` không import `features/centers` (grep xác nhận) nên không có nguy cơ chu trình;
`centers_test` là external test package nên kể cả có chiều ngược lại Go vẫn cho phép;
và test đó phủ đúng thứ mà không test nào khác phủ được — hành vi handover chạy trong tx của
`RemoveMember` với hai service thật. Đây là bằng chứng end-to-end cho AC10, đánh đổi hợp lý.

---

## Việc nên làm, theo thứ tự

1. Sửa M1 (`ErrInvalidInput` → 422 + `fields`), cập nhật test `TestDeleteColumnMoveToItselfIsBadRequest`
   và annotation swagger N2 trong cùng một lượt.
2. Sửa M2 (nil-guard cho `eventSink`) — một hàm 5 dòng.
3. Sửa M3 (bọc `ReorderColumns` trong `uow.Within`).
4. Sửa M5 (bản địa hoá announcement) và thêm test cho vùng live region.
5. Sửa N3 + N4 trong migration 000022 khi nó còn chưa lên production.
6. Sửa M4 (thêm port `MinPositionInColumn`, thêm `limit` cho `ListBoard`) — có thể tách thành
   một lượt riêng nếu muốn ship trước.
7. Dọn N5–N14 theo mức độ thuận tiện.
8. Chạy `make test-api` khi Docker rảnh — 23 test integration của package `tasks` chưa từng
   được xác nhận trong phiên review này.
