# Code review: Zalo personal mapping backend (migration 000005 + endpoints)

Reviewer: code-reviewer subagent. Scope: migration 000005, contacts mapping endpoints, GET /me/zalo/friends, schema docs.

## Blockers (nên sửa trước khi land)

**B1. `notifications.run_id` thiếu ràng buộc cùng tenant** — `apps/api/migrations/000005_zalo_personal_mapping.up.sql:44`

Baseline dùng một convention nhất quán để DB tự chặn liên kết chéo giáo viên: mỗi bảng tenant-scoped có `CONSTRAINT uq_<table>_tid UNIQUE (id, teacher_id)` và được trỏ tới bằng composite FK. Chính `notifications.statement_id` cũng vậy: `FOREIGN KEY (statement_id, teacher_id) REFERENCES statements(id, teacher_id)`. `notification_runs` không có `uq_notification_runs_tid`, và `run_id` khai báo FK đơn cột, nên ở tầng DB một notification của giáo viên A vẫn trỏ được vào run của giáo viên B. Vì counters của run derive bằng `COUNT` trên `notifications.run_id` (locked decision), một bug ở phase 3 sẽ biến thành tiến độ run trộn số của tenant khác mà không có gì chặn lại.

Cách sửa, kèm một cái bẫy: `ON DELETE SET NULL` trên composite FK sẽ cố set cả `teacher_id` về NULL và fail vì cột đó NOT NULL, nên phải dùng dạng có column list.

```sql
-- trong CREATE TABLE notification_runs
CONSTRAINT uq_notification_runs_tid UNIQUE (id, teacher_id)

-- thay cho: run_id UUID REFERENCES notification_runs(id) ON DELETE SET NULL
ALTER TABLE notifications ADD COLUMN run_id UUID;
ALTER TABLE notifications
    ADD FOREIGN KEY (run_id, teacher_id) REFERENCES notification_runs(id, teacher_id)
        ON DELETE SET NULL (run_id);
```

Cú pháp `SET NULL (col)` cần PG ≥ 15. Compose dùng `postgres:17-alpine`, testcontainer dùng `postgres:16-alpine`, nên chạy được cả hai.

**B2. Thiếu unique index trên `(teacher_id, zalo_user_id)`**

Hiện hai contacts của cùng một giáo viên map được vào cùng một người Zalo. Hậu quả ở phase 3 không chỉ là gửi trùng mà là data exposure: người đó nhận link statement của cả hai gia đình, và statement token dẫn tới dữ liệu công nợ của gia đình khác. Baseline đã có đúng tiền lệ — `uq_contacts_phone` tồn tại với comment "trùng số trong cùng giáo viên sẽ làm vỡ việc gộp thông báo và gộp công nợ", lý do y hệt.

```sql
CREATE UNIQUE INDEX uq_contacts_zalo_user
    ON contacts(teacher_id, zalo_user_id)
    WHERE zalo_user_id IS NOT NULL AND deleted_at IS NULL;
```

Index không chặn được việc chọn sai người, nhưng chặn đúng cái subset phát hiện được. Nếu duplicate mapping là hành vi chấp nhận có chủ đích thì cần ghi vào schema comment, vì hiện không có gì nói lên điều đó.

## Majors

**M1. Friend có `displayName` rỗng thì không map được** — `zalo/dto.go:46`, `contacts/dto.go:28`

`FetchFriends` decode cả `zaloName`, `Service.Friend` mang nó qua, rồi `newFriendResponses` bỏ đi. Nếu Zalo trả `displayName` rỗng (friend chưa được đặt alias), picker hiện dòng trắng và PUT trả 422 vì `ZaloName` có `binding:"required,min=1"` — teacher không có đường nào map bạn đó. Fix rẻ nhất: trong `newFriendResponses` fallback sang `f.ZaloName` khi `f.DisplayName == ""`. Việc này xử lý luôn chuyện `ZaloName` đang là surface chết — plan phase 1 chỉ yêu cầu `userId`/`displayName`/`avatar`, nên trường này hiện là YAGNI drift; biến nó thành fallback là cách chính đáng để giữ.

**M2. Nhánh chuyển `zalo_personal` → `zalo_manual` của down migration chưa từng chạy với dữ liệu**

`TestMigrationRoundTrip` down tới zero với `notifications` rỗng, nên câu `UPDATE` và việc validate lại CHECK cũ chưa bao giờ thực thi thật — đúng acceptance criterion #1 nhưng chưa được chứng minh. Logic tôi đọc thì đúng: `UPDATE` chạy khi CHECK mới còn hiệu lực nên cho phép cả hai giá trị, và notifications không có unique index nào để việc fold gây vỡ. Thêm test: up → insert notification `channel='zalo_personal'` → `MigrateDown(m, 4)` → assert row thành `zalo_manual` và constraint về danh sách cũ.

**M3. Guard credential end-to-end chưa cover endpoint mới**

