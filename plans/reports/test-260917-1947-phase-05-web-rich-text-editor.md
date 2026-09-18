# Báo cáo QA Phase 5: Web Rich-Text Editor cho Mô Tả Công Việc

**Ngày kiểm tra:** 2026-09-17  
**Thời gian:** 19:47  
**Tester:** QA Lead (Tester Bot)  
**Scope:** Phase 5 - Web rich-text editor TipTap + RichTextView an toàn

---

## 1. Kết Quả Gate (Build & Quality Checks)

### 1.1 ESLint
**Status:** ✅ PASS  
**Chi tiết:**
- Errors: 0
- Warnings: 6 (toàn bộ cũ từ React Compiler về `form.watch()`, không liên quan Phase 5)
- Không phát hiện lỗi mới từ phase này

### 1.2 TypeScript Typecheck
**Status:** ✅ PASS  
- Không có lỗi type
- Strict mode: bật

### 1.3 Test Suite
**Status:** ✅ PASS  
- Test Files: 98 passed
- Total Tests: 802 passed, 3 skipped (tăng từ 791 → 802, thêm 11 test edge case mới)
- Duration: 24.54s
- Không có test fail

### 1.4 Production Build
**Status:** ✅ PASS  
- Build success với chunk editor tách riêng: `dist/assets/task-description-editor-BtPEJX9n.js` (396.29 KB raw / 125.38 KB gzip)
- Lazy loading TipTap hoạt động đúng: chunk chỉ tải khi mở modal form
- Không có warning hoặc error

### 1.5 Test Coverage
**Status:** ✅ PASS  
- **Tổng số:** Statements 87.9% | Branches 80.89% | Functions 85.58% | Lines 88.37%
- **File `rich-text.ts`:** 100% line coverage (core utility library)
- **File `rich-text-view.tsx`:** 80% line coverage (RichTextView component)
- **File `task-description-editor.tsx`:** 79.74% line coverage

---

## 2. Độ Phủ Test Chi Tiết

### 2.1 `rich-text.test.ts` — 22 test (✅ all passed)

**Baseline tests:**
- `sanitizeDescription()`: 5 test ✅
  - Giữ lại subset định dạng cho phép (p, br, strong, em, u, s, ul, ol, li, a)
  - Xóa script, onclick, style, img — giữ text
  - Xóa javascript:/data:/relative link — ép rel/target trên http(s)/mailto
  - Xóa attr ngoài allowlist
  - Collapse whitespace-only thành ""

- `normalizeIncoming()`: 4 test ✅
  - Bọc plain text thành paragraph với escaped entity + <br> cho newline
  - Tag dẫn đầu được sanitize
  - "<3 phút" được xử lý như plain text
  - Blank input → ""

- `plainTextLength()`: 2 test ✅
  - Đếm code point của text (decode entity, bỏ tag)
  - Emoji emoji đơn: 3 emoji = 3 code point

- `textFromHtml()`: 1 test ✅
  - Flatten block/break thành single space cho preview

**Edge case tests (thêm mới):**
- `javascript:` protocol: 3 variant (lowercase, uppercase, whitespace) ✅
- SVG + event handler: 3 ca test ✅
- iframe/style/script removal: ✅
- Nested tag handling: nested list + mixed formatting ✅
- Empty/whitespace tags: ✅
- Entity decoding: `&lt;`, `&nbsp;`, `&amp;nbsp;` ✅
- Emoji: emoji đơn (3) + ZWJ sequence (>1 code point) ✅
- mailto: validation (accept valid, accept any format) ✅
- Relative/protocol-relative link: reject ✅
- normalizeIncoming: entity escaping + CRLF/LF ✅
- Malformed HTML: unclosed tag, badly nested ✅

**Coverage:** Hàm cốt lõi `sanitizeDescription`, `plainTextLength`, `textFromHtml`, `normalizeIncoming` được cover tổng cộng 22 case, bao gồm happy path + edge case XSS.

### 2.2 `task-description-editor.test.tsx` — 7 test (✅ all passed)

**Test list:**
1. A11y: textbox `aria-multiline`, `aria-invalid`, `aria-describedby`, toolbar role + button labels ✅
2. Seed từ `value` + toolbar pressed state phản ánh ✅
3. Type text → emit HTML + clear → emit "" ✅
4. Bold formatting + aria-pressed="true" ✅
5. Link URL panel: reject javascript:, accept https: (kiểm Edge case) ✅
6. External value change không echo ✅
7. Arrow key navigation: ArrowLeft/Right giữa toolbar button ✅

**Coverage:** Editor component, toolbar button state (bold/italic/underline/strike/list/link), link panel validation, accessibility attributes đều được kiểm tra.

### 2.3 `task-form-modal.test.tsx` — 5 test (✅ all passed)

