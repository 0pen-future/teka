# Code review — Phase 5: Web rich-text editor cho mô tả công việc

- Plan: `plans/260917-1515-task-dnd-rich-text/phase-05-web-rich-text-editor.md`
- Diff: working tree chưa commit (6 file mới, 11 file sửa) trên `master`
- Reviewer: reviewer-phase5 — 2026-09-17 19:54

## Phạm vi đã đọc

Mới: `lib/rich-text.ts`, `components/rich-text-view.tsx`,
`components/task-description-editor.tsx`, `__tests__/rich-text.test.ts`,
`__tests__/task-description-editor.test.tsx`, `__tests__/task-form-modal.test.tsx`.
Sửa: `task-form-modal.tsx`, `task-card.tsx`, `task-schemas.ts`,
`__tests__/tasks-handlers.ts`, `__tests__/task-board-page.test.tsx`,
`src/test/setup.ts`, `src/test/msw/handlers.ts`, `src/styles/globals.css`,
`eslint.config.js`, `package.json`, `docs/frontend-guidelines.md`.
Đối chiếu thêm: `apps/api/internal/features/tasks/description.go` (parity bộ đếm),
`src/features/tasks/pages/task-board-page.tsx` (điểm mount modal),
`src/components/ui/field.tsx`, `lib/map-api-error.ts`, `e2e/`.

## Gate đã chạy

| Gate | Kết quả |
|------|---------|
| `npm run typecheck` | Xanh, không lỗi |
| `npm run lint` | 0 error, 6 warning (đều là `react-hooks/incompatible-library` do `form.watch`, có sẵn từ trước) |
| `npm run test` (toàn bộ 98 file) | Lần 2: **802 passed / 3 skipped, 0 failed** |
| `npm run build` | Thành công; chunk `task-description-editor-*.js` **396.29 kB / 125.38 kB gz** tách riêng, chunk `task-board-page-*.js` 129.20 kB / 42.80 kB gz |

Ghi chú về hai lần chạy test trước đó:

- Lần chạy đầu `rich-text.test.ts` đỏ 1 ca (`sanitizeDescription("<p>   </p>text")`
  kỳ vọng `"text"`). Ca này thuộc khối `describe("XSS edge cases")` do một agent
  khác (tester) **đang sửa đồng thời** trong working tree; đến lần chạy sau đã
  được chỉnh lại thành `"<p>   </p>text"` và xanh. Đây là trạng thái file đang
  thay đổi, không phải lỗi mã nguồn Phase 5.
- Lần chạy đầu `features/teaching/__tests__/classbook-page.test.tsx` đỏ 1 ca
  (`findByRole("dialog", { name: "Còn 1 ô…" })` timeout). Chạy riêng file đó:
  19/19 xanh; chạy lại toàn bộ suite: xanh. Kết luận: flake do tranh chấp tài
  nguyên khi chạy song song, **không** liên quan polyfill mới trong `setup.ts`
  (đã kiểm: `Range.prototype.getClientRects` và `Document.prototype.elementFromPoint`
  vốn `undefined` trong jsdom nên polyfill là bổ sung, không phải ghi đè).

## Đánh giá chung

Chất lượng cao, đúng kiến trúc đã chốt: `rich-text.ts` là một lớp thuần được test
kỹ, `RichTextView` là điểm duy nhất chạm `dangerouslySetInnerHTML` và có rule
ESLint canh, editor được lazy hoá thật sự (chunk riêng, modal chỉ mount khi có
`formTarget`), bộ đếm ký tự khớp cách API đếm. Không có lỗ hổng XSS nào tìm thấy
qua 19 payload đối kháng. Không có finding Critical/High. Các mục bên dưới là
Medium/Low, đều sửa cục bộ được và không chặn merge nếu controller chấp nhận.

## Những điểm đã xác minh là ổn

