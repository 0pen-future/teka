---
title: "Phase 5: editor TipTap cho mô tả việc + RichTextView"
date: 2026-09-17
summary: "Web tasks: TipTap lazy, DOMPurify hook khớp bluemonday, roving toolbar, test paste; reviewer 4M/7L đã xử lý"
---

# Phase 5: editor TipTap cho mô tả việc + RichTextView

## Việc đã làm

- Thêm `lib/rich-text.ts` (DOMPurify instance riêng, `normalizeIncoming`, `plainTextLength`, `textFromHtml`), `RichTextView` (nơi duy nhất được `dangerouslySetInnerHTML`, khoá bằng rule `no-restricted-syntax`) và `TaskDescriptionEditor` (TipTap StarterKit cắt heading/blockquote/code, lazy qua `React.lazy` trong `task-form-modal.tsx`).
- Schema zod: text ≤ 2000 code point (khớp Go đếm rune), HTML ≤ 20000.
- Test: `rich-text.test.ts` (22 ca XSS/normalize), `task-description-editor.test.tsx` (10 ca gồm paste HTML lạ, roving tabindex, focus sau Escape), `task-form-modal.test.tsx`, `rich-text-view.test.tsx`; trang bảng mock editor để không nạp TipTap.
- Build tách chunk `task-description-editor-*.js` 396 kB (gzip 125 kB); 808 test xanh; lint 0 lỗi.

## Review

Reviewer DONE_WITH_CONCERNS, 0 Critical/High. Đã sửa hết: read-only bỏ qua `normalizeIncoming` (M1), test trang bảng vẫn nạp TipTap (M2), thiếu test paste (M3), `aria-live` trên bộ đếm đọc lại mỗi phím (M4), `protocols` linkify in cảnh báo (L1), Escape rơi focus về body (L2), toolbar thiếu roving tabindex (L3), polyfill jsdom đè vô điều kiện (L4), thuộc tính link khác bluemonday nên mở-rồi-Lưu PATCH mô tả khác đi (L5).

## Bài học

- TipTap `focus()` hoãn qua `requestAnimationFrame`: test phải `waitFor(toHaveFocus)`.
- jsdom thiếu `Range.getClientRects` và `Document.elementFromPoint`; kiểm bằng `"x" in proto` (widen sang `object` để TS không narrow thành `never`) thay vì `??=` vì rule `unbound-method`.
- `fireEvent.paste` với `clipboardData.getData` giả là đủ để ProseMirror chạy đường paste HTML trong jsdom.

## Còn lại

- Kiểm tay Telex trên Chrome/Android và dán từ Google Docs trước khi ship (Phase 6).
- Phase 6: e2e, docs, hợp đồng v2; backup DB prod trước khi chạy migration 000023.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