**Test list:**
1. Load HTML từ task → edit → gửi HTML mới ✅
2. Legacy plain text → wrap thành paragraph ✅
3. Clear editor → gửi "" ✅
4. Overlong (2001 char) → lỗi zod hiển thị, aria-invalid + aria-describedby ✅
5. Display on card (text preview 2 line) + read-only view (HTML sanitize, link hardening) ✅
6. Create task với empty description ✅

**Coverage:** Integration flow: modal → editor → form validation → API request; legacy data migration path; empty description; overflow handling; card preview + read-only display.

---

## 3. Kiểm Tra `dangerouslySetInnerHTML`

**Lệnh:**
```bash
grep -rn "dangerouslySetInnerHTML" src --include='*.ts' --include='*.tsx'
```

**Kết quả:**
- **Tổng cộng: 1 chỗ** (đúng theo yêu cầu)
- **Vị trí:** `src/features/tasks/components/rich-text-view.tsx:30`
  ```tsx
  <div
    className={cn("prose-task text-[14px] text-ink-700", className)}
    dangerouslySetInnerHTML={{ __html: clean }}
  />
  ```
- **Defense in depth:** Input `clean` đã qua `sanitizeDescription()` với DOMPurify + allowlist strict
- **ESLint guard:** File chứa `dangerouslySetInnerHTML` không bị cảnh báo (khác file không check)

---

## 4. Kiểm Tra XSS & Edge Case

### 4.1 Ca test mới thêm (11 ca)

| Ca test | Input | Kết quả | Pass |
|---------|-------|--------|------|
| `javascript:` variants | `javascript:alert()`, `JavaScript:void(0)`, newline variant | Tất cả được xóa href | ✅ |
| SVG `onload` | `<svg onload="alert(1)">` | Element bị xóa hoàn toàn | ✅ |
| `<iframe>` | `<iframe src="evil.com"></iframe>` | Xóa, giữ text nếu có | ✅ |
| `<style>` tag | `<style>body{...}</style>` | Xóa, giữ text | ✅ |
| Nested list | `<ul><li>one<ul><li>nested` | Giữ lại structure | ✅ |
| Entity decoding | `&lt;script&gt;` → `<script>` | Đếm đúng 8 code point | ✅ |
| Emoji ZWJ | 👨‍👩‍👧‍👦 (family) | Đếm >1 code point (khác single emoji) | ✅ |
| mailto valid | `mailto:valid@example.com` | Giữ, ép rel/target | ✅ |
| Relative link | `/path/to/page` | Xóa href | ✅ |
| Quote escape | `&<>"'` | Escaped as `&amp;&lt;&gt;"'` | ✅ |
| CRLF normalization | `Line1\r\nLine2` | Thành `<br>` | ✅ |

### 4.2 Không phát hiện lỗi XSS

- DOMPurify config: strict allowlist, chỉ cho p/br/strong/em/u/s/ul/ol/li/a
- `afterSanitizeAttributes` hook: ép `rel="noopener noreferrer nofollow"` + `target="_blank"` trên link
- Kết luận: **Xanh, không có lỗ hổng XSS**

---

## 5. Kiểm Tra Accessibility

### 5.1 Editor Component
- ✅ `role="textbox"` + `aria-multiline="true"`
- ✅ `aria-labelledby="desc-label"` trỏ tới label "Mô tả"
- ✅ `aria-invalid` thay đổi theo state
- ✅ `aria-describedby` gắn tới lỗi validation khi có

### 5.2 Toolbar
- ✅ `role="toolbar"` + `aria-label="Định dạng mô tả"`
- ✅ Button có `aria-label` tiếng Việt (Đậm, Nghiêng, Gạch chân, etc.)
- ✅ `aria-pressed={true/false}` phản ánh mark active
- ✅ Arrow key navigation (Left/Right giữa button, Escape về editor)

### 5.3 Character Counter
- ✅ `aria-live="polite"` trên span hiển thị `count/2000`
- ✅ Cập nhật live khi gõ (screen reader đọc)

### 5.4 Link Input Panel
- ✅ `aria-label="Địa chỉ liên kết"` trên input
- ✅ `aria-invalid` + `aria-describedby` khi validation fail
- ✅ Error message có `role="alert"`

**Kết luận:** ✅ **A11y compliant**

---

## 6. Các Lỗi & Vấn Đề Tìm Được

### 6.1 Không phát hiện lỗi code

Tất cả 802 test xanh, không có regression, coverage tốt (87.9% overall, 100% cho `rich-text.ts`).

### 6.2 Hành vi DOMPurify cần lưu ý

Những điểm sau **không phải lỗi** nhưng cần awareness:

| Điểm | Hành vi | Reason | Status |
|------|--------|--------|--------|
| Empty tag `<p></p>` | Giữ lại | DOMPurify với `KEEP_CONTENT:true` không xóa empty | Expected |
| Emoji ZWJ sequence | Đếm >1 code point | `Array.from()` tách ZWJ joiner | Expected (API cũng đếm như vậy) |
| Quote char | Không escape | DOMPurify render literal `"'` | Expected (không XSS risk) |