**Sanitizer (đã dò bằng dompurify 3.4.15 + jsdom thật, đúng `SANITIZE_CONFIG` và
hook trong mã nguồn):** 19 payload đều sạch —
`href="  javascript:"` (khoảng trắng đầu), `java<TAB>script:`, `java<LF>script:`,
`&#106;avascript:`, ký tự điều khiển `` đứng trước scheme, `data:`,
`vbscript:`, `xlink:href`, `iframe srcdoc`, mXSS `<form><math><mtext></form>…<mglyph><style>`,
mXSS `<noscript>`, mXSS comment cắt `</p>`, `<svg><p><style><img onerror>`,
`<template>`, `<a>` lồng nhau, `JaVaScRiPt:` hoa/thường, `//evil.com`,
`<a target rel>` không href. DOMPurify chuẩn hoá khoảng trắng/ký tự điều khiển
trong giá trị thuộc tính **trước** khi test `ALLOWED_URI_REGEXP`, nên regex
`^(?:https?:|mailto:)` không bị vượt bằng whitespace/tab/newline. Allowlist không
có `svg`/`math`/`template` nên không còn vector mXSS round-trip đã biết.
`ALLOW_DATA_ATTR: false` + `ALLOWED_ATTR` tối thiểu chặn cả DOM clobbering
(`id`/`name` không được phép).

**Instance riêng + hook một lần:** `DOMPurify(window)` khởi tạo lazy, hook
`afterSanitizeAttributes` chỉ đăng ký cùng lúc với instance nên không nhân đôi
khi HMR/test nạp lại. Nhánh `<a>` không có `href` bị gỡ `target`/`rel` — đúng.

**Parity bộ đếm với Go:** `plainTextLength` (DOMParser + `textContent`),
`characterCount` (`doc.textBetween(0, size, "", "")`) và
`descriptionText` phía API (`bluemonday.StrictPolicy` + `html.UnescapeString`)
cho cùng kết quả: không separator giữa block, `<br>` = 0 ký tự, entity được giải
mã. Kiểm thực tế: `<p>a<br>b</p><p>c</p>` → `"abc"` ở cả editor lẫn lib.

**Định dạng editor khớp allowlist server** (chạy TipTap 3.31.3 thật trong Node):
gạch ngang xuất `<s>`, gạch chân xuất `<u>` — đúng thẻ bluemonday cho phép.
Dán nội dung lạ được ánh xạ về schema: `<h1>`→`<p>`, `<img>`/`<span>`/`<script>`
bị bỏ, `<a href="javascript:">` còn lại text thuần, `<table>` mất ranh giới ô
(đúng như Risk Assessment đã ghi).

**Rỗng gửi `""`:** `editor.isEmpty` true cho cả `<p></p>` lẫn `<p>   </p>`
(ProseMirror gộp whitespace khi parse), nên form gửi `""` — test modal xác nhận.

**`useEditorState` không gây re-render vô hạn:** bản cài đặt dùng
`useSyncExternalStoreWithSelector` với `equalityFn` mặc định là `deepEqual`
(đã in mã hàm từ gói đã cài), nên selector trả object mới mỗi lần là an toàn.

**Tách chunk:** modal chỉ được mount khi `formTarget` khác `undefined`
(`task-board-page.tsx:228`), nên bảng không kích hoạt `import()` của editor cho
tới khi người dùng mở form. Build xác nhận chunk riêng.

**CSS `.prose-task` không bị purge:** biên dịch `globals.css` bằng
`@tailwindcss/cli` v4.3.3 ra 7 selector `.prose-task` trong output. Biến
`--mint-600`, `--ink-400` có thật trong `tokens/colors.css`.

**ESLint rule đúng thứ tự:** rule đặt ở block base `files: ["**/*.{ts,tsx}"]`,
override tắt cho `rich-text-view.tsx` đứng sau nên thắng. Selector chỉ khớp
`JSXAttribute` nên file `.ts` thuần không bị ảnh hưởng. Lint toàn repo 0 error →
không có chỗ nào khác đang dùng `dangerouslySetInnerHTML`.

**Không phá hợp đồng công khai:** `AppTask`, props `TaskCardBody`,
`TaskFormModalProps`, export schema đều giữ nguyên; `taskFormSchema.description`
nới từ 2000 → 20000 HTML + refine 2000 text (đúng D12, chỉ có modal dùng);
`applyKanbanFormError` vẫn map `fields.description` vào đúng field vì
`description` nằm trong `form.getValues()`. `e2e/` không tham chiếu ô mô tả nên
không có e2e nào gãy.

**Không có vấn đề authz/dữ liệu:** logic `readOnly` không đổi; không thêm lời gọi
API; không có vòng lặp gọi mạng; preview thẻ dùng `useMemo` theo
`task.description` nên `DOMParser` chỉ chạy lại khi mô tả đổi.

## Findings

### M1 — Medium: modal read-only bỏ qua `normalizeIncoming`

