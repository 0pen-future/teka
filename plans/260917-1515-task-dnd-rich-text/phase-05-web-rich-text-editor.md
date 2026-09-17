---
phase: 5
title: "Web: editor TipTap cho mô tả + RichTextView an toàn"
status: completed
priority: P1
effort: "1.5d"
dependencies: [2]
---

# Phase 5: Web — editor TipTap cho mô tả + `RichTextView` an toàn

## Overview

Thay `<textarea>` mô tả trong `TaskFormModal` bằng editor TipTap 3 (lazy) với
toolbar tối thiểu khớp allowlist server, lưu HTML; hiển thị mô tả qua
`RichTextView` (DOMPurify + `dangerouslySetInnerHTML`). Validation zod đếm văn
bản thuần ≤ 2000 và HTML ≤ 20000.

## Requirements

- Functional:
  - Toolbar: Đậm, Nghiêng, Gạch chân, Gạch ngang, Danh sách chấm, Danh sách số, Liên kết (ô nhập URL inline dưới toolbar; chỉ `http/https/mailto`; nút bỏ liên kết). Phím tắt mặc định của TipTap (Ctrl+B/I/U, Ctrl+Shift+8/7 list).
  - Extensions: `StarterKit` với `heading:false, blockquote:false, codeBlock:false, code:false, horizontalRule:false, dropcursor:false, gapcursor:false`; starter-kit 3.x đã gồm `link`, `underline`, `strike` — cấu hình `link: { openOnClick:false, autolink:true, protocols:["http","https","mailto"], defaultProtocol:"https" }` (kiểm bằng spike 15 phút rằng click vào link trong editor không mở tab); `CharacterCount` import từ `@tiptap/extensions` (gói `@tiptap/extension-character-count` chỉ là shim deprecated) — **chỉ để hiển thị** `k/2000`, nguồn sự thật là `plainTextLength` (zod) và Go.
  - Trạng thái toolbar: TipTap v3 `useEditor` mặc định **không re-render theo transaction** → `aria-pressed={editor.isActive("bold")}` sẽ hiển thị trạng thái cũ. Dùng `useEditorState({ editor, selector: ctx => ({ bold: ctx.editor.isActive("bold"), italic: …, underline: …, strike: …, bulletList: …, orderedList: …, link: … }) })` cho toolbar; test assert sau khi bấm Đậm nút có `aria-pressed="true"`.
  - Paste: TipTap ánh xạ HTML dán vào theo schema — heading thành paragraph, ảnh bị bỏ; kiểm bằng test `editor.commands.insertContent` với HTML lạ → `getHTML()` không chứa thẻ ngoài allowlist.
  - Giá trị form `description`: `editor.isEmpty ? "" : editor.getHTML()`; RHF `Controller` + `field.onChange` trong `onUpdate`.
  - Edit task cũ: `toFormValues` đặt `defaultValues.description = normalizeIncoming(task.description)` (không chỉ ở `content` của editor — nếu người dùng không chạm editor, giá trị submit vẫn phải là HTML). `normalizeIncoming`: chuỗi không bắt đầu bằng `<` → escape + `<br>` + bọc `<p>` tương đương migration.
  - `RichTextView` (`components/rich-text-view.tsx`): `DOMPurify.sanitize(html, { ALLOWED_TAGS: [p, br, strong, em, u, s, ul, ol, li, a], ALLOWED_ATTR: [href, rel, target], ALLOWED_URI_REGEXP: /^(https?:|mailto:)/i })`, hook `afterSanitizeAttributes` (đăng ký **một lần ở module scope**, kiểm `node.tagName === "A"`; không đăng ký trong render để HMR/test không nhân đôi hook) ép `rel="noopener noreferrer nofollow"` + `target="_blank"`; render trong `div.prose-task` (class thủ công cho `ul/ol/a/p`, không kéo `@tailwindcss/typography`).
  - Card: hiển thị 2 dòng văn bản thuần rút gọn (`textFromHtml` = `DOMParser` → `textContent`, `useMemo` theo `task.description`) — không render HTML trên card; modal render editor.
  - Zod: `description: z.string().max(20000, "Mô tả quá dài, hãy bỏ bớt định dạng").refine(v => plainTextLength(v) <= 2000, "Mô tả tối đa 2000 ký tự")`; `plainTextLength` dùng `DOMParser`.
  - Editor lazy: `const TaskDescriptionEditor = lazy(() => import("./task-description-editor"))`; `Suspense` fallback là khung cùng chiều cao để không nhảy layout.
  - A11y: `EditorContent` nhận `aria-labelledby` trỏ label "Mô tả", `aria-invalid`, `aria-describedby` tới lỗi; toolbar `role="toolbar"` + `aria-pressed` cho nút đang active; nút có `aria-label` tiếng Việt.
