# Code review — Phase 2: mô tả rich text (sanitize, giới hạn, migration 000023)

Ngày: 2026-09-17 · Reviewer: reviewer-phase2 · Phạm vi: diff chưa commit trên `master`

## Phạm vi đã xem

- Mới: `apps/api/internal/features/tasks/description.go`, `description_test.go`,
  `apps/api/migrations/000023_task_description_html.up.sql`, `.down.sql`
- Sửa: `apps/api/internal/features/tasks/{dto.go,errors.go,handler.go,service.go,service_test.go}`,
  `apps/api/migrations/migrations_test.go`, `apps/api/go.mod`, `go.sum`, `docs/api-guidelines.md`
- Generated: `apps/api/docs/*` (không review tay, chỉ kiểm diff sạch)

## Kiểm chứng đã chạy

| Kiểm chứng | Kết quả |
|---|---|
| `go build ./...` | pass |
| `go test ./internal/features/tasks/... ./pkg/kanban/...` | pass |
| `make test-api-unit` | pass, không FAIL |
| `make lint-api` (gồm `scopelint`) | 0 issues |
| `go test ./pkg/kanban -run TestImportBoundary` | pass; `go list -deps ./pkg/kanban \| grep bluemonday` = 0 |
| `go vet -tags integration ./internal/features/tasks/... ./migrations/...` | pass (integration test vẫn biên dịch) |
| `make api-docs` rồi so md5 | 3 file `docs/*` không đổi → diff sạch |
| Probe bảo mật độc lập (25 vector XSS/mXSS + DoS) | không có bypass |

Integration test có Docker không chạy (theo yêu cầu, tránh chạy song song).

## Trạng thái acceptance criteria

- **AC5** — đạt đủ. Có test cho `<script>`/`onclick`/`javascript:`/`<img>` →
  chỉ còn allowlist (`description_test.go:10-64`), `<p></p>` → `""`
  (`description_test.go:86-94`), `a\nb` → `<p>a<br>b</p>`
  (`description_test.go:72`, `service_test.go:768-773`), >4000 rune → 422
  `fields.description` (`service_test.go:776-791`), và — sau vòng sửa ngày
  2026-09-17 — vế "body > 20000 rune → 422" qua
  `TestDescriptionBindingCapsRawMarkup` (`service_test.go`, cuối file). Xem phần
  "Bổ sung" ở cuối báo cáo.
- **AC6** — đạt. `TestTaskDescriptionWrapsLegacyPlainText`
  (`migrations_test.go:2143-2217`) phủ rỗng-giữ-rỗng, escape `<`/`&`, `\r\n`+`\n`
  → `<br>`, hàng soft-deleted, và text bắt đầu bằng `<p>` bị escape; down chạy
  không lỗi. "Up idempotent" được chứng minh bằng vòng up → down → up cho cùng
  kết quả, đúng với quyết định đã chốt là `schema_migrations` chặn chạy hai lần.
- **TestImportBoundary** — xanh, `pkg/kanban` vẫn dependency-free.
  `govulncheck` không có trong `Makefile` (`lint-api` chỉ gọi `scopelint` +
  `golangci-lint`) nên vế này N/A.
- **Swagger** — mô tả subset đúng ở cả create và update, `maxLength` 4000 → 20000
  ở hai DTO; `make api-docs` tái sinh ra đúng file đang có.

## Critical

Không có.

## High

Không có.

## Medium

### M1 — Chuỗi bắt đầu bằng `<` nhưng không phải HTML mất xuống dòng và mất `<p>`

`apps/api/internal/features/tasks/description.go:69`

Heuristic `!strings.HasPrefix(trimmed, "<")` phân loại `"<3 me\nline two"` là
HTML. Nhánh HTML không đổi `\n` thành `<br>`, nên kết quả lưu là
`"&lt;3 me\nline two"`: không có block wrapper và xuống dòng biến mất khi render
(đã kiểm bằng probe chạy đúng policy hiện tại). Cùng một nội dung nếu bắt đầu
bằng ký tự khác thì được bọc `<p>…<br>…</p>` đúng.

Đây không phải mở lại quyết định "prefix `<` = HTML" mà là bằng chứng mới về hệ
quả của nó với text người dùng gõ (`<3`, `<= 5`, `<Tên>`). Xác suất thấp sau
phase 5 (TipTap luôn gửi `<p>`), nhưng client cũ và curl thì gặp.

Gợi ý: thu hẹp heuristic về ký tự thật sự mở thẻ, ví dụ chỉ coi là HTML khi khớp
`^<[a-zA-Z!/]`; hoặc sau sanitize, nếu kết quả không chứa block nào thì bọc
`<p>…</p>`. Nếu chấp nhận hiện trạng thì nên ghi rõ vào doc comment của
`normalizeDescription`.