`apps/web/src/features/tasks/components/task-form-modal.tsx:253`

Nhánh sửa dùng `normalizeIncoming(task.description)` (dòng 83) nhưng nhánh
read-only truyền thẳng `html={task.description}`. Với hàng chưa chạy migration
000023 (dev DB, hoặc client cũ ghi trước khi API Phase 2 deploy), hai nhánh hiển
thị khác nhau:

- `"Hỏi lớp 6A & 7B\nbáo lại trước thứ 6"` → nhánh sửa hiện 2 dòng, nhánh
  read-only mất xuống dòng (newline gộp thành khoảng trắng).
- Text thuần chứa `<b>quan trọng</b>` → nhánh sửa hiện đúng chữ đã escape, nhánh
  read-only bị DOMPurify coi là markup và bỏ thẻ.

Fixture `assignedToOthersTaskId` là đúng ca này nhưng test chỉ mở nó ở chế độ sửa,
nên không lộ ra.

Sửa đề xuất: `html={normalizeIncoming(task.description)}` (hoặc gọi
`normalizeIncoming` ngay trong `RichTextView` thay cho `sanitizeDescription` để
mọi nơi render đi cùng một đường).

### M2 — Medium: AC11 "task-board-page.test.tsx không nạp @tiptap/*" chưa đạt

`apps/web/src/features/tasks/__tests__/task-board-page.test.tsx:89` và `:273`

Hai ca này mở dialog "Tạo công việc" và dialog sửa ở chế độ ghi, nên
`Controller` + `Suspense` kích hoạt `lazy(() => import("./task-description-editor"))`
→ file test này vẫn nạp toàn bộ TipTap/ProseMirror. Phần quan trọng của AC11
(tách chunk runtime) đã đạt và đã được build xác nhận; chỉ mệnh đề về file test
là không đúng thực tế.

Sửa đề xuất — chọn một:
1. Cập nhật AC11 trong `plan.md`/phase file cho khớp (khuyến nghị: rẻ, không đổi
   hành vi), hoặc
2. `vi.mock("../components/task-description-editor", …)` trong
   `task-board-page.test.tsx` để file này giữ đúng cam kết không nạp editor.

### M3 — Medium: thiếu test cho ánh xạ paste/schema mà phase file yêu cầu