- Non-functional:
  - Packages: `@tiptap/react@^3.31.3`, `@tiptap/starter-kit@^3.31.3`, `@tiptap/pm@^3.31.3`, `@tiptap/extensions@^3.31.3` (khai báo trực tiếp để import `CharacterCount`), `dompurify@^3.4.15` (có types sẵn). Không cài `@tiptap/extension-link` riêng (báo cáo researcher về điểm này lỗi thời với v3: starter-kit đã gồm link/underline/strike/list).
  - Chunk editor tách riêng (~110–130 KB gz) chỉ tải khi mở modal.
  - jsdom polyfill cho ProseMirror trong `src/test/setup.ts`: `Range.prototype.getClientRects`/`getBoundingClientRect`, `Element.prototype.getClientRects`.

## Architecture

```
TaskFormModal
 ├─ <Controller name="description" render={({field}) =>
 │     <Suspense fallback={<EditorSkeleton/>}>
 │        <TaskDescriptionEditor value={field.value} onChange={field.onChange} invalid describedBy />
 │     </Suspense>}
 └─ zod: taskFormSchema.description = html string, refine plainTextLength ≤ 2000

TaskDescriptionEditor (lazy chunk)
 ├─ useEditor({ extensions, content: normalizeIncoming(value), onUpdate: ({editor}) => onChange(editor.isEmpty ? "" : editor.getHTML()) })
 ├─ <EditorToolbar editor />  (role=toolbar; bold/italic/underline/strike/bullet/ordered/link)
 └─ <EditorContent editor />  + <span aria-live="polite">{count}/2000</span>

RichTextView ── DOMPurify(html, config) ── <div className="prose-task" dangerouslySetInnerHTML/>
TaskCard     ── textFromHtml(description) → 2 dòng (line-clamp-2)
```

Lớp phòng thủ: server bluemonday (phase 2) là ranh giới tin cậy; DOMPurify là lớp độc lập thứ hai cho `dangerouslySetInnerHTML`. Editor không phải lớp bảo mật.

## Related Code Files