Tất cả hành vi này **match API server** (Go `plainTextLength` cũng đếm ZWJ như này), không cần fix.

---

## 7. Validation Schema Kiểm Tra

**File:** `apps/web/src/features/tasks/schemas/task-schemas.ts`

Schema zod cho `description`:
```ts
description: z.string()
  .max(20000, "Mô tả quá dài, hãy bỏ bớt định dạng")
  .refine(v => plainTextLength(v) <= 2000, "Mô tả tối đa 2000 ký tự")
```

✅ **Test xanh:**
- Max 20000 byte HTML → error if overflow
- Max 2000 character text (code point) → error if overflow
- Empty string "" → pass
- 2001 character → error hiển thị, modal gắn aria-describedby

---

## 8. Compliance với Success Criteria Phase 5

### AC10: Tạo/sửa việc với rich text
- ✅ Toolbar đủ 7 nút: Bold, Italic, Underline, Strike, BulletList, OrderedList, Link
- ✅ Link panel: ô nhập URL inline, validate http(s)/mailto, reject javascript:
- ✅ Phím tắt TipTap hoạt động (Ctrl+B/I/U, Ctrl+Shift+8/7 list)
- ✅ Paste: HTML lạ (heading, img) được ép về allowlist
- ✅ Gửi HTML subset, mở lại thấy đúng định dạng ✅
- ✅ Mô tả rỗng gửi "" ✅

### AC11: XSS & dangerouslySetInnerHTML
- ✅ `rich-text.test.ts`: 22 test XSS xanh (script, onclick, event handler, javascript:, data:, svg, iframe, style)
- ✅ `dangerouslySetInnerHTML` chỉ 1 chỗ: `rich-text-view.tsx:30`
- ✅ DOMPurify + allowlist strict trên server (Phase 2 bluemonday)

### Editor chunk lazy
- ✅ `task-description-editor-BtPEJX9n.js` (396KB) tách riêng
- ✅ Trang bảng không tải TipTap khi chưa mở modal

### A11y
- ✅ Toolbar `role="toolbar"`, button `aria-label` + `aria-pressed`
- ✅ Editor `textbox` `aria-labelledby`, `aria-invalid`, `aria-describedby`
- ✅ Counter `aria-live="polite"`

### Test web xanh
- ✅ 802 test pass (tăng 11 test edge case)
- ✅ jsdom polyfill ProseMirror layout: ✅ (Range.getClientRects, Element.getClientRects)

---

## 9. Các Lệnh & Kết Quả

### Build
```bash
npm run build
✓ built in 1.13s
```

### Test
```bash
npm run test
Test Files  98 passed (98)
Tests  802 passed | 3 skipped (805)
Duration  24.54s
```

### Coverage
```bash
npm run test:coverage
Statements   : 87.9% ( 6220/7076 )
Branches     : 80.89% ( 4128/5103 )
Functions    : 85.58% ( 2162/2526 )
Lines        : 88.37% ( 5884/6658 )
```

### Lint
```bash
npm run lint
✖ 6 problems (0 errors, 6 warnings) [toàn cũ]
```

### Typecheck
```bash
npm run typecheck
[no error]
```

---

## 10. Tóm Tắt

| Khía cạnh | Kết quả | Ghi chú |
|-----------|---------|---------|
| **Lint** | ✅ 0 error | 6 warning cũ |
| **Type** | ✅ Pass | Strict mode |
| **Test** | ✅ 802 pass | +11 edge case XSS |
| **Build** | ✅ Pass | Chunk editor lazy 396KB |
| **Coverage** | ✅ 87.9% | `rich-text.ts` 100% |
| **XSS** | ✅ Safe | DOMPurify + strict allowlist |
| **A11y** | ✅ Full | toolbar, textbox, live region |
| **Edge case** | ✅ Pass | Entity, emoji, nested tag, mailto |

---

## 11. Kiến Nghị & Lưu Ý

### Không có concern blocking
- Tất cả gate pass
- Không phát hiện regression
- XSS defense in depth: server (bluemonday) + client (DOMPurify)

### Lưu ý cho Phase 6 (API migration)
- Schema `task-schemas.ts` limit 20000 HTML + plainText 2000 **phải deploy cùng API** (Phase 6) để không break old client
- Dữ liệu legacy (plain text chưa migrate) được handle bởi `normalizeIncoming` ✅

### Testing note
- ProseMirror trong jsdom: polyfill Range/Element layout methods hoạt động tốt
- `userEvent.type` hoạt động với editor

---

**Status:** DONE ✅  
**Summary:** Phase 5 Web Rich-Text Editor đạt completion criteria. Tất cả test, type, lint, build xanh; coverage 87.9%; XSS defense tốt; A11y compliant; 11 test edge case XSS mới thêm để bao phủ ca lạ (javascript: variant, ZWJ emoji, nested tag, malformed HTML, etc.). Sẵn sàng merge.

**Concerns/Blockers:** Không có
