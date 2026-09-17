---
phase: 2
title: "API: mô tả rich text (sanitize, giới hạn, migration 000023)"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 2: API mô tả rich text (sanitize, giới hạn, migration 000023)

## Overview

`description` trở thành HTML subset đã sanitize phía server (bluemonday) với
allowlist cố định; giới hạn văn bản thuần 4000 rune; migration 000023 bọc dữ
liệu cũ. Core `pkg/kanban` không đổi (mô tả với core vẫn là chuỗi mờ).

## Requirements

- Functional:
  - Allowlist: `p`, `br`, `strong`, `em`, `u`, `s`, `ul`, `ol`, `li`,
    `a[href]` với scheme `http`, `https`, `mailto`; `a` được thêm
    `rel="noopener noreferrer nofollow"` và `target="_blank"`. Mọi thứ khác
    (thẻ, thuộc tính `style`/`class`/`on*`, `data:`/`javascript:` URL) bị bỏ,
    giữ text bên trong.
  - Chuẩn hoá: nếu chuỗi (sau trim) không bắt đầu bằng `<` thì coi là văn bản
    thuần và bọc như migration (escape `&<>`, `\n` → `<br>`, `<p>…</p>`)
    **trước** khi sanitize — tránh "nửa HTML" (client cũ/curl gửi plain text
    có xuống dòng). Sau sanitize: strip whitespace hai đầu; kết quả chỉ còn
    `<p></p>`/`<p><br></p>` hoặc rỗng → `""`.
  - Giới hạn: body thô `binding:"max=20000"` (validator đếm **rune**, không
    phải byte — đủ dùng) trên cả Create/Update;
    văn bản thuần (bluemonday `StrictPolicy` để strip → `html.UnescapeString`)
    `utf8.RuneCountInString ≤ 4000` → nếu vượt: 422
    `fields.description = "tối đa 4000 ký tự"`.
  - `TaskResponse.description` trả HTML đã lưu (không đổi kiểu).
  - Migration 000023: `UPDATE tasks SET description = '<p>' || replace(…escape…) || '</p>' WHERE description <> ''` (bao gồm cả hàng soft-deleted để trạng thái đồng nhất; **không** thêm guard `NOT LIKE '<p>%'` — mô tả legacy là văn bản thuần, một mô tả cũ bắt đầu bằng chữ `<p>` cũng phải được escape; idempotency do `schema_migrations` đảm bảo); `\r\n`/`\n` → `<br>`; down: `<br>` → `\n`, strip thẻ, unescape theo thứ tự `&lt;`, `&gt;` rồi `&amp;` cuối cùng (best-effort, ghi rõ lossy trong comment SQL).
- Non-functional:
  - `github.com/microcosm-cc/bluemonday v1.0.27` chỉ được import trong
    `internal/features/tasks`; `import_boundary_test` của core vẫn xanh.
  - Policy bluemonday tạo một lần (package-level `var`), an toàn goroutine.
  - **Backup DB trước khi migrate** ở môi trường có dữ liệu thật (prod bắt
    buộc — phase 6); dev/e2e stack seed lại từ đầu nên không cần.

## Architecture

```
CreateTaskRequest.Description ─┐
UpdateTaskRequest.Description ─┴► tasks.Service: desc, err := normalizeDescription(raw)
                                     │ sanitized := descriptionPolicy.Sanitize(raw)
                                     │ sanitized = strings.TrimSpace(sanitized)
                                     │ if isBlankHTML(sanitized) → ""
                                     │ if runeCount(plainText(sanitized)) > 4000 → ErrDescriptionTooLong
                                     ▼
                              kanban.Service.CreateTask/UpdateTask (Description = sanitized)
```

File mới `apps/api/internal/features/tasks/description.go`:

```go
// descriptionPolicy is the single allowlist for task descriptions. It is
// the trust boundary: whatever the client sends, only this subset is stored.
var descriptionPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "u", "s", "ul", "ol", "li")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}()

var textOnlyPolicy = bluemonday.StrictPolicy()

const maxDescriptionRunes = 4000

// normalizeDescription wraps plain text (no leading '<') into escaped
// paragraphs, sanitizes to the allowlist, collapses an empty document to
// "", and rejects text longer than maxDescriptionRunes.
func normalizeDescription(raw string) (string, error)
```

`errors.go` thêm sentinel feature-level `errDescriptionTooLong` → 422
`fields.description`. (Không đưa vào core: core không biết HTML.)

Migration `000023_task_description_html.up.sql`:

```sql
-- Wrap legacy plain-text descriptions in a single <p> so every stored
-- description is the sanitized HTML subset the API now writes. Text is
-- HTML-escaped first; line breaks become <br>. Empty stays empty.
UPDATE tasks
SET description = '<p>' ||
  replace(replace(
    replace(replace(replace(description, '&', '&amp;'), '<', '&lt;'), '>', '&gt;'),
    E'\r\n', '<br>'), E'\n', '<br>') ||
  '</p>'
WHERE description <> '';
```

Không có guard theo nội dung: mọi mô tả legacy là văn bản thuần nên đều phải
escape; chạy lại được ngăn bởi `schema_migrations`. Sau migrate kiểm:
`SELECT count(*) FROM tasks WHERE description <> '' AND description NOT LIKE '<p>%'`
phải bằng 0. Down: `regexp_replace(description, '<br\s*/?>', E'\n', 'gi')` →
strip `<[^>]+>` → unescape `&lt;`, `&gt;` rồi `&amp;` sau cùng; comment rõ
"lossy: bỏ định dạng".