- Modify: `apps/web/package.json` (5 package trên)
- Create: `apps/web/src/features/tasks/components/task-description-editor.tsx` (lazy chunk: `useEditor` + `useEditorState`, toolbar, ô nhập URL **inline** hiện dưới toolbar khi bấm Liên kết — repo chưa có Popover wrapper trong `components/hv`/`components/ui`, không thêm primitive mới cho một chỗ dùng)
- Create: `apps/web/src/features/tasks/components/rich-text-view.tsx`
- Create: `apps/web/src/features/tasks/lib/rich-text.ts` (`plainTextLength`, `textFromHtml`, `normalizeIncoming`, `SANITIZE_CONFIG`, `sanitizeDescription`)
- Modify: `apps/web/src/features/tasks/components/task-form-modal.tsx:229-244` (Controller + Suspense thay `textarea`)
- Modify: `apps/web/src/features/tasks/components/task-card.tsx` (mô tả rút gọn dùng `textFromHtml`)
- Modify: `apps/web/src/features/tasks/schemas/task-schemas.ts:104` (schema mới)
- Modify: `apps/web/src/styles/globals.css` (CSS toàn cục thật, import ở `src/app/main.tsx`; class `.prose-task` với `@apply`: `ul` disc, `ol` decimal, `a` underline + màu brand, `p` margin)
- Modify: `apps/web/src/test/setup.ts` (polyfill Range/ClientRects, comment nêu ProseMirror cần)
- Create: `apps/web/src/features/tasks/__tests__/rich-text.test.ts` (sanitize: script/onclick/javascript:/img/style bị bỏ, link được ép rel/target, `plainTextLength` với entity + tiếng Việt, `normalizeIncoming` với chuỗi thuần)
- Create: `apps/web/src/features/tasks/__tests__/task-description-editor.test.tsx` (render editor; gõ chữ → `onChange` HTML; nút Đậm → `<strong>` **và** `aria-pressed="true"`; insert HTML có `<h1>`/`<img>` → không còn trong `getHTML()`; rỗng → `""`)
- Create: `apps/web/src/features/tasks/__tests__/task-form-modal.test.tsx` (mở modal tạo → chờ editor lazy (`findByRole("textbox", { name: "Mô tả" })`) → nhập → submit → body `description` là `<p>…</p>`; sửa task có HTML → editor hiện nội dung; sửa task mô tả thuần chưa migrate → submit không chạm editor vẫn gửi HTML; quá 2000 chữ → lỗi zod hiển thị). Tách khỏi `task-board-page.test.tsx` để file test trang bảng không nạp `@tiptap/*`.
- Modify: `apps/web/src/test/msw/handlers.ts` (fixture task có `description` HTML)
- Modify: `docs/frontend-guidelines.md` (mục "Rich text: TipTap lazy trong modal; luôn qua `RichTextView`; không `dangerouslySetInnerHTML` ở nơi khác")

## Implementation Steps

1. Cài package; `npm run build` để xác nhận chunk editor tách riêng (có file chunk mang tên `task-description-editor` trong thư mục build).
2. Viết `lib/rich-text.ts` + test (chạy trước, không cần editor).
3. `setup.ts` polyfill:
   ```ts
   // ProseMirror measures selection rectangles; jsdom has no layout engine.
   const emptyRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
   Range.prototype.getClientRects = emptyRects;
   Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0);
   Element.prototype.getClientRects ??= emptyRects;
   ```
4. Viết `task-description-editor.tsx` (extensions như trên; toolbar dùng `HvButton` size nhỏ, `aria-pressed` lấy từ `useEditorState`; ô URL inline: input + "Áp dụng"/"Bỏ liên kết", validate scheme trước `setLink`), test editor.
5. Viết `rich-text-view.tsx` + css `.prose-task`; sửa `task-card.tsx` dùng `textFromHtml`.
6. Sửa `task-form-modal.tsx`: `Controller` + `Suspense`; schema zod; cập nhật test page (gõ bằng `userEvent.type`; nếu ProseMirror không nhận từ user-event, dùng `fireEvent.input` hoặc `editor.commands.insertContent` qua ref test).
7. Kiểm tay: gõ tiếng Việt Telex trên Chrome + Android (composition), dán từ Google Docs (heading → paragraph), link `javascript:` bị từ chối ở ô nhập URL.
8. Gate: `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build`.

## Success Criteria

- [x] AC10: tạo/sửa việc với đậm/nghiêng/gạch chân/gạch ngang/list/link → API nhận HTML subset; mở lại thấy đúng định dạng; mô tả rỗng gửi `""`.
- [x] AC11: `rich-text.test.ts` XSS xanh; `dangerouslySetInnerHTML` chỉ xuất hiện trong `rich-text-view.tsx` (thêm test grep hoặc rule eslint `react/no-danger` với override cho file đó).
- [x] Editor chunk lazy tách riêng; trang bảng không tải TipTap khi chưa mở modal.
- [x] A11y: toolbar `role="toolbar"`, nút có tên; editor `textbox` có tên "Mô tả"; lỗi zod gắn `aria-describedby`.
- [x] Toàn bộ test web xanh trong jsdom với polyfill.

## Execution Notes

Điểm lệch so với thiết kế ở trên, đã xác minh bằng test/build (2026-09-17):