Phase file mục Requirements → "Paste: … kiểm bằng test `editor.commands.insertContent`
với HTML lạ → `getHTML()` không chứa thẻ ngoài allowlist" và Related Code Files →
`task-description-editor.test.tsx` ("insert HTML có `<h1>`/`<img>` → không còn
trong `getHTML()`"). Trong `task-description-editor.test.tsx` hiện không có ca này.

Tôi đã tự kiểm bằng cách dựng editor thật trong Node với đúng cấu hình extension:
`<h1>tieu de</h1><img><table>…<span style>…<script>…<a href="javascript:">` →
`<p>tieu de</p><p>ab</p><p>okspan</p><p>bad</p>`. Hành vi đúng, nhưng repo chưa
có test nào khoá lại hành vi đó (nâng cấp TipTap sau này có thể đổi mà không ai
biết).

Sửa đề xuất: thêm một ca dùng `editor.commands.insertContent` qua harness hoặc
`user.paste()` và assert `getHTML()` chỉ còn thẻ trong allowlist.

### M4 — Medium: `aria-live="polite"` trên bộ đếm đọc lại mỗi lần gõ phím

`apps/web/src/features/tasks/components/task-description-editor.tsx:263-271`

Nội dung `{count}/2000` đổi sau **mỗi** ký tự, nên trình đọc màn hình sẽ đọc
"1/2000", "2/2000", … liên tục khi người dùng gõ, che mất nội dung đang soạn.
Đây là mẫu phản tác dụng phổ biến với live region.

Sửa đề xuất: bỏ `aria-live` (số vẫn đọc được khi người dùng điều hướng tới), hoặc
chỉ bật thông báo khi vượt ngưỡng, ví dụ render thêm một `role="status"` ẩn chỉ
xuất hiện lúc `count > DESCRIPTION_MAX_CHARS`.

### L1 — Low: `protocols: ["http","https","mailto"]` vô hiệu và gây 3 warning console

`apps/web/src/features/tasks/components/task-description-editor.tsx:90`

Chạy editor thật cho thấy lần autolink đầu tiên in ra 3 dòng:
`linkifyjs: already initialized - will not register custom scheme "http" …`
(tương tự cho `https`, `mailto`). Lý do: linkify đã init trước khi TipTap gọi
`registerCustomProtocol`. Ba scheme này vốn là scheme mặc định của linkify nên
autolink vẫn chạy đúng (`https://vnexpress.net` vẫn thành `<a>`), tức option chỉ
để lại tiếng ồn trong console production.

Sửa đề xuất: bỏ `protocols` (giữ `defaultProtocol: "https"`); giới hạn scheme
thật sự nằm ở `LINK_PATTERN` phía panel và ở bluemonday phía server.

### L2 — Low: Escape trong ô nhập liên kết làm mất focus

`apps/web/src/features/tasks/components/task-description-editor.tsx:240-243`

Nhấn Escape đóng panel (đúng, và `preventDefault` giữ modal không bị đóng theo)
nhưng ô input bị unmount trong khi focus đang ở đó → focus rơi về `<body>`,
người dùng bàn phím phải Tab lại từ đầu tài liệu. Nhánh `applyLink` thành công
thì không bị vì có `.focus()` trong chain.

Sửa đề xuất: gọi `editor.commands.focus()` trước/sau `setLinkPanel(null)` ở nhánh
Escape (và cả nhánh bấm lại nút "Liên kết" để đóng, dòng 221). Tiện thể: panel mở
ra nhưng không tự focus vào ô URL — thêm `autoFocus` sẽ giảm một bước Tab.

### L3 — Low: toolbar chưa có roving tabindex

`apps/web/src/features/tasks/components/task-description-editor.tsx:279-305`

`role="toolbar"` theo WAI-ARIA kỳ vọng chỉ một nút nằm trong tab order, các nút
còn lại chuyển bằng mũi tên. Hiện cả 7 nút đều `tabIndex` mặc định nên người dùng
bàn phím phải Tab 7 lần mới qua được toolbar. Ngoài ra handler
`event.preventDefault()` chạy cho ArrowLeft/ArrowRight ngay cả khi focus không ở
nút nào trong toolbar (dòng 283-291), nuốt phím mũi tên trong trường hợp đó.

Sửa đề xuất: thêm `tabIndex={pressed || isFirst ? 0 : -1}` (state focus index) và
chỉ `preventDefault()` sau khi xác định `index !== -1`.

### L4 — Low: `Element.prototype.getClientRects` bị ghi đè vô điều kiện

`apps/web/src/test/setup.ts:69`

Phase file quy định `Element.prototype.getClientRects ??= emptyRects`. Bản cài đặt
gán đè, thay thế luôn implementation có sẵn của jsdom cho toàn bộ 98 file test
(`Range.prototype.getClientRects` và `Document.prototype.elementFromPoint` thì
đúng là `undefined` trong jsdom nên cần polyfill). Hiện chưa gây hỏng test nào,
nhưng object giả chỉ có `length`/`item`/iterator; component nào dùng API khác của
`DOMRectList` sẽ nhận hành vi lạ.

Sửa đề xuất: dùng `??=` như plan.

### L5 — Low: mở rồi Lưu một việc không sửa gì vẫn ghi lại mô tả khác đi

`apps/web/src/features/tasks/components/task-form-modal.tsx:83`

`normalizeIncoming` chạy DOMPurify với hook ép `rel="noopener noreferrer nofollow"`
+ `target="_blank"`, trong khi bluemonday phía server ghi `rel="nofollow noreferrer"`
và chỉ thêm `target` cho link fully-qualified (link `mailto:` không được thêm).
Hệ quả: với việc có chứa link, `defaultValues.description` khác chuỗi đang lưu,
nên bấm "Lưu" mà không sửa gì vẫn PATCH một mô tả khác (chỉ khác thứ tự `rel`/có
thêm `target`). Không mất dữ liệu, không ảnh hưởng hiển thị; chỉ là ghi thừa.

Sửa đề xuất: chấp nhận và ghi chú, hoặc cho hook web dùng đúng thứ tự/điều kiện
của bluemonday nếu muốn hai phía hội tụ.

### L6 — Low (thông tin): `z.string().max(20000)` đếm UTF-16, API đếm rune

`apps/web/src/features/tasks/schemas/task-schemas.ts:116-119`

Với mô tả nhiều emoji/ký tự ngoài BMP, web sẽ báo lỗi sớm hơn server một chút.
Hướng chặt hơn server là an toàn, không cần sửa; nêu để khỏi bị coi là bug sau này.

### L7 — Low (thông tin): khối `describe("XSS edge cases")` mới thêm vào `rich-text.test.ts`

`apps/web/src/features/tasks/__tests__/rich-text.test.ts:91-201`

Khối này do tester thêm song song trong lúc review. Nhận xét:

- Trùng lặp với các ca đã có ở trên trong cùng file: `javascript:` href, link
  tương đối, `<img onerror>`, `<script>`, giải mã entity trong `plainTextLength`.
- Một số assertion bị nới cho khớp output thay vì diễn đạt ý định:
  `toContain("<li>one")`, `toContain("bold")`, `toBeGreaterThan(1)` cho emoji ZWJ
  (giá trị đúng là 7 code point, và Go cũng đếm 7 rune — nên assert số chính xác
  được).
- Giá trị thật nằm ở các ca mới: svg/math, `<iframe>`, `<style>`, link
  protocol-relative, HTML lỗi cú pháp. Nên giữ các ca này và bỏ phần trùng.

Không chặn; controller quyết định cắt gọn hay giữ nguyên.

## Success Criteria của phase

| Tiêu chí | Trạng thái |
|----------|-----------|
| AC10 — toolbar đủ 7 chức năng, lưu/mở lại giữ định dạng, rỗng gửi `""` | Đạt (test modal + editor; kiểm thêm `<s>`/`<u>` khớp allowlist server) |
| AC11 — test XSS xanh, `dangerouslySetInnerHTML` chỉ ở `rich-text-view.tsx` | Đạt (rule ESLint + lint 0 error) |
| AC11 — `task-board-page.test.tsx` không nạp `@tiptap/*` | **Chưa đạt** (M2) |
| Chunk editor lazy, bảng không tải TipTap khi chưa mở modal | Đạt (build + điểm mount) |
| A11y: `role="toolbar"`, nút có tên, textbox tên "Mô tả", lỗi gắn `aria-describedby` | Đạt phần bắt buộc; còn M4, L2, L3 |
| Toàn bộ test web xanh trong jsdom với polyfill | Đạt (802 passed) |
| Kiểm tay: Telex/composition trên Chrome + Android, dán từ Google Docs | **Chưa làm** (Implementation Step 7, cần người thao tác thật) |

## Khuyến nghị theo thứ tự

1. M1 — dùng `normalizeIncoming` cho nhánh read-only (1 dòng), thêm một ca test
   mở read-only trên fixture mô tả text thuần.
2. M2 — chốt cách xử lý AC11: sửa plan hoặc `vi.mock` editor trong test bảng.
3. M3 — thêm test paste/`insertContent` khoá hành vi ánh xạ schema.
4. M4 — bỏ `aria-live` trên bộ đếm hoặc chỉ thông báo khi vượt ngưỡng.
5. L1, L2 — bỏ `protocols`, trả focus về editor khi đóng panel liên kết.
6. L3, L4, L5, L6, L7 — tuỳ chọn, gom vào lần dọn sau hoặc ghi vào phase 6.
7. Trước khi ship: chạy kiểm tay bước 7 của phase (gõ Telex, dán từ Google Docs,
   thử `javascript:` ở ô URL) — không thể thay bằng test tự động.

## Unresolved questions

- Thứ tự `rel` và điều kiện `target` giữa web (DOMPurify) và server (bluemonday)
  hiện lệch nhau (L5). Có muốn hai phía hội tụ đúng từng ký tự không, hay chấp
  nhận server là nguồn sự thật và web chỉ cần "không kém an toàn hơn"?
- AC11 mệnh đề về file test: sửa plan hay sửa test? (M2)

Status: DONE_WITH_CONCERNS
Summary: Phase 5 đúng kiến trúc, không có lỗ hổng XSS nào qua 19 payload đối kháng,
typecheck/lint/build/test đều xanh và chunk editor tách riêng đúng như AC; còn 4
finding Medium (read-only bỏ qua `normalizeIncoming`, AC11 về file test bảng,
thiếu test paste, bộ đếm `aria-live` đọc mỗi phím) và 7 finding Low.
Concerns/Blockers: Không có blocker. Cần controller quyết M1–M4 và chốt cách xử lý
mệnh đề AC11; bước kiểm tay (Telex/composition, dán từ Google Docs) vẫn chưa ai làm.