## Related Code Files

- Create: `apps/api/internal/features/tasks/description.go`
- Create: `apps/api/internal/features/tasks/description_test.go` (bảng: script, on*, javascript:, img, style, nested list hợp lệ, `<p></p>` → "", 4000/4001 rune, tiếng Việt có dấu đếm rune đúng, link mailto giữ, link `ftp:` → bluemonday bỏ **cả thẻ** `<a>` chỉ còn text, plain text có `\n` → `<p>a<br>b</p>`, plain text chứa `<b>` → escape thành `&lt;b&gt;`)
- Create: `apps/api/migrations/000023_task_description_html.up.sql`, `.down.sql`
- Modify: `apps/api/migrations/migrations_test.go` (hoặc test parity mới) — fixture 3 hàng: rỗng, có `<`/`&`/`\n`, văn bản thuần bắt đầu bằng `<p>` (phải bị escape) → assert sau up; down chạy không lỗi và unescape đúng thứ tự
- Modify: `apps/api/internal/features/tasks/dto.go` (`binding:"max=20000"` cho `Description` ở Create và `omitempty,max=20000` ở Update; comment nêu HTML subset)
- Modify: `apps/api/internal/features/tasks/service.go` (`CreateTask`/`UpdateTask` gọi `normalizeDescription` trước khi vào core)
- Modify: `apps/api/internal/features/tasks/errors.go` (map `errDescriptionTooLong`)
- Modify: `apps/api/internal/features/tasks/service_test.go` (HTTP: payload XSS → lưu sạch; quá dài → 422)
- Modify: `apps/api/internal/features/tasks/handler.go` (swag `@Description` cho create/update: "description is an HTML subset (p, br, strong, em, u, s, ul, ol, li, a); server sanitizes; ≤ 4000 text characters")
- Modify: `apps/api/go.mod`, `go.sum` (`go get github.com/microcosm-cc/bluemonday@v1.0.27`)
- Modify: `docs/api-guidelines.md` (mục ngắn: "Rich text fields — sanitize at the feature layer with bluemonday; allowlist lives next to the DTO")
- Generated: `apps/api/docs/*`

## Implementation Steps

1. `go get github.com/microcosm-cc/bluemonday@v1.0.27` trong `apps/api`; chạy `go mod tidy`; xác nhận `go list -deps ./pkg/kanban` không có bluemonday (`TestImportBoundary`).
2. Viết `description.go` + `description_test.go` theo bảng ở trên. Kiểm `AddTargetBlankToFullyQualifiedLinks` chỉ thêm `target` cho link tuyệt đối — `mailto:` không bị thêm (test một case).
3. `service.go`: trong `CreateTask` và `UpdateTask` (nhánh `req.Description != nil`) gọi `normalizeDescription`; lỗi → `translateError` → 422. Ghi chú: chuỗi rỗng sau sanitize lưu `""` để `description = ''` DEFAULT vẫn đúng ngữ nghĩa.
4. `dto.go`: đổi tag binding; `handler.go`: swag; `make api-docs`.
5. Migration 000023 up/down; test parity: tạo 3 task bằng SQL thô trước migration (dùng helper `migrations_test.go` chạy tới 000022 rồi lên 000023) và assert nội dung.
6. Feature HTTP test: POST create với `description: "<p>Hi</p><script>alert(1)</script><a href=\"javascript:x\" onclick=\"y\">l</a>"` → lưu `<p>Hi</p>l` (bluemonday bỏ hẳn `<a>` không có href hợp lệ; assert không chứa `javascript`/`onclick`/`script`); POST với plain text `"a\nb"` → lưu `<p>a<br>b</p>`; PATCH với 4001 chữ "ă" → 422 `fields.description`.
7. Cập nhật `docs/api-guidelines.md`; gate `make test-api-unit`, `make lint-api`, `make test-api`, `make api-docs` diff sạch.

## Success Criteria

- [ ] AC5 đầy đủ trong `description_test.go` + HTTP test.
- [ ] AC6: migration test up/down xanh; up idempotent (chạy 2 lần cùng kết quả).
- [ ] `TestImportBoundary` xanh; `govulncheck ./...` (nếu có trong lint) không báo bluemonday.
- [ ] Swagger mô tả đúng subset; `make api-docs` diff sạch.

## Risk Assessment

- **bluemonday bỏ hẳn `<a>`** khi URL sai scheme (đã kiểm với v1.0.27): chỉ còn text — an toàn; TipTap phía client cũng chỉ cho `http/https/mailto` (phase 5) nên hiếm gặp.
- **Đếm rune trên text sau `StrictPolicy`** có thể còn entity (`&amp;`) → `html.UnescapeString` trước khi đếm; test "&&&&" 4000 lần.
- **Migration trên prod chạy lâu**: bảng `tasks` nhỏ (một trung tâm vài trăm việc) → không cần batch. Tín hiệu vỡ: lock > vài giây → không xảy ra ở quy mô này; nếu có, chia theo `center_id`.
- **Client web cũ nhìn thấy HTML thô** trong khoảng giữa deploy API và web → deploy cùng lúc (phase 6), chấp nhận vài giây.