- Không cài `@tiptap/extensions`/`CharacterCount`: bộ đếm lấy `doc.textBetween(0, size, "", "")` đếm code point, khớp cách Go đếm rune trên text đã bỏ thẻ; bỏ được một gói và một nguồn lệch số.
- Bỏ `protocols: ["http","https","mailto"]` khỏi cấu hình link: ba scheme này là mặc định của linkify, khai báo lại chỉ in cảnh báo `linkifyjs: already initialized` ra console. Giới hạn scheme thật nằm ở panel URL (`LINK_PATTERN`), DOMPurify và bluemonday.
- Hook DOMPurify ghi `rel`/`target` đúng thứ tự và điều kiện của bluemonday (`nofollow noreferrer noopener` + `target="_blank"` cho link http(s); `nofollow noreferrer` cho mailto) để mở-rồi-Lưu không gửi PATCH mô tả khác đi.
- Polyfill jsdom chỉ gán khi thiếu (`"getClientRects" in Range.prototype`, không dùng `??=` vì rule `unbound-method`); `Element.prototype.getClientRects` jsdom đã có nên không đè.
- `task-board-page.test.tsx` `vi.mock` editor bằng textarea để file test trang bảng không nạp `@tiptap/*`; editor thật kiểm ở `task-description-editor.test.tsx` và `task-form-modal.test.tsx`.
- Bộ đếm không còn `aria-live` (đọc lại mỗi phím gõ); thay bằng `role="status"` ẩn chỉ có nội dung khi vượt 2000.
- Chưa kiểm tay bước 7 (Telex trên Chrome/Android, dán từ Google Docs) — cần làm trước khi ship Phase 6.

## Risk Assessment

- **ProseMirror trong jsdom** vẫn có thể ném ở API layout khác → tín hiệu: stack trace ProseMirror `domObserver`/`posAtCoords`; ứng phó: polyfill đúng prototype thiếu, không mock toàn bộ editor. Nếu quá 2h → test editor qua `editor.commands` (không render) + kiểm gesture ở Playwright.
- **`userEvent.type` không vào ProseMirror**: dùng `fireEvent.input`/`insertContent` trong test; hành vi thật kiểm ở e2e.
- **Dán nội dung lớn** (>20000 ký tự HTML nhưng <2000 chữ do thẻ lồng): zod báo "Mô tả quá dài, hãy bỏ bớt định dạng"; hiếm.
- **Tailwind v4 không có plugin typography** → `.prose-task` thủ công, ~10 dòng.
- **Dữ liệu dev chưa migrate** → `normalizeIncoming` che (cả `defaultValues` lẫn `content`); không dùng làm lý do bỏ migration.
- **`CharacterCount` đếm khác `plainTextLength`** (có thể đếm cả markup/entity) → chỉ là gợi ý hiển thị; lỗi thật do zod/Go quyết định. Nếu lệch gây khó hiểu, thay số hiển thị bằng `plainTextLength(editor.getHTML())`.
- **Server không cân bằng thẻ** (bluemonday giữ nguyên `<p><strong>a` chưa đóng; review Phase 2 L4): `RichTextView` phải render trong một container riêng qua DOMPurify, không nối chuỗi mô tả vào markup khác. DOMPurify sẽ tự đóng thẻ khi parse.
- **Bảng dán từ Word/Sheets mất ranh giới ô** (`<table>` ngoài allowlist → text nối liền "ab"; review Phase 2 L6): editor TipTap không có extension table nên paste đã ép về paragraph; nếu người dùng phàn nàn, cân nhắc mở rộng allowlist ở cả Go lẫn DOMPurify, không sửa riêng một phía.
- **Schema cũ cap 2000 ký tự HTML** (`task-schemas.ts:104`; review Phase 2 M2): sau migration 000023 một mô tả thuần ~1995 ký tự thành HTML dài hơn 2000, nên schema mới (đếm text ≤ 2000, HTML ≤ 20000) phải deploy cùng API ở Phase 6, không tách rời.