### M2 — Client web vẫn cap 2000 ký tự, chặn lưu lại mô tả legacy sau migration

`apps/web/src/features/tasks/schemas/task-schemas.ts:104`
(`z.string().max(2000, "Mô tả tối đa 2000 ký tự")`)

Sau migration 000023, một mô tả legacy dài ~1995 ký tự trở thành `<p>…</p>` dài
hơn 2000. Form web hiện tại nạp nguyên chuỗi đã lưu vào textarea; người dùng mở
việc đó ra sửa sẽ bị chính validation client chặn, dù API chấp nhận. Ngoài ra
trong khoảng giữa deploy API và web, textarea hiển thị markup thô.

Đây là việc của phase 5/6 nhưng phải được ghi nhận ngay để không rơi: nâng/xoá
cap client và đổi textarea sang editor phải đi cùng lần deploy này (plan đã ghi
rủi ro deploy đồng thời, chưa ghi rủi ro cap 2000).

### M3 — Sanitize làm phình output; giới hạn 20000 là trên input, không phải trên dữ liệu lưu

`apps/api/internal/features/tasks/dto.go:152,166` và
`description.go:64-81`

`RequireNoFollowOnLinks` + `RequireNoReferrerOnLinks` +
`AddTargetBlankToFullyQualifiedLinks` thêm ~45 byte cho mỗi `<a>`. Đo thực tế:
input 20000 ký tự toàn link → output 51875 byte (hệ số 2.6). Cột là `TEXT` nên
không lỗi ghi, nhưng `Board` trả tối đa 50 task/cột kèm nguyên `description`
(`service.go:22,52-80`), nên payload board xấu nhất tăng từ ~800 KB/cột lên
~2.6 MB/cột nếu có người cố tình nhồi.

