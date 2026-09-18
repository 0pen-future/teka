# Hợp đồng API v2: di chuyển việc có vị trí và mô tả rich text

Nguồn sự thật: `apps/api/docs/swagger.yaml` (sinh bởi `make api-docs`),
`apps/api/internal/features/tasks/dto.go`, `description.go`, `errors.go`.
Tài liệu này tóm tắt phần thay đổi so với v1 để client đối chiếu; khi lệch,
swagger thắng.

## `POST /api/v1/tasks/:id/move`

Quyền: chủ trung tâm, người tạo việc hoặc người được giao (không đổi so với v1).

### Body

| Trường | Kiểu | Bắt buộc | Ý nghĩa |
|---|---|---|---|
| `column_id` | UUID | có | Cột đích (có thể là cột hiện tại để sắp xếp lại trong cột). |
| `after_task_id` | UUID \| null | không | Việc mà việc được chuyển sẽ nằm **ngay dưới**. Vắng hoặc `null` → nằm **đầu cột** (đúng hành vi v1). |

Body v1 `{ "column_id": "…" }` vẫn hợp lệ và cho kết quả y như trước: việc lên
đầu cột. Client cũ không cần đổi gì.

### Ví dụ

Sắp xếp lại trong cột (kéo việc `t3` xuống dưới `t1`):

```json
POST /api/v1/tasks/t3/move
{ "column_id": "col-a", "after_task_id": "t1" }
```

Chuyển sang cột khác, thả vào vùng trống (cuối cột): client gửi
`after_task_id` = id của việc cuối cùng trong cột đích; cột đích rỗng → bỏ
`after_task_id` (đầu cột cũng là cuối cột).

Chuyển qua menu "Chuyển cột" (không có vị trí):

```json
{ "column_id": "col-b" }
```

### Phản hồi

`200` với `data` là `tasks.TaskResponse` của việc sau khi chuyển (`column_id`,
`position`, `completed_at` đã cập nhật: `completed_at` được đặt/xoá theo
`is_done` của cột đích, như v1).

### Lỗi

| Mã | Khi nào | `error.fields` |
|---|---|---|
| 400 | JSON hỏng, `column_id`/`after_task_id` không phải UUID | — (BindError) |
| 401 | Thiếu/ hết hạn token | — |
| 403 | Không phải chủ/người tạo/người được giao | — |
| 404 | Việc hoặc cột đích không tồn tại trong trung tâm | — |
| 422 | `after_task_id` không phải việc **đang sống** trong **cột đích**, hoặc chính là việc đang chuyển | `{"after_task_id": "phải là việc đang nằm trong cột đích"}` |

### Ngữ nghĩa vị trí

`position` là `float64`, cột đọc theo `position` tăng dần. Đầu cột = `min − 1`
(0 nếu cột rỗng); sau một việc = trung điểm giữa việc đó và việc kế tiếp
(`after.position + 1` nếu không có kế tiếp). Khi khoảng cách xuống dưới
`1e-6`, server renormalize cả cột về `0..n−1` trong cùng transaction, dưới
khoá cột, rồi tính lại. Client **không** được suy diễn hay gửi `position`;
sau mỗi move hãy dùng `position` server trả về (hoặc tải lại
`GET /tasks/board`). Hai lệnh move đồng thời vào sau cùng một
`after_task_id` được tuần tự hoá bởi khoá cột nên không bao giờ trùng vị trí.

## Trường `description` (`POST /tasks`, `PATCH /tasks/:id`, mọi response)

`description` giờ là **HTML subset đã sanitize**, không còn là văn bản thuần.

### Cho phép

- Thẻ: `p`, `br`, `strong`, `em`, `u`, `s`, `ul`, `ol`, `li`, `a`.
- Thuộc tính: chỉ `href` trên `a`, với scheme `http`, `https`, `mailto`.
- Server tự thêm vào mọi `a`: `rel="nofollow noreferrer"`; link đầy đủ
  (`http`/`https`) còn thêm `noopener` và `target="_blank"`. Client không cần
  gửi `rel`/`target`; nếu gửi sẽ bị ghi đè.

Mọi thẻ/thuộc tính khác (`script`, `img`, `style`, `class`, `on*`, URL
`javascript:`/`data:`, link tương đối) bị loại bỏ nhưng **giữ lại phần chữ**
bên trong. Vì vậy `GET` có thể trả về mô tả khác với body đã gửi.

### Giới hạn

| Giới hạn | Giá trị | Ở đâu | Lỗi |
|---|---|---|---|
| Body HTTP | ≤ `HTTP.MaxBodyBytes` (mặc định 1 MiB) | middleware `BodyLimit` | 413 `PAYLOAD_TOO_LARGE` (trước khi validate) |
| HTML thô (body) | ≤ 20 000 ký tự | `binding:"max=20000"` trên DTO | 422 `fields.description` |
| Chữ sau khi bỏ thẻ | ≤ 4 000 rune | `description.go` (`maxDescriptionRunes`) | 422 `{"description": "tối đa 4000 ký tự"}` |
| Client web | ≤ 2 000 ký tự chữ, ≤ 20 000 HTML | zod `taskFormSchema` | chặn trước khi gửi |

Web tự đặt mức 2 000 ký tự chặt hơn server để mô tả đọc được trên card và
điện thoại; API vẫn nhận tới 4 000 cho client khác.

### Văn bản thuần và dữ liệu cũ

- Body `description` là văn bản thuần (không bắt đầu bằng thẻ) được bọc thành
  một `<p>`, xuống dòng thành `<br>`, ký tự đặc biệt được escape — cùng cách
  migration `000023` bọc các dòng cũ. Client chỉ gửi text thuần vẫn dùng được,
  và luôn nhận về HTML.
- Chuỗi rỗng/chỉ khoảng trắng → lưu `""`.
- Client hiển thị **phải** sanitize lại trước khi render (web dùng DOMPurify
  với cùng allow-list trong `apps/web/src/features/tasks/lib/rich-text.ts`);
  không tin markup từ server dù server đã sanitize.

## Không đổi

- `GET /tasks/board`, `GET /tasks/:id`, `DELETE /tasks/:id`, các route
  `task-columns`.
- `MoveTaskRequest` v1 (`column_id` đơn lẻ) và toàn bộ response envelope
  `{ "success": true, "data": … }`.
