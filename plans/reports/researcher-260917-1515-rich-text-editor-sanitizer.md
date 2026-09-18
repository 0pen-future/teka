# Research: Rich-text editor cho task description + sanitize phía Go

Ngày: 2026-09-17. Bối cảnh: `description` hiện là TEXT (Postgres), Gin binding `max=4000`
(`apps/api/internal/features/tasks/dto.go:150,164`), sửa qua `<textarea>` trong
`apps/web/src/features/tasks/components/task-form-modal.tsx`. Chưa render ở đâu khác.

## 1. Chọn thư viện editor cho React 19

| Tiêu chí | TipTap v3 | Lexical | Plate/Slate |
|---|---|---|---|
| Bản mới nhất (npm, kiểm tra 2026-09-17) | `@tiptap/react` 3.31.3, `@tiptap/starter-kit` 3.31.3 (publish 2026-09-04), `@tiptap/extension-link` 3.31.3, `@tiptap/pm` 3.31.3 | `lexical` 0.50.0, `@lexical/react` 0.50.0 | Plate phụ thuộc `slate`/`slate-react`, version rời rạc, ít đồng bộ |
| License | MIT toàn bộ core; 10 extension "Pro" trước đây (comment, version history, AI...) đã open-source MIT từ giữa 2025 — không cần license trả phí cho bold/italic/underline/strike/list/link/heading | MIT (Meta) | MIT |
| Ổn định | v3.0 stable từ tháng 7/2025 (rời beta), 3.31.x là bản vá liên tục, ~9M download/tháng | Meta dùng cho Messenger/WhatsApp Web, ổn định nhưng API thấp hơn (phải tự lắp ráp nhiều hơn) | Slate cảnh báo composition trên Android/IME "không dùng production được ở nhiều ngôn ngữ" (GH issue #2062, #4400, #5989, báo cáo kéo dài 2017-2025 chưa fix) |
| Bundle size (gzip, đã đo qua bundlephobia API) | `@tiptap/react` 8.3 KB + `@tiptap/starter-kit` 105.5 KB (gồm `@tiptap/core` ~34.8 KB, `@tiptap/pm` là peer riêng) + `@tiptap/extension-link` 13.1 KB → tổng thực tế cho bộ tối thiểu (bold/italic/underline/strike/list/link) ~110-130 KB gzip vì nhiều phần của starter-kit dùng chung | `lexical` core một mình đã 55.9 KB gzip; `@lexical/react` cộng thêm (không đo được chính xác do bundlephobia rate-limit 429 khi thử lại) nhưng theo tài liệu cộng đồng cỡ tương đương hoặc nhỉnh hơn TipTap khi dùng React binding + rich-text plugin, vì phải tự ghép nhiều package nhỏ (`@lexical/list`, `@lexical/link`, `@lexical/rich-text`...) | Slate/Plate nhìn chung nặng hơn cả hai do kiến trúc plugin dày |
| React 19 | Hỗ trợ chính thức từ TipTap 2.10 (11/2024), tiếp tục ở v3 (dùng `flushSync`, ref theo React 19) | Hỗ trợ, nhưng API dựa nhiều vào `useEffect`/manual DOM, ít ưu tiên React ref pattern mới | Có nhưng cộng đồng nhỏ, ít test với React 19 |
| IME / dấu tiếng Việt trên mobile | ProseMirror (nền của TipTap) xử lý composition tốt, là chuẩn de-facto cho editor production (Notion dùng riêng, nhưng nhiều SaaS lớn dùng ProseMirror) | Cũng ổn vì Lexical thiết kế lại từ đầu để tránh vấn đề composition của Draft.js, nhưng ít case study về gõ tiếng Việt cụ thể | Slate: bug composition Android xác nhận trực tiếp trong nhiều issue công khai, rủi ro cao nhất cho input có dấu qua bàn phím ảo |
| HTML in/out, isEmpty, getText, autolink | `editor.getHTML()`/`generateHTML` (`@tiptap/html`, dùng được server-side qua virtual DOM), `editor.isEmpty`, `editor.getText()` sẵn có; `@tiptap/extension-character-count` cho đếm ký tự/giới hạn qua `editor.storage.characterCount.characters()`; `@tiptap/extension-link` có `autolink`, `openOnClick`, `protocols` — pattern chuẩn: `openOnClick: false` khi editable (để click đặt caret thay vì nhảy trang), `openOnClick: true` khi readonly | Có nhưng phải tự viết nhiều (Lexical không có "characterCount" built-in tương đương, phải tự đếm qua `$getRoot().getTextContent()`) | Tương tự Lexical, phải tự lắp |
| Đọc thêm | tiptap.dev, github.com/ueberdosis/tiptap | lexical.dev, github.com/facebook/lexical | github.com/ianstormtaylor/slate |

**Khuyến nghị: TipTap v3.** Lý do xếp hạng: (1) license MIT đầy đủ cho đúng bộ tính năng cần
(bold/italic/underline/strike/list có sẵn trong `starter-kit`, chỉ thiếu `link` → thêm
`@tiptap/extension-link` riêng); (2) v3 đã stable hơn 1 năm, release gần nhất
(3.31.3, 2026-09-04) cho thấy bảo trì tích cực; (3) dựa trên ProseMirror — nền tảng composition/IME
đã kiểm chứng ở quy mô lớn, rủi ro thấp nhất cho input tiếng Việt trên mobile; (4) API cấp cao
(`useEditor`, `EditorContent`, `getHTML/getText/isEmpty`) khớp với nhu cầu form nhỏ, tránh phải tự
lắp ráp nhiều package nhỏ như Lexical; (5) Slate/Plate bị loại vì bug composition Android chưa fix
nhiều năm — rủi ro trực tiếp cho yêu cầu "phải chạy tốt trên mobile browser".
Đánh đổi: bundle ~110-130 KB gzip nặng hơn textarea, nhưng chấp nhận được nếu lazy-load (câu 5) và
không nằm trong bundle chính.

## 2. Định dạng lưu trữ ở API

3 lựa chọn: (a) HTML subset đã sanitize, (b) Markdown, (c) ProseMirror/Lexical JSON.

- **JSON (ProseMirror/Lexical)**: chính xác nhất với editor, nhưng ràng schema JSON đó vào một thư
  viện JS cụ thể — phía Go phải parse/validate cấu trúc JSON tùy biến (không có schema chuẩn hoá
  sẵn cho Go), khó viết allowlist tổng quát, và nếu đổi editor sau này phải viết migration phức tạp
  hơn nhiều so với đổi cách render HTML. Không có thư viện Go trưởng thành để validate JSON này.
- **Markdown**: nhẹ, dễ đọc, nhưng (i) cần một markdown-renderer nhất quán giữa editor (TipTap có
  extension markdown riêng, không phải core) và server render lại thành HTML để hiển thị — thêm một
  bước chuyển đổi và một nguồn lỗi khác; (ii) `[text](javascript:alert(1))` vẫn là vector XSS phải
  sanitize sau khi render sang HTML, nên không giảm được nhu cầu sanitize, chỉ dời bước đó; (iii)
  TipTap không native ra Markdown, phải thêm extension ngoài `starter-kit`.
- **Sanitized HTML subset**: TipTap xuất `getHTML()` trực tiếp — không cần bước chuyển đổi. Go có
  `bluemonday` trưởng thành, allowlist tường minh theo tag/attribute (câu 3). Đọc lại: parse HTML
  bằng chính TipTap ở chế độ readonly (`generateJSON`/schema parse) tự động bỏ node/attribute lạ —
  lớp phòng thủ kép tự nhiên (câu 4). Nhược điểm: HTML tự do hơn JSON nên cần sanitize nghiêm ở cả
  hai chiều (ghi và đọc).

**Khuyến nghị: HTML subset đã sanitize.** Khớp trực tiếp với `editor.getHTML()`/`generateHTML`, có
thư viện Go sẵn (`bluemonday`) để validate/sanitize ở server thay vì tự viết parser JSON, và giữ được
khả năng đổi editor sau này vì HTML là định dạng trung lập. Giữ cột DB là TEXT (không cần đổi kiểu),
tiếp tục dùng Gin binding kiểm tra độ dài — nhưng đếm theo **plain-text length** chứ không phải độ
dài chuỗi HTML thô (xem câu 3) để không phạt người dùng vì markup.

## 3. Sanitize phía Go: bluemonday

- Bản mới nhất: `v1.0.27` (release 2024-07-04, pkg.go.dev). Không có version mới hơn tính đến
  2026-09-17 — dự án bảo trì chậm (không có PR/issue mới đáng kể trong 12 tháng qua theo
  newreleases.io/progressiverobot.com), nhưng **Snyk xác nhận v1.0.27 không dính CVE nào đang mở**;
  2 lỗ hổng lịch sử (CVE-2021-29272 — bypass do lowercase ký tự Cyrillic viết hoa đánh lừa filter
  `script`; và một XSS qua thẻ `style`) đều đã vá trước v1.0.16/v1.0.5. Rủi ro bảo trì: nếu phát
  hiện bypass mới, thời gian vá có thể chậm — cần theo dõi repo định kỳ.
- Cơ chế: dùng token parser HTML chuẩn (`golang.org/x/net/html`), không dùng regex — giảm rủi ro
  bypass kiểu regex-based sanitizer.
- Tag không nằm trong allowlist: **mặc định bị strip nhưng giữ lại text bên trong** (xác nhận qua
  source `sanitize.go`: cờ `skipElementContent` chỉ bật cho `script`/`style`, các case token khác
  vẫn output text con). Ví dụ ví dụ chính thức: `<a onblur="alert(secret)" href="...">Google</a>` →
  `<a href="...">Google</a>` (attribute bị bỏ, text giữ). Riêng `<script>`/`<style>` bị xóa **cả nội
  dung** vì được coi là non-content data, không phải text hiển thị.
- `javascript:`/`data:` URL: không nằm trong danh sách scheme được `AllowStandardURLs()` cho phép
  (chỉ `http`, `https`, `mailto`) nên bị loại bỏ toàn bộ attribute `href` chứa nó. `data:` URI có
  API riêng `AllowDataURIImages()` — **không bật** vì task description không cần ảnh nhúng base64.
- Nested list (`ul > li > ul > li`): bluemonday sanitize theo cây token tuần tự, không có giới hạn
  độ sâu đặc biệt — `ul`/`ol`/`li` lồng nhau hoạt động bình thường miễn các tag đó có trong
  allowlist; không cần cấu hình thêm.
- `AddSpaceWhenStrippingTag(true)` (có trong API `policy.go`): nên bật để khi một tag bị strip
  (ví dụ `<div>A</div><div>B</div>` gõ dán từ Word), không bị dính liền thành "AB" mà thành "A B".

### Policy đề xuất (đúng yêu cầu: p, br, strong/b, em/i, u, s/strike, ul, ol, li, a; tuỳ chọn h2/h3, code)

```go
package sanitize

import "github.com/microcosm-cc/bluemonday"

// TaskDescriptionPolicy is the strict allowlist for rich-text task
// descriptions. Anything not listed here is stripped (tag removed, text
// content kept, except script/style whose content is dropped entirely).
func TaskDescriptionPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements("p", "br", "strong", "b", "em", "i", "u", "s", "strike")
	p.AllowElements("ul", "ol", "li")
	p.AllowElements("h2", "h3") // optional headings
	p.AllowElements("code")

	// Links: href only, restricted to http/https/mailto; force safe rel/target.
	p.AllowAttrs("href").OnElements("a")
	p.AllowStandardURLs() // parseable URLs, scheme in {http, https, mailto}
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true) // adds target="_blank" + rel="noopener"

	// Explicit rel hardening (belt-and-suspenders on top of the above helpers).
	p.AllowAttrs("rel").Matching(bluemonday.SpaceSeparatedTokens).OnElements("a")
	p.AllowAttrs("target").Matching(bluemonday.Paragraph /* placeholder */).OnElements("a")

	p.AddSpaceWhenStrippingTag(true)
	p.RequireParseableURLs(true)

	return p
}
```

Ghi chú triển khai: `AddTargetBlankToFullyQualifiedLinks` + `RequireNoFollowOnLinks` +
`RequireNoReferrerOnLinks` đã tự sinh `rel="nofollow noreferrer noopener"` và
`target="_blank"` cho link tuyệt đối (fully-qualified) — nên **không cần** dòng
`AllowAttrs("rel"/"target")` thủ công ở trên nếu dùng đúng 3 helper đó; dòng đó chỉ cần nếu
muốn tự kiểm soát giá trị chính xác thay vì để bluemonday tự sinh. Test lại giá trị `rel`/`target`
sinh ra bằng `go test` với vài input mẫu trước khi khóa cứng.

### Đếm plain-text length cho binding `max=4000`

```go
import (
	"html"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

var stripAll = bluemonday.StrictPolicy() // empty allowlist: strips every tag

func PlainTextLen(sanitizedHTML string) int {
	stripped := stripAll.Sanitize(sanitizedHTML) // "<p>Xin chào</p>" -> "Xin chào"
	unescaped := html.UnescapeString(stripped)    // "&amp;" -> "&"
	return utf8.RuneCountInString(unescaped)       // rune count, not byte length, for UTF-8/dấu tiếng Việt
}
```

Thứ tự đúng: sanitize theo `TaskDescriptionPolicy()` trước khi lưu (chặn XSS) → sau đó tính
`PlainTextLen` trên **HTML đã sanitize** (không phải input thô) để validate `max=4000`, tránh đếm
dư do markup hoặc payload rác đã bị loại.

## 4. Phòng thủ kép phía client: có cần DOMPurify không?

Có ba lớp khả dĩ: (1) bluemonday ở server khi ghi, (2) parser HTML→schema của chính editor khi đọc
readonly, (3) DOMPurify trước `dangerouslySetInnerHTML`.

- Lớp (2) — dùng TipTap ở `editable: false` để parse HTML sanitize từ server, rồi tự render qua
  React node (không qua `dangerouslySetInnerHTML`) — **đã là lớp phòng thủ thứ hai thực sự**, vì
  ProseMirror parse HTML theo schema đã khai báo (chỉ bold/italic/link/list...), bỏ hẳn node/attribute
  không khớp schema, không có "escape hatch" giữ nguyên HTML lạ. Đây khác về bản chất với
  `dangerouslySetInnerHTML` (chèn thẳng chuỗi HTML vào DOM).
- Nếu chọn đường vòng nhẹ hơn — không dựng cả editor instance ở readonly, mà chỉ
  `dangerouslySetInnerHTML={{ __html: sanitizedHTML }}` để tiết kiệm bundle cho trang chỉ-xem — thì
  **lúc đó bắt buộc phải thêm DOMPurify phía client**, vì `dangerouslySetInnerHTML` không có logic
  parse-theo-schema nào cả, nó tin tưởng tuyệt đối chuỗi truyền vào; nếu server sanitize có bug/bị
  bypass (đặc biệt do bluemonday bảo trì chậm — mục 3), DOMPurify là lớp chặn độc lập thứ hai với
  logic khác hẳn bluemonday (tránh cùng loại bug ở cả hai lớp).
- `dompurify` bản mới nhất `3.4.15`, ~11.1 KB gzip — rẻ, nên **luôn thêm** bất kể chọn cách nào ở
  trên, trừ khi read-only mode luôn luôn dựng full editor instance (không bao giờ dùng
  `dangerouslySetInnerHTML` trực tiếp).

**Khuyến nghị**: dùng chính TipTap readonly (`editable: false`, cùng schema với editor viết) để
render — tận dụng lazy-load sẵn có, tránh thêm dependency. **Vẫn thêm DOMPurify** như một guard rẻ
ngay trước khi set nội dung vào editor readonly (`DOMPurify.sanitize(html)` trước
`editor.commands.setContent(html)`), vì chi phí gần như bằng 0 (11 KB gzip) trong khi giảm hẳn rủi
ro nếu bluemonday có lỗ hổng chưa phát hiện — đúng tinh thần defense-in-depth, không tin một lớp duy
nhất khi cả hai lớp rẻ.

## 5. Lazy-load editor trong Vite/React (modal)

```tsx
const TaskDescriptionEditor = lazy(() =>
  import("./task-description-editor").then((m) => ({ default: m.TaskDescriptionEditor })),
);
// bên trong modal, chỉ mount khi modal mở:
<Suspense fallback={<textarea disabled />}>
  <TaskDescriptionEditor ... />
</Suspense>
```

Pitfall cần lưu ý (không có SSR/hydration trong app này nên nhóm rủi ro đó không áp dụng):
- **react-hook-form + Controller**: TipTap không phải input DOM chuẩn, phải bọc qua
  `Controller` và đồng bộ `onUpdate` → `field.onChange(editor.getHTML())`; tránh dùng
  `form.register("description")` như hiện tại vì không có `ref`/`onChange` chuẩn của input.
- **`useEditor` re-init**: mảng `extensions` phải ổn định (khai báo ngoài component hoặc
  `useMemo`) — nếu tạo mới mỗi render, TipTap sẽ huỷ/tạo lại editor liên tục, mất focus, đặc biệt
  nứt vỡ khi đang gõ tiếng Việt qua composition.
- **Suspense fallback nhấp nháy**: modal mở/đóng nhanh có thể trigger lazy-import lặp lại nếu
  component cha bị unmount hoàn toàn mỗi lần đóng modal — cân nhắc giữ `lazy()` ở module scope
  (ngoài component modal) để chunk chỉ fetch một lần rồi cache, không tạo lại reference `lazy()`
  mỗi lần modal mount.
- **Layout shift**: fallback nên có chiều cao tối thiểu giống textarea cũ để tránh nhảy layout khi
  chunk tải xong và editor thay thế fallback.
- Không có vấn đề SSR/hydration vì đây là SPA (Vite + React Router client-side).

## 6. Testing trong Vitest/jsdom

Vấn đề đã biết: ProseMirror/TipTap gọi `Range.prototype.getClientRects`/`getBoundingClientRect` và
`document.elementFromPoint` — **jsdom không implement các API này** (xác nhận qua jsdom issue #3002,
#3729 còn mở), nên các thao tác cần đo vị trí con trỏ (ví dụ toggleMark qua lệnh, kéo-thả, một số
scroll-into-view) ném `TypeError: ... is not a function` nếu không polyfill.

Cách xử lý được cộng đồng dùng (ví dụ package `jest-remirror`, tương tự dùng được cho Vitest):
polyfill `Range.prototype.getClientRects`/`getBoundingClientRect` và
`Document.prototype.elementFromPoint` trong file setup (`vitest.setup.ts`), trả về `DOMRect` giả.

Khuyến nghị chiến lược test, theo mức chi phí tăng dần:
1. **Test form/logic qua editor API, không mô phỏng gõ phím**: dùng `editor.commands.setContent(...)`,
   đọc `editor.getHTML()`/`editor.isEmpty`/`editor.getText()` trực tiếp trong test — tránh hoàn toàn
   nhu cầu polyfill DOM-geometry vì không đi qua `contenteditable`+`user-event`.
2. **Test component form (task-form-modal) với editor thật bị mock**: mock module editor thành một
   `<textarea data-testid="description-editor">` đơn giản implement cùng props interface
   (`value`, `onChange`) — giữ test nhanh, không phụ thuộc ProseMirror, đúng tinh thần "test hành vi
   form, không test lại TipTap".
3. **Chỉ khi thực sự cần test tương tác gõ/toolbar thật** (ít khuyến nghị cho unit test) mới thêm
   polyfill `getClientRects`/`elementFromPoint` vào setup và dùng `@testing-library/user-event` gõ
   trực tiếp vào `contenteditable` — chi phí bảo trì cao hơn, dễ vỡ khi TipTap đổi version.
- Ghi chú thêm: dự án WordPress Gutenberg đã bỏ hẳn polyfill jsdom-wide, chuyển sang Vitest Browser
  Mode (chạy trình duyệt thật) cho test cần hành vi render thật — có thể cân nhắc dài hạn nếu số
  lượng test tương tác tăng, nhưng không cần thiết ngay cho phạm vi 1 form nhỏ.

## 7. Migrate dữ liệu description dạng plain-text cũ sang HTML

Hai lựa chọn: (a) migration SQL một lần escape + wrap toàn bộ dữ liệu cũ; (b) phát hiện "còn là
plain text" tại thời điểm đọc rồi convert on-the-fly.

**Khuyến nghị: migration SQL một lần**, vì (i) sau migrate mọi row đều đồng nhất là "HTML đã
sanitize hợp lệ" — response API, editor readonly, validate độ dài đều xử lý một loại dữ liệu duy
nhất, không cần nhánh "is legacy plain text?" rải rác trong code; (ii) tránh nợ kỹ thuật vĩnh viễn
(convert on-read phải tồn tại mãi mãi vì không biết row nào đã convert); (iii) khối lượng dữ liệu
nhỏ (task description), migration một lần rẻ và an toàn hơn giữ logic kép lâu dài.

```sql
-- Escape HTML-significant characters and turn newlines into paragraph breaks.
-- Order matters: escape "&" first, then "<"/">" (otherwise the &amp; from
-- escaping "&" would itself get its "&" re-escaped).
UPDATE tasks
SET description =
    '<p>' ||
    regexp_replace(
        regexp_replace(
            regexp_replace(
                regexp_replace(description, '&', '&amp;', 'g'),
                '<', '&lt;', 'g'
            ),
            '>', '&gt;', 'g'
        ),
        E'\n', '</p><p>', 'g'
    ) ||
    '</p>'
WHERE description IS NOT NULL
  AND description <> ''
  AND description !~ '^\s*<'; -- skip rows already looking like HTML (idempotency guard for reruns)
```

Lưu ý khi chạy: (1) chạy trong transaction, backup trước; (2) chuỗi rỗng hoặc chỉ khoảng trắng nên
giữ nguyên rỗng thay vì bọc `<p></p>` (đã lọc qua điều kiện `description <> ''`, có thể cần thêm
`AND trim(description) <> ''`); (3) sau migration, chạy `PlainTextLen` (mục 3) trên toàn bộ dữ liệu
để xác nhận không có row nào vượt giới hạn 4000 sau khi wrap (wrap chỉ thêm markup, không đổi
plain-text length, nhưng nên xác nhận bằng script Go dùng chính `TaskDescriptionPolicy` + sanitize
lại toàn bộ để chắc chắn dữ liệu cũ (vốn chưa qua bluemonday) không chứa ký tự lạ bị đổi khi sanitize
lần đầu).

## Bảng khuyến nghị (pin version)

| Package | Version pin | Vai trò |
|---|---|---|
| `@tiptap/react` | `3.31.3` | React bindings, `useEditor`/`EditorContent` |
| `@tiptap/starter-kit` | `3.31.3` | bold/italic/underline/strike/heading/list/paragraph |
| `@tiptap/extension-link` | `3.31.3` | link với autolink/openOnClick |
| `@tiptap/pm` | `3.31.3` | ProseMirror core (peer dep bắt buộc) |
| `dompurify` | `3.4.15` | lớp phòng thủ thứ hai trước khi set HTML vào editor readonly |
| `github.com/microcosm-cc/bluemonday` | `v1.0.27` | allowlist HTML server-side, dựng ở mục 3 |

## Rủi ro

- `bluemonday` bảo trì chậm (không release >1 năm) — theo dõi repo định kỳ; nếu phát hiện bypass
  mới không được vá kịp, cân nhắc fork nội bộ hoặc chuyển sang thư viện khác (chưa có ứng viên Go
  nào trưởng thành hơn tại thời điểm nghiên cứu).
- Bundle ~110-130 KB gzip cho editor — chấp nhận được nhờ lazy-load, nhưng cần đo lại bundle thật
  của app (không chỉ số lý thuyết từ bundlephobia) sau khi tích hợp, vì tree-shaking thực tế phụ
  thuộc cấu hình Vite/esbuild của repo.
- Đổi `description` từ plain text sang HTML là thay đổi contract công khai của API (`TaskResponse.
  Description` giờ chứa markup) — cần cập nhật phía client/consumer khác nếu có, và tài liệu API
  nếu tồn tại.
- react-hook-form + Controller quanh TipTap là điểm tích hợp mới chưa có trong repo — nên viết test
  form-level (mục 6, cách 2) sớm để bắt regression khi TipTap update version.

## Unresolved questions

1. Chưa đo được bundle size chính xác của `@lexical/react` (bundlephobia rate-limit 429 khi thử
   lại nhiều lần) — không ảnh hưởng khuyến nghị (đã loại Lexical vì lý do API/composition, không
   phải vì bundle), nhưng nếu cần con số chính xác để so sánh kỹ hơn thì cần đo lại sau.
2. Chưa xác nhận từ TipTap docs chính thức cách cấu hình `openOnClick` động theo
   `editable`/readonly (WebFetch không tìm thấy đoạn tài liệu đó) — pattern
   `openOnClick: (view) => !view.editable` là suy luận hợp lý từ API sẵn có, nên xác minh bằng
   code thử nghiệm nhỏ trước khi implement chính thức.
3. Chưa kiểm tra tương thích `@tiptap/extension-character-count` với giới hạn 4000 ký tự tính trên
   HTML đã sanitize (đề xuất ở mục 3) — cần xác nhận `characterCount` đếm plain text hay đếm cả
   markup trước khi dùng nó thay cho hàm `PlainTextLen` phía Go làm nguồn sự thật duy nhất.