Gợi ý: hoặc kiểm thêm `len(sanitized)` sau sanitize (ví dụ 64 KB) và trả cùng
lỗi 422, hoặc chấp nhận có chủ ý và ghi vào doc comment của DTO rằng cap 20000
chỉ chặn input. Câu chữ trong `docs/api-guidelines.md` hiện đã nói đúng ("bounds
the raw markup"), chỉ thiếu ràng buộc phía output.

## Low

- **L1** — ~~Không có test cho `binding:"max=20000"`~~ **Đã xử lý** bằng
  `TestDescriptionBindingCapsRawMarkup`; xem phần "Bổ sung".
- **L2** — Doc comment `description.go:24-25` nói link "always carry
  `rel=\"nofollow noreferrer\"`"; với link tuyệt đối bluemonday xuất
  `rel="nofollow noreferrer noopener"`. Test `description_test.go:44` đã ghi
  đúng chuỗi thật; chỉ comment lệch.
- **L3** — `migrations_test.go:2190,2205`: test lên tới đỉnh bằng
  `database.MigrateUp(m)` rồi `MigrateDown(m, 1)`. Khi có 000024, bước down 1 sẽ
  gỡ nhầm migration và test fail (fail ồn, không âm thầm). Dùng `m.Migrate(23)`
  và `m.Migrate(22)` sẽ bền hơn. Cùng họ với bảo trì `MigrateDown(m, 19)` ở dòng
  326 mà diff đã cập nhật đúng.
- **L4** — bluemonday không cân bằng thẻ: `<p><strong>a` được giữ nguyên, không
  đóng. Không phải XSS (parser trình duyệt đóng tại biên container khi render
  bằng `dangerouslySetInnerHTML`), nhưng phase 5 phải render trong một container
  riêng chứ không nối chuỗi vào markup khác.
- **L5** — `service_test.go:786-787` chỉ assert `ae.Fields` có khoá
  `description`, không assert thông điệp `"tối đa 4000 ký tự"` mà AC5 nêu.
- **L6** — Thẻ ngoài allowlist bị bỏ và giữ text, nên `<table><tr><td>a</td><td>b</td></tr></table>`
  thành `"ab"` (mất ranh giới ô) — hành vi của allowlist đã chốt, chỉ ghi nhận
  để phase 5 không dán nội dung bảng từ Word rồi bất ngờ.

## Đã kiểm và không có vấn đề

**Bảo mật sanitize.** 25 vector chạy qua đúng policy hiện tại đều sạch:
`JaVaScRiPt:`, `javascript:` có khoảng trắng/tab/`\n`/null byte, entity
`&#106;avascript:`, `data:`, `vbscript:`, `//evil.com`, `<a>` không href,
`target`/`rel` do client tự đặt (bị ghi đè), `<style>`, `<iframe srcdoc>`,
`<form>/<button formaction>`, `<svg><foreignObject>`, `<textarea>`,
`<noscript>` title-breaker, và vector mXSS
`<math><mtext><table><mglyph><style><!--</style><img src=x onerror=...>` (ra
chuỗi rỗng). Scheme viết hoa được chuẩn hoá về `http://`. `mailto:` giữ href,
không bị gắn `target` — đúng yêu cầu phase.

**DoS.** `BodyLimit` toàn cục 1 MiB (`internal/server/router.go:76`,
`config.go:45`) đứng trước binding, nên 20000 rune (≤ 80 KB UTF-8) không phải là
đường phình. Sanitize hai lượt parse: lồng 2000 cấp `<ul><li>` (36001 rune) mất
8.3 ms, 6600 thẻ `<p>` mở mất 5 ms, 555 link mất 4.9 ms. Không có hành vi bậc
hai.

**Đếm rune.** validator `max` trên string đếm rune (không phải byte), nên cap
20000 đúng như plan ghi. `descriptionText` strip thẻ rồi `html.UnescapeString`
nên `&amp;` tính 1 rune; test `description_test.go:112-116` chốt đúng biên
4000/4001.

**Thứ tự escape migration.** Up: `&` → `&amp;` chạy trước `<`/`>` nên không
double-escape; `<br>` được chèn **sau** khi escape nên không bị escape lại;
`E'\r\n'` lồng trong nên khớp trước `E'\n'` (CRLF ra đúng một `<br>`). Go
`strings.NewReplacer` quét một lượt, không quét lại output, nên khớp hành vi
SQL — tôi đã đối chiếu từng nhánh. Down: `<br\s*/?>` và `</(p|li)>` → `\n`, rồi
strip `<[^>]+>`, rồi unescape `&lt;`, `&gt;`, `&#34;`, `&#39;`, `&amp;` **cuối
cùng**; `&amp;lt;` không bị hiểu nhầm thành `&lt;` (ký tự `&` đứng trước `a`,
không phải `l`), `&amp;#39;` cũng vậy. `rtrim(…, E'\n')` dọn `\n` thừa do
`</p>`. Lossy đã ghi rõ trong comment SQL.

**NULL/empty.** `tasks.description` là `TEXT NOT NULL DEFAULT ''`
(`000022_task_board.up.sql:33`), nên `WHERE description <> ''` không bỏ sót hàng
NULL nào. Phía Go, chuỗi rỗng và tài liệu toàn markup đều quy về `""`, khớp
DEFAULT.

**Không regression business logic.** `CreateTask`/`UpdateTask` không có caller
nào ngoài handler và test trong chính package (grep toàn `apps/api`). `MoveTask`
đi qua `taskRepository.Update` và chuyển tiếp `description` đã sạch, không
normalize lại — đúng. Không có audit/event nào mang `description`
(`events.go` chỉ có `ColumnDeleted`; `middleware/request_events.go` không chụp
body). `seeds/` không tạo mô tả. Integration test của tasks không assert
`description` nên không vỡ. Không có e2e nào chạm field này.

**Public contract.** `TaskResponse` giữ nguyên hình dạng và kiểu; binding nới từ
4000 lên 20000 là nới lỏng, tương thích ngược. Migration là cặp 000023 mới,
không sửa migration cũ. Ngữ nghĩa `description` đổi từ text sang HTML subset —
đây là thay đổi contract có chủ ý, plan đã ràng buộc deploy chung với web
(phase 6); xem thêm M2.

**Pattern.** `errDescriptionTooLong` là sentinel feature-level, map trong
`translateError` đúng vị trí trước `kanban.ErrInvalidInput`, không rò vào
`pkg/kanban`. Test migration theo đúng khuôn của file (`startBarePostgres`,
`seedNotificationParents`, `t.Parallel`). Comment giải thích invariant, không có
plan ID hay số phase trong code. `docs/api-guidelines.md` đặt đoạn rich text
ngay sau mục validation, đúng chỗ.

## Khuyến nghị theo thứ tự

1. Quyết định M1: thu hẹp heuristic `<` hoặc ghi nhận hạn chế vào doc comment.
2. Ghi M2 vào phase 5/6 để cap 2000 phía web được nâng cùng lần deploy.
3. Quyết định M3: thêm cap trên `len(sanitized)` hoặc ghi rõ là chấp nhận.
4. Vá L2 (comment `rel`) và L3 (`m.Migrate(23)`/`m.Migrate(22)`) — chi phí gần 0.
5. L5 nếu muốn đóng AC5 đúng từng chữ (assert cả thông điệp lỗi).

## Câu hỏi còn mở

- M3: có chủ ý để dữ liệu lưu vượt 20000 ký tự không, hay cần cap output?
- M1: `<3` ở đầu mô tả có nằm trong kịch bản người dùng thật của trung tâm không?

## Bổ sung — vòng sửa ngày 2026-09-17 (`TestDescriptionBindingCapsRawMarkup`)

Team-lead thêm test cuối `apps/api/internal/features/tasks/service_test.go`,
import thêm `github.com/gin-gonic/gin/binding`. Đã review và chạy lại:

- `go test -run TestDescriptionBindingCapsRawMarkup` pass; `make lint-api`
  0 issues; `gofmt -l internal/features/tasks/` rỗng; cả package vẫn xanh.
- Test **thật sự chứng minh hành vi**, không phải test hình thức: nó ghim đúng
  biên 20000 (đạt) / 20001 (lỗi) trên cả `CreateTaskRequest` và
  `UpdateTaskRequest`, nên đổi hoặc bỏ tag `max=20000` là fail ngay.
- Chuỗi dùng để test là 20000 ký tự `ă` (40000 byte), nên nó chứng minh luôn
  điều mà comment khẳng định: validator đếm rune chứ không đếm byte. Comment và
  hành vi khớp nhau.
- `validation.BindError(err)` trả `*apperror.AppError`
  (`internal/shared/validation/validation.go:66`) nên `ae.Code` và `ae.Fields`
  hợp lệ; đường map sang 422 `fields.description` là đúng đường mà handler dùng
  (`handler.go:234-237`).
- Nhóm import đặt đúng thứ tự std → third-party → local, `binding` đứng trước
  `uuid` theo alphabet. Không có plan ID hay số phase trong comment.

Một ghi nhận mức Low mới:

- **L7** — Các feature khác kiểm binding ở tầng handler bằng `httptest`
  (`sessions/handler_test.go`, `students/handler_test.go`,
  `auth/handler_test.go`); gọi thẳng `binding.Validator.ValidateStruct` là cách
  dùng đầu tiên trong repo. Chấp nhận được ở đây vì package `tasks` chưa có
  harness HTTP và dựng một cái chỉ để kiểm một tag thì đắt hơn giá trị. Điều
  còn lại chưa được test là handler thật sự gọi `ShouldBindJSON` +
  `validation.BindError`, nhưng đó là đường dùng chung của mọi endpoint và nhìn
  thấy trực tiếp ở `handler.go:234-237`.

Ba Medium (M1, M2, M3) không đổi sau vòng sửa này.

```
Status: DONE_WITH_CONCERNS
Summary: Phase 2 đạt đủ AC5 (sau khi thêm TestDescriptionBindingCapsRawMarkup) và AC6; không tìm được bypass XSS hay lỗi thứ tự escape trong migration; build/lint/unit test/`make api-docs` đều sạch.
Concerns/Blockers: M1 chuỗi bắt đầu bằng `<` nhưng là plain text bị mất xuống dòng và mất `<p>`; M2 cap 2000 ký tự phía web sẽ chặn lưu lại mô tả legacy sau migration (cần đi cùng phase 5/6); M3 sanitize phình output 2.6x nên cap 20000 không ràng buộc dữ liệu lưu lẫn payload board.
```

## Xử lý findings (controller, 2026-09-17 18:30)

| Finding | Quyết định | Bằng chứng |
|---|---|---|
| M1 | **Sửa.** `normalizeDescription` chỉ coi là HTML khi khớp `^<[A-Za-z!/]` (`markupStart`); `<3 me\nline two` → `<p>&lt;3 me<br>line two</p>`. | case mới trong `TestNormalizeDescriptionWrapsPlainText` |
| M2 | **Chấp nhận, chuyển Phase 5/6.** Schema zod mới (text ≤ 2000, HTML ≤ 20000) đã nằm trong phase-05; ghi thêm rủi ro "deploy cùng API" vào phase-05. | `phase-05-web-rich-text-editor.md` mục Risk |
| M3 | **Chấp nhận.** Phình ~2.6x bị chặn gián tiếp bởi binding cap 20000 rune và BodyLimit 1 MiB (tối đa ~52 KB/mô tả); ghi rõ trong doc comment `descriptionPolicy`. | `description.go` |
| L1 | **Đã đóng** trước khi review kết thúc: `TestDescriptionBindingCapsRawMarkup` qua `binding.Validator`. | `service_test.go` |
| L2 | **Sửa** doc comment: nêu `noopener` cho link tuyệt đối. | `description.go` |
| L3 | **Sửa**: test migration dùng `m.Migrate(23)` / `m.Migrate(22)` thay vì `MigrateUp`/`MigrateDown(m, 1)`. | `migrations_test.go` |
| L4, L6 | **Ghi nhận cho Phase 5** (render trong container riêng qua DOMPurify; bảng mất ranh giới ô). | phase-05 mục Risk |
| L5 | **Sửa**: assert thông điệp `"tối đa 4000 ký tự"`. | `service_test.go` |

Sau khi vá: `make test-api-unit` ok, `make lint-api` 0 issues, `go test -tags integration -p 1 ./migrations/... ./internal/features/tasks/...` ok, `make api-docs` tái sinh đúng 3 file.