`TestNoResponseCarriesCredentialMaterial` (`zalo/handler_test.go:370`) record body của link/status, status, unlink nhưng không record `GET /me/zalo/friends`. `TestResponseTypesHaveNoCredentialFields` chỉ so tên field với denylist, không nhìn body thật. Thêm một dòng `record(w)` cho friends.

**M4. Mỗi request friends là một cú gọi live Zalo, không cache, không rate limit**

`FetchFriends` gửi `count: 20000`, không TTL cache; cache miss ở `sessionFor` còn kèm một lần relogin. Picker mở lại nhiều lần nghĩa là nhiều request liên tiếp từ một tài khoản Zalo cá nhân — đúng loại hành vi Zalo hay rate-limit hoặc flag. Không đề nghị phân trang (đã locked). Đề nghị phase 4 đặt `staleTime` cho query, hoặc cache ngắn phía server. Liên quan: `doRequest` không check `resp.StatusCode`, nên 429 từ Zalo hiện ra thành 500 generic.

## Minors

- `omitempty` trên `zalo_user_id`/`zalo_name` (`contacts/dto.go:37-38`) làm key biến mất hẳn khi chưa map thay vì `null`. `contactSchema` phía web là zod non-strict nên không vỡ hiện tại, nhưng phase 4 phải khai `.optional()` chứ không phải `.nullable()`.
- `required,min=1` là dư (`required` đã chặn `""`), và không trim: `{"zalo_user_id":"   "}` được nhận và ghi thẳng xuống DB.
- `UpdateZaloMapping` chạy 2 query (UPDATE rồi `GetByID`) ngoài transaction; contact bị xoá xen giữa thì client nhận 404 dù ghi đã thành công. Vô hại với workload một giáo viên.
- Doc comment lệch tên hàm: `// linkedAccount stores ...` đứng trên `func storeLinkedAccount` (`zalo/service_test.go:601`).
- `notification_runs` không có `updated_at` trong khi các bảng khác đều có.
- Text plan ghi "chưa link → 409/apperror `ErrNotLinked`" còn code trả 404. Code đúng (khớp `linkError`, khớp các endpoint `/me/zalo` hiện có, khớp acceptance criteria); plan cần chỉnh chữ.

## Những chỗ tôi kiểm riêng và thấy ổn

Đáng nêu vì đây là các điểm dễ vỡ nhất của slice. `Service.Update` (`contacts/service.go:63`) load row rồi copy `row.Contact` trước khi `Save`, nên `Save` ghi lại đúng giá trị zalo cũ — mapping không bị null hoá âm thầm khi teacher sửa tên/số điện thoại; đây là regression đắt nhất có thể xảy ra và nó không xảy ra. Tên `notifications_channel_check` khớp auto-name của Postgres cho inline column CHECK, và `TestZaloPersonalMappingSchema` assert theo đúng `conname` đó nên lệch tên sẽ đỏ test. Đường credential sạch: `doRequest`/`newRequest` đã strip query string (URL Zalo mang IMEI cleartext + ZCID), các error trong `FetchFriends`/`decryptDataField`/`encryptPayload` không nhả key hay cookie, và error lạ đi qua `apperror.From` → `Internal` nên client chỉ thấy "internal server error".

## Verdict theo acceptance criteria

| # | Nội dung | Kết quả |
|---|---|---|
| 1 | Migration up/down cycle, tên constraint | Logic đúng, tên đúng — thiếu test dữ liệu (M2) |
| 2 | Mapping endpoints | Pass |
| 3 | `GET /me/zalo/friends` 200/404/409 + auth | Pass |
| 4 | Không rò credential | Pass — canary walk chưa cover friends (M3) |
| 5 | Không regression CRUD/link | Pass |
| 6 | Theo pattern hiện có | Pass, trừ convention tenant-integrity của `notification_runs` (B1) |
| 7 | Lint/type/build | Pass, đã verify lại |

Kiểm chứng lại của tôi: `go vet ./...` sạch, `go test ./internal/features/contacts/... ./internal/features/zalo/...` xanh cả 3 package, golangci-lint v2.7.2 báo 0 issues.

## Câu chưa có lời

1. `displayName` rỗng có thật xảy ra trong payload `getfriends` không? Quyết định M1 phụ thuộc câu này.
2. Hai contacts map cùng một người Zalo là bug hay use case thật (một người đại diện hai gia đình)? Quyết định B2 phụ thuộc câu này.

Status: DONE_WITH_CONCERNS
Summary: Slice đúng convention và không có lỗ rò credential; hai vấn đề schema (thiếu ràng buộc tenant cho `run_id`, thiếu unique index `(teacher_id, zalo_user_id)`) nên sửa trước khi land vì migration 000005 còn chưa commit.
Concerns/Blockers: B1 và B2 ở trên. Thêm nữa, nhánh down-migration chuyển channel chưa có test với dữ liệu thật, friend có `displayName` rỗng hiện không map được, và tôi không ghi được file report nên team-lead cần tự persist nội dung này nếu muốn lưu vết.
