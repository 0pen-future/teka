# Hợp đồng JSON tasks API v1 (chốt cho Phase 3 ↔ Phase 5)

Envelope chuẩn của repo: `{ success: true, data: ... }`; lỗi qua `apperror`
(422 kèm `fields`, 403, 404, 409). Timestamp RFC3339; `due_on` là ngày
`YYYY-MM-DD`; UUID dạng chuỗi. Field vắng → `null` (không `omitempty` cho
nullable để Zod `.nullable()` ổn định).

## Kiểu

```jsonc
Column = { "id", "name", "position": 0, "is_done": false, "created_at", "updated_at" }
Task = {
  "id", "column_id", "title", "description": "",
  "priority": "none" | "low" | "medium" | "high",
  "due_on": "2026-09-30" | null,
  "assignee_id": uuid | null,
  "created_by": uuid,
  "position": -3,                 // float64, nhỏ hơn = trên đầu cột
  "completed_at": RFC3339 | null,
  "created_at", "updated_at"
}
DirectoryEntry = { "teacher_id", "display_name", "role_name": "Giáo viên" | null }
```

## Endpoint (tất cả dưới `/api/v1`, đi qua authChain + routespec)

| Method/path | Key | Body / query | Response `data` |
|---|---|---|---|
| `GET /tasks/board?scope=mine\|center` | `tasks.list` | `scope` mặc định `mine`; `center` chỉ hiệu lực khi có `tasks.view_all` hoặc owner, ngược lại degrade về `mine` (không 403) | `{ "scope": "mine"\|"center", "columns": [ Column & { "tasks": Task[], "has_more": bool } ] }` — cột theo `position`, việc trong cột theo `position, created_at`, tối đa 50 việc/cột (`has_more` = còn việc bị cắt; v1 chưa có endpoint tải thêm) |
| `POST /task-columns` | `tasks.manage_board` | `{ "name", "is_done"?: bool }` | `Column` (201) |
| `PATCH /task-columns/:id` | `tasks.manage_board` | `{ "name"?, "is_done"? }` | `Column` |
| `PUT /task-columns/order` | `tasks.manage_board` | `{ "ids": [uuid...] }` hoán vị đủ | `{ "columns": Column[] }` |
| `DELETE /task-columns/:id?move_to=uuid` | `tasks.manage_board` | `move_to` bắt buộc khi cột có việc (kể cả soft-deleted) | `{ "moved_count": n }` (200) |
| `POST /tasks` | `tasks.create` | `{ "title", "description"?, "column_id"?, "assignee_id"?, "priority"?, "due_on"? }` — thiếu `column_id` → cột position 0 | `Task` (201) |
| `GET /tasks/:id` | `tasks.read` | | `Task` (404 khác tenant; 403 trong tenant nhưng ngoài visibility D3) |
| `PATCH /tasks/:id` | `tasks.edit` | `{ "title"?, "description"?, "assignee_id"?: uuid\|null, "priority"?, "due_on"?: date\|null }` — handler load task hiện tại rồi merge vào `UpdateTaskInput` full-replace của core; gửi `null` tường minh = xoá giá trị, vắng field = giữ | `Task` |
| `POST /tasks/:id/move` | `tasks.edit` | `{ "column_id" }` (v1 luôn đặt lên đầu cột, không có `position`) | `Task` |
| `DELETE /tasks/:id` | `tasks.delete` | | 204 |
| `GET /centers/me/members/directory` | `members.list` | | `DirectoryEntry[]` (parseArray; không SĐT/email; chỉ stint sống) |

## Mã lỗi

| Tình huống | HTTP | Nguồn |
|---|---|---|
| Cột/việc không thuộc tenant | 404 | `ErrColumnNotFound`/`ErrTaskNotFound` |
| Trùng tên cột, quá 8 cột, permutation lệch, input sai (tên/tiêu đề rỗng sau trim, `move_to` trỏ vào chính cột), assignee không phải thành viên | 422 + `fields` | `ErrDuplicateColumnName`/`ErrColumnLimit`/`ErrInvalidPermutation`/`ErrInvalidInput` (+ sentinel con `ErrEmptyName`/`ErrEmptyTitle`/`ErrMoveToSelf`)/`ErrAssigneeNotMember` |
| Cột còn việc mà thiếu `move_to`; cột cuối cùng | 409 | `ErrColumnNotEmpty`/`ErrLastColumn` |
| Không phải owner/creator/assignee theo luật D3 (kể cả `GET /tasks/:id` trong tenant) | 403 | `ErrForbidden` |

Ghi chú: reorder lệch trả 422 theo mapping sentinel (phase 3 bước 6), web coi
409 **và** 422 của `PUT /task-columns/order` như "bảng đã thay đổi" → refetch.
